package kafka

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	pb "real-time-analytics-platform/api/proto"
)

// FilterProcessor filters events based on criteria
func FilterProcessor(filterFunc func(*pb.Event) bool) ProcessorFunc {
	return func(ctx context.Context, event *pb.Event) (*pb.Event, error) {
		if filterFunc(event) {
			return event, nil
		}
		return nil, nil // Filter out
	}
}

// EnrichProcessor enriches events with additional data
func EnrichProcessor(enrichFunc func(*pb.Event) error) ProcessorFunc {
	return func(ctx context.Context, event *pb.Event) (*pb.Event, error) {
		if err := enrichFunc(event); err != nil {
			return nil, fmt.Errorf("enrichment failed: %w", err)
		}
		return event, nil
	}
}

// TransformProcessor transforms events
func TransformProcessor(transformFunc func(*pb.Event) (*pb.Event, error)) ProcessorFunc {
	return func(ctx context.Context, event *pb.Event) (*pb.Event, error) {
		return transformFunc(event)
	}
}

// ValidationProcessor validates events
func ValidationProcessor(rules map[string]func(string) bool) ProcessorFunc {
	return func(ctx context.Context, event *pb.Event) (*pb.Event, error) {
		// Validate required fields
		if event.Id == "" {
			return nil, fmt.Errorf("event ID is required")
		}
		if event.UserId == "" {
			return nil, fmt.Errorf("user ID is required")
		}
		if event.EventType == "" {
			return nil, fmt.Errorf("event type is required")
		}

		// Apply custom validation rules
		for field, rule := range rules {
			value, exists := event.Properties[field]
			if exists && !rule(value) {
				return nil, fmt.Errorf("validation failed for field %s", field)
			}
		}

		return event, nil
	}
}

// AggregationProcessor aggregates events (stateful)
type AggregationProcessor struct {
	window     time.Duration
	store      map[string]*AggregationState
	storeMutex sync.RWMutex
	logger     *logrus.Logger
}

type AggregationState struct {
	Count      int64
	LastUpdate time.Time
	Properties map[string]interface{}
	FirstEvent time.Time
}

func NewAggregationProcessor(window time.Duration, logger *logrus.Logger) *AggregationProcessor {
	processor := &AggregationProcessor{
		window: window,
		store:  make(map[string]*AggregationState),
		logger: logger,
	}

	// Start cleanup goroutine
	go processor.cleanupExpiredWindows()

	return processor
}

func (a *AggregationProcessor) Process(ctx context.Context, event *pb.Event) (*pb.Event, error) {
	key := fmt.Sprintf("%s:%s", event.UserId, event.EventType)

	a.storeMutex.Lock()
	defer a.storeMutex.Unlock()

	state, exists := a.store[key]
	if !exists || time.Since(state.LastUpdate) > a.window {
		state = &AggregationState{
			Count:      0,
			LastUpdate: time.Now(),
			FirstEvent: time.Now(),
			Properties: make(map[string]interface{}),
		}
		a.store[key] = state
	}

	state.Count++
	state.LastUpdate = time.Now()

	// Update aggregated properties
	for k, v := range event.Properties {
		state.Properties[k] = v // Last value wins strategy
	}

	// Create aggregated event only when window closes or count reaches threshold
	if state.Count >= 10 || time.Since(state.FirstEvent) >= a.window {
		aggregatedEvent := &pb.Event{
			Id:        fmt.Sprintf("agg_%s_%d", key, time.Now().UnixNano()),
			UserId:    event.UserId,
			EventType: event.EventType + "_aggregated",
			Properties: map[string]string{
				"count":        fmt.Sprintf("%d", state.Count),
				"window_ms":    fmt.Sprintf("%d", a.window.Milliseconds()),
				"original_id":  event.Id,
				"first_event":  fmt.Sprintf("%d", state.FirstEvent.Unix()),
				"window_start": fmt.Sprintf("%d", state.FirstEvent.Unix()),
				"window_end":   fmt.Sprintf("%d", state.LastUpdate.Unix()),
			},
			Timestamp: time.Now().Unix(),
			Source:    "aggregation_processor",
		}

		// Reset state for next window
		delete(a.store, key)

		return aggregatedEvent, nil
	}

	// Don't emit event yet, still aggregating
	return nil, nil
}

func (a *AggregationProcessor) cleanupExpiredWindows() {
	ticker := time.NewTicker(a.window / 2)
	defer ticker.Stop()

	for range ticker.C {
		a.storeMutex.Lock()

		now := time.Now()
		expired := 0
		for key, state := range a.store {
			if now.Sub(state.LastUpdate) > a.window {
				delete(a.store, key)
				expired++
			}
		}

		if expired > 0 {
			a.logger.WithField("expired_windows", expired).Debug("Cleaned up expired aggregation windows")
		}

		a.storeMutex.Unlock()
	}
}

// DeduplicationProcessor removes duplicate events
type DeduplicationProcessor struct {
	window     time.Duration
	seenEvents map[string]time.Time
	mutex      sync.RWMutex
	logger     *logrus.Logger
}

func NewDeduplicationProcessor(window time.Duration, logger *logrus.Logger) *DeduplicationProcessor {
	processor := &DeduplicationProcessor{
		window:     window,
		seenEvents: make(map[string]time.Time),
		logger:     logger,
	}

	// Start cleanup goroutine
	go processor.cleanupExpiredEvents()

	return processor
}

func (d *DeduplicationProcessor) Process(ctx context.Context, event *pb.Event) (*pb.Event, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if lastSeen, exists := d.seenEvents[event.Id]; exists {
		if time.Since(lastSeen) <= d.window {
			// Duplicate event within window
			d.logger.WithFields(logrus.Fields{
				"event_id":  event.Id,
				"last_seen": lastSeen,
				"window_ms": d.window.Milliseconds(),
			}).Debug("Duplicate event filtered")
			return nil, nil
		}
	}

	d.seenEvents[event.Id] = time.Now()
	return event, nil
}

func (d *DeduplicationProcessor) cleanupExpiredEvents() {
	ticker := time.NewTicker(d.window / 2)
	defer ticker.Stop()

	for range ticker.C {
		d.mutex.Lock()

		now := time.Now()
		expired := 0
		for eventId, timestamp := range d.seenEvents {
			if now.Sub(timestamp) > d.window {
				delete(d.seenEvents, eventId)
				expired++
			}
		}

		if expired > 0 {
			d.logger.WithField("expired_events", expired).Debug("Cleaned up expired deduplication cache")
		}

		d.mutex.Unlock()
	}
}

// RouteProcessor routes events to different topics based on criteria
type RouteProcessor struct {
	routes map[string]string // event_type -> topic_name
	logger *logrus.Logger
}

func NewRouteProcessor(routes map[string]string, logger *logrus.Logger) *RouteProcessor {
	return &RouteProcessor{
		routes: routes,
		logger: logger,
	}
}

func (r *RouteProcessor) Process(ctx context.Context, event *pb.Event) (*pb.Event, error) {
	// Add routing information to event properties
	if targetTopic, exists := r.routes[event.EventType]; exists {
		if event.Properties == nil {
			event.Properties = make(map[string]string)
		}
		event.Properties["target_topic"] = targetTopic

		r.logger.WithFields(logrus.Fields{
			"event_type":   event.EventType,
			"target_topic": targetTopic,
			"event_id":     event.Id,
		}).Debug("Event routed")
	}

	return event, nil
}

// Common processor factories

// CreatePageViewProcessor creates processor for page view events
func CreatePageViewProcessor() ProcessorFunc {
	return func(ctx context.Context, event *pb.Event) (*pb.Event, error) {
		if event.EventType != "page_view" {
			return event, nil
		}

		// Enrich page view events
		if event.Properties == nil {
			event.Properties = make(map[string]string)
		}

		// Add session information
		event.Properties["session_id"] = fmt.Sprintf("session_%s_%d", event.UserId, time.Now().Unix()/3600)

		// Extract domain from page URL
		if page, exists := event.Properties["page"]; exists {
			event.Properties["page_category"] = categorizePagePath(page)
		}

		return event, nil
	}
}

// CreateUserActionProcessor creates processor for user action events
func CreateUserActionProcessor() ProcessorFunc {
	return func(ctx context.Context, event *pb.Event) (*pb.Event, error) {
		if event.EventType != "user_action" {
			return event, nil
		}

		// Enrich user action events
		if event.Properties == nil {
			event.Properties = make(map[string]string)
		}

		// Categorize action importance
		if action, exists := event.Properties["action"]; exists {
			event.Properties["action_importance"] = categorizeActionImportance(action)
		}

		return event, nil
	}
}

// CreateErrorEventProcessor creates processor for system error events
func CreateErrorEventProcessor() ProcessorFunc {
	return func(ctx context.Context, event *pb.Event) (*pb.Event, error) {
		if event.EventType != "system_event" {
			return event, nil
		}

		// Check if it's an error event
		if level, exists := event.Properties["level"]; exists && level == "error" {
			// Transform to error event
			event.EventType = "error_event"

			if event.Properties == nil {
				event.Properties = make(map[string]string)
			}

			event.Properties["severity"] = "high"
			event.Properties["alert_required"] = "true"
		}

		return event, nil
	}
}

// Helper functions

func categorizePagePath(path string) string {
	switch {
	case strings.HasPrefix(path, "/dashboard"):
		return "dashboard"
	case strings.HasPrefix(path, "/products"):
		return "product"
	case strings.HasPrefix(path, "/admin"):
		return "admin"
	case strings.HasPrefix(path, "/api"):
		return "api"
	default:
		return "general"
	}
}

func categorizeActionImportance(action string) string {
	highImportanceActions := map[string]bool{
		"purchase": true,
		"signup":   true,
		"login":    true,
		"logout":   true,
		"delete":   true,
	}

	if highImportanceActions[action] {
		return "high"
	}

	mediumImportanceActions := map[string]bool{
		"click":  true,
		"search": true,
		"submit": true,
		"upload": true,
	}

	if mediumImportanceActions[action] {
		return "medium"
	}

	return "low"
}
