# Requirements: OpenBuro Server

**Defined:** 2026-04-09
**Core Value:** A client app can discover, at any moment, which other apps can fulfill a given intent (e.g. "pick a file of MIME type X"), and be notified instantly when that set changes.

## v1 Requirements

Requirements for the reference implementation. Each maps to exactly one roadmap phase.

### Foundation

- [x] **FOUND-01**: Project builds with `go build ./...` on Go 1.26 with pinned dependencies in `go.mod`
- [x] **FOUND-02**: Configuration loaded from `config.yaml` at startup (port, TLS, credential path, registry path, WS ping interval)
- [ ] **FOUND-03**: Structured logging via `log/slog` (JSON handler in production, text in dev) injected into all components
- [ ] **FOUND-04**: `GET /health` endpoint returns 200 without requiring auth
- [ ] **FOUND-05**: Startup banner log line captures version, config path, listen address, TLS state, registry path, ping interval
- [x] **FOUND-06**: CI pipeline runs `go test ./... -race`, `go vet`, and `gofmt` check
- [x] **FOUND-07**: Example `config.yaml` and `credentials.yaml` files exist at repo root for quickstart

### Registry

- [ ] **REG-01**: `Manifest` domain type validates required fields (`id`, `name`, `url`, `version`, non-empty `capabilities[]`)
- [ ] **REG-02**: `capabilities[].action` is validated against the enum `PICK | SAVE`
- [ ] **REG-03**: `capabilities[].properties.mimeTypes` must be a non-empty list; MIME strings canonicalized (lowercased, parameters stripped)
- [ ] **REG-04**: In-memory `Store` guards all state with `sync.RWMutex`; mutations serialize, reads parallelize
- [ ] **REG-05**: `Store.Upsert` creates the manifest if absent and fully replaces it if the id already exists
- [ ] **REG-06**: `Store.Delete` removes a manifest by id and reports whether it existed
- [ ] **REG-07**: `Store.Get` returns a single manifest by id or a not-found signal
- [ ] **REG-08**: `Store.List` returns all manifests in deterministic order

### Capabilities

- [ ] **CAP-01**: `Store.Capabilities(filter)` returns the flattened list of all capabilities across all manifests with `appId`, `appName`, `action`, `path`, `properties` fields
- [ ] **CAP-02**: Capability results are sorted deterministically (by `appId`, then `action`, then first mime type) so API responses are stable
- [ ] **CAP-03**: Filtering by `action` returns only capabilities whose action matches the query value exactly
- [ ] **CAP-04**: Filtering by `mimeType` supports **symmetric wildcard matching** across the full 3×3 matrix: `exact`, `type/*`, `*/*` on both the capability side and the query side
- [ ] **CAP-05**: MIME matching is covered by an exhaustive table-driven test over every 3×3 wildcard combination plus malformed input rejection

### Persistence

- [ ] **PERS-01**: Registry state loads from `registry.json` at startup; a missing file yields an empty registry without error
- [ ] **PERS-02**: Each mutation persists to disk using atomic write: temp file in the **same directory**, `tmp.Sync()`, `os.Rename`, directory fsync
- [ ] **PERS-03**: If persistence fails, the in-memory registry rolls back to the pre-mutation snapshot and the mutation returns an error
- [ ] **PERS-04**: Corrupted `registry.json` at startup fails fast with a clear error message (no silent data loss)
- [ ] **PERS-05**: `registry.json` is written human-readable (indented JSON) for inspection and debugging

### Authentication

- [ ] **AUTH-01**: `credentials.yaml` is loaded at startup with bcrypt password hashes (cost ≥ 12 enforced at load time)
- [ ] **AUTH-02**: HTTP Basic Auth middleware protects all write routes (`POST /api/v1/registry`, `DELETE /api/v1/registry/{appId}`)
- [ ] **AUTH-03**: Read routes (`GET /api/v1/registry`, `GET /api/v1/registry/{appId}`, `GET /api/v1/capabilities`, `GET /health`) are publicly accessible
- [ ] **AUTH-04**: Auth comparison is timing-safe: no early return on username mismatch, username compared via `subtle.ConstantTimeCompare`, bcrypt always runs (dummy hash fallback)
- [ ] **AUTH-05**: Credentials (Authorization header, username, password) never appear in any log line; enforced by a dedicated test

### HTTP API

- [ ] **API-01**: `POST /api/v1/registry` upserts a manifest — returns `201 Created` on create, `200 OK` on update, `400 Bad Request` on invalid payload, `401 Unauthorized` without valid auth
- [ ] **API-02**: `DELETE /api/v1/registry/{appId}` deletes a manifest — returns `204 No Content`, `404 Not Found`, or `401 Unauthorized`
- [ ] **API-03**: `GET /api/v1/registry` returns all manifests with `{ manifests: [...], count: N }` shape
- [ ] **API-04**: `GET /api/v1/registry/{appId}` returns one manifest or `404 Not Found`
- [ ] **API-05**: `GET /api/v1/capabilities` returns aggregated capabilities with `{ capabilities: [...], count: N }` shape, supporting `?action=` and `?mimeType=` query params
- [ ] **API-06**: Routes are registered using Go 1.22+ `http.ServeMux` method patterns (no third-party router)
- [ ] **API-07**: Middleware chain wraps handlers in order: recover → log → CORS → (per-route) auth
- [ ] **API-08**: Panic recovery middleware catches handler panics, logs them, returns `500 Internal Server Error`, and keeps the server alive
- [ ] **API-09**: JSON responses use a consistent error envelope on 4xx/5xx (`{ "error": "...", "details": {...} }`)
- [ ] **API-10**: Every response sets `Content-Type: application/json` where applicable
- [ ] **API-11**: Request bodies are fully read and closed so connection reuse works correctly

### WebSocket

- [ ] **WS-01**: `GET /api/v1/capabilities/ws` upgrades to WebSocket using `coder/websocket`
- [ ] **WS-02**: Centralized hub pattern: `Hub` holds subscribers map under a mutex, `subscriber` has a buffered outbound channel (default 16)
- [ ] **WS-03**: Non-blocking fan-out: publishing to a slow subscriber whose buffer is full triggers `closeSlow` (drop the client) rather than blocking the publisher
- [ ] **WS-04**: Each subscriber calls `conn.CloseRead(ctx)` so control frames are handled and closed clients are detected (prevents goroutine leaks)
- [ ] **WS-05**: Every mutation (upsert, delete) broadcasts a `REGISTRY_UPDATED` event with `{ event, timestamp, payload: { appId, change: ADDED|UPDATED|REMOVED } }`
- [ ] **WS-06**: On connect, the new subscriber receives a full-state `REGISTRY_UPDATED` snapshot before any subsequent events (eliminates connect-then-fetch race)
- [ ] **WS-07**: Periodic ping frames keep connections alive (default 30s, configurable from `config.yaml`)
- [ ] **WS-08**: WebSocket origin checking uses `AcceptOptions.OriginPatterns` sourced from the same allow-list as CORS; `InsecureSkipVerify` never appears in production code
- [ ] **WS-09**: Broadcast is triggered by the HTTP handler layer **after** the registry mutation succeeds — the registry package never imports the wshub package (enforced by design to prevent ABBA deadlock)
- [ ] **WS-10**: Goroutine leak integration test: 1000 connect/disconnect cycles leave `runtime.NumGoroutine()` flat (±epsilon)

### Operations

- [ ] **OPS-01**: CORS middleware (`rs/cors`) is configured from `config.yaml` with an explicit origin allow-list (no `*` when credentials are involved)
- [ ] **OPS-02**: `cmd/server/main.go` wires all components via compose-root pattern and remains under ~100 lines
- [ ] **OPS-03**: Signal-aware graceful shutdown: `signal.NotifyContext` listens for `SIGTERM`/`SIGINT`
- [ ] **OPS-04**: **Two-phase shutdown**: `httpSrv.Shutdown(ctx)` first, then `hub.Close()` which sends `StatusGoingAway` close frames to every WebSocket client
- [ ] **OPS-05**: Optional TLS termination when `server.tls.enabled = true` using `ListenAndServeTLS` with cert and key paths from config
- [ ] **OPS-06**: Write operations emit a structured audit log line (`slog.Info("audit", "user", …, "action", …, "appId", …)`) without leaking credentials

### Testing & Quality

- [ ] **TEST-01**: Table-driven unit tests cover `Manifest.Validate`, `Store` mutations, and the full MIME matching matrix
- [ ] **TEST-02**: Integration tests via `httptest.NewServer` cover REST round-trips (upsert → list → delete) and WebSocket round-trips (mutation → event received within timeout)
- [ ] **TEST-03**: Test suite passes under `go test ./... -race` without warnings
- [ ] **TEST-04**: Persistence rollback test uses an unwritable directory to prove in-memory state rolls back on persist failure
- [ ] **TEST-05**: WebSocket origin-rejection test asserts a disallowed `Origin` header returns `403`
- [ ] **TEST-06**: Dedicated test captures slog output across a failed-auth scenario and asserts no credential material appears
- [x] **TEST-07**: Project follows idiomatic Go layout: `cmd/server/` + `internal/{config,registry,httpapi,wshub}/`

## v2 Requirements

Deferred to future releases. Tracked but not in the current roadmap.

### Storage

- **STOR-V2-01**: Pluggable storage backend (SQLite, Postgres)

### Concurrency

- **CONC-V2-01**: Optimistic concurrency control with version field and `409 Conflict` response

### Operations

- **OPS-V2-01**: Hot-reload of `credentials.yaml` without server restart
- **OPS-V2-02**: Prometheus `/metrics` endpoint
- **OPS-V2-03**: OpenTelemetry tracing

### Security

- **SEC-V2-01**: Authentication on WebSocket and read routes (restricted registries)
- **SEC-V2-02**: Rate limiting / abuse protection
- **SEC-V2-03**: OAuth/OIDC support

### Multi-tenancy

- **MULT-V2-01**: Multiple independent registries per server instance

### Features

- **FEAT-V2-01**: Event coalescing / debounce (50-100ms) for burst mutations
- **FEAT-V2-02**: Additional capability actions beyond `PICK | SAVE` (e.g. `VIEW`, `EDIT`, `SEND`)

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Capability invocation through the broker | Violates the out-of-data-path architectural bet — the broker discovers, it does not proxy |
| Subscribe/unsubscribe WS protocol | Defeats "broker has one thing to say to everyone" — adds topic-routing complexity without value |
| Multiple WebSocket event types (ADDED/UPDATED/REMOVED as separate events) | Forces clients into reconcile-diff logic; single event type lets clients do `state = event.capabilities` |
| JSON Schema validation | Hand-rolled validation is ~50 lines; JSON Schema adds a "which source of truth wins" conversation |
| Per-event diff payloads | Premature optimization; full re-fetch is fine at expected registry size |
| Concurrency stress testing | Reference impl, not a hardened service; standard `-race` coverage is sufficient |
| Full `golang-standards/project-layout` ceremony | Not an official standard; buries a 4-domain server under empty directories |
| Framework (gin, echo, fiber) | stdlib `net/http` ServeMux covers every routing need post-Go 1.22 |
| ORM / database layer | Spec mandates file-based JSON for v1 |
| Windows-specific persistence atomicity work | Linux-first reference impl; Windows `os.Rename` caveat noted in README |

## Traceability

Populated during roadmap creation on 2026-04-09.

| Requirement | Phase | Status |
|-------------|-------|--------|
| FOUND-01 | Phase 1 | Complete |
| FOUND-02 | Phase 1 | Complete |
| FOUND-03 | Phase 1 | Pending |
| FOUND-04 | Phase 1 | Pending |
| FOUND-05 | Phase 1 | Pending |
| FOUND-06 | Phase 1 | Complete |
| FOUND-07 | Phase 1 | Complete |
| REG-01 | Phase 2 | Pending |
| REG-02 | Phase 2 | Pending |
| REG-03 | Phase 2 | Pending |
| REG-04 | Phase 2 | Pending |
| REG-05 | Phase 2 | Pending |
| REG-06 | Phase 2 | Pending |
| REG-07 | Phase 2 | Pending |
| REG-08 | Phase 2 | Pending |
| CAP-01 | Phase 2 | Pending |
| CAP-02 | Phase 2 | Pending |
| CAP-03 | Phase 2 | Pending |
| CAP-04 | Phase 2 | Pending |
| CAP-05 | Phase 2 | Pending |
| PERS-01 | Phase 2 | Pending |
| PERS-02 | Phase 2 | Pending |
| PERS-03 | Phase 2 | Pending |
| PERS-04 | Phase 2 | Pending |
| PERS-05 | Phase 2 | Pending |
| AUTH-01 | Phase 4 | Pending |
| AUTH-02 | Phase 4 | Pending |
| AUTH-03 | Phase 4 | Pending |
| AUTH-04 | Phase 4 | Pending |
| AUTH-05 | Phase 4 | Pending |
| API-01 | Phase 4 | Pending |
| API-02 | Phase 4 | Pending |
| API-03 | Phase 4 | Pending |
| API-04 | Phase 4 | Pending |
| API-05 | Phase 4 | Pending |
| API-06 | Phase 4 | Pending |
| API-07 | Phase 4 | Pending |
| API-08 | Phase 4 | Pending |
| API-09 | Phase 4 | Pending |
| API-10 | Phase 4 | Pending |
| API-11 | Phase 4 | Pending |
| WS-01 | Phase 4 | Pending |
| WS-02 | Phase 3 | Pending |
| WS-03 | Phase 3 | Pending |
| WS-04 | Phase 3 | Pending |
| WS-05 | Phase 4 | Pending |
| WS-06 | Phase 4 | Pending |
| WS-07 | Phase 3 | Pending |
| WS-08 | Phase 4 | Pending |
| WS-09 | Phase 4 | Pending |
| WS-10 | Phase 3 | Pending |
| OPS-01 | Phase 4 | Pending |
| OPS-02 | Phase 5 | Pending |
| OPS-03 | Phase 5 | Pending |
| OPS-04 | Phase 5 | Pending |
| OPS-05 | Phase 5 | Pending |
| OPS-06 | Phase 4 | Pending |
| TEST-01 | Phase 2 | Pending |
| TEST-02 | Phase 4 | Pending |
| TEST-03 | Phase 5 | Pending |
| TEST-04 | Phase 2 | Pending |
| TEST-05 | Phase 4 | Pending |
| TEST-06 | Phase 4 | Pending |
| TEST-07 | Phase 1 | Complete |

**Coverage:**
- v1 requirements: 64 total (the header count of "63" from initial drafting was off by one — actual checkbox count is 64)
- Mapped to phases: 64 ✓
- Unmapped: 0

**Phase Distribution:**

| Phase | Requirements | Count |
|-------|--------------|-------|
| 1. Foundation | FOUND-01..07, TEST-07 | 8 |
| 2. Registry Core | REG-01..08, CAP-01..05, PERS-01..05, TEST-01, TEST-04 | 20 |
| 3. WebSocket Hub | WS-02, WS-03, WS-04, WS-07, WS-10 | 5 |
| 4. HTTP API | AUTH-01..05, API-01..11, WS-01, WS-05, WS-06, WS-08, WS-09, OPS-01, OPS-06, TEST-02, TEST-05, TEST-06 | 26 |
| 5. Wiring, Shutdown & Polish | OPS-02, OPS-03, OPS-04, OPS-05, TEST-03 | 5 |
| **Total** | | **64** |

---
*Requirements defined: 2026-04-09*
*Last updated: 2026-04-09 — traceability populated by roadmapper; phase distribution finalized*
