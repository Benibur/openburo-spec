# Project Research Summary

**Project:** OpenBuro Server
**Domain:** Go HTTP REST + WebSocket capability broker (reference implementation of the OpenBuro platform layer)
**Researched:** 2026-04-09
**Confidence:** HIGH

## Executive Summary

OpenBuro Server is a small, single-binary Go reference implementation of a capability broker — the "Plateforme" layer of the OpenBuro ecosystem. It belongs to the same family as Android's `PackageManager`/`IntentResolver`, Cozy Stack `/intents`, and XDG Desktop Portal: apps register manifests declaring `(action, mimeType[])` capabilities, clients query the broker to discover who can fulfill an intent, and the broker pushes change notifications over WebSocket. The reference-implementation framing is load-bearing for every decision — **clarity beats completeness**, and every feature must either be essential to the broker pattern or illustrate a protocol decision downstream implementers need to understand.

Research converges on a dependency-minimal, stdlib-first Go 1.26 stack: `net/http` ServeMux (post-1.22 method matching removes the need for chi), `coder/websocket` (the canonical successor to the archived `gorilla/websocket`), `log/slog`, `encoding/json`, `sync.RWMutex`, plus four small direct deps — `coder/websocket`, `go.yaml.in/yaml/v3`, `golang.org/x/crypto/bcrypt`, `rs/cors`, and `testify/require` — for a total of **five direct dependencies**. The architecture is four `internal/` packages (`config`, `registry`, `httpapi`, `wshub`) with a strict unidirectional dependency graph: `registry` knows nothing about the hub, `wshub` knows nothing about registry types, and the HTTP handler layer is the sole wiring point between them.

The key risks are all known and concentrated: (1) **MIME wildcard matching** is the single biggest correctness concern — it must be symmetric (capability `image/*` matches query `image/png` AND capability `image/png` matches query `image/*`), and Cozy Stack's own resolver has a public reputation for getting this wrong. (2) The **registry↔hub dependency direction** is the architectural trap — any cross-package lock acquisition produces an ABBA deadlock that `go test -race` cannot catch; the mitigation is that only the HTTP handler layer talks to both. (3) **WebSocket goroutine leaks** from forgetting `coder/websocket`'s `CloseRead` pattern and the **two-phase graceful shutdown** (`http.Server.Shutdown` does NOT close hijacked WebSocket connections) are non-optional. (4) **Atomic persistence** (temp+fsync+rename+dir-fsync with in-memory rollback on failure) must land in the first registry commit, not as a later hardening pass. Every one of these has a well-documented fix, and the research documents each in detail.

## Key Findings

### Recommended Stack

A dependency-minimal, idiomatic Go 1.26 server. Stdlib where possible; one dependency each for the things stdlib doesn't cover well (WebSocket, YAML, bcrypt, CORS, test ergonomics). Total direct deps: **5**. No framework, no viper, no ORM, no logger library. See [STACK.md](./STACK.md) for full rationale, version pins, and alternatives-considered.

**Core technologies:**
- **Go 1.26** (latest stable, 2026-02-10 release, 1.26.2 as of 2026-04-07) — project constraint is "latest stable"; 1.22+ is the floor for `ServeMux` method patterns.
- **`net/http` + `http.ServeMux`** — since Go 1.22, ServeMux supports `POST /api/v1/registry` and `GET /api/v1/registry/{appId}` natively. For ~5 routes and 2 middleware concerns, stdlib is sufficient and strictly clearer for a reference impl than chi/gin/echo.
- **`log/slog`** (stdlib) — hard PROJECT.md constraint; JSON handler in prod, text in dev.
- **`encoding/json` + `sync.RWMutex`** (stdlib) — in-memory registry with whole-file atomic write to `registry.json`.
- **`github.com/coder/websocket` v1.8.x** — the canonical post-2025 choice. `gorilla/websocket` was archived; `nhooyr/websocket` was adopted by Coder. Context-aware, concurrent-write-safe (no more gorilla's "two goroutines writing → panic" footgun), `CloseRead` idiom for write-only hubs.
- **`go.yaml.in/yaml/v3`** — note the new canonical import path (the YAML org took over after go-yaml was marked unmaintained in April 2025). API-identical to `gopkg.in/yaml.v3`.
- **`golang.org/x/crypto/bcrypt`** — for Basic Auth credentials with cost ≥ 12 (PROJECT.md requirement).
- **`github.com/rs/cors` v1.11.x** — CORS middleware for browser clients. Hand-rolled CORS is the #1 source of subtle bugs; `rs/cors` is the conservative, well-established pick.
- **`github.com/stretchr/testify/require`** — test assertions only (no `mock`, no `suite`); table-driven `t.Run` plus `httptest` from stdlib for integration tests.

**Explicitly NOT used:** gorilla/websocket, gin/echo/fiber, spf13/viper, logrus, zap, any ORM, the full `golang-standards/project-layout`, `testify/mock`, `testify/suite`.

### Expected Features

OpenBuro Server is **not** a generic REST CRUD server — it is a capability broker, and features must be judged against prior art (Android `IntentResolver`, Cozy Stack intents, XDG Desktop Portal), not against "typical backend" expectations. See [FEATURES.md](./FEATURES.md) for the full landscape, prior-art comparison, and MIME matching specification.

**Must have (table stakes — the broker is useless without these):**
- Manifest upsert / delete / list / fetch-one with strict validation (enum actions: `PICK`, `SAVE`; non-empty `mimeTypes`; canonicalized MIME strings)
- Capability aggregation (`GET /api/v1/capabilities`) with `?action=` and `?mimeType=` filters
- **Symmetric MIME wildcard matching** across the full 3×3 matrix {exact, `type/*`, `*/*`} × {exact, `type/*`, `*/*`} — single biggest correctness concern
- WebSocket endpoint (`GET /api/v1/capabilities/ws`) broadcasting a single `REGISTRY_UPDATED` event type (not Android's multi-event taxonomy)
- Full-state broadcast on connect (eliminates the connect-then-fetch race by design)
- Periodic ping/keepalive (default 30s, configurable) + origin check derived from the same allow-list as CORS
- HTTP Basic Auth on writes only (reads are public by design) with bcrypt cost ≥ 12 and constant-time compare
- Atomic JSON file persistence (temp+fsync+rename+dir-fsync), load at startup, graceful on missing file
- `/health` liveness, structured `log/slog` request + audit logging, CORS via `rs/cors`, graceful shutdown with WebSocket close-broadcast

**Should have (differentiators — elevate from "CRUD+WS" to "clear reference for the pattern"):**
- Hub pattern with deterministic fan-out and slow-consumer drop (the canonical `coder/websocket` chat hub)
- Event coalescing / 50-100ms debounce for burst mutations (optional; defer to v1.1 if time-constrained)
- `/readyz` readiness endpoint distinct from `/health` liveness
- Example manifests (`examples/manifest-tdrive.json`, etc.) as both fixtures and quickstart docs
- Human-readable indented `registry.json`, deterministic capability ordering in responses
- Reconnect hint semantics via standard close codes (`1001` going away, `1000` normal)

**Defer (v2+, explicitly out of scope per PROJECT.md):**
- Pluggable storage backend (SQLite/Postgres) — upgrade path if registry grows past ~10MB
- Optimistic concurrency / ETags / 409 — single-admin assumption for v1
- Hot-reload of credentials/config — restart is acceptable
- Multi-tenancy, OAuth/OIDC, rate limiting, Prometheus metrics, OpenTelemetry, gRPC, GraphQL

**Anti-features (seductive but wrong for v1):** subscribe/unsubscribe WebSocket protocol (defeats "broker has one thing to say"), multiple event types (forces client reconcile logic), per-event diffs (premature optimization), capability invocation through the broker (violates the out-of-data-path architectural bet), JSON Schema validation (hand-rolled is ~50 lines, schema adds a whole "which source of truth wins" conversation).

### Architecture Approach

Four `internal/` packages matching the four domains, with a **strict unidirectional dependency graph** enforced by construction. `cmd/server/main.go` is ~80 lines of wiring only (the compose-root pattern). Handlers are methods on a single `httpapi.Server` struct with dependencies as fields (Mat Ryer's "How I write HTTP services after eight years" idiom). See [ARCHITECTURE.md](./ARCHITECTURE.md) for the full dependency graph, patterns, and data flow diagrams.

**Major components:**
1. **`internal/config`** — YAML loaders for `config.yaml` and `credentials.yaml`. Pure data. Depends on nothing internal.
2. **`internal/registry`** — `Manifest` domain type, `Store` with `Upsert/Delete/Get/List` + `Capabilities(filter)`, owns the `sync.RWMutex`, owns atomic JSON load/save. Imports only `encoding/json`, `os`, `sync`, `path/filepath`. Knows nothing about HTTP or WebSocket.
3. **`internal/wshub`** — `Hub` (subscribers map + mutex), `subscriber` (buffered `msgs chan` + `closeSlow` callback), `Subscribe(ctx, conn)` writer loop. The canonical `coder/websocket` chat hub pattern — `Publish` is non-blocking with drop-slow-consumer semantics; `CloseRead` is used because this is a write-only hub. Does NOT import `registry` — takes `[]byte` events only.
4. **`internal/httpapi`** — `Server` struct holding `*registry.Store`, `*wshub.Hub`, credentials, logger. Builds `http.ServeMux` with route patterns. Middleware chain: recover → log → CORS → per-route auth. Handlers validate input, call `registry`, then call `hub.Publish` on success. Also owns WebSocket upgrade and hand-off to the hub.

**Key patterns:**
- **Handler-as-method on a `*Server` struct** with deps as fields
- **Mutation → broadcast in the handler layer**, not inside the registry (keeps the core pure, one answer to "what triggers a broadcast")
- **`coder/websocket` canonical chat hub** — no "hub goroutine" needed (gorilla pattern); direct fan-out in `Publish` works because `coder/websocket` guarantees concurrent-write safety on a single conn
- **Atomic write-through persistence** — mutex-held temp+fsync+rename+dir-fsync inside `Store.Upsert`/`Delete`, with in-memory rollback if persist fails
- **Middleware chain as function composition** — `func(http.Handler) http.Handler` decorators composed in one place
- **Two-phase graceful shutdown** — `httpSrv.Shutdown(ctx)` then `hub.Close()`, because `http.Server.Shutdown` does NOT close hijacked connections

### Critical Pitfalls

The full document ([PITFALLS.md](./PITFALLS.md)) catalogs ~12 showstoppers and serious issues with wrong-code/right-code examples. The top ones to keep front-of-mind:

1. **Registry↔hub lock-ordering deadlock (ABBA).** Goroutine A holds `registry.mu` and calls `hub.Broadcast` which takes `hub.mu`; goroutine B holds `hub.mu` on unregister and reaches back into registry. Classic ABBA, invisible to `-race`. **Prevention:** `registry` never imports `wshub`; the HTTP handler is the one and only wiring point. Registry releases its lock BEFORE any callback. Run `go list -deps ./... | grep internal` to verify the DAG.

2. **MIME matching asymmetry.** Forgetting that both sides can be wildcards — capability `image/*` must match query `image/png` AND capability `image/png` must match query `image/*`. This is what Cozy Stack gets wrong and what clients notice first. **Prevention:** an exhaustive table-driven test over the 3×3 wildcard matrix lands with the first line of matching code.

3. **WebSocket goroutine leak via missing `CloseRead`.** Per `coder/websocket` docs: "You must always read from the connection. Otherwise control frames will not be handled." A write-only hub that never reads can't process close frames, and every disconnect leaks the reader+writer goroutines. **Prevention:** call `ctx = conn.CloseRead(ctx)` at the top of `Subscribe`; defer `hub.removeSubscriber`; integration test asserts goroutine count doesn't grow after 1000 connect/disconnect cycles.

4. **Slow-client stall in the hub.** Holding the mutex while calling `conn.Write` in a loop lets one slow client stall every subsequent broadcast and every HTTP write handler. **Prevention:** per-subscriber buffered `msgs chan` with `select { case s.msgs <- msg: default: go s.closeSlow() }` non-blocking fan-out. Drop slow consumers, never back-pressure the publisher.

5. **`registry.json` corruption + memory/disk divergence.** Naive `os.Create` + `Encode` + `Close` leaves an empty file on crash. And even with atomic write, updating the in-memory map *before* persist fails means memory and disk diverge silently. **Prevention:** temp-file + `tmp.Sync()` + `os.Rename` + directory fsync, temp file in the **same directory** (not `/tmp`), with **explicit in-memory rollback** on persist failure (snapshot `prev` before mutating).

6. **Graceful shutdown does not close WebSocket (hijacked) connections.** `http.Server.Shutdown` is marketed as graceful but explicitly excludes hijacked conns. WS clients get abrupt TCP resets on SIGTERM. **Prevention:** two-phase shutdown — `httpSrv.Shutdown(ctx)` then `hub.Close()` which calls `conn.Close(websocket.StatusGoingAway, ...)` on every subscriber.

7. **WebSocket origin check disabled (`InsecureSkipVerify: true`).** `coder/websocket` is strict by default; the easy fix is the wrong fix. CORS middleware does NOT cover WebSocket handshakes — you must configure `AcceptOptions.OriginPatterns` separately (but from the same config-driven allow-list as `rs/cors`, to avoid drift). **Prevention:** the string `InsecureSkipVerify` never appears in production code; a test asserts an `Origin: https://evil.com` request is rejected.

8. **Basic Auth timing attack via early username-mismatch return.** Branching on `user != adminUser` before running bcrypt leaks the valid username via timing. **Prevention:** always run bcrypt (even on unauth'd path, using a dummy hash to equalize), combine results with `subtle.ConstantTimeCompare` on the username, never short-circuit.

9. **CORS `AllowOrigins: ["*"]` with `AllowCredentials: true`.** Browsers silently refuse this combination per spec, and the admin's curl works while the browser UI mysteriously fails. **Prevention:** explicit origin list from config for anything with credentials; document that `*` only works with `AllowCredentials: false`.

10. **bcrypt cost drift.** `bcrypt.DefaultCost` is 10, below the PROJECT.md minimum of 12. Cost > 14 makes writes feel sluggish. Also: bcrypt silently truncates passwords > 72 bytes. **Prevention:** verify cost ≥ 12 at credential load time (`bcrypt.Cost(hash)`); the credential-generator tool rejects > 72-byte passwords with a clear error.

## Implications for Roadmap

Based on the combined research, the natural phase structure flows from the dependency graph: **config → (registry || wshub) → httpapi → cmd wiring → hardening**. Registry and wshub are architecturally independent and can be built in parallel, but both are prerequisites for the httpapi layer that wires them together.

### Phase 1: Foundation — Config, Logging, Scaffolding

**Rationale:** Every other phase depends on config loading and logger construction. Ship the skeleton, prove the build works, get CI and the test harness in place before writing domain code.

**Delivers:**
- Module init (`go mod init`, `go 1.26`), five direct deps pinned in `go.mod`
- Directory scaffold: `cmd/server/`, `internal/{config,registry,httpapi,wshub}/`
- `config.yaml` and `credentials.yaml` loaders with struct-level validation
- `log/slog` construction (JSON prod / text dev) as a dependency injected into everything
- `config.example.yaml`, `credentials.example.yaml` at repo root
- `/health` liveness endpoint as the minimal end-to-end proof of life
- CI: `go test ./... -race`, `go vet`, `gofmt` check
- Startup banner log line (version, config path, listen addr, TLS on/off, ping interval, registry path)

**Uses:** Go 1.26, `net/http`, `log/slog`, `go.yaml.in/yaml/v3`

**Avoids:** Starting any domain package before the skeleton compiles end-to-end.

### Phase 2a: Registry Core (parallel with 2b)

**Rationale:** The registry is the pure domain core with zero internal dependencies; it can be built and fully tested without any transport or hub. Atomic persistence MUST land in the first commit, not as a follow-up — it's a critical pitfall.

**Delivers:**
- `Manifest` domain type with `Validate()` (required fields, action enum, MIME canonicalization)
- `Store{mu, apps, path}` with `Upsert/Delete/Get/List`
- **Atomic persistence day one:** temp file in same directory + `Sync()` + `Rename` + directory fsync
- **In-memory rollback day one:** snapshot `prev` before mutation, restore on persist failure
- Load-at-startup with graceful handling of missing file (empty registry) and fail-fast on corrupted JSON
- `Capabilities(filter)` with **symmetric 3×3 wildcard matching**
- Deterministic ordering (sort capabilities by `(appId, action, mimeType[0])`)
- Table-driven tests: the exhaustive MIME matching matrix, concurrency tests with `-race`, persist-failure-rollback test against an unwritable directory

**Addresses features:** Table-stakes registry CRUD, capability aggregation, MIME filter, atomic persistence

**Avoids pitfalls:** #2 (MIME asymmetry — exhaustive test matrix lands with the code), #5 (corruption + divergence — atomic persist + rollback land in first commit)

### Phase 2b: WebSocket Hub (parallel with 2a)

**Rationale:** The hub is independently testable against a `httptest.NewServer` and depends on nothing registry-shaped. Shipping it in parallel with the registry means httpapi wiring in phase 3 has both sides ready.

**Delivers:**
- `Hub{mu, subscribers, closed}` with `Publish([]byte)` and `Close()`
- `subscriber{msgs chan, closeSlow}` with buffered `msgs` (default 16) and drop-slow-consumer semantics (`select { case s.msgs <- msg: default: go s.closeSlow() }`)
- `Subscribe(ctx, conn)` writer loop using `conn.CloseRead(ctx)` for the write-only case
- Periodic ping via `conn.Ping(ctx)` with ticker (interval from config)
- `Event` serialization: single `REGISTRY_UPDATED` type with denormalized capability payload
- Goroutine leak integration test: 1000 connect/disconnect cycles asserting `runtime.NumGoroutine()` is flat
- Slow-consumer drop test: a subscriber that never drains is kicked without blocking the publisher

**Addresses features:** WebSocket endpoint, hub pattern, ping/keepalive, full-state broadcast on connect (supplied by caller), single event type

**Avoids pitfalls:** #1 (deadlock — hub imports NO registry types), #3 (goroutine leak — `CloseRead` + `defer removeSubscriber` land in first commit), #4 (slow-client stall — non-blocking fan-out is the only code path)

### Phase 3: HTTP API Layer

**Rationale:** This is the wiring phase. It is the FIRST phase where `registry` and `wshub` meet, and therefore the first phase where the deadlock pitfall matters in practice. Shipping auth, CORS, and origin check in the same phase ensures the CORS allow-list and WS origin patterns come from one source of truth.

**Delivers:**
- `httpapi.Server` struct (handler-as-method idiom) with constructor `New(store, hub, creds, logger)`
- Route registration using Go 1.22 `ServeMux` method patterns
- Handlers: `list/get/upsert/delete/capabilities/ws-upgrade/health`
- Middleware chain: recover → log → CORS (`rs/cors`) → per-route `requireAuth` wrapper on write routes
- **Mutation → broadcast in the handler layer** (NOT in the registry) — the single architectural rule that prevents the deadlock
- Basic Auth middleware with bcrypt verify, constant-time username compare, equalized-timing unauthenticated path, and bcrypt cost ≥ 12 validation at credential-load time
- WebSocket upgrade handler with `AcceptOptions.OriginPatterns` from config (shared with CORS allow-list)
- **Full-state broadcast on connect**: before handing the conn to `hub.Subscribe`, send the current capability snapshot as an initial `REGISTRY_UPDATED` event
- JSON error envelope helper; consistent error responses
- Integration tests via `httptest.NewServer`: REST round-trips, WS round-trip (upsert → event received within N ms), origin-rejection test, timing-safe auth test

**Uses:** `net/http`, `rs/cors`, `golang.org/x/crypto/bcrypt`, `crypto/subtle`, `coder/websocket`, `testify/require`, `httptest`

**Avoids pitfalls:** #1 (unidirectional deps enforced here), #7 (OriginPatterns from config, no `InsecureSkipVerify`), #8 (equalized-time auth), #9 (explicit origin list when credentials are in play), #10 (bcrypt cost check at load time)

### Phase 4: Main Wiring + Graceful Shutdown

**Rationale:** `cmd/server/main.go` is pure composition. It's tiny (~80 lines) but the two-phase shutdown is a correctness requirement, not an optimization — a hub without graceful shutdown is a demo-killer. Ship this as its own phase with a "can I restart the server cleanly under SIGTERM with active WS connections" as the Definition of Done.

**Delivers:**
- Flag parsing (`-config`)
- Wiring order: `config.Load` → `registry.NewStore` → `wshub.New` → `httpapi.New` → `http.Server`
- Signal-aware root context via `signal.NotifyContext`
- **Two-phase shutdown**: `httpSrv.Shutdown(shutCtx)` (with ~15s budget), then `hub.Close()` which calls `conn.Close(StatusGoingAway, ...)` on every subscriber
- Optional TLS path (`server.tls.enabled`) using `http.Server.ListenAndServeTLS`
- Startup proof: manual smoke test with `POST` → WS client receives event → `SIGTERM` → WS client receives clean close frame → process exits cleanly

**Uses:** `os/signal`, `syscall`, `context`, everything from prior phases

**Avoids pitfalls:** #6 (two-phase shutdown is THE fix for `http.Server.Shutdown`'s hijacked-connection blind spot)

### Phase 5: Hardening + Reference-Impl Polish

**Rationale:** Everything that elevates this from "works" to "worth reading as a reference." These are optional in the sense that v1 can ship without them, but each is a single-day addition that pays for itself immediately.

**Delivers:**
- `/readyz` readiness endpoint distinct from `/health`
- Event coalescing / 50-100ms debounce for burst mutations
- Example manifests (`examples/manifest-tdrive.json`, `examples/manifest-nextcloud.json`) doubling as quickstart fixtures
- Structured audit log lines for write operations (`slog.Info("audit", ...)` with `user, action, appId`)
- README with: quickstart, manifest schema table, protocol decisions (one event type, no subscribe, reads public), close-code contract documentation, v2 upgrade paths
- `credentials-in-logs` test: scan every log handler output under a load test, assert no `Authorization` header or password ever appears

### Phase Ordering Rationale

- **Config first** because every other package needs config + logger.
- **Registry and wshub in parallel** because their dependency graphs are disjoint (neither imports the other or anything they don't already depend on). This lets a team of two work without blocking, or a solo developer switch contexts when fatigued without re-architecting.
- **HTTP API after both** because it's the ONLY place where registry and hub meet. Shipping this phase third enforces the architectural rule that prevents the ABBA deadlock by construction — there's no earlier phase where a cross-package call could accidentally sneak in.
- **Main wiring after HTTP API** because the compose-root has nothing to compose without the API. Keeping main tiny and late makes it obvious that the real work lives in `internal/`.
- **Hardening last** because everything in phase 5 is a leaf — none of it changes earlier interfaces. This is exactly what "defer polish until correctness is proven" means.
- **MIME matching, atomic persistence, CloseRead, two-phase shutdown, timing-safe auth** all land in their respective first commits, not as later hardening — the research explicitly flags each of these as "must be in the first commit of the phase" because retrofitting them is significantly harder than getting them right the first time.

### Research Flags

Phases likely needing deeper research (`/gsd:research-phase`) during planning:
- **Phase 2a (Registry):** The MIME canonicalization + matching specification is subtle enough that a focused research pass on edge cases (content-type parameters, rejected forms like `*/subtype`, case handling, whitespace) is worth the time before writing the first test. This is the single biggest correctness concern in the project.
- **Phase 3 (HTTP API):** The interaction between `rs/cors` (REST) and `coder/websocket` `OriginPatterns` (WS) is documented in PITFALLS but worth a focused "one source of truth for allowed origins" pass to nail the config schema before the handler is written.

Phases with standard patterns (can skip deeper research, reference ARCHITECTURE.md directly):
- **Phase 1 (Foundation):** Standard Go project layout; STACK.md + ARCHITECTURE.md cover it.
- **Phase 2b (wshub):** The `coder/websocket` canonical chat example is well-documented and ARCHITECTURE.md quotes it directly.
- **Phase 4 (main wiring):** The two-phase shutdown pattern is fully specified in PITFALLS.md Pitfall 6 and ARCHITECTURE.md Pattern 6.
- **Phase 5 (polish):** Individual, additive features; no architectural research needed.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Every choice verified against Context7 / official repos / release pages; five direct deps, all with pinned versions and rationale |
| Features | HIGH | Table stakes map 1:1 to PROJECT.md Active section; differentiators corroborated by prior-art comparison (Android, Cozy, XDG); anti-features all tied to explicit PROJECT.md out-of-scope list |
| Architecture | HIGH | The four-package split + unidirectional dep graph + handler-as-method idiom is the 2024-2026 Go consensus; `coder/websocket` canonical chat hub is quoted from the library's own example |
| Pitfalls | HIGH | Concurrency, bcrypt, graceful-shutdown items verified against Go stdlib docs, `coder/websocket` pkg.go.dev, and Go issue tracker; every pitfall has a wrong-code/right-code example |

**Overall confidence:** HIGH

### Gaps to Address

- **MIME parameter handling details:** The research says "strip parameters (e.g. `; charset=utf-8`), match on `type/subtype` only" but the exact parser behavior on malformed parameters is worth nailing in the Phase 2a test fixtures before coding.
- **Exact ping interval + read deadline ratio:** Defaults are 30s ping and "read deadline = 2× ping interval," but whether to make the ratio configurable or hardcode it is a small open question for Phase 2b.
- **Debounce window for event coalescing:** 50-100ms is the recommended range; the exact number should be chosen during Phase 5 based on "how many flicker frames does a client UI tolerate" (empirical call, not a research gap).
- **`rs/cors` vs `jub0bs/cors`:** Research picks `rs/cors` as the conservative choice but flags `jub0bs/cors` as technically better. Either works; no re-evaluation needed unless a CORS bug surfaces.
- **Credentials-file watching for integration tests:** Not in scope per PROJECT.md, but the test suite's "restart to reload" assertion is worth writing down explicitly in Phase 1 so it doesn't creep back in.

## Sources

### Primary (HIGH confidence)

- `.planning/PROJECT.md` — hard constraints (Go, `log/slog`, bcrypt, Basic Auth, file persistence), Active feature list, explicit out-of-scope list
- `.planning/research/STACK.md` — full stack analysis with version pins and alternatives-considered
- `.planning/research/FEATURES.md` — table-stakes / differentiators / anti-features with prior-art comparison
- `.planning/research/ARCHITECTURE.md` — four-package split, dependency graph, six architectural patterns, data flow diagrams
- `.planning/research/PITFALLS.md` — ~12 showstopper + serious pitfalls with wrong-code / right-code examples
- [Go release history (go.dev/doc/devel/release)](https://go.dev/doc/devel/release) — Go 1.26.2 verified (2026-04-07)
- [Routing Enhancements for Go 1.22 (go.dev/blog/routing-enhancements)](https://go.dev/blog/routing-enhancements) — ServeMux method + pattern matching
- [coder/websocket on GitHub](https://github.com/coder/websocket) — active maintenance, v1.8.14, Go 1.25+ support, `CloseRead` semantics
- [A New Home for nhooyr/websocket (coder.com/blog/websocket)](https://coder.com/blog/websocket) — nhooyr → coder transition verified
- [pkg.go.dev: coder/websocket](https://pkg.go.dev/github.com/coder/websocket) — control-frame handling requirement ("You must always read from the connection")
- [pkg.go.dev: go.yaml.in/yaml/v3](https://pkg.go.dev/go.yaml.in/yaml/v3) — new canonical YAML path verified
- [pkg.go.dev: golang.org/x/crypto/bcrypt](https://pkg.go.dev/golang.org/x/crypto/bcrypt) — cost API, 72-byte truncation
- [Android Intents and Intent Filters](https://developer.android.com/guide/components/intents-filters) — MIME matching semantics, symmetric wildcard rules
- [Cozy Stack `/intents` documentation](https://docs.cozy.io/en/cozy-stack/intents/) — resolver and `availableApps` concept
- [XDG Desktop Portal FileChooser](https://flatpak.github.io/xdg-desktop-portal/docs/doc-org.freedesktop.portal.FileChooser.html) — request/response pattern
- [Go `http.Server.Shutdown` docs](https://pkg.go.dev/net/http#Server.Shutdown) — explicit exclusion of hijacked connections

### Secondary (MEDIUM confidence — community consensus / opinion pieces)

- [Alex Edwards — Which Go Router Should I Use?](https://www.alexedwards.net/blog/which-go-router-should-i-use) — post-1.22 ServeMux-first recommendation
- [Ben Hoyt — Different approaches to HTTP routing in Go](https://benhoyt.com/writings/go-routing/) — comparative analysis
- [Mat Ryer — How I write HTTP services in Go after eight years](https://grafana.com/blog/2024/02/09/how-i-write-http-services-in-go-after-13-years/) — handler-as-method idiom (implicit)
- [jub0bs — rs/cors vs jub0bs/cors](https://jub0bs.com/posts/2024-04-27-jub0bs-cors-a-better-cors-middleware-library-for-go/)
- [WebSocket.org Go WebSocket Guide](https://websocket.org/guides/languages/go/)
- [Go Ecosystem Trends 2025 (JetBrains GoLand blog)](https://blog.jetbrains.com/go/2025/11/10/go-language-trends-ecosystem-2025/)

### Tertiary

None requiring validation — all tertiary material was cross-checked against at least one HIGH-confidence source during the four underlying research passes.

---
*Research completed: 2026-04-09*
*Ready for roadmap: yes*
