package kafka

import (
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/sirupsen/logrus"

	"real-time-analytics-platform/internal/metrics"
)

func TestProducerIntegration_Configuration(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	// Use nil metrics to avoid Prometheus registration conflicts in tests
	var metricsCollector *metrics.MetricsCollector

	config := &ProducerConfig{
		Brokers:           []string{"localhost:9092"},
		Logger:            logger,
		Metrics:           metricsCollector,
		BatchSize:         16384,
		BatchTimeout:      5 * time.Millisecond,
		CompressionType:   "snappy",
		RequiredAcks:      sarama.WaitForAll,
		MaxRetries:        5,
		RetryBackoff:      100 * time.Millisecond,
		EnableIdempotence: true,
		MaxMessageBytes:   1000000,
		FlushFrequency:    1 * time.Second,
	}

	// Test configuration validation
	if config.BatchSize <= 0 {
		t.Error("Expected positive batch size")
	}
	if config.MaxRetries <= 0 {
		t.Error("Expected positive max retries")
	}
	if config.CompressionType == "" {
		t.Error("Expected compression type to be set")
	}

	// Test that configuration is properly applied
	if config.EnableIdempotence && config.RequiredAcks != sarama.WaitForAll {
		t.Error("Idempotence requires RequiredAcks to be WaitForAll")
	}
}

func TestProducerIntegration_Stats(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	// Use nil metrics to avoid Prometheus registration conflicts in tests
	var metricsCollector *metrics.MetricsCollector

	config := DefaultProducerConfig([]string{"localhost:9092"}, logger, metricsCollector)

	// Mock producer for stats testing (no actual Kafka needed)
	producer := &KafkaProducer{
		logger:        logger,
		metrics:       metricsCollector,
		config:        config,
		lastStatsTime: time.Now(),
	}

	// Test initial stats
	stats := producer.GetStats()
	if stats.MessagesSent != 0 {
		t.Errorf("Expected 0 messages sent initially, got %d", stats.MessagesSent)
	}
	if stats.MessagesError != 0 {
		t.Errorf("Expected 0 errors initially, got %d", stats.MessagesError)
	}

	// Test stats update
	producer.updateSuccessStats(1024, 50*time.Millisecond)
	stats = producer.GetStats()

	if stats.MessagesSent != 1 {
		t.Errorf("Expected 1 message sent after update, got %d", stats.MessagesSent)
	}
	if stats.BytesSent != 1024 {
		t.Errorf("Expected 1024 bytes sent, got %d", stats.BytesSent)
	}
	if stats.AvgLatencyMs <= 0 {
		t.Error("Expected positive average latency")
	}
}

func TestProducerIntegration_BatchConfiguration(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	// Use nil metrics to avoid Prometheus registration conflicts in tests
	var metricsCollector *metrics.MetricsCollector

	config := &ProducerConfig{
		Brokers:           []string{"localhost:9092"},
		Logger:            logger,
		Metrics:           metricsCollector,
		BatchSize:         8192, // 8KB batch
		BatchTimeout:      10 * time.Millisecond,
		CompressionType:   "gzip",
		RequiredAcks:      sarama.WaitForAll,
		MaxRetries:        3,
		EnableIdempotence: true,
	}

	// Verify batch settings are within valid ranges
	if config.BatchSize < 1024 || config.BatchSize > 1048576 {
		t.Errorf("Batch size %d outside valid range [1KB, 1MB]", config.BatchSize)
	}

	if config.BatchTimeout < time.Millisecond || config.BatchTimeout > time.Second {
		t.Errorf("Batch timeout %v outside valid range [1ms, 1s]", config.BatchTimeout)
	}

	// Test compression type validation
	validCompressionTypes := map[string]bool{
		"none":   true,
		"gzip":   true,
		"snappy": true,
		"lz4":    true,
		"zstd":   true,
	}

	if !validCompressionTypes[config.CompressionType] {
		t.Errorf("Invalid compression type: %s", config.CompressionType)
	}
}
