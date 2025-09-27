# Установка Docker и тестирование системы

## 1. Установка Docker

### Windows:
1. Скачайте Docker Desktop: https://www.docker.com/products/docker-desktop/
2. Запустите установщик и следуйте инструкциям
3. Перезагрузите компьютер после установки
4. Запустите Docker Desktop
5. Проверьте установку:
   ```powershell
   docker --version
   docker-compose --version
   ```

### Linux:
```bash
# Ubuntu/Debian
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo usermod -aG docker $USER

# CentOS/RHEL
sudo yum install -y docker
sudo systemctl start docker
sudo systemctl enable docker
```

## 2. Проверка исправлений

✅ **ИСПРАВЛЕНЫ ОШИБКИ КОМПИЛЯЦИИ:**
- `internal/kafka/admin.go:119` - Исправлено использование `DescribeTopics`
- `internal/kafka/admin.go:163` - Исправлено использование `DescribeConsumerGroups`
- Все тесты теперь используют `nil` метрики для избежания конфликтов Prometheus

✅ **ТЕСТЫ ПРОХОДЯТ:**
```powershell
go test ./...                    # Все unit тесты
go test ./internal/kafka/ -v     # Kafka тесты с подробностями
```

## 3. Запуск системы (после установки Docker)

### Первоначальная настройка:
```bash
make setup          # Настройка окружения
make build          # Сборка Docker образов
make run            # Запуск всех сервисов
```

### Проверка здоровья сервисов:
```bash
make health         # Проверка /health endpoints
make ready          # Проверка /ready endpoints
make lag            # Проверка consumer lag
```

### Тестирование:
```bash
make test                    # Unit тесты
make test-integration        # Integration тесты (требует Kafka)
make test-performance        # Performance тесты
make bench                   # Benchmarks
```

### Мониторинг:
```bash
make urls           # Показать все URL сервисов
make logs           # Просмотр логов
```

## 4. Доступные сервисы

После `make run`:
- **Producer Service**: http://localhost:8081
  - Health: http://localhost:8081/health
  - Metrics: http://localhost:8081/metrics
- **Consumer Service**: http://localhost:8082
  - Health: http://localhost:8082/health
  - Lag: http://localhost:8082/lag
- **Analytics Service**: http://localhost:8083
  - Health: http://localhost:8083/health
- **Prometheus**: http://localhost:9090
- **Grafana**: http://localhost:3000 (admin/admin)

## 5. Примеры использования

### Отправка событий:
```bash
curl -X POST http://localhost:8081/events \
  -H "Content-Type: application/json" \
  -d '{"user_id":"123","event_type":"click","timestamp":"2023-12-01T10:00:00Z"}'
```

### Проверка статистики:
```bash
curl http://localhost:8082/stats | jq .
curl http://localhost:8081/stats | jq .
```

### Управление топиками:
```bash
make create-topics   # Создать стандартные топики
make list-topics     # Список топиков
make kafka-console   # Консольный consumer
```

## 6. Особенности реализации

✅ **Production-ready функции:**
- Backpressure handling
- Manual offset commits
- Health checks и readiness probes
- Prometheus метрики
- Retry механизмы с DLQ
- Rate limiting
- Graceful shutdown
- Performance тесты

✅ **Kafka интеграция:**
- Sarama v1.42.1
- Admin API для управления топиками
- Consumer lag мониторинг
- Batch processing
- Compression (Snappy)
- Configurable retention policies 