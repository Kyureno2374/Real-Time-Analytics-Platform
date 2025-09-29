package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/sirupsen/logrus"

	"real-time-analytics-platform/pkg/client"
)

func main() {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	logger.SetFormatter(&logrus.JSONFormatter{})

	fmt.Println("=== gRPC Client Example ===\n")

	// Example 1: Basic Client Usage
	fmt.Println("Example 1: Creating a basic gRPC client")
	basicClientExample(logger)

	// Example 2: Client with Custom Configuration
	fmt.Println("\nExample 2: Client with custom configuration")
	customConfigExample(logger)

	// Example 3: Connection Pool Usage
	fmt.Println("\nExample 3: Using connection pool")
	connectionPoolExample(logger)

	// Example 4: Authentication and Metadata
	fmt.Println("\nExample 4: Using authentication and metadata")
	authenticationExample(logger)
}

func basicClientExample(logger *logrus.Logger) {
	// Create client with default configuration
	grpcClient, err := client.NewClient(nil, logger)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer grpcClient.Close()

	fmt.Println("✓ Client created successfully")
	fmt.Printf("  Server: %s\n", grpcClient.GetConnection().Target())
}

func customConfigExample(logger *logrus.Logger) {
	// Create custom configuration
	config := &client.ClientConfig{
		ServerAddress:     "localhost:50051",
		APIKey:            "your-api-key-here",
		MaxRetries:        5,
		RetryDelay:        2 * time.Second,
		ConnectionTimeout: 15 * time.Second,
		RequestTimeout:    60 * time.Second,
		EnableKeepalive:   true,
		KeepaliveTime:     45 * time.Second,
		KeepaliveTimeout:  15 * time.Second,
	}

	// Create client with custom configuration
	grpcClient, err := client.NewClient(config, logger)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer grpcClient.Close()

	fmt.Println("✓ Client created with custom configuration")
	fmt.Printf("  Server: %s\n", config.ServerAddress)
	fmt.Printf("  Max Retries: %d\n", config.MaxRetries)
	fmt.Printf("  Request Timeout: %v\n", config.RequestTimeout)
	fmt.Printf("  Keepalive Enabled: %v\n", config.EnableKeepalive)
}

func connectionPoolExample(logger *logrus.Logger) {
	// Create configuration
	config := &client.ClientConfig{
		ServerAddress:     "localhost:50051",
		APIKey:            "your-api-key-here",
		MaxRetries:        3,
		RetryDelay:        time.Second,
		ConnectionTimeout: 10 * time.Second,
		RequestTimeout:    30 * time.Second,
		EnableKeepalive:   true,
	}

	// Create connection pool with 5 connections
	pool, err := client.NewConnectionPool(5, config, logger)
	if err != nil {
		log.Fatalf("Failed to create connection pool: %v", err)
	}
	defer pool.Close()

	fmt.Println("✓ Connection pool created with 5 connections")

	// Simulate using different connections
	for i := 0; i < 10; i++ {
		conn := pool.Get()
		fmt.Printf("  Request %d using connection: %s\n", i+1, conn.GetConnection().Target())
	}
}

func authenticationExample(logger *logrus.Logger) {
	// Create client with API key
	config := &client.ClientConfig{
		ServerAddress:     "localhost:50051",
		APIKey:            "my-secret-api-key",
		ConnectionTimeout: 10 * time.Second,
		RequestTimeout:    30 * time.Second,
	}

	grpcClient, err := client.NewClient(config, logger)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer grpcClient.Close()

	// Create context with API key
	ctx := context.Background()
	ctx = grpcClient.WithAPIKey(ctx)

	// Add custom request ID
	requestID := fmt.Sprintf("req-%d", time.Now().UnixNano())
	ctx = grpcClient.WithRequestID(ctx, requestID)

	fmt.Println("✓ Context prepared with authentication")
	fmt.Printf("  API Key: %s\n", config.APIKey)
	fmt.Printf("  Request ID: %s\n", requestID)

	// In real usage, you would make gRPC calls with this context:
	// Example (commented out because proto may not be generated yet):
	//
	// client := pb.NewEventIngestionServiceClient(grpcClient.GetConnection())
	//
	// event := &pb.Event{
	// 	Id:        uuid.New().String(),
	// 	UserId:    "user-123",
	// 	EventType: pb.EventType_EVENT_TYPE_USER,
	// 	Timestamp: timestamppb.Now(),
	// 	Source:    "web-app",
	// }
	//
	// req := &pb.IngestEventRequest{
	// 	Event: event,
	// }
	//
	// resp, err := client.IngestEvent(ctx, req)
	// if err != nil {
	// 	log.Printf("Failed to ingest event: %v", err)
	// 	return
	// }
	//
	// log.Printf("Event ingested: %s", resp.EventId)
}

/*
Example output when running this program:

=== gRPC Client Example ===

Example 1: Creating a basic gRPC client
✓ Client created successfully
  Server: localhost:50051

Example 2: Client with custom configuration
✓ Client created with custom configuration
  Server: localhost:50051
  Max Retries: 5
  Request Timeout: 1m0s
  Keepalive Enabled: true

Example 3: Using connection pool
✓ Connection pool created with 5 connections
  Request 1 using connection: localhost:50051
  Request 2 using connection: localhost:50051
  Request 3 using connection: localhost:50051
  Request 4 using connection: localhost:50051
  Request 5 using connection: localhost:50051
  Request 6 using connection: localhost:50051
  Request 7 using connection: localhost:50051
  Request 8 using connection: localhost:50051
  Request 9 using connection: localhost:50051
  Request 10 using connection: localhost:50051

Example 4: Using authentication and metadata
✓ Context prepared with authentication
  API Key: my-secret-api-key
  Request ID: req-1234567890
*/
