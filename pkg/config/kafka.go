package config

import (
	"time"

	"github.com/IBM/sarama"
)

// KafkaAdvancedConfig contains advanced Kafka configuration
type KafkaAdvancedConfig struct {
	// Broker settings
	Brokers               []string      `mapstructure:"brokers"`
	ConnectionMaxIdleTime time.Duration `mapstructure:"connection_max_idle_time"`
	RequestTimeout        time.Duration `mapstructure:"request_timeout"`

	// Topics
	Topics TopicsConfig `mapstructure:"topics"`

	// Producer settings
	Producer ProducerAdvancedConfig `mapstructure:"producer"`

	// Consumer settings
	Consumer ConsumerAdvancedConfig `mapstructure:"consumer"`

	// Streams settings
	Streams StreamsConfig `mapstructure:"streams"`

	// Security settings
	Security SecurityConfig `mapstructure:"security"`

	// Monitoring
	Monitoring MonitoringConfig `mapstructure:"monitoring"`
}

type TopicsConfig struct {
	Events    string `mapstructure:"events"`
	Metrics   string `mapstructure:"metrics"`
	DLQ       string `mapstructure:"dlq"`
	Processed string `mapstructure:"processed"`
}

type ProducerAdvancedConfig struct {
	// Batching
	BatchSize       int           `mapstructure:"batch_size"`
	BatchTimeout    time.Duration `mapstructure:"batch_timeout"`
	MaxMessageBytes int           `mapstructure:"max_message_bytes"`
	FlushFrequency  time.Duration `mapstructure:"flush_frequency"`

	// Reliability
	RequiredAcks      string        `mapstructure:"required_acks"`
	MaxRetries        int           `mapstructure:"max_retries"`
	RetryBackoff      time.Duration `mapstructure:"retry_backoff"`
	EnableIdempotence bool          `mapstructure:"enable_idempotence"`

	// Compression
	CompressionType string `mapstructure:"compression_type"`

	// Performance
	LingerTime         time.Duration `mapstructure:"linger_time"`
	BufferMemory       int           `mapstructure:"buffer_memory"`
	SendBufferBytes    int           `mapstructure:"send_buffer_bytes"`
	ReceiveBufferBytes int           `mapstructure:"receive_buffer_bytes"`
}

type ConsumerAdvancedConfig struct {
	// Group settings
	GroupID           string        `mapstructure:"group_id"`
	SessionTimeout    time.Duration `mapstructure:"session_timeout"`
	HeartbeatInterval time.Duration `mapstructure:"heartbeat_interval"`
	MaxProcessingTime time.Duration `mapstructure:"max_processing_time"`

	// Fetch settings
	FetchMinBytes  int32         `mapstructure:"fetch_min_bytes"`
	FetchMaxWait   time.Duration `mapstructure:"fetch_max_wait"`
	MaxPollRecords int           `mapstructure:"max_poll_records"`

	// Offset management
	AutoCommit         bool          `mapstructure:"auto_commit"`
	AutoCommitInterval time.Duration `mapstructure:"auto_commit_interval"`
	OffsetReset        string        `mapstructure:"offset_reset"`

	// Retry settings
	EnableRetry  bool          `mapstructure:"enable_retry"`
	MaxRetries   int           `mapstructure:"max_retries"`
	RetryDelay   time.Duration `mapstructure:"retry_delay"`
	RetryBackoff float64       `mapstructure:"retry_backoff"`

	// Dead Letter Queue
	EnableDLQ bool   `mapstructure:"enable_dlq"`
	DLQTopic  string `mapstructure:"dlq_topic"`

	// Processing
	Concurrency int `mapstructure:"concurrency"`
	BufferSize  int `mapstructure:"buffer_size"`
}

type StreamsConfig struct {
	// Deduplication
	EnableDeduplication bool          `mapstructure:"enable_deduplication"`
	DeduplicationWindow time.Duration `mapstructure:"deduplication_window"`

	// Aggregation
	EnableAggregation    bool          `mapstructure:"enable_aggregation"`
	AggregationWindow    time.Duration `mapstructure:"aggregation_window"`
	AggregationThreshold int           `mapstructure:"aggregation_threshold"`

	// Filtering
	EnableFiltering     bool `mapstructure:"enable_filtering"`
	FilterLowImportance bool `mapstructure:"filter_low_importance"`

	// State management
	StateCleanupInterval time.Duration `mapstructure:"state_cleanup_interval"`
	MaxStateEntries      int           `mapstructure:"max_state_entries"`
}

type SecurityConfig struct {
	Protocol        string `mapstructure:"protocol"`
	SSLCALocation   string `mapstructure:"ssl_ca_location"`
	SSLCertLocation string `mapstructure:"ssl_cert_location"`
	SSLKeyLocation  string `mapstructure:"ssl_key_location"`
	SASLMechanism   string `mapstructure:"sasl_mechanism"`
	SASLUsername    string `mapstructure:"sasl_username"`
	SASLPassword    string `mapstructure:"sasl_password"`
}

type MonitoringConfig struct {
	RecordingLevel    string        `mapstructure:"recording_level"`
	SampleWindow      time.Duration `mapstructure:"sample_window"`
	NumSamples        int           `mapstructure:"num_samples"`
	EnableJMXReporter bool          `mapstructure:"enable_jmx_reporter"`
}

// Convert config to Sarama producer config
func (p *ProducerAdvancedConfig) ToSaramaConfig() *sarama.Config {
	config := sarama.NewConfig()
	config.Version = sarama.V3_0_0_0

	// Producer settings
	config.Producer.Return.Successes = true
	config.Producer.Return.Errors = true
	config.Producer.Partitioner = sarama.NewHashPartitioner
	config.Producer.Timeout = 10 * time.Second
	config.Producer.MaxMessageBytes = p.MaxMessageBytes

	// Batching
	config.Producer.Flush.Bytes = p.BatchSize
	config.Producer.Flush.Frequency = p.BatchTimeout
	config.Producer.Flush.Messages = 100

	// Retries
	config.Producer.Retry.Max = p.MaxRetries
	config.Producer.Retry.Backoff = p.RetryBackoff

	// Acks
	switch p.RequiredAcks {
	case "none":
		config.Producer.RequiredAcks = sarama.NoResponse
	case "leader":
		config.Producer.RequiredAcks = sarama.WaitForLocal
	case "all":
		config.Producer.RequiredAcks = sarama.WaitForAll
	default:
		config.Producer.RequiredAcks = sarama.WaitForAll
	}

	// Compression
	switch p.CompressionType {
	case "gzip":
		config.Producer.Compression = sarama.CompressionGZIP
	case "snappy":
		config.Producer.Compression = sarama.CompressionSnappy
	case "lz4":
		config.Producer.Compression = sarama.CompressionLZ4
	case "zstd":
		config.Producer.Compression = sarama.CompressionZSTD
	default:
		config.Producer.Compression = sarama.CompressionSnappy
	}

	// Idempotence
	if p.EnableIdempotence {
		config.Producer.Idempotent = true
		config.Net.MaxOpenRequests = 1
	}

	return config
}

// Convert config to Sarama consumer config
func (c *ConsumerAdvancedConfig) ToSaramaConfig() *sarama.Config {
	config := sarama.NewConfig()
	config.Version = sarama.V3_0_0_0

	// Consumer settings
	config.Consumer.Return.Errors = true
	config.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin
	config.Consumer.Group.Session.Timeout = c.SessionTimeout
	config.Consumer.Group.Heartbeat.Interval = c.HeartbeatInterval
	config.Consumer.MaxProcessingTime = c.MaxProcessingTime

	// Fetch settings
	config.Consumer.Fetch.Min = c.FetchMinBytes
	config.Consumer.Fetch.Max = 1024 * 1024 // 1MB
	config.Consumer.MaxWaitTime = c.FetchMaxWait

	// Offset settings
	if c.OffsetReset == "earliest" {
		config.Consumer.Offsets.Initial = sarama.OffsetOldest
	} else {
		config.Consumer.Offsets.Initial = sarama.OffsetNewest
	}

	if c.AutoCommit {
		config.Consumer.Offsets.AutoCommit.Enable = true
		config.Consumer.Offsets.AutoCommit.Interval = c.AutoCommitInterval
	}

	return config
}

// Default configurations
func DefaultKafkaAdvancedConfig() *KafkaAdvancedConfig {
	return &KafkaAdvancedConfig{
		Brokers:               []string{"localhost:9092"},
		ConnectionMaxIdleTime: 9 * time.Minute,
		RequestTimeout:        30 * time.Second,

		Topics: TopicsConfig{
			Events:    "analytics-events",
			Metrics:   "metrics-data",
			DLQ:       "analytics-events-dlq",
			Processed: "analytics-events-processed",
		},

		Producer: ProducerAdvancedConfig{
			BatchSize:          16384,
			BatchTimeout:       5 * time.Millisecond,
			MaxMessageBytes:    1048576,
			FlushFrequency:     1 * time.Second,
			RequiredAcks:       "all",
			MaxRetries:         5,
			RetryBackoff:       100 * time.Millisecond,
			EnableIdempotence:  true,
			CompressionType:    "snappy",
			LingerTime:         5 * time.Millisecond,
			BufferMemory:       33554432,
			SendBufferBytes:    131072,
			ReceiveBufferBytes: 65536,
		},

		Consumer: ConsumerAdvancedConfig{
			GroupID:            "analytics-consumer-group",
			SessionTimeout:     30 * time.Second,
			HeartbeatInterval:  3 * time.Second,
			MaxProcessingTime:  2 * time.Minute,
			FetchMinBytes:      1024,
			FetchMaxWait:       500 * time.Millisecond,
			MaxPollRecords:     500,
			AutoCommit:         true,
			AutoCommitInterval: 1 * time.Second,
			OffsetReset:        "earliest",
			EnableRetry:        true,
			MaxRetries:         3,
			RetryDelay:         1 * time.Second,
			RetryBackoff:       2.0,
			EnableDLQ:          true,
			DLQTopic:           "analytics-events-dlq",
			Concurrency:        3,
			BufferSize:         1000,
		},

		Streams: StreamsConfig{
			EnableDeduplication:  true,
			DeduplicationWindow:  5 * time.Minute,
			EnableAggregation:    true,
			AggregationWindow:    1 * time.Minute,
			AggregationThreshold: 10,
			EnableFiltering:      true,
			FilterLowImportance:  true,
			StateCleanupInterval: 30 * time.Second,
			MaxStateEntries:      10000,
		},

		Security: SecurityConfig{
			Protocol: "PLAINTEXT",
		},

		Monitoring: MonitoringConfig{
			RecordingLevel: "INFO",
			SampleWindow:   30 * time.Second,
			NumSamples:     2,
		},
	}
}
