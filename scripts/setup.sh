#!/bin/bash

set -e

echo "Setting up Real-Time Analytics Platform..."

# Check if Docker is running
if ! docker info >/dev/null 2>&1; then
    echo "Docker is not running. Please start Docker and try again."
    exit 1
fi

# Check if Docker Compose is available
if ! command -v docker-compose &> /dev/null; then
    echo "Docker Compose is not installed. Please install Docker Compose and try again."
    exit 1
fi

# Build all services
echo "Building services..."
docker-compose build

# Start infrastructure services first
echo "Starting infrastructure services..."
docker-compose up -d zookeeper kafka prometheus grafana

# Wait for Kafka to be ready
echo "Waiting for Kafka to be ready..."
sleep 30

# Create Kafka topics
echo "Creating Kafka topics..."
docker exec kafka kafka-topics --create --topic analytics-events --bootstrap-server localhost:9092 --partitions 3 --replication-factor 1 --if-not-exists
docker exec kafka kafka-topics --create --topic metrics-data --bootstrap-server localhost:9092 --partitions 3 --replication-factor 1 --if-not-exists

# Start application services
echo "Starting application services..."
docker-compose up -d producer-service consumer-service analytics-service

echo "Setup complete!"
echo ""
echo "Services are now running:"
echo "- Producer Service: http://localhost:8081"
echo "- Consumer Service: http://localhost:8082"
echo "- Analytics Service: http://localhost:8083"
echo "- Prometheus: http://localhost:9090"
echo "- Grafana: http://localhost:3000 (admin/admin)"
echo ""
echo "To view logs: make logs"
echo "To stop services: make stop" 