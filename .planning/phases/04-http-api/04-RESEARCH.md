# Phase 4: HTTP API - Research

**Researched:** 2026-04-10
**Domain:** `internal/httpapi` transport layer — `net/http` ServeMux (Go 1.22+ method patterns), bcrypt (`golang.org/x/crypto/bcrypt`), CORS middleware (`rs/cors` v1.11.1), `coder/websocket` v1.8.14 `OriginPatterns`, timing-safe Basic Auth, and the integration-test harness that glues it all to `httptest.NewServer`
**Confidence:** HIGH — every library API claim below was verified against source in `~/go/pkg/mod/` or `~/sdk/go1.26.2/src/`

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Server constructor — expand Phase 1 without breaking it.**
- Signature: `New(logger *slog.Logger, store *registry.Store, hub *wshub.Hub, creds Credentials, cfg Config) *Server`
- All dependencies are required; nil values for any of `store`, `hub`, `creds`, or `logger` panic at construction (no silent `slog.Default()` fallback)
- `cfg` is a Phase 4-local struct carrying `AllowedOrigins []string` + `WSPingInterval time.Duration` (compose-root in Phase 5 translates `config.Config` → `httpapi.Config`; `internal/httpapi` never imports `internal/config`)
- Phase 1's `handleHealth` handler and `TestHealth` / `TestHealth_RejectsWrongMethod` stay; the health test is adapted to use the new `newTestServer(t)` helper rather than a backward-compat `NewForHealthOnly` constructor
- Empty `AllowedOrigins: []` causes the Server constructor to return an error (fail-fast at startup)

**Credentials type and loading (AUTH-01).**
- Exported `Credentials` type: `struct { users map[string][]byte }` — map of username → bcrypt hash bytes
- `LoadCredentials(path string) (Credentials, error)` enforces bcrypt cost ≥ 12 via `bcrypt.Cost(hash)`; missing file is an ERROR (not silent empty)
- `(c Credentials) Lookup(username string) ([]byte, bool)` — returns hash + ok
- Zero value is an empty table; all writes return 401

**Timing-safe Basic Auth middleware (AUTH-02, AUTH-04).**
- `authBasic` runs `bcrypt.CompareHashAndPassword` **unconditionally** for every request (with a `dummyHash` fallback on unknown user)
- Authorization decision = `subtle.ConstantTimeCompare([]byte{foundByte, matchByte}, []byte{1, 1}) == 1` — NOT a short-circuit `if found && bcryptMatches`
- Dummy hash is precomputed once at package init at cost 12: `bcrypt.GenerateFromPassword([]byte("openburo:dummy:do-not-match"), 12)`
- On failure, Warn log line carries only `path`, `method`, `remote` — **NEVER** `username`, `Authorization`, or `password`
- On success, username is stashed in request context under an unexported `ctxKeyUser` type (not a string literal)

**Credential PII guard (TEST-06).**
- Dedicated test captures slog output across successful + failed auth via `slog.NewTextHandler` on a mutex-guarded `bytes.Buffer`
- Asserts `require.NotContains` for: the literal `Basic ...` header, decoded username, decoded password, raw bcrypt hash

**Error envelope (API-09).**
- Every 4xx/5xx response uses `{"error": "...", "details": {...}}`
- `writeJSONError(w, status, message, details)` helper + shortcut constructors per status
- 401 responses set `WWW-Authenticate: Basic realm="openburo"`
- Error messages are short, lowercase, no trailing period

**Route registration (API-06).**
- Go 1.22+ method-prefixed `http.ServeMux` patterns; no third-party router
- `{appId}` wildcard extracted via `r.PathValue("appId")`
- Method-prefixed patterns auto-return 405 for wrong methods (empirically confirmed by existing `TestHealth_RejectsWrongMethod` in `internal/httpapi/health_test.go`)
- Routes (in plan-04-03/04-04 order):
  - `GET /health` (public — carried over from Phase 1)
  - `POST /api/v1/registry` (auth required)
  - `DELETE /api/v1/registry/{appId}` (auth required)
  - `GET /api/v1/registry` (public)
  - `GET /api/v1/registry/{appId}` (public)
  - `GET /api/v1/capabilities` (public)
  - `GET /api/v1/capabilities/ws` (public, WS upgrade)

**Middleware chain order (API-07) — recover is OUTERMOST.**
- Request flow: `recover → log → CORS → mux → (per-route) auth → handler`
- `recoverMiddleware` wraps the entire chain so a panic in log/CORS/auth/handler is still caught
- `logMiddleware` explicitly skips `/health` to avoid log spam
- `authBasic` is applied per-route inside `registerRoutes`, NOT as a global wrap
- CORS middleware is `cors.New(opts).Handler(next)` from `github.com/rs/cors`

**JSON decoding discipline (API-11).**
- `json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))` — 1 MiB body cap
- `dec.DisallowUnknownFields()` so typos fail fast with 400
- `defer r.Body.Close()` in every POST/DELETE handler (API-11 connection reuse contract)

**Event shape (WS-05, WS-06).**
- Every broadcast is `{"event": "REGISTRY_UPDATED", "timestamp": "<RFC 3339 millis UTC>", "payload": {...}}`
- Upsert/delete payload: `{"appId": "<id>", "change": "ADDED" | "UPDATED" | "REMOVED"}`
- Initial WS snapshot payload: `{"change": "SNAPSHOT", "capabilities": [... full Store.Capabilities(filter{}) output ...]}`
- `eventPayload` struct uses `omitempty` on both `AppID` and `Capabilities` so upsert events have no `capabilities` field and snapshot events have no `appId` field
- Timestamp format: `time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00")` (RFC 3339 with millisecond precision)
- `changeType` is an unexported string type with four constants: `changeAdded`, `changeUpdated`, `changeRemoved`, `changeSnapshot`

**Mutation-then-broadcast wiring (WS-05, WS-09).**
- `Store.Upsert` / `Store.Delete` completes first, THEN `hub.Publish(newRegistryUpdatedEvent(...))` fires
- `internal/registry` NEVER imports `internal/wshub` — enforced by `go list -deps ./internal/registry | grep -E 'wshub|httpapi'` must be empty
- Phase 3's gate continues to apply: `go list -deps ./internal/wshub | grep -E 'registry|httpapi'` must also be empty

**CORS middleware (OPS-01).**
- `github.com/rs/cors` v1.11.x configured from `cfg.AllowedOrigins`
- Same `AllowedOrigins` slice is also passed to `websocket.AcceptOptions.OriginPatterns` (shared allow-list)
- `AllowedMethods: GET, POST, DELETE, OPTIONS`; `AllowedHeaders: Authorization, Content-Type`; `AllowCredentials: true`; `MaxAge: 300`
- **`AllowCredentials: true` + `AllowedOrigins: ["*"]` combination is rejected at Server-constructor time** — see §"CORS v1.11.1 behavior (CORRECTION)" below; CONTEXT.md's assumption that rs/cors panics is WRONG and the Server constructor must enforce this itself

**WebSocket upgrade handler (WS-01, WS-06, WS-08).**
- `websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: s.cfg.AllowedOrigins})`
- `InsecureSkipVerify` is NEVER set — reviewers grep for it as a banned string
- Snapshot write uses a 5s bounded context: `context.WithTimeout(r.Context(), 5*time.Second)`
- On snapshot write failure: `conn.Close(websocket.StatusInternalError, "snapshot write failed")` then return
- After snapshot, hand off to `s.hub.Subscribe(r.Context(), conn)` which blocks until disconnect

**Audit log (OPS-06).**
- SECOND log line per write, emitted AFTER mutation succeeds and broadcast publishes: `logger.Info("httpapi: audit", "user", username, "action", "upsert"|"delete", "appId", id)`
- Never log manifest body, URL, credentials, or Authorization header

**Request log (API-07, OPS-06).**
- Fields: `method`, `path`, `status`, `duration_ms`, `remote` — deliberately NO `user` field
- Uses a `statusCapturingWriter` wrapper to capture the final status code
- `/health` is explicitly skipped inside `logMiddleware`

**Recover middleware (API-08).**
- `defer func() { if rec := recover(); rec != nil { ... } }` pattern
- Logs: `path`, `method`, `panic` (via `%v` not `%+v`), `stack` (via `debug.Stack()`)
- Writes 500 + error envelope; server keeps serving
- Test: `TestRecover_PanicCaught` registers a panicking route, hits via `httptest.NewServer`, asserts 500 + envelope + server alive on next request

**`clientIP` helper** — respects X-Forwarded-For first entry, falls back to `r.RemoteAddr`. Documented as reference-impl simplification; README notes a trusted-proxy allow-list would be needed for production.

**Package layout — 8 production files + matching tests + testdata/.**
- `doc.go`, `server.go`, `credentials.go`, `auth.go`, `middleware.go`, `errors.go`, `events.go`, `handlers_registry.go`, `handlers_caps.go`, `health.go` (unchanged from Phase 1)
- Tests: `server_test.go`, `credentials_test.go`, `auth_test.go`, `middleware_test.go`, `errors_test.go`, `events_test.go`, `handlers_registry_test.go`, `handlers_caps_test.go`, `health_test.go` (Phase 1, adapted to `newTestServer`)
- `testdata/credentials-valid.yaml`, `credentials-low-cost.yaml`, `credentials-malformed.yaml`
- Target: ~800–1200 LoC production + ~1500–2000 LoC tests

**Plan breakdown (5 sequential plans, ROADMAP-aligned).**
- `04-01-server-middleware` (Wave 1): Server expansion + Config + recover/log/CORS middleware skeleton (CORS wired but allow-list not yet shared with WS) + error envelope + registerRoutes with handler stubs. Lands: API-06, API-07, API-08, API-09, API-10, API-11.
- `04-02-auth-credentials` (Wave 2): Credentials + LoadCredentials + authBasic + dummyHash + PII-guard test. Lands: AUTH-01..05, TEST-06. Depends on 04-01.
- `04-03-registry-handlers` (Wave 3): 4 REST handlers + events.go + mutation-then-broadcast + audit log. Lands: API-01..04, WS-05, WS-09, OPS-06. Depends on 04-02.
- `04-04-capabilities-ws` (Wave 4): capabilities handler + WS upgrade + snapshot on connect. Lands: API-05, WS-01, WS-06. Depends on 04-03.
- `04-05-cors-integration-tests` (Wave 5): shared allow-list wiring, OriginPatterns, integration tests (REST round-trip + WS round-trip), origin rejection test, PII end-to-end. Lands: OPS-01, WS-08, TEST-02, TEST-05, TEST-06 (final). Depends on 04-04.
- Waves are sequential — no intra-phase parallelism.

**Go toolchain.**
- Every `<automated>` verify command MUST use `~/sdk/go1.26.2/bin/go`. System `go` is 1.22 and will fail.

**Dependencies to add.**
- `golang.org/x/crypto/bcrypt` — Task 0 of plan 04-02 (`go get golang.org/x/crypto@latest`)
- `github.com/rs/cors` — Task 0 of plan 04-05 (`go get github.com/rs/cors@latest`)
- `github.com/coder/websocket` is already a direct dep from Phase 3
- All via `~/sdk/go1.26.2/bin/go get` + `go mod tidy`

### Claude's Discretion

- Exact test function names (mirror Phase 2/3 style: `TestServer_*`, `TestAuth_*`, `TestHandleRegistry_*`)
- Whether to split `handlers_registry.go` and `handlers_caps.go` or keep them combined (`handlers.go`) — planner's call
- Exact phrasing of error envelope messages (keep short + lowercase + no trailing period)
- How to generate bcrypt fixtures for testdata (one-shot helper test or shell script)
- `t.Cleanup` inside `newTestServer` vs returning a manual `cleanup()` — either works
- Integration test structure: monolithic function vs `t.Run` sub-steps — both acceptable

### Deferred Ideas (OUT OF SCOPE)

- `cmd/server/main.go` compose-root wiring (OPS-02) — Phase 5
- Signal-aware graceful shutdown (OPS-03) — Phase 5
- Two-phase `httpSrv.Shutdown` → `hub.Close` (OPS-04) — Phase 5
- Optional TLS via `ListenAndServeTLS` (OPS-05) — Phase 5
- Whole-module `go test ./... -race` gate (TEST-03) — Phase 5
- WS / read-route authentication (SEC-V2-01) — v2
- Rate limiting (SEC-V2-02) — v2
- OAuth/OIDC (SEC-V2-03) — v2
- Hot-reload credentials (OPS-V2-01) — v2
- Prometheus `/metrics` (OPS-V2-02) — v2
- OpenTelemetry tracing (OPS-V2-03) — v2
- Event coalescing / debounce (FEAT-V2-01) — v2
- Trusted-proxy allow-list for X-Forwarded-For — v2
- Request body streaming — not planned; 1 MiB cap covers reference-impl workload
- Pagination on list endpoints — v2 if ever needed
- Optimistic concurrency / ETag (CONC-V2-01) — v2

</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| **AUTH-01** | `credentials.yaml` loaded at startup with bcrypt hashes, cost ≥ 12 enforced | `bcrypt.Cost([]byte) (int, error)` verified (§bcrypt API). `LoadCredentials` rejects cost < 12, missing file, and malformed hash with distinct errors. Fixtures in `testdata/`. |
| **AUTH-02** | HTTP Basic Auth middleware protects `POST /api/v1/registry` and `DELETE /api/v1/registry/{appId}` | `authBasic` applied per-route in `registerRoutes` via `s.mux.Handle("POST /api/v1/registry", s.authBasic(http.HandlerFunc(s.handleRegistryUpsert)))`. |
| **AUTH-03** | Read routes + `/health` are publicly accessible | `handleRegistryList`, `handleRegistryGet`, `handleCapabilities`, `handleCapabilitiesWS`, `handleHealth` registered via `HandleFunc` (no auth wrap). |
| **AUTH-04** | Timing-safe: no early return on username mismatch, bcrypt always runs | Verified pattern: bcrypt runs unconditionally with `dummyHash` on unknown user, final decision via `subtle.ConstantTimeCompare([]byte{foundByte, matchByte}, []byte{1, 1})`. See §Timing-Safe Basic Auth. |
| **AUTH-05** | Credentials never appear in logs | Enforced by `TestAuth_NoCredentialsInLogs` — captures slog output across successful + failed auth, asserts `require.NotContains` for `Basic`, username, password, hash. |
| **API-01** | `POST /api/v1/registry` — 201 create, 200 update, 400 invalid, 401 unauth | `handleRegistryUpsert` snapshots `existed` before `store.Upsert`, uses result to choose 201 vs 200. |
| **API-02** | `DELETE /api/v1/registry/{appId}` — 204, 404, 401 | `handleRegistryDelete` extracts `appId` via `r.PathValue("appId")` (verified §PathValue), calls `store.Delete`. |
| **API-03** | `GET /api/v1/registry` returns `{manifests, count}` | `handleRegistryList` shape-wraps `store.List()`. |
| **API-04** | `GET /api/v1/registry/{appId}` returns one or 404 | `handleRegistryGet` via `store.Get(appId)`. |
| **API-05** | `GET /api/v1/capabilities` with `?action=` + `?mimeType=` | Parse query via `r.URL.Query().Get(...)`; pre-validate `mimeType` via `registry.CanonicalizeMIME` for 400 on malformed (Phase 2 lock: bad filter → empty result, so pre-validate to distinguish). |
| **API-06** | Go 1.22+ `http.ServeMux` method patterns, no router | Verified §ServeMux Method Patterns — patterns like `"POST /api/v1/registry"` with space separator; precedence via disjoint paths; 405 (not 404) for wrong method via `Allow` header. |
| **API-07** | Middleware chain: recover → log → CORS → (per-route) auth | Chain constructed in `Server.Handler()` with `recoverMiddleware` as outermost wrap. |
| **API-08** | Panic recovery middleware catches handler panics, logs, returns 500, keeps server alive | `defer func() { recover() }` in `recoverMiddleware`; writes envelope via `writeInternal`. Test via a panicking sub-route registered only in tests. |
| **API-09** | JSON error envelope `{error, details}` on 4xx/5xx | `writeJSONError` helper; all branches use it. |
| **API-10** | `Content-Type: application/json` on every response | Set at top of every handler before `WriteHeader`; `writeJSONError` sets it too. |
| **API-11** | Request bodies fully read and closed for connection reuse | `defer r.Body.Close()` in every handler that touches the body; `json.Decoder` drains on decode. |
| **WS-01** | `GET /api/v1/capabilities/ws` upgrades via `coder/websocket` | `handleCapabilitiesWS` calls `websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: s.cfg.AllowedOrigins})`. |
| **WS-05** | Every mutation broadcasts `REGISTRY_UPDATED` with `change = ADDED/UPDATED/REMOVED` | Handler layer calls `s.hub.Publish(newRegistryUpdatedEvent(id, change))` AFTER `store.Upsert/Delete` returns nil. |
| **WS-06** | New subscriber receives full-state snapshot before subsequent events | `handleCapabilitiesWS` calls `s.buildFullStateSnapshot()` (SNAPSHOT event with `payload.capabilities = Store.Capabilities(filter{})`), writes it with a 5s-bounded context, THEN hands off to `hub.Subscribe`. |
| **WS-08** | `AcceptOptions.OriginPatterns` from shared CORS allow-list, no `InsecureSkipVerify` | Both fed from `s.cfg.AllowedOrigins`. Verified §`OriginPatterns` Semantics — patterns use `path.Match` glob syntax. |
| **WS-09** | Broadcast triggered by HTTP handler AFTER mutation succeeds; registry never imports wshub | Architectural gate: `~/sdk/go1.26.2/bin/go list -deps ./internal/registry \| grep -E 'wshub\|httpapi'` must produce empty output. |
| **OPS-01** | CORS middleware from `config.yaml` with explicit origin allow-list (no `*` when credentials involved) | `rs/cors` v1.11.1 used; the `*`+credentials combination is rejected at Server-constructor time (rs/cors does NOT reject it itself — see §CORS v1.11.1 CORRECTION). |
| **OPS-06** | Write operations emit structured audit log | Second log line per write: `logger.Info("httpapi: audit", "user", ..., "action", ..., "appId", ...)`. |
| **TEST-02** | Integration tests via `httptest.NewServer` — REST round-trip + WS round-trip | `TestServer_Integration_RESTRoundTrip` + `TestServer_Integration_WebSocketRoundTrip` use `httptest.NewServer(srv.Handler())`. |
| **TEST-05** | WS origin-rejection test — disallowed Origin returns 403 | Dial with `DialOptions{HTTPHeader: http.Header{"Origin": []string{"https://evil.example"}}}`, assert `resp.StatusCode == 403`. Verified §WS Origin Rejection Path. |
| **TEST-06** | Dedicated test captures slog output across failed auth, asserts no credential material | `bytes.Buffer` behind a mutex + `slog.NewTextHandler`; `require.NotContains` for 4 PII categories. |

</phase_requirements>

## Summary

Phase 4 is a **100% stdlib + 5 already-blessed deps** transport layer. Every dependency API claim in CONTEXT.md was verified against module source in `~/go/pkg/mod/` and `~/sdk/go1.26.2/src/`. **Six findings in this research contradict, sharpen, or correct CONTEXT.md assumptions** — the planner must incorporate them verbatim:

1. **rs/cors v1.11.1 does NOT panic on `AllowedOrigins: ["*"] + AllowCredentials: true`** (CONTEXT.md is wrong). It silently emits both `Access-Control-Allow-Origin: *` and `Access-Control-Allow-Credentials: true`, which browsers then reject at the fetch layer. The Server constructor must enforce this combination itself via an explicit `if` check before calling `cors.New`.
2. **`coder/websocket` v1.8.14 `OriginPatterns` uses `path.Match` glob syntax**, not prefix/exact. Patterns are matched case-insensitively. `*` as a standalone pattern is discouraged (the library prints a warning telling you to use `InsecureSkipVerify` instead). Scheme-aware patterns are supported: `https://*.example.com` matches against `scheme://host`.
3. **`websocket.Dial` accepts `http://` / `https://` URLs natively** (dial.go:119). The CONTEXT.md test example uses `"ws" + strings.TrimPrefix(ts.URL, "http")` — this rewrite still works but is unnecessary. Planner may simplify by passing `ts.URL + "/api/v1/capabilities/ws"` directly to `Dial`.
4. **`http.MaxBytesReader` does NOT auto-write 413.** It returns `*http.MaxBytesError` from `Read()` when the limit is exceeded. The handler must catch the decoder error (it will be wrapped) and write its own 400 (CONTEXT.md already calls `writeBadRequest` on decode error, so this is already correct — but the planner should know the specific error type exists for `errors.As` pattern matching if finer-grained handling is ever needed).
5. **`Origin: ""` (missing Origin header) is ALWAYS allowed by `authenticateOrigin`** (accept.go:230). CLI tools like `curl`, `wscat`, and `websocket.Dial` without `HTTPHeader: {"Origin": ...}` send no Origin header, and the handshake succeeds. The origin rejection test MUST explicitly set `DialOptions.HTTPHeader["Origin"]`.
6. **Same-host requests bypass `OriginPatterns` entirely** (accept.go:239 — `if strings.EqualFold(r.Host, u.Host) { return nil }`). A request whose `Origin` header's host matches the request `Host` header is auto-authorized regardless of `OriginPatterns`. For `httptest.NewServer`, this means a test that sets `Origin: <ts.URL host>` will always pass origin check — the disallowed-origin test must set a DIFFERENT host (e.g. `https://evil.example`).

**Primary recommendation:** Mirror CONTEXT.md's locked patterns verbatim, with the 6 corrections above folded in. The Server constructor gains explicit validation of the `*`+credentials combination; the origin rejection test uses an explicit evil-host Origin header; the WS integration test can simplify by passing the http:// URL directly to `websocket.Dial`.

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `golang.org/x/crypto/bcrypt` | latest tracked via `x/crypto` v0.50.0 (2026) | Password hashing + cost verification | Project-level lock in STACK.md. Exposes `GenerateFromPassword`, `CompareHashAndPassword`, `Cost`. Verified at `~/go/pkg/mod/golang.org/x/crypto@v0.48.0/bcrypt/bcrypt.go`. |
| `github.com/rs/cors` | **v1.11.1** (published 2024-08-29, still latest 2026-04-10) | CORS middleware | Project-level lock in STACK.md. Wraps any `http.Handler`. API verified by fetching `v1.11.1/cors.go` from GitHub. |
| `github.com/coder/websocket` | **v1.8.14** | WebSocket Accept + Dial | Already direct dep from Phase 3. `Accept(w, r, *AcceptOptions).OriginPatterns` is the only new surface touched in Phase 4. |
| `net/http` (Go 1.22+) | stdlib (Go 1.26.2) | ServeMux method patterns + `r.PathValue` + `http.MaxBytesReader` | Project-level lock. Post-1.22 `ServeMux` is sufficient for every routing need in this phase. |
| `crypto/subtle` | stdlib | `ConstantTimeCompare` for the final auth decision | Standard Go timing-safe comparison primitive. |
| `encoding/json` | stdlib | JSON decode/encode with `DisallowUnknownFields` | Project-level lock. |
| `log/slog` | stdlib | Structured logging (injected, never `slog.Default()`) | Phase 1 lock. |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `go.yaml.in/yaml/v3` | v3.0.x (already direct dep) | Parse `credentials.yaml` | Existing Phase 1 dep; used by `LoadCredentials`. |
| `github.com/stretchr/testify/require` | v1.11.1 (already direct dep) | Test assertions incl. `require.Eventually` | Phase 1 lock. `require.Eventually` replaces `time.Sleep` in WS event arrival tests. |

### Alternatives Considered
| Instead of | Could Use | Tradeoff | Verdict |
|------------|-----------|----------|---------|
| `rs/cors` | `jub0bs/cors` | Better validation (incl. rejecting `*`+credentials at construction), cleaner API | STACK.md lock says rs/cors; stay the course. Add the validation manually in `httpapi.New`. |
| Go 1.22 ServeMux | `go-chi/chi` | Sub-route groups, per-group middleware | Overkill for 6 routes. Stdlib sufficient. |
| `subtle.ConstantTimeCompare` on byte tuple | Compute `bool` then combine arithmetically | Same timing profile, less clarity | Keep CONTEXT.md tuple pattern — it's the defensible explicit form. |

**Installation (reference):**
```bash
~/sdk/go1.26.2/bin/go get golang.org/x/crypto@latest
~/sdk/go1.26.2/bin/go get github.com/rs/cors@latest
~/sdk/go1.26.2/bin/go mod tidy
```

**Version verification performed 2026-04-10:**
- `go list -m -versions golang.org/x/crypto` → latest is v0.50.0
- `go list -m -versions github.com/rs/cors` → latest is v1.11.1 (unchanged since 2024-08-29)
- `coder/websocket` v1.8.14 already pinned from Phase 3 (latest tag)

## Architecture Patterns

### Recommended Project Structure

```
internal/httpapi/
├── doc.go                  // package comment
├── server.go               // Server, Config, New, Handler, registerRoutes
├── credentials.go          // Credentials + LoadCredentials + Lookup + YAML struct
├── auth.go                 // authBasic + dummyHash + ctxKeyUser
├── middleware.go           // recover/log/CORS + statusCapturingWriter + clientIP
├── errors.go               // writeJSONError + shortcut constructors
├── events.go               // registryUpdatedEvent + newRegistryUpdatedEvent + newSnapshotEvent
├── handlers_registry.go    // 4 REST handlers
├── handlers_caps.go        // capabilities handler + WS upgrade + snapshot builder
├── health.go               // EXISTING Phase 1 — unchanged
│
├── server_test.go          // newTestServer helper + Server lifecycle
├── credentials_test.go     // LoadCredentials coverage
├── auth_test.go            // timing-safe + PII-free auth tests
├── middleware_test.go      // recover + log middleware
├── errors_test.go          // envelope shape
├── events_test.go          // marshal + snapshot shape
├── handlers_registry_test.go  // integration via httptest
├── handlers_caps_test.go   // capabilities + WS integration
├── health_test.go          // EXISTING Phase 1 — adapted to newTestServer
│
└── testdata/
    ├── credentials-valid.yaml
    ├── credentials-low-cost.yaml
    └── credentials-malformed.yaml
```

### Pattern 1: Go 1.22+ ServeMux Method Patterns

**What:** Register routes with the method as a prefix separated by a single space. The ServeMux distinguishes between "no pattern matches" (404) and "pattern matches but wrong method" (405) automatically.

**When to use:** Every route in this phase. No third-party router needed.

**Example** (verified against `~/sdk/go1.26.2/src/net/http/server.go:2690-2711`):

```go
// Source: internal/httpapi/health_test.go + Phase 1 placeholder
mux.HandleFunc("GET /health", handleHealth)
mux.Handle("POST /api/v1/registry", s.authBasic(http.HandlerFunc(s.handleRegistryUpsert)))
mux.Handle("DELETE /api/v1/registry/{appId}", s.authBasic(http.HandlerFunc(s.handleRegistryDelete)))
mux.HandleFunc("GET /api/v1/registry", s.handleRegistryList)
mux.HandleFunc("GET /api/v1/registry/{appId}", s.handleRegistryGet)
mux.HandleFunc("GET /api/v1/capabilities", s.handleCapabilities)
mux.HandleFunc("GET /api/v1/capabilities/ws", s.handleCapabilitiesWS)
```

**Go 1.26 source (server.go:2699-2710):** When a request's path matches some pattern but its method doesn't match any of those patterns' methods, the mux returns 405 with an `Allow` header listing allowed methods. This is empirically confirmed by the existing `TestHealth_RejectsWrongMethod` in Phase 1.

### Pattern 2: Timing-Safe Basic Auth (Dummy-Hash + Byte-Tuple ConstantTimeCompare)

**What:** Run `bcrypt.CompareHashAndPassword` on every request regardless of whether the user exists, then combine `(found, matches)` via `subtle.ConstantTimeCompare` on a 2-byte tuple.

**When to use:** The one and only `authBasic` middleware. This is the PITFALLS #8 anchor for Phase 4.

**Why the byte-tuple form:** Constant-time combination of two boolean results. A short-circuit `if found && bcryptMatches` would be branchy; the byte-tuple form is explicit and matches crypto-review expectations. `subtle.ConstantTimeCompare` on equal-length 2-byte slices is meaningful (the length-mismatch 0-return edge case doesn't apply).

**Example** (verified against `~/sdk/go1.26.2/src/crypto/subtle/constant_time.go:17-23` — "If the lengths of x and y do not match it returns 0 immediately"):

```go
// Source: CONTEXT.md §Timing-safe Basic Auth middleware (verbatim)
// Dummy hash precomputed at init
var dummyHash []byte
func init() {
    h, err := bcrypt.GenerateFromPassword([]byte("openburo:dummy:do-not-match"), 12)
    if err != nil { panic(fmt.Sprintf("httpapi: failed to generate dummy hash: %v", err)) }
    dummyHash = h
}

func (s *Server) authBasic(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        username, password, ok := r.BasicAuth()
        if !ok {
            writeUnauthorized(w)
            return
        }
        storedHash, found := s.creds.Lookup(username)
        if !found {
            storedHash = dummyHash
        }
        bcryptErr := bcrypt.CompareHashAndPassword(storedHash, []byte(password))
        bcryptMatches := bcryptErr == nil

        var foundByte, matchByte byte
        if found { foundByte = 1 }
        if bcryptMatches { matchByte = 1 }
        if subtle.ConstantTimeCompare([]byte{foundByte, matchByte}, []byte{1, 1}) != 1 {
            s.logger.Warn("httpapi: basic auth failed",
                "path", r.URL.Path,
                "method", r.Method,
                "remote", clientIP(r))
            writeUnauthorized(w)
            return
        }
        ctx := context.WithValue(r.Context(), ctxKeyUser, username)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

### Pattern 3: Middleware Chain (Recover OUTERMOST)

**What:** Wrap the mux with recover on the outside, then log, then CORS, with per-route auth inside the mux.

**Why recover is outermost:** If CORS or log panics, recover still catches it. If recover were innermost, a panic in the log middleware would crash the server.

**Example:**

```go
// Source: CONTEXT.md §Middleware chain order (locked)
func (s *Server) Handler() http.Handler {
    var h http.Handler = s.mux      // mux contains per-route authBasic
    h = s.corsMiddleware(h)         // 3. CORS wraps log+recover+mux
    h = s.logMiddleware(h)          // 2. log wraps recover+mux (skips /health)
    h = s.recoverMiddleware(h)      // 1. recover is OUTERMOST
    return h
}
// Request flow: recover → log → CORS → mux → (per-route auth) → handler
```

### Pattern 4: Mutation-Then-Broadcast (WS-05, WS-09)

**What:** The HTTP handler calls `store.Upsert`/`store.Delete` FIRST. Only after the mutation returns nil does the handler call `hub.Publish`. The registry package never imports the wshub package.

**Why:** Prevents the PITFALLS #1 ABBA deadlock scenario and the "phantom event" bug (broadcasting success for a mutation that failed persistence rollback).

**Example:**

```go
// inside handleRegistryUpsert
_, alreadyExisted := s.store.Get(manifest.ID)
if err := s.store.Upsert(manifest); err != nil {
    writeInternal(w, "failed to persist manifest")
    return
}
change := changeAdded
if alreadyExisted {
    change = changeUpdated
}
s.hub.Publish(newRegistryUpdatedEvent(manifest.ID, change))
// audit log + write response
```

**Architectural gate:**
```bash
~/sdk/go1.26.2/bin/go list -deps ./internal/registry | grep -E 'wshub|httpapi'
# Must produce empty output.
```

### Pattern 5: WebSocket Upgrade with Snapshot-Before-Subscribe

**What:** Call `websocket.Accept` with `OriginPatterns` from shared config. On success, write the full-state snapshot to the connection BEFORE handing off to `hub.Subscribe`. This eliminates the connect-then-fetch race.

**Why:** If the handler subscribes first and then writes the snapshot, a race window exists where another goroutine's `hub.Publish` can reach the new subscriber before the snapshot does — giving the client events before it has initial state.

**Example:**

```go
// Source: CONTEXT.md §WebSocket upgrade handler + corrected snapshot path
func (s *Server) handleCapabilitiesWS(w http.ResponseWriter, r *http.Request) {
    conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
        OriginPatterns: s.cfg.AllowedOrigins,
        // InsecureSkipVerify is NEVER set.
    })
    if err != nil {
        // websocket.Accept already wrote the rejection response (403).
        return
    }
    // WS-06: snapshot FIRST, then Subscribe.
    snapshot := s.buildFullStateSnapshot()
    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    err = conn.Write(ctx, websocket.MessageText, snapshot)
    cancel()
    if err != nil {
        conn.Close(websocket.StatusInternalError, "snapshot write failed")
        return
    }
    _ = s.hub.Subscribe(r.Context(), conn)
}
```

### Anti-Patterns to Avoid

- **Short-circuit `if user == cfg.AdminUser` anywhere in auth code.** Reveals user existence via timing. Use the dummy-hash pattern.
- **`slog.*("...", "request", r)` or logging `r.Header`.** Serializes the entire request including the `Authorization` header.
- **Calling `hub.Publish` BEFORE `store.Upsert` returns.** If the persist rolls back, subscribers see a phantom event.
- **Setting `InsecureSkipVerify: true` in `websocket.AcceptOptions`.** Disables CSRF protection. The phase has a grep gate against this literal string.
- **`AllowedOrigins: []string{"*"}` + `AllowCredentials: true` in `cors.Options`.** rs/cors will silently emit both headers but browsers reject them. Fail-fast in `httpapi.New`.
- **`recover` applied as innermost middleware.** A panic in log/CORS will crash the process. Recover must be OUTERMOST.
- **Broadcasting without first snapshotting `existed`.** Must know whether the Upsert was a create or update BEFORE the store call to emit the correct change value.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Basic Auth parsing | Custom base64 decode + split | `r.BasicAuth()` (stdlib) | Verified at `~/sdk/go1.26.2/src/net/http/request.go:973-979`. Handles case-insensitive `Basic ` prefix, base64 decode, colon split, empty-auth edge case. Returns `(user, pass, ok)` with `ok == false` on any failure mode. |
| Timing-safe comparison | Manual XOR loop | `crypto/subtle.ConstantTimeCompare` | Stdlib, intrinsified. Returns 1 if equal, 0 otherwise; returns 0 immediately on length mismatch (still leaks length — acceptable for this use case). |
| Password hashing | Argon2 / scrypt / SHA-256+salt | `golang.org/x/crypto/bcrypt` | Project lock. bcrypt's `CompareHashAndPassword` is itself constant-time w.r.t. password content. |
| Pattern matching for routes | Regex router | Go 1.22+ `ServeMux` method patterns | Stdlib. Handles method matching, wildcard params, 405 vs 404 distinction, trailing-slash redirects. |
| Request body size limits | Custom byte counter | `http.MaxBytesReader(w, r.Body, n)` | Stdlib. Returns `*http.MaxBytesError` on Read past limit (verified at `request.go:1193-1201`). Does NOT auto-write 413 — the handler must catch the decode error and emit 400. |
| CORS preflight / headers | Manual `Access-Control-*` headers | `github.com/rs/cors` v1.11.1 | Project lock. Handles preflight caching, Vary headers, method/header allow-lists. BUT: does NOT validate the `*`+credentials combination — that must be caught at construction time by the caller. |
| WebSocket origin check | Manual Origin header parsing | `websocket.AcceptOptions.OriginPatterns` | Coder library does this itself via `path.Match` glob patterns (verified at `accept.go:228-260`). |
| JSON error encoding | `w.Write([]byte(...))` | `json.NewEncoder(w).Encode(envelopeStruct)` | Stdlib. Handles escaping, content-length, trailing newline. |
| WebSocket upgrade | Raw hijack + handshake | `websocket.Accept(w, r, opts)` | Writes response on all error paths (verified at `accept.go:111,123,131,162`). |
| Goroutine-safe broadcast | Manual subscriber map | `wshub.Hub` from Phase 3 | Phase 3 ships the `Publish`/`Subscribe` contract. Handler only calls `Publish` and `Subscribe`. |
| Manifest validation | Custom field checks | `registry.Manifest.Validate()` | Phase 2 ships this. Handler just calls it and returns 400 on error. |
| MIME canonicalization | Custom parser | `registry.CanonicalizeMIME(s)` | Phase 2 ships this. Handler uses it to pre-validate the `?mimeType=` query param. |

**Key insight:** Every non-trivial primitive Phase 4 needs is already in stdlib, the 5-dep stack, or Phase 2/3. The ONLY custom code is the glue: middleware chain, handlers, error envelope, event marshalers, and the 2-byte-tuple timing-safe decision.

## Common Pitfalls

### Pitfall 1: rs/cors v1.11.1 Does NOT Panic on `*` + Credentials (CORRECTION)

**What goes wrong:** CONTEXT.md claims `cors.New(opts)` panics at construction time when `AllowedOrigins: ["*"]` and `AllowCredentials: true` are combined. **This is wrong.** rs/cors v1.11.1 silently accepts the combination and emits both `Access-Control-Allow-Origin: *` and `Access-Control-Allow-Credentials: true` headers on every response — which browsers then reject at the fetch layer (per the CORS spec).

**Why it happens:** The rs/cors maintainers never added this check. v1.11.1 (Aug 2024) is the latest tag and no subsequent release has added it. Verified by fetching `https://raw.githubusercontent.com/rs/cors/v1.11.1/cors.go` and searching for validation logic — the `New()` function processes `AllowedOrigins` with "if the special '*' value is present, turn the whole list into match-all" and handles `AllowCredentials` as an independent boolean flag, with no cross-validation.

**How to avoid:** The Server constructor (`httpapi.New`) must enforce the combination explicitly:

```go
// Source: CORRECTION to CONTEXT.md §CORS middleware
func New(logger *slog.Logger, store *registry.Store, hub *wshub.Hub, creds Credentials, cfg Config) (*Server, error) {
    // ... nil-guard panics ...
    if len(cfg.AllowedOrigins) == 0 {
        return nil, errors.New("httpapi: AllowedOrigins must not be empty")
    }
    for _, o := range cfg.AllowedOrigins {
        if o == "*" {
            return nil, errors.New(`httpapi: AllowedOrigins cannot contain "*" because AllowCredentials is true`)
        }
    }
    // ... rest of construction ...
}
```

**Constructor signature change:** CONTEXT.md spec has `New(...) *Server`. This MUST become `New(...) (*Server, error)` to surface the fail-fast validation. The Phase 5 compose-root will handle the error; tests construct via `newTestServer(t)` which can `require.NoError`. This is the ONE ergonomic cost of getting config-validation right.

**Warning signs:**
- `"*"` appearing anywhere in Phase 4 test config for `AllowedOrigins` (except in a dedicated "constructor rejects `*`" test)
- Server constructor signature returning just `*Server` without an error

**Phase to address:** plan 04-01 (Server + Config) — the validation lands with the constructor.

### Pitfall 2: `httptest.NewServer` Origin Header Is Same-Host by Default (Silent Bypass)

**What goes wrong:** `coder/websocket`'s `authenticateOrigin` allows the connection through unconditionally if `strings.EqualFold(r.Host, u.Host)` — i.e. if the browser's Origin header host equals the request's Host header. `websocket.Dial` by default sends no Origin header at all (empty Origin is always allowed). A naive origin-rejection test that forgets to set `DialOptions.HTTPHeader["Origin"]` will ALWAYS pass the origin check — masking the bug it's supposed to catch.

**Why it happens:** The default behavior is friendly for CLI tools and same-origin requests, but obscures what the test actually needs to prove.

**How to avoid (verified at `~/go/pkg/mod/github.com/coder/websocket@v1.8.14/accept.go:228-240`):**

```go
// Source: verified coder/websocket source
func TestServer_WebSocket_RejectsDisallowedOrigin(t *testing.T) {
    srv, cleanup := newTestServer(t) // cfg.AllowedOrigins = []string{"https://allowed.example"}
    defer cleanup()
    ts := httptest.NewServer(srv.Handler())
    defer ts.Close()

    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()

    // CRITICAL: must set an Origin header whose host differs from ts.URL's host.
    _, resp, err := websocket.Dial(ctx, ts.URL+"/api/v1/capabilities/ws", &websocket.DialOptions{
        HTTPHeader: http.Header{"Origin": []string{"https://evil.example"}},
    })
    require.Error(t, err)
    require.NotNil(t, resp)
    require.Equal(t, http.StatusForbidden, resp.StatusCode)
}
```

**Warning signs:**
- Origin-rejection test that dials without `HTTPHeader`
- Origin-rejection test that sets `Origin` to the `ts.URL` host (would auto-pass via same-host rule)

**Phase to address:** plan 04-05 (integration tests).

### Pitfall 3: `OriginPatterns` Use `path.Match`, Not Prefix / Regex / Exact

**What goes wrong:** Operators write `OriginPatterns: []string{"https://myapp.com"}` expecting a prefix match, or `"*.myapp.com"` expecting a suffix match. Subdomain patterns may or may not work depending on whether the pattern contains `://`.

**How it actually works (verified at `accept.go:243-264`):** Each pattern is matched via `path.Match(strings.ToLower(pattern), strings.ToLower(target))`:
- If the pattern contains `://`, it is matched against `scheme://host` (e.g. `https://app.example.com`)
- Otherwise it is matched against `host` only (e.g. `app.example.com`)
- `path.Match` is a glob: `*` matches any run of non-separator characters, `?` matches a single character, `[abc]` matches one char from a set
- Match is case-insensitive (both pattern and target lowercased before matching)

**Examples:**
- Pattern `"app.example.com"` matches Origin `https://app.example.com/` — because `u.Host == "app.example.com"` matches literal pattern
- Pattern `"https://app.example.com"` matches the same Origin — because pattern contains `://`, target becomes `"https://app.example.com"`
- Pattern `"*.example.com"` matches Origin `https://sub.example.com` — glob `*` matches `sub`
- Pattern `"https://*.example.com"` matches Origin `https://sub.example.com` — combined scheme + host glob
- Pattern `"*"` matches EVERYTHING (but the library prints a warning and recommends `InsecureSkipVerify` instead). STACK has explicit advice against this.

**Empty `OriginPatterns` (nil or `[]string{}`) + non-empty Origin header:** `authenticateOrigin` falls through the loop with no match and returns an error — 403 is written. Same-host requests still pass via the `strings.EqualFold(r.Host, u.Host)` check.

**Empty Origin header:** Always passes (line 230-232: `if origin == "" { return nil }`). This is why curl and stdlib `websocket.Dial` (without `HTTPHeader`) succeed.

**Bad pattern (invalid `path.Match` syntax):** Returns an error which surfaces as a 403 at accept time. The library logs `"websocket: %v"` via the standard `log` package (accept.go:120) and sets the error to a generic `"Forbidden"` status text to avoid leaking the bad pattern. Planner should NEVER rely on this loggingMaintain — instead, validate patterns at Server construction time (see Pitfall 4).

**Phase to address:** plan 04-05 (CORS + WS integration). Document these semantics in the `Config.AllowedOrigins` godoc.

### Pitfall 4: Bad `path.Match` Pattern in `OriginPatterns` Bypasses Everything at Runtime

**What goes wrong:** An operator writes `OriginPatterns: []string{"[badpattern"}` — an unclosed bracket. `path.Match` returns `path.ErrBadPattern`. Per accept.go:119-122, the library logs the error via stdlib `log.Printf` (NOT the injected `slog.Logger`) and rewrites the error to just `"Forbidden"`. The connection is rejected (safely), but the debugging experience is awful — the operator sees `403 Forbidden` with no indication why, and the diagnostic is written to a different log stream than the rest of the server.

**How to avoid:** Validate patterns at Server construction time by calling `path.Match(pattern, "probe")` for each pattern and surfacing any `path.ErrBadPattern` immediately:

```go
import "path"

// inside httpapi.New, after the "*" check:
for _, pattern := range cfg.AllowedOrigins {
    if _, err := path.Match(pattern, "probe.example"); err != nil {
        return nil, fmt.Errorf("httpapi: invalid AllowedOrigins pattern %q: %w", pattern, err)
    }
}
```

**Phase to address:** plan 04-01 (Server construction).

### Pitfall 5: `http.MaxBytesReader` Does NOT Auto-Write 413

**What goes wrong:** Developer assumes `http.MaxBytesReader` wraps the body and automatically returns 413 when the limit is exceeded. It does not. It returns `*http.MaxBytesError` from the underlying `Read()` call, which bubbles up through `json.Decoder.Decode` as a wrapped error. If the handler doesn't catch it explicitly, the response may be half-written and the client sees a confusing error.

**How it actually works (verified at `~/sdk/go1.26.2/src/net/http/request.go:1186-1201`):**

```go
func MaxBytesReader(w ResponseWriter, r io.ReadCloser, n int64) io.ReadCloser { /* ... */ }
type MaxBytesError struct { Limit int64 }
func (e *MaxBytesError) Error() string { return "http: request body too large" }
```

The wrapper "If possible, tells the ResponseWriter to close the connection after the limit has been reached" but does NOT write any status or body. The handler must catch the error.

**CONTEXT.md's pattern already handles this correctly:** the `json.Decoder.Decode` error from an oversize body becomes `"http: request body too large"`, and `writeBadRequest(w, "invalid JSON body", map[string]any{"reason": err.Error()})` surfaces it to the client as a 400. This is acceptable — the body is "invalid" from the handler's perspective because it cannot be decoded. Upgrading to a proper 413 would require:

```go
var mbErr *http.MaxBytesError
if errors.As(err, &mbErr) {
    writeJSONError(w, http.StatusRequestEntityTooLarge, "request body too large",
        map[string]any{"limit_bytes": mbErr.Limit})
    return
}
```

**Planner's call:** CONTEXT.md locks 400 for all decode errors. Keeping it 400 is fine (simpler, reference-impl). The 413 upgrade is optional polish — document the choice in the handler's comment.

**Phase to address:** plan 04-03 (registry handlers).

### Pitfall 6: `bcrypt.CompareHashAndPassword` Error Distinguishes Wrong Password vs Malformed Hash — But CONTEXT.md's `err == nil` Check Is Correct

**What goes wrong:** Developer thinks they need to distinguish "wrong password" from "malformed hash" and uses `errors.Is(err, bcrypt.ErrMismatchedHashAndPassword)` in the auth path. This is unnecessary and actually dangerous for timing: any decision branch based on error type can leak timing information about whether the hash is valid.

**How it actually works (verified at `~/go/pkg/mod/golang.org/x/crypto@v0.48.0/bcrypt/bcrypt.go:106-125`):**

```go
func CompareHashAndPassword(hashedPassword, password []byte) error {
    p, err := newFromHash(hashedPassword)
    if err != nil {
        return err  // Errors: ErrHashTooShort, HashVersionTooNewError, InvalidHashPrefixError, InvalidCostError
    }
    otherHash, err := bcrypt(password, p.cost, p.salt)
    if err != nil { return err }
    if subtle.ConstantTimeCompare(p.Hash(), otherP.Hash()) == 1 { return nil }
    return ErrMismatchedHashAndPassword
}
```

Possible returned errors: `ErrMismatchedHashAndPassword`, `ErrHashTooShort`, `HashVersionTooNewError`, `InvalidHashPrefixError`, `InvalidCostError`. CONTEXT.md's `bcryptMatches := bcryptErr == nil` correctly treats ALL error cases as "not authenticated" without branching on error type — which is both correct and timing-safe.

**Verification for CONTEXT.md's assumptions:**
- ✅ `bcrypt.GenerateFromPassword(password []byte, cost int) ([]byte, error)` — confirmed signature (bcrypt.go:95)
- ✅ `bcrypt.CompareHashAndPassword(hashedPassword, password []byte) error` — confirmed signature (bcrypt.go:108)
- ✅ `bcrypt.Cost(hashedPassword []byte) (int, error)` — confirmed signature (bcrypt.go:131); returns cost on success, error on malformed hash, does NOT verify a password
- ✅ `bcrypt.MinCost = 4`, `bcrypt.MaxCost = 31`, `bcrypt.DefaultCost = 10` — confirmed (bcrypt.go:21-25). CONTEXT.md's cost-12 choice is above the default and CPU-affordable.
- ✅ Cost > 31 returns `InvalidCostError`; cost < `MinCost` is silently bumped to `DefaultCost=10` (bcrypt.go:139-141). CONTEXT.md passes 12 hardcoded, so this edge is not triggered.
- ✅ `bcrypt.ErrMismatchedHashAndPassword` sentinel exists (bcrypt.go:29) — but CONTEXT.md's `err == nil` check is preferred.
- ✅ Passwords > 72 bytes: `GenerateFromPassword` returns `ErrPasswordTooLong` (bcrypt.go:96-98); `CompareHashAndPassword` silently accepts and compares only the first 72 bytes. The dummyHash generator at init uses a 27-byte string, so this is not a concern for Phase 4.

**Phase to address:** plan 04-02 (auth + credentials).

### Pitfall 7: `cors.New` Accepts Any Slice Including Empty — Allow-List Validation Must Live in Caller

**What goes wrong:** Empty `AllowedOrigins: []` passed to `cors.New` does not panic or error. It creates a CORS middleware that rejects every cross-origin request. Combined with an empty `OriginPatterns` passed to `websocket.Accept`, you get a server that refuses all browser clients silently — there's no startup signal of misconfig.

**How to avoid:** Validate non-empty at Server construction time (already locked in CONTEXT.md). See Pitfall 1 for the code snippet.

**Phase to address:** plan 04-01 (Server constructor).

### Pitfall 8: `r.PathValue("appId")` Returns `""` for Unknown Names — No Panic

**What goes wrong:** Developer misspells the wildcard name (`r.PathValue("app_id")` vs `"appId"`) and expects a panic or error. Instead they get an empty string, which silently propagates to `store.Get("")`, which returns `(Manifest{}, false)`, which surfaces as a 404 "manifest not found" — misleading because the real bug is the misspelling.

**How it actually works (verified at `~/sdk/go1.26.2/src/net/http/request.go:1469-1474`):**

```go
func (r *Request) PathValue(name string) string {
    if i := r.patIndex(name); i >= 0 { return r.matches[i] }
    return r.otherValues[name]  // empty-string default via map zero value
}
```

**How to avoid:**
- Keep the wildcard name `appId` consistent between the route pattern and the handler: one constant per wildcard in package scope (e.g. `const pathParamAppID = "appId"`), or just keep both strings in sync by code review.
- Validate non-empty at the top of the handler:
  ```go
  appID := r.PathValue("appId")
  if appID == "" {
      writeBadRequest(w, "missing appId in path", nil)
      return
  }
  ```
  This also defends against a theoretical future route that shadows `{appId}` with nothing.

**Phase to address:** plan 04-03 (registry handlers).

### Pitfall 9: `r.BasicAuth()` Returns `ok == false` Only for Missing or Malformed Header

**What goes wrong:** Developer thinks `ok == true` means the credentials are valid. It only means the header was present and parseable.

**How it actually works (verified at `~/sdk/go1.26.2/src/net/http/request.go:973-979`):**

```go
func (r *Request) BasicAuth() (username, password string, ok bool) {
    auth := r.Header.Get("Authorization")
    if auth == "" { return "", "", false }
    return parseBasicAuth(auth)
}
```

`parseBasicAuth` (same file) returns `ok == false` if:
- Prefix is not case-insensitive `"Basic "`
- Base64 decode fails
- Decoded content has no `:` separator

It returns `ok == true` with whatever username/password were parsed — including empty strings — if the structural check passes. A request with `Authorization: Basic Og==` (decoded: `":"`) returns `("", "", true)`.

**CONTEXT.md's pattern handles this correctly:** it runs bcrypt against whatever password was parsed, which will fail against any real hash, and also fails against the dummy hash. The auth decision is still timing-safe.

**Phase to address:** plan 04-02 (auth). The auth_test.go coverage should include an empty-password test case as a regression.

### Pitfall 10: `httptest.NewServer` + `websocket.Dial` — http:// URL Is Accepted Natively

**What goes wrong:** Developer writes `"ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/capabilities/ws"` because every WebSocket tutorial on the internet does the URL-scheme rewrite. The rewrite works, but it's unnecessary in `coder/websocket`.

**How it actually works (verified at `~/go/pkg/mod/github.com/coder/websocket@v1.8.14/dial.go:119`):**

```go
// URLs with http/https schemes will work and are interpreted as ws/wss.
func Dial(ctx context.Context, u string, opts *DialOptions) (*Conn, *http.Response, error)
```

**How to avoid:** Simplify the test to pass `ts.URL + "/api/v1/capabilities/ws"` directly.

```go
// Simplified form
conn, _, err := websocket.Dial(ctx, ts.URL+"/api/v1/capabilities/ws", nil)
```

**Planner's call:** CONTEXT.md shows the trimmed-prefix form. Both work. The simpler form is preferred but not mandatory — maintain consistency with Phase 3's leak test (which uses `srv.URL` directly per its research notes). Phase 4 plan should use the direct `ts.URL` form to match Phase 3 style.

**Phase to address:** plan 04-05 (integration tests).

### Pitfall 11: Flaky time-based tests (PITFALLS #16 inherited from Phase 3)

**What goes wrong:** `time.Sleep(100 * time.Millisecond)` + assert "WS event arrived". Passes locally, flakes on CI.

**How to avoid:** `require.Eventually(t, func() bool { ... }, 2*time.Second, 10*time.Millisecond)` — polling, not blocking. For WS event arrival tests, set a `context.WithTimeout` on the Read call and rely on the timeout to bound the wait.

**Phase to address:** plans 04-04 and 04-05. There is a grep gate: `! grep -n 'time\.Sleep' internal/httpapi/*_test.go`.

**Phase 3 lesson learned:** The literal substring `"time.Sleep"` in a COMMENT will trip the gate. When documenting the no-sleep rule in comments, phrase it as e.g. `// polling-based wait` not `// no time.Sleep`.

### Pitfall 12: `httptest.NewServer` leak (PITFALLS #17)

**What goes wrong:** Forgetting `defer ts.Close()` leaks an accept-loop goroutine per test, which the race detector then blames for unrelated warnings.

**How to avoid:** `defer ts.Close()` immediately on `httptest.NewServer(...)`. The Phase 4 `newTestServer(t)` helper should also manage `ts` if the helper creates one — alternatively, tests create their own `ts` and `defer ts.Close()` inline.

**Phase to address:** all integration tests in plans 04-03, 04-04, 04-05.

## Code Examples

Verified patterns from source. Every snippet below is either copied verbatim from a module source or constructed to match its documented API.

### Example 1: Go 1.22+ ServeMux 405 Behavior (from Go source)

```go
// Source: ~/sdk/go1.26.2/src/net/http/server.go:2699-2710
if n == nil {
    // We didn't find a match with the request method. To distinguish between
    // Not Found and Method Not Allowed, see if there is another pattern that
    // matches except for the method.
    allowedMethods := mux.matchingMethods(host, path)
    if len(allowedMethods) > 0 {
        return HandlerFunc(func(w ResponseWriter, r *Request) {
            w.Header().Set("Allow", strings.Join(allowedMethods, ", "))
            Error(w, StatusText(StatusMethodNotAllowed), StatusMethodNotAllowed)
        }), "", nil, nil
    }
    return NotFoundHandler(), "", nil, nil
}
```

**Empirical confirmation:** `internal/httpapi/health_test.go:35-47` already asserts 405 for POST/PUT/DELETE against `GET /health`.

### Example 2: `coder/websocket` Origin Check (verbatim from library)

```go
// Source: ~/go/pkg/mod/github.com/coder/websocket@v1.8.14/accept.go:228-264
func authenticateOrigin(r *http.Request, originHosts []string) error {
    origin := r.Header.Get("Origin")
    if origin == "" {
        return nil  // No Origin header → always allowed (CLI tools, curl)
    }
    u, err := url.Parse(origin)
    if err != nil {
        return fmt.Errorf("failed to parse Origin header %q: %w", origin, err)
    }
    if strings.EqualFold(r.Host, u.Host) {
        return nil  // Same host → always allowed (bypass OriginPatterns)
    }
    for _, hostPattern := range originHosts {
        target := u.Host
        if strings.Contains(hostPattern, "://") {
            target = u.Scheme + "://" + u.Host
        }
        matched, err := match(hostPattern, target)
        if err != nil {
            return fmt.Errorf("failed to parse path pattern %q: %w", hostPattern, err)
        }
        if matched { return nil }
    }
    if u.Host == "" {
        return fmt.Errorf("request Origin %q is not a valid URL with a host", origin)
    }
    return fmt.Errorf("request Origin %q is not authorized for Host %q", u.Host, r.Host)
}

func match(pattern, s string) (bool, error) {
    return path.Match(strings.ToLower(pattern), strings.ToLower(s))
}
```

### Example 3: `coder/websocket` Accept Error Path → 403 (verbatim from library)

```go
// Source: ~/go/pkg/mod/github.com/coder/websocket@v1.8.14/accept.go:115-126
opts = opts.cloneWithDefaults()
if !opts.InsecureSkipVerify {
    err = authenticateOrigin(r, opts.OriginPatterns)
    if err != nil {
        if errors.Is(err, path.ErrBadPattern) {
            log.Printf("websocket: %v", err)  // stdlib log, NOT injected slog
            err = errors.New(http.StatusText(http.StatusForbidden))
        }
        http.Error(w, err.Error(), http.StatusForbidden)
        return nil, err
    }
}
```

**Implication for handler:** when `websocket.Accept` returns an error, the response has already been written. The handler must just `return` without touching `w` further.

### Example 4: `bcrypt.CompareHashAndPassword` Internal (verbatim from library)

```go
// Source: ~/go/pkg/mod/golang.org/x/crypto@v0.48.0/bcrypt/bcrypt.go:106-125
func CompareHashAndPassword(hashedPassword, password []byte) error {
    p, err := newFromHash(hashedPassword)
    if err != nil { return err }
    otherHash, err := bcrypt(password, p.cost, p.salt)
    if err != nil { return err }
    otherP := &hashed{otherHash, p.salt, p.cost, p.major, p.minor}
    if subtle.ConstantTimeCompare(p.Hash(), otherP.Hash()) == 1 {
        return nil
    }
    return ErrMismatchedHashAndPassword
}
```

### Example 5: `subtle.ConstantTimeCompare` Godoc (verbatim from stdlib)

```go
// Source: ~/sdk/go1.26.2/src/crypto/subtle/constant_time.go:17-23
// ConstantTimeCompare returns 1 if the two slices, x and y, have equal contents
// and 0 otherwise. The time taken is a function of the length of the slices and
// is independent of the contents. If the lengths of x and y do not match it
// returns 0 immediately.
func ConstantTimeCompare(x, y []byte) int {
    return subtle.ConstantTimeCompare(x, y)
}
```

**Implication for CONTEXT.md's 2-byte tuple:** both slices are length 2, so the length-mismatch branch is not triggered. The comparison is meaningful.

### Example 6: rs/cors v1.11.1 `Options` Struct (verbatim from library)

```go
// Source: https://raw.githubusercontent.com/rs/cors/v1.11.1/cors.go (WebFetch 2026-04-10)
type Options struct {
    AllowedOrigins             []string
    AllowOriginFunc            func(origin string) bool
    AllowOriginRequestFunc     func(r *http.Request, origin string) bool
    AllowOriginVaryRequestFunc func(r *http.Request, origin string) (bool, []string)
    AllowedMethods             []string
    AllowedHeaders             []string
    ExposedHeaders             []string
    MaxAge                     int
    AllowCredentials           bool
    AllowPrivateNetwork        bool
    OptionsPassthrough         bool
    OptionsSuccessStatus       int
    Debug                      bool
    Logger                     Logger
}
```

**Canonical wrapper form** (also verified):
```go
c := cors.New(cors.Options{ /* ... */ })
wrapped := c.Handler(next)  // returns http.Handler
```

### Example 7: Full CORS Middleware (CONTEXT.md locked + correction)

```go
// Source: CONTEXT.md §CORS middleware, with rs/cors bug correction
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
    c := cors.New(cors.Options{
        AllowedOrigins:   s.cfg.AllowedOrigins, // validated non-empty and no "*" by New()
        AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodOptions},
        AllowedHeaders:   []string{"Authorization", "Content-Type"},
        AllowCredentials: true,
        MaxAge:           300,
    })
    return c.Handler(next)
}
```

### Example 8: `http.MaxBytesError` (verbatim from stdlib)

```go
// Source: ~/sdk/go1.26.2/src/net/http/request.go:1186-1201
func MaxBytesReader(w ResponseWriter, r io.ReadCloser, n int64) io.ReadCloser {
    if n < 0 { n = 0 }
    return &maxBytesReader{w: w, r: r, i: n, n: n}
}

// MaxBytesError is returned by [MaxBytesReader] when its read limit is exceeded.
type MaxBytesError struct {
    Limit int64
}

func (e *MaxBytesError) Error() string {
    return "http: request body too large"
}
```

### Example 9: WS Origin Rejection Test (corrected form)

```go
// Source: CONTEXT.md §WebSocket origin rejection test — CORRECTED to use ts.URL directly
func TestServer_WebSocket_RejectsDisallowedOrigin(t *testing.T) {
    srv, cleanup := newTestServer(t) // cfg.AllowedOrigins = []string{"https://allowed.example"}
    defer cleanup()
    ts := httptest.NewServer(srv.Handler())
    defer ts.Close()

    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()

    _, resp, err := websocket.Dial(ctx, ts.URL+"/api/v1/capabilities/ws", &websocket.DialOptions{
        HTTPHeader: http.Header{"Origin": []string{"https://evil.example"}},
    })
    require.Error(t, err)
    require.NotNil(t, resp)
    require.Equal(t, http.StatusForbidden, resp.StatusCode)
}
```

### Example 10: WS Integration Test (corrected form)

```go
// Source: CONTEXT.md §Integration test strategy — CORRECTED to use ts.URL directly + require.Eventually
func TestServer_Integration_WebSocketRoundTrip(t *testing.T) {
    srv, cleanup := newTestServer(t)
    defer cleanup()
    ts := httptest.NewServer(srv.Handler())
    defer ts.Close()

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    // Simplified URL: websocket.Dial accepts http:// natively
    conn, _, err := websocket.Dial(ctx, ts.URL+"/api/v1/capabilities/ws", nil)
    require.NoError(t, err)
    defer conn.Close(websocket.StatusNormalClosure, "")

    // 1. First message is SNAPSHOT
    _, msg, err := conn.Read(ctx)
    require.NoError(t, err)
    // ... unmarshal, assert event=REGISTRY_UPDATED, change=SNAPSHOT, capabilities==[]

    // 2. Upsert via REST client against ts.URL
    // ... authed POST ...

    // 3. Next WS message arrives within timeout (bounded by ctx)
    _, msg2, err := conn.Read(ctx)
    require.NoError(t, err)
    // ... assert change=ADDED, appId matches
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `gorilla/mux` + method filtering | Go 1.22+ `ServeMux` with method-prefixed patterns | Go 1.22 (Feb 2024) | No third-party router needed for this project. Empirically proven in Phase 1's `TestHealth_RejectsWrongMethod`. |
| Hand-rolled CORS via `Access-Control-*` headers | `rs/cors` v1.11.1 middleware | Stable since ~2016 | Handles preflight caching, Vary, method/header allow-lists. But: does NOT validate the `*`+credentials combination — caller responsibility (NEW finding this research). |
| `gorilla/websocket` | `coder/websocket` v1.8.14 | Gorilla archived 2022; coder adopted nhooyr Nov 2023 | Phase 3 already migrated. Phase 4 consumes the origin-check API which is strict-by-default. |
| Basic auth via string `==` | `crypto/subtle.ConstantTimeCompare` + unconditional bcrypt with dummy hash | OWASP consensus since ~2014 | Required to defend against timing-based username enumeration. |
| `http.Request` in logs | Explicit allow-list of fields (method, path, status, duration, remote) | Post-slog consensus (Go 1.21+) | Prevents accidental credential leaks. Phase 1 already locked the "skip `/health` + never log headers" pattern. |

**Deprecated / outdated:**
- `gorilla/websocket`: Archived. Do not import.
- `gorilla/mux`: Superseded by Go 1.22+ `ServeMux` for this use case. Only reach for it if you need mux-specific features (regex paths, URL reversal, sub-routers with per-router middleware).
- `InsecureSkipVerify: true` in `websocket.AcceptOptions`: Deprecated guidance. The library's own godoc says "prefer InsecureSkipVerify over `*` as an OriginPatterns entry" which is itself a warning that `*` is not recommended — use a real allow-list.
- `bcrypt.DefaultCost` (10): Below the project's cost ≥ 12 requirement. Never hardcode.

## Open Questions

1. **Should the `/api/v1/capabilities?mimeType=<malformed>` query return 400 or empty 200?**
   - What we know: Phase 2 lock: malformed filter.MimeType → empty result (not error). CONTEXT.md says "pre-validate via CanonicalizeMIME for 400 response."
   - What's unclear: Two reasonable interpretations — (a) pre-validate and 400 on malformed (matches REST convention "client sent garbage"), or (b) pass through and return empty 200 (consistent with Phase 2 Store behavior).
   - Recommendation: Go with CONTEXT.md's lock (pre-validate for 400) — it gives clients a clearer error signal and matches REST convention.

2. **Should the Server constructor return `(*Server, error)` instead of `*Server` to surface validation errors?**
   - What we know: CONTEXT.md spec has `func New(...) *Server`. But the rs/cors `*`+credentials correction forces validation that can fail on config input.
   - What's unclear: Panicking vs returning an error for config-validation failures.
   - Recommendation: Change to `(*Server, error)` and have the test helper `newTestServer(t)` call `require.NoError(t, err)`. Phase 5's compose-root will surface the error as a startup failure. This is the least-invasive correction to CONTEXT.md.

3. **Should `buildFullStateSnapshot` be a method on Server or a free function?**
   - What we know: CONTEXT.md shows it as a `(s *Server)` method but doesn't lock the signature.
   - What's unclear: Testability — if it's a method, tests need a full Server; if it's a free function taking `store` and returning `[]byte`, it's isolated.
   - Recommendation: Keep as a method on Server (simpler closure over `s.store`); unit-test via the integration test rather than in isolation.

4. **Does the auth log line on failure need a rate limiter to avoid log flooding under a password-spray attack?**
   - What we know: No rate limiter is locked in v1 (SEC-V2-02 deferred).
   - What's unclear: An attacker hammering `/api/v1/registry` with random credentials will generate one Warn per request, filling the log.
   - Recommendation: Accept the log flood for v1 — it's a signal the attack is happening. Document in PITFALLS: "under active auth flood, logs will grow fast; operator should use log-rotation + a fail2ban-style front-end per the 'Known Limitations' README section."

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | `testing` (stdlib) + `github.com/stretchr/testify/require` v1.11.1 |
| Config file | none — `go test` reads directly from `*_test.go` in each package |
| Quick run command | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -run <TestName> -race` |
| Full suite command | `~/sdk/go1.26.2/bin/go test ./internal/httpapi/... -race -count=1` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| AUTH-01 | `LoadCredentials` rejects missing file, malformed YAML, malformed bcrypt hash, cost < 12 | unit | `go test ./internal/httpapi -run TestLoadCredentials -race` | Wave 0 |
| AUTH-02 | POST /api/v1/registry without auth returns 401; with valid auth returns 201/200 | integration | `go test ./internal/httpapi -run TestHandleRegistryUpsert_Auth -race` | Wave 0 |
| AUTH-02 | DELETE /api/v1/registry/{id} without auth returns 401; with valid auth returns 204 | integration | `go test ./internal/httpapi -run TestHandleRegistryDelete_Auth -race` | Wave 0 |
| AUTH-03 | GET /api/v1/registry, /api/v1/registry/{id}, /api/v1/capabilities, /health return 200 without auth | integration | `go test ./internal/httpapi -run TestPublicRoutes -race` | Wave 0 |
| AUTH-04 | Timing-safe: bcrypt runs on every request regardless of username validity | unit | `go test ./internal/httpapi -run TestAuth_TimingSafe -race` | Wave 0 |
| AUTH-05 | No credential material appears in captured slog output across auth cycle | unit + integration | `go test ./internal/httpapi -run TestAuth_NoCredentialsInLogs -race` | Wave 0 |
| API-01 | POST: 201 create, 200 update, 400 invalid, 401 unauth | integration | `go test ./internal/httpapi -run TestHandleRegistryUpsert -race` | Wave 0 |
| API-02 | DELETE: 204, 404, 401 | integration | `go test ./internal/httpapi -run TestHandleRegistryDelete -race` | Wave 0 |
| API-03 | GET /api/v1/registry returns `{manifests, count}` | integration | `go test ./internal/httpapi -run TestHandleRegistryList -race` | Wave 0 |
| API-04 | GET /api/v1/registry/{appId} returns one or 404 | integration | `go test ./internal/httpapi -run TestHandleRegistryGet -race` | Wave 0 |
| API-05 | GET /api/v1/capabilities with `?action=` and `?mimeType=` filters | integration | `go test ./internal/httpapi -run TestHandleCapabilities -race` | Wave 0 |
| API-06 | Routes registered with Go 1.22 method patterns; wrong method returns 405 | integration | `go test ./internal/httpapi -run TestServer_MethodNotAllowed -race` | Wave 0 |
| API-07 | Middleware chain in order: recover → log → CORS → auth → handler | integration | `go test ./internal/httpapi -run TestMiddleware_ChainOrder -race` | Wave 0 |
| API-08 | Panic in handler caught by recover; 500 returned; server survives | integration | `go test ./internal/httpapi -run TestRecover_PanicCaught -race` | Wave 0 |
| API-09 | 4xx/5xx responses use `{error, details}` envelope | unit | `go test ./internal/httpapi -run TestErrors_Envelope -race` | Wave 0 |
| API-10 | Every response has `Content-Type: application/json` | integration | `go test ./internal/httpapi -run TestHandlers_ContentType -race` | Wave 0 |
| API-11 | Request bodies are closed for connection reuse | integration | `go test ./internal/httpapi -run TestHandlers_BodyClosed -race` | Wave 0 |
| WS-01 | `GET /api/v1/capabilities/ws` upgrades successfully | integration | `go test ./internal/httpapi -run TestHandleCapabilitiesWS_Upgrade -race` | Wave 0 |
| WS-05 | Upsert + delete broadcast `REGISTRY_UPDATED` with correct change | integration | `go test ./internal/httpapi -run TestServer_Integration_WebSocketRoundTrip -race` | Wave 0 |
| WS-06 | First WS message on connect is full SNAPSHOT | integration | `go test ./internal/httpapi -run TestHandleCapabilitiesWS_Snapshot -race` | Wave 0 |
| WS-08 | Disallowed Origin returns 403 | integration | `go test ./internal/httpapi -run TestServer_WebSocket_RejectsDisallowedOrigin -race` | Wave 0 |
| WS-09 | Architectural: registry package does not import wshub | gate | `~/sdk/go1.26.2/bin/go list -deps ./internal/registry \| grep -E 'wshub\|httpapi'` (must be empty) | n/a — gate |
| OPS-01 | CORS allow-list rejects `*`+credentials combination at Server construction | unit | `go test ./internal/httpapi -run TestServer_New_RejectsWildcardWithCredentials -race` | Wave 0 |
| OPS-06 | Writes emit structured audit log with `user`, `action`, `appId` | integration | `go test ./internal/httpapi -run TestServer_AuditLog -race` | Wave 0 |
| TEST-02 | REST round-trip + WS round-trip via `httptest.NewServer` | integration | `go test ./internal/httpapi -run TestServer_Integration -race` | Wave 0 |
| TEST-05 | WS origin-rejection test | integration | `go test ./internal/httpapi -run TestServer_WebSocket_RejectsDisallowedOrigin -race` | Wave 0 |
| TEST-06 | Dedicated credential PII test | unit | `go test ./internal/httpapi -run TestAuth_NoCredentialsInLogs -race` | Wave 0 |

### Sampling Rate

- **Per task commit:** `~/sdk/go1.26.2/bin/go test ./internal/httpapi -run <relevant test regex> -race`
- **Per wave merge:** `~/sdk/go1.26.2/bin/go test ./internal/httpapi/... -race -count=1`
- **Phase gate:** Full suite green + all architectural/grep gates pass before `/gsd:verify-work`

### Wave 0 Gaps

All Phase 4 test files are new. The following must be created in Wave 0 of each plan (Task 0 file creation):

- [ ] `internal/httpapi/server_test.go` — `newTestServer(t)` helper + `TestServer_New_*` constructor tests (plan 04-01)
- [ ] `internal/httpapi/middleware_test.go` — `TestMiddleware_ChainOrder`, `TestRecover_PanicCaught`, `TestLogMiddleware_SkipsHealth` (plan 04-01)
- [ ] `internal/httpapi/errors_test.go` — `TestErrors_Envelope`, `TestWriteUnauthorized_Header` (plan 04-01)
- [ ] `internal/httpapi/credentials_test.go` — `TestLoadCredentials_*` (plan 04-02)
- [ ] `internal/httpapi/auth_test.go` — `TestAuth_TimingSafe`, `TestAuth_NoCredentialsInLogs`, `TestAuth_EmptyHeader`, `TestAuth_WrongPassword`, `TestAuth_UnknownUser` (plan 04-02)
- [ ] `internal/httpapi/testdata/credentials-valid.yaml` (plan 04-02)
- [ ] `internal/httpapi/testdata/credentials-low-cost.yaml` (plan 04-02)
- [ ] `internal/httpapi/testdata/credentials-malformed.yaml` (plan 04-02)
- [ ] `internal/httpapi/events_test.go` — `TestNewRegistryUpdatedEvent`, `TestNewSnapshotEvent` (plan 04-03)
- [ ] `internal/httpapi/handlers_registry_test.go` — `TestHandleRegistryUpsert_*`, `TestHandleRegistryDelete_*`, `TestHandleRegistryList`, `TestHandleRegistryGet` (plan 04-03)
- [ ] `internal/httpapi/handlers_caps_test.go` — `TestHandleCapabilities`, `TestHandleCapabilitiesWS_Upgrade`, `TestHandleCapabilitiesWS_Snapshot` (plan 04-04)
- [ ] `internal/httpapi/integration_test.go` — `TestServer_Integration_RESTRoundTrip`, `TestServer_Integration_WebSocketRoundTrip`, `TestServer_WebSocket_RejectsDisallowedOrigin` (plan 04-05)
- [ ] Adapt existing `internal/httpapi/health_test.go` to call `newTestServer(t)` after 04-01 constructor expansion

**Framework install:** already present (testify v1.11.1 direct dep from Phase 1). No new framework install needed in Wave 0.

### Architectural / Grep Gates (non-test verifications)

These run as part of the per-plan and per-phase verification, not as `go test` runs. They produce no output if they pass; any output is a failure.

| Gate | Command | Enforces |
|------|---------|----------|
| Architectural isolation — registry never imports wshub/httpapi | `~/sdk/go1.26.2/bin/go list -deps ./internal/registry \| grep -E 'wshub\|httpapi'` | WS-09 |
| Architectural isolation — wshub never imports registry/httpapi | `~/sdk/go1.26.2/bin/go list -deps ./internal/wshub \| grep -E 'registry\|httpapi'` | Phase 3 gate (continues in Phase 4) |
| No `slog.Default()` in internal/ | `! grep -rE 'slog\.Default\(' internal/httpapi/*.go \| grep -v _test.go` | Phase 1 lock |
| No `time.Sleep` in httpapi tests | `! grep -n 'time\.Sleep' internal/httpapi/*_test.go` | PITFALLS #16 (inherited from Phase 3) |
| No `InsecureSkipVerify` in httpapi (production code) | `! grep -rn 'InsecureSkipVerify' internal/httpapi/*.go \| grep -v _test.go` | PITFALLS #7 (WS-08) |
| No `internal/config` import in httpapi | `! grep -rn '"github.com/[^"]*/internal/config"' internal/httpapi/*.go` | CONTEXT.md §"Package layout" |
| `go vet` clean | `~/sdk/go1.26.2/bin/go vet ./internal/httpapi/...` | standard Go hygiene |
| `gofmt` clean | `! ~/sdk/go1.26.2/bin/gofmt -l internal/httpapi/ \| grep .` | standard Go hygiene |

## Sources

### Primary (HIGH confidence — verified against source)

- `~/sdk/go1.26.2/src/net/http/server.go:2690-2711` — ServeMux 405 Method Not Allowed path via `matchingMethods` + `Allow` header
- `~/sdk/go1.26.2/src/net/http/pattern.go:210-240` — pattern precedence rules ("more specific wins")
- `~/sdk/go1.26.2/src/net/http/request.go:973-998` — `Request.BasicAuth` + `parseBasicAuth` implementation
- `~/sdk/go1.26.2/src/net/http/request.go:1186-1201` — `MaxBytesReader` + `MaxBytesError` type and behavior
- `~/sdk/go1.26.2/src/net/http/request.go:1469-1487` — `PathValue` + `SetPathValue` (empty-string default for unknown names)
- `~/sdk/go1.26.2/src/crypto/subtle/constant_time.go:17-23` — `ConstantTimeCompare` signature and length-mismatch semantics
- `~/go/pkg/mod/golang.org/x/crypto@v0.48.0/bcrypt/bcrypt.go:21-125` — `MinCost/MaxCost/DefaultCost` constants, `GenerateFromPassword`, `CompareHashAndPassword`, `Cost`, `ErrMismatchedHashAndPassword`, `ErrPasswordTooLong`
- `~/go/pkg/mod/github.com/coder/websocket@v1.8.14/accept.go:24-53,96-126,228-264` — `AcceptOptions`, `Accept` error-response path, `authenticateOrigin`, `path.Match` glob semantics
- `~/go/pkg/mod/github.com/coder/websocket@v1.8.14/dial.go:23-45,107-122` — `DialOptions.HTTPHeader`, `Dial` accepting http/https URLs natively
- `~/go/pkg/mod/github.com/coder/websocket@v1.8.14/close.go:78-84` — `CloseStatus(err) StatusCode`
- `internal/httpapi/health_test.go:35-47` — empirical confirmation of Go 1.22 mux 405 behavior
- `~/sdk/go1.26.2/bin/go list -m -versions golang.org/x/crypto` — latest version confirmed v0.50.0 (2026)
- `~/sdk/go1.26.2/bin/go list -m -versions github.com/rs/cors` — latest version confirmed v1.11.1

### Secondary (MEDIUM confidence — verified via web fetch)

- [https://raw.githubusercontent.com/rs/cors/v1.11.1/cors.go](https://raw.githubusercontent.com/rs/cors/v1.11.1/cors.go) — `Options` struct field names, `New()` behavior, runtime handling of `*`+credentials (DOES NOT panic or validate — **correction to CONTEXT.md**)
- [https://github.com/rs/cors/tags](https://github.com/rs/cors/tags) — v1.11.1 (2024-08-29) confirmed as still latest
- [https://pkg.go.dev/net/http](https://pkg.go.dev/net/http) — `ServeMux`, `PathValue`, `MaxBytesReader`, `MaxBytesError` reference docs

### Project inputs

- `.planning/phases/04-http-api/04-CONTEXT.md` — all locked decisions; research verified each library claim against source
- `.planning/REQUIREMENTS.md` — 26 Phase 4 REQ-IDs
- `.planning/STATE.md` — prior phase completion status + critical research flags
- `.planning/research/STACK.md` — library version locks
- `.planning/research/PITFALLS.md` §1 (broadcast path), §7 (WS origin), §8 (CORS), §9 (timing auth), §16 (flaky time tests)
- `.planning/phases/03-websocket-hub/03-RESEARCH.md` — pre-verified `coder/websocket` v1.8.14 API surface
- `.planning/phases/02-registry-core/02-CONTEXT.md` — Store API surface
- `.planning/phases/03-websocket-hub/03-CONTEXT.md` — Hub API surface
- `internal/httpapi/server.go`, `health.go`, `health_test.go` — Phase 1 baseline
- `internal/registry/store.go` — Store method signatures
- `internal/wshub/hub.go`, `subscribe.go` — Hub method signatures
- `.planning/config.json` — `workflow.nyquist_validation = true` (enables Validation Architecture section)

## Metadata

**Confidence breakdown:**

| Area | Level | Reason |
|------|-------|--------|
| Standard stack | HIGH | All versions verified via `go list -m -versions`; all API signatures verified against source in `~/go/pkg/mod` and `~/sdk/go1.26.2/src` |
| Architecture patterns | HIGH | All patterns traced to specific file:line in stdlib or library source; no uncited claims |
| Timing-safe auth | HIGH | Pattern verified against `subtle.ConstantTimeCompare` godoc + `bcrypt.CompareHashAndPassword` internal flow; CONTEXT.md byte-tuple form is defensible |
| CORS middleware | HIGH | **Correction flagged** — rs/cors does NOT panic on `*`+credentials; Server constructor must validate. Verified by fetching v1.11.1 source. |
| WS origin semantics | HIGH | Verified against accept.go:228-264 including same-host bypass, empty-Origin allow, `path.Match` glob syntax |
| ServeMux 405 behavior | HIGH | Verified against pattern.go and server.go:2690-2711 + empirically confirmed by `TestHealth_RejectsWrongMethod` |
| MaxBytesReader behavior | HIGH | Verified against request.go:1186-1201 — does NOT auto-write 413 |
| Pitfalls | HIGH | 12 pitfalls identified with file:line citations; 6 of them are corrections/sharpenings of CONTEXT.md assumptions |
| Validation Architecture | HIGH | Every Phase 4 REQ-ID mapped to a specific test name + command |

**Research date:** 2026-04-10
**Valid until:** 2026-05-10 (30 days for stable stdlib + mature deps; rs/cors v1.11.1 has been stable since 2024-08-29 so unlikely to change)
