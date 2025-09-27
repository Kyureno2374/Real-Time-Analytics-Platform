package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/sirupsen/logrus"

	"real-time-analytics-platform/internal/metrics"
)

type Producer interface {
	SendMessage(ctx context.Context, topic string, key string, value interface{}) error
	SendMessageWithPartition(ctx context.Context, topic string, partition int32, key string, value interface{}) error
	SendMessageAsync(ctx context.Context, topic string, key string, value interface{}) error
	SendBatchMessages(ctx context.Context, topic string, messages []Message) error
	Flush() error
	Close() error
	GetStats() ProducerStats
}

type Message struct {
	Key     string
	Value   interface{}
	Headers map[string]string
}

type ProducerStats struct {
	MessagesSent     int64
	MessagesError    int64
	BytesSent        int64
	AvgLatencyMs     float64
	ThroughputPerSec float64
}

type KafkaProducer struct {
	syncProducer  sarama.SyncProducer
	asyncProducer sarama.AsyncProducer
	logger        *logrus.Logger
	metrics       *metrics.MetricsCollector
	config        *ProducerConfig

	// Stats tracking
	statsMutex    sync.RWMutex
	stats         ProducerStats
	lastStatsTime time.Time
}

// Helper methods for safe metrics usage
func (p *KafkaProducer) incrementErrors(service, errorType string) {
	if p.metrics != nil {
		p.metrics.IncrementErrors(service, errorType)
	}
}

func (p *KafkaProducer) observeProcessingTime(service, operation string, duration float64) {
	if p.metrics != nil {
		p.metrics.ObserveProcessingTime(service, operation, duration)
	}
}

func (p *KafkaProducer) incrementEventsProcessed(service, operation, status string) {
	if p.metrics != nil {
		p.metrics.IncrementEventsProcessed(service, operation, status)
	}
}

type ProducerConfig struct {
	Brokers           []string
	Logger            *logrus.Logger
	Metrics           *metrics.MetricsCollector
	BatchSize         int
	BatchTimeout      time.Duration
	CompressionType   string
	RequiredAcks      sarama.RequiredAcks
	MaxRetries        int
	RetryBackoff      time.Duration
	EnableIdempotence bool
	MaxMessageBytes   int
	FlushFrequency    time.Duration
}

func NewProducer(config *ProducerConfig) (Producer, error) {
	kafkaConfig := sarama.NewConfig()

	// Producer settings
	kafkaConfig.Producer.RequiredAcks = config.RequiredAcks
	kafkaConfig.Producer.Retry.Max = config.MaxRetries
	kafkaConfig.Producer.Retry.Backoff = config.RetryBackoff
	kafkaConfig.Producer.Return.Successes = true
	kafkaConfig.Producer.Return.Errors = true
	kafkaConfig.Producer.Partitioner = sarama.NewHashPartitioner
	kafkaConfig.Producer.Timeout = 10 * time.Second
	kafkaConfig.Producer.MaxMessageBytes = config.MaxMessageBytes

	// Batching settings
	kafkaConfig.Producer.Flush.Bytes = config.BatchSize
	kafkaConfig.Producer.Flush.Frequency = config.BatchTimeout
	kafkaConfig.Producer.Flush.Messages = 100

	// Compression
	switch config.CompressionType {
	case "gzip":
		kafkaConfig.Producer.Compression = sarama.CompressionGZIP
	case "snappy":
		kafkaConfig.Producer.Compression = sarama.CompressionSnappy
	case "lz4":
		kafkaConfig.Producer.Compression = sarama.CompressionLZ4
	case "zstd":
		kafkaConfig.Producer.Compression = sarama.CompressionZSTD
	default:
		kafkaConfig.Producer.Compression = sarama.CompressionSnappy
	}

	// Idempotence
	if config.EnableIdempotence {
		kafkaConfig.Producer.Idempotent = true
		kafkaConfig.Net.MaxOpenRequests = 1
	}

	// Version
	kafkaConfig.Version = sarama.V3_0_0_0

	// Create sync producer
	syncProducer, err := sarama.NewSyncProducer(config.Brokers, kafkaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create sync kafka producer: %w", err)
	}

	// Create async producer
	asyncProducer, err := sarama.NewAsyncProducer(config.Brokers, kafkaConfig)
	if err != nil {
		syncProducer.Close()
		return nil, fmt.Errorf("failed to create async kafka producer: %w", err)
	}

	producer := &KafkaProducer{
		syncProducer:  syncProducer,
		asyncProducer: asyncProducer,
		logger:        config.Logger,
		metrics:       config.Metrics,
		config:        config,
		lastStatsTime: time.Now(),
	}

	// Start async producer monitoring
	go producer.monitorAsyncProducer()

	return producer, nil
}

func (p *KafkaProducer) SendMessage(ctx context.Context, topic string, key string, value interface{}) error {
	start := time.Now()

	valueBytes, err := json.Marshal(value)
	if err != nil {
		p.updateErrorStats()
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	msg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(key),
		Value: sarama.StringEncoder(valueBytes),
		Headers: []sarama.RecordHeader{
			{
				Key:   []byte("timestamp"),
				Value: []byte(time.Now().Format(time.RFC3339)),
			},
			{
				Key:   []byte("producer_id"),
				Value: []byte("analytics-producer"),
			},
		},
	}

	partition, offset, err := p.syncProducer.SendMessage(msg)
	if err != nil {
		p.updateErrorStats()
		p.metrics.IncrementErrors("producer", "kafka_send")
		p.logger.WithError(err).Error("Failed to send message to kafka")
		return fmt.Errorf("failed to send message: %w", err)
	}

	// Update stats and metrics
	latency := time.Since(start)
	p.updateSuccessStats(len(valueBytes), latency)
	p.metrics.ObserveProcessingTime("producer", "sync_send", latency.Seconds())
	p.metrics.IncrementEventsProcessed("producer", "sync", "success")

	p.logger.WithFields(logrus.Fields{
		"topic":      topic,
		"partition":  partition,
		"offset":     offset,
		"key":        key,
		"latency_ms": latency.Milliseconds(),
		"size_bytes": len(valueBytes),
	}).Debug("Message sent successfully")

	return nil
}

func (p *KafkaProducer) SendMessageWithPartition(ctx context.Context, topic string, partition int32, key string, value interface{}) error {
	start := time.Now()

	valueBytes, err := json.Marshal(value)
	if err != nil {
		p.updateErrorStats()
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	msg := &sarama.ProducerMessage{
		Topic:     topic,
		Partition: partition,
		Key:       sarama.StringEncoder(key),
		Value:     sarama.StringEncoder(valueBytes),
		Headers: []sarama.RecordHeader{
			{
				Key:   []byte("timestamp"),
				Value: []byte(time.Now().Format(time.RFC3339)),
			},
		},
	}

	resultPartition, offset, err := p.syncProducer.SendMessage(msg)
	if err != nil {
		p.updateErrorStats()
		p.metrics.IncrementErrors("producer", "kafka_send")
		p.logger.WithError(err).Error("Failed to send message to kafka")
		return fmt.Errorf("failed to send message: %w", err)
	}

	// Update stats and metrics
	latency := time.Since(start)
	p.updateSuccessStats(len(valueBytes), latency)
	p.metrics.ObserveProcessingTime("producer", "sync_send_partition", latency.Seconds())

	p.logger.WithFields(logrus.Fields{
		"topic":      topic,
		"partition":  resultPartition,
		"offset":     offset,
		"key":        key,
		"latency_ms": latency.Milliseconds(),
	}).Debug("Message sent successfully")

	return nil
}

func (p *KafkaProducer) SendMessageAsync(ctx context.Context, topic string, key string, value interface{}) error {
	valueBytes, err := json.Marshal(value)
	if err != nil {
		p.updateErrorStats()
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	msg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(key),
		Value: sarama.StringEncoder(valueBytes),
		Headers: []sarama.RecordHeader{
			{
				Key:   []byte("timestamp"),
				Value: []byte(time.Now().Format(time.RFC3339)),
			},
			{
				Key:   []byte("async"),
				Value: []byte("true"),
			},
		},
	}

	select {
	case p.asyncProducer.Input() <- msg:
		p.metrics.IncrementEventsProcessed("producer", "async", "queued")
		p.logger.WithFields(logrus.Fields{
			"topic": topic,
			"key":   key,
		}).Debug("Message queued for async sending")
		return nil
	case <-ctx.Done():
		p.updateErrorStats()
		return ctx.Err()
	}
}

func (p *KafkaProducer) SendBatchMessages(ctx context.Context, topic string, messages []Message) error {
	start := time.Now()
	successCount := 0
	totalBytes := 0

	for _, msg := range messages {
		valueBytes, err := json.Marshal(msg.Value)
		if err != nil {
			p.updateErrorStats()
			p.logger.WithError(err).WithField("key", msg.Key).Error("Failed to marshal batch message")
			continue
		}

		headers := []sarama.RecordHeader{
			{
				Key:   []byte("timestamp"),
				Value: []byte(time.Now().Format(time.RFC3339)),
			},
			{
				Key:   []byte("batch"),
				Value: []byte("true"),
			},
		}

		// Add custom headers
		for k, v := range msg.Headers {
			headers = append(headers, sarama.RecordHeader{
				Key:   []byte(k),
				Value: []byte(v),
			})
		}

		kafkaMsg := &sarama.ProducerMessage{
			Topic:   topic,
			Key:     sarama.StringEncoder(msg.Key),
			Value:   sarama.StringEncoder(valueBytes),
			Headers: headers,
		}

		_, _, err = p.syncProducer.SendMessage(kafkaMsg)
		if err != nil {
			p.updateErrorStats()
			p.metrics.IncrementErrors("producer", "batch_send")
			p.logger.WithError(err).WithField("key", msg.Key).Error("Failed to send batch message")
			continue
		}

		successCount++
		totalBytes += len(valueBytes)
	}

	// Update stats and metrics
	latency := time.Since(start)
	p.updateSuccessStats(totalBytes, latency)
	p.metrics.ObserveProcessingTime("producer", "batch_send", latency.Seconds())
	p.metrics.IncrementEventsProcessed("producer", "batch", "success")

	p.logger.WithFields(logrus.Fields{
		"topic":        topic,
		"total_msgs":   len(messages),
		"success_msgs": successCount,
		"failed_msgs":  len(messages) - successCount,
		"total_bytes":  totalBytes,
		"latency_ms":   latency.Milliseconds(),
	}).Info("Batch messages processed")

	if successCount == 0 {
		return fmt.Errorf("failed to send any messages in batch")
	}

	return nil
}

func (p *KafkaProducer) Flush() error {
	// Force flush of async producer
	if p.asyncProducer != nil {
		p.asyncProducer.AsyncClose()

		// Wait for all messages to be sent
		for range p.asyncProducer.Successes() {
			// Drain success channel
		}
		for range p.asyncProducer.Errors() {
			// Drain error channel
		}
	}

	p.logger.Debug("Producer flushed successfully")
	return nil
}

func (p *KafkaProducer) Close() error {
	var errs []error

	if p.asyncProducer != nil {
		if err := p.asyncProducer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close async producer: %w", err))
		}
	}

	if p.syncProducer != nil {
		if err := p.syncProducer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close sync producer: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing producer: %v", errs)
	}

	p.logger.Info("Kafka producer closed successfully")
	return nil
}

func (p *KafkaProducer) GetStats() ProducerStats {
	p.statsMutex.RLock()
	defer p.statsMutex.RUnlock()

	// Calculate throughput
	elapsed := time.Since(p.lastStatsTime).Seconds()
	if elapsed > 0 {
		p.stats.ThroughputPerSec = float64(p.stats.MessagesSent) / elapsed
	}

	return p.stats
}

func (p *KafkaProducer) monitorAsyncProducer() {
	for {
		select {
		case success := <-p.asyncProducer.Successes():
			if success != nil {
				p.updateSuccessStatsAsync(success)
				p.metrics.IncrementEventsProcessed("producer", "async", "success")

				p.logger.WithFields(logrus.Fields{
					"topic":     success.Topic,
					"partition": success.Partition,
					"offset":    success.Offset,
				}).Debug("Async message sent successfully")
			}

		case err := <-p.asyncProducer.Errors():
			if err != nil {
				p.updateErrorStats()
				p.metrics.IncrementErrors("producer", "async_send")

				p.logger.WithError(err.Err).WithFields(logrus.Fields{
					"topic": err.Msg.Topic,
					"key":   string(err.Msg.Key.(sarama.StringEncoder)),
				}).Error("Async message send failed")
			}
		}
	}
}

func (p *KafkaProducer) updateSuccessStats(bytes int, latency time.Duration) {
	p.statsMutex.Lock()
	defer p.statsMutex.Unlock()

	p.stats.MessagesSent++
	p.stats.BytesSent += int64(bytes)

	// Update average latency using exponential moving average
	alpha := 0.1
	newLatencyMs := float64(latency.Milliseconds())
	if p.stats.AvgLatencyMs == 0 {
		p.stats.AvgLatencyMs = newLatencyMs
	} else {
		p.stats.AvgLatencyMs = alpha*newLatencyMs + (1-alpha)*p.stats.AvgLatencyMs
	}
}

func (p *KafkaProducer) updateSuccessStatsAsync(msg *sarama.ProducerMessage) {
	p.statsMutex.Lock()
	defer p.statsMutex.Unlock()

	p.stats.MessagesSent++
	if msg.Value != nil {
		if value, ok := msg.Value.(sarama.StringEncoder); ok {
			p.stats.BytesSent += int64(len(value))
		}
	}
}

func (p *KafkaProducer) updateErrorStats() {
	p.statsMutex.Lock()
	defer p.statsMutex.Unlock()

	p.stats.MessagesError++
}

// DefaultProducerConfig returns producer config with sensible defaults
func DefaultProducerConfig(brokers []string, logger *logrus.Logger, metrics *metrics.MetricsCollector) *ProducerConfig {
	return &ProducerConfig{
		Brokers:           brokers,
		Logger:            logger,
		Metrics:           metrics,
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
}
