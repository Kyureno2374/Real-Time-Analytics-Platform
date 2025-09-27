package kafka

import (
	"context"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"github.com/sirupsen/logrus"
)

// HealthChecker provides health checking capabilities for Kafka
type HealthChecker struct {
	brokers []string
	logger  *logrus.Logger
	timeout time.Duration
}

// HealthStatus represents the health status of Kafka connection
type HealthStatus struct {
	IsHealthy    bool           `json:"is_healthy"`
	ErrorMessage string         `json:"error_message,omitempty"`
	Brokers      []BrokerStatus `json:"brokers"`
	Topics       []string       `json:"topics,omitempty"`
	ResponseTime time.Duration  `json:"response_time"`
	Timestamp    time.Time      `json:"timestamp"`
}

// BrokerStatus represents the status of individual Kafka broker
type BrokerStatus struct {
	ID       int32  `json:"id"`
	Address  string `json:"address"`
	IsOnline bool   `json:"is_online"`
	Error    string `json:"error,omitempty"`
}

// NewHealthChecker creates a new Kafka health checker
func NewHealthChecker(brokers []string, logger *logrus.Logger) *HealthChecker {
	return &HealthChecker{
		brokers: brokers,
		logger:  logger,
		timeout: 10 * time.Second,
	}
}

// CheckHealth performs comprehensive health check of Kafka cluster
func (h *HealthChecker) CheckHealth(ctx context.Context) (*HealthStatus, error) {
	start := time.Now()

	status := &HealthStatus{
		IsHealthy: false,
		Brokers:   make([]BrokerStatus, 0),
		Timestamp: start,
	}

	// Create Kafka client for health check
	config := sarama.NewConfig()
	config.Version = sarama.V3_0_0_0
	config.Net.DialTimeout = h.timeout
	config.Net.ReadTimeout = h.timeout
	config.Net.WriteTimeout = h.timeout

	client, err := sarama.NewClient(h.brokers, config)
	if err != nil {
		status.ErrorMessage = fmt.Sprintf("Failed to create Kafka client: %v", err)
		status.ResponseTime = time.Since(start)
		return status, nil
	}
	defer client.Close()

	// Check broker connectivity
	brokers := client.Brokers()
	onlineBrokers := 0

	for _, broker := range brokers {
		brokerStatus := BrokerStatus{
			ID:      broker.ID(),
			Address: broker.Addr(),
		}

		// Test broker connection
		connected, err := broker.Connected()
		if err != nil || !connected {
			err = broker.Open(config)
			if err != nil {
				brokerStatus.Error = err.Error()
				h.logger.WithError(err).WithField("broker", broker.Addr()).Warn("Broker connectivity check failed")
			} else {
				brokerStatus.IsOnline = true
				onlineBrokers++
				broker.Close()
			}
		} else {
			brokerStatus.IsOnline = true
			onlineBrokers++
		}

		status.Brokers = append(status.Brokers, brokerStatus)
	}

	// Check if we have enough online brokers
	if onlineBrokers == 0 {
		status.ErrorMessage = "No brokers are online"
		status.ResponseTime = time.Since(start)
		return status, nil
	}

	// List topics to verify cluster is responsive
	topics, err := client.Topics()
	if err != nil {
		status.ErrorMessage = fmt.Sprintf("Failed to list topics: %v", err)
		status.ResponseTime = time.Since(start)
		return status, nil
	}

	status.Topics = topics
	status.IsHealthy = true
	status.ResponseTime = time.Since(start)

	h.logger.WithFields(logrus.Fields{
		"online_brokers": onlineBrokers,
		"total_brokers":  len(brokers),
		"topics_count":   len(topics),
		"response_time":  status.ResponseTime,
	}).Debug("Kafka health check completed")

	return status, nil
}

// CheckConnectivity performs a simple connectivity check to Kafka
func (h *HealthChecker) CheckConnectivity(ctx context.Context) error {
	config := sarama.NewConfig()
	config.Version = sarama.V3_0_0_0
	config.Net.DialTimeout = 5 * time.Second

	client, err := sarama.NewClient(h.brokers, config)
	if err != nil {
		return fmt.Errorf("kafka connectivity failed: %w", err)
	}
	defer client.Close()

	// Try to refresh metadata to ensure connection works
	if err := client.RefreshMetadata(); err != nil {
		return fmt.Errorf("kafka metadata refresh failed: %w", err)
	}

	return nil
}

// WaitForKafka waits for Kafka to become available with retries
func (h *HealthChecker) WaitForKafka(ctx context.Context, maxRetries int, retryInterval time.Duration) error {
	h.logger.WithFields(logrus.Fields{
		"brokers":        h.brokers,
		"max_retries":    maxRetries,
		"retry_interval": retryInterval,
	}).Info("Waiting for Kafka to become available")

	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			h.logger.WithField("attempt", attempt).Debug("Retrying Kafka connection")
			time.Sleep(retryInterval)
		}

		if err := h.CheckConnectivity(ctx); err != nil {
			lastErr = err
			h.logger.WithError(err).WithField("attempt", attempt+1).Warn("Kafka connection attempt failed")
			continue
		}

		h.logger.WithField("attempts", attempt+1).Info("Kafka is now available")
		return nil
	}

	return fmt.Errorf("kafka not available after %d attempts: %w", maxRetries, lastErr)
}

// MonitorHealth continuously monitors Kafka health
func (h *HealthChecker) MonitorHealth(ctx context.Context, interval time.Duration, callback func(*HealthStatus)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	h.logger.WithField("interval", interval).Info("Started Kafka health monitoring")

	for {
		select {
		case <-ctx.Done():
			h.logger.Info("Kafka health monitoring stopped")
			return
		case <-ticker.C:
			status, err := h.CheckHealth(ctx)
			if err != nil {
				h.logger.WithError(err).Error("Health check failed")
				continue
			}

			if callback != nil {
				callback(status)
			}

			if !status.IsHealthy {
				h.logger.WithField("error", status.ErrorMessage).Warn("Kafka health check failed")
			}
		}
	}
}
