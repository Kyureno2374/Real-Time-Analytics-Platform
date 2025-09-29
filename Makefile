.PHONY: build run stop clean proto test setup test-services generate-data

# Build all services
build:
	docker-compose build

# Run all services
run:
	docker-compose up -d

# Stop all services
stop:
	docker-compose down

# Clean up containers and images
clean:
	docker-compose down -v
	docker system prune -f

# Generate protobuf files
proto:
	@echo "Generating protobuf files..."
	protoc -I. -I./api/proto -I./third_party \
		--go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		--grpc-gateway_out=. --grpc-gateway_opt=paths=source_relative \
		--grpc-gateway_opt=generate_unbound_methods=true \
		--validate_out=lang=go,paths=source_relative:. \
		api/proto/analytics.proto
	@echo "Protobuf files generated successfully"

# Install proto tools
proto-tools:
	@echo "Installing protobuf tools..."
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest
	go install github.com/envoyproxy/protoc-gen-validate@latest
	@echo "Protobuf tools installed successfully"

# Run tests
test:
	go test ./...

# Run integration tests  
test-integration:
	go test -v -run Integration ./internal/kafka/

# Run performance tests
test-performance:
	go test -v -run Performance ./internal/kafka/

# Run benchmarks
bench:
	go test -bench=. -benchmem ./internal/kafka/

# Build and run
dev: build run

# View logs
logs:
	docker-compose logs -f

# Setup project (first time)
setup:
	chmod +x scripts/setup.sh
	./scripts/setup.sh

# Test all services
test-services:
	chmod +x scripts/test-services.sh
	./scripts/test-services.sh

# Generate sample data
generate-data:
	go run scripts/generate-sample-data.go

# Run client example
example:
	go run examples/client/main.go

# Kafka topics management
create-topics:
	docker exec kafka kafka-topics --create --topic analytics-events --bootstrap-server localhost:9092 --partitions 3 --replication-factor 1 --if-not-exists
	docker exec kafka kafka-topics --create --topic metrics-data --bootstrap-server localhost:9092 --partitions 3 --replication-factor 1 --if-not-exists

# List Kafka topics
list-topics:
	docker exec kafka kafka-topics --list --bootstrap-server localhost:9092

# Monitor Kafka
kafka-console:
	docker exec -it kafka kafka-console-consumer --bootstrap-server localhost:9092 --topic analytics-events --from-beginning

# Check service health
health:
	@echo "Checking service health..."
	@curl -s http://localhost:8081/health | jq .
	@curl -s http://localhost:8082/health | jq .
	@curl -s http://localhost:8083/health | jq .

# Check service readiness
ready:
	@echo "Checking service readiness..."
	@curl -s http://localhost:8081/ready | jq .
	@curl -s http://localhost:8082/ready | jq .
	@curl -s http://localhost:8083/ready | jq .

# Check consumer lag
lag:
	@echo "Checking consumer lag..."
	@curl -s http://localhost:8082/lag | jq .

# Show service URLs
urls:
	@echo "Service URLs:"
	@echo "Producer Service: http://localhost:8081"
	@echo "Consumer Service: http://localhost:8082"
	@echo "Analytics Service: http://localhost:8083"
	@echo "Prometheus: http://localhost:9090"
	@echo "Grafana: http://localhost:3000 (admin/admin)" 