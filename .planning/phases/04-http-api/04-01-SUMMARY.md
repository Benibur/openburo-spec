---
phase: 04-http-api
plan: 01
subsystem: api
tags: [http, middleware, cors, error-envelope, slog, method-prefixed-routes, recover, stdlib-http]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: "Phase 1 httpapi.Server skeleton with /health handler + injected *slog.Logger"
  - phase: 02-registry-core
    provides: "*registry.Store with NewStore/Get/List/Upsert/Delete/Capabilities — wired into Server.store"
  - phase: 03-websocket-hub
    provides: "*wshub.Hub with New/Publish/Close/Subscribe — wired into Server.hub"
provides:
  - "httpapi.Server expanded from (logger) to (logger, store, hub, creds, cfg) (*Server, error) — stable for all Phase 4 downstream plans"
  - "httpapi.Config{AllowedOrigins, WSPingInterval} — locks the CORS+WS origin allow-list as config, constructor-validated"
  - "httpapi.Credentials STUB (empty struct with users map) — Plan 04-02 replaces with real LoadCredentials/Lookup"
  - "Middleware chain: recover -> log -> cors (placeholder) -> mux, with recover outermost so panics in any inner layer become 500 envelopes"
  - "writeJSONError + writeBadRequest/Unauthorized/Forbidden/NotFound/Internal single source of error-envelope truth"
  - "6 Phase 4 routes registered as stub501 (POST/DELETE/GET /api/v1/registry[/{appId}], GET /api/v1/capabilities[/ws]) so middleware chain is exercisable end-to-end"
  - "newTestServer(t)/newTestServerWithLogger(t,logger) helper used by every downstream plan"
affects: [04-02-auth-credentials, 04-03-registry-handlers, 04-04-capabilities-ws, 04-05-cors-integration-tests, 05-wiring-shutdown-polish]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Constructor-validated config: New returns (*Server, error) and rejects empty/wildcard/bad-pattern CORS allow-lists at construction time (not runtime)"
    - "Nil deps panic at construction (programmer error, no recovery path); operator misconfig returns error (recoverable)"
    - "Middleware chain composed via nested wrap in Handler() — recover is outermost, attached LAST in the chain so it is OUTERMOST when serving"
    - "Error envelope single source of truth: writeJSONError funnel with {error, details,omitempty}; shortcut helpers keep handlers 1-liner"
    - "501 stub handlers for not-yet-wired routes so integration tests exercise the full chain from day one (downstream plans replace in place)"
    - "statusCapturingWriter wraps http.ResponseWriter to capture status for log middleware without breaking handler contract"
    - "logMiddleware skips /health explicitly (inherited from Phase 1's never-log-health convention)"
    - "syncBuffer (mutex-guarded bytes.Buffer) test helper for capturing slog output — mirrors Phase 3 pattern"

key-files:
  created:
    - "internal/httpapi/middleware.go"
    - "internal/httpapi/errors.go"
    - "internal/httpapi/server_test.go"
    - "internal/httpapi/middleware_test.go"
    - "internal/httpapi/errors_test.go"
  modified:
    - "internal/httpapi/server.go"
    - "internal/httpapi/health_test.go"
    - "cmd/server/main.go"

key-decisions:
  - "Server.New signature locked as (logger, store, hub, creds, cfg) (*Server, error) — stable across all Phase 4 downstream plans"
  - "Credentials declared as STUB struct with unexported users map in Plan 04-01 so the New signature compiles now; Plan 04-02 replaces the type in place (no signature change)"
  - "corsMiddleware is a Plan 04-01 pass-through placeholder; the method exists and is wired into Handler() so Plan 04-05 only swaps the body"
  - "[Rule 3 blocking fix] cmd/server/main.go expanded from Phase 1 single-arg New to minimal 5-arg wiring (NewStore, wshub.New, empty Credentials, Config from cfg.CORS+WebSocket) — Phase 5 will replace with full compose-root wiring + graceful shutdown"
  - "Constructor validation: empty AllowedOrigins, literal '*', and invalid path.Match globs all return errors at New-time (fail-fast per PITFALLS #9 — wildcard+credentials incompatibility not caught by rs/cors)"

patterns-established:
  - "Chain order in Handler() documented line-by-line with 'OUTERMOST' comment so future refactors cannot accidentally reorder the wrapping"
  - "501 stub placeholders: downstream plans replace handler bodies (not route registrations) so the route table is stable"
  - "Constructor errors name the field and cite PITFALLS entry numbers when the validation exists because a 3rd-party library failed to reject the misconfig"

requirements-completed: [API-06, API-07, API-08, API-09, API-10, API-11]

# Metrics
duration: 5min
completed: 2026-04-10
---

# Phase 4 Plan 01: Server + Middleware Summary

**Expanded the Phase 1 httpapi.Server skeleton into the Phase 4 transport shell: validated 5-arg constructor returning (*Server, error), recover->log->cors->mux middleware chain with recover outermost, error envelope helpers, and 501-stub handlers for every Phase 4 route so downstream plans slot into stable contracts.**

## Performance

- **Duration:** ~5 min
- **Started:** 2026-04-10T12:35:43Z
- **Completed:** 2026-04-10T12:40:25Z
- **Tasks:** 2 (TDD RED/GREEN)
- **Files modified:** 8 (3 created prod, 3 created test, 2 modified prod, 1 modified test)
- **Tests added:** 13 test functions (21 test cases including subtests)

## Accomplishments

- Server.New signature locked at `(logger, store, hub, creds, cfg) (*Server, error)` — stable for every downstream Phase 4 plan
- Middleware chain composed in the correct recover-outermost order, verified by TestRecover_PanicCaught and TestMiddleware_ChainOrder (panics in handler become 500 envelopes, server survives)
- Constructor validation rejects empty/wildcard/bad-pattern CORS allow-lists at New-time, and panics on nil logger/store/hub (programmer error)
- Error envelope locked via writeJSONError funnel: `{error, details,omitempty}` with Content-Type: application/json; writeUnauthorized sets `WWW-Authenticate: Basic realm="openburo"`
- 6 Phase 4 routes registered as stub501 placeholders so downstream plans replace handler bodies in place; the route table is stable and integration tests exercise the full chain from day one
- logMiddleware skips /health and logs everything else with method/path/status/duration_ms/remote fields
- newTestServer(t) helper + syncBuffer mutex-buffer pattern established for capturing slog output race-free

## Task Commits

1. **Task 1: RED — failing tests for Server.New validation, middleware chain, recover, error envelope, method-not-allowed, health_test.go adaptation** — `34718e0` (test)
2. **Task 2: GREEN — implement Server + Config + middleware chain + errors.go + 501 route stubs** — `b48a511` (feat)

## Files Created/Modified

**Production code:**
- `internal/httpapi/server.go` (modified) — Phase 4 Server struct, Config, Credentials stub, New constructor with validation, Handler middleware chain, stub501, registerRoutes with all 6 Phase 4 routes
- `internal/httpapi/middleware.go` (created) — recoverMiddleware (outermost panic catch), logMiddleware (skips /health), corsMiddleware (Plan 04-05 placeholder), statusCapturingWriter, clientIP
- `internal/httpapi/errors.go` (created) — writeJSONError funnel + writeBadRequest/Unauthorized/Forbidden/NotFound/Internal shortcuts, WWW-Authenticate header on 401
- `cmd/server/main.go` (modified, [Rule 3 blocking]) — expanded Phase 1 single-arg New call to minimal 5-arg wiring with NewStore/wshub.New/empty Credentials/Config-from-cfg

**Tests:**
- `internal/httpapi/server_test.go` (created) — newTestServer helper, syncBuffer, TestServer_New_* constructor validation (empty/wildcard/bad-pattern/nil-deps/Valid), TestServer_MethodNotAllowed
- `internal/httpapi/middleware_test.go` (created) — TestRecover_PanicCaught, TestLogMiddleware_SkipsHealth, TestLogMiddleware_LogsOtherRoute, TestMiddleware_ChainOrder
- `internal/httpapi/errors_test.go` (created) — TestErrors_Envelope, TestErrors_Envelope_NoDetails, TestWriteUnauthorized_Header
- `internal/httpapi/health_test.go` (modified) — Phase 1 tests adapted to newTestServer(t) helper (Phase 1 single-arg New call no longer compiles)

## Decisions Made

1. **Credentials stub shape** — Declared as `struct { users map[string][]byte }` in Plan 04-01 so Server.New compiles; Plan 04-02 will add LoadCredentials/Lookup methods + bcrypt cost validation, not change the struct fields. Empty Credentials literal remains legal forever and means "all writes return 401" once authBasic lands.
2. **corsMiddleware as pass-through placeholder** — The method exists and is wired into Handler() (innermost wrap, closest to mux) in Plan 04-01 so Plan 04-05's rs/cors integration only swaps the body, not the chain order or the field plumbing.
3. **stub501 shared handler** — A single `s.stub501` method serves every not-yet-wired route so Plan 04-02/03/04 replace individual HandleFunc lines (not the route table). This keeps `grep -c "s.mux.HandleFunc"` a stable 7 across Plan 04-01..04-04.
4. **Constructor panics on nil deps, errors on bad config** — Programmer error (nil logger/store/hub) has no recovery path and panics at New-time; operator error (bad AllowedOrigins) is recoverable and returns an error, so Phase 5's main.go can surface a clear fatal message to stderr.
5. **[Rule 3 blocking fix] cmd/server/main.go minimal wiring** — Phase 4 Plan 04-01 changed httpapi.New's signature, which broke `~/sdk/go1.26.2/bin/go test ./...` and `go build ./...`. Rather than let the whole module stay broken until Phase 5, we added minimal wiring: NewStore(cfg.RegistryFile), wshub.New with cfg.WebSocket.PingInterval, empty Credentials, Config built from cfg.CORS.AllowedOrigins + cfg.WebSocket.PingInterval. Phase 5 will replace this with full graceful-shutdown wiring (LoadCredentials from cfg.CredentialsFile, two-phase Close, signal handling).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] cmd/server/main.go compile error after Server.New signature change**
- **Found during:** Task 2 GREEN (after implementing the new 5-arg New signature)
- **Issue:** Phase 1's cmd/server/main.go still called `httpapi.New(logger)` (single-arg). After Task 2's signature change, `go build ./...` and `go test ./...` failed with `assignment mismatch: 1 variable but httpapi.New returns 2 values` and `not enough arguments in call`. The plan's scope is `internal/httpapi` only, and the plan explicitly said no main.go changes in Phase 4 — but the whole-module build was broken, blocking the Task 2 verification sweep.
- **Fix:** Added minimal 5-arg wiring in cmd/server/main.go: imported internal/registry + internal/wshub, constructed store via `registry.NewStore(cfg.RegistryFile)`, constructed hub via `wshub.New(logger, wshub.Options{PingInterval: cfg.WebSocket.PingInterval})` with `defer hub.Close()`, passed empty `httpapi.Credentials{}`, built `httpapi.Config{AllowedOrigins: cfg.CORS.AllowedOrigins, WSPingInterval: cfg.WebSocket.PingInterval}`. Kept an explanatory comment pointing to Phase 5 for full compose-root wiring.
- **Files modified:** cmd/server/main.go
- **Verification:** `~/sdk/go1.26.2/bin/go build ./...` passes; `~/sdk/go1.26.2/bin/go test ./... -race` all packages green.
- **Committed in:** b48a511 (Task 2 GREEN commit)

---

**Total deviations:** 1 auto-fixed (Rule 3 blocking build fix)
**Impact on plan:** The fix is the minimum surface area needed to keep the whole module compiling — only 4 new imports, 1 new hub.Close defer, 1 error-return added. Phase 5's compose-root wiring will replace this entirely (LoadCredentials, signal-aware shutdown, two-phase Close). No scope creep.

## Issues Encountered

None. The plan's specs were byte-accurate; the only surprise was the cmd/server/main.go breakage described in Deviations above.

## Verification Results

**Full plan verification (from the plan's `<verification>` block):**

- `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -count=1 -timeout 60s` — **PASS** (1.019s)
- `! ~/sdk/go1.26.2/bin/go list -deps ./internal/registry | grep -E 'wshub|httpapi'` — **PASS** (registry isolation holds)
- `! ~/sdk/go1.26.2/bin/go list -deps ./internal/wshub | grep -E 'registry|httpapi'` — **PASS** (wshub isolation holds)
- `! grep -rE 'slog\.Default' internal/httpapi/*.go | grep -v _test.go` — **PASS**
- `! grep -rn 'InsecureSkipVerify' internal/httpapi/*.go` — **PASS**
- `! grep -rn '"github.com/openburo/openburo-server/internal/config"' internal/httpapi/*.go` — **PASS**
- `~/sdk/go1.26.2/bin/go vet ./internal/httpapi/...` — **PASS**
- `test -z "$(~/sdk/go1.26.2/bin/gofmt -l internal/httpapi/)"` — **PASS**
- `~/sdk/go1.26.2/bin/go test ./... -race -timeout 120s` — **PASS** (all packages green after cmd/server fix)

**Named-test validation (04-VALIDATION.md rows owned by this plan):**

- `TestServer_New_RejectsWildcardWithCredentials`, `TestServer_New_RejectsEmptyAllowList` — **PASS** (OPS-01)
- `TestServer_MethodNotAllowed` — **PASS** (API-06)
- `TestMiddleware_ChainOrder` — **PASS** (API-07)
- `TestRecover_PanicCaught` — **PASS** (API-08)
- `TestErrors_Envelope`, `TestErrors_Envelope_NoDetails` — **PASS** (API-09)
- `TestWriteUnauthorized_Header` — **PASS** (AUTH-02 header contract)
- `TestLogMiddleware_SkipsHealth`, `TestLogMiddleware_LogsOtherRoute` — **PASS** (API-07 log shape)
- `TestHealth`, `TestHealth_RejectsWrongMethod/{POST,PUT,DELETE}` — **PASS** (Phase 1 regression via newTestServer)

Total: 21 test cases pass race-clean in ~1 second.

## User Setup Required

None — no external service configuration required for Plan 04-01. Plan 04-02 will introduce credentials.yaml, but that's on the operator after deploy, not during build.

## Next Phase Readiness

**Ready for Plan 04-02 (Auth + Credentials):**
- `Credentials` type exists as a stub — Plan 04-02 replaces the struct body (adds validated map + LoadCredentials/Lookup) without touching the Server.New signature
- `writeUnauthorized` helper already sets the right WWW-Authenticate header — authBasic middleware only needs to call it
- `newTestServer(t)` helper already instantiates an empty Credentials table; Plan 04-02 tests can either keep using that (for public-route tests) or pass a populated table via a new helper variant
- stub501 handlers for POST/DELETE /api/v1/registry are already registered — Plan 04-02's authBasic will wrap those specific routes in registerRoutes

**Notes for Plan 04-02:**
- The authBasic middleware should be applied per-route (via a helper like `s.authed(s.stub501)`) rather than globally, so Plan 04-03's public GET routes continue to work without auth
- dummyHash bcrypt.GenerateFromPassword in init() will add ~100-200ms to package test startup once — acceptable tradeoff for timing-safe unknown-user handling

**No blockers or concerns.**

## Self-Check: PASSED

Verified:
- `internal/httpapi/server.go` — FOUND
- `internal/httpapi/middleware.go` — FOUND
- `internal/httpapi/errors.go` — FOUND
- `internal/httpapi/server_test.go` — FOUND
- `internal/httpapi/middleware_test.go` — FOUND
- `internal/httpapi/errors_test.go` — FOUND
- `internal/httpapi/health_test.go` — FOUND (modified)
- `cmd/server/main.go` — FOUND (modified)
- Commit `34718e0` (test RED) — FOUND in git log
- Commit `b48a511` (feat GREEN) — FOUND in git log

---
*Phase: 04-http-api*
*Completed: 2026-04-10*
