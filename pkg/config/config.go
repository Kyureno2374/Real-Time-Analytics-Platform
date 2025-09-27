package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Kafka      KafkaConfig      `mapstructure:"kafka"`
	GRPC       GRPCConfig       `mapstructure:"grpc"`
	HTTP       HTTPConfig       `mapstructure:"http"`
	Prometheus PrometheusConfig `mapstructure:"prometheus"`
	Service    ServiceConfig    `mapstructure:"service"`
}

type KafkaConfig struct {
	Brokers           []string `mapstructure:"brokers"`
	TopicEvents       string   `mapstructure:"topic_events"`
	TopicMetrics      string   `mapstructure:"topic_metrics"`
	ConsumerGroupID   string   `mapstructure:"consumer_group_id"`
	PartitionStrategy string   `mapstructure:"partition_strategy"`
}

type GRPCConfig struct {
	Port string `mapstructure:"port"`
}

type HTTPConfig struct {
	Port string `mapstructure:"port"`
}

type PrometheusConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Path    string `mapstructure:"path"`
}

type ServiceConfig struct {
	Name        string `mapstructure:"name"`
	Environment string `mapstructure:"environment"`
	LogLevel    string `mapstructure:"log_level"`
}

func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./config")
	viper.AddConfigPath(".")

	// Set defaults
	setDefaults()

	// Enable environment variable override
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}

func setDefaults() {
	// Kafka defaults
	viper.SetDefault("kafka.brokers", []string{"localhost:9092"})
	viper.SetDefault("kafka.topic_events", "analytics-events")
	viper.SetDefault("kafka.topic_metrics", "metrics-data")
	viper.SetDefault("kafka.consumer_group_id", "analytics-consumer-group")
	viper.SetDefault("kafka.partition_strategy", "hash")

	// gRPC defaults
	viper.SetDefault("grpc.port", "50051")

	// HTTP defaults
	viper.SetDefault("http.port", "8081")

	// Prometheus defaults
	viper.SetDefault("prometheus.enabled", true)
	viper.SetDefault("prometheus.path", "/metrics")

	// Service defaults
	viper.SetDefault("service.name", "analytics-service")
	viper.SetDefault("service.environment", "development")
	viper.SetDefault("service.log_level", "info")
}
