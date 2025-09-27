package kafka

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestKafkaProducer_SendMessage(t *testing.T) {
	// Mock test - in real implementation would use testcontainers
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

	// This test would normally require a running Kafka instance
	t.Skip("Skipping integration test - requires Kafka")

	producer, err := NewProducer(&ProducerConfig{
		Brokers: []string{"localhost:9092"},
		Logger:  logger,
	})
	if err != nil {
		t.Fatalf("Failed to create producer: %v", err)
	}
	defer producer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	testEvent := map[string]interface{}{
		"id":         "test-event-1",
		"user_id":    "test-user",
		"event_type": "test",
		"timestamp":  time.Now().Unix(),
	}

	err = producer.SendMessage(ctx, "test-topic", "test-key", testEvent)
	if err != nil {
		t.Errorf("Failed to send message: %v", err)
	}
}

func TestKafkaProducer_SendMessageWithPartition(t *testing.T) {
	// Mock test - in real implementation would use testcontainers
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	t.Skip("Skipping integration test - requires Kafka")

	producer, err := NewProducer(&ProducerConfig{
		Brokers: []string{"localhost:9092"},
		Logger:  logger,
	})
	if err != nil {
		t.Fatalf("Failed to create producer: %v", err)
	}
	defer producer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	testEvent := map[string]interface{}{
		"id": "test-event-2",
	}

	err = producer.SendMessageWithPartition(ctx, "test-topic", 0, "test-key", testEvent)
	if err != nil {
		t.Errorf("Failed to send message with partition: %v", err)
	}
}
