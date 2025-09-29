package grpc

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"real-time-analytics-platform/internal/metrics"
)

// AuthInterceptor validates authentication tokens
type AuthInterceptor struct {
	logger *logrus.Logger
}

func NewAuthInterceptor(logger *logrus.Logger) *AuthInterceptor {
	return &AuthInterceptor{logger: logger}
}

func (a *AuthInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Skip auth for health checks
		if info.FullMethod == "/grpc.health.v1.Health/Check" ||
			info.FullMethod == "/analytics.v1.EventIngestionService/Health" ||
			info.FullMethod == "/analytics.v1.AnalyticsService/Health" ||
			info.FullMethod == "/analytics.v1.MetricsService/Health" {
			return handler(ctx, req)
		}

		// Extract metadata
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}

		// Check for API key or token
		apiKeys := md.Get("x-api-key")
		if len(apiKeys) == 0 {
			tokens := md.Get("authorization")
			if len(tokens) == 0 {
				return nil, status.Error(codes.Unauthenticated, "missing authentication credentials")
			}
			// In production, validate the token here
			// For now, we just check if it exists
		}

		// Add user info to context
		ctx = metadata.AppendToOutgoingContext(ctx, "authenticated", "true")

		return handler(ctx, req)
	}
}

func (a *AuthInterceptor) Stream() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// Extract metadata
		md, ok := metadata.FromIncomingContext(ss.Context())
		if !ok {
			return status.Error(codes.Unauthenticated, "missing metadata")
		}

		// Check for API key or token
		apiKeys := md.Get("x-api-key")
		if len(apiKeys) == 0 {
			tokens := md.Get("authorization")
			if len(tokens) == 0 {
				return status.Error(codes.Unauthenticated, "missing authentication credentials")
			}
		}

		return handler(srv, ss)
	}
}

// LoggingInterceptor provides detailed logging for all gRPC calls
type LoggingInterceptor struct {
	logger *logrus.Logger
}

func NewLoggingInterceptor(logger *logrus.Logger) *LoggingInterceptor {
	return &LoggingInterceptor{logger: logger}
}

func (l *LoggingInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()

		// Extract request metadata
		md, _ := metadata.FromIncomingContext(ctx)
		requestID := extractRequestID(md)

		l.logger.WithFields(logrus.Fields{
			"method":     info.FullMethod,
			"request_id": requestID,
			"type":       "grpc_request",
		}).Info("Handling gRPC request")

		// Call handler
		resp, err := handler(ctx, req)

		// Log response
		duration := time.Since(start)
		fields := logrus.Fields{
			"method":     info.FullMethod,
			"request_id": requestID,
			"duration":   duration.Milliseconds(),
			"type":       "grpc_response",
		}

		if err != nil {
			st, _ := status.FromError(err)
			fields["error"] = err.Error()
			fields["code"] = st.Code().String()
			l.logger.WithFields(fields).Error("gRPC request failed")
		} else {
			fields["status"] = "success"
			l.logger.WithFields(fields).Info("gRPC request completed")
		}

		return resp, err
	}
}

func (l *LoggingInterceptor) Stream() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()

		md, _ := metadata.FromIncomingContext(ss.Context())
		requestID := extractRequestID(md)

		l.logger.WithFields(logrus.Fields{
			"method":     info.FullMethod,
			"request_id": requestID,
			"is_server":  info.IsServerStream,
			"is_client":  info.IsClientStream,
			"type":       "grpc_stream_start",
		}).Info("Starting gRPC stream")

		err := handler(srv, ss)

		duration := time.Since(start)
		fields := logrus.Fields{
			"method":     info.FullMethod,
			"request_id": requestID,
			"duration":   duration.Milliseconds(),
			"type":       "grpc_stream_end",
		}

		if err != nil {
			fields["error"] = err.Error()
			l.logger.WithFields(fields).Error("gRPC stream failed")
		} else {
			l.logger.WithFields(fields).Info("gRPC stream completed")
		}

		return err
	}
}

// RecoveryInterceptor recovers from panics
type RecoveryInterceptor struct {
	logger *logrus.Logger
}

func NewRecoveryInterceptor(logger *logrus.Logger) *RecoveryInterceptor {
	return &RecoveryInterceptor{logger: logger}
}

func (r *RecoveryInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		defer func() {
			if rec := recover(); rec != nil {
				r.logger.WithFields(logrus.Fields{
					"method": info.FullMethod,
					"panic":  rec,
					"stack":  string(debug.Stack()),
				}).Error("gRPC handler panicked")
				err = status.Errorf(codes.Internal, "internal server error: %v", rec)
			}
		}()

		return handler(ctx, req)
	}
}

func (r *RecoveryInterceptor) Stream() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) (err error) {
		defer func() {
			if rec := recover(); rec != nil {
				r.logger.WithFields(logrus.Fields{
					"method": info.FullMethod,
					"panic":  rec,
					"stack":  string(debug.Stack()),
				}).Error("gRPC stream handler panicked")
				err = status.Errorf(codes.Internal, "internal server error: %v", rec)
			}
		}()

		return handler(srv, ss)
	}
}

// MetricsInterceptor collects metrics for gRPC calls
type MetricsInterceptor struct {
	metrics *metrics.MetricsCollector
}

func NewMetricsInterceptor(metrics *metrics.MetricsCollector) *MetricsInterceptor {
	return &MetricsInterceptor{metrics: metrics}
}

func (m *MetricsInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()

		resp, err := handler(ctx, req)

		duration := time.Since(start).Seconds()
		statusLabel := "success"
		if err != nil {
			statusLabel = grpcErrorType(err)
		}

		m.metrics.ObserveProcessingTime("grpc", info.FullMethod, duration)
		m.metrics.IncrementEventsProcessed("grpc", info.FullMethod, statusLabel)

		return resp, err
	}
}

func (m *MetricsInterceptor) Stream() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()

		err := handler(srv, ss)

		duration := time.Since(start).Seconds()
		statusLabel := "success"
		if err != nil {
			statusLabel = grpcErrorType(err)
		}

		m.metrics.ObserveProcessingTime("grpc_stream", info.FullMethod, duration)
		m.metrics.IncrementEventsProcessed("grpc_stream", info.FullMethod, statusLabel)

		return err
	}
}

// Helper functions
func extractRequestID(md metadata.MD) string {
	if requestIDs := md.Get("x-request-id"); len(requestIDs) > 0 {
		return requestIDs[0]
	}
	return fmt.Sprintf("req-%d", time.Now().UnixNano())
}
