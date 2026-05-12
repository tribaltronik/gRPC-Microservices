# gRPC Microservices PoC

A proof-of-concept microservices architecture demonstrating gRPC communication with Go, secured with mTLS, fronted by Envoy proxy, and instrumented with OpenTelemetry.

## Architecture

- **API Gateway**: Envoy proxy (HTTP/1.1 → gRPC transcoding)
- **User Service**: gRPC server with PostgreSQL
- **Order Service**: gRPC server with PostgreSQL + Redis
- **Observability**: Prometheus + Grafana + Jaeger
- **Secrets**: HashiCorp Vault

## Quick Start

### Prerequisites

- Go 1.21+
- Docker Desktop (with Compose v2)
- protoc + Buf CLI
- cfssl
- k6 (optional, for load testing)

### Generate Proto Code

```bash
buf mod update
buf lint
buf generate
```

Generated Go code appears under `services/*/pb/`.

### Project Structure

```
├── proto/                    # Protobuf definitions
│   ├── common/v1/            # Shared types (Pagination)
│   ├── user/v1/              # User service
│   └── order/v1/             # Order service
├── services/
│   ├── user-service/         # User service implementation
│   └── order-service/        # Order service implementation
├── docs/
│   └── proto-design.md       # Proto design decisions
└── buf.yaml                  # Buf module configuration
```

## Proto Design

See [docs/proto-design.md](docs/proto-design.md) for detailed design decisions including field numbering, data types, pagination, and HTTP transcoding conventions.
