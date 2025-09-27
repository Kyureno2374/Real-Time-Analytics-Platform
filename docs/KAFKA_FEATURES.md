# Kafka Advanced Features

## 🚀 **Новые возможности Producer и Consumer сервисов**

### **1. 🔒 Enhanced Security**

#### **Non-root Docker Containers**
Все сервисы теперь запускаются под non-root пользователем для повышения безопасности:
- User ID: 1001, Group ID: 1001
- Минимальные права доступа
- Multi-stage Docker builds для оптимизации размера образов

### **2. 📊 Advanced Consumer Lag Monitoring**

#### **Real-time Lag Calculation**
```bash
# Проверка consumer lag через Admin API
curl http://localhost:8082/lag | jq
```

Consumer теперь использует Kafka Admin API для точного расчета lag:
- Получение high water marks для каждой партиции
- Расчет lag = high_water_mark - consumer_offset
- Экспорт метрик lag в Prometheus
- Мониторинг состояния consumer groups

### **3. ⚡ Backpressure Handling**

#### **Rate Limiting**
```yaml
consumer:
  enable_backpressure: true
  max_concurrent_messages: 100
  message_buffer_size: 2000
  max_processing_rate: 500.0  # messages per second
```

Защита от перегрузки Consumer:
- Семафор для ограничения concurrent processing
- Rate limiting для контроля throughput
- Message buffering для сглаживания нагрузки
- Graceful degradation при высокой нагрузке

### **4. 🎯 Manual Offset Management**

#### **Precise Control Over Offsets**
```yaml
consumer:
  auto_commit: false
  enable_manual_commit: true
  commit_interval_ms: 2000
```

Возможности:
- Manual commit для критических сценариев
- Batch offset commits для производительности
- Recovery после failures без потери данных
- Fine-grained control над exactly-once semantics

### **5. 🏥 Enhanced Health Checks**

#### **Comprehensive Health Monitoring**
```bash
# Проверка здоровья с Kafka connectivity
curl http://localhost:8081/health | jq

# Проверка готовности к обработке
curl http://localhost:8082/ready | jq
```

Health checks включают:
- Kafka broker connectivity
- Topic availability
- Error rate monitoring
- Processing lag detection
- Service-specific health indicators

### **6. 📈 Performance Testing Framework**

#### **Automated Performance Validation**
```bash
# Запуск performance тестов
make test-performance

# Запуск benchmarks
make bench
```

Возможности:
- High-throughput testing (>1000 msgs/sec)
- Latency benchmarks (<10ms p99)
- Concurrent processing tests
- Memory usage validation
- Backpressure effectiveness testing

### **7. 🔄 Improved Error Recovery**

#### **Resilient Error Handling**
- Wait-for-Kafka startup logic
- Exponential backoff for retries
- Circuit breaker patterns (готовится)
- Graceful degradation при failures

### **8. 🐳 Production-Ready Deployment**

#### **Enhanced Docker Configuration**
```yaml
healthcheck:
  test: ["CMD", "wget", "--spider", "http://localhost:8082/health"]
  interval: 30s
  timeout: 10s
  retries: 3
  start_period: 40s
restart: unless-stopped
```

Возможности:
- Docker health checks
- Restart policies
- Graceful shutdown handling
- Resource optimization

## **🔧 Configuration Examples**

### **High-Throughput Producer**
```yaml
producer:
  batch_size: 32768            # 32KB batches
  batch_timeout_ms: 1          # Fast batching
  compression_type: "lz4"      # Fast compression
  max_retries: 10
  enable_idempotence: true
  flush_frequency_ms: 100
```

### **Resilient Consumer**
```yaml
consumer:
  enable_backpressure: true
  max_concurrent_messages: 100
  max_processing_rate: 500.0
  enable_manual_commit: true
  commit_interval_ms: 2000
  max_retries: 5
  enable_dlq: true
```

### **Production Monitoring**
```yaml
monitoring:
  health_check_interval: 30
  lag_threshold_warning: 1000
  lag_threshold_critical: 5000
  error_rate_threshold: 0.05
```

## **📊 Metrics & Monitoring**

### **New Prometheus Metrics**
- `kafka_consumer_lag_total` - Real-time consumer lag by topic/partition
- `kafka_connectivity_status` - Broker connectivity status  
- `backpressure_active_total` - Number of messages in backpressure queue
- `manual_commits_total` - Manual offset commits counter
- `processing_rate_current` - Current message processing rate

### **Enhanced Grafana Dashboards**
- Consumer lag trending graphs
- Backpressure utilization charts  
- Error rate alerting panels
- Throughput vs latency correlation

## **🚀 Performance Benchmarks**

Ожидаемые показатели производительности:

| Metric | Target | Production Ready |
|--------|--------|------------------|
| Producer Throughput | >1000 msgs/sec | ✅ |
| Consumer Processing | >500 msgs/sec | ✅ |
| Latency P99 | <10ms | ✅ |
| Error Rate | <5% | ✅ |
| Consumer Lag | <1000 messages | ✅ |
| Memory Usage | <512MB per service | ✅ |

## **🔍 Troubleshooting**

### **Common Issues**

1. **High Consumer Lag**
   ```bash
   # Check lag details
   make lag
   
   # Increase consumer concurrency
   # Set max_concurrent_messages higher
   ```

2. **Backpressure Activation**
   ```bash
   # Check processing rate
   curl http://localhost:8082/stats | jq '.consumer.processing_rate'
   
   # Tune rate limits if needed
   ```

3. **Manual Commit Issues**
   ```bash
   # Check offset management
   curl http://localhost:8082/stats | jq '.consumer.manual_commits'
   ``` 