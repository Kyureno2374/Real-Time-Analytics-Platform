# Architecture Documentation

## Overview

Real-Time Analytics Platform - это микросервисная платформа для обработки и анализа данных в реальном времени, построенная на Go с использованием Apache Kafka, gRPC, Prometheus и Grafana.

## Architecture Diagram

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Web Client    │    │   Mobile App    │    │   IoT Device    │
└─────────┬───────┘    └─────────┬───────┘    └─────────┬───────┘
          │                      │                      │
          └──────────────────────┼──────────────────────┘
                                 │
                    ┌─────────────┴───────────┐
                    │   Producer Service      │
                    │   (HTTP + gRPC)         │
                    │   Port: 8081, 50051     │
                    └─────────────┬───────────┘
                                 │
                    ┌─────────────┴───────────┐
                    │     Apache Kafka        │
                    │   (Message Broker)      │
                    │     Port: 9092          │
                    └─────────────┬───────────┘
                                 │
                    ┌─────────────┴───────────┐
                    │   Consumer Service      │
                    │   (Stream Processing)   │
                    │   Port: 8082, 50052     │
                    └─────────────┬───────────┘
                                 │
                    ┌─────────────┴───────────┐
                    │  Analytics Service      │
                    │   (Aggregation)         │
                    │   Port: 8083, 50053     │
                    └─────────────┬───────────┘
                                 │
          ┌──────────────────────┼──────────────────────┐
          │                      │                      │
┌─────────┴───────┐    ┌─────────┴───────┐    ┌─────────┴───────┐
│   Prometheus    │    │    Grafana      │    │   Alerting      │
│   (Metrics)     │    │  (Dashboard)    │    │   (Optional)    │
│   Port: 9090    │    │   Port: 3000    │    │                 │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## Components

### Producer Service
- **Responsibility**: Принимает события от клиентов и отправляет их в Kafka
- **Technologies**: Go, gRPC, HTTP REST API, Kafka Producer
- **Ports**: 8081 (HTTP), 50051 (gRPC)
- **Scaling**: Stateless, можно масштабировать горизонтально

### Consumer Service
- **Responsibility**: Обрабатывает потоковые данные из Kafka
- **Technologies**: Go, Kafka Consumer, gRPC
- **Ports**: 8082 (HTTP), 50052 (gRPC)
- **Scaling**: Масштабируется по количеству партиций Kafka

### Analytics Service
- **Responsibility**: Агрегация метрик и предоставление аналитических данных
- **Technologies**: Go, gRPC, HTTP REST API
- **Ports**: 8083 (HTTP), 50053 (gRPC)
- **Scaling**: Stateless, можно масштабировать горизонтально

### Apache Kafka
- **Responsibility**: Брокер сообщений для потоковой обработки
- **Topics**: 
  - `analytics-events` - основные события (3 партиции)
  - `metrics-data` - метрические данные (3 партиции)
- **Port**: 9092

### Prometheus
- **Responsibility**: Сбор и хранение метрик
- **Port**: 9090
- **Metrics**: События, ошибки, производительность, Kafka лаги

### Grafana
- **Responsibility**: Визуализация метрик и дашборды
- **Port**: 3000
- **Credentials**: admin/admin

## Data Flow

1. **Event Ingestion**:
   - Клиенты отправляют события в Producer Service через HTTP/gRPC
   - Producer валидирует события и отправляет в Kafka топик
   - Kafka автоматически распределяет события по партициям

2. **Stream Processing**:
   - Consumer Service читает события из Kafka
   - Обрабатывает и валидирует каждое событие
   - Выполняет бизнес-логику обработки

3. **Analytics Aggregation**:
   - Analytics Service периодически агрегирует данные
   - Вычисляет метрики и статистики
   - Предоставляет API для получения аналитики

4. **Monitoring**:
   - Все сервисы экспортируют метрики в Prometheus
   - Grafana отображает метрики в реальном времени
   - Алерты на основе пороговых значений

## Partitioning Strategy

События в Kafka партиционируются по `user_id` для:
- Обеспечения порядка событий для каждого пользователя
- Равномерного распределения нагрузки
- Эффективного параллельного потребления

## Error Handling

- **Producer**: Retry mechanism с экспоненциальным backoff
- **Consumer**: Dead letter queue для failed messages
- **gRPC**: Interceptors для логирования и метрик
- **Kafka**: At-least-once delivery guarantee

## Security

- **gRPC**: TLS encryption (в production)
- **Kafka**: SASL/SCRAM authentication (в production)
- **Metrics**: Basic auth для Prometheus/Grafana endpoints

## Deployment

### Docker Compose (Development)
```bash
make setup    # Первоначальная настройка
make dev      # Сборка и запуск
make logs     # Просмотр логов
```

### Kubernetes (Production)
```bash
kubectl apply -f deployments/kubernetes/
```

## Monitoring and Alerting

### Key Metrics
- `events_processed_total` - Общее количество обработанных событий
- `event_processing_duration_seconds` - Время обработки событий
- `kafka_consumer_lag_total` - Отставание consumer'ов
- `errors_total` - Общее количество ошибок

### Grafana Dashboards
- Real-Time Analytics Dashboard
- Kafka Metrics Dashboard
- Service Health Dashboard

## Performance Characteristics

- **Throughput**: 10,000+ events/second per Consumer instance
- **Latency**: <100ms end-to-end latency
- **Availability**: 99.9% uptime with proper deployment
- **Scalability**: Horizontal scaling по demand 