package metrics

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type MetricsCollector struct {
	EventsProcessed   *prometheus.CounterVec
	EventsTotal       prometheus.Counter
	ProcessingTime    *prometheus.HistogramVec
	ErrorsTotal       *prometheus.CounterVec
	KafkaConsumerLag  *prometheus.GaugeVec
	ServiceUptime     *prometheus.GaugeVec
	ActiveConnections *prometheus.GaugeVec
}

func NewMetricsCollector(serviceName string) *MetricsCollector {
	eventsProcessed := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "events_processed_total",
			Help: "Total number of events processed",
		},
		[]string{"service", "event_type", "status"},
	)

	eventsTotal := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "events_total",
			Help: "Total number of events received",
		},
	)

	processingTime := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "event_processing_duration_seconds",
			Help:    "Time spent processing events",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"service", "event_type"},
	)

	errorsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "errors_total",
			Help: "Total number of errors",
		},
		[]string{"service", "error_type"},
	)

	kafkaConsumerLag := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "kafka_consumer_lag_total",
			Help: "Current lag of consumer group",
		},
		[]string{"topic", "partition", "consumer_group"},
	)

	serviceUptime := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "service_uptime_seconds",
			Help: "Service uptime in seconds",
		},
		[]string{"service"},
	)

	activeConnections := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "active_connections_total",
			Help: "Number of active connections",
		},
		[]string{"service", "connection_type"},
	)

	// Register metrics
	prometheus.MustRegister(
		eventsProcessed,
		eventsTotal,
		processingTime,
		errorsTotal,
		kafkaConsumerLag,
		serviceUptime,
		activeConnections,
	)

	return &MetricsCollector{
		EventsProcessed:   eventsProcessed,
		EventsTotal:       eventsTotal,
		ProcessingTime:    processingTime,
		ErrorsTotal:       errorsTotal,
		KafkaConsumerLag:  kafkaConsumerLag,
		ServiceUptime:     serviceUptime,
		ActiveConnections: activeConnections,
	}
}

func (m *MetricsCollector) IncrementEventsProcessed(service, eventType, status string) {
	m.EventsProcessed.WithLabelValues(service, eventType, status).Inc()
}

func (m *MetricsCollector) IncrementEventsTotal() {
	m.EventsTotal.Inc()
}

func (m *MetricsCollector) ObserveProcessingTime(service, eventType string, duration float64) {
	m.ProcessingTime.WithLabelValues(service, eventType).Observe(duration)
}

func (m *MetricsCollector) IncrementErrors(service, errorType string) {
	m.ErrorsTotal.WithLabelValues(service, errorType).Inc()
}

func (m *MetricsCollector) SetKafkaConsumerLag(topic, partition, consumerGroup string, lag float64) {
	m.KafkaConsumerLag.WithLabelValues(topic, partition, consumerGroup).Set(lag)
}

func (m *MetricsCollector) SetServiceUptime(service string, uptime float64) {
	m.ServiceUptime.WithLabelValues(service).Set(uptime)
}

func (m *MetricsCollector) SetActiveConnections(service, connectionType string, count float64) {
	m.ActiveConnections.WithLabelValues(service, connectionType).Set(count)
}

func StartMetricsServer(port string) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(fmt.Sprintf("Failed to start metrics server: %v", err))
		}
	}()

	return server
}
