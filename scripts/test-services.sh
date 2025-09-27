#!/bin/bash

set -e

echo "Testing Real-Time Analytics Platform Services..."

BASE_URL_PRODUCER="http://localhost:8081"
BASE_URL_CONSUMER="http://localhost:8082"
BASE_URL_ANALYTICS="http://localhost:8083"

# Test Producer Service Health
echo "Testing Producer Service Health..."
response=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL_PRODUCER/health")
if [ "$response" = "200" ]; then
    echo "✓ Producer service is healthy"
else
    echo "✗ Producer service health check failed (HTTP $response)"
fi

# Test Consumer Service Health
echo "Testing Consumer Service Health..."
response=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL_CONSUMER/health")
if [ "$response" = "200" ]; then
    echo "✓ Consumer service is healthy"
else
    echo "✗ Consumer service health check failed (HTTP $response)"
fi

# Test Analytics Service Health
echo "Testing Analytics Service Health..."
response=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL_ANALYTICS/health")
if [ "$response" = "200" ]; then
    echo "✓ Analytics service is healthy"
else
    echo "✗ Analytics service health check failed (HTTP $response)"
fi

# Test sending an event
echo "Testing event sending..."
event_data='{
    "id": "test_event_'$(date +%s)'",
    "user_id": "test_user",
    "event_type": "page_view",
    "properties": {
        "page": "/test",
        "browser": "curl"
    },
    "timestamp": '$(date +%s)',
    "source": "test_script"
}'

response=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -d "$event_data" \
    "$BASE_URL_PRODUCER/events")

if [ "$response" = "200" ]; then
    echo "✓ Event sent successfully"
else
    echo "✗ Event sending failed (HTTP $response)"
fi

# Test analytics summary
echo "Testing analytics summary..."
response=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL_ANALYTICS/analytics/summary")
if [ "$response" = "200" ]; then
    echo "✓ Analytics summary retrieved successfully"
else
    echo "✗ Analytics summary failed (HTTP $response)"
fi

echo ""
echo "Service testing completed!"
echo ""
echo "You can also check:"
echo "- Prometheus metrics: http://localhost:9090"
echo "- Grafana dashboard: http://localhost:3000 (admin/admin)" 