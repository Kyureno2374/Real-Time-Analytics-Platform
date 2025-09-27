FROM golang:1.21-alpine AS builder

# Install git and ca-certificates for dependency downloads
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the consumer service with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -a -installsuffix cgo \
    -ldflags='-w -s -extldflags "-static"' \
    -o consumer ./services/consumer

# Runtime stage
FROM alpine:latest

# Install ca-certificates and timezone data
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user for security
RUN addgroup -g 1001 appgroup && \
    adduser -D -u 1001 -G appgroup appuser

WORKDIR /home/appuser

# Copy the binary from builder
COPY --from=builder /app/consumer ./consumer

# Copy config files
COPY --from=builder /app/config ./config

# Change ownership to non-root user
RUN chown -R appuser:appgroup /home/appuser

# Switch to non-root user
USER appuser

# Expose ports for HTTP and gRPC
EXPOSE 8082 50052

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8082/health || exit 1

# Graceful shutdown - consumer service handles SIGTERM properly
STOPSIGNAL SIGTERM

# Run the binary
CMD ["./consumer"] 