# Phase 3: WebSocket Hub - Context

**Gathered:** 2026-04-10
**Status:** Ready for planning

<domain>
## Phase Boundary

Implement `internal/wshub`: a leak-free, byte-oriented pub/sub hub implementing the `coder/websocket` canonical chat pattern. The hub holds a mutex-protected map of subscribers, fans out `[]byte` payloads non-blockingly with drop-slow-consumer semantics, pings periodically, and prevents goroutine leaks via `conn.CloseRead(ctx)` + `defer removeSubscriber`. The package accepts `[]byte` only — it never imports `internal/registry` — making the registry↔hub ABBA deadlock (PITFALLS #1) structurally impossible from this side.

**In scope** (Phase 3 requirements: WS-02, WS-03, WS-04, WS-07, WS-10):
- Hub type + subscriber struct + `New(logger, opts)` constructor with defaults
- `Publish([]byte)` non-blocking fan-out with `select`-default → `go s.closeSlow()`
- `Subscribe(ctx, conn)` writer loop with `CloseRead`, periodic ping, drop-slow-consumer
- `Close()` two-phase helper: kicks all subscribers with `StatusGoingAway`
- Goroutine-leak integration test: 1000 connect/disconnect cycles against `httptest.NewServer`, `runtime.NumGoroutine()` flat ±epsilon
- Slow-consumer test: subscriber that never drains its channel is kicked without blocking publisher or other subscribers

**Out of scope** (deferred to Phase 4 HTTP API):
- HTTP upgrade handler (`WS-01`): Phase 4 handler calls `websocket.Accept` then hands conn to `hub.Subscribe`
- Mutation-then-broadcast wiring (`WS-05`, `WS-09`): Phase 4 handlers call `hub.Publish` *after* a successful `store.Upsert/Delete`
- Full-state `REGISTRY_UPDATED` snapshot on connect (`WS-06`): Phase 4 handler builds snapshot JSON and publishes before entering `Subscribe`
- `AcceptOptions.OriginPatterns` from shared CORS allow-list (`WS-08`): Phase 4 concern, lives on the `websocket.Accept` call
- Two-phase shutdown wiring (`OPS-04`): Phase 5 compose-root calls `hub.Close()` after `httpSrv.Shutdown`

**Package is independently testable** via `go test ./internal/wshub -race` — the goroutine-leak test uses `httptest.NewServer` with a tiny handler that calls `websocket.Accept` + `hub.Subscribe` inline (no dependency on `internal/httpapi`).

</domain>

<decisions>
## Implementation Decisions

All gray areas in Phase 3 are **builder-internal** (package API shape, logging verbosity, test surface, close semantics) — not product-visible. Per user preference ("décide au mieux pour ces questions"), Claude's discretion is used throughout. Each decision below is locked for the planner.

### Hub constructor signature — Options struct with zero-value defaults

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

// New constructs a Hub with the given logger and options. A nil logger
// panics at construction time — the hub must never fall back to
// slog.Default() (enforced by the "no slog.Default in internal/" gate).
func New(logger *slog.Logger, opts Options) *Hub
```

**Rationale:**
- Plain-struct `Options` is the most idiomatic Go for "mostly-defaults, occasionally-overridden" config with 4 knobs. Functional options are over-engineering at this size.
- Zero-value semantics mean callers can pass `wshub.Options{}` and get sane defaults. Phase 4 HTTP API passes config-driven values; tests pass tight timeouts.
- Separate `WriteTimeout` and `PingTimeout` because they're different operations with different SLAs: a data write can realistically take longer than a ping handshake.
- `New(logger, opts)` — not `New(opts)` with logger inside — because the logger is always required and should be visually prominent. Matches the pattern `httpapi.New(store, hub, creds, logger)` already planned for Phase 4.

**Defaults frozen as unexported package-level constants:**
```go
const (
    defaultMessageBuffer = 16
    defaultPingInterval  = 30 * time.Second
    defaultWriteTimeout  = 5 * time.Second
    defaultPingTimeout   = 10 * time.Second
)
```

### Close semantics — two paths, two status codes

Two distinct close flows share the hub:

1. **Slow-subscriber kick** (triggered by full outbound buffer inside `Publish`):
   - Close code: `websocket.StatusPolicyViolation`
   - Close reason: `"subscriber too slow"`
   - Logged at `Warn` — this is operationally interesting (hints at undersized buffer or flaky client)

2. **Hub shutdown** (triggered by `Hub.Close()` from the Phase 5 compose-root):
   - Close code: `websocket.StatusGoingAway`
   - Close reason: `"server shutting down"`
   - Logged at `Info` (once, hub-level — not per-subscriber)

**Implementation:** The `subscriber` struct holds **two** close callbacks captured at `Subscribe` time, each pre-bound to the `*websocket.Conn`:

```go
type subscriber struct {
    msgs         chan []byte
    closeSlow    func() // Publish path: StatusPolicyViolation
    closeGoingAway func() // Hub.Close path: StatusGoingAway
}
```

`Publish` fires `go s.closeSlow()` on a full buffer. `Hub.Close` iterates subscribers and fires `go s.closeGoingAway()` for each. The writer loop in `Subscribe` observes the resulting conn close as a write error on its next iteration and exits cleanly.

**Rationale:** WS-03 mandates slow-consumer drop; OPS-04 mandates `StatusGoingAway` on shutdown. The distinction cannot be made inside `Close()` alone because it doesn't own the conn — the conn lives inside the `subscriber`. Capturing two pre-bound callbacks at `Subscribe` time keeps the close-code decision at the boundary where the conn is known, with zero branching in `Publish`.

**Hub.Close() is idempotent** (second call is a no-op) and sets an internal `closed` flag so subsequent `Publish` calls silently drop. The method returns no error — graceful shutdown must not report failure.

### Drop-subscriber logging — operator-focused, no PII

**Slow-consumer drop** (`closeSlow` called from `Publish`):
```go
h.logger.Warn("wshub: subscriber dropped (slow consumer)",
    "buffer_size", h.msgBuffer)
```

**Hub close** (once, at the top of `Hub.Close`):
```go
h.logger.Info("wshub: closing hub",
    "subscribers", len(h.subscribers))
```

**Writer loop normal exit** (context canceled, peer closed): no log line. This is the common path and logging it creates noise.

**Writer loop abnormal exit** (write error, ping error): logged at `Debug` so the operator can opt in to see flaky-client drops:
```go
h.logger.Debug("wshub: subscriber writer loop exited",
    "error", err.Error())
```

**No per-publish logging** — fan-out must be silent under the hot path.

**No subscriber identity fields** (`subscriber_id`, `peer_addr`, `user_agent`) in the hub's logs. The hub is byte-oriented and has no notion of client identity; that's the Phase 4 HTTP handler's concern. Logging peer addr from inside the hub would require adding identity plumbing that the byte-oriented contract explicitly forbids.

**Rationale:** Drops are operationally interesting (hint at undersized buffer or flaky client); clean close is info; fan-out is silent. No PII preserves the "credentials never appear in logs" invariant from AUTH-05.

### Test surface — minimal public API, package-internal test helpers

**Public API stays minimal** — only `New`, `Publish`, `Subscribe`, `Close`, plus the `Options`, `Hub`, and error sentinels. No `Stats()`, no `NumSubscribers()`, no exported `PingTicker` field.

**Tests that need to observe hub state** are defined in `hub_test.go` or `subscribe_test.go` inside `package wshub` (NOT `package wshub_test`). This makes them part of the same package, so they can access unexported fields directly:

```go
// hub_test.go — internal tests
package wshub

func TestHub_SlowConsumerDropped(t *testing.T) {
    h := New(testLogger(t), Options{MessageBuffer: 1, PingInterval: 10 * time.Millisecond})
    // ...
    h.mu.Lock()
    n := len(h.subscribers)
    h.mu.Unlock()
    require.Equal(t, 0, n, "slow subscriber should have been removed")
}
```

**Ticker injection is NOT used.** Instead, tests pass a short `PingInterval` (e.g. `10 * time.Millisecond`) in `Options` and assert the expected behavior within a reasonable timeout. This is deterministic enough for the 3 tests that care about ping behavior and avoids adding a `PingTicker <-chan time.Time` option that bloats the public API.

**The goroutine-leak test** uses this pattern:
```go
// TestHub_NoGoroutineLeak runs 1000 connect+disconnect cycles against an
// httptest.NewServer wrapping the hub and asserts runtime.NumGoroutine()
// is flat (±epsilon) afterwards.
func TestHub_NoGoroutineLeak(t *testing.T) {
    hub := New(testLogger(t), Options{PingInterval: 50 * time.Millisecond})
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        conn, err := websocket.Accept(w, r, nil)
        if err != nil {
            return
        }
        _ = hub.Subscribe(r.Context(), conn)
    }))
    defer srv.Close()

    runtime.GC()
    baseline := runtime.NumGoroutine()

    for i := 0; i < 1000; i++ {
        ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
        conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
        require.NoError(t, err)
        conn.Close(websocket.StatusNormalClosure, "")
        cancel()
    }

    // Allow the writer loops to observe their contexts canceling and exit.
    require.Eventually(t, func() bool {
        runtime.GC()
        return runtime.NumGoroutine() <= baseline+5
    }, 2*time.Second, 20*time.Millisecond)
}
```

**Epsilon = +5 goroutines** accounts for timing skew between the writer loop's `ctx.Done()` observation and the `NumGoroutine` read. `require.Eventually` polls until the condition holds or a 2-second budget expires. This is not a `time.Sleep` — it's the idiomatic testify way to assert eventual consistency.

**Rationale:** Minimal public API is the reference-impl principle from ARCHITECTURE.md ("`wshub` is deliberately thin — ~3 files, <250 LoC total. Resist over-engineering the hub"). Internal test files keep the test-only surface out of the public API. Short ping intervals are simpler than ticker injection and work for all Phase 3 tests.

### Rate limiter — NOT included in v1

ARCHITECTURE.md showed an optional `publishLimiter *rate.Limiter` field on the Hub. That field is **not included** in Phase 3. Rationale:
- The expected workload (admin-triggered upserts from humans) never generates rate-limiter-relevant traffic.
- `rate.Limiter` adds a golang.org/x/time dependency for zero v1 benefit.
- Deferred to v2 with other operational hardening (see deferred list).

### Subscribe error semantics

```go
// Subscribe registers a new WebSocket subscriber on the hub and blocks
// until the client disconnects or ctx is canceled. It is the caller's
// responsibility to call websocket.Accept before handing the conn in,
// and to close the conn if Subscribe returns a non-context error.
//
// Return values:
//   - nil — normal disconnect (ctx canceled, peer closed cleanly)
//   - context.Canceled or context.DeadlineExceeded — ctx ended
//   - wrapped error from conn.Write / conn.Ping — write or ping failure
//     (including the post-kick error after closeSlow or closeGoingAway)
//
// Callers (Phase 4 HTTP handlers) should treat context.Canceled and
// context.DeadlineExceeded as non-errors for logging purposes and log
// other errors at Warn level.
func (h *Hub) Subscribe(ctx context.Context, conn *websocket.Conn) error
```

No custom `ErrSlowSubscriber` sentinel — when a slow subscriber is kicked, the writer loop's next `conn.Write` fails with an arbitrary wrapped error (the conn is already closed). Downstream handlers don't need to distinguish "kicked" from "flaky client" — both are equally "the client is gone, stop the loop, log at Warn if not a ctx cancel."

### Package structure — three files + doc.go

```
internal/wshub/
├── doc.go           // package comment (already exists from Phase 1)
├── hub.go           // Hub, Options, New, Publish, Close, add/removeSubscriber
├── subscribe.go     // subscriber struct + Subscribe writer loop
├── hub_test.go      // Hub-level tests (slow-consumer, close, publish fan-out)
└── subscribe_test.go // Subscribe-level tests (goroutine-leak, ping, CloseRead)
```

**`doc.go` is extended, not replaced.** The existing Phase 1 placeholder text ("Phase 1 ships this file only; the real implementation lands in Phase 3.") is removed; the package comment is upgraded to the canonical description.

**Test files stay in `package wshub`** (internal tests) so they can touch unexported fields for hub-state assertions.

### Panic safety — no recover inside the hub

The writer loop in `Subscribe` does NOT install a `defer recover()`. Rationale:
- `coder/websocket` is well-tested and does not panic under normal use.
- Phase 4's HTTP handler middleware chain has a recover middleware (API-08) that catches any handler-level panic, including anything that bubbles up from `Subscribe`.
- Adding a recover here would hide real bugs in the hub itself, which is exactly where we want the race detector and panics to fire loudly during testing.

### Logger requirement at construction

`New(nil, opts)` panics immediately with a clear message:
```go
if logger == nil {
    panic("wshub.New: logger is required; use slog.New(slog.NewTextHandler(io.Discard, nil)) in tests")
}
```

**Rationale:** No `slog.Default()` fallback (the "no `slog.Default()` in `internal/`" gate from Phase 1). Better to crash at construction than to silently swallow log lines if a caller forgets to inject. The panic message tells test authors exactly how to construct a no-op logger.

### Plan breakdown — aligns with ROADMAP.md

ROADMAP.md pre-sketched three plans. The planner should honor this breakdown:

- **03-01: Hub type + subscriber struct + Subscribe with CloseRead** — lands the `Hub`, `Options`, `subscriber`, `New`, `addSubscriber`, `removeSubscriber`, and `Subscribe` writer loop. First commit honors critical research flags (`CloseRead`, `defer removeSubscriber`). Slim `Publish` and `Close` stubs.

- **03-02: Publish fan-out + drop-slow-consumer + ping loop** — completes `Publish` with non-blocking `select`-default, wires the two close callbacks into the subscriber, implements `Close` with `StatusGoingAway`, and lands the per-subscriber ping ticker in the writer loop.

- **03-03: goroutine-leak test + slow-consumer test + logging verification** — the two correctness-critical tests plus a smaller unit test that captures `slog` output via `slog.NewTextHandler(buf, ...)` to assert the drop log line appears at `Warn` level and the close log line appears at `Info` level with no PII.

Each plan is executable independently against the Phase 2 output (which is not a dependency — the disjoint dependency graph is the whole point of parallel execution).

### Claude's Discretion (for the planner)

- Exact test names (just be consistent with Phase 2 style: `TestHub_*`, `TestSubscribe_*`)
- Whether to use `require` or `assert` in tests — Phase 1 locked `require`; keep it
- Whether `hub.go` opens with package doc or `doc.go` keeps it — keep package doc in `doc.go` for consistency with Phase 2's `internal/registry` which put the package doc in `manifest.go`; OK to move to `hub.go` if cleaner
- Exact fields of the `Warn`/`Info` log lines (add `op` if useful)
- Whether to use `time.After` or `time.NewTicker` in the ping loop — ARCHITECTURE.md uses `time.NewTicker`; keep it
- `httptest.NewServer` vs `httptest.NewTLSServer` for the goroutine-leak test — plain is fine (origin check is Phase 4)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Contracts and constraints
- `.planning/PROJECT.md` — core value, validated reqs, active reqs (WebSocket items under Real-time notifications)
- `.planning/REQUIREMENTS.md` §WebSocket (WS-02, WS-03, WS-04, WS-07, WS-10) — the 5 Phase 3 REQ-IDs
- `.planning/ROADMAP.md` §"Phase 3: WebSocket Hub" — goal + 5 success criteria + 3 pre-sketched plans
- `.planning/phases/01-foundation/01-CONTEXT.md` — prior locked decisions (Go 1.26, testify/require, table-driven+testdata, no `slog.Default()` in internal/)
- `.planning/phases/02-registry-core/02-CONTEXT.md` — prior locked style (wrapped errors, package doc placement, testdata convention)

### Research (critical for this phase)
- `.planning/research/STACK.md` — `coder/websocket` chosen as the canonical WebSocket library; Go 1.26 stdlib dependencies only
- `.planning/research/ARCHITECTURE.md` §"Pattern 2: `coder/websocket` Canonical Chat Hub (Adapted)" — the full Hub + subscriber + Subscribe skeleton this phase implements (lines 205-327)
- `.planning/research/ARCHITECTURE.md` §"wshub is deliberately thin" — reference-impl principle: ~3 files, <250 LoC total
- `.planning/research/PITFALLS.md` §1 (ABBA deadlock: wshub NEVER imports registry), §3 (goroutine leak: `CloseRead` + `defer removeSubscriber`), §4 (slow-client stall: non-blocking `select`-default fan-out), §6 (graceful shutdown: `http.Server.Shutdown` does NOT close hijacked conns, so `hub.Close()` with `StatusGoingAway` is Phase 5's job), §16 (flaky time-based WebSocket tests: no `time.Sleep` in hub tests)
- `.planning/research/FEATURES.md` — "the broker has one thing to say to everyone" — single event type, no subscribe/unsubscribe protocol, no per-event diffs
- `.planning/research/SUMMARY.md` §"Phase 2b / 3: WebSocket Hub" — deliverables list

### Library documentation
- `coder/websocket` — https://pkg.go.dev/github.com/coder/websocket — canonical chat example is the reference pattern; `CloseRead`, `Accept`, `Conn.Write`, `Conn.Ping`, `StatusPolicyViolation`, `StatusGoingAway` are the API surface used

### Prior phase artifacts to mirror
- `internal/registry/store.go` — the `sync.RWMutex`-guarded state + defer Unlock pattern, the snapshot-copy convention on reads
- `internal/registry/store_test.go` — table-driven + `require.NoError` + `require.Equal` style; `TestStore_ConcurrentAccess` pattern for race-clean concurrent tests
- `internal/config/config.go` — the `%q`-quoting and wrapped-error style for user-facing errors (keep hub errors in the same voice)

### Out-of-scope references (Phase 4, listed here so the planner does NOT pull them into Phase 3)
- `WS-01, WS-05, WS-06, WS-08, WS-09` — Phase 4 (HTTP upgrade, broadcast-on-mutation, full-state snapshot on connect, `OriginPatterns`, mutation-then-broadcast ordering)
- `OPS-04` — Phase 5 (two-phase shutdown wiring; Phase 3 only provides the `Hub.Close()` API)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`internal/wshub/doc.go`** — package doc placeholder from Phase 1. This phase extends it with the full description and removes the "Phase 1 ships this file only" sentence. Keep the package-doc-in-`doc.go` convention (Phase 2 put package doc in `manifest.go`; either works, but consistency within a package matters — `doc.go` exists so keep it).
- **`internal/registry/store.go`** — the `sync.RWMutex` + `defer s.mu.Unlock()` pattern. Phase 3 uses `sync.Mutex` (hub doesn't benefit from RWMutex because `Publish` iterates with the lock held and that's a write-equivalent access pattern). Same defer discipline.
- **`internal/config/config.go`** — the `fmt.Errorf("context: %w", err)` wrapping style with lowercase context strings. All hub errors follow this voice.
- **`internal/registry/store_test.go` `TestStore_ConcurrentAccess`** — the "N goroutines hammering the same API under `-race`" test pattern. Phase 3's slow-consumer test uses the same pattern: one goroutine publishes, N subscribers drain at different rates.

### Established Patterns
- **Logger injection only** — every constructor takes `*slog.Logger` as an explicit parameter. No `slog.Default()` anywhere in `internal/`. Gate: `grep -rE 'log/slog|slog\.Default' internal/wshub/*.go | grep -v _test.go` must be empty after Phase 3.
- **Package doc in one file** — either `doc.go` or the first implementation file carries the `// Package wshub ...` comment. For Phase 3, `doc.go` keeps it because the placeholder already exists.
- **Testdata vs inline tables** — `internal/registry/mime_test.go` uses inline Go literals for the 3×3 matrix (no fixture file). Phase 3 has no fixtures at all — all tests are pure Go state.
- **Table-driven tests via `require`** — `github.com/stretchr/testify/require` was locked in Phase 1. Continue using `require.NoError`, `require.Equal`, `require.Eventually` (for the goroutine-leak test's eventual-consistency assertion).
- **Architectural isolation via `go list -deps`** — Phase 2 enforced `go list -deps ./internal/registry | grep -E 'wshub|httpapi'` as a gate. Phase 3 adds the symmetric gate: `go list -deps ./internal/wshub | grep -E 'registry|httpapi'` must be empty. Both gates belong in the CI workflow but for Phase 3 they run as part of the phase's quality-gate script.

### Integration Points
- **Phase 4 (HTTP API)** will import `internal/wshub` and call `wshub.New(logger, opts)` from `httpapi.New(...)`, then in the WS upgrade handler it calls `websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: ...})` followed by `hub.Subscribe(r.Context(), conn)`. The mutation-then-broadcast flow (`WS-05`, `WS-09`) calls `hub.Publish(eventBytes)` *after* a successful `store.Upsert` or `store.Delete`. Phase 3 must ensure the `New`, `Publish`, `Subscribe`, `Close` signatures are stable enough that Phase 4 can import and use them without modification.
- **Phase 5 (compose-root)** will call `wshub.New(logger, wshub.Options{PingInterval: cfg.WebSocket.PingInterval * time.Second})` from `cmd/server/main.go` and `defer hub.Close()` after `httpSrv.Shutdown(ctx)` (two-phase shutdown). Phase 3's `Options.PingInterval` must accept the config-driven value directly without conversion ceremony.
- **`internal/registry` (Phase 2, shipped)** is NOT a dependency of Phase 3. The disjoint dependency graph is the whole point of parallel execution. The planner MUST NOT import `registry` from `wshub` under any justification. CI gate: `go list -deps ./internal/wshub | grep -E 'registry|httpapi'` → empty.

</code_context>

<specifics>
## Specific Ideas

- **The goroutine-leak test is THE correctness test for Phase 3.** Just as the 3×3 MIME matrix was Phase 2's load-bearing test, the 1000-cycle `NumGoroutine` ±epsilon assertion is Phase 3's. This is the test that catches the `CloseRead` + `defer removeSubscriber` contract silently breaking if someone refactors the writer loop. It should be the last test the planner touches and the first test a reviewer runs under `-race`.
- **`wshub` is deliberately thin — resist over-engineering.** ARCHITECTURE.md calls out "the temptation on a reference project is to over-engineer the hub; resist it." Target: 3 files, <250 LoC total in the production files. The tests can be larger.
- **Byte-oriented contract is load-bearing.** The hub accepts `[]byte` only. It has zero knowledge of JSON, zero knowledge of `REGISTRY_UPDATED`, zero knowledge of the `{event, timestamp, payload}` shape. Phase 4's HTTP handler is where those concerns live. Anyone reviewing Phase 3 should be able to read every file in `internal/wshub/` without ever seeing the word "registry" or "manifest."
- **No subscriber identity in logs.** The hub is byte-oriented — it doesn't know usernames, peer addresses, or user agents. Logging them would require adding plumbing that violates the byte-oriented contract. Phase 4's HTTP handler layer is where subscriber identity lives, because that's where `http.Request` is in scope.
- **No `time.Sleep` anywhere in hub tests.** Use short ping intervals (10-50ms) via `Options.PingInterval` and `require.Eventually` for polling assertions. PITFALLS #16 is clear about why flaky time-based tests poison CI confidence.

</specifics>

<deferred>
## Deferred Ideas

- **Rate limiter on Publish** — ARCHITECTURE.md mentioned an optional `publishLimiter *rate.Limiter`; defer to v2 when operational hardening matters. v1's expected workload (admin-triggered mutations) never stresses a rate limiter.
- **Hub metrics / `Stats()` method** — `NumSubscribers()`, publish counters, drop counters — defer to v2 (tracked as OPS-V2-02 Prometheus endpoint). Tests that need subscriber count touch unexported fields from within the same package.
- **Event coalescing / debounce** — already tracked as v2 `FEAT-V2-01` in REQUIREMENTS.md. Out of scope for Phase 3; the hub fans out every `Publish` call individually.
- **Per-subscriber subscription filter** ("only notify me about apps where action=PICK") — explicitly out of scope per REQUIREMENTS.md "Out of Scope" ("Subscribe/unsubscribe WS protocol — defeats 'broker has one thing to say to everyone'").
- **Multiple event types** (ADDED / UPDATED / REMOVED as separate events) — explicitly out of scope per REQUIREMENTS.md; single `REGISTRY_UPDATED` event type, client does `state = event.capabilities`.
- **Ticker injection in test API** — considered and rejected in favor of short `PingInterval` values. Revisit only if a test emerges that genuinely needs to drive the ticker manually and short intervals prove insufficient.
- **`ErrSlowSubscriber` sentinel error** — considered and rejected. The writer loop observes a post-kick `conn.Write` error, which is indistinguishable from a flaky client from the loop's perspective, and Phase 4 handlers don't need to distinguish the two cases for logging purposes.

</deferred>

---

*Phase: 03-websocket-hub*
*Context gathered: 2026-04-10*
