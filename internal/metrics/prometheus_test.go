package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsCollector_IncrementEventsProcessed(t *testing.T) {
	// Create custom registry for test isolation
	registry := prometheus.NewRegistry()

	collector := &MetricsCollector{
		EventsProcessed: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "test_events_processed_total",
				Help: "Test metric",
			},
			[]string{"service", "event_type", "status"},
		),
		EventsTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "test_events_total",
				Help: "Test metric",
			},
		),
	}

	registry.MustRegister(collector.EventsProcessed, collector.EventsTotal)

	// Test incrementing events
	collector.IncrementEventsProcessed("test", "page_view", "success")
	collector.IncrementEventsTotal()

	// Test passed if no panic occurred
}

func TestMetricsCollector_ObserveProcessingTime(t *testing.T) {
	// Create custom registry for test isolation
	registry := prometheus.NewRegistry()

	collector := &MetricsCollector{
		ProcessingTime: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "test_event_processing_duration_seconds",
				Help:    "Test metric",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"service", "event_type"},
		),
	}

	registry.MustRegister(collector.ProcessingTime)

	// Test observing processing time
	collector.ObserveProcessingTime("test", "page_view", 0.1)

	// Test passed if no panic occurred
}

func TestMetricsCollector_IncrementErrors(t *testing.T) {
	// Create custom registry for test isolation
	registry := prometheus.NewRegistry()

	collector := &MetricsCollector{
		ErrorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "test_errors_total",
				Help: "Test metric",
			},
			[]string{"service", "error_type"},
		),
	}

	registry.MustRegister(collector.ErrorsTotal)

	// Test incrementing errors
	collector.IncrementErrors("test", "kafka")
	collector.IncrementErrors("test", "validation")

	// Test passed if no panic occurred
}
