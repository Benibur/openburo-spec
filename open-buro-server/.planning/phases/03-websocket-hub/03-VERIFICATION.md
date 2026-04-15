---
phase: 03-websocket-hub
verified: 2026-04-10T12:00:00Z
status: passed
score: 5/5 must-haves verified
requirements_verified:
  - WS-02
  - WS-03
  - WS-04
  - WS-07
  - WS-10
---

# Phase 3: WebSocket Hub Verification Report

**Phase Goal:** A leak-free, byte-oriented pub/sub hub (`internal/wshub`) implementing the `coder/websocket` canonical chat pattern with non-blocking fan-out, drop-slow-consumer semantics, and periodic ping keepalive — independently testable with `httptest.NewServer` and a local client.

**Verified:** 2026-04-10
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths (Success Criteria from ROADMAP.md)

| #   | Truth (Success Criterion)                                                                                                                                                                          | Status     | Evidence                                                                                                                                                                                                                                                                                                                                                             |
| --- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | `hub.Publish([]byte)` delivers to every active subscriber without blocking; a subscriber whose buffered channel is full is dropped via `closeSlow` instead of stalling the publisher                | ✓ VERIFIED | `hub.go:112-129` Publish holds h.mu, iterates subscribers, `select { case s.msgs <- msg: default: Warn + go s.closeSlow() }`. `TestHub_Publish_FanOut` (PASS 0.00s) proves two subscribers both receive a single Publish. `TestHub_SlowConsumerDropped` (PASS 5.01s) proves MessageBuffer=1 non-reader is dropped. `go s.closeSlow()` spawned off-mutex (load-bearing). |
| 2   | 1000 connect-then-disconnect cycles against `httptest.NewServer`-backed hub end with `runtime.NumGoroutine()` flat (±epsilon)                                                                       | ✓ VERIFIED | `subscribe_test.go:61-91` TestSubscribe_NoGoroutineLeak runs 1000 dial/close cycles, polls `runtime.NumGoroutine() <= baseline+5` via `require.Eventually` (no time.Sleep). PASS 0.99s under -race. Gate 2 isolated run: PASS 0.60s.                                                                                                                               |
| 3   | Slow subscriber kicked without blocking other subscribers or the publisher                                                                                                                          | ✓ VERIFIED | `hub.go:112-129` Publish uses non-blocking select-default, spawns `go s.closeSlow()` (the `go` keyword is load-bearing — avoids holding h.mu for 5s+5s close handshake). `TestHub_SlowConsumerDropped` (PASS 5.01s) proves drop. `TestHub_Logging_DropIsWarn` (PASS 0.00s) proves the Warn log fires with frozen message and `buffer_size=1` field.                  |
| 4   | Active subscribers receive periodic ping frames at configured interval (default 30s); connection stays open across idle periods                                                                      | ✓ VERIFIED | `subscribe.go:86-96` `case <-tick.C` calls `conn.Ping(pingCtx)` bounded by `h.opts.PingTimeout`; ctx-cancel errors return nil, others log Debug. Default `PingInterval=30*time.Second` at `hub.go:14`. `TestSubscribe_PingKeepsAlive` (PASS 0.30s) uses `require.Never(len(h.subscribers)==0, 300ms)` across 30+ ping cycles at 10ms interval.                       |
| 5   | `go list -deps ./internal/wshub` shows zero imports of `internal/registry` — architectural independence by construction                                                                              | ✓ VERIFIED | `~/sdk/go1.26.2/bin/go list -deps ./internal/wshub \| grep -E 'registry\|httpapi'` returns empty (exit=1). wshub imports only stdlib + `github.com/coder/websocket` + `github.com/stretchr/testify` (test-only).                                                                                                                                                   |

**Score:** 5/5 truths verified

### Required Artifacts (Level 1/2/3)

| Artifact                                  | Expected                                                                                                                      | Exists | Substantive     | Wired | Status     |
| ----------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- | ------ | --------------- | ----- | ---------- |
| `internal/wshub/doc.go`                   | Package doc, canonical chat-hub pattern description, byte-oriented contract                                                   | ✓      | ✓ 16 lines      | ✓     | ✓ VERIFIED |
| `internal/wshub/hub.go`                   | Hub struct, Options with defaults, New (nil-logger panic), add/removeSubscriber, Publish fan-out, Close idempotent             | ✓      | ✓ 150 lines     | ✓     | ✓ VERIFIED |
| `internal/wshub/subscribe.go`             | subscriber struct (two pre-bound close callbacks), Subscribe with CloseRead + defer removeSubscriber + defer CloseNow + ping    | ✓      | ✓ 101 lines     | ✓     | ✓ VERIFIED |
| `internal/wshub/hub_test.go`              | Fan-out, slow-consumer drop, Close_GoingAway, publish-after-close, three logging contract tests, syncBuffer helper             | ✓      | ✓ 296 lines     | ✓     | ✓ VERIFIED |
| `internal/wshub/subscribe_test.go`        | New-panics-on-nil, default options, 1000-cycle goroutine leak, ping-keeps-alive                                                | ✓      | ✓ 136 lines     | ✓     | ✓ VERIFIED |
| `github.com/coder/websocket v1.8.14` dep  | Direct require in go.mod                                                                                                      | ✓      | ✓ direct dep    | ✓     | ✓ VERIFIED |

### Key Link Verification

| From            | To                     | Via                                                                                           | Status      | Details                                                                                                                                                     |
| --------------- | ---------------------- | --------------------------------------------------------------------------------------------- | ----------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Hub.Publish`   | `subscriber.msgs`      | non-blocking `select { case s.msgs <- msg: default: ... }` under h.mu                          | WIRED       | `hub.go:118-127` implements exact drop-slow-consumer contract                                                                                                |
| `Hub.Publish`   | `subscriber.closeSlow` | `go s.closeSlow()` on slow drop path                                                           | WIRED       | `hub.go:126` — `go` keyword load-bearing; avoids h.mu held during 5s+5s close handshake                                                                     |
| `Hub.Close`     | `subscriber.closeGoingAway` | iteration `for s := range h.subscribers { go s.closeGoingAway() }` under h.mu            | WIRED       | `hub.go:147-149`; idempotent via `h.closed` guard `hub.go:142-145`                                                                                          |
| `Subscribe`     | `conn.CloseRead(ctx)`  | Top-of-method assignment `ctx = conn.CloseRead(ctx)`                                           | WIRED       | `subscribe.go:50`; PITFALLS #3 research flag #1; ctx is load-bearing for `<-ctx.Done()` branch                                                              |
| `Subscribe`     | `h.removeSubscriber`   | `defer h.removeSubscriber(s)` after addSubscriber                                              | WIRED       | `subscribe.go:62`; PITFALLS #3 research flag #2; idempotent cleanup on any return path                                                                      |
| `Subscribe`     | `conn.CloseNow`        | `defer conn.CloseNow()` safety-net after defer removeSubscriber                                | WIRED       | `subscribe.go:63`; catches any unexpected early return from writer loop                                                                                    |
| `Subscribe`     | `conn.Ping`            | `case <-tick.C` → `conn.Ping(pingCtx)` bounded by `h.opts.PingTimeout`                         | WIRED       | `subscribe.go:86-96`; ctx-cancel → return nil; other errors → Debug log + return err                                                                        |
| `Subscribe`     | `conn.Write`           | `case msg := <-s.msgs` → `conn.Write(writeCtx, MessageText, msg)` bounded by `WriteTimeout`   | WIRED       | `subscribe.go:75-85`; same error shape as Ping branch                                                                                                       |
| `New`           | zero-value default replacement | Replaces `opts.MessageBuffer/PingInterval/WriteTimeout/PingTimeout == 0` with const defaults | WIRED       | `hub.go:65-76`; proven by `TestHub_DefaultOptions` (16 / 30s / 5s / 10s)                                                                                    |

### Requirements Coverage

| Requirement | Source Plan | Description                                                                                                                      | Status       | Evidence                                                                                                                                                                                                                  |
| ----------- | ----------- | -------------------------------------------------------------------------------------------------------------------------------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **WS-02**   | 03-01       | Centralized hub pattern: Hub holds subscribers map under a mutex, subscriber has a buffered outbound channel (default 16)         | ✓ SATISFIED  | `hub.go:46-53` Hub struct with `mu sync.Mutex` + `subscribers map[*subscriber]struct{}`; `subscribe.go:18-22` subscriber.msgs `chan []byte`; default 16 at `hub.go:13`. Verified by `TestHub_DefaultOptions`.            |
| **WS-03**   | 03-02       | Non-blocking fan-out: publishing to a slow subscriber whose buffer is full triggers `closeSlow` (drop client) rather than blocking | ✓ SATISFIED  | `hub.go:118-127` non-blocking select-default fan-out + `go s.closeSlow()`. Verified by `TestHub_Publish_FanOut`, `TestHub_SlowConsumerDropped`, `TestHub_Logging_DropIsWarn`.                                             |
| **WS-04**   | 03-01       | Each subscriber calls `conn.CloseRead(ctx)` so control frames are handled and closed clients are detected (prevents goroutine leaks) | ✓ SATISFIED  | `subscribe.go:50` `ctx = conn.CloseRead(ctx)` at top of Subscribe; paired with `defer h.removeSubscriber(s)` at line 62. Verified by `TestSubscribe_NoGoroutineLeak` (1000 cycles).                                       |
| **WS-07**   | 03-02       | Periodic ping frames keep connections alive (default 30s, configurable from config.yaml)                                           | ✓ SATISFIED  | `subscribe.go:86-96` `case <-tick.C` → `conn.Ping(pingCtx)`; default `PingInterval = 30 * time.Second` at `hub.go:14`; configurable via `Options.PingInterval`. Verified by `TestSubscribe_PingKeepsAlive`.              |
| **WS-10**   | 03-03 (+03-01) | Goroutine leak integration test: 1000 connect/disconnect cycles leave `runtime.NumGoroutine()` flat (±epsilon)                    | ✓ SATISFIED  | `subscribe_test.go:61-91` `TestSubscribe_NoGoroutineLeak` runs 1000 cycles against `httptest.NewServer` with `require.Eventually` polling `runtime.NumGoroutine() <= baseline+5`. PASS 0.99s -race.                      |

**Orphaned requirements check:** REQUIREMENTS.md line 233 maps Phase 3 → `WS-02, WS-03, WS-04, WS-07, WS-10` (5 IDs). All 5 IDs appear in the union of plan `requirements-completed` fields (03-01: WS-02, WS-04; 03-02: WS-03, WS-07; 03-03: WS-10). **No orphans.**

### Anti-Patterns Found

**Scan scope:** `internal/wshub/hub.go`, `internal/wshub/subscribe.go`, `internal/wshub/doc.go`, `internal/wshub/hub_test.go`, `internal/wshub/subscribe_test.go`.

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| _(none)_ | — | — | — | — |

- No `TODO(03-02)` markers remain (gate 6 empty)
- No `time.Sleep` in tests (gate 5 empty — including no comment references)
- No `slog.Default` in production (gate 4 empty)
- No stub `return null`/empty-body handlers — every function is substantive
- No `PLACEHOLDER`/`FIXME`/`XXX`/`HACK` comments (grepped)
- `Publish` early-return on `h.closed` is a documented silent-no-op for two-phase shutdown, not a stub

### Verification Commands (all PASS)

| # | Command                                                                                                                     | Expected          | Actual            | Status |
| - | --------------------------------------------------------------------------------------------------------------------------- | ----------------- | ----------------- | ------ |
| 1 | `~/sdk/go1.26.2/bin/go test ./internal/wshub -race -count=1 -v`                                                              | 11 tests PASS     | 11/11 PASS ~17.4s | PASS   |
| 2 | `~/sdk/go1.26.2/bin/go test ./... -race -count=1`                                                                           | all packages PASS | config/httpapi/registry/wshub all PASS | PASS |
| 3 | `~/sdk/go1.26.2/bin/go build ./...`                                                                                         | EXIT=0            | EXIT=0            | PASS   |
| 4 | `~/sdk/go1.26.2/bin/go vet ./internal/wshub`                                                                                | EXIT=0            | EXIT=0            | PASS   |
| 5 | `gofmt -l internal/wshub/`                                                                                                  | empty             | empty             | PASS   |
| 6 | `~/sdk/go1.26.2/bin/go list -deps ./internal/wshub \| grep -E 'registry\|httpapi'`                                         | empty             | empty (exit=1)    | PASS   |
| 7 | `grep -rE 'slog\.Default' internal/wshub/*.go \| grep -v _test.go`                                                          | empty             | empty             | PASS   |
| 8 | `grep -n 'time\.Sleep' internal/wshub/*_test.go`                                                                            | empty             | empty             | PASS   |
| 9 | `grep -n 'TODO(03-02)' internal/wshub/*.go`                                                                                  | empty             | empty             | PASS   |

**Test output excerpt (verbatim):**

```
=== RUN   TestHub_Publish_FanOut
--- PASS: TestHub_Publish_FanOut (0.00s)
=== RUN   TestHub_SlowConsumerDropped
--- PASS: TestHub_SlowConsumerDropped (5.01s)
=== RUN   TestHub_Close_GoingAway
--- PASS: TestHub_Close_GoingAway (5.01s)
=== RUN   TestHub_Publish_AfterCloseIsNoOp
--- PASS: TestHub_Publish_AfterCloseIsNoOp (0.00s)
=== RUN   TestHub_Logging_DropIsWarn
--- PASS: TestHub_Logging_DropIsWarn (0.00s)
=== RUN   TestHub_Logging_CloseIsInfo
--- PASS: TestHub_Logging_CloseIsInfo (0.00s)
=== RUN   TestHub_Logging_NoPII
--- PASS: TestHub_Logging_NoPII (5.01s)
=== RUN   TestHub_New_PanicsOnNilLogger
--- PASS: TestHub_New_PanicsOnNilLogger (0.00s)
=== RUN   TestHub_DefaultOptions
--- PASS: TestHub_DefaultOptions (0.00s)
=== RUN   TestSubscribe_NoGoroutineLeak
--- PASS: TestSubscribe_NoGoroutineLeak (0.99s)
=== RUN   TestSubscribe_PingKeepsAlive
--- PASS: TestSubscribe_PingKeepsAlive (0.30s)
PASS
ok  	github.com/openburo/openburo-server/internal/wshub	17.356s
```

### Commit Chain Verified

All commits cited in the three SUMMARY.md files exist in git history:

- `738d58a` chore(03-01): add github.com/coder/websocket v1.8.14 dependency
- `85407cb` test(03-01): 1000-cycle goroutine-leak + defaults + nil-logger tests (RED)
- `c3ecaf9` feat(03-01): Hub + Options + Subscribe writer loop (GREEN)
- `32d0ffd` test(03-02): slow-consumer + fan-out + Close-GoingAway + ping-keepalive (RED)
- `9a27fa8` feat(03-02): Publish fan-out + Close(StatusGoingAway) + ping keepalive (GREEN)
- `2d7c2a5` test(03-03): logging contract (Warn-on-drop, Info-on-close, no-PII)
- `fe4bafb` docs(03-03): phase 3 gate sweep results

### Human Verification Required

None. All success criteria are programmatically verifiable and verified by automated tests + shell gates. The byte-oriented hub has no UI, no visual behavior, no external service integration, and no configuration surface beyond the Options struct (covered by `TestHub_DefaultOptions`).

### Gaps Summary

**No gaps.** Phase 3 is feature-complete:

- All 5 ROADMAP success criteria are observably verified by named passing tests
- All 5 requirement IDs (WS-02, WS-03, WS-04, WS-07, WS-10) are implemented, wired, and tested
- All 9 verification commands pass
- All 8 architectural gates from `03-GATES.md` still pass (re-confirmed in this verification run)
- The two load-bearing research flags (PITFALLS #3 goroutine leak, PITFALLS #4 slow-client stall) are proven by dedicated tests: `TestSubscribe_NoGoroutineLeak` (1000 cycles) and `TestHub_SlowConsumerDropped`
- The no-PII log contract is frozen by `TestHub_Logging_NoPII` + `requireNoPII` helper + 11-entry `piiSubstrings` list
- The Warn-on-drop / Info-on-close log format contract is frozen by `TestHub_Logging_DropIsWarn` and `TestHub_Logging_CloseIsInfo`
- Architectural isolation is structurally enforced: `go list -deps` shows zero imports of `internal/registry` or `internal/httpapi`
- The package is ready for Phase 4 (HTTP API) to call `hub.Subscribe(ctx, conn)` and `hub.Publish([]byte)` with full confidence

Phase 3 is ready to be marked complete in ROADMAP.md. Phase 4 (HTTP API) has no blockers from this side.

---

_Verified: 2026-04-10_
_Verifier: Claude (gsd-verifier)_
