package client

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Client represents a gRPC client for the Real-Time Analytics Platform
type Client struct {
	conn   *grpc.ClientConn
	config *ClientConfig
	logger *logrus.Logger
}

// ClientConfig holds configuration for the gRPC client
type ClientConfig struct {
	ServerAddress      string
	APIKey             string
	MaxRetries         int
	RetryDelay         time.Duration
	ConnectionTimeout  time.Duration
	RequestTimeout     time.Duration
	EnableKeepalive    bool
	KeepaliveTime      time.Duration
	KeepaliveTimeout   time.Duration
	MaxConnections     int
	MaxIdleConnections int
}

// DefaultConfig returns a default client configuration
func DefaultConfig() *ClientConfig {
	return &ClientConfig{
		ServerAddress:     "localhost:50051",
		MaxRetries:        3,
		RetryDelay:        time.Second,
		ConnectionTimeout: 10 * time.Second,
		RequestTimeout:    30 * time.Second,
		EnableKeepalive:   true,
		KeepaliveTime:     30 * time.Second,
		KeepaliveTimeout:  10 * time.Second,
		MaxConnections:    10,
	}
}

// NewClient creates a new gRPC client with connection pooling and retry logic
func NewClient(config *ClientConfig, logger *logrus.Logger) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if logger == nil {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)
	}

	// Set up connection options
	var opts []grpc.DialOption

	// TLS/Credentials (using insecure for development)
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))

	// Keepalive parameters
	if config.EnableKeepalive {
		kaParams := keepalive.ClientParameters{
			Time:                config.KeepaliveTime,
			Timeout:             config.KeepaliveTimeout,
			PermitWithoutStream: true,
		}
		opts = append(opts, grpc.WithKeepaliveParams(kaParams))
	}

	// Connection pool size
	opts = append(opts, grpc.WithDefaultCallOptions(
		grpc.MaxCallRecvMsgSize(10*1024*1024), // 10MB
		grpc.MaxCallSendMsgSize(10*1024*1024), // 10MB
	))

	// Interceptors
	opts = append(opts,
		grpc.WithChainUnaryInterceptor(
			unaryClientInterceptor(config, logger),
			retryInterceptor(config, logger),
		),
		grpc.WithChainStreamInterceptor(
			streamClientInterceptor(config, logger),
		),
	)

	// Establish connection
	ctx, cancel := context.WithTimeout(context.Background(), config.ConnectionTimeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, config.ServerAddress, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server: %w", err)
	}

	logger.WithField("address", config.ServerAddress).Info("Connected to gRPC server")

	return &Client{
		conn:   conn,
		config: config,
		logger: logger,
	}, nil
}

// Close closes the gRPC connection
func (c *Client) Close() error {
	if c.conn != nil {
		c.logger.Info("Closing gRPC connection")
		return c.conn.Close()
	}
	return nil
}

// GetConnection returns the underlying gRPC connection
func (c *Client) GetConnection() *grpc.ClientConn {
	return c.conn
}

// WithAPIKey adds API key to the context
func (c *Client) WithAPIKey(ctx context.Context) context.Context {
	if c.config.APIKey != "" {
		md := metadata.Pairs("x-api-key", c.config.APIKey)
		ctx = metadata.NewOutgoingContext(ctx, md)
	}
	return ctx
}

// WithRequestID adds a request ID to the context
func (c *Client) WithRequestID(ctx context.Context, requestID string) context.Context {
	md := metadata.Pairs("x-request-id", requestID)
	if existing, ok := metadata.FromOutgoingContext(ctx); ok {
		md = metadata.Join(md, existing)
	}
	return metadata.NewOutgoingContext(ctx, md)
}

// unaryClientInterceptor adds authentication and logging
func unaryClientInterceptor(config *ClientConfig, logger *logrus.Logger) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		start := time.Now()

		// Add API key if configured
		if config.APIKey != "" {
			md := metadata.Pairs("x-api-key", config.APIKey)
			ctx = metadata.NewOutgoingContext(ctx, md)
		}

		// Add request timeout
		ctx, cancel := context.WithTimeout(ctx, config.RequestTimeout)
		defer cancel()

		// Log request
		logger.WithFields(logrus.Fields{
			"method": method,
			"type":   "grpc_client_request",
		}).Debug("Sending gRPC request")

		// Invoke the RPC
		err := invoker(ctx, method, req, reply, cc, opts...)

		// Log response
		duration := time.Since(start)
		fields := logrus.Fields{
			"method":   method,
			"duration": duration.Milliseconds(),
			"type":     "grpc_client_response",
		}

		if err != nil {
			st, _ := status.FromError(err)
			fields["error"] = err.Error()
			fields["code"] = st.Code().String()
			logger.WithFields(fields).Error("gRPC request failed")
		} else {
			logger.WithFields(fields).Debug("gRPC request succeeded")
		}

		return err
	}
}

// streamClientInterceptor adds authentication and logging for streams
func streamClientInterceptor(config *ClientConfig, logger *logrus.Logger) grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		// Add API key if configured
		if config.APIKey != "" {
			md := metadata.Pairs("x-api-key", config.APIKey)
			ctx = metadata.NewOutgoingContext(ctx, md)
		}

		logger.WithField("method", method).Info("Opening gRPC stream")

		return streamer(ctx, desc, cc, method, opts...)
	}
}

// retryInterceptor implements retry logic
func retryInterceptor(config *ClientConfig, logger *logrus.Logger) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		var err error
		for attempt := 0; attempt <= config.MaxRetries; attempt++ {
			if attempt > 0 {
				// Wait before retrying
				select {
				case <-time.After(config.RetryDelay * time.Duration(attempt)):
				case <-ctx.Done():
					return ctx.Err()
				}

				logger.WithFields(logrus.Fields{
					"method":  method,
					"attempt": attempt,
				}).Warn("Retrying gRPC request")
			}

			err = invoker(ctx, method, req, reply, cc, opts...)

			// Check if we should retry
			if err == nil {
				return nil
			}

			st, ok := status.FromError(err)
			if !ok {
				return err
			}

			// Retry only on specific codes
			switch st.Code() {
			case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted:
				// Retry these errors
				continue
			default:
				// Don't retry other errors
				return err
			}
		}

		return err
	}
}

// ConnectionPool manages a pool of gRPC connections
type ConnectionPool struct {
	connections []*Client
	current     int
	config      *ClientConfig
	logger      *logrus.Logger
}

// NewConnectionPool creates a new connection pool
func NewConnectionPool(size int, config *ClientConfig, logger *logrus.Logger) (*ConnectionPool, error) {
	if size <= 0 {
		size = 5
	}

	pool := &ConnectionPool{
		connections: make([]*Client, size),
		config:      config,
		logger:      logger,
	}

	// Create connections
	for i := 0; i < size; i++ {
		client, err := NewClient(config, logger)
		if err != nil {
			// Close any already created connections
			pool.Close()
			return nil, fmt.Errorf("failed to create connection %d: %w", i, err)
		}
		pool.connections[i] = client
	}

	logger.WithField("pool_size", size).Info("Connection pool created")

	return pool, nil
}

// Get returns the next available connection
func (p *ConnectionPool) Get() *Client {
	p.current = (p.current + 1) % len(p.connections)
	return p.connections[p.current]
}

// Close closes all connections in the pool
func (p *ConnectionPool) Close() error {
	var lastErr error
	for _, conn := range p.connections {
		if conn != nil {
			if err := conn.Close(); err != nil {
				lastErr = err
			}
		}
	}
	return lastErr
}
