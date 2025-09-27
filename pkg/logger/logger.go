package logger

import (
	"os"

	"github.com/sirupsen/logrus"
)

func NewLogger(serviceName string) *logrus.Logger {
	logger := logrus.New()

	// Set formatter
	logger.SetFormatter(&logrus.JSONFormatter{
		TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime:  "timestamp",
			logrus.FieldKeyLevel: "level",
			logrus.FieldKeyMsg:   "message",
		},
	})

	// Set log level from environment or default to INFO
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	level, err := logrus.ParseLevel(logLevel)
	if err != nil {
		logger.WithError(err).Warn("Invalid log level, defaulting to INFO")
		level = logrus.InfoLevel
	}

	logger.SetLevel(level)

	// Add service name to all logs
	logger = logger.WithField("service", serviceName).Logger

	return logger
}
