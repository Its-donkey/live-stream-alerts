# Architecture Overview

This document explains how the alert server is structured so new contributors can reason about responsibilities without re-reading the entire codebase.

## Layered flow

```
cmd/alertserver ➜ internal/app ➜ internal/api/v1 ➜ services ➜ stores/platform clients
```

1. **`cmd/alertserver`** wires CLI flags/env vars and delegates to `internal/app`.
2. **`internal/app`** loads configuration, builds dependencies (stores, HTTP router, YouTube lease monitor), and manages process lifecycle (HTTP server + background workers) using contexts.
3. **`internal/api/v1`** registers HTTP routes. Handlers remain thin: they validate HTTP specifics (verbs, headers, JSON) and hand work to dedicated services.
4. **Services** (for streamers, admin, YouTube channel/metadata/subscription/alert flows) encapsulate business rules and call downstream dependencies via small interfaces so tests can mock them.
5. **Stores/platform clients** are the only layers allowed to touch disk or make outbound HTTP requests. Stores hide file locking/encoding; platform clients keep PubSubHubbub and YouTube parsing contained.

## Key packages

| Package | Responsibility |
| --- | --- |
| `internal/app` | Bootstraps config, logging, servers, background monitors. |
| `internal/api/v1` | HTTP router; each handler defers to a service interface quickly. |
| `internal/streamers/service` | Streamer CRUD + submissions queueing. |
| `internal/platforms/youtube/service` | Channel lookup, metadata scraping, subscription proxying, WebSub alert processing. |
| `internal/platforms/youtube/subscriptions` | PubSubHubbub client, lease monitor, renewal helpers. |
| `internal/admin/service` | Auth + submission approval flows. |
| `internal/streamers` & `internal/submissions` | File-backed stores with per-path mutexes. |

## Background workers

- **Lease monitor**: `internal/platforms/youtube/subscriptions.LeaseMonitor` watches stored YouTube records and silently renews subscriptions 5% before expiration. `app.Run` owns its lifecycle via `StartLeaseMonitor/Stop`.
- **Streamers watch SSE**: `internal/api/v1/streamers_watch.go` polls `streamers.json` and streams change notifications to clients. The poller is scoped to the HTTP handler request context so it automatically stops when clients disconnect.

## Configuration surfaces

- `config/config.go` loads `config.json`, merging `server`, `youtube`, and `admin` blocks with CLI/env overrides.
- Flags/env vars are declared in `cmd/alertserver/main.go`; everything is passed through `app.Options`, avoiding global mutable config.

## Testing philosophy

- Services accept interfaces (stores, HTTP clients, clock/ID generators) so tests can inject determinism.
- Table-driven tests cover validation edge cases (`internal/streamers/service`, `internal/platforms/youtube/service`, stores).
- Handler tests mock services/processors to assert HTTP behavior separately from core logic.

## Adding new features

1. Decide whether the change belongs in a service or store. Handlers should only parse HTTP inputs and forward to services.
2. Update/add services/interfaces if new business logic is required, keeping dependencies injectable.
3. Document new routes in `README.md` and extend this architecture doc if a significant new subsystem is introduced.
4. Ensure `gofmt`, `go vet`, and `go test ./...` pass locally—the CI workflow enforces all three.
