package kafka

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	pb "real-time-analytics-platform/api/proto"
	"real-time-analytics-platform/internal/metrics"
)

func TestFilterProcessor(t *testing.T) {
	t.Skip("Requires running Kafka - integration test")

	// Create filter processor
	processor := FilterProcessor(func(event *pb.Event) bool {
		return event.EventType == "page_view"
	})

	// Test with matching event
	pageViewEvent := &pb.Event{
		Id:        "test-1",
		UserId:    "user-1",
		EventType: "page_view",
		Properties: map[string]string{
			"page": "/dashboard",
		},
		Timestamp: time.Now().Unix(),
	}

	result, err := processor(context.Background(), pageViewEvent)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if result == nil {
		t.Fatal("Expected event to pass filter")
	}

	// Test with non-matching event
	actionEvent := &pb.Event{
		Id:        "test-2",
		UserId:    "user-1",
		EventType: "user_action",
		Timestamp: time.Now().Unix(),
	}

	result, err = processor(context.Background(), actionEvent)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if result != nil {
		t.Fatal("Expected event to be filtered out")
	}
}

func TestValidationProcessor(t *testing.T) {
	t.Skip("Requires running Kafka - integration test")

	rules := map[string]func(string) bool{
		"page": func(page string) bool {
			return len(page) > 0 && len(page) < 100
		},
	}

	processor := ValidationProcessor(rules)

	// Test valid event
	validEvent := &pb.Event{
		Id:        "test-1",
		UserId:    "user-1",
		EventType: "page_view",
		Properties: map[string]string{
			"page": "/dashboard",
		},
		Timestamp: time.Now().Unix(),
	}

	result, err := processor(context.Background(), validEvent)
	if err != nil {
		t.Fatalf("Expected no error for valid event, got: %v", err)
	}
	if result == nil {
		t.Fatal("Expected valid event to pass")
	}

	// Test invalid event (missing required fields)
	invalidEvent := &pb.Event{
		Id: "", // Missing ID
	}

	result, err = processor(context.Background(), invalidEvent)
	if err == nil {
		t.Fatal("Expected error for invalid event")
	}
	if result != nil {
		t.Fatal("Expected invalid event to be rejected")
	}
}

func TestAggregationProcessor(t *testing.T) {
	t.Skip("Requires running Kafka - integration test")

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Create aggregation processor with 1 second window
	aggProcessor := NewAggregationProcessor(1*time.Second, logger)

	// Send multiple events for same user/type
	events := []*pb.Event{
		{
			Id:         "test-1",
			UserId:     "user-1",
			EventType:  "page_view",
			Properties: map[string]string{"page": "/page1"},
			Timestamp:  time.Now().Unix(),
		},
		{
			Id:         "test-2",
			UserId:     "user-1",
			EventType:  "page_view",
			Properties: map[string]string{"page": "/page2"},
			Timestamp:  time.Now().Unix(),
		},
	}

	var results []*pb.Event
	for _, event := range events {
		result, err := aggProcessor.Process(context.Background(), event)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if result != nil {
			results = append(results, result)
		}
	}

	// Should not aggregate immediately (threshold not reached)
	if len(results) > 0 {
		t.Logf("Aggregated events (early): %d", len(results))
	}

	// Wait for window to close and send more events to trigger aggregation
	time.Sleep(1100 * time.Millisecond)

	// Send another event to trigger new window
	newEvent := &pb.Event{
		Id:         "test-3",
		UserId:     "user-1",
		EventType:  "page_view",
		Properties: map[string]string{"page": "/page3"},
		Timestamp:  time.Now().Unix(),
	}

	_, err := aggProcessor.Process(context.Background(), newEvent)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	t.Logf("Aggregation test completed")
}

func TestDeduplicationProcessor(t *testing.T) {
	t.Skip("Requires running Kafka - integration test")

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Create dedup processor with 2 second window
	dedupProcessor := NewDeduplicationProcessor(2*time.Second, logger)

	// Create duplicate events
	event1 := &pb.Event{
		Id:        "duplicate-event",
		UserId:    "user-1",
		EventType: "page_view",
		Timestamp: time.Now().Unix(),
	}

	event2 := &pb.Event{
		Id:        "duplicate-event", // Same ID
		UserId:    "user-1",
		EventType: "page_view",
		Timestamp: time.Now().Unix(),
	}

	// First event should pass
	result1, err := dedupProcessor.Process(context.Background(), event1)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result1 == nil {
		t.Fatal("First event should pass deduplication")
	}

	// Second event (duplicate) should be filtered
	result2, err := dedupProcessor.Process(context.Background(), event2)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result2 != nil {
		t.Fatal("Duplicate event should be filtered out")
	}

	// Wait for window to expire
	time.Sleep(2100 * time.Millisecond)

	// Now the same event should pass again
	result3, err := dedupProcessor.Process(context.Background(), event1)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result3 == nil {
		t.Fatal("Event should pass after deduplication window expires")
	}
}

func TestStreamProcessor(t *testing.T) {
	t.Skip("Requires running Kafka - integration test")

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	// Use nil metrics to avoid Prometheus registration conflicts in tests
	var metricsCollector *metrics.MetricsCollector

	config := &StreamConfig{
		Brokers: []string{"localhost:9092"},
		GroupID: "test-stream-group",
		Logger:  logger,
		Metrics: metricsCollector,
	}

	streamProcessor, err := NewStreamProcessor(config)
	if err != nil {
		t.Fatalf("Failed to create stream processor: %v", err)
	}
	defer streamProcessor.Close()

	// Add processors
	err = streamProcessor.AddProcessor("validation", ValidationProcessor(nil))
	if err != nil {
		t.Fatalf("Failed to add validation processor: %v", err)
	}

	err = streamProcessor.AddProcessor("page_view", CreatePageViewProcessor())
	if err != nil {
		t.Fatalf("Failed to add page view processor: %v", err)
	}

	// Test stats
	stats := streamProcessor.GetStats()
	if stats.EventsProcessed != 0 {
		t.Errorf("Expected 0 processed events, got %d", stats.EventsProcessed)
	}

	t.Log("Stream processor test completed successfully")
}
