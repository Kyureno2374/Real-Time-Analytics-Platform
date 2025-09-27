package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"

	"real-time-analytics-platform/pkg/config"
)

func main() {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	cfg, err := config.Load()
	if err != nil {
		logger.WithError(err).Fatal("Failed to load config")
	}

	// Override config from environment variables
	if brokers := os.Getenv("KAFKA_BROKERS"); brokers != "" {
		cfg.Kafka.Brokers = []string{brokers}
	}
	if grpcPort := os.Getenv("GRPC_PORT"); grpcPort != "" {
		cfg.GRPC.Port = grpcPort
	}
	if httpPort := os.Getenv("HTTP_PORT"); httpPort != "" {
		cfg.HTTP.Port = httpPort
	}

	logger.WithFields(logrus.Fields{
		"kafka_brokers": cfg.Kafka.Brokers,
		"grpc_port":     cfg.GRPC.Port,
		"http_port":     cfg.HTTP.Port,
	}).Info("Starting consumer service with config")

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		logger.Info("Received shutdown signal")
		cancel()
	}()

	// This would typically start the actual service
	// For now, just wait for shutdown signal
	<-ctx.Done()
	logger.Info("Consumer service shutting down")
}
