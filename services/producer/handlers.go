package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	pb "real-time-analytics-platform/api/proto"
	"real-time-analytics-platform/internal/kafka"
)

func (s *ProducerService) SendEvent(ctx context.Context, req *pb.Event) (*pb.EventResponse, error) {
	start := time.Now()

	// Validate request
	if req.Id == "" || req.EventType == "" {
		s.metrics.IncrementErrors("producer", "validation")
		return &pb.EventResponse{
			Success: false,
			Message: "Event ID and type are required",
		}, nil
	}

	// Set timestamp if not provided
	if req.Timestamp == 0 {
		req.Timestamp = time.Now().Unix()
	}

	// Send to Kafka
	err := s.kafkaProducer.SendMessage(ctx, s.config.Kafka.TopicEvents, req.UserId, req)
	if err != nil {
		s.metrics.IncrementErrors("producer", "kafka")
		s.logger.WithError(err).Error("Failed to send event to Kafka")
		return &pb.EventResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to send event: %v", err),
		}, nil
	}

	// Record metrics
	duration := time.Since(start).Seconds()
	s.metrics.ObserveProcessingTime("producer", req.EventType, duration)
	s.metrics.IncrementEventsProcessed("producer", req.EventType, "success")
	s.metrics.IncrementEventsTotal()

	s.logger.WithFields(map[string]interface{}{
		"event_id":   req.Id,
		"event_type": req.EventType,
		"user_id":    req.UserId,
		"duration":   duration,
	}).Info("Event sent successfully")

	return &pb.EventResponse{
		Success: true,
		Message: "Event sent successfully",
		EventId: req.Id,
	}, nil
}

func (s *ProducerService) SendBatchEvents(ctx context.Context, req *pb.BatchEventsRequest) (*pb.BatchEventsResponse, error) {
	start := time.Now()

	if len(req.Events) == 0 {
		s.metrics.IncrementErrors("producer", "validation")
		return &pb.BatchEventsResponse{
			Success: false,
			Message: "No events provided",
		}, nil
	}

	// Convert to batch messages
	var messages []kafka.Message
	for _, event := range req.Events {
		if event.Timestamp == 0 {
			event.Timestamp = time.Now().Unix()
		}

		messages = append(messages, kafka.Message{
			Key:   event.UserId,
			Value: event,
			Headers: map[string]string{
				"event_type": event.EventType,
				"source":     event.Source,
			},
		})
	}

	// Send batch to Kafka
	err := s.kafkaProducer.SendBatchMessages(ctx, s.config.Kafka.TopicEvents, messages)
	processedCount := int32(len(req.Events))
	var failedEventIds []string

	if err != nil {
		s.metrics.IncrementErrors("producer", "kafka_batch")
		s.logger.WithError(err).Error("Failed to send batch events")

		// For simplicity, mark all as failed if batch fails
		for _, event := range req.Events {
			failedEventIds = append(failedEventIds, event.Id)
		}
		processedCount = 0
	}

	duration := time.Since(start).Seconds()
	s.metrics.ObserveProcessingTime("producer", "batch", duration)

	s.logger.WithFields(map[string]interface{}{
		"total_events":     len(req.Events),
		"processed_events": processedCount,
		"failed_events":    len(failedEventIds),
		"duration":         duration,
	}).Info("Batch events processed")

	return &pb.BatchEventsResponse{
		Success:        len(failedEventIds) == 0,
		Message:        fmt.Sprintf("Processed %d/%d events", processedCount, len(req.Events)),
		ProcessedCount: processedCount,
		FailedEventIds: failedEventIds,
	}, nil
}

func (s *ProducerService) GetProducerHealth(ctx context.Context, req *pb.HealthRequest) (*pb.HealthResponse, error) {
	// Check Kafka connectivity
	healthChecker := kafka.NewHealthChecker(s.config.Kafka.Brokers, s.logger)
	kafkaHealth, err := healthChecker.CheckHealth(ctx)
	if err != nil || !kafkaHealth.IsHealthy {
		status := "Producer service unhealthy"
		if kafkaHealth != nil {
			status = fmt.Sprintf("Kafka connectivity issues: %s", kafkaHealth.ErrorMessage)
		}
		return &pb.HealthResponse{
			Healthy:   false,
			Status:    status,
			Timestamp: time.Now().Unix(),
		}, nil
	}

	// Check producer stats
	stats := s.kafkaProducer.GetStats()
	healthy := true
	status := "Producer service is healthy"

	// Check if producer has high error rate
	if stats.MessagesSent > 0 {
		errorRate := float64(stats.MessagesError) / float64(stats.MessagesSent)
		if errorRate > 0.1 { // More than 10% error rate
			healthy = false
			status = fmt.Sprintf("High error rate detected: %.2f%%", errorRate*100)
		}
	}

	return &pb.HealthResponse{
		Healthy:   healthy,
		Status:    status,
		Timestamp: time.Now().Unix(),
	}, nil
}

func (s *ProducerService) handleHTTPEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var event pb.Event
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		s.metrics.IncrementErrors("producer", "json_decode")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error":"Invalid JSON: %v"}`, err)
		return
	}

	// Set timestamp if not provided
	if event.Timestamp == 0 {
		event.Timestamp = time.Now().Unix()
	}

	resp, err := s.SendEvent(r.Context(), &event)
	if err != nil {
		s.metrics.IncrementErrors("producer", "grpc")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error":"Internal server error: %v"}`, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if resp.Success {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusBadRequest)
	}

	json.NewEncoder(w).Encode(resp)
}

func (s *ProducerService) handleHTTPBatchEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var batchRequest pb.BatchEventsRequest
	if err := json.NewDecoder(r.Body).Decode(&batchRequest); err != nil {
		s.metrics.IncrementErrors("producer", "json_decode")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error":"Invalid JSON: %v"}`, err)
		return
	}

	resp, err := s.SendBatchEvents(r.Context(), &batchRequest)
	if err != nil {
		s.metrics.IncrementErrors("producer", "grpc")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error":"Internal server error: %v"}`, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if resp.Success {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusPartialContent)
	}

	json.NewEncoder(w).Encode(resp)
}

func (s *ProducerService) handleHTTPAsyncEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var event pb.Event
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		s.metrics.IncrementErrors("producer", "json_decode")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error":"Invalid JSON: %v"}`, err)
		return
	}

	// Set timestamp if not provided
	if event.Timestamp == 0 {
		event.Timestamp = time.Now().Unix()
	}

	// Send async
	err := s.kafkaProducer.SendMessageAsync(r.Context(), s.config.Kafka.TopicEvents, event.UserId, &event)
	if err != nil {
		s.metrics.IncrementErrors("producer", "kafka_async")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error":"Failed to queue async event: %v"}`, err)
		return
	}

	s.metrics.IncrementEventsTotal()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "Event queued for async processing",
		"event_id": event.Id,
	})
}

func (s *ProducerService) handleHTTPStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	stats := s.kafkaProducer.GetStats()
	uptime := time.Since(s.startTime).Seconds()

	response := map[string]interface{}{
		"service":            "producer",
		"uptime_seconds":     uptime,
		"messages_sent":      stats.MessagesSent,
		"messages_error":     stats.MessagesError,
		"bytes_sent":         stats.BytesSent,
		"avg_latency_ms":     stats.AvgLatencyMs,
		"throughput_per_sec": stats.ThroughputPerSec,
		"timestamp":          time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
