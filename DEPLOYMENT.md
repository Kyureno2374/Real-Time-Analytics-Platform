# Deployment Guide

## Development Environment

### Предварительные требования
- Docker 20.10+
- Docker Compose 2.0+
- Go 1.21+ (опционально для локальной разработки)

### Запуск

1. **Клонирование и настройка**
   ```bash
   git clone <repository-url>
   cd Real-Time_Analytics_Platform
   cp env.example .env  # Настройте переменные окружения
   ```

2. **Быстрый запуск**
   ```bash
   make setup
   ```

3. **Проверка работоспособности**
   ```bash
   make health
   make test-services
   ```

4. **Мониторинг**
   - Grafana: http://localhost:3000 (admin/admin)
   - Prometheus: http://localhost:9090
   - Producer API: http://localhost:8081/health

## Production Environment

### Kubernetes Deployment

1. **Подготовка образов**
   ```bash
   # Сборка образов
   docker build -t analytics-platform/producer:v1.0.0 -f services/producer/Dockerfile .
   docker build -t analytics-platform/consumer:v1.0.0 -f services/consumer/Dockerfile .
   docker build -t analytics-platform/analytics:v1.0.0 -f services/analytics/Dockerfile .
   
   # Загрузка в registry
   docker push analytics-platform/producer:v1.0.0
   docker push analytics-platform/consumer:v1.0.0
   docker push analytics-platform/analytics:v1.0.0
   ```

2. **Развертывание в Kubernetes**
   ```bash
   # Применение манифестов
   kubectl apply -f deployments/kubernetes/
   
   # Проверка статуса
   kubectl get pods -l component=analytics-platform
   kubectl get services -l component=analytics-platform
   ```

3. **Настройка мониторинга**
   ```bash
   # Установка Prometheus Operator
   helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
   helm install prometheus prometheus-community/kube-prometheus-stack
   
   # Установка Grafana dashboards
   kubectl apply -f config/grafana/
   ```

### Scaling

```bash
# Масштабирование Producer Service
kubectl scale deployment producer-service --replicas=5

# Масштабирование Consumer Service (по количеству партиций)
kubectl scale deployment consumer-service --replicas=3

# Масштабирование Analytics Service
kubectl scale deployment analytics-service --replicas=3
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `KAFKA_BROKERS` | Kafka brokers list | localhost:9092 |
| `GRPC_PORT` | gRPC server port | 50051 |
| `HTTP_PORT` | HTTP server port | 8081 |
| `LOG_LEVEL` | Logging level | info |
| `SERVICE_NAME` | Service name | analytics-service |
| `CONSUMER_GROUP_ID` | Kafka consumer group | analytics-consumer-group |

### Configuration Files

- `config/config.yaml` - основная конфигурация
- `config/production.yaml` - production настройки
- `config/prometheus/prometheus.yml` - конфигурация Prometheus
- `config/grafana/` - настройки и дашборды Grafana

## Monitoring and Alerting

### Ключевые метрики для мониторинга

1. **Throughput Metrics**
   - `events_processed_total` - общее количество событий
   - `rate(events_processed_total[1m])` - события в секунду

2. **Latency Metrics**
   - `event_processing_duration_seconds` - время обработки
   - `histogram_quantile(0.95, event_processing_duration_seconds)` - 95th percentile

3. **Error Metrics**
   - `errors_total` - общее количество ошибок
   - `rate(errors_total[5m])` - частота ошибок

4. **Kafka Metrics**
   - `kafka_consumer_lag_total` - отставание consumer'ов
   - `kafka_broker_availability` - доступность брокеров

### Алерты

```yaml
# Пример алертов для AlertManager
groups:
- name: analytics-platform
  rules:
  - alert: HighErrorRate
    expr: rate(errors_total[5m]) > 0.1
    for: 2m
    labels:
      severity: warning
    annotations:
      summary: "High error rate detected"
  
  - alert: HighConsumerLag
    expr: kafka_consumer_lag_total > 1000
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "Kafka consumer lag is too high"
```

## Security

### Production Security Checklist

- [ ] Включить TLS для gRPC соединений
- [ ] Настроить SASL/SCRAM для Kafka
- [ ] Использовать secrets для паролей
- [ ] Настроить network policies в Kubernetes
- [ ] Включить pod security policies
- [ ] Настроить RBAC для service accounts
- [ ] Использовать private Docker registry
- [ ] Включить audit logging

### TLS Configuration

```yaml
# Пример TLS конфигурации для gRPC
grpc:
  tls:
    enabled: true
    cert_file: "/etc/certs/server.crt"
    key_file: "/etc/certs/server.key"
    ca_file: "/etc/certs/ca.crt"
```

## Backup and Recovery

### Kafka Topics Backup
```bash
# Backup Kafka topics
kafka-mirror-maker --consumer.config consumer.properties \
  --producer.config producer.properties \
  --whitelist analytics-events,metrics-data
```

### Prometheus Data Backup
```bash
# Backup Prometheus data
kubectl exec prometheus-pod -- tar czf /tmp/prometheus-backup.tar.gz /prometheus
kubectl cp prometheus-pod:/tmp/prometheus-backup.tar.gz ./prometheus-backup.tar.gz
```

## Performance Tuning

### Kafka Tuning
```properties
# Producer settings
acks=all
retries=2147483647
batch.size=16384
linger.ms=5
compression.type=snappy

# Consumer settings
fetch.min.bytes=1024
fetch.max.wait.ms=500
max.poll.records=500
```

### Go Service Tuning
```bash
# Environment variables for Go runtime
GOMAXPROCS=4
GOGC=100
GOMEMLIMIT=512MiB
```

## Troubleshooting

### Common Issues

1. **Kafka Connection Issues**
   ```bash
   # Check Kafka connectivity
   docker exec kafka kafka-broker-api-versions --bootstrap-server localhost:9092
   ```

2. **Consumer Lag Issues**
   ```bash
   # Check consumer group status
   docker exec kafka kafka-consumer-groups --bootstrap-server localhost:9092 --describe --group analytics-consumer-group
   ```

3. **Memory Issues**
   ```bash
   # Check service memory usage
   kubectl top pods -l component=analytics-platform
   ```

### Debugging Commands

```bash
# View service logs
kubectl logs -f deployment/producer-service
kubectl logs -f deployment/consumer-service
kubectl logs -f deployment/analytics-service

# Check Kafka topics
make list-topics

# Monitor Kafka messages
make kafka-console

# Check metrics
curl http://localhost:8081/metrics
curl http://localhost:8082/metrics
curl http://localhost:8083/metrics
``` 