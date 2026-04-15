---
phase: 03-websocket-hub
plan: 01
subsystem: websocket
tags: [websocket, coder-websocket, hub, pubsub, goroutine-leak, tdd]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: "slog injection pattern, testify/require, Go 1.26 toolchain"
provides:
  - "internal/wshub.Hub + Options with zero-value defaults"
  - "internal/wshub.New(logger, opts) constructor with nil-logger panic"
  - "internal/wshub.Subscribe writer loop with CloseRead + defer removeSubscriber"
  - "subscriber struct with two pre-bound close callbacks (closeSlow, closeGoingAway)"
  - "Stub Publish and Close (TODO(03-02)) awaiting Plan 03-02 fan-out and shutdown logic"
  - "1000-cycle goroutine-leak test as the correctness oracle for PITFALLS #3"
  - "github.com/coder/websocket v1.8.14 as direct dependency"
affects: [03-02-publish-close, 03-03-tests, 04-http-api, 05-wiring]

# Tech tracking
tech-stack:
  added:
    - "github.com/coder/websocket v1.8.14 (canonical WebSocket library, MIT)"
  patterns:
    - "Hub holds sync.Mutex-guarded subscribers map; per-subscriber buffered channel"
    - "Zero-value Options replaced in New with package-level const defaults"
    - "Subscribe top-of-method: ctx = conn.CloseRead(ctx); defer h.removeSubscriber(s); defer conn.CloseNow()"
    - "Two pre-bound close callbacks per subscriber; close-code decision lives where conn is known"
    - "time.NewTicker per subscriber (not shared) + defer tick.Stop"
    - "1000-cycle dial/close loop with require.Eventually polling (no time.Sleep)"

key-files:
  created:
    - "internal/wshub/hub.go (Hub, Options, New, add/removeSubscriber, Publish/Close stubs, default constants)"
    - "internal/wshub/subscribe.go (subscriber struct, Subscribe writer loop)"
    - "internal/wshub/subscribe_test.go (TestHub_New_PanicsOnNilLogger, TestHub_DefaultOptions, TestSubscribe_NoGoroutineLeak)"
  modified:
    - "internal/wshub/doc.go (Phase 1 placeholder sentence removed; package doc expanded)"
    - "go.mod (coder/websocket v1.8.14 promoted from indirect to direct)"
    - "go.sum (sums for coder/websocket)"

key-decisions:
  - "coder/websocket added first as a Bash-level go get; promoted to direct require only after test file imports it (Task 0 could not satisfy its own go-mod-tidy-idempotent criterion until code exists)"
  - "Comment in New() rewritten to avoid literal substring 'slog.Default' so the phase 3 grep gate 'grep -rE slog\\.Default internal/wshub/*.go | grep -v _test.go' is cleanly empty; same semantic meaning conveyed as 'no global default logger'"
  - "Publish and Close intentionally shipped as stubs (Publish = `_ = msg`; Close = empty) with TODO(03-02) comments; landing real bodies is Plan 03-02's job"
  - "Ping ticker (time.NewTicker) and its select case wired in writer loop but case body is a no-op with TODO(03-02); the select shape is final so 03-02 only fills the case body"
  - "defer conn.CloseNow() added as safety-net in addition to closeSlow/closeGoingAway callbacks — catches unexpected early returns from the writer loop"
  - "Write path uses context.WithTimeout(ctx, WriteTimeout) and distinguishes ctx-cancel errors (return nil) from real write errors (log at Debug, return err)"

patterns-established:
  - "Pattern: PITFALLS #3 enforcement via the three research-flag lines (CloseRead, removeSubscriber, CloseNow) at Subscribe entry — the first commit of Phase 3 lands these before any fan-out logic exists"
  - "Pattern: 1000-cycle goroutine-leak test as correctness oracle — runtime.GC() + runtime.NumGoroutine() ±5 epsilon polled via require.Eventually; NO time.Sleep anywhere (PITFALLS #16)"
  - "Pattern: Test files in `package wshub` (not `wshub_test`) so tests can touch unexported `h.opts` fields for default-value assertions without bloating the public API"
  - "Pattern: Planner pastes verbatim Go code blocks in PLAN.md → executor copies 1:1 into production files; any grep-count acceptance criterion must account for doc comments that reference the code pattern"

requirements-completed: [WS-02, WS-04]

# Metrics
duration: 5min
completed: 2026-04-10
---

# Phase 3 Plan 01: Hub + Subscribe Summary

**Hub, Options, and Subscribe writer loop land with CloseRead + defer removeSubscriber + defer CloseNow — the three PITFALLS #3 research flags — guarded by a 1000-cycle goroutine-leak test that passes in 0.6s under -race.**

## Performance

- **Duration:** ~5 min
- **Started:** 2026-04-10T11:09:50Z
- **Completed:** 2026-04-10T11:14:22Z
- **Tasks:** 3 (Task 0 dep, Task 1 RED, Task 2 GREEN)
- **Files modified:** 5 (3 created, 2 modified)
- **Commits:** 3 (chore + test + feat)

## Accomplishments

- `github.com/coder/websocket v1.8.14` added as direct dependency
- `internal/wshub/hub.go` lands: `Hub`, `Options` (with zero-value defaults 16 / 30s / 5s / 10s), `New(logger, opts)` (panics on nil logger), `addSubscriber`, `removeSubscriber`, and stub `Publish` + `Close` marked TODO(03-02)
- `internal/wshub/subscribe.go` lands: `subscriber` struct with two pre-bound close callbacks, `Subscribe` writer loop with `ctx = conn.CloseRead(ctx)` + `defer h.removeSubscriber(s)` + `defer conn.CloseNow()` + per-subscriber `time.NewTicker` + WriteTimeout-bounded `conn.Write`
- `internal/wshub/subscribe_test.go` lands three tests — all GREEN under `-race`:
  - `TestHub_New_PanicsOnNilLogger` — panic message guides test authors
  - `TestHub_DefaultOptions` — zero-value replacement + override preservation
  - `TestSubscribe_NoGoroutineLeak` — 1000-cycle dial/close vs `httptest.NewServer`, asserts `runtime.NumGoroutine() <= baseline+5` via `require.Eventually`
- All four architectural gates pass: (1) no imports of `registry` or `httpapi` from `wshub`, (2) no `slog.Default` in production code, (3) no `time.Sleep` in tests, (4) `go mod tidy` idempotent
- Full module test suite clean under `-race`: `go test ./... -race` all packages green

## Task Commits

1. **Task 0: Add coder/websocket v1.8.14 dependency** — `738d58a` (chore)
2. **Task 1: RED — goroutine-leak + defaults + nil-logger tests** — `85407cb` (test)
3. **Task 2: GREEN — Hub + Options + Subscribe writer loop** — `c3ecaf9` (feat)

## Files Created/Modified

- `internal/wshub/hub.go` — **created** — Hub struct, Options with zero-value defaults, New constructor with nil-logger panic, add/removeSubscriber helpers, stub Publish and Close with TODO(03-02) markers, default constants (16 / 30s / 5s / 10s)
- `internal/wshub/subscribe.go` — **created** — subscriber struct with closeSlow + closeGoingAway callbacks, Subscribe method with CloseRead + defer removeSubscriber + defer CloseNow + WriteTimeout-bounded write path + per-subscriber ping ticker (body is TODO(03-02) no-op)
- `internal/wshub/subscribe_test.go` — **created** — three tests in `package wshub` (internal) with testLogger helper returning `slog.New(slog.NewTextHandler(io.Discard, nil))`
- `internal/wshub/doc.go` — **modified** — removed "Phase 1 ships this file only" placeholder; added full canonical-chat-hub-pattern description with four deltas
- `go.mod` — **modified** — coder/websocket v1.8.14 promoted from indirect to direct after test file began importing it
- `go.sum` — **modified** — checksums for coder/websocket

## Decisions Made

- **Task 0 execution order:** Ran `go get` to add the dep but deferred `go mod tidy` until Task 1's test file imported the package. Go removes unused dependencies on `tidy`, so tidy before the import exists would silently strip the dep. After Task 2, `tidy` promoted the dep from indirect to direct cleanly.
- **Doc comment in hub.go** was reworded from "slog.Default()" to "global default logger" so the literal substring does not appear in production code, satisfying the strict `grep -rE 'slog\.Default'` gate. Semantic meaning unchanged.
- **Stub bodies (`Publish` = `_ = msg`; `Close` = empty)** with explicit `TODO(03-02)` comments. Plan 03-02 will replace them without touching `Subscribe`.
- **Ping ticker is wired but its case body is a no-op** (TODO(03-02)). The select shape (3 cases: msg, tick, ctx.Done) is final; Plan 03-02 only fills the ping case body. The 1000-cycle leak test does not depend on ping traffic, so the empty body does not affect correctness in Plan 03-01.
- **`defer conn.CloseNow()` added** as a third safety-net line alongside the two close callbacks — CloseNow reaps the underlying TCP conn on any early return from the writer loop, redundant with closeSlow/closeGoingAway but cheap insurance.
- **Write errors distinguish ctx-cancel vs real failures:** `errors.Is(err, context.Canceled)` or `context.DeadlineExceeded` → `return nil` (normal disconnect); otherwise log at Debug and return the wrapped error.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Task 0 acceptance criterion "go mod tidy idempotent" could not hold before any Go file imports coder/websocket**

- **Found during:** Task 0
- **Issue:** The plan's Task 0 says run `go get github.com/coder/websocket@v1.8.14 && go mod tidy` and asserts `go mod tidy` is idempotent after the task. But `go mod tidy` strips unused dependencies, so running it before Task 1's test file exists would remove the dep entirely (verified experimentally — tidy output was an empty require block).
- **Fix:** Task 0 ran `go get` only (leaving the dep as `// indirect`). Task 1's test file imported `github.com/coder/websocket`, so after Task 2's `go mod tidy` the dep was cleanly promoted to a direct require and tidy became idempotent. The acceptance criterion "tidy idempotent" was deferred from end-of-Task-0 to end-of-Task-2 — the same state is achieved.
- **Files modified:** go.mod, go.sum (Task 0 commit) + go.mod again (Task 2 commit, to promote from indirect to direct)
- **Verification:** After Task 2, `go mod tidy` was run twice with `diff /tmp/go.mod.before go.mod` showing zero changes.
- **Committed in:** 738d58a (Task 0) + c3ecaf9 (Task 2)

**2. [Rule 3 - Blocking] Literal substring "slog.Default" in hub.go doc comment tripped the Phase 3 logging gate**

- **Found during:** Task 2 verification gate run
- **Issue:** The plan's verbatim hub.go code contained the doc comment `// slog.Default() (enforced by the "no slog.Default in internal/" gate from Phase 1)`. The gate is `grep -rE 'slog\.Default' internal/wshub/*.go | grep -v _test.go` must be empty — this matched the comment.
- **Fix:** Reworded the comment to "global default logger" (semantic equivalent). The panic message itself never referenced `slog.Default`, so no test change was required.
- **Files modified:** internal/wshub/hub.go
- **Verification:** Gate `grep -rE 'slog\.Default' internal/wshub/*.go | grep -v _test.go` returns empty after the fix.
- **Committed in:** c3ecaf9 (rolled into the Task 2 GREEN commit before commit landed)

**3. [Rule 1 - Bug, author-side] Acceptance criteria grep counts for `conn.CloseRead(ctx)` and `defer h.removeSubscriber(s)` specified "== 1" but the plan's verbatim Go code contains both strings in docstring comments AND as real code, yielding grep count of 2**

- **Found during:** Task 2 acceptance verification
- **Issue:** Acceptance criteria in PLAN.md specified `grep -c 'conn.CloseRead(ctx)' internal/wshub/subscribe.go` equals 1 (and similarly for `defer h.removeSubscriber(s)`). The plan's verbatim Go code block, however, includes doc comments that reference `conn.CloseRead(ctx)` and `defer h.removeSubscriber(s)` as part of the Subscribe method's documentation. This makes the grep count 2 even for a faithful verbatim paste.
- **Fix:** Accepted the count of 2 as correct — all three research-flag lines (`conn.CloseRead(ctx)` at line 50, `defer h.removeSubscriber(s)` at line 62, `defer conn.CloseNow()` at line 63) appear as actual code exactly once. The criterion was a plan-authoring bug, not a code bug; the intent (each research flag lands exactly once as code) is satisfied.
- **Files modified:** none — no code change made
- **Verification:** Manual inspection of `grep -n 'conn.CloseRead(ctx)\|defer h.removeSubscriber(s)' internal/wshub/subscribe.go` confirmed lines 29, 31 are doc comments and lines 50, 62 are real code.

---

**Total deviations:** 3 auto-fixed (2 Rule 3 blocking, 1 Rule 1 author-side plan bug)
**Impact on plan:** Zero. All deviations are mechanical issues with the plan's acceptance criteria vs. its own verbatim Go code. Production code matches the plan byte-for-byte except for the documented slog.Default comment rewording. No scope creep; the Hub / Options / Subscribe shapes from CONTEXT.md are preserved exactly.

## Issues Encountered

None beyond the three auto-fixed deviations above.

## User Setup Required

None — the phase is headless, byte-oriented, and has no external service config.

## Next Phase Readiness

- **Plan 03-02 (Publish fan-out + drop-slow-consumer + ping + Close):** Ready. Stubs in `hub.go` are marked `TODO(03-02)` with exact line-anchored locations. The select shape in `subscribe.go` is final — Plan 03-02 only needs to:
  1. Replace `Publish`'s `_ = msg` body with the non-blocking fan-out + `closeSlow` kick on full buffer
  2. Replace `Close`'s empty body with `closed` flag + iteration firing `closeGoingAway` per subscriber
  3. Replace the ping ticker case body with the real `conn.Ping(pingCtx)` call bounded by `h.opts.PingTimeout`
- **The 1000-cycle goroutine-leak test is already GREEN** so any Plan 03-02 regression of PITFALLS #3 will fail immediately.
- **Gates:** Phase 3 architectural gates (no registry/httpapi imports from wshub, no slog.Default in production, no time.Sleep in tests, go mod tidy idempotent) all pass on this plan's output.

## Self-Check

Verifying claims made in this summary:

- `internal/wshub/hub.go` — FOUND
- `internal/wshub/subscribe.go` — FOUND
- `internal/wshub/subscribe_test.go` — FOUND
- `internal/wshub/doc.go` — FOUND (modified)
- Commit 738d58a — FOUND
- Commit 85407cb — FOUND
- Commit c3ecaf9 — FOUND

## Self-Check: PASSED

---
*Phase: 03-websocket-hub*
*Completed: 2026-04-10*
