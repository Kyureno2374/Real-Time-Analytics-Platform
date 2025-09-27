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

var (
	eventTypes = []string{"page_view", "user_action", "system_event"}
	userIds    = []string{"user1", "user2", "user3", "user4", "user5"}
	pages      = []string{"/home", "/products", "/about", "/contact", "/login"}
	actions    = []string{"click", "scroll", "hover", "search", "purchase"}
)

func main() {
	producerURL := "http://localhost:8081/events"

	log.Println("Starting sample data generation...")

	// Generate events every second
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Generate 1-5 events per tick
			eventCount := rand.Intn(5) + 1
			for i := 0; i < eventCount; i++ {
				event := generateRandomEvent()
				if err := sendEvent(producerURL, event); err != nil {
					log.Printf("Failed to send event: %v", err)
				} else {
					log.Printf("Sent event: %s (%s)", event.Id, event.EventType)
				}
			}
		}
	}
}

func generateRandomEvent() *pb.Event {
	eventType := eventTypes[rand.Intn(len(eventTypes))]
	userId := userIds[rand.Intn(len(userIds))]

	event := &pb.Event{
		Id:         fmt.Sprintf("event_%d", time.Now().UnixNano()),
		UserId:     userId,
		EventType:  eventType,
		Timestamp:  time.Now().Unix(),
		Source:     "sample-generator",
		Properties: make(map[string]string),
	}

	// Add event-specific properties
	switch eventType {
	case "page_view":
		event.Properties["page"] = pages[rand.Intn(len(pages))]
		event.Properties["duration"] = fmt.Sprintf("%d", rand.Intn(300)+10)
	case "user_action":
		event.Properties["action"] = actions[rand.Intn(len(actions))]
		event.Properties["element"] = fmt.Sprintf("button_%d", rand.Intn(10))
	case "system_event":
		event.Properties["system"] = "backend"
		event.Properties["level"] = []string{"info", "warning", "error"}[rand.Intn(3)]
	}

	return event
}

func sendEvent(url string, event *pb.Event) error {
	jsonData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("received non-200 status code: %d", resp.StatusCode)
	}

	return nil
}
