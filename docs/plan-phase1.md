# gRPC Microservices PoC - Phase 1 Implementation Plan

## Agent Workstation Setup

### Prerequisites
- [ ] Docker Desktop (with Compose v2)
- [x] Go 1.21+ (v1.22.3 installed)
- [x] protoc compiler (v29.3) + language plugins (protoc-gen-go, protoc-gen-go-grpc)
- [x] buf CLI (v1.69.0)
- [x] cfssl (v1.6.5 installed)
- [x] k6 (v2.0.0, load testing)
- [x] git

---

## Project Structure

```
grpc-microservices-poc/
├── README.md
├── go.mod                          # Root Go module
├── .gitignore
├── Makefile                        # (pending)
├── buf.yaml                        # Buf module: lint + breaking rules, deps
├── buf.gen.yaml                    # Code generation: Go + gRPC, managed mode
├── buf.lock                        # Resolved dependency versions
├── proto/
│   ├── common/v1/common.proto      # Pagination, shared types
│   ├── user/v1/user.proto          # User CRUD + HTTP + FieldMask
│   └── order/v1/order.proto        # Order CRUD+L + OrderItem + Pagination + HTTP
├── services/
│   ├── common/v1/common.pb.go      # Generated Go types
│   ├── user/v1/                    # Generated User service stubs
│   │   ├── user.pb.go
│   │   └── user_grpc.pb.go
│   ├── order/v1/                   # Generated Order service stubs
│   │   ├── order.pb.go
│   │   └── order_grpc.pb.go
│   ├── user-service/               # gRPC server, pgx DB, mTLS, health, zap logging
│   │   ├── config.go
│   │   ├── repository.go
│   │   ├── server.go
│   │   └── main.go
│   └── order-service/              # gRPC server, pgx DB, Redis cache, mTLS, user client
│       ├── config.go
│       ├── repository.go
│       ├── cache.go
│       ├── client.go
│       ├── server.go
│       └── main.go
├── Dockerfile.user-service            # Multi-stage distroless build
├── Dockerfile.order-service           # Multi-stage distroless build
├── Dockerfile.vault-init              # Init container for Vault setup
├── docker-compose.yml                   # Multi-service orchestration
├── internal/
│   ├── tls/tls.go                     # mTLS credential loading
│   ├── log/log.go                     # Zap logger init
│   ├── shutdown/shutdown.go           # Graceful shutdown handler
│   └── vault/client.go                # HashiCorp Vault client (dynamic DB creds, static secrets)
├── db/
│   └── migrations/
│       ├── embed.go                   # Embedded SQL runner
│       ├── 000001_create_users.{up,down}.sql
│       ├── 000002_create_orders.{up,down}.sql
│       ├── 000003_create_order_items.{up,down}.sql
│       └── 000004_drop_order_user_fk.{up,down}.sql
├── envoy/
│   ├── envoy.yaml                       # Full Envoy configuration
│   ├── proto-descriptor.pb              # Proto descriptor for transcoding
│   ├── Dockerfile                       # Envoy v1.28-latest image
│   └── ext_authz/
│       ├── main.go                      # API key validation service
│       └── Dockerfile                   # Multi-stage distroless build
├── certs/                          # (generated)
├── vault/
│   ├── config.hcl                     # Production Vault config
│   ├── policies/
│   │   ├── user-service.hcl           # Vault policy for user-service
│   │   └── order-service.hcl          # Vault policy for order-service
│   └── tokens/                        # (generated)
├── tests/                          # Integration tests (pytest + grpcurl)
│   ├── conftest.py
│   ├── requirements.txt
│   ├── test_user_service.py
│   ├── test_order_service.py
│   ├── test_errors.py
│   ├── test_auth.py
│   ├── test_mtls.py
│   ├── test_resilience.py
│   └── .venv/                      # Python virtual environment
├── monitoring/                     # Prometheus, Grafana configs + dashboards
├── scripts/
│   ├── init-vault.sh                  # Vault initialization automation
│   ├── verify.sh                      # Static + runtime verification
│   ├── test-resilience.sh             # Resilience test suite
│   ├── wait-for-health.sh             # Health check waiter
│   ├── load-test-chaos.sh             # Chaos load test
│   └── loadtest/
│       └── main.go                    # Go gRPC load tester
└── docs/
    ├── plan-phase1.md
    ├── plan-phase2.md
    └── proto-design.md             # Proto architecture decisions
```

---

## Implementation Phases

### Week 1: Foundation

#### Day 1-2: Repository & Proto Definitions

Status: **Completed**

| Tooling | Version |
|---------|---------|
| Go | 1.22.3 |
| protoc | 29.3 |
| protoc-gen-go | latest |
| protoc-gen-go-grpc | installed |
| buf | 1.69.0 |

- [x] Initialize git repo with .gitignore
- [x] Create proto definitions with best practices:
  - **User service** (`proto/user/v1/user.proto`): CreateUser, GetUser, UpdateUser (with `FieldMask`), DeleteUser
  - **Order service** (`proto/order/v1/order.proto`): CreateOrder, GetOrder, ListOrders (with pagination), CancelOrder
  - **Common types** (`proto/common/v1/common.proto`): Pagination, PaginationResponse
  - **HTTP annotations** (`google.api.http`) on all RPCs for Envoy transcoding
  - **OrderItem** value object with product_id, product_name, quantity, unit_price
  - **OrderStatus** enum (UNSPECIFIED, PENDING, CONFIRMED, SHIPPED, DELIVERED, CANCELLED)
  - **FieldMask** support for partial updates on User
  - **CancelOrder** replaces DeleteOrder (DDD: orders are not deleted)
  - Named response types for all RPCs (no bare `google.protobuf.Empty`)
  - Tracing metadata propagated via gRPC headers (documented in design doc)
- [x] Setup buf configuration:
  - `buf.yaml`: lint (STANDARD + COMMENTS), breaking change detection (FILE), deps (googleapis, wellknowntypes)
  - `buf.gen.yaml`: managed mode for `go_package_prefix`, local plugins
  - `buf lint` passes clean
- [x] Generate Go code:
  - `services/user/v1/user.pb.go` + `user_grpc.pb.go`
  - `services/order/v1/order.pb.go` + `order_grpc.pb.go`
  - `services/common/v1/common.pb.go`
  - `go vet ./services/...` and `go build ./...` pass clean
- [x] Root Go module (`github.com/tiagoricardo/grpc-microservices`)
- [x] Document proto design decisions in `docs/proto-design.md`

#### Day 3: Certificate Infrastructure

Status: **Completed**

- [x] Install cfssl (v1.6.5 via Homebrew)
- [x] Create CA configuration (`certs/ca-csr.json`, `certs/ca-config.json`):
  - ECDSA P-256 keys (smaller certs, better perf than RSA)
  - 10-year CA validity, 1-year service certs
  - Three signing profiles: `server`, `client`, `peer` (mutual TLS)
- [x] Generate service certificates:
  - `certs/user-service/` — SANs: localhost, user-service, 127.0.0.1
  - `certs/order-service/` — SANs: localhost, order-service, 127.0.0.1
  - `certs/api-gateway/` — SANs: localhost, api-gateway, 127.0.0.1
  - All signed with `peer` profile (both `serverAuth` + `clientAuth` EKU)
- [x] Create `certs/generate-certs.sh` script (fully automated, idempotent)
- [x] Add cert rotation docs in `certs/cert-rotation.md`:
  - Planned rotation procedure (11-month cycle)
  - Emergency rotation (compromised key)
  - Expiry monitoring commands
  - Docker secrets deployment reference
  - Troubleshooting table
- [x] Verify mTLS locally with OpenSSL:
  - `openssl verify -CAfile ca.pem <service>/cert.pem` — chain valid for all 3 services
  - `openssl s_server` + `openssl s_client` — mTLS handshake verified
- [x] Set restrictive permissions (`chmod 600` on private keys)
- [x] `.gitignore` configured to exclude `*.pem` and `*.csr`

#### Day 4-5: Base Service Scaffolding

Status: **Completed**

- [x] **Shared internal packages** (`internal/`):
  - `internal/tls` — `LoadServerConfig()`, `LoadClientConfig()` — mTLS credentials from certs
  - `internal/log` — `New(level)` — zap structured logger (JSON output)
  - `internal/shutdown` — `Graceful()` — SIGTERM/SIGINT handler with ordered callbacks
- [x] **Database migrations** (`db/migrations/`):
  - Users table (UUID PK, name, email UNIQUE, timestamps)
  - Orders table (UUID PK, user_id FK, status, total, timestamps)
  - Order items table (UUID PK, order_id FK, product_id, quantity, unit_price)
  - `schema_migrations` table for tracking applied migrations
  - `migrations.Up()` runner with embedded SQL files
- [x] **User Service** (`services/user-service/`):
  - `config.go` — env-based config (PORT, DATABASE_URL, TLS cert paths)
  - `repository.go` — pgx CRUD with dynamic FieldMask update support
  - `server.go` — gRPC handlers with input validation + gRPC status codes
  - `main.go` — wiring: DB pool → migrations → mTLS → gRPC server → health → reflection → graceful shutdown
  - `loggingInterceptor` — zap middleware logging method, duration, status code
- [x] **Order Service** (`services/order-service/`):
  - `config.go` — env-based config (adds REDIS_URL, USER_SERVICE_ADDR)
  - `client.go` — gRPC client to user-service:50051 with mTLS
  - `repository.go` — pgx transactional CreateOrder, paginated ListOrders, CancelOrder
  - `cache.go` — Redis cache-aside for GetOrder (5min TTL, protojson serialization)
  - `server.go` — gRPC handlers with User validation before CreateOrder
  - `main.go` — wiring: DB → Redis → UserClient → mTLS → gRPC server → health → reflection → shutdown
- [x] **Docker images** (multi-stage builds):
  - `Dockerfile.user-service` — `golang:1.22-alpine` → `distroless/static-debian12:nonroot`
  - `Dockerfile.order-service` — same pattern, `USER 1000:1000`
- [x] **Verification**: `go vet ./...` and `go build ./...` pass clean

### Week 2: Integration & Security

#### Day 6-7: API Gateway with Envoy

Status: **Completed**

- [x] Envoy config (`envoy/envoy.yaml`) with full architecture:
  - **Listener** on port 8080 (HTTP), admin on port 9901
  - **Filter chain order**: ext_authz → local_ratelimit → grpc_json_transcoder → router
- [x] Route definitions per prefix:
  - `/api/v1/users*` → user-service:50051
  - `/api/v1/orders*` → order-service:50052
- [x] External auth filter (envoy.ext_authz):
  - HTTP ext_authz service on ext-authz:50053
  - Validates `x-api-key` header against env-configured key (default: `grpc-poc-api-key-2026`)
  - Implemented in `envoy/ext_authz/main.go` — lightweight Go HTTP server, no extra deps
  - Multi-stage Dockerfile (`envoy/ext_authz/Dockerfile`)
- [x] Circuit breaker config per cluster:
  - `max_connections: 100`, `max_pending_requests: 10`, `max_requests: 50`, `max_retries: 5`
- [x] Retry policies per route:
  - 3 retries on `unavailable,reset,resource_exhausted,connect-failure`
  - Exponential backoff: base 250ms, max 2s
- [x] Rate limiting (local token bucket):
  - 100 tokens max, refill 10 per 60s
  - Returns `x-rate-limited` header when throttled
- [x] TLS termination (upstream):
  - mTLS to both services using `certs/api-gateway/` client cert + `certs/ca/ca.pem` verification
  - HTTP/2 protocol for gRPC upstreams
- [x] gRPC-JSON transcoding:
  - Proto descriptor generated via `buf build -o envoy/proto-descriptor.pb` (115KB)
  - Services: `user.v1.UserService`, `order.v1.OrderService`
  - Converts gRPC status codes to HTTP status codes

#### Day 8: Vault Integration

Status: **Completed**

- [x] Vault configuration (`vault/config.hcl`) — Raft storage, TCP listener, UI enabled
- [x] Secret policies per service:
  - `vault/policies/user-service.hcl` — read `database/creds/user-service`, read/list `secret/user-service/*`
  - `vault/policies/order-service.hcl` — read `database/creds/order-service`, read/list `secret/order-service/*`
- [x] Dynamic database credentials via `database/` secret engine (PostgreSQL plugin)
- [x] Static secrets via KV v1 at `secret/` (fallback DB URLs, API key)
- [x] Init script (`scripts/init-vault.sh`) — 8-step automation: wait → login → enable DB engine → configure PG connections → create roles → store static secrets → write policies → create service tokens
- [x] Go vault client (`internal/vault/client.go`):
  - `New(address, token)` — constructor
  - `GetDBCreds(ctx, role)` → `DBCreds{Username, Password, LeaseID, LeaseDuration}`
  - `GetSecret(ctx, path)` → `map[string]interface{}`
  - `RenewLease(ctx, leaseID, increment)`
- [x] User service Vault integration:
  - `config.go` — added `VaultAddr`, `VaultToken`, `VaultDBRole`, `UseVault()` guard
  - `main.go` — fetches dynamic DB creds before pool creation when `VAULT_ADDR` + `VAULT_TOKEN` set
- [x] Order service Vault integration — same pattern, role defaults to `order-service`
- [x] Dependency: `github.com/hashicorp/vault/api v1.15.0`
- [x] Verification: `go vet ./...` and `go build ./...` pass clean

#### Day 9: Docker Compose Assembly

Status: **Completed**

- [x] `docker-compose.yml` with all 8 services:
  - **vault** (hashicorp/vault:1.15, dev mode, IPC_LOCK, health check)
  - **vault-init** (init container, configures DB engine, policies, service tokens)
  - **user-db** (postgres:16-alpine, health check, named volume)
  - **order-db** (postgres:16-alpine, health check, named volume)
  - **redis** (redis:7-alpine, health check)
  - **user-service** (local build, `depends_on: vault-init:service_completed_successfully`, `read_only`, `no-new-privileges`)
  - **order-service** (local build, same security constraints)
  - **ext-authz** (local build, `read_only`, `no-new-privileges`)
  - **envoy** (local build, `read_only`, `no-new-privileges`)
- [x] Health checks for all infrastructure services (vault, postgres, redis)
- [x] `depends_on` with conditions (`service_healthy`, `service_completed_successfully`)
- [x] Secrets management via shared volume (`vault-tokens` for service tokens, bind mount for certs)
- [x] Network segmentation: `frontend` (envoy, ext-authz) and `backend` (all services)
- [x] Container security: `read_only: true`, `tmpfs: [/tmp]`, `no-new-privileges`, `user: "1000:1000"`
- [x] `Dockerfile.vault-init` (hashicorp/vault:1.15 + jq, copies init script and policies)
- [x] `VAULT_TOKEN_FILE` support in both services (fallback when `VAULT_TOKEN` not set)
- [x] Verification: `go vet ./...` and `go build ./...` pass clean

#### Day 10: Resilience Implementation

Status: **Completed**

- [x] Connection pool tuning
  - `config.go` both services: env-configurable `MaxConns` (25), `MinConns` (5), `MaxConnLifetime` (30m), `MaxConnIdleTime` (5m), `HealthCheckPeriod` (30s)
  - Uses `pgxpool.ParseConfig` + `NewWithConfig` for full pool configuration
- [x] Query timeouts
  - `repository.go` — every DB call wrapped with `context.WithTimeout(ctx, 10s)`
- [x] Panic recovery interceptor
  - `main.go` — custom `recoveryInterceptor` via `ChainUnaryInterceptor`, logs panic + returns `codes.Internal`
- [x] Health check propagation
  - `monitorDBHealth` goroutine pings pool every 15s, updates gRPC health to `NOT_SERVING` on DB failure
- [x] Graceful shutdown ordering
  - `internal/shutdown/shutdown.go`: callbacks run **sequentially** (removed goroutines) — gRPC drains before DB pool closes
- [x] Container restart policies
  - `docker-compose.yml`: `restart: unless-stopped` on user-service, order-service, ext-authz, envoy
- [x] Vault credential auto-refresh
  - `internal/vault/client.go`: `StartCredentialRefresher` goroutine re-fetches creds at 50% lease interval
  - `main.go`: callback creates new `pgxpool.Pool`, swaps via repository's `SetPool()`, closes old pool
- [x] Resilience test suite
  - `scripts/test-resilience.sh`: tests restart policy, graceful compose restart, DB health monitoring, Envoy circuit breaker, pool config logging
  - `make test-resilience` target added

### Week 3: Observability & Testing

#### Day 11-12: Monitoring Stack

Status: **Completed**

- [x] **OpenTelemetry SDK** (`internal/otel/otel.go`):
  - OTLP trace exporter → Jaeger (gRPC, port 4317)
  - Prometheus metric exporter (bridged via `promhttp`)
  - Context propagation (TraceContext + Baggage)
  - `otelgrpc` stats handler for auto-instrumentation on all RPCs
- [x] **Prometheus**
  - Service discovery (static configs with labels)
  - gRPC metrics (`rpc_server_call_duration_seconds_count`, _bucket, _sum)
  - Recording rules (`grpc:requests:rate5m`, `grpc:errors:rate5m`, `grpc:error_ratio:rate5m`, `grpc:latency:p95`, `grpc:latency:p99`)
  - OTLP receiver enabled for direct metric ingestion
- [x] **Grafana**
  - Datasource config (Prometheus, uid: `prometheus`, default)
  - Provisioned dashboard: RED method (Rate, Errors, Duration per RPC)
  - Provisioned dashboard: Service Resources (CPU, memory placeholders)
- [x] **Jaeger**
  - All-in-one container (docker-compose, OTLP ingest on 4317, UI on 16686)
  - OpenTelemetry instrumentation in both services
  - Traces captured for all endpoints (CreateUser, GetUser, CreateOrder, etc.)

#### Day 13: Integration Tests

Status: **Completed** — 42 tests, 41 passed, 1 known limitation

Test infrastructure (`tests/`):
```
tests/
├── conftest.py              # Fixtures: grpc_call helper, unique_email, user_id/order_id
├── requirements.txt         # pytest, requests
├── test_user_service.py     # 17 tests: full CRUD lifecycle + error paths
├── test_order_service.py    # 18 tests: full CRUD lifecycle + pagination + cross-service
├── test_errors.py           # 6 tests: edge cases, special chars, clamping
├── test_auth.py             # 4 tests: missing/invalid/valid API key via Envoy
├── test_mtls.py             # 3 tests: valid cert, service cert, missing cert
└── test_resilience.py       # 1 test: DB stop → Internal errors → recovery
```

- [x] **Happy path scenarios** — Full CRUD lifecycle for both services: Create→Get→Update→Delete (user), Create→Get→List→Cancel (order)
- [x] **Error handling** — InvalidArgument for all validation rules, NotFound for missing resources, AlreadyExists for duplicate email
- [x] **Auth failures** — Missing API key (401), invalid key (401), valid key (authorized) via Envoy HTTP gateway
- [x] **mTLS failures** — Valid client cert succeeds, missing cert rejected, service cert as client tested
- [x] **Database connection failures** — Stop user-db → calls return Internal → restart → recovery verified
- [x] **Makefile targets** — `make test-integration`, `make test-integration-auth`, `make test-integration-mtls`, `make test-integration-resilience`

**Known limitation:** ListOrders pagination response omits `hasMore` when false (protobuf zero-value JSON omission). Tests check `totalCount` instead.

#### Day 14: Load Testing

Status: **Completed** — 17,708 requests across 2 scenarios, 0 errors

| Scenario | VUs | Duration | RPS | p95 | Errors |
|----------|-----|----------|-----|-----|--------|
| Moderate | 25 | 30s | 14.0 | 185ms | 0% |
| Heavy | 50 | 20s | 7.2 | 361ms | 0% |

- [x] **Go gRPC load tester** (`scripts/loadtest/main.go`) — concurrent VUs, configurable mix, mTLS support, p50/p95/p99 reporting
- [x] **Load test report** (`docs/load-test-report.md`) — methodology, results, observations, recommendations
- [x] **Chaos scenarios** (`scripts/load-test-chaos.sh`) — kill user-service during load, verify circuit breaker, recovery
- [x] **Envoy rate limiter relaxed** — for meaningful throughput during testing
- [x] **k6 upgraded** to v2.0.0 (was v0.39.0) — gRPC module verified but mTLS compatibility issues required Go-based tester
- [x] **Makefile targets** — `make load-test`, `make load-test-heavy`, `make load-test-chaos`

**Key findings:**
- **CreateOrder is the bottleneck** (cross-service gRPC + DB transaction) at 163ms avg / 291ms p95
- **Zero errors** across all 17,708 requests — all circuit breakers/pools handled correctly
- **p95 exceeds 100ms target** at >25 VUs — production would need larger connection pools
- **Reads scale better than writes** — GetUser throughput drops 55% when VUs double vs CreateOrder at 46%

#### Day 15: Documentation & Polish
- [ ] **README.md**
  - Quick start (make up)
  - Architecture diagram (ASCII or link to PNG)
  - Technology rationale
  - Security features
  - Known limitations
- [ ] **Makefile**
  ```makefile
  .PHONY: up down logs test load-test certs
  up: certs
      docker-compose up -d
  
  test:
      docker-compose exec user-service go test ./...
  
  load-test:
      k6 run scripts/load-test.js
  ```
- [ ] **docs/security.md**
  - Threat model
  - mTLS setup
  - Secrets management
  - Security scan results (trivy)
- [ ] **docs/runbook.md**
  - Common operations
  - Troubleshooting
  - Disaster recovery

---

## Security Checklist

### Container Security
- [ ] Non-root users in all Dockerfiles
- [ ] Read-only root filesystem
- [ ] No secrets in images or env vars
- [ ] Minimal base images (distroless/alpine)
- [ ] Image scanning in CI (trivy/grype)
- [ ] HEALTHCHECK in Dockerfiles

### Network Security
- [ ] mTLS between all gRPC services
- [ ] Network segmentation (separate Docker networks)
- [ ] No unnecessary port exposure
- [ ] API key auth on gateway
- [ ] Rate limiting configured

### Secrets Management
- [ ] No secrets in git (use .gitignore for certs/)
- [ ] Vault for dynamic credentials
- [ ] Docker secrets for static values
- [ ] Cert expiry monitoring

### Application Security
- [ ] Input validation on all endpoints
- [ ] SQL injection prevention (prepared statements)
- [ ] Proper error handling (don't leak internals)
- [ ] Request size limits
- [ ] Timeout configurations

---

## Testing Strategy

### Unit Tests
- [ ] Proto validation
- [ ] Business logic coverage > 80%
- [ ] Mock gRPC clients for dependencies

### Integration Tests
- [ ] End-to-end API flows
- [ ] Database interactions
- [ ] Service-to-service communication
- [ ] Error propagation

### Load Tests
- [ ] Baseline performance metrics
- [ ] Chaos scenarios (kill pods)
- [ ] Resource limits testing
- [ ] Latency under load (p50, p95, p99)

### Security Tests
- [ ] mTLS validation (reject invalid certs)
- [ ] Auth bypass attempts
- [ ] SQL injection attempts
- [ ] Container escape attempts

---

## Monitoring & Alerting

### Key Metrics
- [ ] Request rate (per service, per endpoint)
- [ ] Error rate (4xx, 5xx)
- [ ] Latency (p50, p95, p99)
- [ ] Circuit breaker state
- [ ] Connection pool utilization
- [ ] Database query performance

### Alerts (for Phase 2)
- Error rate > 5% for 5min
- p95 latency > 200ms
- Circuit breaker open
- Certificate expiry < 7 days

---

## Deliverables Checklist

- [ ] GitHub repo with clear README
- [ ] `make up` works on fresh machine
- [ ] Architecture diagram (docs/architecture.png)
- [ ] All services running with `docker-compose ps`
- [x] Grafana dashboards accessible at localhost:3000 (admin:admin)
- [x] Load test results in docs/load-test-report.md
- [ ] Security scan report (trivy)
- [ ] Demo video or GIF in README
- [ ] License file (MIT/Apache)
- [ ] CONTRIBUTING.md for phase 2 ideas

---

## Interview Talking Points

### Technical Depth
- "Implemented mTLS with custom CA for zero-trust architecture"
- "Used Vault dynamic secrets for database credential rotation"
- "Circuit breakers via Envoy reduced cascading failures by..."
- "Achieved p95 latency of Xms under Y RPS load"

### Trade-offs
- "Chose Envoy over custom gateway for battle-tested resilience patterns"
- "Selected [language] for [reason], considered alternatives"
- "Phase 1 uses docker-compose for local dev speed, Phase 2 moves to K8s for prod patterns"

### Production Readiness
- "Added graceful shutdown to prevent request loss during deploys"
- "Implemented distributed tracing for debugging microservice issues"
- "Rate limiting protects against abuse and cascading failures"

---

## Next Steps (Phase 2 Preview)

- [ ] Convert compose services to Helm charts
- [ ] Add Istio for advanced traffic management
- [ ] Implement HPA based on custom metrics
- [ ] Add chaos engineering (Litmus/Chaos Mesh)
- [ ] GitOps with ArgoCD
- [ ] Policy enforcement (OPA/Kyverno)
- [ ] Service mesh observability upgrades

---

## Daily Standup Template

**Yesterday:**
- Completed: [tasks]
- Blocked: [issues]

**Today:**
- Focus: [current phase tasks]
- Risk: [potential blockers]

**Metrics:**
- Services running: X/Y
- Tests passing: X/Y
- Coverage: X%