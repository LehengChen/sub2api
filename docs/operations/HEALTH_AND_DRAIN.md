# Health and Drain Contract

状态：应用源码契约已实现；生产是否激活以私有 release manifest 和实际运行工件为准。

观察/编写时间：2026-07-22（Asia/Tokyo）

## 端点

| Endpoint | Meaning | Failure behavior |
|---|---|---|
| `/livez` | HTTP process is responding | Always `200` while the process can serve the handler |
| `/health` | Backward-compatible liveness alias | Same behavior as `/livez`; do not use it as an ALB readiness target |
| `/readyz` | Can receive business traffic | `200` only after initialization, PostgreSQL, Redis and migration checksum checks pass; `503` otherwise |

Readiness checks use one bounded `server.readiness_timeout_seconds` deadline and never
return driver errors, connection strings, or migration checksums in the response. The
migration check is read-only: it does not acquire the migration lock, create tables, or
apply SQL. Rows from a newer compatible release are tolerated so N/N-1 instances can
coexist.

## Drain sequence

On `SIGTERM`, the process:

1. atomically marks itself draining;
2. immediately returns `503` from `/readyz` and rejects new non-health requests with
   `Retry-After: 1`;
3. calls `http.Server.Shutdown` and waits for ordinary HTTP/SSE handlers;
4. waits for explicitly registered long-lived connections;
5. calls `Server.Close` after `server.shutdown_timeout_seconds` expires, then runs the
   existing application cleanup path.

The timeout default is 30 seconds and must be aligned with the load balancer,
container and service-manager limits in a production deployment. A deployment that
needs longer LLM streams must set the value explicitly and record the corresponding
drain budget in the private ops change plan.

Ordinary handlers are counted by the drain middleware. A handler that hijacks a
connection, including a WebSocket, must call `HealthService.RegisterLongLivedConnection`
and release the returned function when the connection closes. Until every production
WebSocket path is wired to that registry and tested across two instances, the
application must not claim complete WebSocket zero-interruption behavior.

The endpoint implementation and lifecycle state live in
`backend/internal/server/health.go`; the generated Wire graph must include
`HealthService`. `go test -tags=unit ./internal/server ./internal/repository ./cmd/server`
is the minimum source-level verification. The repository integration test exercises
the migration check when Docker-backed PostgreSQL and Redis are available.

This document describes application source behavior only. ALB target deregistration,
connection-drain settings, systemd/container timeouts, and WebSocket client retry
policy remain private infrastructure and release responsibilities.
