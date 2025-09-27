package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/IBM/sarama"
	"github.com/sirupsen/logrus"

	pb "real-time-analytics-platform/api/proto"
	grpcServer "real-time-analytics-platform/internal/grpc"
	"real-time-analytics-platform/internal/kafka"
	"real-time-analytics-platform/internal/metrics"
	"real-time-analytics-platform/pkg/config"
	"real-time-analytics-platform/pkg/logger"
)

type ProducerService struct {
	pb.UnimplementedProducerServiceServer
	kafkaProducer kafka.Producer
	logger        *logrus.Logger
	metrics       *metrics.MetricsCollector
	config        *config.Config
	startTime     time.Time
}

func main() {
	// Initialize logger
	log := logger.NewLogger("producer-service")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.WithError(err).Fatal("Failed to load config")
	}

	// Override from environment variables
	overrideConfigFromEnv(cfg, log)

	// Initialize metrics
	metricsCollector := metrics.NewMetricsCollector("producer")

	// Create advanced Kafka producer configuration
	producerConfig := &kafka.ProducerConfig{
		Brokers:           cfg.Kafka.Brokers,
		Logger:            log,
		Metrics:           metricsCollector,
		BatchSize:         16384, // 16KB
		BatchTimeout:      5 * time.Millisecond,
		CompressionType:   "snappy",
		RequiredAcks:      sarama.WaitForAll,
		MaxRetries:        5,
		RetryBackoff:      100 * time.Millisecond,
		EnableIdempotence: true,
		MaxMessageBytes:   1000000, // 1MB
		FlushFrequency:    1 * time.Second,
	}

	// Wait for Kafka to be available before starting producer
	healthChecker := kafka.NewHealthChecker(cfg.Kafka.Brokers, log)
	if err := healthChecker.WaitForKafka(context.Background(), 10, 5*time.Second); err != nil {
		log.WithError(err).Fatal("Kafka is not available")
	}

	// Initialize Kafka producer
	kafkaProducer, err := kafka.NewProducer(producerConfig)
	if err != nil {
		log.WithError(err).Fatal("Failed to create Kafka producer")
	}

	// Create producer service
	producerService := &ProducerService{
		kafkaProducer: kafkaProducer,
		logger:        log,
		metrics:       metricsCollector,
		config:        cfg,
		startTime:     time.Now(),
	}

	// Start gRPC server
	grpcSrv, err := grpcServer.NewServer(&grpcServer.ServerConfig{
		Port:    cfg.GRPC.Port,
		Logger:  log,
		Metrics: metricsCollector,
	})
	if err != nil {
		log.WithError(err).Fatal("Failed to create gRPC server")
	}

	pb.RegisterProducerServiceServer(grpcSrv.GetGRPCServer(), producerService)

	// Start HTTP server for REST API
	httpServer := startHTTPServer(cfg.HTTP.Port, producerService, log)

	// Start metrics monitoring
	go producerService.startMetricsReporting(context.Background())

	log.WithFields(logrus.Fields{
		"grpc_port":     cfg.GRPC.Port,
		"http_port":     cfg.HTTP.Port,
		"kafka_brokers": cfg.Kafka.Brokers,
	}).Info("Producer service started successfully")

	// Graceful shutdown
	cancel := func() {} // Placeholder for consistency
	defer cancel()

	var wg sync.WaitGroup

	// Start gRPC server
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := grpcSrv.Serve(); err != nil {
			log.WithError(err).Error("gRPC server error")
		}
	}()

	// Wait for interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	log.Info("Shutting down producer service...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Flush pending messages
	if err := kafkaProducer.Flush(); err != nil {
		log.WithError(err).Error("Failed to flush producer")
	}

	// Shutdown servers
	grpcSrv.Stop()
	httpServer.Shutdown(shutdownCtx)
	kafkaProducer.Close()

	wg.Wait()
	log.Info("Producer service stopped gracefully")
}

func overrideConfigFromEnv(cfg *config.Config, logger *logrus.Logger) {
	if brokers := os.Getenv("KAFKA_BROKERS"); brokers != "" {
		cfg.Kafka.Brokers = []string{brokers}
		logger.WithField("kafka_brokers", brokers).Info("Kafka brokers overridden from environment")
	}
	if grpcPort := os.Getenv("GRPC_PORT"); grpcPort != "" {
		cfg.GRPC.Port = grpcPort
	}
	if httpPort := os.Getenv("HTTP_PORT"); httpPort != "" {
		cfg.HTTP.Port = httpPort
	}
	if logLevel := os.Getenv("LOG_LEVEL"); logLevel != "" {
		if level, err := logrus.ParseLevel(logLevel); err == nil {
			logger.SetLevel(level)
		}
	}
}

func startHTTPServer(port string, service *ProducerService, logger *logrus.Logger) *http.Server {
	mux := http.NewServeMux()

	// Health check endpoint with Kafka connectivity
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		stats := service.kafkaProducer.GetStats()
		uptime := time.Since(service.startTime).Seconds()

		// Check Kafka health
		healthChecker := kafka.NewHealthChecker(service.config.Kafka.Brokers, service.logger)
		kafkaHealth, _ := healthChecker.CheckHealth(r.Context())

		overallStatus := "healthy"
		statusCode := http.StatusOK

		if kafkaHealth == nil || !kafkaHealth.IsHealthy {
			overallStatus = "unhealthy"
			statusCode = http.StatusServiceUnavailable
		} else if stats.MessagesSent > 0 {
			// Check error rate
			errorRate := float64(stats.MessagesError) / float64(stats.MessagesSent)
			if errorRate > 0.1 {
				overallStatus = "degraded"
				statusCode = http.StatusPartialContent
			}
		}

		health := map[string]interface{}{
			"status":             overallStatus,
			"service":            "producer",
			"timestamp":          time.Now().Format(time.RFC3339),
			"uptime_seconds":     uptime,
			"kafka_health":       kafkaHealth,
			"messages_sent":      stats.MessagesSent,
			"messages_error":     stats.MessagesError,
			"avg_latency_ms":     stats.AvgLatencyMs,
			"throughput_per_sec": stats.ThroughputPerSec,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(health)
	})

	// Single event endpoint
	mux.HandleFunc("/events", service.handleHTTPEvent)

	// Batch events endpoint
	mux.HandleFunc("/events/batch", service.handleHTTPBatchEvents)

	// Async event endpoint
	mux.HandleFunc("/events/async", service.handleHTTPAsyncEvent)

	// Producer stats endpoint
	mux.HandleFunc("/stats", service.handleHTTPStats)

	// Readiness probe endpoint
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		stats := service.kafkaProducer.GetStats()

		ready := map[string]interface{}{
			"ready":          true,
			"service":        "producer",
			"timestamp":      time.Now().Format(time.RFC3339),
			"producer_ready": true,
			"messages_sent":  stats.MessagesSent,
			"uptime_seconds": time.Since(service.startTime).Seconds(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(ready)
	})

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.WithField("port", port).Info("Starting HTTP server")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.WithError(err).Fatal("HTTP server error")
		}
	}()

	return server
}

func (s *ProducerService) startMetricsReporting(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stats := s.kafkaProducer.GetStats()
			uptime := time.Since(s.startTime).Seconds()

			// Report to Prometheus
			s.metrics.SetServiceUptime("producer", uptime)

			s.logger.WithFields(logrus.Fields{
				"messages_sent":      stats.MessagesSent,
				"messages_error":     stats.MessagesError,
				"bytes_sent":         stats.BytesSent,
				"avg_latency_ms":     stats.AvgLatencyMs,
				"throughput_per_sec": stats.ThroughputPerSec,
			}).Debug("Producer metrics updated")
		}
	}
}
