---
phase: 04-http-api
plan: 04
subsystem: api
tags: [rest, websocket, snapshot-on-connect, coder-websocket, mime-filter, origin-patterns, hijacker, pitfalls-7]

# Dependency graph
requires:
  - phase: 04-http-api
    provides: "Plan 04-01 Server + logMiddleware + recoverMiddleware + stub501 placeholders"
  - phase: 04-http-api
    provides: "Plan 04-03 events.go (eventPayload w/ omitempty Capabilities + changeSnapshot pre-declared constant)"
  - phase: 02-registry-core
    provides: "*registry.Store.Capabilities(filter) + registry.CanonicalizeMIME exported wrapper"
  - phase: 03-websocket-hub
    provides: "*wshub.Hub.Subscribe(ctx, conn) — blocks until disconnect; handles CloseRead + defer removeSubscriber internally"
provides:
  - "internal/httpapi/handlers_caps.go: handleCapabilities (REST w/ action+mimeType filtering, 400 on malformed mime), buildFullStateSnapshot, handleCapabilitiesWS (WS upgrade + snapshot-before-subscribe)"
  - "internal/httpapi/events.go: newSnapshotEvent helper + dedicated snapshotEvent/snapshotPayload types (no omitempty on Capabilities) so SNAPSHOT events ALWAYS render `\"capabilities\":[]` — never null, never missing"
  - "statusCapturingWriter.Hijack() forwarding shim so logMiddleware no longer breaks any WebSocket upgrade (retroactive fix to a latent Plan 04-01 bug uncovered by this plan's WS tests)"
  - "stub501 deleted from server.go; all 6 Phase 4 routes now wired to real handlers"
affects: [04-05-cors-integration-tests, 05-wiring-shutdown-polish]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Snapshot-before-subscribe (WS-06): buildFullStateSnapshot marshal → conn.Write(snapshot) → hub.Subscribe. The conn.Write line number (17) is strictly less than the hub.Subscribe line number (28) inside handleCapabilitiesWS so the ordering is statically auditable via awk+grep"
    - "Dedicated snapshotPayload struct (no omitempty Capabilities) + reuse of top-level Event/Timestamp keys gives two valid JSON shapes (upsert-via-eventPayload, snapshot-via-snapshotPayload) that unmarshal cleanly into the same registryUpdatedEvent shape on the test side. Single Unmarshal target, two Marshal sources"
    - "MaxBytesReader / DisallowUnknownFields intentionally absent from handleCapabilities — GET has no body, so the only input validation is query-string parsing via CanonicalizeMIME pre-check"
    - "http.Hijacker passthrough on any ResponseWriter wrapper — generalizable rule: middleware that wraps w MUST forward http.Hijacker (and similar stdlib interfaces like http.Flusher, http.Pusher) or silently break the WebSocket upgrade path with a 501"
    - "Comment-as-grep-gate collision pattern (third instance, first in Phase 4): the InsecureSkipVerify grep gate tripped on our own source comments that referenced the knob by name. Fix: reword comments to avoid the literal substring. Phase 3 hit this twice already (slog.Default, time.Sleep); the rule 'always grep-check comments before committing gate-sensitive files' now has four instances"

key-files:
  created:
    - "internal/httpapi/handlers_caps.go"
    - "internal/httpapi/handlers_caps_test.go"
  modified:
    - "internal/httpapi/events.go"
    - "internal/httpapi/events_test.go"
    - "internal/httpapi/server.go"
    - "internal/httpapi/middleware.go"

key-decisions:
  - "Dedicated snapshotPayload/snapshotEvent types rather than reusing eventPayload for SNAPSHOT: Go's json omitempty drops zero-length slices, not just nil slices, so `Capabilities: []registry.CapabilityView{}` on an omitempty-tagged field still omits the key. The fix is a separate payload type without omitempty on Capabilities. The two types share the exact same JSON wire shape when populated; the ONLY difference is how the zero-value serializes. The test-side Unmarshal into registryUpdatedEvent still works because Unmarshal doesn't care about omitempty"
  - "Deleted stub501 entirely rather than keep it as dead code with a 'no longer wired' comment. All 6 Phase 4 routes are real now; retaining stub501 would confuse future reviewers scanning for unwired routes. grep -c 'stub501' on server.go returns 0, which is now a negative-assertion invariant for future plans (Plan 04-05 should keep it at 0)"
  - "Snapshot is written with a 5-second WriteTimeout separate from the r.Context() that Subscribe will use. The ctx.WithTimeout wraps r.Context so peer disconnect during the snapshot send still cancels promptly. If the snapshot write fails we Close with StatusInternalError and Warn-log — we do NOT fall through to hub.Subscribe on a broken conn"
  - "[Rule 1 - Bug] statusCapturingWriter did not implement http.Hijacker, which silently broke the WebSocket upgrade path: coder/websocket.Accept calls w.(http.Hijacker) on the ResponseWriter, and when that type assertion fails it writes a 501 Not Implemented response. The bug was latent since Plan 04-01 (where logMiddleware was added) but had no observable effect until Plan 04-04 introduced the first WebSocket route. Fix is a 15-line Hijack() method that forwards to the inner writer and promotes the captured status to 101 on success so the request log reflects the protocol switch"
  - "CanonicalizeMIME pre-validation lives in the handler (not the store). The Store.Capabilities contract documented since Plan 02-03 says malformed MimeType filters yield empty results, not errors; the HTTP handler is the layer that wants a 400 on malformed input, and the pre-validation via the exported CanonicalizeMIME wrapper is the documented path. This keeps the store 400-free (no HTTP-layer concerns leak into the domain) and gives REST clients a clear error envelope"
  - "The SubscribesAfterSnapshot test uses `srv.store.Upsert` + `srv.hub.Publish` directly rather than POSTing to /api/v1/registry (which would require auth). This isolates the WS subscribe-after-snapshot ordering from the full mutation-then-broadcast pipeline — plan 04-05 adds the integrated end-to-end test that goes through HTTP POST → store → hub → WS read"
  - "conn.Close(StatusNormalClosure) is defer'd in every WS test AFTER the conn is verified non-nil. The coder/websocket StatusNormalClosure close handshake has a 5s+5s budget per Phase 3's observations, but the test ctx is 5s so normal-closure cleanup fits comfortably. Tests average <200ms each; the only slow test is the subscribe-after-snapshot one which has to wait for the Publish to propagate (still <100ms)"

requirements-completed: [API-05, WS-01, WS-06, API-10]

# Metrics
duration: 8min
completed: 2026-04-10
---

# Phase 4 Plan 04: Capabilities REST + WebSocket Summary

**Replaced the two remaining Phase 4 stubs with real handlers: GET /api/v1/capabilities (REST with action+mimeType filtering and 400-on-malformed-mime via CanonicalizeMIME pre-validation) and GET /api/v1/capabilities/ws (coder/websocket upgrade that writes the full-state SNAPSHOT event BEFORE handing off to hub.Subscribe, eliminating the connect-then-fetch race). Caught and fixed a latent Plan 04-01 bug where statusCapturingWriter did not implement http.Hijacker, which had been silently sabotaging every WebSocket upgrade attempt since the middleware was introduced.**

## Performance

- **Duration:** ~8 min
- **Started:** 2026-04-10T13:25:50Z
- **Completed:** 2026-04-10T13:34:00Z
- **Tasks:** 2 (TDD RED/GREEN)
- **Files created:** 2 (1 prod, 1 test)
- **Files modified:** 4 (events.go, events_test.go, server.go, middleware.go)
- **Tests added:** 10 test functions (2 snapshot + 8 caps REST/WS)

## Accomplishments

- **API-05 (GET /api/v1/capabilities):** Returns `{capabilities:[], count:N}` with `Content-Type: application/json`. Supports `?action=PICK|SAVE` (exact-match) and `?mimeType=X/Y` (symmetric wildcard via store.Capabilities). Proven by TestHandleCapabilities, TestHandleCapabilities_ActionFilter, TestHandleCapabilities_MimeTypeFilter (exact + `*/*` wildcard + non-match paths).
- **API-10 (envelope shape, continued):** Malformed `?mimeType=notamime` returns 400 with the standard envelope shape via writeBadRequest → writeJSONError. Error body contains "mime". Proven by TestHandleCapabilities_MalformedMime.
- **WS-01 (WebSocket upgrade):** `GET /api/v1/capabilities/ws` upgrades to WebSocket via `websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: s.cfg.AllowedOrigins})`. InsecureSkipVerify is NEVER set (grep gate: `grep -rn 'InsecureSkipVerify' internal/httpapi/*.go | grep -v _test.go` → EMPTY). Proven by TestHandleCapabilitiesWS_Upgrade.
- **WS-06 (snapshot-on-connect):** The first message on the WebSocket is a SNAPSHOT REGISTRY_UPDATED event carrying the full unfiltered capability list. The write is structurally ordered BEFORE `hub.Subscribe` so the client cannot miss events between connect and subscribe: empty snapshot (TestHandleCapabilitiesWS_Snapshot_Empty), populated snapshot (TestHandleCapabilitiesWS_Snapshot_WithData), and snapshot-then-live-publish continuity (TestHandleCapabilitiesWS_SubscribesAfterSnapshot — seed store, dial, read snapshot, direct Publish, read ADDED event within 5s ctx deadline).
- **Snapshot serialization contract locked:** `newSnapshotEvent` emits `{event:REGISTRY_UPDATED, timestamp:..., payload:{change:SNAPSHOT, capabilities:[...]}}` with NO `appId` field (absent from the dedicated snapshotPayload type) and `capabilities` ALWAYS present as an array (never null, never missing) — proven by TestNewSnapshotEvent (populated) and TestNewSnapshotEvent_EmptyList (zero-length array renders as `"capabilities":[]`, not `"capabilities":null` and not missing).
- **Deleted stub501:** All 6 Phase 4 routes are now real handlers. `grep -c 'stub501' internal/httpapi/server.go` → 0. The negative assertion is a Plan-05+ invariant going forward.
- **Architectural gates still hold:**
  - `~/sdk/go1.26.2/bin/go list -deps ./internal/registry | grep -E 'wshub|httpapi'` → EMPTY
  - `~/sdk/go1.26.2/bin/go list -deps ./internal/wshub | grep -E 'registry|httpapi'` → EMPTY
  - `grep -rE 'slog\.Default' internal/httpapi/*.go | grep -v _test.go` → EMPTY (no global default logger)
  - `grep -rn 'InsecureSkipVerify' internal/httpapi/*.go | grep -v _test.go` → EMPTY (PITFALLS #7)

## Task Commits

1. **Task 1: RED — failing tests for caps REST + WS upgrade + snapshot + newSnapshotEvent** — `47dc5fb` (test)
2. **Task 2: GREEN — implement handlers_caps.go + newSnapshotEvent + Hijacker shim + wire routes** — `23e9f79` (feat)

## Files Created/Modified

**Production code:**
- `internal/httpapi/handlers_caps.go` (created) — 3 functions: `handleCapabilities` (filter parsing + CanonicalizeMIME pre-validation + store.Capabilities call + JSON encode), `buildFullStateSnapshot` (store.Capabilities with empty filter → newSnapshotEvent), `handleCapabilitiesWS` (websocket.Accept → 5s-timeout conn.Write(snapshot) → hub.Subscribe(r.Context(), conn))
- `internal/httpapi/events.go` (modified) — added `snapshotPayload` struct (Change + Capabilities, NO omitempty on Capabilities), `snapshotEvent` envelope, and `newSnapshotEvent(caps)` helper that nil-coerces caps to `[]registry.CapabilityView{}` before marshaling
- `internal/httpapi/server.go` (modified) — registerRoutes: replaced `s.stub501` for the two capabilities routes with `s.handleCapabilities` / `s.handleCapabilitiesWS`; deleted `stub501` function entirely
- `internal/httpapi/middleware.go` (modified, Rule 1 auto-fix) — `statusCapturingWriter.Hijack()` added: forwards to inner writer's http.Hijacker, promotes captured status to 101 on success; now imports `bufio`, `errors`, `net`

**Tests:**
- `internal/httpapi/handlers_caps_test.go` (created) — 8 test functions + `seedManifest` helper: TestHandleCapabilities (REST shape), TestHandleCapabilities_ActionFilter (PICK/SAVE), TestHandleCapabilities_MimeTypeFilter (exact/wildcard/non-match), TestHandleCapabilities_MalformedMime (400 envelope), TestHandleCapabilitiesWS_Upgrade, TestHandleCapabilitiesWS_Snapshot_Empty, TestHandleCapabilitiesWS_Snapshot_WithData, TestHandleCapabilitiesWS_SubscribesAfterSnapshot
- `internal/httpapi/events_test.go` (modified) — added TestNewSnapshotEvent (populated), TestNewSnapshotEvent_EmptyList (`"capabilities":[]` substring assertion, and NotContains `"capabilities":null`), and the `registry` package import

## Decisions Made

1. **Separate snapshotPayload type rather than reusing eventPayload for SNAPSHOT events** — Go's `encoding/json` omitempty tag drops slices with `len == 0`, not just `nil` slices. So `Capabilities: []registry.CapabilityView{}` on an omitempty-tagged field still omits the JSON key. The two-omitempty-fields-on-one-struct pattern from Plan 04-03 worked for upsert/delete events (AppID populated, Capabilities absent) but failed the "capabilities ALWAYS renders as `[]` on snapshot, never null or missing" contract. The fix is a dedicated `snapshotPayload` struct with NO omitempty on Capabilities, wrapped in a dedicated `snapshotEvent` envelope. The two types share the exact same wire shape when populated; the ONLY difference is zero-value serialization. Unmarshal-side code (tests) still uses the original `registryUpdatedEvent` target because Unmarshal doesn't care about omitempty. This is a one-way marshal-only divergence that cleanly isolates the "never emit null" guarantee to exactly the code paths that need it.

2. **Deleted stub501 entirely** — All 6 Phase 4 routes are now wired to real handlers. Keeping stub501 with a "no longer wired" comment would confuse reviewers scanning for unwired routes. `grep -c 'stub501' internal/httpapi/server.go` → 0 is now a negative invariant for Plan 04-05. If stub501 ever reappears, it means a regression where a real handler got unwired.

3. **Snapshot write uses a 5s WriteTimeout wrapped around r.Context()** — The snapshot send is bounded separately from the subsequent `hub.Subscribe` call so a wedged peer during snapshot delivery can't stall the handler goroutine indefinitely. The ctx.WithTimeout chains off r.Context so peer TCP disconnect during snapshot send still cancels promptly via the parent context. On snapshot write failure, we close with StatusInternalError and Warn-log the error — we do NOT fall through to hub.Subscribe on a broken conn.

4. **[Rule 1 - Bug] statusCapturingWriter http.Hijacker shim** — First run of the TDD GREEN code surfaced 5 test failures, 4 of which were `TestHandleCapabilitiesWS_*` reporting "expected handshake response status code 101 but got 501". The root cause was that `coder/websocket.Accept` calls `w.(http.Hijacker)` on the response writer to take over the TCP connection; when the assertion fails it writes a 501 Not Implemented response. `statusCapturingWriter` in logMiddleware (added in Plan 04-01) did not forward the Hijacker interface, so every WebSocket upgrade through the middleware chain was silently failing. The bug was latent since Plan 04-01 because no WebSocket route existed until this plan. Fix: 15-line `Hijack()` method that forwards to the inner writer and promotes the captured status to 101 on a successful hijack so the request log reflects the protocol switch rather than the default 200. This is a generalizable rule: any middleware that wraps `http.ResponseWriter` MUST forward the full stdlib interface set (http.Hijacker, http.Flusher, http.Pusher, http.CloseNotifier — the ones net/http reflects against) or it will silently break innocent downstream handlers. The Plan 04-03 handlers all called `w.Header().Set`, `w.WriteHeader`, `w.Write` and never needed Hijacker, which is why the bug didn't surface until now.

5. **CanonicalizeMIME pre-validation in the handler, not the store** — Store.Capabilities has documented since Plan 02-03 that malformed MimeType filters return empty slices (not errors). The HTTP handler is the layer that wants a 400 on malformed input; the handler pre-validates via the exported CanonicalizeMIME wrapper and writes the 400 envelope via writeBadRequest. This keeps the store 400-free and gives REST clients a clear error envelope. The pre-validated (canonicalized) MIME is then passed back into `filter.MimeType` so store.Capabilities sees a canonical form, avoiding double-canonicalization inside the store.

6. **SubscribesAfterSnapshot test uses direct store.Upsert + hub.Publish (not REST)** — The test seeds the store via `seedManifest` and calls `srv.hub.Publish(newRegistryUpdatedEvent(...))` directly rather than going through POST /api/v1/registry with auth. This isolates the WS snapshot-then-subscribe ordering from the full mutation-then-broadcast pipeline. Plan 04-05 adds the integrated end-to-end test that goes through HTTP POST → store → hub → WS read, which exercises the full stack. Splitting the two concerns this way keeps each test focused and fast (this one is <100ms).

7. **Comment-grep-gate collision (InsecureSkipVerify)** — The grep gate `grep -rn 'InsecureSkipVerify' internal/httpapi/*.go | grep -v _test.go` initially failed because handlers_caps.go referenced the knob by name in two doc comments ("InsecureSkipVerify is NEVER set. PITFALLS #7 anchor."). This is the fourth instance across phases of a gate tripping on its own documentation (Phase 3 hit this with slog.Default and time.Sleep twice). Rule: **always grep-check comments before committing gate-sensitive files.** Fix: reword to "origin-skip knob" and "The origin-skip knob on AcceptOptions is deliberately omitted (PITFALLS #7 anchor); reviewers should grep the source tree for that knob to prove it's absent in production code." Semantic meaning preserved; literal substring gone.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] statusCapturingWriter missing http.Hijacker implementation**
- **Found during:** Task 2 GREEN verification — first `go test` run returned 4 `TestHandleCapabilitiesWS_*` failures with "expected handshake response status code 101 but got 501".
- **Issue:** `logMiddleware` wraps `http.ResponseWriter` in `statusCapturingWriter`, which did not forward the `http.Hijacker` interface. `coder/websocket.Accept` uses `w.(http.Hijacker)` to take over the connection; when the assertion failed it wrote a 501 response and the upgrade aborted. The bug was latent since Plan 04-01 because no WebSocket route existed until this plan.
- **Fix:** Added a `Hijack() (net.Conn, *bufio.ReadWriter, error)` method to `statusCapturingWriter` that forwards to the inner writer's Hijacker (with a clear error if the inner writer doesn't support hijacking) and promotes the captured status to 101 on success so the request log reflects the protocol switch. Added `bufio`, `errors`, `net` imports.
- **Files modified:** `internal/httpapi/middleware.go`
- **Verification:** All 4 WS tests pass after the shim; full httpapi suite still race-clean in ~77s.
- **Committed in:** `23e9f79` (Task 2 GREEN commit, alongside the production code)

**2. [Rule 1 - Bug] newSnapshotEvent using omitempty-tagged field drops `"capabilities":[]` on empty slices**
- **Found during:** Task 2 GREEN verification — `TestNewSnapshotEvent_EmptyList` failed with output `{"change":"SNAPSHOT"}` (capabilities key missing).
- **Issue:** The plan verbatim reused the existing `eventPayload` struct whose `Capabilities` field was tagged `omitempty` by Plan 04-03 (so upsert/delete events don't emit the key). Go's `encoding/json` omitempty drops slices of length 0, not just nil slices, so `Capabilities: []registry.CapabilityView{}` still omitted the key. The test required `"capabilities":[]` to render because WebSocket consumers do `state = event.payload.capabilities` and expect an array they can iterate.
- **Fix:** Created dedicated `snapshotPayload` struct (Change + Capabilities, NO omitempty on Capabilities) and `snapshotEvent` envelope. Updated `newSnapshotEvent` to marshal the new types. The two payload types share the exact same wire shape when populated; the only difference is zero-value serialization. Test-side Unmarshal into `registryUpdatedEvent` still works because Unmarshal ignores omitempty tags.
- **Files modified:** `internal/httpapi/events.go`
- **Verification:** `TestNewSnapshotEvent_EmptyList` passes; `TestEventPayload_OmitemptyCapabilities` (from Plan 04-03) still passes because upsert events still use the old eventPayload struct.
- **Committed in:** `23e9f79` (Task 2 GREEN commit, alongside the production code)

**3. [Rule 3 - Blocking] InsecureSkipVerify grep gate tripped on source comments**
- **Found during:** Task 2 GREEN verification — architectural gate check failed with two hits in handlers_caps.go doc comments.
- **Issue:** The plan's verbatim code included two comments referencing "InsecureSkipVerify" by name to document that the knob was intentionally absent. The grep gate (`grep -rn 'InsecureSkipVerify' internal/httpapi/*.go | grep -v _test.go`) is a literal substring search that doesn't distinguish between code and comments, so the gate failed even though the knob was not actually set anywhere in the source.
- **Fix:** Reworded both comments to use "origin-skip knob" instead of the literal constant name. The semantic meaning (the knob is intentionally not set) is preserved; the grep-visible literal is gone.
- **Files modified:** `internal/httpapi/handlers_caps.go`
- **Verification:** `grep -rn 'InsecureSkipVerify' internal/httpapi/*.go | grep -v _test.go` → EMPTY
- **Committed in:** `23e9f79` (Task 2 GREEN commit, alongside the production code)

---

**Total deviations:** 3 auto-fixed (2 Rule 1 bugs, 1 Rule 3 blocking gate collision)
**Impact on plan:** Three test-run iterations instead of one. The two Rule 1 bugs are both latent issues that the plan couldn't have anticipated: the Hijacker shim was a Plan 04-01 latent bug, and the omitempty-vs-empty-slice issue is a Go json package subtlety. Neither required architectural change or scope creep. The Rule 3 gate collision is the fourth instance of the "grep gates trip on their own documentation" pattern (Phase 3 hit slog.Default and time.Sleep twice; this is Phase 4's first).

## Issues Encountered

None beyond the deviations above. The plan's interface specs for handleCapabilities, buildFullStateSnapshot, and handleCapabilitiesWS were byte-accurate; the acceptance criteria caught both real bugs on the first run.

## Verification Results

**Full plan verification (from the plan's `<verification>` block):**

- `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -count=1 -timeout 120s` — **PASS** (77.2s, all 66 tests including 10 new caps tests)
- `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestHandleCapabilities|^TestNewSnapshotEvent' -timeout 30s` — **PASS** (3.2s, 10 plan-specific tests)
- `! ~/sdk/go1.26.2/bin/go list -deps ./internal/registry | grep -E 'wshub|httpapi'` — **PASS** (registry isolation holds)
- `! ~/sdk/go1.26.2/bin/go list -deps ./internal/wshub | grep -E 'registry|httpapi'` — **PASS** (wshub isolation holds)
- `! grep -rn 'InsecureSkipVerify' internal/httpapi/*.go | grep -v _test.go` — **PASS** (empty after Rule 3 comment reword)
- `! grep -rE 'slog\.Default' internal/httpapi/*.go | grep -v _test.go` — **PASS** (no global default logger)
- `~/sdk/go1.26.2/bin/go vet ./internal/httpapi/...` — **PASS**
- `test -z "$(~/sdk/go1.26.2/bin/gofmt -l internal/httpapi/)"` — **PASS**
- `~/sdk/go1.26.2/bin/go test ./... -race -count=1 -timeout 240s` — **PASS** (all 5 packages green: config 1.0s, httpapi 76.8s, registry 1.3s, wshub 16.9s)

**Snapshot-before-subscribe static ordering check:**

```
=== handleCapabilitiesWS body ===
17:	err = conn.Write(writeCtx, websocket.MessageText, snapshot)
28:	_ = s.hub.Subscribe(r.Context(), conn)
```

The `conn.Write(snapshot)` line number (17) is strictly less than the `s.hub.Subscribe` line number (28) within the function body. WS-06 ordering is mechanically auditable.

**Acceptance criteria spot checks:**

- `grep -c '^func newSnapshotEvent' internal/httpapi/events.go` → **1**
- `grep -c 'caps = \[\]registry.CapabilityView{}' internal/httpapi/events.go` → **1**
- `grep -c '^func (s \*Server) handleCapabilities(' internal/httpapi/handlers_caps.go` → **1**
- `grep -c '^func (s \*Server) handleCapabilitiesWS(' internal/httpapi/handlers_caps.go` → **1**
- `grep -c '^func (s \*Server) buildFullStateSnapshot()' internal/httpapi/handlers_caps.go` → **1**
- `grep -c 'registry.CanonicalizeMIME' internal/httpapi/handlers_caps.go` → **1**
- `grep -c 'OriginPatterns: s.cfg.AllowedOrigins' internal/httpapi/handlers_caps.go` → **1**
- `grep -c 's.handleCapabilities' internal/httpapi/server.go` → **2** (REST + WS routes)
- `grep -c 'stub501' internal/httpapi/server.go` → **0** (deleted entirely)

**Named-test validation (10 new test functions):**

- `TestNewSnapshotEvent` → **PASS**
- `TestNewSnapshotEvent_EmptyList` → **PASS**
- `TestHandleCapabilities` → **PASS**
- `TestHandleCapabilities_ActionFilter` → **PASS**
- `TestHandleCapabilities_MimeTypeFilter` → **PASS** (exact + wildcard + non-match)
- `TestHandleCapabilities_MalformedMime` → **PASS** (400 envelope)
- `TestHandleCapabilitiesWS_Upgrade` → **PASS**
- `TestHandleCapabilitiesWS_Snapshot_Empty` → **PASS**
- `TestHandleCapabilitiesWS_Snapshot_WithData` → **PASS**
- `TestHandleCapabilitiesWS_SubscribesAfterSnapshot` → **PASS**

Total: 10 new tests + pre-existing 56 httpapi tests from Plans 04-01/04-02/04-03 = 66 tests, full suite race-clean in 77.2s.

## User Setup Required

None.

## Next Phase Readiness

**Ready for Plan 04-05 (CORS + integration tests):**
- All 6 Phase 4 routes are wired to real handlers; `grep -c stub501 internal/httpapi/server.go` → 0 is a stable invariant.
- The `corsMiddleware` pass-through placeholder in middleware.go is still in place (Plan 04-01 comment: "Plan 04-05 replaces this with the real rs/cors wrap"). Plan 04-05 swaps the body in place without touching the chain order.
- The WS-08 shared allow-list contract is half-landed: `handleCapabilitiesWS` uses `s.cfg.AllowedOrigins` via `websocket.AcceptOptions{OriginPatterns}`. Plan 04-05 lands the second half by wiring the same `s.cfg.AllowedOrigins` into `rs/cors.AllowedOrigins`.
- The full-suite baseline is 77s under -race. Plan 04-05 will add CORS round-trip tests and the big POST→WS end-to-end integration test, which should extend the suite by ~5-10s.
- **Note for Plan 04-05:** the Hijacker shim on statusCapturingWriter is now part of the middleware contract. If Plan 04-05 adds any new ResponseWriter wrappers, they MUST also forward http.Hijacker (or the CORS middleware's ResponseWriter wrap, if any, will break the WS path again). Add this to the plan's PITFALLS checklist.

**No blockers or concerns.**

## Self-Check: PASSED

Verified:
- `internal/httpapi/handlers_caps.go` — FOUND
- `internal/httpapi/handlers_caps_test.go` — FOUND
- `internal/httpapi/events.go` — FOUND (modified)
- `internal/httpapi/events_test.go` — FOUND (modified)
- `internal/httpapi/server.go` — FOUND (modified, stub501 deleted)
- `internal/httpapi/middleware.go` — FOUND (modified, Hijacker shim added)
- `.planning/phases/04-http-api/04-04-SUMMARY.md` — FOUND (this file)
- Commit `47dc5fb` (Task 1 RED) — FOUND in git log
- Commit `23e9f79` (Task 2 GREEN) — FOUND in git log

---
*Phase: 04-http-api*
*Completed: 2026-04-10*
