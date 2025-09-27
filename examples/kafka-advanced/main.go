package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"github.com/sirupsen/logrus"

	pb "real-time-analytics-platform/api/proto"
	"real-time-analytics-platform/internal/kafka"
	"real-time-analytics-platform/internal/metrics"
	"real-time-analytics-platform/pkg/logger"
	"real-time-analytics-platform/pkg/utils"
)

func main() {
	// Initialize logger
	log := logger.NewLogger("kafka-example")
	log.SetLevel(logrus.DebugLevel)

	// Initialize metrics
	metricsCollector := metrics.NewMetricsCollector("example")

	// Example 1: Advanced Producer with batching and compression
	fmt.Println("=== Example 1: Advanced Producer ===")
	if err := runProducerExample(log, metricsCollector); err != nil {
		log.WithError(err).Error("Producer example failed")
	}

	// Example 2: Consumer with retry and DLQ
	fmt.Println("\n=== Example 2: Consumer with Retry/DLQ ===")
	if err := runConsumerExample(log, metricsCollector); err != nil {
		log.WithError(err).Error("Consumer example failed")
	}

	// Example 3: Stream Processing
	fmt.Println("\n=== Example 3: Stream Processing ===")
	if err := runStreamExample(log, metricsCollector); err != nil {
		log.WithError(err).Error("Stream example failed")
	}

	// Example 4: Admin operations
	fmt.Println("\n=== Example 4: Admin Operations ===")
	if err := runAdminExample(log); err != nil {
		log.WithError(err).Error("Admin example failed")
	}

	fmt.Println("\n=== All examples completed ===")
}

func runProducerExample(logger *logrus.Logger, metrics *metrics.MetricsCollector) error {
	brokers := []string{"localhost:9092"}

	// Create advanced producer config
	producerConfig := &kafka.ProducerConfig{
		Brokers:           brokers,
		Logger:            logger,
		Metrics:           metrics,
		BatchSize:         32768, // 32KB batches
		BatchTimeout:      10 * time.Millisecond,
		CompressionType:   "snappy",
		RequiredAcks:      sarama.WaitForAll,
		MaxRetries:        5,
		RetryBackoff:      200 * time.Millisecond,
		EnableIdempotence: true,
		MaxMessageBytes:   1000000, // 1MB
		FlushFrequency:    2 * time.Second,
	}

	producer, err := kafka.NewProducer(producerConfig)
	if err != nil {
		return fmt.Errorf("failed to create producer: %w", err)
	}
	defer producer.Close()

	topic := "analytics-events"

	// Single message
	event := &pb.Event{
		Id:        utils.GenerateEventID(),
		UserId:    utils.GenerateUserID(),
		EventType: "page_view",
		Properties: map[string]string{
			"page":     "/dashboard",
			"duration": "5000",
			"referrer": "google.com",
		},
		Timestamp: utils.GetCurrentTimestamp(),
		Source:    "web-app",
	}

	fmt.Printf("Sending single event: %s\n", event.Id)
	if err := producer.SendMessage(context.Background(), topic, event.UserId, event); err != nil {
		return fmt.Errorf("failed to send single message: %w", err)
	}

	// Async message
	asyncEvent := &pb.Event{
		Id:        utils.GenerateEventID(),
		UserId:    utils.GenerateUserID(),
		EventType: "user_action",
		Properties: map[string]string{
			"action": "click",
			"target": "button-submit",
		},
		Timestamp: utils.GetCurrentTimestamp(),
		Source:    "web-app",
	}

	fmt.Printf("Sending async event: %s\n", asyncEvent.Id)
	if err := producer.SendMessageAsync(context.Background(), topic, asyncEvent.UserId, asyncEvent); err != nil {
		return fmt.Errorf("failed to send async message: %w", err)
	}

	// Batch messages
	var batchMessages []kafka.Message
	for i := 0; i < 5; i++ {
		batchEvent := &pb.Event{
			Id:        utils.GenerateEventID(),
			UserId:    utils.GenerateUserID(),
			EventType: "system_event",
			Properties: map[string]string{
				"component": "auth-service",
				"level":     "info",
				"message":   fmt.Sprintf("User login attempt %d", i),
			},
			Timestamp: utils.GetCurrentTimestamp(),
			Source:    "auth-service",
		}

		batchMessages = append(batchMessages, kafka.Message{
			Key:   batchEvent.UserId,
			Value: batchEvent,
			Headers: map[string]string{
				"event_type": batchEvent.EventType,
				"batch_id":   "batch-001",
			},
		})
	}

	fmt.Printf("Sending batch of %d messages\n", len(batchMessages))
	if err := producer.SendBatchMessages(context.Background(), topic, batchMessages); err != nil {
		return fmt.Errorf("failed to send batch messages: %w", err)
	}

	// Display stats
	stats := producer.GetStats()
	fmt.Printf("Producer Stats:\n")
	fmt.Printf("  Messages Sent: %d\n", stats.MessagesSent)
	fmt.Printf("  Messages Error: %d\n", stats.MessagesError)
	fmt.Printf("  Bytes Sent: %d\n", stats.BytesSent)
	fmt.Printf("  Avg Latency: %.2f ms\n", stats.AvgLatencyMs)
	fmt.Printf("  Throughput: %.2f msg/sec\n", stats.ThroughputPerSec)

	// Flush before closing
	if err := producer.Flush(); err != nil {
		return fmt.Errorf("failed to flush producer: %w", err)
	}

	return nil
}

func runConsumerExample(logger *logrus.Logger, metrics *metrics.MetricsCollector) error {
	brokers := []string{"localhost:9092"}

	// Create DLQ producer first
	dlqProducerConfig := kafka.DefaultProducerConfig(brokers, logger, metrics)
	dlqProducer, err := kafka.NewProducer(dlqProducerConfig)
	if err != nil {
		return fmt.Errorf("failed to create DLQ producer: %w", err)
	}
	defer dlqProducer.Close()

	// Create consumer config with retry and DLQ
	consumerConfig := kafka.DefaultConsumerConfig(brokers, "example-consumer-group", logger, metrics)
	consumerConfig.EnableRetry = true
	consumerConfig.MaxRetries = 2
	consumerConfig.RetryDelay = 500 * time.Millisecond
	consumerConfig.RetryBackoff = 2.0
	consumerConfig.EnableDLQ = true
	consumerConfig.DLQTopic = "analytics-events-dlq"
	consumerConfig.DLQProducer = dlqProducer

	consumer, err := kafka.NewConsumer(consumerConfig)
	if err != nil {
		return fmt.Errorf("failed to create consumer: %w", err)
	}
	defer consumer.Close()

	// Message handler with simulated failures
	messageHandler := func(ctx context.Context, message *sarama.ConsumerMessage) error {
		var event pb.Event
		if err := json.Unmarshal(message.Value, &event); err != nil {
			return fmt.Errorf("failed to unmarshal event: %w", err)
		}

		logger.WithFields(logrus.Fields{
			"event_id":   event.Id,
			"event_type": event.EventType,
			"user_id":    event.UserId,
		}).Info("Processing event")

		// Simulate processing failure for error events
		if event.EventType == "error_event" {
			return fmt.Errorf("simulated processing failure for error event")
		}

		// Simulate processing time
		time.Sleep(100 * time.Millisecond)

		logger.WithField("event_id", event.Id).Debug("Event processed successfully")
		return nil
	}

	// Error handler for DLQ messages
	errorHandler := func(ctx context.Context, message *sarama.ConsumerMessage, err error) error {
		logger.WithError(err).WithFields(logrus.Fields{
			"topic":     message.Topic,
			"partition": message.Partition,
			"offset":    message.Offset,
		}).Warn("Event processing failed, handled by error handler")
		return nil
	}

	// Start consuming with context timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	go func() {
		if err := consumer.SubscribeWithErrorHandler(ctx, []string{"analytics-events"}, messageHandler, errorHandler); err != nil {
			logger.WithError(err).Error("Consumer subscription failed")
		}
	}()

	// Wait for some processing
	time.Sleep(10 * time.Second)

	// Display stats
	stats := consumer.GetStats()
	fmt.Printf("Consumer Stats:\n")
	fmt.Printf("  Messages Processed: %d\n", stats.MessagesProcessed)
	fmt.Printf("  Messages Error: %d\n", stats.MessagesError)
	fmt.Printf("  Messages Retried: %d\n", stats.MessagesRetried)
	fmt.Printf("  Messages to DLQ: %d\n", stats.MessagesDLQ)
	fmt.Printf("  Processing Rate: %.2f msg/sec\n", stats.ProcessingRate)
	fmt.Printf("  Avg Processing Time: %.2f ms\n", stats.AvgProcessingTimeMs)

	return nil
}

func runStreamExample(logger *logrus.Logger, metrics *metrics.MetricsCollector) error {
	brokers := []string{"localhost:9092"}

	// Create stream processor
	streamConfig := &kafka.StreamConfig{
		Brokers: brokers,
		GroupID: "example-stream-group",
		Logger:  logger,
		Metrics: metrics,
	}

	streamProcessor, err := kafka.NewStreamProcessor(streamConfig)
	if err != nil {
		return fmt.Errorf("failed to create stream processor: %w", err)
	}
	defer streamProcessor.Close()

	// Add validation processor
	validationRules := map[string]func(string) bool{
		"page": func(page string) bool {
			return len(page) > 0 && len(page) < 1000
		},
	}
	streamProcessor.AddProcessor("validation", kafka.ValidationProcessor(validationRules))

	// Add page view enrichment
	streamProcessor.AddProcessor("page_enrichment", kafka.CreatePageViewProcessor())

	// Add user action enrichment
	streamProcessor.AddProcessor("action_enrichment", kafka.CreateUserActionProcessor())

	// Add custom transformation
	streamProcessor.AddProcessor("custom_transform", kafka.TransformProcessor(
		func(event *pb.Event) (*pb.Event, error) {
			// Add processing timestamp
			if event.Properties == nil {
				event.Properties = make(map[string]string)
			}
			event.Properties["processed_at"] = time.Now().Format(time.RFC3339)
			event.Properties["processor"] = "stream-example"

			return event, nil
		},
	))

	// Add deduplication
	dedupProcessor := kafka.NewDeduplicationProcessor(5*time.Minute, logger)
	streamProcessor.AddProcessor("deduplication", dedupProcessor.Process)

	// Start stream processing with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	go func() {
		if err := streamProcessor.ProcessStream(ctx, "analytics-events", "analytics-events-processed"); err != nil {
			logger.WithError(err).Error("Stream processing failed")
		}
	}()

	// Wait for some processing
	time.Sleep(10 * time.Second)

	// Display stream stats
	stats := streamProcessor.GetStats()
	fmt.Printf("Stream Processor Stats:\n")
	fmt.Printf("  Events Processed: %d\n", stats.EventsProcessed)
	fmt.Printf("  Events Filtered: %d\n", stats.EventsFiltered)
	fmt.Printf("  Events Error: %d\n", stats.EventsError)
	fmt.Printf("  Avg Latency: %.2f ms\n", stats.AvgLatencyMs)
	fmt.Printf("  Throughput: %.2f events/sec\n", stats.ThroughputPerSec)

	return nil
}

func runAdminExample(logger *logrus.Logger) error {
	brokers := []string{"localhost:9092"}

	// Create admin client
	adminConfig := kafka.DefaultAdminConfig(brokers, logger)
	admin, err := kafka.NewAdminClient(adminConfig)
	if err != nil {
		return fmt.Errorf("failed to create admin client: %w", err)
	}
	defer admin.Close()

	ctx := context.Background()

	// List existing topics
	topics, err := admin.ListTopics(ctx)
	if err != nil {
		return fmt.Errorf("failed to list topics: %w", err)
	}

	fmt.Printf("Existing topics (%d):\n", len(topics))
	for _, topic := range topics {
		fmt.Printf("  - %s\n", topic)
	}

	// Create standard topics
	fmt.Println("\nCreating standard topics...")
	if err := kafka.CreateStandardTopics(admin, logger); err != nil {
		return fmt.Errorf("failed to create topics: %w", err)
	}

	// Get topic metadata
	for _, topicName := range []string{"analytics-events", "analytics-events-dlq"} {
		metadata, err := admin.GetTopicMetadata(ctx, topicName)
		if err != nil {
			logger.WithError(err).WithField("topic", topicName).Warn("Failed to get topic metadata")
			continue
		}

		fmt.Printf("\nTopic: %s\n", metadata.Name)
		fmt.Printf("  Partitions: %d\n", len(metadata.Partitions))
		for _, partition := range metadata.Partitions {
			fmt.Printf("    Partition %d: Leader=%d, Replicas=%v\n",
				partition.ID, partition.Leader, partition.Replicas)
		}
	}

	// List consumer groups
	groups, err := admin.GetConsumerGroups(ctx)
	if err != nil {
		return fmt.Errorf("failed to get consumer groups: %w", err)
	}

	fmt.Printf("\nConsumer groups (%d):\n", len(groups))
	for _, group := range groups {
		fmt.Printf("  - %s\n", group)

		// Get lag for each group
		lag, err := admin.GetConsumerGroupLag(ctx, group)
		if err != nil {
			logger.WithError(err).WithField("group", group).Warn("Failed to get consumer lag")
			continue
		}

		for topic, partitions := range lag {
			for partition, lagValue := range partitions {
				if lagValue > 0 {
					fmt.Printf("    Topic %s, Partition %d: Lag %d\n", topic, partition, lagValue)
				}
			}
		}
	}

	return nil
}

// Utility function to generate sample events
func generateSampleEvents(count int) []*pb.Event {
	var events []*pb.Event

	eventTypes := []string{"page_view", "user_action", "system_event"}
	pages := []string{"/dashboard", "/products", "/admin", "/api/users"}
	actions := []string{"click", "hover", "submit", "purchase", "signup"}

	for i := 0; i < count; i++ {
		eventType := eventTypes[i%len(eventTypes)]

		event := &pb.Event{
			Id:        utils.GenerateEventID(),
			UserId:    utils.GenerateUserID(),
			EventType: eventType,
			Timestamp: utils.GetCurrentTimestamp(),
			Source:    "sample-generator",
		}

		// Add type-specific properties
		switch eventType {
		case "page_view":
			event.Properties = map[string]string{
				"page":     pages[i%len(pages)],
				"duration": fmt.Sprintf("%d", 1000+i*100),
				"referrer": "direct",
			}
		case "user_action":
			event.Properties = map[string]string{
				"action": actions[i%len(actions)],
				"target": fmt.Sprintf("element-%d", i),
			}
		case "system_event":
			level := "info"
			if i%10 == 0 {
				level = "error" // 10% error rate
			}
			event.Properties = map[string]string{
				"component": "web-server",
				"level":     level,
				"message":   fmt.Sprintf("System event %d", i),
			}
		}

		events = append(events, event)
	}

	return events
}

// Performance test function
func runPerformanceTest(logger *logrus.Logger, metrics *metrics.MetricsCollector) error {
	fmt.Println("\n=== Performance Test ===")

	brokers := []string{"localhost:9092"}

	// High-performance producer config
	producerConfig := &kafka.ProducerConfig{
		Brokers:           brokers,
		Logger:            logger,
		Metrics:           metrics,
		BatchSize:         65536, // 64KB batches
		BatchTimeout:      1 * time.Millisecond,
		CompressionType:   "lz4",               // Fast compression
		RequiredAcks:      sarama.WaitForLocal, // Faster acks
		MaxRetries:        3,
		RetryBackoff:      50 * time.Millisecond,
		EnableIdempotence: false, // Disable for performance
		MaxMessageBytes:   1000000,
		FlushFrequency:    500 * time.Millisecond,
	}

	producer, err := kafka.NewProducer(producerConfig)
	if err != nil {
		return fmt.Errorf("failed to create performance producer: %w", err)
	}
	defer producer.Close()

	// Generate and send 1000 events
	events := generateSampleEvents(1000)
	start := time.Now()

	var batchMessages []kafka.Message
	for _, event := range events {
		batchMessages = append(batchMessages, kafka.Message{
			Key:   event.UserId,
			Value: event,
		})
	}

	if err := producer.SendBatchMessages(context.Background(), "analytics-events", batchMessages); err != nil {
		return fmt.Errorf("failed to send performance test batch: %w", err)
	}

	elapsed := time.Since(start)
	stats := producer.GetStats()

	fmt.Printf("Performance Test Results:\n")
	fmt.Printf("  Events Sent: %d\n", len(events))
	fmt.Printf("  Total Time: %v\n", elapsed)
	fmt.Printf("  Events/Second: %.2f\n", float64(len(events))/elapsed.Seconds())
	fmt.Printf("  Avg Latency: %.2f ms\n", stats.AvgLatencyMs)
	fmt.Printf("  Total Bytes: %d\n", stats.BytesSent)

	return nil
}
