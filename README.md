# Real-Time Analytics Platform

Микросервисная платформа аналитики в реальном времени, построенная на Go с использованием Apache Kafka, gRPC, Prometheus и Grafana.

## 🏗️ Архитектура

Платформа состоит из следующих компонентов:

- **Producer Service** - Сервис для отправки событий в Kafka (HTTP + gRPC API)
- **Consumer Service** - Сервис для обработки потоковых данных из Kafka
- **Analytics Service** - Сервис агрегации метрик и аналитики
- **Apache Kafka** - Брокер сообщений для потоковой обработки с партиционированием
- **Prometheus** - Система мониторинга и сбора метрик
- **Grafana** - Визуализация дашбордов и метрик

## 📁 Структура проекта

```
├── api/
│   └── proto/           # Protobuf определения и сгенерированные файлы
├── cmd/                 # Точки входа для каждого сервиса
│   ├── producer/
│   ├── consumer/
│   └── analytics/
├── config/              # Конфигурационные файлы
│   ├── prometheus/      # Настройки Prometheus
│   └── grafana/         # Настройки и дашборды Grafana
├── deployments/         # Kubernetes манифесты
│   └── kubernetes/
├── docs/                # Документация
├── examples/            # Примеры использования
│   └── client/
├── internal/            # Внутренние пакеты
│   ├── kafka/           # Kafka producer/consumer
│   ├── metrics/         # Prometheus метрики
│   └── grpc/           # gRPC сервер с middleware
├── pkg/                 # Публичные пакеты
│   ├── config/          # Управление конфигурацией
│   ├── logger/          # Настройка логирования
│   └── utils/           # Утилитные функции
├── scripts/             # Вспомогательные скрипты
└── services/            # Микросервисы
    ├── producer/        # Producer service
    ├── consumer/        # Consumer service
    └── analytics/       # Analytics service
```

## 🚀 Быстрый старт

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

## 📊 API Endpoints

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

## 🔧 Управление

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

## 📈 Мониторинг и метрики

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

## 🐳 Docker & Kubernetes

### Docker Compose
Для разработки используется `docker-compose.yml` с полной настройкой всех сервисов.

### Kubernetes
Готовые манифесты в `deployments/kubernetes/` для production развертывания.

## 🔌 gRPC Services

Все сервисы предоставляют gRPC API на портах 50051-50053:

- **ProducerService** (50051): SendEvent, SendBatchEvents, GetProducerHealth
- **ConsumerService** (50052): GetConsumerHealth, GetConsumerStats  
- **AnalyticsService** (50053): GetAnalyticsSummary, GetMetrics, GetAnalyticsHealth

## 📝 Примеры использования

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

## 🛠️ Разработка

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
- ✅ Следование SOLID принципам дизайна
- ✅ Comprehensive error handling и логирование
- ✅ Graceful shutdown для всех сервисов
- ✅ Health checks и readiness probes
- ✅ Prometheus метрики для мониторинга
- ✅ Structured logging с JSON форматом
- ✅ Партиционирование Kafka по user_id
- ✅ gRPC interceptors для метрик и логирования

## 📚 Документация

- [API Documentation](docs/API.md) - Подробное описание API
- [Architecture](docs/ARCHITECTURE.md) - Детали архитектуры системы

## 🤝 Contributing

1. Fork проект
2. Создайте feature branch
3. Сделайте изменения
4. Добавьте тесты
5. Запустите `make test`
6. Создайте Pull Request

## 📄 License

MIT License 