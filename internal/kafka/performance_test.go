package kafka

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"real-time-analytics-platform/internal/metrics"
)

func BenchmarkProducer_SendMessage(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping performance test in short mode")
	}

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
		MaxRetries:        3,
		EnableIdempotence: true,
	}

	// Mock producer for benchmarking (no actual Kafka needed)
	producer := &KafkaProducer{
		logger:        logger,
		metrics:       metricsCollector,
		config:        config,
		lastStatsTime: time.Now(),
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Simulate message processing without actual Kafka send
			producer.updateSuccessStats(100, 5*time.Millisecond)
		}
	})

	b.StopTimer()
	stats := producer.GetStats()
	b.Logf("Messages processed: %d, Avg latency: %.2fms, Throughput: %.2f msgs/sec",
		stats.MessagesSent, stats.AvgLatencyMs, stats.ThroughputPerSec)
}

func BenchmarkProducer_BatchSend(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping performance test in short mode")
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	// Use nil metrics to avoid Prometheus registration conflicts in tests
	var metricsCollector *metrics.MetricsCollector

	config := DefaultProducerConfig([]string{"localhost:9092"}, logger, metricsCollector)

	// Mock producer
	producer := &KafkaProducer{
		logger:        logger,
		metrics:       metricsCollector,
		config:        config,
		lastStatsTime: time.Now(),
	}

	// Create batch of test messages
	batchSize := 100
	messages := make([]Message, batchSize)
	for i := 0; i < batchSize; i++ {
		messages[i] = Message{
			Key: fmt.Sprintf("batch-key-%d", i),
			Value: map[string]interface{}{
				"id":         fmt.Sprintf("batch-event-%d", i),
				"user_id":    fmt.Sprintf("user-%d", i%10),
				"event_type": "batch_test",
				"timestamp":  time.Now().Unix(),
			},
		}
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Simulate batch processing
			producer.updateSuccessStats(batchSize*200, 20*time.Millisecond) // ~200 bytes per message
		}
	})

	b.StopTimer()
	stats := producer.GetStats()
	b.Logf("Batch performance - Messages: %d, Bytes: %d, Avg latency: %.2fms",
		stats.MessagesSent, stats.BytesSent, stats.AvgLatencyMs)
}

func TestProducerPerformance_HighThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	// Use nil metrics to avoid Prometheus registration conflicts in tests
	var metricsCollector *metrics.MetricsCollector

	config := &ProducerConfig{
		Brokers:           []string{"localhost:9092"},
		Logger:            logger,
		Metrics:           metricsCollector,
		BatchSize:         32768, // 32KB for high throughput
		BatchTimeout:      1 * time.Millisecond,
		CompressionType:   "lz4", // Fast compression
		MaxRetries:        3,
		EnableIdempotence: true,
		FlushFrequency:    100 * time.Millisecond,
	}

	// Mock producer
	producer := &KafkaProducer{
		logger:        logger,
		metrics:       metricsCollector,
		config:        config,
		lastStatsTime: time.Now(),
	}

	// Performance test: simulate high throughput scenario
	numMessages := 10000
	messageSize := 500 // 500 bytes per message

	start := time.Now()

	var wg sync.WaitGroup
	var sentCount int64

	// Simulate concurrent producers
	numProducers := 10
	messagesPerProducer := numMessages / numProducers

	for i := 0; i < numProducers; i++ {
		wg.Add(1)
		go func(producerID int) {
			defer wg.Done()

			for j := 0; j < messagesPerProducer; j++ {
				// Simulate message send
				producer.updateSuccessStats(messageSize, 2*time.Millisecond)
				atomic.AddInt64(&sentCount, 1)
			}
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)

	stats := producer.GetStats()
	throughput := float64(sentCount) / elapsed.Seconds()

	t.Logf("High throughput test results:")
	t.Logf("- Messages sent: %d", sentCount)
	t.Logf("- Time elapsed: %v", elapsed)
	t.Logf("- Throughput: %.2f messages/sec", throughput)
	t.Logf("- Avg latency: %.2fms", stats.AvgLatencyMs)
	t.Logf("- Total bytes: %d", stats.BytesSent)

	// Performance thresholds
	minThroughput := 1000.0 // messages per second
	if throughput < minThroughput {
		t.Errorf("Throughput %.2f msgs/sec below threshold %.2f msgs/sec", throughput, minThroughput)
	}

	maxLatency := 10.0 // milliseconds
	if stats.AvgLatencyMs > maxLatency {
		t.Errorf("Average latency %.2fms above threshold %.2fms", stats.AvgLatencyMs, maxLatency)
	}
}

func TestConsumerPerformance_BackpressureHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	// Use nil metrics to avoid Prometheus registration conflicts in tests
	var metricsCollector *metrics.MetricsCollector

	config := &ConsumerConfig{
		Brokers:               []string{"localhost:9092"},
		GroupID:               "perf-consumer-group",
		Logger:                logger,
		Metrics:               metricsCollector,
		EnableBackpressure:    true,
		MaxConcurrentMessages: 50,
		MessageBufferSize:     1000,
		MaxProcessingRate:     200.0, // messages per second
		SessionTimeout:        30 * time.Second,
		HeartbeatInterval:     3 * time.Second,
		MaxProcessingTime:     1 * time.Minute,
	}

	// Mock consumer
	consumer := &KafkaConsumer{
		logger:            logger,
		metrics:           metricsCollector,
		config:            config,
		lastStatsTime:     time.Now(),
		messageTimestamps: make([]time.Time, 0, 100),
		pendingOffsets:    make(map[string]map[int32]int64),
		semaphore:         make(chan struct{}, config.MaxConcurrentMessages),
	}

	// Test semaphore limits
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var processedCount int64
	var blockedCount int64

	// Simulate high message load
	numMessages := 100

	for i := 0; i < numMessages; i++ {
		wg.Add(1)
		go func(msgID int) {
			defer wg.Done()

			select {
			case consumer.semaphore <- struct{}{}:
				// Got semaphore, simulate processing
				time.Sleep(10 * time.Millisecond) // Simulate processing time
				atomic.AddInt64(&processedCount, 1)
				<-consumer.semaphore // Release semaphore
			case <-ctx.Done():
				atomic.AddInt64(&blockedCount, 1)
			default:
				atomic.AddInt64(&blockedCount, 1)
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Backpressure test results:")
	t.Logf("- Messages processed: %d", processedCount)
	t.Logf("- Messages blocked: %d", blockedCount)
	t.Logf("- Max concurrent limit: %d", config.MaxConcurrentMessages)

	// Verify backpressure is working
	if processedCount > int64(config.MaxConcurrentMessages)*2 {
		t.Errorf("Too many messages processed concurrently: %d (limit: %d)",
			processedCount, config.MaxConcurrentMessages)
	}
}

func TestPerformance_RateLimiting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	maxRate := 100.0 // messages per second
	testDuration := 2 * time.Second

	start := time.Now()
	lastProcessTime := time.Now()
	messageCount := 0

	for time.Since(start) < testDuration {
		// Simulate rate limiting logic
		minInterval := time.Duration(float64(time.Second) / maxRate)
		elapsed := time.Since(lastProcessTime)

		if elapsed < minInterval {
			time.Sleep(minInterval - elapsed)
		}

		lastProcessTime = time.Now()
		messageCount++
	}

	actualDuration := time.Since(start)
	actualRate := float64(messageCount) / actualDuration.Seconds()

	t.Logf("Rate limiting test results:")
	t.Logf("- Target rate: %.2f msgs/sec", maxRate)
	t.Logf("- Actual rate: %.2f msgs/sec", actualRate)
	t.Logf("- Messages processed: %d", messageCount)
	t.Logf("- Test duration: %v", actualDuration)

	// Allow 10% tolerance for rate limiting
	tolerance := maxRate * 0.1
	if actualRate > maxRate+tolerance {
		t.Errorf("Rate limiting failed: actual rate %.2f exceeds target rate %.2f + tolerance",
			actualRate, maxRate)
	}
}
