---
phase: 03-websocket-hub
plan: 02
type: execute
wave: 2
depends_on:
  - 03-01
files_modified:
  - internal/wshub/hub.go
  - internal/wshub/subscribe.go
  - internal/wshub/hub_test.go
  - internal/wshub/subscribe_test.go
autonomous: true
gap_closure: false
requirements:
  - WS-03
  - WS-07
requirements_addressed:
  - WS-03
  - WS-07

must_haves:
  truths:
    - "Publish fans out to every active subscriber via non-blocking `select { case s.msgs <- msg: default: ... }` — a slow subscriber whose buffer is full never blocks the publisher"
    - "When Publish observes a full buffer it logs at Warn with the exact message 'wshub: subscriber dropped (slow consumer)' and buffer_size field, then fires `go s.closeSlow()` (the `go` keyword is load-bearing — closeSlow calls conn.Close which has a 5s+5s handshake budget)"
    - "Hub.Close is idempotent: second call is a no-op; first call sets h.closed=true, logs Info with 'wshub: closing hub' and subscribers count, then fires `go s.closeGoingAway()` for every active subscriber"
    - "After Hub.Close, subsequent Publish calls are silent no-ops (closed flag short-circuits)"
    - "Subscribe's writer loop pings the conn every opts.PingInterval via conn.Ping(pingCtx) bounded by opts.PingTimeout; a ping error exits the writer loop cleanly"
    - "TestHub_SlowConsumerDropped: a subscriber that never reads from its channel is removed from hub.subscribers (observed via internal-test access to hub.mu + hub.subscribers) within 1 second"
    - "TestSubscribe_PingKeepsAlive: a subscriber whose conn is idle for >10×PingInterval stays connected (ping keepalive works)"
    - "TestHub_Close_GoingAway: after Hub.Close, hub.closed is true, a second call is a no-op, and a connected subscriber's writer loop exits within 1 second"
  artifacts:
    - path: "internal/wshub/hub.go"
      provides: "Real Publish body with select-default fan-out + Warn log; real Close body with idempotent closed flag + Info log"
      contains: "func (h *Hub) Publish"
    - path: "internal/wshub/subscribe.go"
      provides: "Writer loop tick.C case fully implemented with conn.Ping + PingTimeout context"
      contains: "conn.Ping(pingCtx)"
    - path: "internal/wshub/hub_test.go"
      provides: "TestHub_SlowConsumerDropped, TestHub_Publish_FanOut, TestHub_Close_GoingAway, TestHub_Publish_AfterCloseIsNoOp"
      contains: "TestHub_SlowConsumerDropped"
    - path: "internal/wshub/subscribe_test.go"
      provides: "TestSubscribe_PingKeepsAlive appended to the existing file"
      contains: "TestSubscribe_PingKeepsAlive"
  key_links:
    - from: "internal/wshub/hub.go Publish"
      to: "subscriber.closeSlow"
      via: "go s.closeSlow() on select default branch"
      pattern: "go s\\.closeSlow\\(\\)"
    - from: "internal/wshub/hub.go Close"
      to: "subscriber.closeGoingAway"
      via: "go s.closeGoingAway() iteration under h.mu"
      pattern: "go s\\.closeGoingAway\\(\\)"
    - from: "internal/wshub/subscribe.go Subscribe tick.C case"
      to: "github.com/coder/websocket Conn.Ping"
      via: "context.WithTimeout(ctx, h.opts.PingTimeout) + conn.Ping(pingCtx)"
      pattern: "conn\\.Ping\\(pingCtx\\)"
---

<objective>
Complete Phase 3's fan-out and shutdown mechanics: fill the `Publish` and `Close` stubs from Plan 03-01 with their real bodies, and wire the ping-keepalive case in the Subscribe writer loop. This plan lands the second critical research flag — non-blocking `select`-default fan-out with drop-slow-consumer semantics (PITFALLS #4) — and the ping loop that keeps idle WebSocket connections alive (WS-07).

Plan 03-01 intentionally left three things as stubs: `Hub.Publish` (no-op), `Hub.Close` (no-op), and the `tick.C` branch in Subscribe's writer loop (no-op with a `TODO(03-02)` comment). This plan replaces all three with their real bodies and adds the four tests that prove the behavior. The Subscribe method's overall shape does NOT change — we only fill the tick.C case body. This keeps Plan 03-01's goroutine-leak test (which runs against the whole writer loop) green throughout.

Purpose: Solve the second of Phase 3's two load-bearing correctness risks (slow-client stall, PITFALLS #4) and ship the WS-07 ping keepalive. After this plan, `hub.Publish(msg)` and `hub.Close()` are fully functional — Phase 4 can wire them without any further hub-side work, and Phase 5 can call `hub.Close()` in the two-phase shutdown with confidence.

Output: real bodies for `Publish` + `Close` in `hub.go`, real body for the `tick.C` case in `subscribe.go`, and `hub_test.go` (new) + one new test appended to `subscribe_test.go`.
</objective>

<execution_context>
@/home/ben/.claude/get-shit-done/workflows/execute-plan.md
@/home/ben/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/REQUIREMENTS.md
@.planning/phases/03-websocket-hub/03-CONTEXT.md
@.planning/phases/03-websocket-hub/03-RESEARCH.md
@.planning/phases/03-websocket-hub/03-VALIDATION.md
@.planning/phases/03-websocket-hub/03-01-SUMMARY.md
@.planning/research/PITFALLS.md
@internal/wshub/hub.go
@internal/wshub/subscribe.go
@internal/wshub/subscribe_test.go
@internal/registry/store.go
@internal/registry/store_test.go

<interfaces>
<!-- These types already exist from Plan 03-01. Quoted here for executor convenience. -->

```go
// From hub.go (Plan 03-01):
type Hub struct {
    logger      *slog.Logger
    opts        Options

    mu          sync.Mutex
    subscribers map[*subscriber]struct{}
    closed      bool
}

// From subscribe.go (Plan 03-01):
type subscriber struct {
    msgs           chan []byte
    closeSlow      func() // Publish path: StatusPolicyViolation
    closeGoingAway func() // Hub.Close path: StatusGoingAway
}

// Plan 03-01 left these as TODO(03-02) stubs — this plan replaces them:
func (h *Hub) Publish(msg []byte) { _ = msg }
func (h *Hub) Close() {}
// The tick.C branch in (h *Hub).Subscribe is a no-op with a TODO(03-02) comment.
```
</interfaces>

<locked_decisions>
<!-- All from 03-CONTEXT.md. DO NOT re-open. -->
1. **Publish fan-out shape**: `for s := range h.subscribers { select { case s.msgs <- msg: default: <log Warn>; go s.closeSlow() } }` under `h.mu.Lock()`. The `go` keyword on `closeSlow` is LOAD-BEARING — calling `conn.Close` inline would block the publisher for up to 10s (5s+5s handshake budget).
2. **Publish short-circuits on closed hub**: if `h.closed` is true, return immediately with no fan-out, no log.
3. **Warn log format (exact)**: `h.logger.Warn("wshub: subscriber dropped (slow consumer)", "buffer_size", h.opts.MessageBuffer)`. No PII, no subscriber identity, no peer address. The log lives INSIDE `Publish` (not in `closeSlow`) because `closeSlow` runs in a detached goroutine and would need the logger plumbed into the closure.
4. **Close idempotence**: first thing under `h.mu.Lock()` is `if h.closed { return }`; then `h.closed = true`; then log once; then iterate subscribers firing `go s.closeGoingAway()`. Do NOT clear `h.subscribers` — the writer loops clean themselves via `defer h.removeSubscriber(s)` when they observe the close.
5. **Info log format (exact)**: `h.logger.Info("wshub: closing hub", "subscribers", len(h.subscribers))`. Logged ONCE at hub level, NOT per-subscriber.
6. **Ping case in writer loop**: `pingCtx, cancel := context.WithTimeout(ctx, h.opts.PingTimeout); err := conn.Ping(pingCtx); cancel()`. On error, log at Debug and return. On ctx.Canceled / ctx.DeadlineExceeded from a clean shutdown, return nil (same as the msgs case).
7. **No logging of dropped message content** (PII risk in Phase 4 when messages contain app names).
8. **No ticker injection** — tests use short `PingInterval` values (10-50ms) via `Options` and `require.Eventually` for polling.
9. **Internal test file naming**: new tests go in `hub_test.go` (created this plan) for Hub-level behavior and append to `subscribe_test.go` for Subscribe-level behavior (the ping-keepalive test). Both files are `package wshub` (not `wshub_test`) so they can touch unexported `h.mu` / `h.subscribers`.
</locked_decisions>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: RED — hub_test.go with slow-consumer, fan-out, Close-goes-away, Publish-after-Close; ping-keepalive appended to subscribe_test.go</name>
  <files>internal/wshub/hub_test.go, internal/wshub/subscribe_test.go</files>
  <read_first>
- internal/wshub/hub.go (Plan 03-01 stubs: Publish no-op, Close no-op, h.closed field already declared)
- internal/wshub/subscribe.go (Plan 03-01 writer loop: tick.C case is a TODO stub)
- internal/wshub/subscribe_test.go (existing testLogger helper, existing three tests from 03-01 — APPEND to this file, do not replace)
- .planning/phases/03-websocket-hub/03-CONTEXT.md §"Close semantics", §"Drop-subscriber logging", §"Test surface"
- .planning/phases/03-websocket-hub/03-RESEARCH.md §"Example 2: Non-blocking Publish fan-out with Warn logging" (verbatim), §"Example 3: Idempotent Close with StatusGoingAway" (verbatim), §"Example 5: Slow-consumer kick test" (verbatim), §"Pitfall 1: Broadcasting under a mutex", §"Pitfall 9: Ping requires a concurrent reader"
- .planning/phases/03-websocket-hub/03-VALIDATION.md rows 03-02-01, 03-02-02, 03-02-03
- internal/registry/store_test.go TestStore_ConcurrentAccess (mutex-lock-test style template)
- .planning/research/PITFALLS.md §1 (broadcasting under a mutex)
  </read_first>
  <behavior>
**New file `internal/wshub/hub_test.go`** — four tests in `package wshub`:

1. **TestHub_Publish_FanOut** — connects two subscribers to a single hub via httptest.NewServer. Publishes one byte-slice. Both subscribers MUST receive the message (read via `conn.Read(ctx)` on the test client) within 1 second (`require.Eventually`). Uses a large-enough MessageBuffer (default 16) so neither is dropped.

2. **TestHub_SlowConsumerDropped** — the canonical slow-consumer kick. Connects one subscriber with `Options{MessageBuffer: 1, PingInterval: 10 * time.Millisecond}`. The test client NEVER reads from its conn (that's the "slow" simulation). Waits via `require.Eventually` until the hub has exactly one subscriber (observed via `h.mu.Lock(); len(h.subscribers); h.mu.Unlock()`). Publishes 5 messages. Asserts via `require.Eventually` that `len(h.subscribers) == 0` within 1 second. This is the WS-03 correctness test.

3. **TestHub_Close_GoingAway** — connects one subscriber, waits until the hub has one subscriber, then calls `hub.Close()`. Asserts: `hub.closed == true`; `len(hub.subscribers) == 0` within 1 second (the writer loop observes its conn close and calls `defer h.removeSubscriber(s)`); a second call to `hub.Close()` returns without error and without panic (idempotence).

4. **TestHub_Publish_AfterCloseIsNoOp** — connects no subscribers, calls `hub.Close()`, then calls `hub.Publish([]byte("noise"))`. Asserts no panic, no log line produced (using a bytes.Buffer logger and asserting the buffer contains ONLY the "closing hub" Info line, not any drop-related Warn line).

**Appended to existing `internal/wshub/subscribe_test.go`**:

5. **TestSubscribe_PingKeepsAlive** — connects one subscriber with `Options{PingInterval: 10 * time.Millisecond}` and a read-loop on the test client side that drains all incoming data/control frames (uses `conn.Read(ctx)` in a goroutine with `context.WithTimeout`). Waits 200ms (10+ ping cycles) via `require.Eventually` on a condition that's TRUE on entry (e.g., `return true` after the expected delay is reached via a ticker — still no `time.Sleep`). Actually, simpler: use `require.Never` with a short window to assert the subscriber stays in `hub.subscribers` despite no traffic.

    Pattern (no time.Sleep):
    ```go
    start := time.Now()
    require.Never(t, func() bool {
        hub.mu.Lock()
        n := len(hub.subscribers)
        hub.mu.Unlock()
        return n == 0
    }, 300*time.Millisecond, 20*time.Millisecond,
       "ping keepalive failed — subscriber dropped after %v", time.Since(start))
    ```
    This proves that across 300ms (30+ ping intervals at 10ms), the subscriber stays registered. If ping traffic were broken, the conn would eventually be closed by the server side or the writer loop would error out; either way, `h.subscribers` would shrink to 0.

All five tests MUST fail initially because `Publish`, `Close`, and the tick.C ping case are stubs from Plan 03-01.
  </behavior>
  <action>
**RED phase: create `internal/wshub/hub_test.go` and append one test to `internal/wshub/subscribe_test.go`.**

**Step 1: Create `internal/wshub/hub_test.go` with EXACTLY this content** (paste verbatim):

```go
package wshub

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

// subscribeHandler is a minimal httptest handler that accepts a WS
// conn and hands it to hub.Subscribe. Used by every integration test
// in this file.
func subscribeHandler(hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		_ = hub.Subscribe(r.Context(), conn)
	}
}

// waitForSubscribers polls hub.subscribers length via the internal mu.
// This is the justification for internal tests — the public API
// deliberately does not expose Stats() or NumSubscribers().
func waitForSubscribers(t *testing.T, hub *Hub, want int, within time.Duration) {
	t.Helper()
	require.Eventually(t, func() bool {
		hub.mu.Lock()
		defer hub.mu.Unlock()
		return len(hub.subscribers) == want
	}, within, 10*time.Millisecond, "expected %d subscribers", want)
}

func TestHub_Publish_FanOut(t *testing.T) {
	hub := New(testLogger(t), Options{PingInterval: 50 * time.Millisecond})
	srv := httptest.NewServer(subscribeHandler(hub))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Two subscribers.
	conn1, _, err := websocket.Dial(ctx, srv.URL, nil)
	require.NoError(t, err)
	defer conn1.CloseNow()

	conn2, _, err := websocket.Dial(ctx, srv.URL, nil)
	require.NoError(t, err)
	defer conn2.CloseNow()

	waitForSubscribers(t, hub, 2, time.Second)

	// Publish one message; both subscribers must receive it.
	hub.Publish([]byte("hello"))

	readOne := func(c *websocket.Conn) []byte {
		rctx, rcancel := context.WithTimeout(ctx, 2*time.Second)
		defer rcancel()
		_, data, err := c.Read(rctx)
		require.NoError(t, err)
		return data
	}
	require.Equal(t, []byte("hello"), readOne(conn1))
	require.Equal(t, []byte("hello"), readOne(conn2))
}

func TestHub_SlowConsumerDropped(t *testing.T) {
	hub := New(testLogger(t), Options{
		MessageBuffer: 1,
		PingInterval:  10 * time.Millisecond,
	})
	srv := httptest.NewServer(subscribeHandler(hub))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, srv.URL, nil)
	require.NoError(t, err)
	defer conn.CloseNow()

	// Do NOT read from conn — this is the slow-consumer simulation.
	waitForSubscribers(t, hub, 1, time.Second)

	// Publish enough messages to overflow the 1-slot buffer.
	for i := 0; i < 5; i++ {
		hub.Publish([]byte("msg"))
	}

	// The slow subscriber must be kicked.
	waitForSubscribers(t, hub, 0, time.Second)
}

func TestHub_Close_GoingAway(t *testing.T) {
	hub := New(testLogger(t), Options{PingInterval: 50 * time.Millisecond})
	srv := httptest.NewServer(subscribeHandler(hub))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, srv.URL, nil)
	require.NoError(t, err)
	defer conn.CloseNow()

	waitForSubscribers(t, hub, 1, time.Second)

	// First Close: kicks all subscribers, sets closed=true.
	hub.Close()

	hub.mu.Lock()
	require.True(t, hub.closed, "Close must set h.closed = true")
	hub.mu.Unlock()

	waitForSubscribers(t, hub, 0, time.Second)

	// Second Close: idempotent no-op (must not panic).
	require.NotPanics(t, func() { hub.Close() })
}

func TestHub_Publish_AfterCloseIsNoOp(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	hub := New(logger, Options{})

	// Close an empty hub, then publish. No panic, no Warn drop log,
	// and the only log line should be the Info "closing hub" from
	// Close itself (subscribers=0).
	hub.Close()
	hub.Publish([]byte("noise"))

	out := buf.String()
	require.Contains(t, out, "wshub: closing hub", "Close must log Info once")
	require.Contains(t, out, "subscribers=0", "Close log carries subscribers count")
	require.NotContains(t, out, "wshub: subscriber dropped", "Publish after Close must NOT fan out")
}
```

**Step 2: Append `TestSubscribe_PingKeepsAlive` to `internal/wshub/subscribe_test.go`** — add this function at the end of the file, preserving all existing test functions:

```go
// TestSubscribe_PingKeepsAlive asserts that a connected subscriber stays
// registered across many ping intervals with no inbound or outbound data
// traffic. If the ping loop were broken (PITFALLS #9: Ping requires a
// concurrent reader — CloseRead provides it), the subscriber would be
// dropped and h.subscribers would shrink to 0.
//
// We use require.Never (not require.Eventually) to assert the condition
// "len(h.subscribers) == 0" NEVER becomes true across 300ms — at a
// PingInterval of 10ms, that's 30+ ping cycles of aliveness.
func TestSubscribe_PingKeepsAlive(t *testing.T) {
	hub := New(testLogger(t), Options{PingInterval: 10 * time.Millisecond})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		_ = hub.Subscribe(r.Context(), conn)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, srv.URL, nil)
	require.NoError(t, err)
	defer conn.CloseNow()

	// Wait until the subscriber is registered.
	require.Eventually(t, func() bool {
		hub.mu.Lock()
		defer hub.mu.Unlock()
		return len(hub.subscribers) == 1
	}, time.Second, 10*time.Millisecond)

	// Across 300ms (30+ ping cycles), the subscriber must stay
	// registered. If Ping silently blocks because CloseRead is missing
	// or PingTimeout is ignored, this assertion fails.
	require.Never(t, func() bool {
		hub.mu.Lock()
		defer hub.mu.Unlock()
		return len(hub.subscribers) == 0
	}, 300*time.Millisecond, 20*time.Millisecond,
		"ping keepalive failed — subscriber was dropped during idle period")
}
```

Run the tests and confirm they fail (RED bar):

```
cd /home/ben/Dev-local/openburo-spec/open-buro-server
~/sdk/go1.26.2/bin/go test ./internal/wshub -race -run '^TestHub_Publish_FanOut$|^TestHub_SlowConsumerDropped$|^TestHub_Close_GoingAway$|^TestHub_Publish_AfterCloseIsNoOp$|^TestSubscribe_PingKeepsAlive$' -timeout 30s -v
```

Expected outcome: all five tests FAIL. `TestHub_Publish_FanOut` fails because Publish is a no-op (conn.Read times out). `TestHub_SlowConsumerDropped` fails for the same reason. `TestHub_Close_GoingAway` fails because Close is a no-op (h.closed stays false). `TestHub_Publish_AfterCloseIsNoOp` fails because Close doesn't log. `TestSubscribe_PingKeepsAlive` may pass or fail depending on the 03-01 tick.C stub's exact behavior — if it does pass, that's still fine, the GREEN phase must keep it passing.

Commit as: `test(03-02): add slow-consumer, fan-out, Close-GoingAway, Publish-after-Close, ping-keepalive tests (RED)`.
  </action>
  <verify>
    <automated>cd /home/ben/Dev-local/openburo-spec/open-buro-server && ! ~/sdk/go1.26.2/bin/go test ./internal/wshub -race -run '^TestHub_Publish_FanOut$|^TestHub_SlowConsumerDropped$|^TestHub_Close_GoingAway$|^TestHub_Publish_AfterCloseIsNoOp$' -timeout 30s</automated>
  </verify>
  <acceptance_criteria>
- `internal/wshub/hub_test.go` exists
- `grep -c 'func TestHub_Publish_FanOut' internal/wshub/hub_test.go` equals 1
- `grep -c 'func TestHub_SlowConsumerDropped' internal/wshub/hub_test.go` equals 1
- `grep -c 'func TestHub_Close_GoingAway' internal/wshub/hub_test.go` equals 1
- `grep -c 'func TestHub_Publish_AfterCloseIsNoOp' internal/wshub/hub_test.go` equals 1
- `grep -c 'func TestSubscribe_PingKeepsAlive' internal/wshub/subscribe_test.go` equals 1
- `grep -c 'require.Never' internal/wshub/subscribe_test.go` is at least 1
- `grep -c 'hub.mu.Lock' internal/wshub/hub_test.go` is at least 2 (justifies internal test access)
- `! grep -n 'time\.Sleep' internal/wshub/hub_test.go internal/wshub/subscribe_test.go` (PITFALLS #16 gate)
- `~/sdk/go1.26.2/bin/go test ./internal/wshub -race -run '^TestHub_Publish_FanOut$|^TestHub_SlowConsumerDropped$|^TestHub_Close_GoingAway$|^TestHub_Publish_AfterCloseIsNoOp$' -timeout 30s` EXITS NON-ZERO (RED-bar confirmation — at least one of the four must fail because of stubbed Publish/Close)
- Existing Plan 03-01 tests (`TestHub_New_PanicsOnNilLogger`, `TestHub_DefaultOptions`, `TestSubscribe_NoGoroutineLeak`) STILL PASS (no regression)
  </acceptance_criteria>
  <done>
`hub_test.go` created with four tests; `subscribe_test.go` has `TestSubscribe_PingKeepsAlive` appended; Plan 03-01's three tests remain green; the four new Hub-level tests are RED. Commit recorded.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: GREEN — real Publish + Close bodies in hub.go; real ping case in subscribe.go tick.C branch</name>
  <files>internal/wshub/hub.go, internal/wshub/subscribe.go</files>
  <read_first>
- internal/wshub/hub.go (current state: Publish and Close are TODO(03-02) stubs)
- internal/wshub/subscribe.go (current state: tick.C case is a TODO(03-02) stub)
- internal/wshub/hub_test.go (the RED tests from Task 1 — this file's purpose is to make them GREEN)
- internal/wshub/subscribe_test.go (TestSubscribe_PingKeepsAlive — also must GREEN)
- .planning/phases/03-websocket-hub/03-CONTEXT.md §"Drop-subscriber logging" (exact Warn/Info message strings), §"Close semantics" (idempotence + don't-clear-subscribers rationale)
- .planning/phases/03-websocket-hub/03-RESEARCH.md §"Example 2: Non-blocking Publish fan-out" (verbatim), §"Example 3: Idempotent Close with StatusGoingAway" (verbatim), §"Example 1: Minimal Subscribe writer loop" (tick.C case body — verbatim)
- internal/registry/store.go (mu.Lock/defer mu.Unlock discipline to mirror)
- .planning/research/PITFALLS.md §1 (broadcasting under a mutex — the `go s.closeSlow()` keyword is load-bearing)
  </read_first>
  <action>
**GREEN phase: replace the three stubs from Plan 03-01 with real bodies.**

**Step 1: Replace `Publish` and `Close` bodies in `internal/wshub/hub.go`.**

Locate the stub methods (they currently contain `_ = msg` and `{}`) and replace them with the following, VERBATIM:

```go
// Publish delivers msg to every active subscriber without blocking.
// Slow subscribers whose outbound buffer is full are kicked via
// closeSlow rather than stalling the publisher.
//
// If the hub is closed, Publish is a silent no-op — it does not log,
// fan out, or panic. This makes it safe for Phase 5's two-phase
// shutdown to race with in-flight HTTP handlers.
//
// The `go` keyword on closeSlow is load-bearing: conn.Close has a
// 5s+5s handshake budget, and we hold h.mu during the loop. Calling
// closeSlow inline would block every other subscriber's enqueue.
func (h *Hub) Publish(msg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	for s := range h.subscribers {
		select {
		case s.msgs <- msg:
			// queued
		default:
			// Slow consumer — log Warn, kick off-mutex via `go`.
			h.logger.Warn("wshub: subscriber dropped (slow consumer)",
				"buffer_size", h.opts.MessageBuffer)
			go s.closeSlow()
		}
	}
}

// Close shuts down the hub, sending a StatusGoingAway close frame to
// every active subscriber. Idempotent: a second call is a no-op.
// Returns no error — graceful shutdown must not report failure.
//
// Close does NOT clear h.subscribers. Each writer loop observes its
// conn close as a write error (or ctx.Done() cancellation from the
// client-side TCP close) and calls `defer h.removeSubscriber(s)`.
// Clearing the map here would race with those defers.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return // idempotent: second call is a no-op
	}
	h.closed = true
	h.logger.Info("wshub: closing hub", "subscribers", len(h.subscribers))
	for s := range h.subscribers {
		go s.closeGoingAway()
	}
}
```

**Step 2: Replace the `tick.C` case body in `internal/wshub/subscribe.go`.**

Locate the `case <-tick.C:` branch in the writer loop (currently a `TODO(03-02)` comment) and replace it with:

```go
		case <-tick.C:
			pingCtx, cancel := context.WithTimeout(ctx, h.opts.PingTimeout)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return nil
				}
				h.logger.Debug("wshub: subscriber writer loop exited", "error", err.Error())
				return err
			}
```

The `errors` import already exists in `subscribe.go` from Plan 03-01 (the msgs case already uses `errors.Is`). No new imports needed.

**Step 3: Run the full wshub test suite under -race.**

```
cd /home/ben/Dev-local/openburo-spec/open-buro-server
~/sdk/go1.26.2/bin/go test ./internal/wshub -race -timeout 60s -v
```

Expected outcome: ALL tests pass — Plan 03-01's three (`TestHub_New_PanicsOnNilLogger`, `TestHub_DefaultOptions`, `TestSubscribe_NoGoroutineLeak`) AND Plan 03-02's five (`TestHub_Publish_FanOut`, `TestHub_SlowConsumerDropped`, `TestHub_Close_GoingAway`, `TestHub_Publish_AfterCloseIsNoOp`, `TestSubscribe_PingKeepsAlive`).

Commit as: `feat(03-02): implement Publish fan-out, Close(StatusGoingAway), ping keepalive (GREEN)`.

Note: if `TestHub_SlowConsumerDropped` is flaky under heavy CI load, do NOT increase timeouts or loosen assertions. Investigate instead — flakiness here indicates a real race. The `require.Eventually` budget (1 second) is already generous for a drop that happens within ~10-20ms of the publish under normal conditions.
  </action>
  <verify>
    <automated>cd /home/ben/Dev-local/openburo-spec/open-buro-server && ~/sdk/go1.26.2/bin/go test ./internal/wshub -race -timeout 60s && ~/sdk/go1.26.2/bin/go vet ./internal/wshub && ~/sdk/go1.26.2/bin/gofmt -l internal/wshub/hub.go internal/wshub/subscribe.go</automated>
  </verify>
  <acceptance_criteria>
- `grep -c 'h.mu.Lock()' internal/wshub/hub.go` is at least 4 (addSubscriber, removeSubscriber, Publish, Close)
- `grep -c 'if h.closed' internal/wshub/hub.go` equals 2 (Publish short-circuit, Close idempotence guard)
- `grep -c 'go s.closeSlow()' internal/wshub/hub.go` equals 1
- `grep -c 'go s.closeGoingAway()' internal/wshub/hub.go` equals 1
- `grep -c 'wshub: subscriber dropped (slow consumer)' internal/wshub/hub.go` equals 1
- `grep -c 'wshub: closing hub' internal/wshub/hub.go` equals 1
- `grep -c 'buffer_size' internal/wshub/hub.go` equals 1
- `grep -c 'conn.Ping(pingCtx)' internal/wshub/subscribe.go` equals 1
- `! grep -n 'TODO(03-02)' internal/wshub/hub.go internal/wshub/subscribe.go` (all Plan 03-01 stubs filled in)
- `! grep -rE 'slog\.Default' internal/wshub/hub.go internal/wshub/subscribe.go` (no-slog.Default gate intact)
- `~/sdk/go1.26.2/bin/go test ./internal/wshub -race -timeout 60s` exits 0 (all 8 tests GREEN)
- `~/sdk/go1.26.2/bin/go vet ./internal/wshub` exits 0
- `~/sdk/go1.26.2/bin/gofmt -l internal/wshub/hub.go internal/wshub/subscribe.go` produces no output
- `~/sdk/go1.26.2/bin/go list -deps ./internal/wshub | grep -E '^github\.com/openburo/openburo-server/internal/(registry|httpapi)$'` produces no output (byte-oriented contract)
  </acceptance_criteria>
  <done>
Publish fan-out with non-blocking select-default + Warn log is live. Close is idempotent, logs Info once, fires closeGoingAway off-mutex. Ping keepalive works. All eight tests (three from 03-01 + five from 03-02) pass under `-race`. The `TODO(03-02)` markers from Plan 03-01 are gone. Phase 3's two load-bearing correctness requirements (PITFALLS #3 goroutine leak, PITFALLS #4 slow-client stall) are both proven.
  </done>
</task>

</tasks>

<verification>
Overall plan verification (run after both tasks land):

```
cd /home/ben/Dev-local/openburo-spec/open-buro-server

# 1. Full wshub test suite under -race.
~/sdk/go1.26.2/bin/go test ./internal/wshub -race -timeout 60s

# 2. Architectural gate: wshub does NOT import registry or httpapi.
~/sdk/go1.26.2/bin/go list -deps ./internal/wshub | grep -E '^github\.com/openburo/openburo-server/internal/(registry|httpapi)$' && { echo "GATE FAIL: wshub leaks into registry/httpapi" >&2; exit 1; } || true

# 3. Logging gate: no slog.Default in production wshub code.
grep -rE 'slog\.Default' internal/wshub/*.go | grep -v _test.go && { echo "GATE FAIL: slog.Default found in wshub production code" >&2; exit 1; } || true

# 4. No time.Sleep gate: PITFALLS #16 enforcement in wshub tests.
grep -n 'time\.Sleep' internal/wshub/*_test.go && { echo "GATE FAIL: time.Sleep found in wshub tests" >&2; exit 1; } || true

# 5. No leftover TODO(03-02) markers from Plan 03-01 stubs.
grep -n 'TODO(03-02)' internal/wshub/*.go && { echo "GATE FAIL: Plan 03-01 stubs still marked TODO(03-02)" >&2; exit 1; } || true

# 6. Full module build + vet + gofmt.
~/sdk/go1.26.2/bin/go build ./...
~/sdk/go1.26.2/bin/go vet ./...
~/sdk/go1.26.2/bin/gofmt -l internal/wshub/
```

All checks MUST exit 0.
</verification>

<success_criteria>
- `Hub.Publish` uses the canonical non-blocking `select { case s.msgs <- msg: default: ... }` pattern under `h.mu.Lock()` with `go s.closeSlow()` on the default branch (WS-03)
- `Hub.Close` is idempotent, logs exactly once at Info, and fires `go s.closeGoingAway()` for every subscriber
- The writer loop pings every `opts.PingInterval` with a `context.WithTimeout(ctx, opts.PingTimeout)`-bounded `conn.Ping` call (WS-07)
- A slow subscriber (MessageBuffer=1, no client reads) is dropped within 1 second of the second publish (`TestHub_SlowConsumerDropped` green)
- A connected subscriber survives 30+ ping cycles with no data traffic (`TestSubscribe_PingKeepsAlive` green via `require.Never`)
- `Hub.Close` kicks all subscribers and the second call is a no-op (`TestHub_Close_GoingAway` green)
- Publishing after Close is a silent no-op with no drop logs (`TestHub_Publish_AfterCloseIsNoOp` green)
- All 8 Phase 3 tests pass under `-race` in the wshub package
- `go list -deps ./internal/wshub` still produces zero matches for `registry` or `httpapi`
- `TODO(03-02)` markers are gone; `slog.Default` is not present in production wshub code
</success_criteria>

<output>
After completion, create `.planning/phases/03-websocket-hub/03-02-SUMMARY.md` following the template. Include: exact log line strings shipped, the "go keyword is load-bearing" rationale preserved in comments, test names now GREEN, and confirmation that Plan 03-01's tests still pass (no regression).
</output>
