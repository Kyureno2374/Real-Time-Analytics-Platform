package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/sirupsen/logrus"

	pb "real-time-analytics-platform/api/proto"
	"real-time-analytics-platform/internal/metrics"
)

// StreamProcessor represents a Kafka Streams-like processor
type StreamProcessor interface {
	ProcessStream(ctx context.Context, inputTopic, outputTopic string) error
	AddProcessor(name string, processor ProcessorFunc) error
	Close() error
	GetStats() StreamStats
}

type ProcessorFunc func(ctx context.Context, event *pb.Event) (*pb.Event, error)

type StreamStats struct {
	EventsProcessed  int64
	EventsFiltered   int64
	EventsError      int64
	AvgLatencyMs     float64
	ThroughputPerSec float64
}

type KafkaStreamProcessor struct {
	consumer   Consumer
	producer   Producer
	logger     *logrus.Logger
	metrics    *metrics.MetricsCollector
	processors map[string]ProcessorFunc

	// Stats tracking
	statsMutex    sync.RWMutex
	stats         StreamStats
	lastStatsTime time.Time
}

type StreamConfig struct {
	Brokers    []string
	GroupID    string
	Logger     *logrus.Logger
	Metrics    *metrics.MetricsCollector
	BufferSize int
	Workers    int
}

func NewStreamProcessor(config *StreamConfig) (StreamProcessor, error) {
	// Create consumer
	consumerConfig := DefaultConsumerConfig(config.Brokers, config.GroupID, config.Logger, config.Metrics)
	consumerConfig.GroupID = config.GroupID + "-stream"

	consumer, err := NewConsumer(consumerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream consumer: %w", err)
	}

	// Create producer
	producerConfig := DefaultProducerConfig(config.Brokers, config.Logger, config.Metrics)
	producer, err := NewProducer(producerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream producer: %w", err)
	}

	return &KafkaStreamProcessor{
		consumer:      consumer,
		producer:      producer,
		logger:        config.Logger,
		metrics:       config.Metrics,
		processors:    make(map[string]ProcessorFunc),
		lastStatsTime: time.Now(),
	}, nil
}

func (s *KafkaStreamProcessor) AddProcessor(name string, processor ProcessorFunc) error {
	if _, exists := s.processors[name]; exists {
		return fmt.Errorf("processor %s already exists", name)
	}

	s.processors[name] = processor
	s.logger.WithField("processor", name).Info("Stream processor added")

	return nil
}

func (s *KafkaStreamProcessor) ProcessStream(ctx context.Context, inputTopic, outputTopic string) error {
	s.logger.WithFields(logrus.Fields{
		"input_topic":  inputTopic,
		"output_topic": outputTopic,
		"processors":   len(s.processors),
	}).Info("Starting stream processing")

	handler := func(ctx context.Context, message *sarama.ConsumerMessage) error {
		return s.processMessage(ctx, message, outputTopic)
	}

	return s.consumer.Subscribe(ctx, []string{inputTopic}, handler)
}

func (s *KafkaStreamProcessor) processMessage(ctx context.Context, message *sarama.ConsumerMessage, outputTopic string) error {
	start := time.Now()

	// Parse event
	var event pb.Event
	if err := json.Unmarshal(message.Value, &event); err != nil {
		s.updateErrorStats()
		s.metrics.IncrementErrors("stream", "unmarshal")
		return fmt.Errorf("failed to unmarshal event: %w", err)
	}

	s.logger.WithFields(logrus.Fields{
		"event_id":   event.Id,
		"event_type": event.EventType,
		"user_id":    event.UserId,
	}).Debug("Processing stream event")

	// Apply all processors in sequence
	processedEvent := &event
	for name, processor := range s.processors {
		result, err := processor(ctx, processedEvent)
		if err != nil {
			s.updateErrorStats()
			s.metrics.IncrementErrors("stream", "processor_"+name)
			s.logger.WithError(err).WithField("processor", name).Error("Processor failed")
			return err
		}

		if result == nil {
			// Event filtered out
			s.updateFilterStats()
			s.metrics.IncrementEventsProcessed("stream", "filtered", "success")
			s.logger.WithFields(logrus.Fields{
				"processor": name,
				"event_id":  event.Id,
			}).Debug("Event filtered by processor")
			return nil
		}

		processedEvent = result
	}

	// Send processed event to output topic
	if outputTopic != "" {
		if err := s.producer.SendMessage(ctx, outputTopic, processedEvent.UserId, processedEvent); err != nil {
			s.updateErrorStats()
			s.metrics.IncrementErrors("stream", "output_send")
			return fmt.Errorf("failed to send processed event: %w", err)
		}
	}

	// Update stats
	s.updateProcessStats(start)
	s.metrics.ObserveProcessingTime("stream", "process", time.Since(start).Seconds())
	s.metrics.IncrementEventsProcessed("stream", "processed", "success")

	return nil
}

func (s *KafkaStreamProcessor) Close() error {
	var errs []error

	if err := s.consumer.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close consumer: %w", err))
	}

	if err := s.producer.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close producer: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing stream processor: %v", errs)
	}

	s.logger.Info("Stream processor closed successfully")
	return nil
}

func (s *KafkaStreamProcessor) GetStats() StreamStats {
	s.statsMutex.RLock()
	defer s.statsMutex.RUnlock()

	// Calculate throughput
	elapsed := time.Since(s.lastStatsTime).Seconds()
	if elapsed > 0 {
		s.stats.ThroughputPerSec = float64(s.stats.EventsProcessed) / elapsed
	}

	return s.stats
}

func (s *KafkaStreamProcessor) updateProcessStats(startTime time.Time) {
	s.statsMutex.Lock()
	defer s.statsMutex.Unlock()

	s.stats.EventsProcessed++

	// Update average processing time
	processingTime := time.Since(startTime)
	alpha := 0.1
	newLatencyMs := float64(processingTime.Milliseconds())

	if s.stats.AvgLatencyMs == 0 {
		s.stats.AvgLatencyMs = newLatencyMs
	} else {
		s.stats.AvgLatencyMs = alpha*newLatencyMs + (1-alpha)*s.stats.AvgLatencyMs
	}
}

func (s *KafkaStreamProcessor) updateErrorStats() {
	s.statsMutex.Lock()
	defer s.statsMutex.Unlock()

	s.stats.EventsError++
}

func (s *KafkaStreamProcessor) updateFilterStats() {
	s.statsMutex.Lock()
	defer s.statsMutex.Unlock()

	s.stats.EventsFiltered++
}
