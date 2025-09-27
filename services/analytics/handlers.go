package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	pb "real-time-analytics-platform/api/proto"
)

func (s *AnalyticsService) GetAnalyticsSummary(ctx context.Context, req *pb.AnalyticsSummaryRequest) (*pb.AnalyticsSummary, error) {
	s.analytics.mutex.RLock()
	defer s.analytics.mutex.RUnlock()

	// Create top metrics (example)
	topMetrics := []*pb.Metric{
		{
			Name:      "page_views_per_minute",
			Value:     120.5,
			Labels:    map[string]string{"service": "web", "type": "view"},
			Timestamp: time.Now().Unix(),
		},
		{
			Name:      "user_sessions_active",
			Value:     45.0,
			Labels:    map[string]string{"service": "web", "type": "session"},
			Timestamp: time.Now().Unix(),
		},
	}

	return &pb.AnalyticsSummary{
		TotalEvents:  s.analytics.TotalEvents,
		TotalUsers:   s.analytics.TotalUsers,
		EventsByType: s.analytics.EventsByType,
		TopMetrics:   topMetrics,
		LastUpdated:  s.analytics.LastUpdated.Unix(),
	}, nil
}

func (s *AnalyticsService) GetMetrics(ctx context.Context, req *pb.MetricsRequest) (*pb.MetricsResponse, error) {
	// Simulate fetching metrics based on request
	var metricsData []*pb.Metric

	if req.MetricName != "" {
		// Fetch specific metric
		metricsData = append(metricsData, &pb.Metric{
			Name:      req.MetricName,
			Value:     100.0, // Example value
			Labels:    req.Filters,
			Timestamp: time.Now().Unix(),
		})
	} else {
		// Fetch all metrics in time range
		metricsData = []*pb.Metric{
			{
				Name:      "cpu_usage",
				Value:     75.5,
				Labels:    map[string]string{"instance": "web-1"},
				Timestamp: time.Now().Unix(),
			},
			{
				Name:      "memory_usage",
				Value:     60.2,
				Labels:    map[string]string{"instance": "web-1"},
				Timestamp: time.Now().Unix(),
			},
		}
	}

	return &pb.MetricsResponse{
		Metrics: metricsData,
		Success: true,
		Message: "Metrics retrieved successfully",
	}, nil
}

func (s *AnalyticsService) GetAnalyticsHealth(ctx context.Context, req *pb.HealthRequest) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{
		Healthy:   true,
		Status:    "Analytics service is healthy",
		Timestamp: time.Now().Unix(),
	}, nil
}

func (s *AnalyticsService) handleHTTPSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters for time range
	from := time.Now().Add(-time.Hour).Unix()
	to := time.Now().Unix()

	resp, err := s.GetAnalyticsSummary(r.Context(), &pb.AnalyticsSummaryRequest{
		From: from,
		To:   to,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error":"Failed to get analytics summary: %v"}`, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

func (s *AnalyticsService) runAnalyticsProcessor(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.processAnalytics()
		}
	}
}

func (s *AnalyticsService) processAnalytics() {
	s.analytics.mutex.Lock()
	defer s.analytics.mutex.Unlock()

	// Simulate analytics processing
	// In real implementation, this would:
	// - Fetch data from consumer service
	// - Calculate aggregations
	// - Update analytics data
	// - Store results in database

	s.analytics.TotalEvents += 100 // Simulate new events
	s.analytics.TotalUsers += 5    // Simulate new users

	// Update event types count
	eventTypes := []string{"page_view", "user_action", "system_event"}
	for _, eventType := range eventTypes {
		s.analytics.EventsByType[eventType] += 20
	}

	s.analytics.LastUpdated = time.Now()

	s.logger.WithFields(map[string]interface{}{
		"total_events": s.analytics.TotalEvents,
		"total_users":  s.analytics.TotalUsers,
	}).Debug("Analytics data updated")

	// Update metrics
	s.metrics.IncrementEventsProcessed("analytics", "aggregation", "success")
}
