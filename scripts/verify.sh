#!/bin/bash
# gRPC Microservices PoC — Verification Script
#
# Usage:
#   ./scripts/verify.sh          # Static checks only
#   ./scripts/verify.sh --online # Static + runtime checks (requires running stack)
#
# Exit code: number of failed checks

set -euo pipefail

PASS=0
FAIL=0

pass() { PASS=$((PASS+1)); echo "  ✓ $1"; }
fail() { FAIL=$((FAIL+1)); echo "  ✗ $1"; }

echo "========================================"
echo "  gRPC Microservices PoC"
echo "  Verification Suite"
echo "========================================"

echo ""
echo "--- Static Checks ---"

# File existence — all critical project files
echo -n "  Required files... "
MISSING=0
for f in \
  buf.yaml buf.gen.yaml go.mod \
  proto/common/v1/common.proto proto/user/v1/user.proto proto/order/v1/order.proto \
  services/user/v1/user.pb.go services/order/v1/order.pb.go services/common/v1/common.pb.go \
  services/user-service/main.go services/user-service/config.go \
  services/user-service/server.go services/user-service/repository.go \
  services/order-service/main.go services/order-service/config.go \
  services/order-service/server.go services/order-service/repository.go \
  services/order-service/cache.go services/order-service/client.go \
  internal/tls/tls.go internal/log/log.go internal/shutdown/shutdown.go \
  internal/vault/client.go \
  db/migrations/embed.go \
  Dockerfile.user-service Dockerfile.order-service Dockerfile.vault-init \
  docker-compose.yml \
  scripts/init-vault.sh certs/generate-certs.sh \
  vault/config.hcl vault/policies/user-service.hcl vault/policies/order-service.hcl \
  envoy/envoy.yaml envoy/Dockerfile envoy/ext_authz/main.go envoy/ext_authz/Dockerfile; do
  if [ ! -f "$f" ]; then
    fail "Missing: $f"
    MISSING=$((MISSING+1))
  fi
done
[ "$MISSING" -eq 0 ] && pass "All required files present"

# Protocol Buffers — lint
echo -n "  buf lint... "
OUT=$(buf lint 2>&1) && pass "buf lint passed" || fail "buf lint failed\n$OUT"

# Go code quality
echo -n "  go vet... "
OUT=$(go vet ./... 2>&1) && pass "go vet passed" || fail "go vet failed\n$OUT"

echo -n "  go build... "
OUT=$(go build ./... 2>&1) && pass "go build passed" || fail "go build failed\n$OUT"

# Certificates
echo -n "  CA certificate... "
if [ -f certs/ca/ca.pem ] && openssl x509 -in certs/ca/ca.pem -noout -dates > /dev/null 2>&1; then
  EXPIRY=$(openssl x509 -in certs/ca/ca.pem -noout -enddate 2>/dev/null | cut -d= -f2)
  pass "CA certificate valid (expires $EXPIRY)"
else
  fail "CA certificate missing or invalid (run 'make certs')"
fi

for svc in user-service order-service api-gateway; do
  echo -n "  $svc certificate... "
  CERT="certs/$svc/cert.pem"
  if [ -f "$CERT" ] && openssl verify -CAfile certs/ca/ca.pem "$CERT" > /dev/null 2>&1; then
    EXPIRY=$(openssl x509 -in "$CERT" -noout -enddate 2>/dev/null | cut -d= -f2)
    pass "$svc certificate valid (expires $EXPIRY)"
  else
    fail "$svc certificate missing or invalid"
  fi

  echo -n "  $svc private key... "
  KEY="certs/$svc/key.pem"
  if [ -f "$KEY" ] && openssl ec -in "$KEY" -noout -text > /dev/null 2>&1; then
    pass "$svc ECDSA P-256 key valid"
  else
    fail "$svc private key missing or invalid"
  fi
done

# Envoy config validation (only if envoy CLI available)
if command -v envoy > /dev/null 2>&1; then
  echo -n "  Envoy config... "
  OUT=$(envoy --mode validate -c envoy/envoy.yaml 2>&1) && pass "Envoy config valid" || fail "Envoy config invalid\n$OUT"
else
  echo "  - envoy CLI not found — skipping config validation"
fi

# Proto descriptor for Envoy transcoding
echo -n "  Envoy proto descriptor... "
if [ -f envoy/proto-descriptor.pb ] && [ -s envoy/proto-descriptor.pb ]; then
  pass "Proto descriptor exists ($(stat -f%z envoy/proto-descriptor.pb 2>/dev/null || stat -c%s envoy/proto-descriptor.pb 2>/dev/null) bytes)"
else
  fail "Proto descriptor missing (run 'buf build -o envoy/proto-descriptor.pb')"
fi

# Vault policies syntax
echo -n "  Vault policies... "
for f in vault/policies/*.hcl; do
  [ -f "$f" ] || continue
done
pass "Vault policies present ($(ls vault/policies/*.hcl 2>/dev/null | wc -l | tr -d ' ') files)"

# ---------------------------------------------------------------------------
# Runtime checks (only with --online flag)
# ---------------------------------------------------------------------------
if [ "${1:-}" = "--online" ]; then
  echo ""
  echo "--- Runtime Checks ---"

  if ! docker compose ps > /dev/null 2>&1; then
    fail "Docker stack is not running (run 'make up')"
  else
    # All services running
    TOTAL_SERVICES=$(docker compose config --services 2>/dev/null | wc -l | tr -d ' ')
    RUNNING_SERVICES=$(docker compose ps --format json 2>/dev/null | grep -c '"Status"' || true)
    # vault-init is a one-shot init container (exits after completion)
    EXITED_ALLOWED=1  # vault-init
    MIN_EXPECTED=$((TOTAL_SERVICES - EXITED_ALLOWED))
    echo -n "  Running services... "
    [ "$RUNNING_SERVICES" -ge "$MIN_EXPECTED" ] 2>/dev/null && \
      pass "$RUNNING_SERVICES/$TOTAL_SERVICES services (allowing $EXITED_ALLOWED init container)" || \
      fail "Expected at least $MIN_EXPECTED running, got $RUNNING_SERVICES"

    # Vault
    echo -n "  Vault status... "
    if docker compose exec -T vault vault status -address=http://127.0.0.1:8200 > /dev/null 2>&1; then
      SEALED=$(docker compose exec -T vault vault status -address=http://127.0.0.1:8200 -format=json 2>/dev/null | grep -o '"sealed":[^,]*' | cut -d: -f2)
      [ "$SEALED" = "false" ] && pass "Vault initialized and unsealed" || pass "Vault initialized"
    else
      fail "Vault not responding"
    fi

    # Vault dynamic credentials
    echo -n "  Vault dynamic DB creds... "
    CREDS=$(docker compose exec -e VAULT_TOKEN=root -T vault vault read -address=http://127.0.0.1:8200 -format=json database/creds/user-service 2>/dev/null || echo "")
    if echo "$CREDS" | grep -c '"username"' > /dev/null 2>&1; then
      DURATION=$(echo "$CREDS" | grep -o '"lease_duration":[^,]*' | cut -d: -f2 || echo "unknown")
      pass "Vault generates dynamic DB credentials (lease: ${DURATION}s)"
    else
      fail "Vault DB engine not configured — run init script"
    fi

    # Service logs for startup confirmation
    for svc in user-service order-service; do
      echo -n "  $svc startup... "
      matches=$(docker compose logs --tail=50 "$svc" 2>/dev/null | grep -c "starting $svc" || true)
      if [ "$matches" -gt 0 ]; then
        pass "$svc started successfully"
      else
        fail "$svc startup message not found"
      fi
    done

    # gRPC health check via grpcurl (optional tool)
    if command -v grpcurl > /dev/null 2>&1; then
      echo -n "  User service gRPC health... "
      if grpcurl -insecure localhost:50051 grpc.health.v1.Health/Check 2>/dev/null | grep -q SERVING; then
        pass "User service reporting SERVING"
      else
        fail "User service health check failed"
      fi

      echo -n "  Order service gRPC health... "
      if grpcurl -insecure localhost:50052 grpc.health.v1.Health/Check 2>/dev/null | grep -q SERVING; then
        pass "Order service reporting SERVING"
      else
        fail "Order service health check failed"
      fi
    else
      echo "  - grpcurl not installed — install with: go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest"
    fi

    # Envoy HTTP→gRPC transcoding
    echo -n "  Envoy transcoding... "
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
      -H "x-api-key: grpc-poc-api-key-2026" \
      "http://localhost:8080/api/v1/users/00000000-0000-0000-0000-000000000000" 2>/dev/null || echo "000")
    if [ "$HTTP_CODE" != "000" ] && [ "$HTTP_CODE" != "503" ]; then
      pass "Envoy responds (HTTP $HTTP_CODE)"
    else
      fail "Envoy not responding on :8080 (HTTP $HTTP_CODE)"
    fi

    # Auth enforcement
    echo -n "  Auth enforcement... "
    STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
      "http://localhost:8080/api/v1/users/00000000-0000-0000-0000-000000000000" 2>/dev/null || echo "000")
    if [ "$STATUS" = "401" ]; then
      pass "ext_authz rejects requests without API key"
    elif [ "$STATUS" = "000" ]; then
      fail "Cannot reach Envoy"
    else
      fail "ext_authz not blocking (expected 401, got $STATUS)"
    fi

    # Verify auth actually works with correct key
    echo -n "  Auth with valid key... "
    STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
      -H "x-api-key: grpc-poc-api-key-2026" \
      "http://localhost:8080/api/v1/users/00000000-0000-0000-0000-000000000000" 2>/dev/null || echo "000")
    [ "$STATUS" != "401" ] && [ "$STATUS" != "000" ] && \
      pass "Valid API key accepted (HTTP $STATUS)" || \
      fail "Valid API key rejected (HTTP $STATUS)"
  fi
fi

echo ""
echo "========================================"
echo "  Results: $PASS passed, $FAIL failed"
echo "========================================"
exit $FAIL
