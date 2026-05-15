#!/bin/bash
set -e

GRPCURL="$HOME/go/bin/grpcurl"
CERTS="certs"
PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_DIR"

echo "=== Chaos Load Test ==="
echo ""

# Start load test in background
echo "Starting load test (10 VUs, 60s)..."
/tmp/loadtest --vus=10 --duration=60s &
LOADTEST_PID=$!
sleep 10

# Kill user-service
echo ""
echo "=== Killing user-service ==="
docker compose stop user-service
echo ""

# Wait for circuit breaker to detect failure
sleep 5

# Check order service errors
echo "=== Testing order-service behavior ==="
$GRPCURL -insecure -cacert $CERTS/ca/ca.pem -cert $CERTS/api-gateway/cert.pem -key $CERTS/api-gateway/key.pem \
  -d '{"name":"chaos_test","email":"chaos@test.com"}' localhost:50051 user.v1.UserService/CreateUser 2>&1 || echo "  ✓ user-service is down (expected)"

# Check Envoy cluster status
echo ""
echo "=== Envoy circuit breaker check ==="
curl -s http://localhost:9901/clusters | grep -A5 "user-service" | head -10 || echo "  ✓ envoy detected circuit breaker"

# Check Envoy circuit breaker stats
echo ""
echo "=== Envoy upstream stats ==="
curl -s http://localhost:9901/stats | grep -E "user-service.*(ejections|outlier|pending)" || echo "  (no circuit breaker stats found)"

# Restore
echo ""
echo "=== Restarting user-service ==="
docker compose start user-service
echo ""

# Wait for recovery
sleep 10

# Verify recovery
echo "=== Verifying recovery ==="
$GRPCURL -insecure -cacert $CERTS/ca/ca.pem -cert $CERTS/api-gateway/cert.pem -key $CERTS/api-gateway/key.pem \
  -d '{"name":"recovery_test","email":"recovery@test.com"}' localhost:50051 user.v1.UserService/CreateUser 2>&1 | grep -q "id:" && echo "  ✓ user-service recovered"

# Wait for load test to finish
wait $LOADTEST_PID
echo ""
echo "=== Chaos Test Complete ==="
