---
phase: 03-websocket-hub
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/wshub/doc.go
  - internal/wshub/hub.go
  - internal/wshub/subscribe.go
  - internal/wshub/subscribe_test.go
  - go.mod
  - go.sum
autonomous: true
gap_closure: false
requirements:
  - WS-02
  - WS-04
requirements_addressed:
  - WS-02
  - WS-04

must_haves:
  truths:
    - "New(nil, opts) panics immediately with a clear message that tells test authors how to build a no-op logger"
    - "New(logger, Options{}) returns a Hub whose zero-valued Options fields are replaced by defaultMessageBuffer=16, defaultPingInterval=30s, defaultWriteTimeout=5s, defaultPingTimeout=10s"
    - "Subscribe calls conn.CloseRead(ctx) exactly once at the top of the method so control frames are handled and ctx cancels on peer close"
    - "Subscribe installs `defer h.removeSubscriber(s)` so silent disconnects (network drop, tab closed) cannot leak the writer goroutine"
    - "Subscribe installs `defer conn.CloseNow()` as a safety-net so an early return from an error path reaps the underlying TCP conn"
    - "The 1000-cycle goroutine-leak test against httptest.NewServer ends with runtime.NumGoroutine() <= baseline+5 (via require.Eventually, NO time.Sleep)"
    - "go list -deps ./internal/wshub | grep -E 'registry|httpapi' produces no output (the byte-oriented contract is enforced structurally)"
  artifacts:
    - path: "internal/wshub/hub.go"
      provides: "Hub, Options, New, addSubscriber, removeSubscriber, default constants, stub Publish + Close"
      contains: "type Hub struct"
    - path: "internal/wshub/subscribe.go"
      provides: "subscriber struct + Subscribe method with CloseRead + defer removeSubscriber + writer loop skeleton"
      contains: "func (h *Hub) Subscribe"
    - path: "internal/wshub/subscribe_test.go"
      provides: "TestHub_DefaultOptions, TestHub_New_PanicsOnNilLogger, TestSubscribe_NoGoroutineLeak"
      contains: "TestSubscribe_NoGoroutineLeak"
    - path: "internal/wshub/doc.go"
      provides: "Updated package doc (Phase 1 placeholder sentence removed)"
      contains: "Package wshub"
  key_links:
    - from: "internal/wshub/subscribe.go Subscribe"
      to: "github.com/coder/websocket Conn.CloseRead"
      via: "ctx = conn.CloseRead(ctx) at the top of the method"
      pattern: "conn\\.CloseRead\\(ctx\\)"
    - from: "internal/wshub/subscribe.go Subscribe"
      to: "internal/wshub/hub.go removeSubscriber"
      via: "defer h.removeSubscriber(s)"
      pattern: "defer h\\.removeSubscriber"
    - from: "internal/wshub/subscribe_test.go TestSubscribe_NoGoroutineLeak"
      to: "runtime.NumGoroutine + require.Eventually"
      via: "1000-cycle dial/close loop against httptest.NewServer"
      pattern: "runtime\\.NumGoroutine"
---

<objective>
Ship the foundation of `internal/wshub`: the `Hub` struct, `Options` struct with zero-value defaults, the `subscriber` struct, `New`, `addSubscriber`, `removeSubscriber`, and the `Subscribe` writer loop. The first commit of this phase lands the two critical research flags — `conn.CloseRead(ctx)` and `defer removeSubscriber` (PITFALLS #3) — and the goroutine-leak test that enforces them (WS-10's sampling preview; the full WS-10 acceptance test also runs in Plan 03-03 for defense in depth).

`Publish` and `Close` are **stub-only** in this plan: `Publish` is a no-op (body contains only `_ = msg`) and `Close` is a no-op. This lets Plan 03-02 fill them in without touching `Subscribe`. The writer loop IS functional — it selects on `<-s.msgs` and `<-ctx.Done()` and writes with a bounded context — but the ping ticker path and the Publish fan-out land in Plan 03-02.

Purpose: Solve Phase 3's biggest correctness risk (goroutine leak on silent disconnect, PITFALLS #3) with TDD before the fan-out logic exists. The leak test is the correctness oracle for the whole phase; landing it first means every later change is guarded.

Output: `hub.go`, `subscribe.go`, `subscribe_test.go`, updated `doc.go`, and `github.com/coder/websocket v1.8.14` added to `go.mod`.
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
@.planning/research/ARCHITECTURE.md
@.planning/research/PITFALLS.md
@internal/wshub/doc.go
@internal/registry/store.go
@internal/registry/store_test.go

<interfaces>
<!-- Canonical type shapes from CONTEXT.md + RESEARCH.md. Paste VERBATIM into hub.go / subscribe.go. -->

```go
// Options configures a Hub. Zero-value fields use documented defaults.
type Options struct {
    // MessageBuffer is the per-subscriber outbound channel capacity.
    // Zero means use defaultMessageBuffer (16). Slow subscribers whose
    // buffer fills are kicked via closeSlow.
    MessageBuffer int

    // PingInterval is how often the Hub sends a WebSocket ping frame
    // to keep idle connections alive. Zero means use defaultPingInterval
    // (30 * time.Second).
    PingInterval time.Duration

    // WriteTimeout bounds each conn.Write call so a wedged connection
    // cannot stall the writer goroutine. Zero means use defaultWriteTimeout
    // (5 * time.Second).
    WriteTimeout time.Duration

    // PingTimeout bounds each conn.Ping call separately from WriteTimeout
    // so the two budgets don't compete. Zero means use defaultPingTimeout
    // (10 * time.Second).
    PingTimeout time.Duration
}

// Hub is the byte-oriented broadcast hub: a mutex-guarded map of
// subscribers that fans out []byte payloads non-blockingly. The hub
// knows nothing about the registry package — events are opaque byte
// slices supplied by the HTTP handler layer.
type Hub struct {
    logger      *slog.Logger
    opts        Options

    mu          sync.Mutex
    subscribers map[*subscriber]struct{}
    closed      bool
}

// subscriber represents a single WebSocket subscriber. Messages are
// sent on msgs; if the client cannot keep up, closeSlow is called by
// Publish. closeGoingAway is called by Hub.Close at shutdown.
type subscriber struct {
    msgs           chan []byte
    closeSlow      func() // Publish path: StatusPolicyViolation
    closeGoingAway func() // Hub.Close path: StatusGoingAway
}

// Default values used when Options fields are zero.
const (
    defaultMessageBuffer = 16
    defaultPingInterval  = 30 * time.Second
    defaultWriteTimeout  = 5 * time.Second
    defaultPingTimeout   = 10 * time.Second
)
```
</interfaces>

<locked_decisions>
<!-- All from 03-CONTEXT.md. DO NOT re-open. -->
1. **Constructor**: `func New(logger *slog.Logger, opts Options) *Hub`. `New(nil, opts)` panics with the test-author-friendly message. No `slog.Default()` fallback.
2. **Zero-value defaults**: replace zero-valued fields with package-level constants inside `New`. Mutate a local copy of `opts` and store it on the Hub.
3. **Mutex**: `sync.Mutex`, not `sync.RWMutex` (Publish iterates with the lock held — write-equivalent access). Follow `internal/registry/store.go`'s `defer mu.Unlock()` discipline.
4. **Package doc location**: keep in `doc.go` (Phase 2 moved registry's doc into `manifest.go`; for Phase 3 the doc stays in `doc.go` because it already exists and the package-doc-in-doc.go convention is equally valid).
5. **`Publish` and `Close` in this plan are stubs.** Leave a `TODO(03-02)` comment in each. Landing the real bodies is Plan 03-02's job. Do NOT anticipate 03-02's logic here.
6. **No `recover()` in the writer loop.** Phase 4 middleware (API-08) catches any panic. Recovering here would hide real bugs.
7. **ctx flows through CloseRead**: `ctx = conn.CloseRead(ctx)` uses the caller's ctx, NOT `context.Background()`. HTTP-layer cancellation must reach the writer loop.
8. **Package structure**: `doc.go` + `hub.go` + `subscribe.go` + test files. <250 LoC in the three production files combined.
9. **`go get github.com/coder/websocket@v1.8.14`** is a Task 0 prereq executed via Bash before Task 1 (NOT via a Go file modification).
</locked_decisions>
</context>

<tasks>

<task type="auto">
  <name>Task 0: Add coder/websocket v1.8.14 dependency</name>
  <files>go.mod, go.sum</files>
  <read_first>
- go.mod (to see the current five-dep pinned state from Phase 1)
- .planning/phases/03-websocket-hub/03-RESEARCH.md §"Standard Stack" (v1.8.14 verification)
  </read_first>
  <action>
Run, from the repo root, via Bash (NOT via a Go source modification):

```
~/sdk/go1.26.2/bin/go get github.com/coder/websocket@v1.8.14
~/sdk/go1.26.2/bin/go mod tidy
```

After these commands, `go.mod` MUST contain a `require` line for `github.com/coder/websocket v1.8.14` and `go.sum` MUST contain the corresponding checksum lines. Do NOT manually edit `go.mod` — let the tooling write it.

Do not add any other dependencies. The only new direct dep is `github.com/coder/websocket v1.8.14`. If `go mod tidy` wants to add or remove anything else (besides the transitive deps of `coder/websocket`, which are none as of v1.8.14), investigate rather than accept.
  </action>
  <verify>
    <automated>~/sdk/go1.26.2/bin/go list -m github.com/coder/websocket | grep -q '^github.com/coder/websocket v1.8.14$' && ~/sdk/go1.26.2/bin/go build ./...</automated>
  </verify>
  <acceptance_criteria>
- `grep -q 'github.com/coder/websocket v1.8.14' go.mod` exits 0
- `grep -q 'github.com/coder/websocket v1.8.14' go.sum` exits 0
- `~/sdk/go1.26.2/bin/go list -m github.com/coder/websocket` prints exactly `github.com/coder/websocket v1.8.14`
- `~/sdk/go1.26.2/bin/go build ./...` exits 0
- `~/sdk/go1.26.2/bin/go mod tidy` is a no-op after this task (idempotent)
  </acceptance_criteria>
  <done>
`go.mod` lists `github.com/coder/websocket v1.8.14` as a direct dependency; `go.sum` has matching sums; `go build ./...` compiles (the existing Phase 1/2 packages remain intact — no wshub code yet in this task).
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 1: RED — goroutine-leak test + default-options test + nil-logger-panic test</name>
  <files>internal/wshub/subscribe_test.go</files>
  <read_first>
- internal/wshub/doc.go (existing Phase 1 placeholder)
- .planning/phases/03-websocket-hub/03-CONTEXT.md §"Test surface" + §"Close semantics" + §"Hub constructor"
- .planning/phases/03-websocket-hub/03-RESEARCH.md §"Example 4: Goroutine-leak test" (paste verbatim) + §"Pitfall 10: 1000 rapid Dial+Close cycles" (why sequential, not parallel)
- .planning/phases/03-websocket-hub/03-VALIDATION.md rows 03-01-01 and 03-01-02
- internal/registry/store_test.go `TestStore_ConcurrentAccess` (style template: require + table-driven + no time.Sleep)
- .planning/research/PITFALLS.md §3 (goroutine leak) and §16 (flaky time-based tests)
  </read_first>
  <behavior>
Three tests, all in `package wshub` (internal test file so we can touch unexported fields):

1. **TestHub_New_PanicsOnNilLogger** — calls `New(nil, Options{})` and asserts a panic with `require.PanicsWithValue` or `require.Panics`. The panic message MUST mention "logger" so test authors know what went wrong.

2. **TestHub_DefaultOptions** — calls `New(testLogger(t), Options{})` and asserts the Hub's internal `opts` field has `MessageBuffer == 16`, `PingInterval == 30 * time.Second`, `WriteTimeout == 5 * time.Second`, `PingTimeout == 10 * time.Second`. Also calls `New(testLogger(t), Options{MessageBuffer: 4, PingInterval: 5 * time.Millisecond, WriteTimeout: 100 * time.Millisecond, PingTimeout: 200 * time.Millisecond})` and asserts those overrides are preserved (zero-value substitution is selective).

3. **TestSubscribe_NoGoroutineLeak** — the load-bearing correctness test. 1000 sequential dial+close cycles against an `httptest.NewServer` wrapping `hub.Subscribe`. Uses `runtime.GC()` before baseline, then 1000 iterations of `websocket.Dial(ctx, srv.URL, nil)` + `conn.Close(websocket.StatusNormalClosure, "")`. Asserts via `require.Eventually(t, func() bool { runtime.GC(); return runtime.NumGoroutine() <= baseline+5 }, 2*time.Second, 20*time.Millisecond)`. NO `time.Sleep` anywhere.

Include a package-local `testLogger` helper that returns `slog.New(slog.NewTextHandler(io.Discard, nil))`.

This is the RED phase: these tests MUST NOT compile yet because `Hub`, `New`, `Subscribe` don't exist. That's the point — the RED commit is a red-bar moment that proves the test catches the absence of the implementation.

Note on the leak test's `hub` variable: even though this plan's `Publish` is a stub, `Subscribe` fully runs its writer loop (minus the ping path which is Plan 03-02). The leak test does not depend on Publish or ping — it only depends on `Subscribe` entering its select and returning cleanly on `ctx.Done()` after `conn.Close(StatusNormalClosure, "")`.
  </behavior>
  <action>
**RED phase: write the test file first. The whole file will fail to compile — that's expected.**

Create `internal/wshub/subscribe_test.go` with EXACTLY this content (paste verbatim, do not paraphrase):

```go
package wshub

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

// testLogger returns a discard-backed slog logger for tests that don't
// care about log output. Tests that DO capture logs (see hub_test.go in
// Plan 03-03) construct their own logger with a bytes.Buffer handler.
func testLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestHub_New_PanicsOnNilLogger(t *testing.T) {
	require.PanicsWithValue(
		t,
		"wshub.New: logger is required; use slog.New(slog.NewTextHandler(io.Discard, nil)) in tests",
		func() { New(nil, Options{}) },
	)
}

func TestHub_DefaultOptions(t *testing.T) {
	// Zero-value Options should be replaced with package defaults.
	h := New(testLogger(t), Options{})
	require.Equal(t, 16, h.opts.MessageBuffer, "default MessageBuffer")
	require.Equal(t, 30*time.Second, h.opts.PingInterval, "default PingInterval")
	require.Equal(t, 5*time.Second, h.opts.WriteTimeout, "default WriteTimeout")
	require.Equal(t, 10*time.Second, h.opts.PingTimeout, "default PingTimeout")

	// Non-zero overrides should be preserved exactly.
	h2 := New(testLogger(t), Options{
		MessageBuffer: 4,
		PingInterval:  5 * time.Millisecond,
		WriteTimeout:  100 * time.Millisecond,
		PingTimeout:   200 * time.Millisecond,
	})
	require.Equal(t, 4, h2.opts.MessageBuffer)
	require.Equal(t, 5*time.Millisecond, h2.opts.PingInterval)
	require.Equal(t, 100*time.Millisecond, h2.opts.WriteTimeout)
	require.Equal(t, 200*time.Millisecond, h2.opts.PingTimeout)
}

// TestSubscribe_NoGoroutineLeak is THE correctness test for Phase 3.
// 1000 connect-then-disconnect cycles against an httptest.NewServer-backed
// hub MUST end with runtime.NumGoroutine() <= baseline+5. The +5 epsilon
// accounts for in-flight teardown goroutines caught mid-exit by the poll.
//
// This test enforces PITFALLS #3: CloseRead + defer removeSubscriber.
// If a future refactor drops either one, this test fails within seconds.
func TestSubscribe_NoGoroutineLeak(t *testing.T) {
	hub := New(testLogger(t), Options{PingInterval: 50 * time.Millisecond})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		_ = hub.Subscribe(r.Context(), conn)
	}))
	defer srv.Close()

	// Allow the httptest accept-loop goroutine to settle into the baseline.
	runtime.GC()
	baseline := runtime.NumGoroutine()

	for i := 0; i < 1000; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		conn, _, err := websocket.Dial(ctx, srv.URL, nil)
		require.NoError(t, err, "dial cycle %d", i)
		_ = conn.Close(websocket.StatusNormalClosure, "")
		cancel()
	}

	// Poll until the writer loops observe ctx.Done() and exit.
	// runtime.GC() inside the closure reaps finalizer-held conns.
	require.Eventually(t, func() bool {
		runtime.GC()
		return runtime.NumGoroutine() <= baseline+5
	}, 2*time.Second, 20*time.Millisecond,
		"goroutines did not drain after 1000 disconnect cycles")
}
```

Commit this as the RED commit: `test(03-01): add hub defaults, nil-logger panic, and 1000-cycle goroutine-leak tests (RED)`.

The commit MUST fail to compile at this point (`hub.Hub` type, `h.opts` field, `New` function, `Subscribe` method all undefined). That failure is the RED-bar signal.
  </action>
  <verify>
    <automated>cd /home/ben/Dev-local/openburo-spec/open-buro-server && ~/sdk/go1.26.2/bin/go vet ./internal/wshub 2>&1 | grep -qE 'undefined: (Hub|New|Options)|cannot find' && echo RED-OK</automated>
  </verify>
  <acceptance_criteria>
- `internal/wshub/subscribe_test.go` exists
- `grep -c 'func TestHub_New_PanicsOnNilLogger' internal/wshub/subscribe_test.go` equals 1
- `grep -c 'func TestHub_DefaultOptions' internal/wshub/subscribe_test.go` equals 1
- `grep -c 'func TestSubscribe_NoGoroutineLeak' internal/wshub/subscribe_test.go` equals 1
- `grep -c 'runtime.GC()' internal/wshub/subscribe_test.go` is at least 2 (baseline + inside poll)
- `grep -c 'require.Eventually' internal/wshub/subscribe_test.go` is at least 1
- `! grep -n 'time\.Sleep' internal/wshub/subscribe_test.go` (no time.Sleep anywhere — PITFALLS #16)
- `~/sdk/go1.26.2/bin/go vet ./internal/wshub` FAILS with at least one `undefined:` error for `Hub`, `New`, or `Options` (RED-bar confirmation)
  </acceptance_criteria>
  <done>
`subscribe_test.go` on disk contains the three tests verbatim; compiling the wshub package fails because the production types don't exist; RED commit recorded.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: GREEN — Hub + Options + New + add/removeSubscriber + subscriber struct + Subscribe writer loop (stub Publish/Close)</name>
  <files>internal/wshub/hub.go, internal/wshub/subscribe.go, internal/wshub/doc.go</files>
  <read_first>
- internal/wshub/subscribe_test.go (the RED tests from Task 1 — this file's purpose is to make them GREEN)
- internal/wshub/doc.go (existing Phase 1 placeholder — extend it)
- .planning/phases/03-websocket-hub/03-CONTEXT.md §"Hub constructor", §"Close semantics", §"Package structure", §"Logger requirement at construction"
- .planning/phases/03-websocket-hub/03-RESEARCH.md §"Pattern 1: The Canonical Chat Hub" (verbatim chat example), §"Pattern 3: Two-Callback Subscriber" (Subscribe writer loop skeleton — paste verbatim), §"Example 1: Minimal Subscribe writer loop", §"Pitfall 1: Broadcasting under a mutex", §"Pitfall 2: Goroutine leak", §"Pitfall 9: Ping requires a concurrent reader"
- internal/registry/store.go (sync.Mutex discipline: `defer mu.Unlock()`, error wrapping voice)
- .planning/research/ARCHITECTURE.md §"Pattern 2: coder/websocket Canonical Chat Hub (Adapted)"
  </read_first>
  <action>
**GREEN phase: write the minimum production code to make Task 1's tests pass.**

**Step 1: Update `internal/wshub/doc.go`.**

Replace the file's entire content with:

```go
// Package wshub implements the WebSocket broadcast hub using the
// coder/websocket library. It holds a map of subscribers under a mutex
// and fans out events non-blockingly with drop-slow-consumer semantics.
//
// wshub intentionally knows nothing about the registry package — events
// are opaque byte slices supplied by the handler layer. This inversion
// keeps the dependency graph acyclic and makes the registry-hub ABBA
// deadlock structurally impossible from this side (see .planning/research/
// PITFALLS.md §1).
//
// The hub is the canonical coder/websocket chat-hub pattern, minus the
// rate limiter and plus four deltas: (1) injected *slog.Logger, (2) an
// Options struct with zero-value defaults, (3) a per-subscriber ping
// ticker inside the writer loop, (4) two close callbacks per subscriber
// (closeSlow for slow-consumer kicks, closeGoingAway for hub shutdown).
package wshub
```

The Phase 1 sentence "Phase 1 ships this file only; the real implementation lands in Phase 3." is removed — the real implementation IS this phase.

**Step 2: Create `internal/wshub/hub.go`.**

Paste verbatim:

```go
package wshub

import (
	"log/slog"
	"sync"
	"time"
)

// Default values used when Options fields are zero. Documented on the
// Options struct fields; duplicated here as unexported package-level
// constants so tests and the New constructor share one source of truth.
const (
	defaultMessageBuffer = 16
	defaultPingInterval  = 30 * time.Second
	defaultWriteTimeout  = 5 * time.Second
	defaultPingTimeout   = 10 * time.Second
)

// Options configures a Hub. Zero-value fields use documented defaults.
type Options struct {
	// MessageBuffer is the per-subscriber outbound channel capacity.
	// Zero means use the package default (16). Slow subscribers whose
	// buffer fills are kicked via closeSlow.
	MessageBuffer int

	// PingInterval is how often the Hub sends a WebSocket ping frame
	// to keep idle connections alive. Zero means use the package
	// default (30 * time.Second).
	PingInterval time.Duration

	// WriteTimeout bounds each conn.Write call so a wedged connection
	// cannot stall the writer goroutine. Zero means use the package
	// default (5 * time.Second).
	WriteTimeout time.Duration

	// PingTimeout bounds each conn.Ping call separately from
	// WriteTimeout so the two budgets don't compete. Zero means use
	// the package default (10 * time.Second).
	PingTimeout time.Duration
}

// Hub is the byte-oriented broadcast hub: a mutex-guarded map of
// subscribers that fans out []byte payloads non-blockingly. The hub
// knows nothing about the registry package — events are opaque byte
// slices supplied by the HTTP handler layer.
type Hub struct {
	logger *slog.Logger
	opts   Options

	mu          sync.Mutex
	subscribers map[*subscriber]struct{}
	closed      bool
}

// New constructs a Hub with the given logger and options. A nil logger
// panics at construction time — the hub must never fall back to
// slog.Default() (enforced by the "no slog.Default in internal/" gate
// from Phase 1).
//
// Zero-valued Options fields are replaced with package defaults.
func New(logger *slog.Logger, opts Options) *Hub {
	if logger == nil {
		panic("wshub.New: logger is required; use slog.New(slog.NewTextHandler(io.Discard, nil)) in tests")
	}
	if opts.MessageBuffer == 0 {
		opts.MessageBuffer = defaultMessageBuffer
	}
	if opts.PingInterval == 0 {
		opts.PingInterval = defaultPingInterval
	}
	if opts.WriteTimeout == 0 {
		opts.WriteTimeout = defaultWriteTimeout
	}
	if opts.PingTimeout == 0 {
		opts.PingTimeout = defaultPingTimeout
	}
	return &Hub{
		logger:      logger,
		opts:        opts,
		subscribers: make(map[*subscriber]struct{}),
	}
}

// addSubscriber registers s in the hub's subscriber set. Called at the
// top of Subscribe; paired with removeSubscriber via defer.
func (h *Hub) addSubscriber(s *subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subscribers[s] = struct{}{}
}

// removeSubscriber removes s from the hub's subscriber set. Idempotent:
// deleting an already-absent key is a no-op. Called via defer at the
// top of Subscribe so silent disconnects cannot leak.
func (h *Hub) removeSubscriber(s *subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subscribers, s)
}

// Publish delivers msg to every active subscriber without blocking.
// Slow subscribers whose outbound buffer is full are kicked via
// closeSlow rather than stalling the publisher.
//
// TODO(03-02): full non-blocking fan-out + Warn log lands in Plan 03-02.
// In Plan 03-01 this is a no-op so the goroutine-leak test in
// subscribe_test.go can run against a compilable package.
func (h *Hub) Publish(msg []byte) {
	_ = msg
}

// Close shuts down the hub, sending a StatusGoingAway close frame to
// every active subscriber. Idempotent: a second call is a no-op.
// Returns no error — graceful shutdown must not report failure.
//
// TODO(03-02): full close-with-StatusGoingAway + Info log + idempotent
// closed-flag lands in Plan 03-02. In Plan 03-01 this is a no-op.
func (h *Hub) Close() {
}
```

**Step 3: Create `internal/wshub/subscribe.go`.**

Paste verbatim:

```go
package wshub

import (
	"context"
	"errors"
	"time"

	"github.com/coder/websocket"
)

// subscriber represents a single WebSocket subscriber. Messages are
// sent on msgs; if the client cannot keep up, closeSlow is called by
// Publish. closeGoingAway is called by Hub.Close at shutdown.
//
// Both close callbacks are pre-bound to the *websocket.Conn at
// Subscribe time so the close-code decision lives at the boundary
// where the conn is known, keeping Publish and Close branch-free.
type subscriber struct {
	msgs           chan []byte
	closeSlow      func() // Publish path: StatusPolicyViolation
	closeGoingAway func() // Hub.Close path: StatusGoingAway
}

// Subscribe registers a new WebSocket subscriber on the hub and blocks
// until the client disconnects or ctx is canceled. It is the caller's
// responsibility to call websocket.Accept before handing the conn in,
// and to close the conn if Subscribe returns a non-context error.
//
// The method installs conn.CloseRead(ctx) at the top so control frames
// (ping, pong, close) are handled and ctx cancels on peer disconnect.
// It also installs `defer h.removeSubscriber(s)` so silent disconnects
// cannot leak the writer goroutine (PITFALLS #3), and `defer
// conn.CloseNow()` as a safety-net that reaps the TCP conn on any
// unexpected return path.
//
// Return values:
//   - nil — normal disconnect (ctx canceled, peer closed cleanly)
//   - wrapped error from conn.Write / conn.Ping — write or ping
//     failure (including the post-kick error after closeSlow or
//     closeGoingAway fires on the conn)
//
// Callers (Phase 4 HTTP handlers) should treat context.Canceled and
// context.DeadlineExceeded as non-errors for logging purposes.
func (h *Hub) Subscribe(ctx context.Context, conn *websocket.Conn) error {
	// CloseRead spawns an internal reader goroutine that handles
	// ping/pong/close control frames and cancels ctx when the peer
	// closes. Assigning the returned ctx back into the local ctx is
	// load-bearing — the writer loop's <-ctx.Done() branch observes
	// this cancellation.
	ctx = conn.CloseRead(ctx)

	s := &subscriber{
		msgs: make(chan []byte, h.opts.MessageBuffer),
		closeSlow: func() {
			conn.Close(websocket.StatusPolicyViolation, "subscriber too slow")
		},
		closeGoingAway: func() {
			conn.Close(websocket.StatusGoingAway, "server shutting down")
		},
	}
	h.addSubscriber(s)
	defer h.removeSubscriber(s)
	defer conn.CloseNow()

	// Per-subscriber ping ticker. The ping loop case lands fully in
	// Plan 03-02; the ticker is declared here so the select shape is
	// final and 03-02 only needs to fill the case body with real ping
	// logic. Using time.NewTicker (not time.After) per ARCHITECTURE.md
	// Pattern 2 and 03-CONTEXT.md "Claude's Discretion".
	tick := time.NewTicker(h.opts.PingInterval)
	defer tick.Stop()

	for {
		select {
		case msg := <-s.msgs:
			writeCtx, cancel := context.WithTimeout(ctx, h.opts.WriteTimeout)
			err := conn.Write(writeCtx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return nil
				}
				h.logger.Debug("wshub: subscriber writer loop exited", "error", err.Error())
				return err
			}
		case <-tick.C:
			// TODO(03-02): real ping via conn.Ping(pingCtx) with
			// h.opts.PingTimeout. Plan 03-01 keeps this a no-op so the
			// ticker is wired but silent; the 1000-cycle goroutine-leak
			// test does not depend on ping traffic.
		case <-ctx.Done():
			return nil
		}
	}
}
```

**Step 4: Run the RED tests and verify they now pass (GREEN).**

```
cd /home/ben/Dev-local/openburo-spec/open-buro-server
~/sdk/go1.26.2/bin/go test ./internal/wshub -race -run '^TestHub_New_PanicsOnNilLogger$|^TestHub_DefaultOptions$|^TestSubscribe_NoGoroutineLeak$' -timeout 30s -v
```

All three tests MUST pass. If the goroutine-leak test fails with `goroutines did not drain after 1000 disconnect cycles`, STOP and investigate — it means either `CloseRead` or `defer removeSubscriber` is broken. Do NOT increase the epsilon or the timeout to make the test pass; the +5 epsilon and 2-second poll are the documented contract from CONTEXT.md + RESEARCH.md.

Commit as: `feat(03-01): implement Hub, Options, Subscribe writer loop with CloseRead + defer removeSubscriber (GREEN)`.
  </action>
  <verify>
    <automated>cd /home/ben/Dev-local/openburo-spec/open-buro-server && ~/sdk/go1.26.2/bin/go test ./internal/wshub -race -run '^TestHub_New_PanicsOnNilLogger$|^TestHub_DefaultOptions$|^TestSubscribe_NoGoroutineLeak$' -timeout 30s && ~/sdk/go1.26.2/bin/go vet ./internal/wshub && ~/sdk/go1.26.2/bin/gofmt -l internal/wshub/hub.go internal/wshub/subscribe.go internal/wshub/doc.go</automated>
  </verify>
  <acceptance_criteria>
- `internal/wshub/hub.go` contains `type Hub struct`, `type Options struct`, `func New(logger *slog.Logger, opts Options) *Hub`, `defaultMessageBuffer = 16`, `defaultPingInterval  = 30 * time.Second`
- `internal/wshub/subscribe.go` contains `type subscriber struct`, `func (h *Hub) Subscribe(ctx context.Context, conn *websocket.Conn) error`
- `grep -c 'conn.CloseRead(ctx)' internal/wshub/subscribe.go` equals 1
- `grep -c 'defer h.removeSubscriber(s)' internal/wshub/subscribe.go` equals 1
- `grep -c 'defer conn.CloseNow()' internal/wshub/subscribe.go` equals 1
- `grep -c 'websocket.StatusPolicyViolation' internal/wshub/subscribe.go` equals 1
- `grep -c 'websocket.StatusGoingAway' internal/wshub/subscribe.go` equals 1
- `grep -c 'subscriber too slow' internal/wshub/subscribe.go` equals 1
- `grep -c 'server shutting down' internal/wshub/subscribe.go` equals 1
- `grep -c 'TODO(03-02)' internal/wshub/hub.go` equals 2 (Publish stub + Close stub)
- `! grep -E 'slog\.Default' internal/wshub/hub.go internal/wshub/subscribe.go internal/wshub/doc.go` (no slog.Default anywhere)
- `! grep -n 'Phase 1 ships this file only' internal/wshub/doc.go` (Phase 1 placeholder sentence removed)
- `~/sdk/go1.26.2/bin/go test ./internal/wshub -race -run '^TestHub_New_PanicsOnNilLogger$|^TestHub_DefaultOptions$|^TestSubscribe_NoGoroutineLeak$' -timeout 30s` exits 0
- `~/sdk/go1.26.2/bin/go vet ./internal/wshub` exits 0
- `~/sdk/go1.26.2/bin/gofmt -l internal/wshub/hub.go internal/wshub/subscribe.go internal/wshub/doc.go` produces no output
- `~/sdk/go1.26.2/bin/go list -deps ./internal/wshub | grep -E '^github\.com/openburo/openburo-server/internal/(registry|httpapi)$'` produces no output (byte-oriented contract)
  </acceptance_criteria>
  <done>
Three tests from Task 1 are GREEN under `-race`. `hub.go` + `subscribe.go` + `doc.go` compile, pass `go vet`, pass `gofmt`. The architectural gate (no imports of `registry` or `httpapi` from `wshub`) holds. `Publish` and `Close` are documented stubs awaiting Plan 03-02. The critical research flag (`CloseRead` + `defer removeSubscriber`) lands in the first commit of the phase.
  </done>
</task>

</tasks>

<verification>
Overall plan verification (run after both tasks land):

```
cd /home/ben/Dev-local/openburo-spec/open-buro-server

# 1. Full wshub test suite under -race (only the tests from Plan 03-01 exist yet).
~/sdk/go1.26.2/bin/go test ./internal/wshub -race -timeout 60s

# 2. Architectural gate: wshub does NOT import registry or httpapi.
~/sdk/go1.26.2/bin/go list -deps ./internal/wshub | grep -E '^github\.com/openburo/openburo-server/internal/(registry|httpapi)$' && { echo "GATE FAIL: wshub leaks into registry/httpapi" >&2; exit 1; } || true

# 3. Logging gate: no slog.Default in production wshub code.
grep -rE 'slog\.Default' internal/wshub/*.go | grep -v _test.go && { echo "GATE FAIL: slog.Default found in wshub production code" >&2; exit 1; } || true

# 4. No time.Sleep gate: PITFALLS #16 enforcement in wshub tests.
grep -n 'time\.Sleep' internal/wshub/*_test.go && { echo "GATE FAIL: time.Sleep found in wshub tests" >&2; exit 1; } || true

# 5. Full module build + vet + gofmt.
~/sdk/go1.26.2/bin/go build ./...
~/sdk/go1.26.2/bin/go vet ./...
~/sdk/go1.26.2/bin/gofmt -l internal/wshub/
```

All five checks MUST exit 0 (no output means gate passes for the `grep ... && exit 1` form).
</verification>

<success_criteria>
- `go.mod` contains `github.com/coder/websocket v1.8.14` as a direct dependency
- `internal/wshub/hub.go`, `internal/wshub/subscribe.go`, and updated `internal/wshub/doc.go` exist and compile under Go 1.26
- `TestHub_New_PanicsOnNilLogger`, `TestHub_DefaultOptions`, `TestSubscribe_NoGoroutineLeak` all pass under `-race` in under 30 seconds
- `conn.CloseRead(ctx)` + `defer h.removeSubscriber(s)` + `defer conn.CloseNow()` all appear exactly once in `subscribe.go` (the three research-flag lines)
- `Publish` and `Close` are documented stubs marked with `TODO(03-02)` comments
- `go list -deps ./internal/wshub` produces zero matches for `registry` or `httpapi`
- `go vet ./internal/wshub` clean; `gofmt -l internal/wshub/` empty; `go test ./... -race` clean across the whole module
- WS-02 (Hub + subscriber + buffer defaults) and WS-04 (CloseRead + defer removeSubscriber) are satisfied by observable artifacts
- The 1000-cycle goroutine-leak test preview runs here; the full acceptance version also runs in Plan 03-03 for defense in depth
</success_criteria>

<output>
After completion, create `.planning/phases/03-websocket-hub/03-01-SUMMARY.md` following the template at `/home/ben/.claude/get-shit-done/templates/summary.md`. Include: locked decisions honored, artifacts created, the exact test names now GREEN, and any deviations from CONTEXT/RESEARCH (there should be none).
</output>
