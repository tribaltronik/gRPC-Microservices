# Proto Design Decisions

## Overview

This document records the architectural decisions made for the gRPC service definitions in Phase 1 of the gRPC Microservices PoC.

## Language & Tooling

- **Interface Definition Language**: Proto3 — chosen for its lightweight syntax, wide ecosystem support, and native gRPC support.
- **Code Generation**: [Buf](https://buf.build) with managed mode — automates `go_package` options, enforces lint rules, and detects breaking changes.
- **Go Plugins**: `protoc-gen-go` for message types, `protoc-gen-go-grpc` for service stubs.

## Proto Package Structure

```
proto/
├── common/v1/       # Cross-cutting types (Pagination, shared enums)
├── user/v1/         # User service definitions
└── order/v1/        # Order service definitions
```

### Rationale
- Domain-based packages align with bounded contexts (DDD)
- Version suffix (`v1`) enables API evolution with breaking changes behind a new version
- Shared `common` package avoids circular dependencies between domain packages

## Field Numbering Strategy

| Number Range | Usage | Notes |
|---|---|---|
| 1-15 | Frequently occurring fields (id, name) | 1 byte on the wire |
| 16-2047 | Less frequent fields (timestamps, metadata) | 2+ bytes |

Following Google's recommendation to reserve 1-15 for fields that appear in every message instance.

## Data Type Decisions

| Concept | Chosen Type | Alternatives Considered | Rationale |
|---|---|---|---|
| UUID | `string` (RFC 4122) | `bytes` (16 bytes) | Human-readable in logs, no binary encoding needed for HTTP transcoding |
| Timestamp | `google.protobuf.Timestamp` | Unix `int64` | Timezone-aware, standard formatting, built-in comparison |
| Money | `double` | `google.type.Money` | Acceptable for PoC; document that floating-point precision is a known limitation |
| Enum sentinel | `ENUM_NAME_UNSPECIFIED = 0` | First value as valid state | Proto3 default is 0, prevents accidental valid state |
| Empty response | `google.protobuf.Empty` | Custom response message | Standard, no ambiguity (e.g., DeleteUser, CancelOrder) |

## Enum Convention

All enums follow Google's style:
- `UPPER_SNAKE_CASE`
- First value is `ENUM_NAME_UNSPECIFIED = 0` (proto3 zero-value default)
- Use `ENUM_NAME_UNRECOGNIZED = -1` only when needed

## Partial Updates (Field Masks)

`UpdateUser` uses `google.protobuf.FieldMask` to specify which fields to update. This follows the Google API design pattern:
- Client sends only modified fields in the `user` message
- `update_mask` specifies the field paths to apply
- Unlisted fields in the mask are ignored, preventing accidental overwrites

## Pagination

`ListOrders` uses a cursor-agnostic page-based pagination via `common.v1.Pagination`:
- `page` is 1-indexed (page 1 returns the first page)
- `page_size` defaults to 20, max 100
- Response includes `total_count` and `has_more` for UI rendering

## HTTP/gRPC Transcoding

All RPCs include `google.api.http` annotations for Envoy's gRPC-JSON transcoder:
- Envoy reads these at runtime from proto descriptors — no code generation needed
- Enables REST clients to call gRPC services through the API Gateway
- HTTP methods: POST for creates, GET for reads, PATCH for partial updates, DELETE for deletes

## Tracing Context

Trace and span IDs are propagated via **gRPC metadata headers** (not protobuf message fields):
- Headers: `x-trace-id`, `x-span-id`
- Propagated through interceptors/middleware at each service boundary
- Decouples tracing from business logic — no proto changes needed for tracing

## Domain Relationships

```
User (aggregate root) 1──N Order (aggregate root)
                            │
                            └── OrderItem (value object, owned by Order)
```

- User references Order via `user_id` string (not a nested message) — loose coupling between aggregates
- OrderItem is a value object embedded in Order — no separate identity, no standalone repository
- Order references User by ID only — follows DDD aggregate pattern where aggregates reference each other by identity

## Breaking Change Policy

Breaking changes are detected by buf (`FILE` rule set):
- **Allowed**: Adding new fields, new RPCs, new services, new packages
- **Forbidden**: Renaming fields, changing field types, removing fields, changing RPC signatures
- **Exception**: UNSPECIFIED enum values can be renamed (same numeric value)

## Future Considerations

- **Protovalidate**: Add `buf build/protovalidate` rules for input validation when services are implemented
- **google.type.Money**: Replace `double` for monetary values before production use
- **Cursor-based pagination**: Consider for high-volume list operations in Phase 2
- **gRPC health protocol**: Standard `grpc.health.v1` will be added at service scaffolding time

## References

- [Google API Design Guide](https://cloud.google.com/apis/design)
- [Buf Best Practices](https://buf.build/docs/best-practices)
- [Protobuf Style Guide](https://protobuf.dev/programming-guides/style/)
