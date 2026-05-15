# Operations Runbook

## Common Operations

### Start the Stack
```bash
make up
```
This generates certificates, builds Docker images, starts all 12 services, and waits for health checks.

### Stop the Stack
```bash
make down      # Stop and remove all containers + volumes (destructive)
make stop      # Stop containers without deleting volumes (preserves data)
```

### View Logs
```bash
make logs                          # All services (follow)
make logs-user-service             # Specific service
docker compose logs -f envoy       # Or directly with docker
```

### Check Service Status
```bash
make ps
docker compose ps
docker compose ps --status running
```

### Run Integration Tests
```bash
make test-integration              # All tests
make test-integration-auth         # Auth tests only
make test-integration-mtls         # mTLS tests only
make test-integration-resilience   # DB resilience tests
```
Requires running stack (`make up`).

### Run Load Tests
```bash
make load-test                     # 25 VUs, 30s
make load-test-heavy               # 50 VUs, 30s
make load-test-chaos               # Kill user-service during load
```

### Run Verification
```bash
make verify                        # Static checks (no Docker needed)
make verify-online                 # Full checks (requires running stack)
```

## Monitoring URLs

| Service | URL | Auth |
|---------|-----|------|
| Envoy HTTP | http://localhost:8080 | API key (`x-api-key`) |
| Envoy Admin | http://localhost:9901 | None |
| Grafana | http://localhost:3000 | admin:admin |
| Prometheus | http://localhost:9090 | None |
| Jaeger UI | http://localhost:16686 | None |
| Vault UI | http://localhost:8200 | root token |

## Service Health Checks

### gRPC Health Check
```bash
grpcurl -insecure \
  -cacert certs/ca/ca.pem \
  -cert certs/api-gateway/cert.pem \
  -key certs/api-gateway/key.pem \
  localhost:50051 grpc.health.v1.Health/Check
```
Expected: `{"status":"SERVING"}`

### Prometheus Targets
```
http://localhost:9090/targets
```
All targets (envoy, user-service, order-service) should show `UP`.

## Certificate Rotation

### Planned Rotation (11-month cycle)
Certs are valid for 1 year. Rotate at month 11:
```bash
# Regenerate all certificates
certs/generate-certs.sh

# Verify chains
openssl verify -CAfile certs/ca/ca.pem certs/user-service/cert.pem
openssl verify -CAfile certs/ca/ca.pem certs/order-service/cert.pem
openssl verify -CAfile certs/ca/ca.pem certs/api-gateway/cert.pem

# Restart services to pick up new certs
docker compose restart user-service order-service envoy
```

### Emergency Rotation (compromised key)
```bash
# 1. Generate new CA + all certs
rm -rf certs && certs/generate-certs.sh

# 2. Restart entire stack
make up
```

### Expiry Monitoring
```bash
# Check cert expiry dates
openssl x509 -in certs/user-service/cert.pem -noout -enddate
openssl x509 -in certs/order-service/cert.pem -noout -enddate
openssl x509 -in certs/ca/ca.pem -noout -enddate
```

## Troubleshooting

### Envoy Returns 404 for HTTP Requests
**Problem**: REST calls to `http://localhost:8080/api/v1/users` return 404.
**Root cause**: gRPC-JSON transcoding filter cannot match HTTP routes to RPC methods.
**Workaround**: Use direct gRPC calls instead:
```bash
grpcurl -insecure -cacert certs/ca/ca.pem \
  -cert certs/api-gateway/cert.pem \
  -key certs/api-gateway/key.pem \
  -d '{"name":"test","email":"test@test.com"}' \
  localhost:50051 user.v1.UserService/CreateUser
```
**Status**: Known issue. Proto descriptor needs rebuild with correct `--as-file-descriptor-set` and include imports.

### Database Health Check Errors ("closed pool")
**Problem**: Logs show `database health check failed: closed pool`.
**Root cause**: Vault credential refresh creates a new pool and closes the old one. The health check goroutine references the old pool.
**Impact**: Low — the new pool works correctly; only the health check is stale.
**Workaround**: Restart the service to clear the warning.
**Status**: Pre-existing issue. The health monitor goroutine should reference the repository's active pool.

### Order Creation Fails with FK Violation
**Problem**: CreateOrder returns `Internal: failed to create order`.
**Root cause**: The order-db's `orders` table had a foreign key constraint `user_id → users(id)`, but users are stored in user-db, not order-db. The constraint was removed via migration `000004_drop_order_user_fk`.
**Fix**: Ensure the migration has been applied:
```bash
docker compose exec order-db psql -U postgres -d orders \
  -c "SELECT filename FROM schema_migrations ORDER BY filename;"
```
Look for `000004_drop_order_user_fk.up.sql` in the list.

### Vault Init Failures
**Problem**: vault-init container exits with error.
**Root cause**: Vault not healthy, or script times out.
**Troubleshooting**:
```bash
# Check vault is healthy
docker compose exec vault vault status

# Check vault-init logs
docker compose logs vault-init

# Re-run init if needed
docker compose up -d vault-init
```

### Container Won't Start
```bash
# Check container logs
docker compose logs <service-name>

# Check if port is already in use
lsof -i :PORT

# Rebuild from scratch
docker compose build --no-cache <service-name>
docker compose up -d <service-name>
```

## Disaster Recovery

### Full Reset
```bash
# Stop everything and delete all data
make down

# Start fresh
make up
```

### Preserve Data
```bash
# Stop without deleting volumes
make stop

# Restart later
make up   # (up also generates certs and builds)
```

### Individual Service Recovery
```bash
# Restart a single service
docker compose restart <service-name>

# Check logs after restart
docker compose logs --tail=50 <service-name>
```
