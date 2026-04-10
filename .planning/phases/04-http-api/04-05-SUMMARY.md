---
phase: 04-http-api
plan: 05
subsystem: api
tags: [cors, rs-cors, integration-tests, websocket, round-trip, same-host-bypass, pii-guard, phase-4-gates, ws-08]

# Dependency graph
requires:
  - phase: 04-http-api
    provides: "Plan 04-01 Server + middleware chain + corsMiddleware pass-through placeholder"
  - phase: 04-http-api
    provides: "Plan 04-02 LoadCredentials + testdata/credentials-valid.yaml + authBasic middleware"
  - phase: 04-http-api
    provides: "Plan 04-03 real REST handlers (upsert/delete/list/get) + audit log"
  - phase: 04-http-api
    provides: "Plan 04-04 handleCapabilitiesWS + snapshot-before-subscribe + Hijacker shim"
  - external: "github.com/rs/cors v1.11.1 (direct)"
provides:
  - "internal/httpapi/middleware.go: REAL corsMiddleware using rs/cors.New with s.cfg.AllowedOrigins (replaces the Plan 04-01 pass-through placeholder). Shared allow-list with handleCapabilitiesWS OriginPatterns — WS-08 contract fully landed"
  - "internal/httpapi/integration_test.go: 4 end-to-end tests exercising the full middleware chain — REST round-trip (11 sub-steps), WebSocket round-trip (snapshot→added→removed), WS origin rejection (same-host-bypass-defeating Origin header), CORS preflight + disallowed-origin header assertions"
  - "internal/httpapi/auth_test.go: TestAuth_NoCredentialsInLogs upgraded from authBasic-only to FULL middleware chain via httptest.NewServer, asserting 8 forbidden substrings including bcrypt $2a$ hash prefix and a supplied-but-unknown username"
  - ".planning/phases/04-http-api/04-GATES.md: Phase 4 final architectural gate sweep mirror of Phase 3's 03-GATES.md"
affects: [05-wiring-shutdown-polish, phase-4-verification]

# Tech tracking
tech-stack:
  added:
    - "github.com/rs/cors v1.11.1 (flipped from indirect to direct in Task 2 GREEN when middleware.go added the import)"
  patterns:
    - "Shared AllowedOrigins allow-list: s.cfg.AllowedOrigins feeds BOTH rs/cors.Options.AllowedOrigins (REST CORS) AND websocket.AcceptOptions.OriginPatterns (WS handshake). Single source of truth — a browser client that passes the REST CORS check also passes the WS origin check"
  - "Same-host bypass defeat in WS origin-rejection test: DialOptions.HTTPHeader['Origin'] = 'https://evil.example' (a host DIFFERENT from ts.URL's host) because coder/websocket v1.8.14 has a strings.EqualFold(r.Host, u.Host) early-exit that auto-passes the origin check when the request and origin hosts match. Without this deliberate divergence the test would be a false positive"
  - "rs/cors v1.11.1 Access-Control-Request-Headers parsing is STRICT: the header value MUST be lowercase (per Fetch spec 'cors-unsafe request-header names' normalization) AND sorted lexicographically. 'authorization,content-type' works; 'Authorization,Content-Type' yields 'headers not allowed' and the preflight is silently dropped (204 with no ACAO). This is a library quirk our integration test has to match"
  - "Integration tests use httptest.NewServer(srv.Handler()) so every request traverses the FULL chain: recover → log → cors → mux → authBasic (on write routes) → handler. This proves the middleware wrapping order locked in Plan 04-01 is still correct after 4 more plans of expansion"
  - "REST round-trip uses t.Run subtests for per-step failure localization: 11 sub-steps total covering POST/GET/PUT/DELETE/401/400 across registry and capabilities routes in a single flow"
  - "WS round-trip proves the full mutation-then-broadcast pipeline end-to-end: dial → read SNAPSHOT → POST via REST (triggers store.Upsert + hub.Publish) → read ADDED event → DELETE via REST → read REMOVED event. This is the first test that covers the entire Phase 4 stack in a single connected flow"

key-files:
  created:
    - "internal/httpapi/integration_test.go"
    - ".planning/phases/04-http-api/04-GATES.md"
  modified:
    - "internal/httpapi/middleware.go"
    - "internal/httpapi/auth_test.go"
    - "go.mod"
    - "go.sum"

key-decisions:
  - "rs/cors v1.11.1 Access-Control-Request-Headers values MUST be lowercase AND sorted. The test was initially written with 'Authorization,Content-Type' (Go's stdlib canonical form) but rs/cors silently dropped the preflight because its internal SortedSet stores lowercase names and does a case-sensitive map lookup. Fix was a comment-documented 'authorization,content-type' in the test. This is a Fetch spec compliance gotcha, not a bug — browsers always send lowercase — but it surprised a stdlib-trained Go developer"
  - "WS origin-rejection test deliberately picks 'https://evil.example' rather than reusing ts.URL because coder/websocket v1.8.14 has a strings.EqualFold(r.Host, u.Host) early-exit in the accept path. A test that used ts.URL as the Origin would pass EVEN IF OriginPatterns were empty — false positive. The comment on TestServer_WebSocket_RejectsDisallowedOrigin documents this mini-trap for future test authors"
  - "rs/cors is wired in the corsMiddleware method as `cors.New(...).Handler(next)` — the *Cors object is reconstructed on every call to corsMiddleware. This runs once per Server.Handler() invocation (which is itself invoked once by cmd/server/main.go for the lifetime of the process), so the reconstruction cost is paid ONCE and amortized to zero. Alternative was to cache the *Cors on the Server struct, rejected as premature optimization"
  - "TestAuth_NoCredentialsInLogs upgraded in place rather than added as a sibling test. The Plan 04-02 version tested only authBasic's log line in isolation; the Plan 04-05 version drives through the full middleware chain (recover→log→cors→authBasic→handler→audit). Same name, broader coverage — TEST-06 is now asserted end-to-end rather than per-layer"
  - "Integration test suite runs against httptest.NewServer(srv.Handler()) rather than calling srv.Handler() directly with httptest.NewRecorder. The NewServer path exercises real TCP + real HTTP/1.1 framing + real client-connection reuse, which catches bugs that unit-level ResponseRecorder tests miss (e.g., the Hijacker shim bug from Plan 04-04 was ONLY observable through real httptest.NewServer; ResponseRecorder does not implement http.Hijacker so no WS upgrade attempt would ever succeed via Recorder)"
  - "Task 0 leaves rs/cors as indirect and Task 2's `go mod tidy` flips it to direct. This mirrors the golang.org/x/crypto indirect/direct dance from Plan 04-02. The cleaner alternative (anchor file) is rejected per Plan 04-02's established convention: the indirect-then-direct dance is the expected consequence of not wanting anchor files, and every plan that adds a new external dep across a task boundary hits it"

requirements-completed: [OPS-01, WS-08, TEST-02, TEST-05, TEST-06]

# Metrics
duration: 8min
completed: 2026-04-10
---

# Phase 4 Plan 05: CORS + Integration Tests Summary

**Wired the real rs/cors middleware into the Plan 04-01 placeholder (replaces the pass-through), landed the two workhorse integration tests (REST round-trip with 11 sub-steps, WebSocket round-trip with snapshot→added→removed), wrote the WebSocket origin-rejection test with a same-host-bypass-defeating Origin header, upgraded TestAuth_NoCredentialsInLogs to drive through the full middleware chain, and ran the Phase 4 final architectural gate sweep. Phase 4 is complete: 26/26 requirements closed, all 8 gates green.**

## Performance

- **Duration:** ~8 min
- **Started:** 2026-04-10T13:40:44Z
- **Completed:** 2026-04-10T13:49:09Z
- **Tasks:** 3 (chore + TDD RED/GREEN)
- **Files created:** 2 (1 test, 1 gates doc)
- **Files modified:** 4 (middleware.go, auth_test.go, go.mod, go.sum)
- **Tests added:** 4 big integration tests (+ TestAuth_NoCredentialsInLogs upgraded in place)

## Accomplishments

- **OPS-01 (shared CORS + WS allow-list):** `corsMiddleware` wraps `s.mux` with `cors.New(cors.Options{AllowedOrigins: s.cfg.AllowedOrigins, AllowedMethods: [GET, POST, DELETE, OPTIONS], AllowedHeaders: [Authorization, Content-Type], AllowCredentials: true, MaxAge: 300}).Handler(next)`. The `s.cfg.AllowedOrigins` slice is the SAME slice that `handleCapabilitiesWS` passes to `websocket.AcceptOptions.OriginPatterns` — a single source of truth for both REST CORS and WebSocket origin validation. Proven by TestServer_CORS_Headers + TestServer_WebSocket_RejectsDisallowedOrigin.
- **WS-08 (shared allow-list contract):** TestServer_WebSocket_RejectsDisallowedOrigin dials `ts.URL + /api/v1/capabilities/ws` with `DialOptions.HTTPHeader["Origin"] = "https://evil.example"` and asserts 403. The Origin value is deliberately a host DIFFERENT from ts.URL's host to defeat the coder/websocket v1.8.14 same-host bypass (`strings.EqualFold(r.Host, u.Host)`); otherwise the test would be a false positive. The comment in the test function documents the trap.
- **TEST-02 (integration tests via httptest.NewServer):** TestServer_Integration_RESTRoundTrip drives 11 t.Run sub-steps through a single httptest.NewServer client session: POST 201 → GET list 200 → GET single 200 → POST 200 (update) → GET capabilities?action=PICK → GET capabilities?mimeType=image/png (wildcard symmetric match) → DELETE 204 → GET 404 → POST 401 (no auth) → POST 400 (invalid JSON) → POST 400 (missing fields). TestServer_Integration_WebSocketRoundTrip drives a single persistent WS connection through snapshot → REST POST → ADDED event → REST DELETE → REMOVED event.
- **TEST-05 (mutation-then-broadcast end-to-end):** The WS round-trip test is the first test in Phase 4 that covers the FULL mutation-then-broadcast pipeline end-to-end: REST POST → authBasic → handleRegistryUpsert → store.Upsert → hub.Publish → subscriber writer goroutine → conn.Write → test.Read. Plan 04-03's TestServer_AuditLog gave indirect evidence via structural log ordering; this test directly observes the event on the wire.
- **TEST-06 (PII-free logs end-to-end):** TestAuth_NoCredentialsInLogs upgraded from authBasic-in-isolation to the full middleware chain via httptest.NewServer. Captures 3 requests (failed auth wrong password, successful auth with audit log, failed auth unknown user) and asserts 8 forbidden substrings never appear in the log: "WRONGPASSWORD_secret_value", "anotherSecret", "testpass", "$2a$" (bcrypt hash prefix), "Basic YWRt", "YWRtaW46", "Authorization", "nonexistent_user". The successful request produces an audit log line (user=admin, action=upsert, appId=mail-app) which is allowed to appear; the forbidden list is explicitly about credential material only.
- **Phase 4 Final Gate Sweep:** All 8 gates pass (see `.planning/phases/04-http-api/04-GATES.md`):
  1. Registry isolation: `go list -deps ./internal/registry | grep -E 'wshub|httpapi'` → EMPTY
  2. Wshub isolation: `go list -deps ./internal/wshub | grep -E 'registry|httpapi'` → EMPTY
  3. No slog.Default in production: `grep -rE 'slog\.Default' internal/httpapi/*.go | grep -v _test.go` → EMPTY
  4. No time.Sleep in tests: `grep -n 'time\.Sleep' internal/httpapi/*_test.go` → EMPTY
  5. No InsecureSkipVerify in production: `grep -rn 'InsecureSkipVerify' internal/httpapi/*.go | grep -v _test.go` → EMPTY
  6. No internal/config import: `grep -rn '"github.com/openburo/openburo-server/internal/config"' internal/httpapi/*.go` → EMPTY
  7. `go vet ./internal/httpapi/...` → clean
  8. `gofmt -l internal/httpapi/` → empty
- **rs/cors flipped to direct:** `grep "github.com/rs/cors" go.mod` → `github.com/rs/cors v1.11.1` (no `// indirect` suffix). Task 0 added it as indirect via `go get`, Task 2's `go mod tidy` after middleware.go imported it flipped it to direct.

## Task Commits

1. **Task 0: Add github.com/rs/cors dependency** — `4a3f650` (chore)
2. **Task 1: RED — integration tests + extended PII test** — `a563d10` (test)
3. **Task 2: GREEN — wire rs/cors + final gate sweep** — `c878252` (feat)

## Files Created/Modified

**Production code:**
- `internal/httpapi/middleware.go` (modified) — Added `"github.com/rs/cors"` import; replaced placeholder `corsMiddleware` with real `cors.New(cors.Options{...}).Handler(next)` wrap driven by `s.cfg.AllowedOrigins`

**Tests:**
- `internal/httpapi/integration_test.go` (created) — 4 integration tests + `newIntegrationTestServer` helper: TestServer_Integration_RESTRoundTrip (11 t.Run sub-steps), TestServer_Integration_WebSocketRoundTrip (snapshot→added→removed), TestServer_WebSocket_RejectsDisallowedOrigin (same-host-bypass-defeating Origin), TestServer_CORS_Headers (preflight + disallowed origin)
- `internal/httpapi/auth_test.go` (modified) — TestAuth_NoCredentialsInLogs rewritten to drive through the full middleware chain via httptest.NewServer with 8 forbidden substrings; added `strings` to imports

**Dependencies:**
- `go.mod` / `go.sum` — `github.com/rs/cors v1.11.1` promoted from indirect to direct

**Documentation:**
- `.planning/phases/04-http-api/04-GATES.md` (created) — Phase 4 final architectural gate sweep results, mirrors Phase 3's 03-GATES.md

## Decisions Made

1. **rs/cors Access-Control-Request-Headers MUST be lowercase and sorted** — The TestServer_CORS_Headers test was initially written with `req.Header.Set("Access-Control-Request-Headers", "Authorization,Content-Type")` (Go stdlib canonical form). rs/cors v1.11.1 silently dropped the preflight with the debug message "Preflight aborted: headers '[Authorization,Content-Type]' not allowed" and returned 204 with no ACAO header. Root cause: rs/cors stores AllowedHeaders as lowercase in an internal SortedSet and does a case-sensitive map lookup on the incoming header values. Per the Fetch spec's "cors-unsafe request-header names" normalization, browsers always send lowercase; rs/cors assumes that contract. Fix: change the test to send `"authorization,content-type"` and add a comment documenting why. This is a compliance gotcha, not a library bug — but it's not obvious to a Go developer who's used to stdlib Header canonicalization.

2. **Same-host bypass defeat via evil.example Origin** — coder/websocket v1.8.14 has an early-exit in the origin check: if `strings.EqualFold(r.Host, u.Host)` (where u is the parsed Origin URL), the origin check auto-passes. If the test used `ts.URL` as the Origin header value, both Host fields would match `127.0.0.1:<port>` and the check would pass even if OriginPatterns were empty — a FALSE POSITIVE for WS-08 enforcement. The test deliberately picks `https://evil.example` as the Origin so the host-match bypass can't fire, meaning the only way the test passes is via the REAL allow-list check. The function doc-comment on TestServer_WebSocket_RejectsDisallowedOrigin documents this trap in 6 lines so future test authors who wonder "why not just use ts.URL?" see the answer inline.

3. **corsMiddleware reconstructs *Cors on every Handler() call** — The real corsMiddleware body is `c := cors.New(...)` followed by `return c.Handler(next)`. This reconstructs the *Cors object every time corsMiddleware is called. In practice that's once per Server.Handler() call, and Server.Handler() is called once by cmd/server/main.go for the lifetime of the process. The reconstruction cost is paid ONCE and amortized to zero. Alternative was to cache *Cors on the Server struct (lazy init via sync.Once or eager init in New) — rejected as premature optimization given the call pattern. If a future refactor makes Handler() variadic or per-request, revisit.

4. **TestAuth_NoCredentialsInLogs upgraded in place, not added as sibling** — The Plan 04-02 version tested authBasic's Warn log line in isolation. The Plan 04-05 version drives the same name through the full middleware chain via httptest.NewServer, covering recover→log→cors→authBasic→handler→audit. Same test name, broader coverage — TEST-06 is now asserted end-to-end rather than per-layer. The 5 original forbidden substrings (testpass, Basic YWRtaW4, YWRtaW46, Authorization, WRONGPASSWORD) become 8 (+ anotherSecret, $2a$ bcrypt prefix, nonexistent_user). The audit log line from the successful request (user=admin, action=upsert, appId=mail-app) is ALLOWED to appear — the forbidden list is strictly about credential material, not domain identifiers.

5. **Integration tests via httptest.NewServer, not httptest.NewRecorder** — NewServer stands up a real TCP listener + real HTTP/1.1 framing + real client-connection reuse. NewRecorder is a ResponseWriter stub that doesn't implement http.Hijacker. The Hijacker shim bug from Plan 04-04 was ONLY observable through NewServer; a NewRecorder-based test would never attempt a WS upgrade. This is the same reason Plan 04-04 used NewServer for the WS tests — Plan 04-05 extends that convention to the REST integration test so all 4 tests share a consistent "real HTTP round-trip" property.

6. **rs/cors indirect/direct dance mirrors Plan 04-02's golang.org/x/crypto pattern** — Task 0 runs `go get github.com/rs/cors` which adds it as indirect (nothing imports it yet; the test file in Task 1 doesn't import it either because rs/cors is only exercised via observable headers). Task 2 adds the `"github.com/rs/cors"` import in middleware.go and `go mod tidy` flips it to direct. This is the same pattern Plan 04-02 used for bcrypt; the indirect-then-direct dance is the expected consequence of not wanting anchor files and is fully recoverable across task boundaries.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] rs/cors v1.11.1 Access-Control-Request-Headers case-sensitivity + sort order**
- **Found during:** Task 2 GREEN verification (first `go test ./internal/httpapi -race -run TestServer_CORS_Headers` run).
- **Issue:** The plan verbatim called `req.Header.Set("Access-Control-Request-Headers", "Authorization,Content-Type")` in TestServer_CORS_Headers. Under rs/cors v1.11.1, the preflight was silently dropped: the response came back as 204 with only Date + Vary headers, no Access-Control-Allow-Origin. Root cause: rs/cors stores AllowedHeaders in an internal SortedSet as lowercase names (per Fetch spec normalization) and does a case-sensitive map lookup on incoming values. "Authorization" and "Content-Type" are not found in the set because the set has "authorization" and "content-type". The cors.Options.Debug=true output was the crucial hint: `Preflight aborted: headers '[Authorization,Content-Type]' not allowed`.
- **Fix:** Changed the test to send `"authorization,content-type"` (lowercase, still sorted lexicographically — 'a' < 'c'). Added a 4-line comment above the `req.Header.Set` line explaining the Fetch spec normalization and why real browsers always send lowercase.
- **Files modified:** `internal/httpapi/integration_test.go`
- **Verification:** `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestServer_CORS_Headers$' -timeout 15s` passes in 3.1s.
- **Committed in:** `c878252` (Task 2 GREEN commit, alongside the middleware.go rs/cors wiring)

---

**Total deviations:** 1 auto-fixed (Rule 3 blocking — test-side compliance fix with rs/cors library contract)
**Impact on plan:** Trivial. No production code change. The rs/cors integration was byte-accurate per the plan; the only drift was that the plan-supplied test called the library with a slightly non-compliant header value that happened to work against the stdlib net/http expectations but not against rs/cors's Fetch-spec-strict parser. One-line fix + comment. No architectural change, no scope creep.

## Issues Encountered

None beyond the deviation above. The rs/cors middleware wiring, the REST round-trip, the WS round-trip, the origin rejection, and the TestAuth_NoCredentialsInLogs upgrade all worked first-compile after the test code was authored.

## Verification Results

**Full plan verification (from the plan's `<verification>` block):**

- `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -count=1 -timeout 240s` — **PASS** (93.7s, full suite including the 4 new integration tests)
- `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestServer_Integration|^TestServer_WebSocket_RejectsDisallowedOrigin$|^TestServer_CORS_Headers$' -timeout 60s` — **PASS** (4 integration tests)
- `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestAuth_NoCredentialsInLogs$' -timeout 10s` — **PASS** (TEST-06 full chain)
- `! ~/sdk/go1.26.2/bin/go list -deps ./internal/registry | grep -E 'wshub|httpapi'` — **PASS** (registry isolation holds)
- `! ~/sdk/go1.26.2/bin/go list -deps ./internal/wshub | grep -E 'registry|httpapi'` — **PASS** (wshub isolation holds)
- `! grep -rE 'slog\.Default' internal/httpapi/*.go | grep -v _test.go` — **PASS** (no global default logger)
- `! grep -n 'time\.Sleep' internal/httpapi/*_test.go` — **PASS** (no sleep-based synchronization in tests)
- `! grep -rn 'InsecureSkipVerify' internal/httpapi/*.go | grep -v _test.go` — **PASS** (PITFALLS #7 anchor)
- `! grep -rn '"github.com/openburo/openburo-server/internal/config"' internal/httpapi/*.go` — **PASS** (dependency inversion locked)
- `~/sdk/go1.26.2/bin/go vet ./internal/httpapi/...` — **PASS**
- `test -z "$(~/sdk/go1.26.2/bin/gofmt -l internal/httpapi/)"` — **PASS**
- `~/sdk/go1.26.2/bin/go build ./...` — **PASS** (whole module builds)
- `~/sdk/go1.26.2/bin/go vet ./...` — **PASS** (whole module vet clean)

**Acceptance criteria spot checks:**

- `grep -c '"github.com/rs/cors"' internal/httpapi/middleware.go` → **1**
- `grep -c "cors.New(cors.Options" internal/httpapi/middleware.go` → **1**
- `grep -c "AllowedOrigins:   s.cfg.AllowedOrigins" internal/httpapi/middleware.go` → **1**
- `grep -c "AllowCredentials: true" internal/httpapi/middleware.go` → **2** (1 code + 1 doc comment)
- `grep -c '^func TestServer_Integration_RESTRoundTrip\|^func TestServer_Integration_WebSocketRoundTrip\|^func TestServer_WebSocket_RejectsDisallowedOrigin\|^func TestServer_CORS_Headers' internal/httpapi/integration_test.go` → **4**
- `grep -c "t.Run(" internal/httpapi/integration_test.go` → **11**
- `grep -c "https://evil.example" internal/httpapi/integration_test.go` → **3**
- `grep -c "same-host bypass\|strings.EqualFold" internal/httpapi/integration_test.go` → **3**
- `grep -c "changeSnapshot\|changeAdded\|changeRemoved" internal/httpapi/integration_test.go` → **3**
- `grep -c "httptest.NewServer" internal/httpapi/auth_test.go` → **2** (1 in TestAuth_NoCredentialsInLogs + 1 import)
- `grep -c '^\s*github.com/rs/cors' go.mod` → **1** (direct, no `// indirect` suffix)

**Named-test validation (5 new/upgraded test functions):**

- `TestServer_Integration_RESTRoundTrip` → **PASS** (11 sub-steps)
- `TestServer_Integration_WebSocketRoundTrip` → **PASS** (snapshot→added→removed round-trip)
- `TestServer_WebSocket_RejectsDisallowedOrigin` → **PASS** (WS-08 enforcement with bypass defeat)
- `TestServer_CORS_Headers` → **PASS** (preflight + disallowed origin)
- `TestAuth_NoCredentialsInLogs` → **PASS** (TEST-06 full chain, 8 forbidden substrings)

Total: 4 new + 1 upgraded = 5 plan-specific test outcomes + the pre-existing 66 httpapi tests from Plans 04-01..04-04 = 70 total tests, full suite race-clean in 93.7s.

## User Setup Required

None for Plan 04-05 itself. Phase 5 compose-root will ask operators to generate a real credentials.yaml; that's a deploy-time concern.

## Next Phase Readiness

**Ready for Phase 4 verification and then Phase 5 (wiring, shutdown, polish):**
- All 26 Phase 4 requirements closed (AUTH-01..05, API-01..11, WS-01, WS-05, WS-06, WS-08, WS-09, OPS-01, OPS-06, TEST-02, TEST-05, TEST-06)
- All 6 Phase 4 routes are wired to real handlers with real middleware and real auth where required
- The middleware chain order (recover → log → cors → mux → authBasic → handler) is locked and integration-tested end-to-end
- The shared AllowedOrigins allow-list is wired into BOTH rs/cors AND websocket.AcceptOptions.OriginPatterns from a single source (s.cfg.AllowedOrigins)
- rs/cors, golang.org/x/crypto, and github.com/coder/websocket are all direct dependencies in go.mod
- All 8 Phase 4 architectural gates pass (see 04-GATES.md)
- Phase 4 is ready for `/gsd:verify-work`

**Notes for Phase 5:**
- cmd/server/main.go currently uses the Plan 04-01 minimal wiring stub (NewStore + wshub.New + empty Credentials + Config from cfg.CORS). Phase 5 replaces this with full compose-root wiring: LoadCredentials from cfg.CredentialsFile, signal-aware graceful shutdown, two-phase Close (hub.Close + http.Server.Shutdown), TLS support
- The Hijacker shim on statusCapturingWriter is now part of the middleware contract. If Phase 5 adds any new ResponseWriter wrappers (for metrics, tracing, request IDs), they MUST also forward http.Hijacker or WebSocket upgrades will silently break again. Add this to Phase 5's PITFALLS checklist
- The rs/cors wiring exposes `AllowedOrigins` as the whole source of truth. If Phase 5 needs to add per-route origin overrides (unlikely for this reference impl), the single-source-of-truth property would need revisiting

**No blockers or concerns.**

## Self-Check: PASSED

Verified:
- `internal/httpapi/integration_test.go` — FOUND
- `internal/httpapi/middleware.go` — FOUND (modified, rs/cors wired)
- `internal/httpapi/auth_test.go` — FOUND (modified, TestAuth_NoCredentialsInLogs extended)
- `go.mod` — FOUND (rs/cors v1.11.1 direct)
- `go.sum` — FOUND (rs/cors hashes present)
- `.planning/phases/04-http-api/04-05-SUMMARY.md` — FOUND (this file)
- `.planning/phases/04-http-api/04-GATES.md` — FOUND
- Commit `4a3f650` (Task 0 chore add rs/cors) — FOUND in git log
- Commit `a563d10` (Task 1 test RED integration tests) — FOUND in git log
- Commit `c878252` (Task 2 feat GREEN wire rs/cors) — FOUND in git log

---
*Phase: 04-http-api*
*Completed: 2026-04-10*
