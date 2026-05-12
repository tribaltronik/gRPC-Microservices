#!/bin/bash
# Wait for all Docker Compose services to be ready
#
# Polls each service's container status. Handles:
#   - Services with health checks → waits for "(healthy)"
#   - Init containers (vault-init) → waits for "Exit 0"
#   - Services without health checks → waits for "Up"

set -euo pipefail

MAX_ATTEMPTS=45
SLEEP=2

echo "Waiting for services to become healthy..."

wait_for_service() {
  local name=$1
  local attempt=0

  while [ $attempt -lt $MAX_ATTEMPTS ]; do
    local status
    status=$(docker compose ps -a --format '{{.Status}}' "$name" 2>/dev/null | head -1 || echo "not found")

    case "$status" in
      *healthy*)
        echo "  ✓ $name — healthy"
        return 0
        ;;
      *"Exited (0)"*)
        echo "  ✓ $name — completed"
        return 0
        ;;
      *Up*)
        # Running but no health check defined — good enough
        echo "  ✓ $name — running"
        return 0
        ;;
      *Exit*)
        echo "  ✗ $name — failed: $status"
        docker compose logs --tail=20 "$name" 2>/dev/null || true
        return 1
        ;;
    esac

    attempt=$((attempt + 1))
    if [ $((attempt % 10)) -eq 0 ]; then
      echo "  … $name ($((attempt * SLEEP))s)"
    fi
    sleep $SLEEP
  done

  echo "  ✗ $name — not ready after $((MAX_ATTEMPTS * SLEEP))s"
  docker compose logs --tail=20 "$name" 2>/dev/null || true
  return 1
}

# Infrastructure — must be healthy first
for svc in vault user-db order-db redis; do
  wait_for_service "$svc" || exit 1
done

# Vault init — depends on DBs and vault
wait_for_service vault-init || exit 1

# Application services
for svc in user-service order-service ext-authz envoy; do
  wait_for_service "$svc" || exit 1
done

echo ""
echo "All services are healthy"
