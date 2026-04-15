---
phase: 03-websocket-hub
plan: 02
subsystem: websocket
tags: [websocket, coder-websocket, hub, pubsub, fan-out, slow-consumer-drop, ping-keepalive, tdd]

# Dependency graph
requires:
  - phase: 03-websocket-hub
    provides: "Hub, Options, subscriber struct, Subscribe writer loop with CloseRead + defer removeSubscriber + defer CloseNow; stub Publish and Close marked TODO(03-02); tick.C case body stub"
provides:
  - "Hub.Publish: non-blocking select-default fan-out with drop-slow-consumer via `go s.closeSlow()` under h.mu"
  - "Hub.Publish Warn log on slow-drop: 'wshub: subscriber dropped (slow consumer)' with buffer_size field"
  - "Hub.Publish silent no-op after Close (h.closed short-circuit)"
  - "Hub.Close: idempotent (closed-flag guard), Info log 'wshub: closing hub' with subscribers count, per-subscriber `go s.closeGoingAway()` off-mutex"
  - "Subscribe writer loop tick.C case: real conn.Ping(pingCtx) with PingTimeout-bounded ctx, ctx-cancel exits cleanly, other errors log Debug"
  - "Four new hub_test.go tests: Publish_FanOut, SlowConsumerDropped, Close_GoingAway, Publish_AfterCloseIsNoOp (internal tests touching h.mu + h.subscribers)"
  - "TestSubscribe_PingKeepsAlive appended to subscribe_test.go using require.Never over 30+ ping cycles"
  - "All three Plan 03-01 TODO(03-02) stubs replaced; grep returns empty"
affects: [03-03-tests, 04-http-api, 05-wiring]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Publish fan-out: `for s := range h.subscribers { select { case s.msgs <- msg: default: Warn + go s.closeSlow() } }` under h.mu — the `go` keyword on closeSlow is load-bearing (5s+5s close handshake budget)"
    - "Close idempotence: `h.closed` flag under h.mu, second call is no-op, Close does NOT clear h.subscribers (writer loops self-cleanup via defer h.removeSubscriber)"
    - "Ping case: `context.WithTimeout(ctx, h.opts.PingTimeout)` + `conn.Ping(pingCtx)` — ctx.Canceled/DeadlineExceeded returns nil, other errors log Debug and return err"
    - "require.Never as ping-keepalive oracle: assert len(h.subscribers)==0 NEVER becomes true across 300ms (30+ ping cycles at 10ms PingInterval)"

key-files:
  created:
    - "internal/wshub/hub_test.go (4 tests: Publish_FanOut, SlowConsumerDropped, Close_GoingAway, Publish_AfterCloseIsNoOp + subscribeHandler + waitForSubscribers helpers)"
  modified:
    - "internal/wshub/hub.go (real Publish fan-out body + real Close body replacing Plan 03-01 TODO(03-02) stubs)"
    - "internal/wshub/subscribe.go (tick.C case body replaced with real conn.Ping + PingTimeout context)"
    - "internal/wshub/subscribe_test.go (TestSubscribe_PingKeepsAlive appended)"

key-decisions:
  - "closeSlow/closeGoingAway each take the full 5s close-handshake budget when the peer never reads (the slow-consumer simulation). Tests for slow-drop and Close-GoingAway had to use 7-second Eventually timeouts, not the 1-second budget the research example suggested. This is a structural library property (coder/websocket v1.8.14 hardcodes 5s waitCloseHandshake), not a bug in the hub"
  - "The `go` keyword on `go s.closeSlow()` and `go s.closeGoingAway()` is load-bearing: calling conn.Close inline would hold h.mu for up to 10s per subscriber. Preserved verbatim from 03-CONTEXT.md and documented in hub.go doc comments"
  - "Close does NOT clear h.subscribers — the writer loops self-cleanup via their `defer h.removeSubscriber(s)`. Clearing the map would race with those defers (PITFALLS #3)"
  - "Publish-after-Close is a silent no-op — no log, no fan-out — so Phase 5's two-phase shutdown can race with in-flight HTTP handlers without spurious Warn spam"
  - "Ping case uses the same ctx-cancel / Debug-log / return err shape as the msgs case for consistency; both exit via `defer h.removeSubscriber(s)`"
  - "TestSubscribe_PingKeepsAlive uses require.Never (not require.Eventually) to assert `len(h.subscribers)==0` NEVER becomes true over 300ms. This is positive-by-negative: if pings silently break, the subscriber's writer loop would error out and the count would drop. 30+ ping cycles at 10ms is deterministic without time.Sleep"

patterns-established:
  - "Pattern: internal test file (`package wshub`) justified by deliberate minimal public API — no Stats() or NumSubscribers() exported; tests touch h.mu + h.subscribers directly for state observation"
  - "Pattern: realistic test timeouts account for library close-handshake budget (7s for slow-consumer/Close-GoingAway tests, not the 1s from research examples)"
  - "Pattern: two-callback subscriber pays off — closeSlow and closeGoingAway captured at Subscribe time, each pre-bound to the *websocket.Conn, so Publish and Close have zero conn-awareness and zero branching"

requirements-completed: [WS-03, WS-07]

# Metrics
duration: 8min
completed: 2026-04-10
---

# Phase 3 Plan 02: Publish + Close + Ping Summary

**Publish non-blocking fan-out with Warn-on-slow-drop, idempotent Close with Info-on-shutdown, and real conn.Ping keepalive — the second critical research flag (PITFALLS #4 slow-client stall) lands with all three Plan 03-01 TODO(03-02) stubs replaced and 8/8 wshub tests green under -race.**

## Performance

- **Duration:** ~8 min
- **Started:** 2026-04-10T11:19:02Z
- **Completed:** 2026-04-10T11:27:18Z
- **Tasks:** 2 (Task 1 RED, Task 2 GREEN)
- **Files modified:** 4 (1 created, 3 modified)
- **Commits:** 2 (test + feat)

## Accomplishments

- `Hub.Publish` real body: non-blocking `select { case s.msgs <- msg: default: Warn + go s.closeSlow() }` under h.mu with `h.closed` short-circuit guard
- Warn log on slow-drop: exact frozen string `"wshub: subscriber dropped (slow consumer)"` with `buffer_size` field only (no PII, no peer identity)
- `Hub.Close` real body: idempotent closed-flag guard, Info log `"wshub: closing hub"` with `subscribers` count once, iteration firing `go s.closeGoingAway()` per subscriber off-mutex
- `Subscribe` writer loop `tick.C` case real body: `context.WithTimeout(ctx, h.opts.PingTimeout)` + `conn.Ping(pingCtx)` with ctx.Canceled/DeadlineExceeded → return nil, other errors → Debug log + return err
- `internal/wshub/hub_test.go` lands with 4 tests — all GREEN under `-race`:
  - `TestHub_Publish_FanOut` — two subscribers receive one published message via conn.Read (sub-second)
  - `TestHub_SlowConsumerDropped` — MessageBuffer=1 + non-reading client → subscriber dropped (~5s for close handshake)
  - `TestHub_Close_GoingAway` — `hub.Close()` sets closed=true, all subscribers kicked (~5s for handshake), second call is no-op (NotPanics)
  - `TestHub_Publish_AfterCloseIsNoOp` — bytes.Buffer logger asserts Info "closing hub" is present AND Warn "subscriber dropped" is absent
- `TestSubscribe_PingKeepsAlive` appended to `subscribe_test.go` — `require.Never(len(h.subscribers)==0, 300ms)` across 30+ ping cycles at 10ms PingInterval
- All three Plan 03-01 `TODO(03-02)` markers replaced; `grep -n 'TODO(03-02)' internal/wshub/*.go` returns empty
- All 8 wshub tests green under `-race`: 3 from Plan 03-01 + 5 from Plan 03-02 (total runtime ~12s, dominated by two ~5s close-handshake waits)
- All architectural gates still pass: (1) no imports of `registry` or `httpapi` from `wshub`, (2) no `slog.Default` in production code, (3) no `time.Sleep` in tests, (4) `go vet ./...` and `gofmt -l internal/wshub/` both clean, (5) full `go test ./... -race` all packages green

## Task Commits

1. **Task 1: RED — hub_test.go + ping keepalive test (4+1 tests, all failing against stubs)** — `32d0ffd` (test)
2. **Task 2: GREEN — real Publish + Close + ping bodies** — `9a27fa8` (feat)

## Files Created/Modified

- `internal/wshub/hub_test.go` — **created** — 4 internal tests (package wshub) + `subscribeHandler` helper + `waitForSubscribers` polling helper that touches `h.mu` + `h.subscribers` directly
- `internal/wshub/hub.go` — **modified** — replaced Plan 03-01 `Publish` stub (`_ = msg`) with real non-blocking fan-out + Warn log + `go s.closeSlow()`; replaced Plan 03-01 `Close` stub (empty) with idempotent closed-flag guard + Info log + `go s.closeGoingAway()` iteration
- `internal/wshub/subscribe.go` — **modified** — replaced Plan 03-01 `case <-tick.C:` TODO stub with real `conn.Ping(pingCtx)` call bounded by `h.opts.PingTimeout` with ctx-cancel handling mirroring the `case msg := <-s.msgs:` branch
- `internal/wshub/subscribe_test.go` — **modified** — appended `TestSubscribe_PingKeepsAlive` using `require.Never` over 300ms (30+ ping cycles at 10ms PingInterval)

## Decisions Made

- **Test timeouts raised from 1s to 7s** for slow-consumer and Close-GoingAway tests. The research example's 1-second budget did not account for coder/websocket v1.8.14's hardcoded 5s `waitCloseHandshake` timeout that fires whenever the peer never reads (which is exactly the slow-consumer simulation). The writer loop's `conn.Ping` blocks on `<-c.closed` until `conn.Close` completes its full handshake-wait → c.close() → c.closed chain, which takes ~5 seconds. 7-second Eventually windows give comfortable slack above the 5-second floor.
- **Publish and Close both hold h.mu for the duration of the fan-out/kick loop**, but both loops are fast: `select`-default on a channel is nanoseconds, `go s.closeXxx()` is just a goroutine dispatch. The 5s close handshake happens in the spawned goroutine, never under h.mu. This is the whole point of the `go` keyword — documented verbatim in both hub.go method doc comments.
- **Close does NOT clear h.subscribers.** Each writer loop observes its conn close (via `<-c.closed` in Ping or net.ErrClosed in Write) and exits through `defer h.removeSubscriber(s)`. Clearing the map under h.mu in Close would race with those defers trying to acquire h.mu.
- **TestSubscribe_PingKeepsAlive uses `require.Never`, not `require.Eventually`.** The condition `len(h.subscribers)==0` should NEVER become true across 300ms of idle — that's 30+ ping cycles at 10ms PingInterval. If pings silently break (e.g., missing concurrent reader from PITFALLS #9), the writer loop would error out and the count would drop. require.Never is the idiomatic testify way to assert "this invariant holds continuously" without `time.Sleep`.
- **TestHub_Publish_AfterCloseIsNoOp uses a bytes.Buffer slog handler** so the test can assert both presence (Info "closing hub") and absence (Warn "subscriber dropped") of specific log lines in the captured output. This is the first log-capture test in wshub; the full log-format test suite lands in Plan 03-03.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Plan acceptance-criteria timeouts of 1 second for slow-consumer and Close-GoingAway tests were unrealistic**

- **Found during:** Task 2 GREEN verification
- **Issue:** The plan specified `waitForSubscribers(t, hub, 0, time.Second)` for both `TestHub_SlowConsumerDropped` and `TestHub_Close_GoingAway`. In practice, `conn.Close(StatusPolicyViolation, ...)` and `conn.Close(StatusGoingAway, ...)` both trigger `closeHandshake` → `waitCloseHandshake` which has a hardcoded 5-second timeout inside coder/websocket v1.8.14 (`close.go:199`). When the test client never reads (which is the entire point of the slow-consumer simulation), that 5-second timeout must elapse fully before `c.close()` fires, which closes `c.closed`, which unblocks the writer loop's `conn.Ping` call. The Ping was what kept the writer loop alive across those 5 seconds, because PingInterval was 10ms and the writer loop kept looping back to the `tick.C` case between every failed Ping attempt. Actual observed drain time: ~5 seconds on a cold machine.
- **Fix:** Both test timeouts raised from `time.Second` to `7*time.Second` (comfortable slack above the 5s floor). Same change applied to the `context.WithTimeout` wrapping the websocket.Dial from 3s to 15s in both tests, so the Dial context doesn't expire before the slow-drop observation.
- **Files modified:** `internal/wshub/hub_test.go` (two tests, four line changes total)
- **Verification:** Both tests now pass in ~5.01s each under `-race`. Full wshub suite runs in ~12 seconds total. A debug run with an in-test per-100ms log confirmed the drop happens at ~5.1s (the 5s handshake wait + a few ms of writer-loop observation latency).
- **Committed in:** 9a27fa8 (rolled into the Task 2 GREEN commit)
- **Rationale for Rule 1:** This is an author-side plan bug — the research example `03-RESEARCH.md` §Example 5 used `time.Second` for the same assertion, but never ran the test against the real coder/websocket v1.8.14 library to verify. The behavior is a structural property of the library (5s+5s handshake budget is documented in `close.go:86-128` and referenced in 03-CONTEXT.md §"Close semantics" itself). The production code is byte-for-byte correct per the plan; only the test's timing assertion needed to be realistic.

---

**Total deviations:** 1 auto-fixed (Rule 1 bug in plan's acceptance criteria timing)
**Impact on plan:** Zero impact on production code or semantics. The correctness contracts — non-blocking fan-out, drop-slow-consumer, StatusPolicyViolation/StatusGoingAway close codes, idempotent Close, Warn/Info log formats — are all preserved byte-for-byte as specified. The only change is a realistic test timeout that matches how the coder/websocket library actually behaves when the peer never reads.

## Issues Encountered

None beyond the 5-second close-handshake investigation documented under Deviations above. The RED → GREEN flow was otherwise mechanical: all five tests started red (4 Hub-level against Publish/Close stubs, 1 Subscribe-level against the tick.C stub); after replacing the three stub bodies with their real contents from the plan's verbatim Go blocks, all five tests went green on the first run. No flakiness observed across repeated runs under `-race`.

## User Setup Required

None — the phase is headless, byte-oriented, and has no external service config. The slow-consumer simulation is entirely in-process (a test client that never reads).

## Next Phase Readiness

- **Plan 03-03 (goroutine-leak stress + log-format verification):** Ready. All hub and subscribe production code is now at its final shape for Phase 3; Plan 03-03 only adds more tests (the 1000-cycle leak test already lives in 03-01, but 03-03 adds the Warn/Info format capture tests with PII-absence assertions, plus potentially a concurrent slow-consumer stress test). No production code changes required for Plan 03-03.
- **Phase 4 (HTTP API):** Ready. `wshub.Hub.Publish(msg []byte)`, `wshub.Hub.Subscribe(ctx, conn)`, and `wshub.Hub.Close()` are all production-ready. Phase 4's HTTP handler can call `websocket.Accept(w, r, opts)` then `hub.Subscribe(r.Context(), conn)` with full confidence; the mutation-then-broadcast flow (`WS-05`, `WS-09`) can call `hub.Publish(eventBytes)` after successful `store.Upsert`/`store.Delete`; Phase 5's two-phase shutdown can call `hub.Close()` after `httpSrv.Shutdown(ctx)` without race concerns.
- **Phase 3's two load-bearing correctness flags are both now proven:** PITFALLS #3 (goroutine leak on silent disconnect — enforced by TestSubscribe_NoGoroutineLeak's 1000-cycle run from Plan 03-01) AND PITFALLS #4 (slow-client stall — enforced by TestHub_SlowConsumerDropped from this plan).
- **Gates:** all Phase 3 architectural gates still pass: (1) `go list -deps ./internal/wshub | grep -E 'registry|httpapi'` empty, (2) `grep -rE 'slog\.Default' internal/wshub/*.go | grep -v _test.go` empty, (3) `grep -n 'time\.Sleep' internal/wshub/*_test.go` empty, (4) `grep -n 'TODO(03-02)' internal/wshub/*.go` empty, (5) `go vet ./...` + `gofmt -l internal/wshub/` + `go build ./...` all clean.

## Self-Check

Verifying claims made in this summary:

- `internal/wshub/hub_test.go` — FOUND (created this plan)
- `internal/wshub/hub.go` — FOUND (Publish + Close bodies replaced)
- `internal/wshub/subscribe.go` — FOUND (tick.C case body replaced)
- `internal/wshub/subscribe_test.go` — FOUND (TestSubscribe_PingKeepsAlive appended)
- Commit 32d0ffd — FOUND (Task 1 RED)
- Commit 9a27fa8 — FOUND (Task 2 GREEN)

## Self-Check: PASSED

---
*Phase: 03-websocket-hub*
*Completed: 2026-04-10*
