package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	pb "real-time-analytics-platform/api/proto"
	grpcServer "real-time-analytics-platform/internal/grpc"
	"real-time-analytics-platform/internal/metrics"
	"real-time-analytics-platform/pkg/config"
)

type AnalyticsService struct {
	pb.UnimplementedAnalyticsServiceServer
	logger    *logrus.Logger
	metrics   *metrics.MetricsCollector
	config    *config.Config
	analytics *AnalyticsData
}

type AnalyticsData struct {
	mutex        sync.RWMutex
	TotalEvents  int64
	TotalUsers   int64
	EventsByType map[string]int64
	LastUpdated  time.Time
}

func main() {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	logger.SetFormatter(&logrus.JSONFormatter{})

	cfg, err := config.Load()
	if err != nil {
		logger.WithError(err).Fatal("Failed to load config")
	}

	// Initialize metrics
	metricsCollector := metrics.NewMetricsCollector("analytics")

	// Initialize analytics data
	analyticsData := &AnalyticsData{
		EventsByType: make(map[string]int64),
		LastUpdated:  time.Now(),
	}

	// Create analytics service
	analyticsService := &AnalyticsService{
		logger:    logger,
		metrics:   metricsCollector,
		config:    cfg,
		analytics: analyticsData,
	}

	// Start gRPC server
	grpcSrv, err := grpcServer.NewServer(&grpcServer.ServerConfig{
		Port:    cfg.GRPC.Port,
		Logger:  logger,
		Metrics: metricsCollector,
	})
	if err != nil {
		logger.WithError(err).Fatal("Failed to create gRPC server")
	}

	pb.RegisterAnalyticsServiceServer(grpcSrv.GetGRPCServer(), analyticsService)

	// Start HTTP server
	httpServer := startHTTPServer(cfg.HTTP.Port, analyticsService, logger)

	// Start metrics server
	metricsServer := metrics.StartMetricsServer("9090")

	// Start background analytics processor
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	// Start gRPC server
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := grpcSrv.Serve(); err != nil {
			logger.WithError(err).Error("gRPC server error")
		}
	}()

	// Start analytics processor
	wg.Add(1)
	go func() {
		defer wg.Done()
		analyticsService.runAnalyticsProcessor(ctx)
	}()

	logger.Info("Analytics service started successfully")

	// Wait for interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	logger.Info("Shutting down analytics service...")

	cancel()
	grpcSrv.Stop()
	httpServer.Shutdown(context.Background())
	metricsServer.Shutdown(context.Background())

	wg.Wait()
	logger.Info("Analytics service stopped")
}

func startHTTPServer(port string, service *AnalyticsService, logger *logrus.Logger) *http.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"healthy","service":"analytics","timestamp":"%s"}`, time.Now().Format(time.RFC3339))
	})

	mux.HandleFunc("/analytics/summary", service.handleHTTPSummary)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		logger.WithField("port", port).Info("Starting HTTP server")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.WithError(err).Fatal("HTTP server error")
		}
	}()

	return server
}
