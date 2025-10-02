# Real-Time Analytics Platform

Микросервисная платформа аналитики в реальном времени, построенная на Go с использованием Apache Kafka, gRPC, Prometheus и Grafana.

## Основные возможности

- **High-Performance gRPC API** - Полнофункциональный gRPC API с поддержкой REST через grpc-gateway
- **Real-Time Event Processing** - Обработка событий в реальном времени с использованием Apache Kafka
- **Streaming Support** - Bidirectional streaming для высокопроизводительной передачи данных
- **Authentication & Authorization** - Встроенная аутентификация через API keys и токены
- **Comprehensive Analytics** - Агрегация и анализ данных с поддержкой funnel analysis и time series
- **Validation & Error Handling** - Автоматическая валидация запросов и продвинутая обработка ошибок
- **Client Library** - Go client library с connection pooling и автоматическими retry
- **Monitoring & Metrics** - Полная интеграция с Prometheus и Grafana
- **Request Tracing** - Поддержка request IDs для отслеживания запросов
- **Health Checks** - Встроенные health checks для всех сервисов

## Архитектура

Платформа состоит из следующих компонентов:

- **Event Ingestion Service** - gRPC/REST сервис для приема событий с валидацией и streaming
- **Analytics Service** - Сервис для запросов агрегированных данных и real-time analytics
- **Metrics Service** - Экспорт метрик в различных форматах (Prometheus, JSON, CSV)
- **Consumer Service** - Сервис для обработки потоковых данных из Kafka
- **Apache Kafka** - Брокер сообщений для потоковой обработки с партиционированием
- **Prometheus** - Система мониторинга и сбора метрик
- **Grafana** - Визуализация дашбордов и метрик

## Быстрый старт

### Предварительные требования
- Docker и Docker Compose
- Go 1.21+ (для локальной разработки)
- Make (опционально)

### Запуск платформы

1. **Клонируйте репозиторий**
   ```bash
   git clone <repository-url>
   cd Real-Time_Analytics_Platform
   ```

2. **Первоначальная настройка**
   ```bash
   make setup
   ```
   Или вручную:
   ```bash
   docker-compose build
   docker-compose up -d zookeeper kafka prometheus grafana
   sleep 30  # Ждем готовности Kafka
   make create-topics
   docker-compose up -d producer-service consumer-service analytics-service
   ```

3. **Проверка состояния сервисов**
   ```bash
   make health
   ```

4. **Открыть дашборды**
   - Grafana: http://localhost:3000 (admin/admin)
   - Prometheus: http://localhost:9090

## gRPC API

Платформа предоставляет полнофункциональный gRPC API с поддержкой REST через grpc-gateway.

### Быстрый старт с gRPC

#### 1. Установка инструментов для протобуферов

```bash
make proto-tools
```

#### 2. Генерация Go кода из proto файлов

```bash
make proto
```

#### 3. Использование Go Client Library

```go
package main

import (
    "context"
    "log"
    
    "github.com/google/uuid"
    "google.golang.org/protobuf/types/known/timestamppb"
    
    pb "real-time-analytics-platform/api/proto"
    "real-time-analytics-platform/pkg/client"
)

func main() {
    // Создать клиент
    grpcClient, err := client.NewClient(nil, nil)
    if err != nil {
        log.Fatal(err)
    }
    defer grpcClient.Close()
    
    // Создать клиент для ingestion
    eventClient := pb.NewEventIngestionServiceClient(grpcClient.GetConnection())
    
    // Отправить событие
    event := &pb.Event{
        Id:        uuid.New().String(),
        UserId:    "user-123",
        EventType: pb.EventType_EVENT_TYPE_USER,
        Timestamp: timestamppb.Now(),
        Source:    "web-app",
    }
    
    ctx := grpcClient.WithAPIKey(context.Background())
    resp, err := eventClient.IngestEvent(ctx, &pb.IngestEventRequest{
        Event: event,
    })
    
    if err != nil {
        log.Fatalf("Failed: %v", err)
    }
    
    log.Printf("Event ingested: %s", resp.EventId)
}
```

#### 4. Использование REST API (grpc-gateway)

```bash
# Отправить событие
curl -X POST http://localhost:8080/v1/events \
  -H "x-api-key: your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "event": {
      "id": "evt-123",
      "user_id": "user-456",
      "event_type": "EVENT_TYPE_USER",
      "timestamp": "2024-01-01T12:00:00Z",
      "source": "web-app"
    }
  }'

# Получить аналитику
curl -X GET "http://localhost:8080/v1/analytics/summary" \
  -H "x-api-key: your-api-key"
```

### Доступные gRPC сервисы

#### EventIngestionService
- `IngestEvent` - Отправка одного события
- `IngestEvents` - Batch отправка до 1000 событий
- `StreamEvents` - Bidirectional streaming для высокой пропускной способности
- `Health` - Проверка здоровья сервиса

#### AnalyticsService
- `GetAnalyticsSummary` - Получить агрегированную сводку
- `GetTimeSeries` - Получить временные ряды
- `GetRealTimeAnalytics` - Real-time streaming аналитики
- `GetUserAnalytics` - Аналитика по конкретному пользователю
- `GetFunnelAnalysis` - Анализ воронки конверсии

#### MetricsService
- `GetMetrics` - Получить метрики
- `StreamMetrics` - Streaming метрик в реальном времени
- `ExportMetrics` - Экспорт в формате Prometheus/JSON/CSV

### Примеры использования

```bash
# Запустить пример клиента
go run examples/grpc_client/main.go

# Тестирование с grpcurl
grpcurl -plaintext localhost:50051 list
```

### Подробная документация

- [gRPC API Documentation](./docs/GRPC_API.md) - Полная документация API
- [gRPC Quick Start Guide](./docs/GRPC_QUICKSTART.md) - Руководство по быстрому старту
- [Client Examples](./examples/grpc_client/) - Примеры использования клиента

### Тестирование

```bash
# Запуск unit тестов
make test

# Запуск integration тестов с Kafka
make test-integration

# Запуск performance тестов
make test-performance

# Запуск benchmarks
make bench

# Тестирование всех сервисов
make test-services

# Генерация тестовых данных
make generate-data

# Запуск примера клиента
make example
```

## API Endpoints

### Producer Service (port 8081)
- `POST /events` - Отправка событий (синхронно)
- `POST /events/async` - Асинхронная отправка событий
- `POST /events/batch` - Batch отправка событий для high-throughput
- `GET /health` - Проверка здоровья сервиса (включая Kafka connectivity)
- `GET /ready` - Проверка готовности к отправке сообщений
- `GET /stats` - Статистика Producer (throughput, latency, errors)
- `GET /metrics` - Prometheus метрики

### Consumer Service (port 8082)
- `GET /health` - Проверка здоровья сервиса (включая Kafka connectivity)
- `GET /ready` - Проверка готовности к обработке сообщений
- `GET /stats` - Детальная статистика обработки
- `GET /lag` - Мониторинг consumer lag через Kafka Admin API
- `GET /metrics` - Prometheus метрики

### Analytics Service (port 8083)
- `GET /analytics/summary` - Общая аналитика
- `GET /health` - Проверка здоровья сервиса
- `GET /metrics` - Prometheus метрики

## Управление

### Основные команды
```bash
make build       # Сборка всех сервисов
make run         # Запуск всех сервисов
make stop        # Остановка всех сервисов
make clean       # Очистка контейнеров и образов
make logs        # Просмотр логов всех сервисов
make health      # Проверка здоровья всех сервисов
make ready       # Проверка готовности всех сервисов
make lag         # Проверка consumer lag
make urls        # Показать URLs всех сервисов
```

### Kafka операции
```bash
make create-topics    # Создание топиков
make list-topics      # Список топиков
make kafka-console    # Мониторинг сообщений
```

## Мониторинг и метрики

### Prometheus метрики
- `events_processed_total` - Количество обработанных событий
- `event_processing_duration_seconds` - Время обработки
- `kafka_consumer_lag_total` - Отставание consumer'ов
- `errors_total` - Количество ошибок
- `service_uptime_seconds` - Время работы сервисов

### Grafana дашборды
- Real-Time Analytics Dashboard - основные метрики платформы
- Service Health Dashboard - здоровье сервисов
- Kafka Metrics Dashboard - метрики Kafka

## Docker & Kubernetes

### Docker Compose
Для разработки используется `docker-compose.yml` с полной настройкой всех сервисов.

### Kubernetes
Готовые манифесты в `deployments/kubernetes/` для production развертывания.

## gRPC Services

Все сервисы предоставляют gRPC API на портах 50051-50053:

- **ProducerService** (50051): SendEvent, SendBatchEvents, GetProducerHealth
- **ConsumerService** (50052): GetConsumerHealth, GetConsumerStats  
- **AnalyticsService** (50053): GetAnalyticsSummary, GetMetrics, GetAnalyticsHealth

## Примеры использования

### Отправка события через HTTP
```bash
curl -X POST http://localhost:8081/events \
  -H "Content-Type: application/json" \
  -d '{
    "id": "event_123",
    "user_id": "user_456", 
    "event_type": "page_view",
    "properties": {
      "page": "/dashboard",
      "duration": "45"
    },
    "timestamp": 1640995200,
    "source": "web-app"
  }'
```

### Получение аналитики
```bash
curl http://localhost:8083/analytics/summary | jq
```

## Разработка

### Локальная разработка
```bash
# Инициализация зависимостей
go mod tidy

# Генерация protobuf (требует protoc)
make proto

# Запуск тестов
go test ./...

# Запуск отдельного сервиса
go run services/producer/main.go services/producer/handlers.go
```

### Добавление новых метрик
1. Определите метрику в `internal/metrics/prometheus.go`
2. Используйте метрику в коде сервиса
3. Добавьте запрос в Grafana дашборд

### Best Practices
-  Следование SOLID принципам дизайна
-  Comprehensive error handling и логирование
-  Graceful shutdown для всех сервисов
-  Health checks и readiness probes
-  Prometheus метрики для мониторинга
-  Structured logging с JSON форматом
-  Партиционирование Kafka по user_id
-  gRPC interceptors для метрик и логирования

## 📄 License

MIT License 
