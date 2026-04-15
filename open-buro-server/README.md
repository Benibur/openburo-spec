# OpenBuro Server

A Go reference implementation of the **OpenBuro** capability broker: an open standard for inter-app communication modelled on Android intents, Cozy Cloud intents, and Freedesktop portals. The server maintains a registry of application manifests and notifies connected clients in real time when the capability set changes.

> **What it does:** a client app can discover, at any moment, which other apps can fulfill a given intent (e.g. "pick a file of MIME type X") and be notified instantly when that set changes — via a simple REST + WebSocket contract.

This is the **reference implementation** of the OpenBuro platform layer. Its goal is to be the clearest, most honest code a reader can open to understand the pattern — not a hardened production service.

---

## Quickstart

**1. Copy the example configs:**

```sh
cp config.example.yaml config.yaml
cp credentials.example.yaml credentials.yaml
```

**2. Generate a bcrypt hash (cost ≥ 12) for your admin user:**

```sh
# Using htpasswd:
htpasswd -bnBC 12 "" your-password | tr -d ':\n'

# Or in Go:
go run golang.org/x/crypto/bcrypt/... # see docs/hash-gen.md
```

Paste the hash into `credentials.yaml`:

```yaml
users:
  admin: "$2a$12$YOUR-HASH-HERE"
```

**3. Edit `config.yaml` to set your CORS allow-list** (the server refuses to start with an empty list):

```yaml
cors:
  allowed_origins:
    - "http://localhost:3000"
```

**4. Build and run:**

```sh
make build
./bin/openburo-server -config config.yaml
```

**5. Test it:**

```sh
# Health check
curl http://localhost:8080/health

# Register a manifest
curl -u admin:your-password -X POST http://localhost:8080/api/v1/registry \
  -H "Content-Type: application/json" \
  -d '{"id":"mail","name":"Mail","url":"https://mail.example/","version":"1.0","capabilities":[{"action":"PICK","path":"/pick","properties":{"mimeTypes":["*/*"]}}]}'

# Query capabilities
curl 'http://localhost:8080/api/v1/capabilities?action=PICK&mimeType=image/png'

# Subscribe to real-time updates
websocat ws://localhost:8080/api/v1/capabilities/ws
```

---

## Configuration

`config.yaml` fields:

| Field | Type | Description |
|-------|------|-------------|
| `server.port` | int | HTTP listen port (1-65535) |
| `server.tls.enabled` | bool | Enable HTTPS via `ListenAndServeTLS` |
| `server.tls.cert_file` | string | TLS certificate path (required if enabled) |
| `server.tls.key_file` | string | TLS key path (required if enabled) |
| `credentials_file` | string | Path to `credentials.yaml` (bcrypt hashes, cost ≥ 12) |
| `registry_file` | string | Path to `registry.json` (persistent manifest store) |
| `websocket.ping_interval_seconds` | int | WS keepalive interval (default 30) |
| `logging.format` | string | `json` (production) or `text` (dev) |
| `logging.level` | string | `debug`, `info`, `warn`, `error` |
| `cors.allowed_origins` | list | Explicit origin allow-list. Shared with the WebSocket origin check. **Must be non-empty**. **Must not contain `"*"`** (incompatible with `AllowCredentials`). |

---

## API Reference

### REST

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/health` | — | Liveness check (returns `{"status":"ok"}`) |
| `POST` | `/api/v1/registry` | Basic | Upsert a manifest (201 create, 200 update, 400 invalid, 401 no auth) |
| `GET` | `/api/v1/registry` | — | List all manifests as `{manifests, count}` |
| `GET` | `/api/v1/registry/{appId}` | — | Fetch one manifest (404 if missing) |
| `DELETE` | `/api/v1/registry/{appId}` | Basic | Delete a manifest (204, 404, or 401) |
| `GET` | `/api/v1/capabilities` | — | Flattened capabilities with optional `?action=` and `?mimeType=` filters (symmetric `*/*` wildcard matching) |

### WebSocket

| Path | Description |
|------|-------------|
| `/api/v1/capabilities/ws` | Subscribe to registry changes. Receives a full `SNAPSHOT` event on connect, then a `REGISTRY_UPDATED` event (`change: ADDED | UPDATED | REMOVED`) on every mutation. Origin must match `cors.allowed_origins`. |

### Error envelope

Every 4xx/5xx response:

```json
{
  "error": "short human-readable message",
  "details": { "optional": "extra context" }
}
```

---

## Development

**Makefile targets:**

| Target | What it does |
|--------|--------------|
| `make build` | Build the binary into `./bin/openburo-server` |
| `make run` | Run the server with `./config.yaml` |
| `make test` | `go test ./... -race -count=1` |
| `make lint` | `gofmt -l . && go vet ./... && staticcheck ./...` |
| `make fmt` | Format all Go files |
| `make ci` | Run lint + test + build (mirrors GitHub Actions) |
| `make clean` | Remove `./bin/` |

**GSD workflow:** planning artifacts live in `.planning/` — this project was built phase-by-phase using the [get-shit-done](https://github.com/get-shit-done) structured workflow. See `.planning/ROADMAP.md` for the phase-by-phase history.

**Go toolchain:** this project requires Go 1.26. If you have multiple Go versions installed, use the full path to the 1.26 binary (e.g. `~/sdk/go1.26.2/bin/go`).

---

## Architecture

Four internal packages with a strict unidirectional dependency graph — enforced by `go list -deps` gates in CI:

```
cmd/server/main.go               (compose-root, ≤100 lines)
  │
  ├──▶ internal/config            Config loader
  │
  ├──▶ internal/registry          Manifest domain + Store + atomic persist
  │      (no imports of wshub or httpapi)
  │
  ├──▶ internal/wshub             WebSocket pub/sub hub (byte-oriented)
  │      (no imports of registry or httpapi)
  │
  └──▶ internal/httpapi           Transport layer — the SOLE package
                                   where registry and wshub meet
```

The `registry ⊥ wshub` isolation prevents the ABBA deadlock that would otherwise arise from having both packages hold their own locks during a broadcast. Mutation-then-broadcast happens exclusively in `internal/httpapi` handlers.

**Shutdown** is two-phase (PITFALLS #6): `http.Server.Shutdown` does NOT close hijacked WebSocket connections, so the compose-root runs `httpSrv.Shutdown(ctx)` first (drains HTTP), then `hub.Close()` (sends `StatusGoingAway` frames to every WS subscriber). See `cmd/server/main.go` and `cmd/server/main_test.go`.

---

## Known Limitations

This is a reference implementation, not a hardened production service. Intentional limitations:

- **No rate limiting or abuse protection** — v1 is designed for trusted admin use.
- **`X-Forwarded-For` is trusted directly** without a proxy allow-list. Production deployments behind a load balancer should add a trusted-proxy middleware.
- **No hot-reload of `credentials.yaml`** — restart the server after editing.
- **No metrics endpoint** (no Prometheus, no OpenTelemetry) — structured `slog` logging only.
- **No OAuth/OIDC** — HTTP Basic Auth only, on write routes.
- **Authentication is on WRITE routes only** — reads (`GET /registry`, `GET /capabilities`, `GET /capabilities/ws`) are public by design in the OpenBuro model.
- **Single admin assumption** — no optimistic concurrency, no `409 Conflict` on racing writes.
- **In-memory registry + JSON file** — no SQLite/Postgres backend.
- **Single event type** for WebSocket broadcasts (`REGISTRY_UPDATED`) — clients refetch `/api/v1/capabilities` on every event rather than consuming diffs. This is deliberate: "the broker has one thing to say to everyone."
- **Linux-first** — `os.Rename` atomicity on Windows has different semantics; file persistence is tuned for POSIX.

Most of these are tracked in `.planning/REQUIREMENTS.md` under the `v2` section and have an evolution path documented in the phase CONTEXT docs.

---

## License

TBD.

---

*Built with [get-shit-done](https://github.com/get-shit-done) — phase-by-phase, test-driven, race-clean.*
