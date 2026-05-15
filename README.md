# gRPC Microservices PoC

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](https://go.dev)
[![gRPC](https://img.shields.io/badge/gRPC-HTTP/2-244c5a)](https://grpc.io)
[![Envoy](https://img.shields.io/badge/Envoy-1.28-2c3e50?logo=envoy-proxy)](https://www.envoyproxy.io)
[![Vault](https://img.shields.io/badge/Vault-1.15-FFCD00?logo=vault)](https://www.vaultproject.io)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791?logo=postgresql)](https://www.postgresql.org)
[![Redis](https://img.shields.io/badge/Redis-7-DC382D?logo=redis)](https://redis.io)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)

> Production-pattern microservices with Go, gRPC, Envoy, and HashiCorp Vault

---

## Architecture Overview

A Go-based microservices proof-of-concept demonstrating production patterns: mutual TLS (mTLS), Vault dynamic database secrets, Envoy API gateway with ext-authz, OpenTelemetry observability, Redis caching, and PostgreSQL persistence. Two services communicate via gRPC with cross-service calls and are deployed with Docker Compose.

**user-service** provides user CRUD operations backed by PostgreSQL. **order-service** provides order CRUD with Redis cache-aside and calls `user-service.GetUser()` via gRPC to verify users before creating orders. Every service-to-service call uses mTLS with short-lived certificates issued by an internal CA. Database credentials are dynamically leased from Vault and automatically refreshed.

## Architecture Diagram

```
┌──────────────┐    HTTP :8080    ┌──────────────────┐    gRPC mTLS    ┌──────────────┐
│   Clients    │ ──────────────── │  Envoy Gateway   │ ────────────── │   Services   │
│ curl/grpcurl │                  │ • ext-authz      │                │ • user:50051 │
│ k6 / apps    │                  │ • rate limit     │ ────────────┐  │ • order:50052│
└──────────────┘                  │ • circuit brk    │             │  └──────┬───────┘
                                  │ • JSON↔gRPC      │             │         │
                                  │ • mTLS upstream  │             │  ┌──────▼───────┐
                                  └──────────────────┘             │  │  order-svc   │
                                                                   └──│  GetUser()   │
                                                                      └──────────────┘
┌──────────────────────────────────────────────────────────────────────────────────┐
│  Infrastructure                                                                  │
│  Vault (dynamic DB creds) │ Prometheus :9090 │ Grafana :3000 │ Jaeger :16686    │
└──────────────────────────────────────────────────────────────────────────────────┘
```

## Quick Start

```bash
# Prerequisites: Docker Desktop, Go 1.21+, buf
make up
```

This generates TLS certificates, builds service binaries, provisions Vault with dynamic database roles, runs database migrations, and starts the full stack. Services report healthy before the gateway accepts traffic.

| Service | Address |
|---------|---------|
| Envoy HTTP gateway | http://localhost:8080 |
| Grafana dashboards | http://localhost:3000 (admin:admin) |
| Prometheus | http://localhost:9090 |
| Jaeger UI | http://localhost:16686 |

## Test Results Summary

- **17,708** requests across load tests with **0** errors
- **42** integration tests passing (user CRUD, order CRUD, auth, mTLS, resilience)

## Technology Stack

| Component | Technology | Purpose |
|-----------|-----------|---------|
| Language | Go 1.25 | gRPC servers, proto-generated stubs |
| Transport | gRPC (HTTP/2) with mTLS | Inter-service communication, TLS 1.3 |
| API Gateway | Envoy v1.28 | Auth, rate limiting, circuit breaker, transcoding |
| Auth | ext-authz (Go) | API key validation at gateway |
| Secrets | HashiCorp Vault 1.15 | Dynamic PostgreSQL credentials |
| Database | PostgreSQL 16 (×2) | Per-service database isolation |
| Cache | Redis 7 | Cache-aside for order lookups |
| Metrics | Prometheus + Grafana | RED method dashboards |
| Tracing | OpenTelemetry → Jaeger | Distributed trace propagation |
| Load Testing | Go gRPC tester | Concurrent VUs with p50/p95/p99 reporting |

## Security Features

- **mTLS** with custom CA, ECDSA P-256 keys, 10-year CA / 1-year service certs
- **Vault dynamic database credentials** with auto-refresh at 50% lease interval
- **API key authentication** on Envoy gateway via ext-authz
- **Container security**: distroless base images, read-only rootfs, no-new-privileges
- **Network segmentation**: frontend/backend Docker networks isolate gateway from DB
- **Input validation** at gRPC layer with proper status codes

## Project Structure

```
├── proto/           # Protocol buffer definitions
├── services/        # gRPC server implementations
├── internal/        # Shared: TLS, logging, shutdown, Vault, OTel
├── db/migrations/   # Embedded SQL migrations
├── envoy/           # Envoy config + ext-authz
├── monitoring/      # Prometheus + Grafana configs
├── tests/           # Integration tests (pytest)
└── scripts/         # Load tester, verify, resilience tests
```

## Makefile Targets

| Target | Description |
|--------|-------------|
| `make up` | Generate certs, build, and start full stack |
| `make down` | Stop and clean all volumes |
| `make verify` | Static verification (lint, vet, build) |
| `make test-integration` | Run integration tests (requires stack) |
| `make load-test` | Run gRPC load test (25 VUs, 30s) |
| `make load-test-chaos` | Kill user-service during load, verify recovery |

## Known Limitations

- Envoy HTTP-to-gRPC transcoding returns 404 (proto descriptor needs rebuild; use direct gRPC)
- Cross-service `CreateOrder` is the bottleneck (~160ms avg due to user verification + DB transaction)
- No CI/CD pipeline (planned for Phase 2 with K8s)
- No unit tests with mocked DB (only integration tests against real services)
