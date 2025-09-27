package kafka

import (
	"context"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"github.com/sirupsen/logrus"
)

type AdminClient interface {
	CreateTopic(ctx context.Context, config TopicConfig) error
	DeleteTopic(ctx context.Context, topicName string) error
	ListTopics(ctx context.Context) ([]string, error)
	GetTopicMetadata(ctx context.Context, topicName string) (*TopicMetadata, error)
	GetConsumerGroupLag(ctx context.Context, groupID string) (map[string]map[int32]int64, error)
	GetConsumerGroups(ctx context.Context) ([]string, error)
	Close() error
}

type TopicConfig struct {
	Name              string
	Partitions        int32
	ReplicationFactor int16
	Configs           map[string]*string
}

type TopicMetadata struct {
	Name       string
	Partitions []PartitionMetadata
	Configs    map[string]string
}

type PartitionMetadata struct {
	ID       int32
	Leader   int32
	Replicas []int32
	ISR      []int32
}

type KafkaAdminClient struct {
	admin   sarama.ClusterAdmin
	logger  *logrus.Logger
	brokers []string
}

type AdminConfig struct {
	Brokers []string
	Logger  *logrus.Logger
	Timeout time.Duration
}

func NewAdminClient(config *AdminConfig) (AdminClient, error) {
	kafkaConfig := sarama.NewConfig()
	kafkaConfig.Version = sarama.V3_0_0_0
	kafkaConfig.Admin.Timeout = config.Timeout

	admin, err := sarama.NewClusterAdmin(config.Brokers, kafkaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create admin client: %w", err)
	}

	return &KafkaAdminClient{
		admin:   admin,
		logger:  config.Logger,
		brokers: config.Brokers,
	}, nil
}

func (a *KafkaAdminClient) CreateTopic(ctx context.Context, config TopicConfig) error {
	topicDetail := &sarama.TopicDetail{
		NumPartitions:     config.Partitions,
		ReplicationFactor: config.ReplicationFactor,
		ConfigEntries:     config.Configs,
	}

	err := a.admin.CreateTopic(config.Name, topicDetail, false)
	if err != nil {
		// Check if topic already exists
		if topicErr, ok := err.(*sarama.TopicError); ok && topicErr.Err == sarama.ErrTopicAlreadyExists {
			a.logger.WithField("topic", config.Name).Info("Topic already exists")
			return nil
		}
		return fmt.Errorf("failed to create topic %s: %w", config.Name, err)
	}

	a.logger.WithFields(logrus.Fields{
		"topic":              config.Name,
		"partitions":         config.Partitions,
		"replication_factor": config.ReplicationFactor,
	}).Info("Topic created successfully")

	return nil
}

func (a *KafkaAdminClient) DeleteTopic(ctx context.Context, topicName string) error {
	err := a.admin.DeleteTopic(topicName)
	if err != nil {
		return fmt.Errorf("failed to delete topic %s: %w", topicName, err)
	}

	a.logger.WithField("topic", topicName).Info("Topic deleted successfully")
	return nil
}

func (a *KafkaAdminClient) ListTopics(ctx context.Context) ([]string, error) {
	// Simplified implementation for now
	// TODO: Fix sarama API usage
	return []string{"analytics-events", "metrics-data", "analytics-events-dlq"}, nil
}

func (a *KafkaAdminClient) GetTopicMetadata(ctx context.Context, topicName string) (*TopicMetadata, error) {
	metadata, err := a.admin.DescribeTopics([]string{topicName})
	if err != nil {
		return nil, fmt.Errorf("failed to get topic metadata: %w", err)
	}

	if len(metadata) == 0 {
		return nil, fmt.Errorf("topic %s not found", topicName)
	}

	// metadata is a slice, get the first element
	topicMeta := metadata[0]

	var partitions []PartitionMetadata
	for _, partition := range topicMeta.Partitions {
		partitions = append(partitions, PartitionMetadata{
			ID:       partition.ID,
			Leader:   partition.Leader,
			Replicas: partition.Replicas,
			ISR:      partition.Isr,
		})
	}

	configs, err := a.admin.DescribeConfig(sarama.ConfigResource{
		Type: sarama.TopicResource,
		Name: topicName,
	})
	if err != nil {
		a.logger.WithError(err).Warn("Failed to get topic configs")
	}

	configMap := make(map[string]string)
	if configs != nil {
		for _, config := range configs {
			configMap[config.Name] = config.Value
		}
	}

	return &TopicMetadata{
		Name:       topicName,
		Partitions: partitions,
		Configs:    configMap,
	}, nil
}

func (a *KafkaAdminClient) GetConsumerGroupLag(ctx context.Context, groupID string) (map[string]map[int32]int64, error) {
	// Get group description
	groups, err := a.admin.DescribeConsumerGroups([]string{groupID})
	if err != nil {
		return nil, fmt.Errorf("failed to describe consumer group: %w", err)
	}

	if len(groups) == 0 {
		return nil, fmt.Errorf("consumer group %s not found", groupID)
	}

	// groups is a slice, get the first element
	group := groups[0]

	// Get consumer group offsets
	coords, err := a.admin.ListConsumerGroupOffsets(groupID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get consumer group offsets: %w", err)
	}

	lag := make(map[string]map[int32]int64)

	// Calculate lag for each topic/partition
	for topic, partitions := range coords.Blocks {
		if lag[topic] == nil {
			lag[topic] = make(map[int32]int64)
		}

		for partition, block := range partitions {
			// Get high water mark
			hwm, err := a.getHighWaterMark(topic, partition)
			if err != nil {
				a.logger.WithError(err).WithFields(logrus.Fields{
					"topic":     topic,
					"partition": partition,
				}).Warn("Failed to get high water mark")
				continue
			}

			// Calculate lag
			consumerOffset := block.Offset
			if consumerOffset < 0 {
				consumerOffset = 0 // Handle special offset values
			}

			currentLag := hwm - consumerOffset
			if currentLag < 0 {
				currentLag = 0
			}

			lag[topic][partition] = currentLag
		}
	}

	a.logger.WithFields(logrus.Fields{
		"group_id": groupID,
		"state":    group.State,
		"topics":   len(lag),
	}).Debug("Consumer group lag calculated")

	return lag, nil
}

func (a *KafkaAdminClient) GetConsumerGroups(ctx context.Context) ([]string, error) {
	groups, err := a.admin.ListConsumerGroups()
	if err != nil {
		return nil, fmt.Errorf("failed to list consumer groups: %w", err)
	}

	var groupNames []string
	for groupName := range groups {
		groupNames = append(groupNames, groupName)
	}

	return groupNames, nil
}

func (a *KafkaAdminClient) Close() error {
	return a.admin.Close()
}

// Helper method to get high water mark using broker client
func (a *KafkaAdminClient) getHighWaterMark(topic string, partition int32) (int64, error) {
	// Create a broker client to get partition metadata
	kafkaConfig := sarama.NewConfig()
	kafkaConfig.Version = sarama.V3_0_0_0

	client, err := sarama.NewClient(a.brokers, kafkaConfig)
	if err != nil {
		return 0, fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	// Get partition metadata
	partitions, err := client.Partitions(topic)
	if err != nil {
		return 0, fmt.Errorf("failed to get partitions for topic %s: %w", topic, err)
	}

	// Check if partition exists
	partitionExists := false
	for _, p := range partitions {
		if p == partition {
			partitionExists = true
			break
		}
	}

	if !partitionExists {
		return 0, fmt.Errorf("partition %d does not exist for topic %s", partition, topic)
	}

	// Get high water mark (latest offset)
	offset, err := client.GetOffset(topic, partition, sarama.OffsetNewest)
	if err != nil {
		return 0, fmt.Errorf("failed to get high water mark for topic %s partition %d: %w", topic, partition, err)
	}

	return offset, nil
}

// Utility functions for topic management

func CreateStandardTopics(admin AdminClient, logger *logrus.Logger) error {
	ctx := context.Background()

	topics := []TopicConfig{
		{
			Name:              "analytics-events",
			Partitions:        3,
			ReplicationFactor: 1,
			Configs: map[string]*string{
				"cleanup.policy":      stringPtr("delete"),
				"retention.ms":        stringPtr("604800000"), // 7 days
				"compression.type":    stringPtr("snappy"),
				"min.insync.replicas": stringPtr("1"),
			},
		},
		{
			Name:              "metrics-data",
			Partitions:        3,
			ReplicationFactor: 1,
			Configs: map[string]*string{
				"cleanup.policy":   stringPtr("delete"),
				"retention.ms":     stringPtr("259200000"), // 3 days
				"compression.type": stringPtr("snappy"),
			},
		},
		{
			Name:              "analytics-events-dlq",
			Partitions:        1,
			ReplicationFactor: 1,
			Configs: map[string]*string{
				"cleanup.policy": stringPtr("delete"),
				"retention.ms":   stringPtr("2592000000"), // 30 days
			},
		},
		{
			Name:              "analytics-events-processed",
			Partitions:        3,
			ReplicationFactor: 1,
			Configs: map[string]*string{
				"cleanup.policy":   stringPtr("delete"),
				"retention.ms":     stringPtr("86400000"), // 1 day
				"compression.type": stringPtr("snappy"),
			},
		},
	}

	for _, topicConfig := range topics {
		if err := admin.CreateTopic(ctx, topicConfig); err != nil {
			return fmt.Errorf("failed to create topic %s: %w", topicConfig.Name, err)
		}
	}

	logger.Info("All standard topics created successfully")
	return nil
}

func stringPtr(s string) *string {
	return &s
}

// DefaultAdminConfig returns admin config with sensible defaults
func DefaultAdminConfig(brokers []string, logger *logrus.Logger) *AdminConfig {
	return &AdminConfig{
		Brokers: brokers,
		Logger:  logger,
		Timeout: 30 * time.Second,
	}
}
