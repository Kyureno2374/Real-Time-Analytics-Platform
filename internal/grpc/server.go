package grpc

import (
	"context"
	"net"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"real-time-analytics-platform/internal/metrics"
)

type Server struct {
	grpcServer *grpc.Server
	listener   net.Listener
	logger     *logrus.Logger
	metrics    *metrics.MetricsCollector
}

type ServerConfig struct {
	Port    string
	Logger  *logrus.Logger
	Metrics *metrics.MetricsCollector
}

func NewServer(config *ServerConfig) (*Server, error) {
	listener, err := net.Listen("tcp", ":"+config.Port)
	if err != nil {
		return nil, err
	}

	server := &Server{
		listener: listener,
		logger:   config.Logger,
		metrics:  config.Metrics,
	}

	// Create gRPC server with interceptors
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(server.unaryInterceptor),
		grpc.StreamInterceptor(server.streamInterceptor),
	)

	server.grpcServer = grpcServer

	return server, nil
}

func (s *Server) GetGRPCServer() *grpc.Server {
	return s.grpcServer
}

func (s *Server) Serve() error {
	s.logger.WithField("port", s.listener.Addr().String()).Info("Starting gRPC server")
	return s.grpcServer.Serve(s.listener)
}

func (s *Server) Stop() {
	s.grpcServer.GracefulStop()
}

func (s *Server) unaryInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	start := time.Now()

	resp, err := handler(ctx, req)

	duration := time.Since(start).Seconds()
	status := "success"
	if err != nil {
		status = "error"
		s.metrics.IncrementErrors("grpc", grpcErrorType(err))
	}

	s.metrics.ObserveProcessingTime("grpc", info.FullMethod, duration)
	s.metrics.IncrementEventsProcessed("grpc", info.FullMethod, status)

	s.logger.WithFields(logrus.Fields{
		"method":   info.FullMethod,
		"duration": duration,
		"status":   status,
	}).Debug("gRPC request processed")

	return resp, err
}

func (s *Server) streamInterceptor(
	srv interface{},
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	start := time.Now()

	err := handler(srv, ss)

	duration := time.Since(start).Seconds()
	status := "success"
	if err != nil {
		status = "error"
		s.metrics.IncrementErrors("grpc", grpcErrorType(err))
	}

	s.metrics.ObserveProcessingTime("grpc", info.FullMethod, duration)
	s.metrics.IncrementEventsProcessed("grpc", info.FullMethod, status)

	return err
}

func grpcErrorType(err error) string {
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.InvalidArgument:
			return "invalid_argument"
		case codes.NotFound:
			return "not_found"
		case codes.Internal:
			return "internal"
		case codes.Unavailable:
			return "unavailable"
		default:
			return "unknown"
		}
	}
	return "unknown"
}
