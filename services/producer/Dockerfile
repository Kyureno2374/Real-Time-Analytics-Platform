FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the producer service
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o producer ./services/producer

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/producer .

# Create config directory
RUN mkdir -p /root/config

# Expose ports
EXPOSE 8081 50051

# Run the binary
CMD ["./producer"] 