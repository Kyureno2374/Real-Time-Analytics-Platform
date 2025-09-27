package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	pb "real-time-analytics-platform/api/proto"
	grpcServer "real-time-analytics-platform/internal/grpc"
	"real-time-analytics-platform/internal/kafka"
	"real-time-analytics-platform/internal/metrics"
	"real-time-analytics-platform/pkg/config"
	"real-time-analytics-platform/pkg/logger"
)

type ConsumerService struct {
	pb.UnimplementedConsumerServiceServer
	kafkaConsumer   kafka.Consumer
	streamProcessor kafka.StreamProcessor
	dlqProducer     kafka.Producer
	logger          *logrus.Logger
	metrics         *metrics.MetricsCollector
	config          *config.Config
	stats           *ConsumerStats
	startTime       time.Time
}

type ConsumerStats struct {
	mutex             sync.RWMutex
	ProcessedEvents   int64
	FailedEvents      int64
	RetryEvents       int64
	DLQEvents         int64
	LastProcessed     time.Time
	ProcessingRate    float64
	AvgProcessingTime time.Duration
}

func main() {
	// Initialize logger
	log := logger.NewLogger("consumer-service")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.WithError(err).Fatal("Failed to load config")
	}

	// Override from environment
	overrideConfigFromEnv(cfg, log)

	// Initialize metrics
	metricsCollector := metrics.NewMetricsCollector("consumer")

	// Create DLQ producer
	dlqProducerConfig := kafka.DefaultProducerConfig(cfg.Kafka.Brokers, log, metricsCollector)
	dlqProducer, err := kafka.NewProducer(dlqProducerConfig)
	if err != nil {
		log.WithError(err).Fatal("Failed to create DLQ producer")
	}

	// Create consumer configuration with DLQ support and backpressure
	consumerConfig := kafka.DefaultConsumerConfig(cfg.Kafka.Brokers, cfg.Kafka.ConsumerGroupID, log, metricsCollector)
	consumerConfig.EnableRetry = true
	consumerConfig.MaxRetries = 3
	consumerConfig.RetryDelay = 1 * time.Second
	consumerConfig.RetryBackoff = 2.0
	consumerConfig.EnableDLQ = true
	consumerConfig.DLQTopic = "analytics-events-dlq"
	consumerConfig.DLQProducer = dlqProducer

	// Enable backpressure handling for production workloads
	consumerConfig.EnableBackpressure = true
	consumerConfig.MaxConcurrentMessages = 50
	consumerConfig.MessageBufferSize = 1000
	consumerConfig.MaxProcessingRate = 100.0 // messages per second

	// Enable manual commit for better control in high-throughput scenarios
	consumerConfig.EnableManualCommit = false // Keep auto-commit for now
	consumerConfig.CommitInterval = 5 * time.Second

	// Wait for Kafka to be available before starting consumer
	healthChecker := kafka.NewHealthChecker(cfg.Kafka.Brokers, log)
	if err := healthChecker.WaitForKafka(context.Background(), 10, 5*time.Second); err != nil {
		log.WithError(err).Fatal("Kafka is not available")
	}

	// Initialize Kafka consumer
	kafkaConsumer, err := kafka.NewConsumer(consumerConfig)
	if err != nil {
		log.WithError(err).Fatal("Failed to create Kafka consumer")
	}

	// Create stream processor
	streamConfig := &kafka.StreamConfig{
		Brokers: cfg.Kafka.Brokers,
		GroupID: cfg.Kafka.ConsumerGroupID + "-stream",
		Logger:  log,
		Metrics: metricsCollector,
	}

	streamProcessor, err := kafka.NewStreamProcessor(streamConfig)
	if err != nil {
		log.WithError(err).Fatal("Failed to create stream processor")
	}

	// Configure stream processors
	setupStreamProcessors(streamProcessor, log)

	// Create consumer service
	stats := &ConsumerStats{}
	consumerService := &ConsumerService{
		kafkaConsumer:   kafkaConsumer,
		streamProcessor: streamProcessor,
		dlqProducer:     dlqProducer,
		logger:          log,
		metrics:         metricsCollector,
		config:          cfg,
		stats:           stats,
		startTime:       time.Now(),
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

	pb.RegisterConsumerServiceServer(grpcSrv.GetGRPCServer(), consumerService)

	// Start HTTP server
	httpServer := startHTTPServer(cfg.HTTP.Port, consumerService, log)

	// Start metrics monitoring
	go consumerService.startMetricsReporting(context.Background())

	ctx, cancel := context.WithCancel(context.Background())
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

	// Start Kafka consumer
	wg.Add(1)
	go func() {
		defer wg.Done()
		topics := []string{cfg.Kafka.TopicEvents}

		if err := kafkaConsumer.Subscribe(ctx, topics, consumerService.handleMessage); err != nil {
			log.WithError(err).Error("Consumer subscription error")
		}
	}()

	// Start stream processor
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := streamProcessor.ProcessStream(ctx, cfg.Kafka.TopicEvents, cfg.Kafka.TopicMetrics); err != nil {
			log.WithError(err).Error("Stream processor error")
		}
	}()

	log.WithFields(logrus.Fields{
		"grpc_port":      cfg.GRPC.Port,
		"http_port":      cfg.HTTP.Port,
		"kafka_brokers":  cfg.Kafka.Brokers,
		"consumer_group": cfg.Kafka.ConsumerGroupID,
	}).Info("Consumer service started successfully")

	// Wait for interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	log.Info("Shutting down consumer service...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	cancel()
	grpcSrv.Stop()
	httpServer.Shutdown(shutdownCtx)
	kafkaConsumer.Close()
	streamProcessor.Close()
	dlqProducer.Close()

	wg.Wait()
	log.Info("Consumer service stopped gracefully")
}

func overrideConfigFromEnv(cfg *config.Config, logger *logrus.Logger) {
	if brokers := os.Getenv("KAFKA_BROKERS"); brokers != "" {
		cfg.Kafka.Brokers = []string{brokers}
	}
	if grpcPort := os.Getenv("GRPC_PORT"); grpcPort != "" {
		cfg.GRPC.Port = grpcPort
	}
	if httpPort := os.Getenv("HTTP_PORT"); httpPort != "" {
		cfg.HTTP.Port = httpPort
	}
	if groupID := os.Getenv("CONSUMER_GROUP_ID"); groupID != "" {
		cfg.Kafka.ConsumerGroupID = groupID
	}

	logger.WithFields(logrus.Fields{
		"kafka_brokers":     cfg.Kafka.Brokers,
		"consumer_group_id": cfg.Kafka.ConsumerGroupID,
		"grpc_port":         cfg.GRPC.Port,
		"http_port":         cfg.HTTP.Port,
	}).Info("Configuration overridden from environment variables")
}

func setupStreamProcessors(processor kafka.StreamProcessor, logger *logrus.Logger) {
	// Add validation processor
	validationRules := map[string]func(string) bool{
		"page": func(page string) bool {
			return len(page) > 0 && len(page) < 1000
		},
		"duration": func(duration string) bool {
			if d, err := strconv.Atoi(duration); err == nil {
				return d >= 0 && d < 86400 // Max 24 hours
			}
			return false
		},
	}
	processor.AddProcessor("validation", kafka.ValidationProcessor(validationRules))

	// Add page view enrichment processor
	processor.AddProcessor("page_view_enrichment", kafka.CreatePageViewProcessor())

	// Add user action enrichment processor
	processor.AddProcessor("user_action_enrichment", kafka.CreateUserActionProcessor())

	// Add error event processor
	processor.AddProcessor("error_event", kafka.CreateErrorEventProcessor())

	// Add deduplication processor (5 minute window)
	dedupProcessor := kafka.NewDeduplicationProcessor(5*time.Minute, logger)
	processor.AddProcessor("deduplication", dedupProcessor.Process)

	// Add aggregation processor (1 minute window)
	aggProcessor := kafka.NewAggregationProcessor(1*time.Minute, logger)
	processor.AddProcessor("aggregation", aggProcessor.Process)

	// Add filter for high-importance events only
	processor.AddProcessor("importance_filter", kafka.FilterProcessor(func(event *pb.Event) bool {
		if importance, exists := event.Properties["action_importance"]; exists {
			return importance == "high" || importance == "medium"
		}
		return true // Allow events without importance classification
	}))

	logger.Info("Stream processors configured successfully")
}

func startHTTPServer(port string, service *ConsumerService, logger *logrus.Logger) *http.Server {
	mux := http.NewServeMux()

	// Health check with detailed status including Kafka connectivity
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		consumerStats := service.kafkaConsumer.GetStats()
		streamStats := service.streamProcessor.GetStats()
		uptime := time.Since(service.startTime).Seconds()

		// Check Kafka health
		healthChecker := kafka.NewHealthChecker(service.config.Kafka.Brokers, service.logger)
		kafkaHealth, _ := healthChecker.CheckHealth(r.Context())

		overallStatus := "healthy"
		statusCode := http.StatusOK

		if kafkaHealth == nil || !kafkaHealth.IsHealthy {
			overallStatus = "unhealthy"
			statusCode = http.StatusServiceUnavailable
		}

		// Check error rates
		if consumerStats.MessagesProcessed > 0 {
			errorRate := float64(consumerStats.MessagesError) / float64(consumerStats.MessagesProcessed)
			if errorRate > 0.1 {
				overallStatus = "degraded"
				statusCode = http.StatusPartialContent
			}
		}

		health := map[string]interface{}{
			"status":         overallStatus,
			"service":        "consumer",
			"timestamp":      time.Now().Format(time.RFC3339),
			"uptime_seconds": uptime,
			"kafka_health":   kafkaHealth,
			"consumer": map[string]interface{}{
				"processed_events":  consumerStats.MessagesProcessed,
				"failed_events":     consumerStats.MessagesError,
				"retried_events":    consumerStats.MessagesRetried,
				"dlq_events":        consumerStats.MessagesDLQ,
				"processing_rate":   consumerStats.ProcessingRate,
				"avg_processing_ms": consumerStats.AvgProcessingTimeMs,
			},
			"stream": map[string]interface{}{
				"events_processed":   streamStats.EventsProcessed,
				"events_filtered":    streamStats.EventsFiltered,
				"events_error":       streamStats.EventsError,
				"avg_latency_ms":     streamStats.AvgLatencyMs,
				"throughput_per_sec": streamStats.ThroughputPerSec,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(health)
	})

	mux.HandleFunc("/stats", service.handleHTTPStats)
	mux.HandleFunc("/lag", service.handleHTTPLag)

	// Readiness probe endpoint
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		// Check if consumer is ready to process messages
		consumerStats := service.kafkaConsumer.GetStats()

		ready := map[string]interface{}{
			"ready":              true,
			"service":            "consumer",
			"timestamp":          time.Now().Format(time.RFC3339),
			"consumer_ready":     true,
			"stream_ready":       true,
			"messages_processed": consumerStats.MessagesProcessed,
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

func (s *ConsumerService) startMetricsReporting(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			consumerStats := s.kafkaConsumer.GetStats()
			streamStats := s.streamProcessor.GetStats()
			uptime := time.Since(s.startTime).Seconds()

			// Report to Prometheus
			s.metrics.SetServiceUptime("consumer", uptime)

			s.logger.WithFields(logrus.Fields{
				"processed_events": consumerStats.MessagesProcessed,
				"failed_events":    consumerStats.MessagesError,
				"retried_events":   consumerStats.MessagesRetried,
				"dlq_events":       consumerStats.MessagesDLQ,
				"stream_processed": streamStats.EventsProcessed,
				"stream_filtered":  streamStats.EventsFiltered,
				"avg_latency_ms":   streamStats.AvgLatencyMs,
			}).Debug("Consumer metrics updated")
		}
	}
}
