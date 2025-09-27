package kafka

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/sirupsen/logrus"

	"real-time-analytics-platform/internal/metrics"
)

type MessageHandler func(ctx context.Context, message *sarama.ConsumerMessage) error
type ErrorHandler func(ctx context.Context, message *sarama.ConsumerMessage, err error) error

type Consumer interface {
	Subscribe(ctx context.Context, topics []string, handler MessageHandler) error
	SubscribeWithErrorHandler(ctx context.Context, topics []string, handler MessageHandler, errorHandler ErrorHandler) error
	Close() error
	GetStats() ConsumerStats
	GetLag() (map[string]map[int32]int64, error)
}

type ConsumerStats struct {
	MessagesProcessed   int64
	MessagesError       int64
	MessagesRetried     int64
	MessagesDLQ         int64
	ProcessingRate      float64
	AvgProcessingTimeMs float64
	LastMessageTime     time.Time
}

type KafkaConsumer struct {
	consumerGroup sarama.ConsumerGroup
	logger        *logrus.Logger
	metrics       *metrics.MetricsCollector
	config        *ConsumerConfig
	handler       MessageHandler
	errorHandler  ErrorHandler
	ready         chan bool
	dlqProducer   Producer

	// Stats tracking
	statsMutex        sync.RWMutex
	stats             ConsumerStats
	lastStatsTime     time.Time
	messageTimestamps []time.Time

	// Manual offset management
	pendingOffsets map[string]map[int32]int64
	offsetsMutex   sync.RWMutex
	commitTicker   *time.Ticker

	// Backpressure handling
	semaphore       chan struct{} // Limits concurrent message processing
	messageBuffer   chan *sarama.ConsumerMessage
	rateLimiter     *time.Ticker
	lastProcessTime time.Time
}

type ConsumerConfig struct {
	Brokers           []string
	GroupID           string
	Logger            *logrus.Logger
	Metrics           *metrics.MetricsCollector
	AutoCommit        bool
	OffsetOldest      bool
	SessionTimeout    time.Duration
	HeartbeatInterval time.Duration
	MaxProcessingTime time.Duration

	// Retry configuration
	EnableRetry  bool
	MaxRetries   int
	RetryDelay   time.Duration
	RetryBackoff float64

	// Dead Letter Queue
	EnableDLQ   bool
	DLQTopic    string
	DLQProducer Producer

	// Consumer tuning
	FetchMinBytes  int32
	FetchMaxWait   time.Duration
	MaxPollRecords int

	// Manual offset management
	EnableManualCommit bool
	CommitInterval     time.Duration

	// Backpressure handling
	EnableBackpressure    bool
	MaxConcurrentMessages int
	MessageBufferSize     int
	MaxProcessingRate     float64 // messages per second
}

type consumerGroupHandler struct {
	consumer *KafkaConsumer
}

func NewConsumer(config *ConsumerConfig) (Consumer, error) {
	kafkaConfig := sarama.NewConfig()
	kafkaConfig.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin
	kafkaConfig.Consumer.Return.Errors = true

	// Consumer settings
	kafkaConfig.Consumer.Group.Session.Timeout = config.SessionTimeout
	kafkaConfig.Consumer.Group.Heartbeat.Interval = config.HeartbeatInterval
	kafkaConfig.Consumer.MaxProcessingTime = config.MaxProcessingTime

	// Fetch settings
	kafkaConfig.Consumer.Fetch.Min = config.FetchMinBytes
	kafkaConfig.Consumer.Fetch.Max = 1024 * 1024 // 1MB
	kafkaConfig.Consumer.MaxWaitTime = config.FetchMaxWait

	// Offset settings
	if config.OffsetOldest {
		kafkaConfig.Consumer.Offsets.Initial = sarama.OffsetOldest
	} else {
		kafkaConfig.Consumer.Offsets.Initial = sarama.OffsetNewest
	}

	if config.AutoCommit && !config.EnableManualCommit {
		kafkaConfig.Consumer.Offsets.AutoCommit.Enable = true
		kafkaConfig.Consumer.Offsets.AutoCommit.Interval = 1 * time.Second
	} else {
		// Disable auto-commit for manual offset management
		kafkaConfig.Consumer.Offsets.AutoCommit.Enable = false
	}

	// Version
	kafkaConfig.Version = sarama.V3_0_0_0

	consumerGroup, err := sarama.NewConsumerGroup(config.Brokers, config.GroupID, kafkaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer group: %w", err)
	}

	consumer := &KafkaConsumer{
		consumerGroup:     consumerGroup,
		logger:            config.Logger,
		metrics:           config.Metrics,
		config:            config,
		ready:             make(chan bool),
		dlqProducer:       config.DLQProducer,
		lastStatsTime:     time.Now(),
		messageTimestamps: make([]time.Time, 0, 100),

		// Manual offset management
		pendingOffsets: make(map[string]map[int32]int64),

		// Backpressure handling
		semaphore:     make(chan struct{}, config.MaxConcurrentMessages),
		messageBuffer: make(chan *sarama.ConsumerMessage, config.MessageBufferSize),
	}

	return consumer, nil
}

func (c *KafkaConsumer) Subscribe(ctx context.Context, topics []string, handler MessageHandler) error {
	return c.SubscribeWithErrorHandler(ctx, topics, handler, c.defaultErrorHandler)
}

func (c *KafkaConsumer) SubscribeWithErrorHandler(ctx context.Context, topics []string, handler MessageHandler, errorHandler ErrorHandler) error {
	c.handler = handler
	c.errorHandler = errorHandler

	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if err := c.consumerGroup.Consume(ctx, topics, &consumerGroupHandler{consumer: c}); err != nil {
					c.logger.WithError(err).Error("Error from consumer")
					return
				}

				c.ready = make(chan bool)
			}
		}
	}()

	<-c.ready
	c.logger.WithField("topics", topics).Info("Kafka consumer up and running...")

	// Start error monitoring goroutine
	go func() {
		for err := range c.consumerGroup.Errors() {
			c.updateErrorStats()
			c.metrics.IncrementErrors("consumer", "group_error")
			c.logger.WithError(err).Error("Consumer group error")
		}
	}()

	wg.Wait()
	return nil
}

func (c *KafkaConsumer) Close() error {
	close(c.ready)
	return c.consumerGroup.Close()
}

func (c *KafkaConsumer) GetStats() ConsumerStats {
	c.statsMutex.RLock()
	defer c.statsMutex.RUnlock()

	// Calculate processing rate
	elapsed := time.Since(c.lastStatsTime).Seconds()
	if elapsed > 0 {
		c.stats.ProcessingRate = float64(c.stats.MessagesProcessed) / elapsed
	}

	return c.stats
}

func (c *KafkaConsumer) GetLag() (map[string]map[int32]int64, error) {
	// Create admin client for lag calculation
	adminConfig := DefaultAdminConfig(c.config.Brokers, c.logger)
	admin, err := NewAdminClient(adminConfig)
	if err != nil {
		c.logger.WithError(err).Error("Failed to create admin client for lag calculation")
		return nil, fmt.Errorf("failed to create admin client: %w", err)
	}
	defer admin.Close()

	// Get actual lag from Kafka Admin API
	lag, err := admin.GetConsumerGroupLag(context.Background(), c.config.GroupID)
	if err != nil {
		c.logger.WithError(err).Error("Failed to get consumer group lag")
		return nil, fmt.Errorf("failed to get consumer group lag: %w", err)
	}

	// Update metrics with lag information
	for topic, partitions := range lag {
		for partition, lagValue := range partitions {
			c.metrics.SetKafkaConsumerLag(topic, fmt.Sprintf("%d", partition), c.config.GroupID, float64(lagValue))
		}
	}

	c.logger.WithField("total_lag_topics", len(lag)).Debug("Consumer lag calculated successfully")
	return lag, nil
}

func (h *consumerGroupHandler) Setup(session sarama.ConsumerGroupSession) error {
	close(h.consumer.ready)

	// Start manual commit worker if enabled
	h.consumer.startManualCommitWorker(session.Context(), session)

	h.consumer.logger.WithFields(logrus.Fields{
		"member_id":     session.MemberID(),
		"generation_id": session.GenerationID(),
		"claims":        session.Claims(),
		"manual_commit": h.consumer.config.EnableManualCommit,
		"backpressure":  h.consumer.config.EnableBackpressure,
	}).Info("Consumer group session setup")

	return nil
}

func (h *consumerGroupHandler) Cleanup(session sarama.ConsumerGroupSession) error {
	h.consumer.logger.Info("Consumer group session cleanup")
	return nil
}

func (h *consumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for message := range claim.Messages() {
		// Backpressure: acquire semaphore before processing
		if h.consumer.config.EnableBackpressure {
			select {
			case h.consumer.semaphore <- struct{}{}:
				// Got semaphore, proceed
			case <-session.Context().Done():
				return nil
			}
		}

		// Rate limiting for backpressure
		if h.consumer.config.EnableBackpressure && h.consumer.config.MaxProcessingRate > 0 {
			minInterval := time.Duration(float64(time.Second) / h.consumer.config.MaxProcessingRate)
			elapsed := time.Since(h.consumer.lastProcessTime)
			if elapsed < minInterval {
				time.Sleep(minInterval - elapsed)
			}
			h.consumer.lastProcessTime = time.Now()
		}

		// Process message in goroutine if backpressure enabled
		if h.consumer.config.EnableBackpressure {
			go h.consumer.processMessageAsync(session, message)
		} else {
			h.consumer.processMessageSync(session, message)
		}
	}

	return nil
}

func (c *KafkaConsumer) processMessageWithRetry(session sarama.ConsumerGroupSession, message *sarama.ConsumerMessage) error {
	var lastErr error

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate backoff delay
			delay := time.Duration(float64(c.config.RetryDelay) * c.config.RetryBackoff * float64(attempt))

			c.logger.WithFields(logrus.Fields{
				"attempt":  attempt,
				"delay_ms": delay.Milliseconds(),
				"topic":    message.Topic,
				"offset":   message.Offset,
			}).Warn("Retrying message processing")

			time.Sleep(delay)
			c.updateRetryStats()
		}

		if err := c.handler(context.Background(), message); err != nil {
			lastErr = err
			c.metrics.IncrementErrors("consumer", fmt.Sprintf("attempt_%d", attempt))

			if attempt == c.config.MaxRetries {
				// Max retries reached, send to DLQ if enabled
				if c.config.EnableDLQ && c.dlqProducer != nil {
					if dlqErr := c.sendToDLQ(message, err); dlqErr != nil {
						c.logger.WithError(dlqErr).Error("Failed to send message to DLQ")
					} else {
						c.updateDLQStats()
						c.logger.WithFields(logrus.Fields{
							"topic":  message.Topic,
							"offset": message.Offset,
						}).Warn("Message sent to DLQ after max retries")
					}
				}
			}
			continue
		}

		// Success
		if attempt > 0 {
			c.logger.WithFields(logrus.Fields{
				"attempt": attempt,
				"topic":   message.Topic,
				"offset":  message.Offset,
			}).Info("Message processed successfully after retry")
		}

		return nil
	}

	return lastErr
}

func (c *KafkaConsumer) defaultErrorHandler(ctx context.Context, message *sarama.ConsumerMessage, err error) error {
	c.logger.WithError(err).WithFields(logrus.Fields{
		"topic":     message.Topic,
		"partition": message.Partition,
		"offset":    message.Offset,
		"key":       string(message.Key),
	}).Error("Default error handler: message processing failed")

	return nil
}

func (c *KafkaConsumer) sendToDLQ(message *sarama.ConsumerMessage, originalErr error) error {
	if c.dlqProducer == nil {
		return fmt.Errorf("DLQ producer not configured")
	}

	dlqMessage := map[string]interface{}{
		"original_topic":     message.Topic,
		"original_partition": message.Partition,
		"original_offset":    message.Offset,
		"original_key":       string(message.Key),
		"original_value":     string(message.Value),
		"error":              originalErr.Error(),
		"timestamp":          time.Now().Unix(),
		"headers":            convertHeaders(message.Headers),
	}

	return c.dlqProducer.SendMessage(context.Background(), c.config.DLQTopic, string(message.Key), dlqMessage)
}

func (c *KafkaConsumer) updateSuccessStats(startTime time.Time) {
	c.statsMutex.Lock()
	defer c.statsMutex.Unlock()

	c.stats.MessagesProcessed++
	c.stats.LastMessageTime = time.Now()

	// Update processing time using exponential moving average
	processingTime := time.Since(startTime)
	alpha := 0.1
	newProcessingMs := float64(processingTime.Milliseconds())

	if c.stats.AvgProcessingTimeMs == 0 {
		c.stats.AvgProcessingTimeMs = newProcessingMs
	} else {
		c.stats.AvgProcessingTimeMs = alpha*newProcessingMs + (1-alpha)*c.stats.AvgProcessingTimeMs
	}

	// Track recent message timestamps for rate calculation
	c.messageTimestamps = append(c.messageTimestamps, time.Now())
	if len(c.messageTimestamps) > 100 {
		c.messageTimestamps = c.messageTimestamps[1:]
	}
}

func (c *KafkaConsumer) updateErrorStats() {
	c.statsMutex.Lock()
	defer c.statsMutex.Unlock()

	c.stats.MessagesError++
}

func (c *KafkaConsumer) updateRetryStats() {
	c.statsMutex.Lock()
	defer c.statsMutex.Unlock()

	c.stats.MessagesRetried++
}

func (c *KafkaConsumer) updateDLQStats() {
	c.statsMutex.Lock()
	defer c.statsMutex.Unlock()

	c.stats.MessagesDLQ++
}

// processMessageSync processes message synchronously (original behavior)
func (c *KafkaConsumer) processMessageSync(session sarama.ConsumerGroupSession, message *sarama.ConsumerMessage) {
	start := time.Now()

	if err := c.processMessageWithRetry(session, message); err != nil {
		c.logger.WithError(err).WithFields(logrus.Fields{
			"topic":     message.Topic,
			"partition": message.Partition,
			"offset":    message.Offset,
		}).Error("Failed to process message after retries")

		// Send to error handler
		if c.errorHandler != nil {
			if handlerErr := c.errorHandler(context.Background(), message, err); handlerErr != nil {
				c.logger.WithError(handlerErr).Error("Error handler failed")
			}
		}
		return
	}

	// Handle offset management
	if c.config.EnableManualCommit {
		c.markPendingOffset(message)
	} else {
		session.MarkMessage(message, "")
	}

	// Update stats
	c.updateSuccessStats(start)
}

// processMessageAsync processes message asynchronously with backpressure
func (c *KafkaConsumer) processMessageAsync(session sarama.ConsumerGroupSession, message *sarama.ConsumerMessage) {
	defer func() {
		// Release semaphore
		if c.config.EnableBackpressure {
			<-c.semaphore
		}
	}()

	start := time.Now()

	if err := c.processMessageWithRetry(session, message); err != nil {
		c.logger.WithError(err).WithFields(logrus.Fields{
			"topic":     message.Topic,
			"partition": message.Partition,
			"offset":    message.Offset,
		}).Error("Failed to process message after retries (async)")

		// Send to error handler
		if c.errorHandler != nil {
			if handlerErr := c.errorHandler(context.Background(), message, err); handlerErr != nil {
				c.logger.WithError(handlerErr).Error("Error handler failed (async)")
			}
		}
		return
	}

	// Handle offset management
	if c.config.EnableManualCommit {
		c.markPendingOffset(message)
	} else {
		session.MarkMessage(message, "")
	}

	// Update stats
	c.updateSuccessStats(start)
}

// markPendingOffset marks offset for manual commit
func (c *KafkaConsumer) markPendingOffset(message *sarama.ConsumerMessage) {
	c.offsetsMutex.Lock()
	defer c.offsetsMutex.Unlock()

	if c.pendingOffsets[message.Topic] == nil {
		c.pendingOffsets[message.Topic] = make(map[int32]int64)
	}

	// Store highest processed offset for each topic/partition
	currentOffset := c.pendingOffsets[message.Topic][message.Partition]
	if message.Offset >= currentOffset {
		c.pendingOffsets[message.Topic][message.Partition] = message.Offset + 1
	}
}

// startManualCommitWorker starts periodic offset commits
func (c *KafkaConsumer) startManualCommitWorker(ctx context.Context, session sarama.ConsumerGroupSession) {
	if !c.config.EnableManualCommit {
		return
	}

	c.commitTicker = time.NewTicker(c.config.CommitInterval)

	go func() {
		defer c.commitTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				c.commitPendingOffsets(session)
				return
			case <-c.commitTicker.C:
				c.commitPendingOffsets(session)
			}
		}
	}()
}

// commitPendingOffsets commits all pending offsets
func (c *KafkaConsumer) commitPendingOffsets(session sarama.ConsumerGroupSession) {
	c.offsetsMutex.Lock()
	defer c.offsetsMutex.Unlock()

	if len(c.pendingOffsets) == 0 {
		return
	}

	// Commit offsets for all topics/partitions
	for topic, partitions := range c.pendingOffsets {
		for partition, offset := range partitions {
			session.MarkOffset(topic, partition, offset, "")
		}
	}

	// Clear pending offsets after commit
	c.pendingOffsets = make(map[string]map[int32]int64)

	c.logger.WithField("committed_topics", len(c.pendingOffsets)).Debug("Manual offset commit completed")
}

func convertHeaders(headers []*sarama.RecordHeader) map[string]string {
	result := make(map[string]string)
	for _, header := range headers {
		result[string(header.Key)] = string(header.Value)
	}
	return result
}

// DefaultConsumerConfig returns consumer config with sensible defaults
func DefaultConsumerConfig(brokers []string, groupID string, logger *logrus.Logger, metrics *metrics.MetricsCollector) *ConsumerConfig {
	return &ConsumerConfig{
		Brokers:           brokers,
		GroupID:           groupID,
		Logger:            logger,
		Metrics:           metrics,
		AutoCommit:        true,
		OffsetOldest:      false,
		SessionTimeout:    30 * time.Second,
		HeartbeatInterval: 3 * time.Second,
		MaxProcessingTime: 2 * time.Minute,

		// Retry settings
		EnableRetry:  true,
		MaxRetries:   3,
		RetryDelay:   1 * time.Second,
		RetryBackoff: 2.0,

		// DLQ settings
		EnableDLQ: true,
		DLQTopic:  "analytics-events-dlq",

		// Fetch settings
		FetchMinBytes:  1024, // 1KB
		FetchMaxWait:   500 * time.Millisecond,
		MaxPollRecords: 500,

		// Manual offset management
		EnableManualCommit: false,
		CommitInterval:     1 * time.Second,

		// Backpressure handling
		EnableBackpressure:    false,
		MaxConcurrentMessages: 100,
		MessageBufferSize:     1000,
		MaxProcessingRate:     100.0, // messages per second
	}
}
