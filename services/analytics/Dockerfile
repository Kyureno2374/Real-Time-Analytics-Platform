FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the analytics service
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o analytics ./services/analytics

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/analytics .

# Create config directory
RUN mkdir -p /root/config

# Expose ports
EXPOSE 8083 50053

# Run the binary
CMD ["./analytics"] 