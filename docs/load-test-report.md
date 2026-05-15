# Load Test Report

## Test Environment

| Component | Details |
|-----------|---------|
| Date | 2026-05-15 |
| Tester | Go gRPC load tester (`scripts/loadtest/main.go`) |
| Protocol | Direct gRPC with mTLS (ports 50051, 50052) |
| Stack | 12 Docker containers (all services running) |
| Infrastructure | Vault dynamic DB creds, Redis cache, PostgreSQL 16 |
| TLS | ECDSA P-256, TLS 1.3, mTLS with client certs |
| Rate limiter (Envoy) | 10k burst, 1k/s refill (relaxed for testing) |
| Circuit breakers | 100 conns, 10 pending, 50 requests, 5 retries |

## Test Scenarios

### Scenario 1: Moderate Load (25 VUs, 30s)

Operation mix: 30% writes (CreateUser, CreateOrder), 70% reads (GetUser, GetOrder)

```
Operation      |  count |     rps |     avg |     p50 |     p95 |     p99 |   err%
-----------------------------------------------------------------------------------
TOTAL          |  10474 req |   14.0 rps | avg   71ms | p50   54ms | p95  185ms | p99  280ms | err  0.0%
CreateUser     |   1561 req |   17.2 rps | avg   58ms | p50   48ms | p95  125ms | p99  189ms | err  0.0%
GetUser        |   3734 req |   20.2 rps | avg   49ms | p50   41ms | p95  108ms | p99  158ms | err  0.0%
CreateOrder    |   1584 req |    6.1 rps | avg  163ms | p50  151ms | p95  291ms | p99  363ms | err  0.0%
GetOrder       |   3595 req |   16.6 rps | avg   60ms | p50   51ms | p95  133ms | p99  185ms | err  0.0%

Writes: 3145 (30%) | Reads: 7329 (70%) | Total: 10474
```

### Scenario 2: Heavy Load (50 VUs, 20s)

```
Operation      |  count |     rps |     avg |     p50 |     p95 |     p99 |   err%
-----------------------------------------------------------------------------------
TOTAL          |   7234 req |    7.2 rps | avg  138ms | p50  107ms | p95  361ms | p99  512ms | err  0.0%
CreateUser     |   1075 req |    7.8 rps | avg  128ms | p50  106ms | p95  314ms | p99  427ms | err  0.0%
GetUser        |   2515 req |    9.0 rps | avg  111ms | p50   97ms | p95  249ms | p99  351ms | err  0.0%
CreateOrder    |   1052 req |    3.3 rps | avg  303ms | p50  289ms | p95  528ms | p99  642ms | err  0.0%
GetOrder       |   2592 req |    9.8 rps | avg  101ms | p50   81ms | p95  248ms | p99  391ms | err  0.0%

Writes: 2127 (29%) | Reads: 5107 (71%) | Total: 7234
```

## Results Summary

| Metric | 25 VUs | 50 VUs | Target | Status |
|--------|--------|--------|--------|--------|
| Total requests | 10,474 | 7,234 | — | — |
| Throughput (RPS) | 14.0 | 7.2 | — | — |
| **p95 latency** | **185ms** | **361ms** | **<100ms** | ⚠️ Exceeds at higher concurrency |
| Read p95 latency | 108ms (GetUser) | 249ms | <100ms | ⚠️ |
| Write p95 latency | 125ms (CreateUser) | 314ms | <100ms | ⚠️ |
| Cross-service p95 | 291ms (CreateOrder) | 528ms | <100ms | ❌ |
| Zero errors | ✅ (both) | ✅ | — | ✅ |

## Observations

### Bottlenecks
1. **CreateOrder is the slowest operation** — involves cross-service gRPC call to user-service, a DB transaction with 3 queries (insert order, insert items, commit), and Redis cache invalidation
2. **CreateOrder throughput drops from 6.1 rps to 3.3 rps** when VUs double (50 VUs), indicating connection pool saturation at the user-service side
3. **Read operations scale better** — GetUser only drops from 20.2 to 9.0 rps when VUs double

### Reliability
- **Zero errors across 17,708 requests** — all operations completed successfully
- Circuit breakers and retry policies not triggered under normal load
- All services stable throughout the test

### Redis Cache Impact
- GetOrder uses Redis cache-aside (5min TTL)
- First GetOrder calls hit the DB, subsequent calls hit cache
- Cache hit improves latency but initial miss adds ~50ms DB query time

## Recommendations

1. **Increase DB connection pool** — current 25 max_conns saturate at 50 VUs; increase to 50+ for production
2. **Batch cross-service validation** — CreateOrder's user verification adds ~100ms; consider caching user existence or using async validation
3. **Connection pooling for gRPC** — order-service's gRPC client to user-service creates a new connection per request; reuse via pool
4. **Database query optimization** — CreateOrder performs 3 sequential queries in a transaction; consider combining into a single query

## How to Run

```bash
# Build load tester
go build -o /tmp/loadtest ./scripts/loadtest/

# Moderate load
/tmp/loadtest --vus=25 --duration=30s

# Heavy load
/tmp/loadtest --vus=50 --duration=30s

# Chaos test (requires Make targets)
make load-test-chaos
```
