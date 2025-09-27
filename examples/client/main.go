package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	pb "real-time-analytics-platform/api/proto"
)

func main() {
	baseURL := "http://localhost:8081"

	log.Println("Analytics Platform Client Example")
	log.Println("=================================")

	// Example 1: Send single event
	log.Println("\n1. Sending single event...")
	singleEvent := &pb.Event{
		Id:        fmt.Sprintf("event_%d", time.Now().UnixNano()),
		UserId:    "user123",
		EventType: "page_view",
		Properties: map[string]string{
			"page":     "/dashboard",
			"duration": "45",
			"browser":  "chrome",
		},
		Timestamp: time.Now().Unix(),
		Source:    "web-app",
	}

	if err := sendEvent(baseURL+"/events", singleEvent); err != nil {
		log.Printf("Error sending single event: %v", err)
	} else {
		log.Println("✓ Single event sent successfully")
	}

	// Example 2: Send batch of events
	log.Println("\n2. Sending batch of events...")
	events := generateSampleEvents(5)
	if err := sendBatchEvents(baseURL+"/batch", events); err != nil {
		log.Printf("Error sending batch events: %v", err)
	} else {
		log.Println("✓ Batch events sent successfully")
	}

	// Example 3: Check service health
	log.Println("\n3. Checking service health...")
	if err := checkHealth(baseURL + "/health"); err != nil {
		log.Printf("Error checking health: %v", err)
	} else {
		log.Println("✓ Service is healthy")
	}

	// Example 4: Get analytics summary
	log.Println("\n4. Getting analytics summary...")
	if err := getAnalyticsSummary("http://localhost:8083/analytics/summary"); err != nil {
		log.Printf("Error getting analytics: %v", err)
	} else {
		log.Println("✓ Analytics summary retrieved")
	}

	log.Println("\nExample completed!")
}

func sendEvent(url string, event *pb.Event) error {
	jsonData, err := json.Marshal(event)
	if err != nil {
		return err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result pb.EventResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if !result.Success {
		return fmt.Errorf("event sending failed: %s", result.Message)
	}

	return nil
}

func sendBatchEvents(url string, events []*pb.Event) error {
	batchRequest := &pb.BatchEventsRequest{
		Events: events,
	}

	jsonData, err := json.Marshal(batchRequest)
	if err != nil {
		return err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result pb.BatchEventsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	log.Printf("Batch result: %d/%d events processed", result.ProcessedCount, len(events))

	return nil
}

func checkHealth(url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed with status: %d", resp.StatusCode)
	}

	return nil
}

func getAnalyticsSummary(url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var summary pb.AnalyticsSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		return err
	}

	log.Printf("Analytics Summary:")
	log.Printf("  Total Events: %d", summary.TotalEvents)
	log.Printf("  Total Users: %d", summary.TotalUsers)
	log.Printf("  Events by Type: %v", summary.EventsByType)

	return nil
}

func generateSampleEvents(count int) []*pb.Event {
	eventTypes := []string{"page_view", "user_action", "system_event"}
	userIds := []string{"user1", "user2", "user3", "user4", "user5"}

	var events []*pb.Event

	for i := 0; i < count; i++ {
		event := &pb.Event{
			Id:        fmt.Sprintf("batch_event_%d_%d", time.Now().UnixNano(), i),
			UserId:    userIds[rand.Intn(len(userIds))],
			EventType: eventTypes[rand.Intn(len(eventTypes))],
			Properties: map[string]string{
				"batch_id": "batch_001",
				"index":    fmt.Sprintf("%d", i),
			},
			Timestamp: time.Now().Unix(),
			Source:    "client-example",
		}

		events = append(events, event)
	}

	return events
}
