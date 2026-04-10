# Roadmap: OpenBuro Server

## Overview

OpenBuro Server is a Go 1.26 single-binary capability broker: the reference implementation of the OpenBuro platform layer. The journey from empty directory to shipping binary flows through five phases: first the foundation (scaffolding, config, logging, health), then the two architecturally-independent domain packages built in parallel (Registry with atomic persistence and symmetric MIME matching; WebSocket Hub with leak-free goroutine lifecycle), then the HTTP API layer that is the sole wiring point between them (enforcing the unidirectional dependency graph that prevents the registry-hub ABBA deadlock), and finally the compose-root with two-phase graceful shutdown plus the reference-implementation polish (examples, README, audit logs) that elevates the binary from "works" to "worth reading."

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

**Parallelization:** Phases 2 and 3 can execute in parallel — their dependency graphs are disjoint (Registry imports nothing from wshub; wshub imports nothing from registry).

- [x] **Phase 1: Foundation** - Scaffolding, config/credentials loading, slog, /health, CI
- [ ] **Phase 2: Registry Core** - Manifest domain, Store with atomic persistence, symmetric MIME matching *(parallel with Phase 3)*
- [ ] **Phase 3: WebSocket Hub** - Leak-free hub with drop-slow-consumer fan-out and ping keepalive *(parallel with Phase 2)*
- [ ] **Phase 4: HTTP API** - Routing, middleware chain, timing-safe auth, CORS, WS upgrade, mutation-then-broadcast wiring
- [ ] **Phase 5: Wiring, Shutdown & Polish** - Compose-root, two-phase shutdown, TLS, examples, README, audit logs

## Phase Details

### Phase 1: Foundation
**Goal**: A buildable, CI-green project skeleton with configuration, logging, and a working /health endpoint — the minimal end-to-end proof of life before any domain code is written.
**Depends on**: Nothing (first phase)
**Requirements**: FOUND-01, FOUND-02, FOUND-03, FOUND-04, FOUND-05, FOUND-06, FOUND-07, TEST-07
**Success Criteria** (what must be TRUE):
  1. `go build ./...` on Go 1.26 succeeds with the five pinned direct dependencies in `go.mod` and the idiomatic layout (`cmd/server/`, `internal/{config,registry,httpapi,wshub}/`) is in place
  2. The binary starts when given `config.yaml` and `credentials.yaml`, logs a structured startup banner with version/listen-addr/TLS/ping-interval/registry-path, and responds `200 OK` to `GET /health` without auth
  3. A developer can copy `config.example.yaml` and `credentials.example.yaml` from the repo root, run the binary, and reach `/health` in under a minute
  4. CI runs `go test ./... -race`, `go vet`, and `gofmt -l` checks and all pass on the skeleton
**Plans**: 3 plans

Plans:
- [x] 01-01-scaffold-deps-ci-PLAN.md — Initialize Go module, four-package internal/ layout, Makefile, .gitignore, GitHub Actions CI
- [x] 01-02-config-examples-PLAN.md — internal/config package (Config types, Load, validate) + testdata fixtures + config/credentials example YAMLs
- [x] 01-03-slog-health-banner-PLAN.md — internal/httpapi Server + /health handler + cmd/server/main.go compose-root with injected slog logger and 10-key startup banner

### Phase 2: Registry Core
**Goal**: A pure domain package (`internal/registry`) owning manifest validation, thread-safe in-memory state, atomic file persistence, and symmetric MIME matching — independently testable with zero transport dependencies.
**Depends on**: Phase 1
**Parallel with**: Phase 3 (disjoint dependency graphs; registry imports nothing from wshub)
**Requirements**: REG-01, REG-02, REG-03, REG-04, REG-05, REG-06, REG-07, REG-08, CAP-01, CAP-02, CAP-03, CAP-04, CAP-05, PERS-01, PERS-02, PERS-03, PERS-04, PERS-05, TEST-01, TEST-04
**Critical research flags honored in first commit:**
- Symmetric 3×3 wildcard MIME matching with exhaustive table-driven test (PITFALLS #2)
- Atomic persistence (temp-in-same-dir + `tmp.Sync()` + `os.Rename` + dir fsync) with in-memory rollback on persist failure (PITFALLS #5)
**Success Criteria** (what must be TRUE):
  1. `Store.Upsert`, `Store.Delete`, `Store.Get`, `Store.List`, and `Store.Capabilities(filter)` behave per contract under concurrent access, proven by `go test -race` on the registry package
  2. A `mimeType` query matches symmetrically across all nine wildcard combinations (`exact`/`type/*`/`*/*` on both sides) and malformed MIME strings are rejected — verified by an exhaustive table-driven test
  3. If the disk write of `registry.json` fails mid-mutation, the in-memory `Store` is observably identical to its pre-mutation state and the mutation returns an error (proven by a test against an unwritable directory)
  4. Restarting the process against an existing `registry.json` yields the same `Store.List` output; a missing file yields an empty registry; a corrupted file fails fast with a clear error
  5. `Store.List` and `Store.Capabilities` return results in a deterministic order so API responses and tests are stable
**Plans**: 3 plans

Plans:
- [x] 02-01-manifest-mime-PLAN.md — Manifest domain types + Validate + canonicalizeMIME + symmetric 3×3 mimeMatch with exhaustive tests
- [x] 02-02-store-persist-PLAN.md — Store mutations (Upsert/Delete/Get/List) + atomic persistence + in-memory rollback + load-at-startup
- [ ] 02-03-capabilities-PLAN.md — Store.Capabilities filter+sort (OR mimeMatch, lower(appName)/appId/action/path tiebreakers)

### Phase 3: WebSocket Hub
**Goal**: A leak-free, byte-oriented pub/sub hub (`internal/wshub`) implementing the `coder/websocket` canonical chat pattern with non-blocking fan-out, drop-slow-consumer semantics, and periodic ping keepalive — independently testable with `httptest.NewServer` and a local client.
**Depends on**: Phase 1
**Parallel with**: Phase 2 (disjoint dependency graphs; wshub imports nothing from registry)
**Requirements**: WS-02, WS-03, WS-04, WS-07, WS-10
**Critical research flags honored in first commit:**
- `conn.CloseRead(ctx)` + `defer hub.removeSubscriber` in `Subscribe` — prevents goroutine leak on disconnect (PITFALLS #3)
- Per-subscriber buffered channel with non-blocking `select`-default fan-out — prevents slow-client stall (PITFALLS #4)
- Hub accepts `[]byte` events only — never imports registry types, making the ABBA deadlock (PITFALLS #1) structurally impossible from this side
**Success Criteria** (what must be TRUE):
  1. `hub.Publish([]byte)` delivers the payload to every active subscriber without blocking, and a subscriber whose buffered channel is full is dropped via `closeSlow` instead of stalling the publisher
  2. A goroutine-leak integration test running 1000 connect-then-disconnect cycles against an `httptest.NewServer`-backed hub ends with `runtime.NumGoroutine()` flat (±epsilon)
  3. A subscriber that never drains its channel is kicked without blocking any other subscriber or the publisher
  4. Active subscribers receive periodic ping frames at the configured interval (default 30s) and the connection stays open across idle periods
  5. `go list -deps ./internal/wshub` shows zero imports of `internal/registry` — the package is architecturally independent by construction
**Plans**: 3 plans

Plans:
- [ ] 03-01-hub-subscribe-PLAN.md — Hub + Options + subscriber struct + Subscribe writer loop with CloseRead + defer removeSubscriber (stub Publish/Close)
- [ ] 03-02-publish-close-PLAN.md — Real Publish (non-blocking fan-out + drop-slow-consumer Warn log) + idempotent Close(StatusGoingAway) + ping keepalive in Subscribe tick.C branch
- [ ] 03-03-leak-test-logging-PLAN.md — Logging contract tests (Warn-on-drop, Info-on-close, no PII) + full architectural gate sweep (WS-10 acceptance + no-registry/httpapi + no-slog.Default + no-time.Sleep)

### Phase 4: HTTP API
**Goal**: The transport layer (`internal/httpapi`) that wires Registry and Hub together behind the OpenBuro HTTP+WebSocket contract — the sole package where both domains meet, enforcing the unidirectional dependency graph and the mutation-then-broadcast rule that prevents the registry↔hub ABBA deadlock.
**Depends on**: Phase 2, Phase 3
**Requirements**: AUTH-01, AUTH-02, AUTH-03, AUTH-04, AUTH-05, API-01, API-02, API-03, API-04, API-05, API-06, API-07, API-08, API-09, API-10, API-11, WS-01, WS-05, WS-06, WS-08, WS-09, OPS-01, OPS-06, TEST-02, TEST-05, TEST-06
**Critical research flags honored in first commit:**
- Timing-safe Basic Auth: `subtle.ConstantTimeCompare` on username, bcrypt always runs (dummy hash fallback on missing user), no early return on mismatch (PITFALLS #8)
- bcrypt cost ≥ 12 verified at credential load time via `bcrypt.Cost(hash)`
- `coder/websocket` `AcceptOptions.OriginPatterns` sourced from the same config-driven allow-list as `rs/cors`; the string `InsecureSkipVerify` never appears in production code (PITFALLS #7)
- CORS: explicit origin list when credentials are involved, never `*` + credentials (PITFALLS #9)
- Mutation-then-broadcast in the handler layer (not inside the registry); `internal/registry` never imports `internal/wshub`, enforced by `go list -deps` check (PITFALLS #1)
- Full-state snapshot broadcast to new WebSocket subscribers on connect, before any subsequent event (eliminates connect-then-fetch race)
**Success Criteria** (what must be TRUE):
  1. A client can execute the full REST round-trip through `httptest.NewServer`: `POST /api/v1/registry` (201 on create, 200 on update, 400 on invalid, 401 without auth) → `GET /api/v1/registry` (list) → `GET /api/v1/registry/{appId}` (single or 404) → `GET /api/v1/capabilities?action=&mimeType=` (filtered) → `DELETE /api/v1/registry/{appId}` (204 or 404 or 401), and every JSON response carries `Content-Type: application/json` with a consistent error envelope on 4xx/5xx
  2. A WebSocket client connecting to `GET /api/v1/capabilities/ws` receives a full-state `REGISTRY_UPDATED` snapshot as its first message, then receives a fresh `REGISTRY_UPDATED` event within milliseconds of any subsequent upsert or delete
  3. Basic Auth on write routes is timing-safe: a dedicated test proves no observable timing signal between "wrong username" and "wrong password," and bcrypt is verified to run on the unauthenticated path
  4. A WebSocket handshake with a disallowed `Origin` header is rejected with 403; a handler panic is caught by the recover middleware, logged, returned as 500, and the server stays alive
  5. A dedicated test captures slog output across a failed-auth scenario and asserts no credential material (Authorization header, username, password) ever appears in any log line, and write operations emit a structured audit log line with `user`, `action`, `appId` fields
  6. `go list -deps ./internal/registry` contains no reference to `internal/wshub` — the unidirectional dependency graph is enforced by construction
**Plans**: 5 plans

Plans:
- [ ] 04-01-server-middleware-PLAN.md — Server struct expansion, Config type, validated New (*Server, error), middleware chain (recover -> log -> cors -> mux), error envelope helpers, 501 route stubs (API-06..11)
- [ ] 04-02-auth-credentials-PLAN.md — Credentials type + LoadCredentials (bcrypt cost >= 12), timing-safe authBasic middleware (dummyHash + subtle.ConstantTimeCompare), PII guard (AUTH-01..05, TEST-06)
- [ ] 04-03-registry-handlers-PLAN.md — REST handlers (upsert/delete/list/get) + events.go + mutation-then-broadcast + audit log (API-01..04, WS-05, WS-09, OPS-06)
- [ ] 04-04-capabilities-ws-PLAN.md — handleCapabilities + handleCapabilitiesWS + buildFullStateSnapshot + WS upgrade with snapshot-on-connect (API-05, WS-01, WS-06)
- [ ] 04-05-cors-integration-tests-PLAN.md — rs/cors wiring + REST round-trip + WS round-trip + WS origin rejection + full Phase 4 gate sweep (OPS-01, WS-08, TEST-02, TEST-05, TEST-06)

### Phase 5: Wiring, Shutdown & Polish
**Goal**: The compose-root (`cmd/server/main.go`) wiring all components under ~100 lines, a two-phase graceful shutdown that cleanly closes WebSocket connections, optional TLS, and the reference-implementation polish (examples, README, race-clean CI) that makes the binary worth reading as documentation of the OpenBuro pattern.
**Depends on**: Phase 4
**Requirements**: OPS-02, OPS-03, OPS-04, OPS-05, TEST-03
**Critical research flags honored in first commit:**
- Two-phase shutdown: `httpSrv.Shutdown(ctx)` first (with ~15s budget), then `hub.Close()` which calls `conn.Close(websocket.StatusGoingAway, ...)` on every subscriber — because `http.Server.Shutdown` explicitly does NOT close hijacked WebSocket connections (PITFALLS #6)
**Success Criteria** (what must be TRUE):
  1. `cmd/server/main.go` is ≤100 lines, wires `config.Load → registry.NewStore → wshub.New → httpapi.New → http.Server` in compose-root style, and parses a `-config` flag
  2. Sending `SIGTERM` or `SIGINT` to a running server with active WebSocket subscribers causes `http.Server.Shutdown` to drain in-flight HTTP requests, then `hub.Close()` sends a `StatusGoingAway` close frame to every subscriber; the process exits cleanly and subscribers observe a clean close rather than a TCP reset
  3. When `server.tls.enabled = true` in `config.yaml`, the binary serves HTTPS via `ListenAndServeTLS` using the configured cert and key paths; when disabled, it serves plain HTTP
  4. `go test ./... -race` across the whole module passes cleanly with no warnings
  5. A new user can follow the README from a clean clone to a running server and a successful upsert+WebSocket-event round-trip in under five minutes
**Plans**: TBD

Plans:
- [ ] 05-01: TBD (main.go compose-root + signal-aware context)
- [ ] 05-02: TBD (two-phase graceful shutdown + optional TLS)
- [ ] 05-03: TBD (README, example manifests, race-clean verification)

## Progress

**Execution Order:**
Phases execute in numeric order with Phase 2 and Phase 3 eligible for parallel execution: 1 → (2 || 3) → 4 → 5

**Parallel Groups:**
- Group A: Phase 2 (Registry Core) and Phase 3 (WebSocket Hub) — disjoint dependency graphs, can be built concurrently after Phase 1 lands

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Foundation | 3/3 | Complete | 2026-04-10 |
| 2. Registry Core | 0/3 | Not started | - |
| 3. WebSocket Hub | 0/3 | Not started | - |
| 4. HTTP API | 0/5 | Not started | - |
| 5. Wiring, Shutdown & Polish | 0/3 | Not started | - |

---
*Roadmap created: 2026-04-09*
