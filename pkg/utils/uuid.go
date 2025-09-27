package utils

import (
	"crypto/rand"
	"fmt"
	"time"
)

// GenerateEventID generates a unique event ID
func GenerateEventID() string {
	timestamp := time.Now().UnixNano()
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)

	return fmt.Sprintf("evt_%d_%x", timestamp, randomBytes)
}

// GenerateUserID generates a unique user ID
func GenerateUserID() string {
	timestamp := time.Now().UnixNano()
	randomBytes := make([]byte, 8)
	rand.Read(randomBytes)

	return fmt.Sprintf("usr_%d_%x", timestamp, randomBytes)
}

// GetCurrentTimestamp returns current Unix timestamp
func GetCurrentTimestamp() int64 {
	return time.Now().Unix()
}
