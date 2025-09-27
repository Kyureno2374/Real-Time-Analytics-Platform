package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/IBM/sarama"
	"github.com/sirupsen/logrus"

	pb "real-time-analytics-platform/api/proto"
	"real-time-analytics-platform/internal/kafka"
)

func (s *ConsumerService) handleMessage(ctx context.Context, message *sarama.ConsumerMessage) error {
	start := time.Now()

	s.logger.WithFields(logrus.Fields{
		"topic":     message.Topic,
		"partition": message.Partition,
		"offset":    message.Offset,
		"key":       string(message.Key),
	}).Debug("Processing message")

	// Parse event
	var event pb.Event
	if err := json.Unmarshal(message.Value, &event); err != nil {
		s.updateFailedStats()
		s.metrics.IncrementErrors("consumer", "unmarshal")
		return fmt.Errorf("failed to unmarshal event: %w", err)
	}

	// Process the event based on type
	if err := s.processEvent(ctx, &event); err != nil {
		s.updateFailedStats()
		s.metrics.IncrementErrors("consumer", "process")
		return fmt.Errorf("failed to process event: %w", err)
	}

	// Update stats
	s.updateProcessedStats(start)
	s.metrics.ObserveProcessingTime("consumer", event.EventType, time.Since(start).Seconds())
	s.metrics.IncrementEventsProcessed("consumer", event.EventType, "success")

	return nil
}

func (s *ConsumerService) processEvent(ctx context.Context, event *pb.Event) error {
	s.logger.WithFields(logrus.Fields{
		"event_id":   event.Id,
		"event_type": event.EventType,
		"user_id":    event.UserId,
	}).Debug("Processing event")

	// Simulate different processing based on event type
	switch event.EventType {
	case "page_view":
		return s.processPageView(ctx, event)
	case "user_action":
		return s.processUserAction(ctx, event)
	case "system_event":
		return s.processSystemEvent(ctx, event)
	case "error_event":
		return s.processErrorEvent(ctx, event)
	default:
		return s.processGenericEvent(ctx, event)
	}
}

func (s *ConsumerService) processPageView(ctx context.Context, event *pb.Event) error {
	// Extract page information
	page := event.Properties["page"]
	duration := event.Properties["duration"]

	s.logger.WithFields(logrus.Fields{
		"page":     page,
		"duration": duration,
		"user_id":  event.UserId,
	}).Debug("Processing page view event")

	// Simulate processing time
	time.Sleep(10 * time.Millisecond)

	// Send metrics to analytics topic
	metric := &pb.Metric{
		Name:  "page_view_count",
		Value: 1.0,
		Labels: map[string]string{
			"page":    page,
			"user_id": event.UserId,
		},
		Timestamp: time.Now().Unix(),
	}

	// Note: In real implementation, this would be sent to metrics topic
	s.logger.WithField("metric", metric.Name).Debug("Page view metric generated")

	return nil
}

func (s *ConsumerService) processUserAction(ctx context.Context, event *pb.Event) error {
	action := event.Properties["action"]
	target := event.Properties["target"]

	s.logger.WithFields(logrus.Fields{
		"action":  action,
		"target":  target,
		"user_id": event.UserId,
	}).Debug("Processing user action event")

	// Simulate processing time based on action complexity
	switch action {
	case "purchase", "signup":
		time.Sleep(50 * time.Millisecond) // Complex actions
	case "click", "hover":
		time.Sleep(5 * time.Millisecond) // Simple actions
	default:
		time.Sleep(20 * time.Millisecond)
	}

	return nil
}

func (s *ConsumerService) processSystemEvent(ctx context.Context, event *pb.Event) error {
	level := event.Properties["level"]
	component := event.Properties["component"]

	s.logger.WithFields(logrus.Fields{
		"level":     level,
		"component": component,
	}).Debug("Processing system event")

	// Alert for error events
	if level == "error" {
		s.logger.WithFields(logrus.Fields{
			"event_id":  event.Id,
			"component": component,
			"message":   event.Properties["message"],
		}).Warn("System error detected")

		// In real implementation, would trigger alerting
	}

	return nil
}

func (s *ConsumerService) processErrorEvent(ctx context.Context, event *pb.Event) error {
	severity := event.Properties["severity"]

	s.logger.WithFields(logrus.Fields{
		"severity": severity,
		"event_id": event.Id,
	}).Warn("Processing error event")

	// Critical errors need immediate attention
	if severity == "critical" {
		s.logger.WithField("event_id", event.Id).Error("Critical error event detected")
		// In real implementation, would send to alerting system
	}

	return nil
}

func (s *ConsumerService) processGenericEvent(ctx context.Context, event *pb.Event) error {
	s.logger.WithFields(logrus.Fields{
		"event_type": event.EventType,
		"event_id":   event.Id,
	}).Debug("Processing generic event")

	// Default processing
	time.Sleep(15 * time.Millisecond)
	return nil
}

func (s *ConsumerService) GetConsumerHealth(ctx context.Context, req *pb.HealthRequest) (*pb.HealthResponse, error) {
	consumerStats := s.kafkaConsumer.GetStats()

	// Check if consumer is healthy
	healthy := true
	status := "Consumer service is healthy"

	// Check Kafka connectivity
	healthChecker := kafka.NewHealthChecker(s.config.Kafka.Brokers, s.logger)
	kafkaHealth, err := healthChecker.CheckHealth(ctx)
	if err != nil || !kafkaHealth.IsHealthy {
		healthy = false
		if kafkaHealth != nil {
			status = fmt.Sprintf("Kafka connectivity issues: %s", kafkaHealth.ErrorMessage)
		} else {
			status = "Failed to check Kafka health"
		}
		return &pb.HealthResponse{
			Healthy:   healthy,
			Status:    status,
			Timestamp: time.Now().Unix(),
		}, nil
	}

	// Check error rate
	if consumerStats.MessagesProcessed > 0 {
		errorRate := float64(consumerStats.MessagesError) / float64(consumerStats.MessagesProcessed)
		if errorRate > 0.1 { // More than 10% error rate
			healthy = false
			status = "High error rate detected"
		}
	}

	// Check if processing recent messages
	if time.Since(consumerStats.LastMessageTime) > 5*time.Minute {
		healthy = false
		status = "No recent message processing"
	}

	return &pb.HealthResponse{
		Healthy:   healthy,
		Status:    status,
		Timestamp: time.Now().Unix(),
	}, nil
}

func (s *ConsumerService) GetConsumerStats(ctx context.Context, req *pb.ConsumerStatsRequest) (*pb.ConsumerStatsResponse, error) {
	consumerStats := s.kafkaConsumer.GetStats()
	streamStats := s.streamProcessor.GetStats()

	return &pb.ConsumerStatsResponse{
		ProcessedEvents:       consumerStats.MessagesProcessed,
		FailedEvents:          consumerStats.MessagesError,
		LastProcessedTime:     consumerStats.LastMessageTime.Unix(),
		ProcessingRate:        consumerStats.ProcessingRate,
		StreamEventsProcessed: streamStats.EventsProcessed,
		StreamEventsFiltered:  streamStats.EventsFiltered,
		AvgProcessingTimeMs:   streamStats.AvgLatencyMs,
	}, nil
}

func (s *ConsumerService) handleHTTPStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	consumerStats := s.kafkaConsumer.GetStats()
	streamStats := s.streamProcessor.GetStats()
	uptime := time.Since(s.startTime).Seconds()

	response := map[string]interface{}{
		"service":        "consumer",
		"uptime_seconds": uptime,
		"consumer": map[string]interface{}{
			"processed_events":  consumerStats.MessagesProcessed,
			"failed_events":     consumerStats.MessagesError,
			"retried_events":    consumerStats.MessagesRetried,
			"dlq_events":        consumerStats.MessagesDLQ,
			"processing_rate":   consumerStats.ProcessingRate,
			"avg_processing_ms": consumerStats.AvgProcessingTimeMs,
			"last_message_time": consumerStats.LastMessageTime.Format(time.RFC3339),
		},
		"stream": map[string]interface{}{
			"events_processed":   streamStats.EventsProcessed,
			"events_filtered":    streamStats.EventsFiltered,
			"events_error":       streamStats.EventsError,
			"avg_latency_ms":     streamStats.AvgLatencyMs,
			"throughput_per_sec": streamStats.ThroughputPerSec,
		},
		"timestamp": time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func (s *ConsumerService) handleHTTPLag(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	lag, err := s.kafkaConsumer.GetLag()
	if err != nil {
		s.metrics.IncrementErrors("consumer", "lag_check")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error":"Failed to get consumer lag: %v"}`, err)
		return
	}

	response := map[string]interface{}{
		"service":   "consumer",
		"lag":       lag,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// Helper methods for stats tracking

func (s *ConsumerService) updateProcessedStats(startTime time.Time) {
	s.stats.mutex.Lock()
	defer s.stats.mutex.Unlock()

	s.stats.ProcessedEvents++
	s.stats.LastProcessed = time.Now()

	// Update average processing time
	processingTime := time.Since(startTime)
	if s.stats.AvgProcessingTime == 0 {
		s.stats.AvgProcessingTime = processingTime
	} else {
		// Exponential moving average
		alpha := 0.1
		s.stats.AvgProcessingTime = time.Duration(float64(processingTime)*alpha + float64(s.stats.AvgProcessingTime)*(1-alpha))
	}
}

func (s *ConsumerService) updateFailedStats() {
	s.stats.mutex.Lock()
	defer s.stats.mutex.Unlock()

	s.stats.FailedEvents++
}

func (s *ConsumerService) updateRetryStats() {
	s.stats.mutex.Lock()
	defer s.stats.mutex.Unlock()

	s.stats.RetryEvents++
}

func (s *ConsumerService) updateDLQStats() {
	s.stats.mutex.Lock()
	defer s.stats.mutex.Unlock()

	s.stats.DLQEvents++
}
