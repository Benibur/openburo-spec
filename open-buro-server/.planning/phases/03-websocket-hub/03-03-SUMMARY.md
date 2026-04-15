---
phase: 03-websocket-hub
plan: 03
subsystem: websocket
tags: [websocket, slog, log-capture, observability, architectural-gates, tdd, phase-gate]

# Dependency graph
requires:
  - phase: 03-websocket-hub
    provides: "Hub.Publish with frozen Warn('wshub: subscriber dropped (slow consumer)', 'buffer_size', ...) log line; Hub.Close with frozen Info('wshub: closing hub', 'subscribers', ...) log line; both shipped in Plan 03-02"
provides:
  - "TestHub_Logging_DropIsWarn: captures slog via TextHandler, asserts level=WARN + exact message + buffer_size field + no PII"
  - "TestHub_Logging_CloseIsInfo: asserts level=INFO + exact message + subscribers=0 + exactly one line + no WARN/ERROR + no PII"
  - "TestHub_Logging_NoPII: full connect → publish → drop → close lifecycle with no-PII assertion across entire captured buffer"
  - "piiSubstrings list + requireNoPII helper freezing the no-PII contract (peer_addr, remote_addr, RemoteAddr, user_agent, User-Agent, authorization, Authorization, Basic, Bearer, username, password)"
  - "syncBuffer (mutex-guarded bytes.Buffer) helper for race-safe log capture across the writer goroutine + test goroutine"
  - ".planning/phases/03-websocket-hub/03-GATES.md: 8-gate sweep results with verbatim command output"
affects: [04-http-api, 05-wiring]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Log capture for contract tests: slog.New(slog.NewTextHandler(&syncBuffer, &slog.HandlerOptions{Level: slog.LevelDebug})) → require.Contains/NotContains on buf.String() — no regex, grep-equivalent substring assertions"
    - "Race-safe log capture: a plain bytes.Buffer is not concurrent-safe; the subscriber writer goroutine logs Debug on exit while the test goroutine reads via buf.String() inside require.Eventually, so log-capture tests MUST wrap bytes.Buffer in a mutex"
    - "No-PII contract via grep-equivalent substring list: piiSubstrings (11 entries) + requireNoPII helper called across all three logging tests — the byte-oriented hub has no notion of client identity, so any PII in logs would be a bug"
    - "Architectural gates as shell commands, not Go tests: go list -deps + grep (no slog.Default, no time.Sleep, no stale TODOs) run once per phase in a documented GATES.md artifact, not as per-commit test runs"

key-files:
  created:
    - ".planning/phases/03-websocket-hub/03-GATES.md (8-gate sweep results with verbatim go test / grep / go list output)"
  modified:
    - "internal/wshub/hub_test.go (three logging-capture tests + piiSubstrings + requireNoPII + syncBuffer helper appended; strings and sync imports added; 145 lines of tests + helper code)"

key-decisions:
  - "syncBuffer mutex-guarded bytes.Buffer helper added because the plan's verbatim bytes.Buffer code raced under -race: the subscriber writer goroutine logs Debug on exit (via h.logger.Debug in subscribe.go:83) while the test goroutine reads via buf.String() inside require.Eventually. A plain bytes.Buffer is not concurrent-safe. The syncBuffer wrapper (Write + String under one mu) fixes this with minimal surface area."
  - "TestHub_Logging_NoPII's context timeout raised from 3s to 15s to accommodate the 7s slow-consumer drop waitForSubscribers budget (coder/websocket v1.8.14 has a hardcoded 5s waitCloseHandshake, already documented in 03-02 deviations). The 3s in the plan's verbatim code would have expired mid-drop."
  - "TestHub_Logging_NoPII's closing comment reworded from '(no time.Sleep)' to '(polling, not blocking)' because the literal substring 'time.Sleep' in the comment tripped the Phase 3 no-time.Sleep gate. Semantic meaning preserved; the comment still explains why require.Eventually is used instead of a blocking wait."
  - "All three logging tests passed on first run against Plan 03-02's production code (after the syncBuffer race fix) — confirming that the hub.go Warn + Info log line formats were already byte-exact per the plan. The tests are a belt-and-suspenders lock-in, not a gap fix."
  - "Plan 03-03 adds zero production code. All changes are test-side (hub_test.go) + docs-side (03-GATES.md). hub.go and subscribe.go are byte-for-byte unchanged from Plan 03-02's committed state."

patterns-established:
  - "Pattern: Log-capture contract tests freeze the observable log format by assertion — `require.Contains(out, 'level=WARN')` + `require.Contains(out, 'wshub: subscriber dropped (slow consumer)')` + `require.Contains(out, 'buffer_size=1')` collectively pin level, message, and fields; a future refactor that changes any of the three would fail the test"
  - "Pattern: Phase-gate sweep as a committed artifact — .planning/phases/XX/XX-GATES.md captures verbatim command output for every gate; reviewers can grep for 'Status: PASS' or inspect individual gate outputs without re-running"
  - "Pattern: grep gates can trip on their own documentation — comments that mention the forbidden substring (e.g., 'no time.Sleep' as a comment) fail the grep gate. Always grep check your comments before committing"

requirements-completed: [WS-10]

# Metrics
duration: 5min
completed: 2026-04-10
---

# Phase 3 Plan 03: Leak Test + Logging Contract Summary

**Three log-capture tests freeze the observable Warn/Info/no-PII contract from 03-02's production code, and the 8-gate Phase 3 sweep is captured verbatim in 03-GATES.md — Phase 3 is feature-complete and ready for `/gsd:verify-work`.**

## Performance

- **Duration:** ~5 min
- **Started:** 2026-04-10T11:32:24Z
- **Completed:** 2026-04-10T11:37:03Z
- **Tasks:** 2 (Task 1 RED→GREEN logging tests, Task 2 gate sweep doc)
- **Files modified:** 2 (1 test file, 1 new docs file)
- **Commits:** 2 (test + docs)

## Accomplishments

- `TestHub_Logging_DropIsWarn` lands — captures slog output via TextHandler at LevelDebug, asserts `level=WARN`, exact message `wshub: subscriber dropped (slow consumer)`, `buffer_size=1` field, and no PII substrings
- `TestHub_Logging_CloseIsInfo` lands — asserts `level=INFO`, exact message `wshub: closing hub`, `subscribers=0` field, exactly one line via `strings.Count`, no WARN/ERROR levels, no PII
- `TestHub_Logging_NoPII` lands — full connect → publish → drop → close lifecycle with a full-buffer no-PII assertion covering all log lines produced across the flow
- `piiSubstrings` list + `requireNoPII` helper freeze the no-PII contract: 11 substrings (peer_addr, remote_addr, RemoteAddr, user_agent, User-Agent, authorization, Authorization, `Basic `, `Bearer `, username, password) enforced across all three logging tests
- `syncBuffer` mutex-guarded `bytes.Buffer` helper added so log capture is race-safe when the subscriber writer goroutine logs Debug on exit while the test goroutine reads via `buf.String()` inside `require.Eventually`
- `.planning/phases/03-websocket-hub/03-GATES.md` lands — 8-gate sweep with verbatim `go test`, `grep`, and `go list` output:
  - **Gate 1:** Full wshub suite (11 tests) PASS under -race in ~17s
  - **Gate 2:** TestSubscribe_NoGoroutineLeak in isolation PASS in 0.6s
  - **Gate 3:** `go list -deps ./internal/wshub` — no registry/httpapi imports
  - **Gate 4:** `grep -rE 'slog\.Default' internal/wshub/*.go | grep -v _test.go` — empty
  - **Gate 5:** `grep -n 'time\.Sleep' internal/wshub/*_test.go` — empty
  - **Gate 6:** `grep -n 'TODO(03-02)' internal/wshub/*.go` — empty
  - **Gate 7:** build + vet + gofmt — clean
  - **Gate 8:** Whole-module `go test ./... -race` — all packages PASS
- All 11 wshub tests pass under `-race` (4 from 03-01 + 4 from 03-02 + 3 from 03-03)
- **Phase 3 requirements all closed:** WS-02, WS-03, WS-04, WS-07, WS-10

## Task Commits

1. **Task 1: RED → GREEN — three logging-capture tests + piiSubstrings + requireNoPII + syncBuffer** — `2d7c2a5` (test)
2. **Task 2: Gate sweep doc + comment reword** — `fe4bafb` (docs)

## Files Created/Modified

- `internal/wshub/hub_test.go` — **modified** — appended 145 lines: `piiSubstrings` var, `requireNoPII` helper, `syncBuffer` type with Write + String methods, and three `TestHub_Logging_*` test functions; added `strings` and `sync` to the imports block; reworded one comment in TestHub_Logging_NoPII from "(no time.Sleep)" to "(polling, not blocking)" to clear the no-time.Sleep grep gate
- `.planning/phases/03-websocket-hub/03-GATES.md` — **created** — 8-gate sweep results with verbatim output, a summary table, the five Phase 3 requirements marked closed, and the Phase 4/5 deferral list

## Decisions Made

- **`syncBuffer` helper added (not in the plan's verbatim code).** The plan's code uses `var buf bytes.Buffer` directly, which is not concurrent-safe. The race detector caught three WARNINGs in TestHub_Logging_DropIsWarn on first run: the subscriber writer goroutine writes to the buffer (via `h.logger.Debug("wshub: subscriber writer loop exited", ...)` when the slow-consumer closeSlow callback fires and the writer loop observes the close) at the same time the test goroutine reads the buffer via `buf.String()` inside `require.Eventually`. Wrapping the buffer in a mutex (syncBuffer) is the minimal fix and matches the idiomatic Go pattern for concurrent log capture. The plan's 03-02 `TestHub_Publish_AfterCloseIsNoOp` did not hit this bug because it has no active subscribers (Close is called on an empty hub, so no writer goroutines exist).
- **TestHub_Logging_NoPII context timeout raised from 3s to 15s.** The plan's verbatim code used `context.WithTimeout(context.Background(), 3*time.Second)` but the test's flow includes a 7-second slow-consumer drop (`waitForSubscribers(t, hub, 0, 7*time.Second)`). The 3s ctx would expire mid-drop and fail the outbound websocket.Dial's ctx check (though in practice the Dial happens at t=0 and only the Read path would have noticed). Raising to 15s matches the 03-02 TestHub_SlowConsumerDropped ceiling and gives comfortable slack above the 5-second floor from coder/websocket v1.8.14's hardcoded waitCloseHandshake timeout.
- **Comment "(no time.Sleep)" reworded to "(polling, not blocking)".** The Phase 3 `grep -n 'time\.Sleep' internal/wshub/*_test.go` gate matches any literal substring, including a comment that describes the absence of time.Sleep. This is the third instance of Phase 3 gates tripping on their own documentation (the first two were in 03-01: the `slog.Default` doc comment and the research-flag grep-count criterion). Pattern: always grep-check your comments before committing a gate-sensitive file.
- **No production code changes in this plan.** hub.go and subscribe.go are byte-for-byte unchanged from Plan 03-02's committed state (`9a27fa8`). All changes are test-side (`hub_test.go`) + docs-side (`03-GATES.md`). The three logging tests passed on first run after the syncBuffer fix, confirming that 03-02's Warn/Info log line formats were already byte-exact per the plan's `<interfaces>` block.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Plan's verbatim `bytes.Buffer` log-capture code races under -race**

- **Found during:** Task 1 initial test run
- **Issue:** The plan specifies `var buf bytes.Buffer` as the log sink for all three logging tests. When `TestHub_Logging_DropIsWarn` runs under `-race`, the race detector catches the subscriber's writer goroutine writing to the buffer (via `subscribe.go:83` — `h.logger.Debug("wshub: subscriber writer loop exited", ...)` fired when closeSlow kicks the conn) concurrently with the test goroutine reading the buffer (via `buf.String()` inside `require.Eventually`). Three separate DATA RACE warnings were reported on the first run — the test failed hard under `-race`. A plain `bytes.Buffer` is not concurrent-safe per the stdlib docs. This is a genuine bug in the plan's verbatim code, not a hub.go bug.
- **Fix:** Added a minimal `syncBuffer` type to `hub_test.go` wrapping a `bytes.Buffer` + `sync.Mutex`, exposing `Write(p []byte) (int, error)` (Writer interface, required by slog.NewTextHandler) and `String() string`. All three logging tests changed `var buf bytes.Buffer` to `var buf syncBuffer`. The `slog.NewTextHandler(&buf, ...)` call works unchanged because both types implement io.Writer.
- **Files modified:** `internal/wshub/hub_test.go` (added syncBuffer type + `sync` import; swapped 3× `var buf bytes.Buffer` → `var buf syncBuffer`)
- **Verification:** After the fix, all three logging tests pass under `-race` with zero DATA RACE warnings. Full wshub suite (11 tests) passes in ~17s under `-race`.
- **Committed in:** 2d7c2a5 (Task 1 test commit — rolled into initial commit before it landed)

**2. [Rule 3 - Blocking] TestHub_Logging_NoPII's plan ctx timeout (3s) expires before the 7s slow-consumer drop completes**

- **Found during:** Task 1 initial test design
- **Issue:** The plan's verbatim code for TestHub_Logging_NoPII uses `context.WithTimeout(context.Background(), 3*time.Second)` for the Dial context, but the test flow includes `waitForSubscribers(t, hub, 0, 7*time.Second)` waiting for the slow-consumer drop. The 3s ctx would expire after 3s of the 7s drop wait. This is the same structural library property (coder/websocket v1.8.14 hardcodes a 5s waitCloseHandshake) that Plan 03-02 documented in its deviations — the plan authors for 03-03 used the shorter timeout from the research example without cross-referencing 03-02's lesson.
- **Fix:** Raised the `context.WithTimeout` in TestHub_Logging_NoPII from `3*time.Second` to `15*time.Second`, matching the 03-02 TestHub_SlowConsumerDropped ceiling. The other two logging tests (DropIsWarn, CloseIsInfo) kept the 3s ctx because DropIsWarn only waits for the Warn log line to appear (sub-100ms) and CloseIsInfo has no active conn at all.
- **Files modified:** `internal/wshub/hub_test.go` (one-line change, `3*time.Second` → `15*time.Second`)
- **Verification:** TestHub_Logging_NoPII now passes in ~5.01s (dominated by the 5s close handshake wait). No ctx-expiry errors observed in test output.
- **Committed in:** 2d7c2a5 (Task 1 commit)

**3. [Rule 1 - Bug] Comment `(no time.Sleep)` in TestHub_Logging_NoPII trips the no-time.Sleep gate**

- **Found during:** Task 2 gate sweep (Gate 5)
- **Issue:** The plan's verbatim code for TestHub_Logging_NoPII includes a comment `// Give any deferred log writes a chance to flush via require.Eventually` `// (no time.Sleep). The condition is immediately true on entry in the` `// common case; the Eventually loop is just a safety margin.`. The `grep -n 'time\.Sleep' internal/wshub/*_test.go` gate (Gate 5) matches any literal substring, including the comment. The comment describes the absence of time.Sleep, but the gate cannot distinguish comment from code.
- **Fix:** Reworded the comment to `// (polling, not blocking). The condition is immediately true on entry` (same semantic meaning, no literal `time.Sleep` substring). No code changes.
- **Files modified:** `internal/wshub/hub_test.go` (two-line comment reword)
- **Verification:** `grep -n 'time\.Sleep' internal/wshub/*_test.go` now returns empty. TestHub_Logging_NoPII still passes unchanged (comments don't affect runtime behavior).
- **Committed in:** fe4bafb (rolled into the Task 2 docs commit)

---

**Total deviations:** 3 auto-fixed (2 Rule 1 bugs in plan's verbatim test code, 1 Rule 3 blocking ctx timeout)
**Impact on plan:** Zero impact on production code or the observable log contract. The Warn-on-drop + Info-on-close + no-PII contract is frozen exactly as specified in the plan's `<interfaces>` block. All three deviations are test-side mechanics (race-safety, ctx budget, comment wording) that the plan authors missed; the hub.go and subscribe.go files are byte-for-byte unchanged from Plan 03-02's commit.

## Issues Encountered

None beyond the three auto-fixed deviations above. The race-safety fix (syncBuffer) is a well-known pattern and the comment reword is cosmetic; neither blocks Phase 3 completion.

## User Setup Required

None — the phase is headless, byte-oriented, and has no external service config.

## Next Phase Readiness

- **Phase 3 is feature-complete and ready for `/gsd:verify-work`.** All five Phase 3 requirements (WS-02, WS-03, WS-04, WS-07, WS-10) are implemented, tested, and observably verified by specific named tests. The `.planning/phases/03-websocket-hub/03-GATES.md` artifact captures all 8 architectural gates as PASS with verbatim command output.
- **Phase 4 (HTTP API):** Ready with zero blockers. `wshub.Hub.Publish([]byte)`, `wshub.Hub.Subscribe(ctx, conn)`, and `wshub.Hub.Close()` are production-stable APIs. Phase 4's HTTP handler can call `websocket.Accept(w, r, opts)` then `hub.Subscribe(r.Context(), conn)`; the mutation-then-broadcast flow (WS-05, WS-09) can call `hub.Publish(eventBytes)` after successful `store.Upsert`/`store.Delete`. The byte-oriented contract is structurally enforced (Gate 3: no registry/httpapi imports from wshub).
- **Phase 5 (compose-root + shutdown):** Ready. `hub.Close()` is idempotent, logs Info once, and returns immediately (the 5s close handshakes happen in spawned goroutines off the hub mutex). Phase 5's two-phase shutdown can `defer hub.Close()` after `httpSrv.Shutdown(ctx)` without race concerns.
- **The no-PII log contract is now frozen by test assertion.** Any future refactor that adds `peer_addr`, `user_agent`, or `Authorization` to a wshub log line will fail TestHub_Logging_NoPII. The contract is no longer just a convention from 03-CONTEXT.md — it's enforced code.
- **The Warn-on-drop + Info-on-close format contract is similarly frozen.** Any future refactor that changes `Warn` to `Info`, drops the `buffer_size` field, or changes the exact message string will fail TestHub_Logging_DropIsWarn or TestHub_Logging_CloseIsInfo.

## Self-Check

Verifying claims made in this summary:

- `internal/wshub/hub_test.go` — FOUND (modified, +145 lines for logging tests + helpers)
- `.planning/phases/03-websocket-hub/03-GATES.md` — FOUND (created)
- Commit 2d7c2a5 — FOUND (Task 1 test)
- Commit fe4bafb — FOUND (Task 2 docs + comment reword)
- `grep -c 'func TestHub_Logging_DropIsWarn' internal/wshub/hub_test.go` → 1 (verified)
- `grep -c 'func TestHub_Logging_CloseIsInfo' internal/wshub/hub_test.go` → 1 (verified)
- `grep -c 'func TestHub_Logging_NoPII' internal/wshub/hub_test.go` → 1 (verified)
- `grep -c 'var piiSubstrings' internal/wshub/hub_test.go` → 1 (verified)
- `grep -c 'func requireNoPII' internal/wshub/hub_test.go` → 1 (verified)

## Self-Check: PASSED

---
*Phase: 03-websocket-hub*
*Completed: 2026-04-10*
