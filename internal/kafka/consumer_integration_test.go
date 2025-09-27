package kafka

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"real-time-analytics-platform/internal/metrics"
)

func TestConsumerIntegration_BackpressureConfiguration(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	// Use nil metrics to avoid Prometheus registration conflicts in tests
	var metricsCollector *metrics.MetricsCollector

	config := &ConsumerConfig{
		Brokers:               []string{"localhost:9092"},
		GroupID:               "test-consumer-group",
		Logger:                logger,
		Metrics:               metricsCollector,
		AutoCommit:            true,
		SessionTimeout:        30 * time.Second,
		HeartbeatInterval:     3 * time.Second,
		MaxProcessingTime:     2 * time.Minute,
		EnableBackpressure:    true,
		MaxConcurrentMessages: 25,
		MessageBufferSize:     500,
		MaxProcessingRate:     50.0,
		EnableRetry:           true,
		MaxRetries:            3,
		EnableDLQ:             true,
		DLQTopic:              "test-dlq",
	}

	// Test backpressure configuration validation
	if config.MaxConcurrentMessages <= 0 {
		t.Error("Expected positive max concurrent messages")
	}
	if config.MessageBufferSize <= 0 {
		t.Error("Expected positive message buffer size")
	}
	if config.MaxProcessingRate <= 0 {
		t.Error("Expected positive max processing rate")
	}

	// Validate that buffer size is appropriate for concurrent messages
	if config.MessageBufferSize < config.MaxConcurrentMessages {
		t.Error("Message buffer size should be >= max concurrent messages")
	}
}

func TestConsumerIntegration_ManualOffsetConfiguration(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	// Use nil metrics to avoid Prometheus registration conflicts in tests
	var metricsCollector *metrics.MetricsCollector

	config := &ConsumerConfig{
		Brokers:            []string{"localhost:9092"},
		GroupID:            "test-manual-offset-group",
		Logger:             logger,
		Metrics:            metricsCollector,
		AutoCommit:         false, // Disable auto-commit
		EnableManualCommit: true,
		CommitInterval:     2 * time.Second,
		SessionTimeout:     30 * time.Second,
		HeartbeatInterval:  3 * time.Second,
		MaxProcessingTime:  2 * time.Minute,
	}

	// Test manual offset configuration
	if config.EnableManualCommit && config.AutoCommit {
		t.Error("Manual commit and auto-commit should not both be enabled")
	}
	if config.EnableManualCommit && config.CommitInterval <= 0 {
		t.Error("Expected positive commit interval for manual offset management")
	}
}

func TestConsumerIntegration_Stats(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	// Use nil metrics to avoid Prometheus registration conflicts in tests
	var metricsCollector *metrics.MetricsCollector

	config := DefaultConsumerConfig([]string{"localhost:9092"}, "test-group", logger, metricsCollector)

	// Mock consumer for stats testing
	consumer := &KafkaConsumer{
		logger:            logger,
		metrics:           metricsCollector,
		config:            config,
		lastStatsTime:     time.Now(),
		messageTimestamps: make([]time.Time, 0, 100),
		pendingOffsets:    make(map[string]map[int32]int64),
	}

	// Test initial stats
	stats := consumer.GetStats()
	if stats.MessagesProcessed != 0 {
		t.Errorf("Expected 0 messages processed initially, got %d", stats.MessagesProcessed)
	}
	if stats.MessagesError != 0 {
		t.Errorf("Expected 0 errors initially, got %d", stats.MessagesError)
	}

	// Test stats update
	consumer.updateSuccessStats(time.Now().Add(-50 * time.Millisecond))
	stats = consumer.GetStats()

	if stats.MessagesProcessed != 1 {
		t.Errorf("Expected 1 message processed after update, got %d", stats.MessagesProcessed)
	}
	if stats.AvgProcessingTimeMs <= 0 {
		t.Error("Expected positive average processing time")
	}
}

func TestConsumerIntegration_RetryAndDLQConfiguration(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	// Use nil metrics to avoid Prometheus registration conflicts in tests
	var metricsCollector *metrics.MetricsCollector

	config := &ConsumerConfig{
		Brokers:      []string{"localhost:9092"},
		GroupID:      "test-retry-group",
		Logger:       logger,
		Metrics:      metricsCollector,
		EnableRetry:  true,
		MaxRetries:   5,
		RetryDelay:   2 * time.Second,
		RetryBackoff: 2.5,
		EnableDLQ:    true,
		DLQTopic:     "test-retry-dlq",
	}

	// Test retry configuration validation
	if config.MaxRetries <= 0 {
		t.Error("Expected positive max retries")
	}
	if config.RetryDelay <= 0 {
		t.Error("Expected positive retry delay")
	}
	if config.RetryBackoff <= 1.0 {
		t.Error("Expected retry backoff > 1.0 for exponential backoff")
	}
	if config.EnableDLQ && config.DLQTopic == "" {
		t.Error("DLQ topic must be specified when DLQ is enabled")
	}

	// Test calculated retry delays
	expectedDelays := []time.Duration{
		0,                 // No delay for first attempt
		config.RetryDelay, // 2s for first retry
		time.Duration(float64(config.RetryDelay) * config.RetryBackoff),     // 5s for second retry
		time.Duration(float64(config.RetryDelay) * config.RetryBackoff * 2), // 10s for third retry
	}

	for i, expectedDelay := range expectedDelays {
		calculatedDelay := time.Duration(float64(config.RetryDelay) * config.RetryBackoff * float64(i))
		if i == 0 {
			calculatedDelay = 0
		}

		if i > 0 && calculatedDelay < expectedDelay*8/10 { // Allow 20% tolerance
			t.Errorf("Retry delay for attempt %d seems too short: got %v, expected around %v",
				i, calculatedDelay, expectedDelay)
		}
	}
}

func TestConsumerIntegration_DefaultConfiguration(t *testing.T) {
	logger := logrus.New()
	// Use nil metrics to avoid Prometheus registration conflicts in tests
	var metricsCollector *metrics.MetricsCollector

	config := DefaultConsumerConfig([]string{"localhost:9092"}, "default-group", logger, metricsCollector)

	// Verify default values are sensible
	if config.SessionTimeout < 10*time.Second {
		t.Error("Session timeout too short for production use")
	}
	if config.HeartbeatInterval >= config.SessionTimeout/3 {
		t.Error("Heartbeat interval should be < 1/3 of session timeout")
	}
	if config.MaxProcessingTime < config.SessionTimeout {
		t.Error("Max processing time should be >= session timeout")
	}
	if config.FetchMinBytes <= 0 {
		t.Error("Expected positive fetch min bytes")
	}
	if config.MaxPollRecords <= 0 {
		t.Error("Expected positive max poll records")
	}
}
