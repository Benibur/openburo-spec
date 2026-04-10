# Phase 3: WebSocket Hub - Research

**Researched:** 2026-04-10
**Domain:** `coder/websocket` canonical chat hub pattern, leak-free goroutine lifecycle, race-clean concurrent WebSocket tests against `httptest.NewServer`
**Confidence:** HIGH — every API claim below was verified against the v1.8.14 source in `~/go/pkg/mod/github.com/coder/websocket@v1.8.14/` and cross-checked against `pkg.go.dev`.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Hub constructor — `Options` struct with zero-value defaults.**
- `Options{MessageBuffer: 16, PingInterval: 30s, WriteTimeout: 5s, PingTimeout: 10s}` — zero value means use the documented package-level constants `defaultMessageBuffer`, `defaultPingInterval`, `defaultWriteTimeout`, `defaultPingTimeout`
- Signature: `func New(logger *slog.Logger, opts Options) *Hub`
- `New(nil, opts)` panics immediately with a test-author-friendly message; no `slog.Default()` fallback
- Plain struct over functional options: 4 knobs is not enough to justify the ceremony

**Close semantics — two paths, two status codes, two pre-bound callbacks per subscriber:**
- Slow-consumer kick: `websocket.StatusPolicyViolation`, reason `"subscriber too slow"`, logged `Warn`
- Hub shutdown: `websocket.StatusGoingAway`, reason `"server shutting down"`, logged `Info` (once, hub-level)
- `subscriber` struct holds `closeSlow func()` and `closeGoingAway func()` captured at `Subscribe` time
- `Hub.Close()` is idempotent, returns no error, sets internal `closed` flag so later `Publish` calls silently drop

**Logging — operator-focused, no PII:**
- `Warn` on slow-drop: `"wshub: subscriber dropped (slow consumer)"` with `buffer_size` only
- `Info` once at `Hub.Close`: `"wshub: closing hub"` with `subscribers` count
- `Debug` on writer-loop abnormal exit: `"wshub: subscriber writer loop exited"` with `error` string
- No per-publish logging; no subscriber identity fields (no `peer_addr`, no `user_agent`)
- Normal disconnect (ctx canceled, peer closed cleanly): silent

**Test surface — minimal public API, short-interval tests, no ticker injection:**
- Public API is only `New`, `Publish`, `Subscribe`, `Close`, plus `Options`, `Hub`
- No `Stats()`, no `NumSubscribers()`, no exported `PingTicker` field
- Tests live inside `package wshub` (not `wshub_test`) to touch unexported fields
- Short `PingInterval` (10–50ms) in tests + `require.Eventually` — NO `time.Sleep`, NO ticker injection
- Goroutine-leak test runs 1000 cycles against `httptest.NewServer`, asserts `runtime.NumGoroutine() <= baseline+5`

**Package structure — three files + doc.go, target <250 LoC production:**
- `doc.go` — keeps the package comment (extends existing Phase 1 placeholder)
- `hub.go` — `Hub`, `Options`, `New`, `Publish`, `Close`, `add/removeSubscriber`, package-level default constants
- `subscribe.go` — `subscriber` struct + `Subscribe` writer loop
- `hub_test.go` — Hub-level tests (slow-consumer, Close, Publish fan-out, log-capture)
- `subscribe_test.go` — Subscribe-level tests (goroutine-leak, ping, CloseRead)

**No recover() in writer loop.** Phase 4 middleware (API-08) catches any handler panic that bubbles up through `Subscribe`. Recovering inside the hub would hide real bugs where race detector + panics are the whole point.

**No `rate.Limiter` in v1.** The canonical chat example's `publishLimiter` is deliberately dropped — the admin-triggered workload never stresses it, and dropping `golang.org/x/time` off the dep graph keeps the module graph clean.

**No `ErrSlowSubscriber` sentinel.** The writer loop observes a post-kick `conn.Write` error which is indistinguishable from a flaky client; Phase 4 handlers don't need to distinguish the two cases for logging.

**Plan breakdown locked (aligns with ROADMAP.md):**
- `03-01`: Hub + subscriber + Subscribe with `CloseRead` + `defer removeSubscriber`
- `03-02`: Publish fan-out + drop-slow-consumer + ping loop + Close with `StatusGoingAway`
- `03-03`: goroutine-leak test + slow-consumer test + logging verification

### Claude's Discretion

- Exact test names — be consistent with Phase 2 style (`TestHub_*`, `TestSubscribe_*`)
- `require` vs `assert` — Phase 1 locked `require`; keep it
- Package doc location — `doc.go` already carries it; keep it there (consistency)
- Exact fields of `Warn`/`Info` log lines (add `op` if useful)
- Ping loop ticker mechanism — `time.NewTicker` (from ARCHITECTURE.md Pattern 2)
- `httptest.NewServer` vs `httptest.NewTLSServer` — plain is fine (origin check is Phase 4)

### Deferred Ideas (OUT OF SCOPE)

- Rate limiter on `Publish` → v2 operational hardening
- `Stats()` / `NumSubscribers()` → v2 (OPS-V2-02 Prometheus endpoint)
- Event coalescing / debounce → v2 FEAT-V2-01
- Per-subscriber subscription filter → out of scope per REQUIREMENTS.md
- Multiple event types (ADDED/UPDATED/REMOVED as separate events) → out of scope
- Ticker injection in test API → rejected in favor of short `PingInterval` values
- `ErrSlowSubscriber` sentinel error → rejected

</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| **WS-02** | Centralized hub pattern: `Hub` holds subscribers map under a mutex, `subscriber` has a buffered outbound channel (default 16) | The exact structure is locked in the [canonical chat example](https://github.com/coder/websocket/blob/v1.8.14/internal/examples/chat/chat.go) — subscribers `map[*subscriber]struct{}` under `sync.Mutex`, `msgs chan []byte` buffered to 16. See §Canonical Chat Server Reference below for the verbatim source. |
| **WS-03** | Non-blocking fan-out: publishing to a slow subscriber whose buffer is full triggers `closeSlow` | `Publish` uses `select { case s.msgs <- msg: default: go s.closeSlow() }`. `closeSlow` fires `c.Close(websocket.StatusPolicyViolation, "subscriber too slow")`. Goroutine dispatch is mandatory — calling `c.Close` inline under `h.mu.Lock()` would block the publisher for up to 10s (Close's internal 5s+5s handshake budget). |
| **WS-04** | Each subscriber calls `conn.CloseRead(ctx)` so control frames are handled and closed clients are detected | `CloseRead` spawns an internal goroutine that reads from the conn, processing ping/pong/close control frames and cancelling the returned context when the conn closes. The writer loop observes cancellation via `<-ctx.Done()` and exits via `defer h.removeSubscriber(s)`. |
| **WS-07** | Periodic ping frames keep connections alive (default 30s, configurable) | `PingInterval` from `Options` drives a per-subscriber `time.NewTicker` inside the writer loop. `c.Ping(ctx)` with a `PingTimeout`-bounded context. `Ping` requires a concurrent reader (which `CloseRead` provides) to receive the pong. |
| **WS-10** | Goroutine-leak integration test: 1000 connect/disconnect cycles leave `runtime.NumGoroutine()` flat | `httptest.NewServer` wraps a handler that calls `websocket.Accept` + `hub.Subscribe`. Loop dials 1000 times with `websocket.Dial(ctx, srv.URL, nil)`, closes each with `StatusNormalClosure`, then `require.Eventually` polls `runtime.GC() + runtime.NumGoroutine() <= baseline+5`. |

</phase_requirements>

## Summary

The Phase 3 hub is the **canonical `coder/websocket` chat server pattern**, minus the rate limiter, plus (1) configurable via an `Options` struct, (2) two distinct close paths (slow-kick vs hub-shutdown), (3) a per-subscriber ping loop, and (4) a goroutine-leak test as the load-bearing correctness guarantee. Every deviation from the reference pattern is locked in CONTEXT.md and the research below verifies each deviation against the v1.8.14 source.

The API surface used by Phase 3 is a seven-symbol subset of `github.com/coder/websocket`: `Accept`, `Dial` (tests only), `Conn.CloseRead`, `Conn.Write`, `Conn.Ping`, `Conn.Close`, `Conn.CloseNow` (defer safety net), plus three `StatusCode` constants and `MessageText`. No part of the phase touches compression, subprotocols, `OnPingReceived`, or origin patterns — those are Phase 4 concerns.

**Primary recommendation:** Mirror the `coder/websocket` canonical chat server (`internal/examples/chat/chat.go` at tag `v1.8.14`) for `Hub`, `subscriber`, `Publish`, `addSubscriber`, `deleteSubscriber`. Swap the chat example's `subscribe` method for a version that adds the locked `Options`-driven ping ticker, the `closeGoingAway` callback for hub shutdown, and the per-write context timeout from `Options.WriteTimeout`. The official `Example_writeOnly` in `example_test.go` is the write-only reference pattern the Subscribe loop structurally matches.

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/coder/websocket` | **v1.8.14** (published 2024-09-06, still the highest tag as of 2026-04-10) | WebSocket server + test client | Already locked in STACK.md as the canonical choice post the `gorilla/websocket` archive. Concurrent-write-safe on a single `*Conn`, native `context.Context` API, native `CloseRead` for write-only servers. |
| `github.com/stretchr/testify/require` | v1.11.1 | Test assertions including `require.Eventually` for eventual-consistency polling | Already locked in Phase 1. The goroutine-leak test uses `require.Eventually` — not `time.Sleep` — to poll `runtime.NumGoroutine()`. |
| `log/slog` | stdlib (Go 1.21+) | Structured logging injected into `New(logger, opts)` | Phase 1 lock: `slog.Default()` forbidden in `internal/`; injection-only. |
| `sync.Mutex` | stdlib | Hub subscribers map guard | Hub doesn't benefit from `RWMutex` — `Publish` iterates with the lock held, which is a write-equivalent access pattern. Plain `sync.Mutex`. Documented in CONTEXT.md "Existing Code Insights". |
| `time.NewTicker` | stdlib | Ping interval ticker inside the Subscribe writer loop | ARCHITECTURE.md Pattern 2 uses this; CONTEXT.md "Claude's Discretion" keeps it. |
| `context.WithTimeout` | stdlib | Per-write and per-ping context bounds | `Options.WriteTimeout` (default 5s) and `Options.PingTimeout` (default 10s) are applied via `context.WithTimeout(ctx, ...)` around each `c.Write` and `c.Ping` call. |
| `runtime.NumGoroutine` / `runtime.GC` | stdlib | Goroutine-leak test (WS-10) | Standard Go leak-detection idiom. `runtime.GC()` before reading `NumGoroutine` is required to reap finalizer-held conns. |
| `net/http/httptest.NewServer` | stdlib | Goroutine-leak test server | Wraps a minimal handler (`websocket.Accept` → `hub.Subscribe`) for the 1000-cycle test. |
| `strings` | stdlib | — | **NOTE: not needed for URL rewrite.** `websocket.Dial` accepts `http`/`https` URLs natively; see §Common Pitfalls #5. |

### Supporting

None. No rate limiter, no JSON codec (byte-oriented contract), no ID generator (no subscriber identity).

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `CloseRead` | Hand-written reader goroutine loop (`for { _, _, err := c.Read(ctx); if err != nil { return } }`) | More code; same behavior. `CloseRead` is the library-blessed idiom for write-only servers and is explicitly called out in `example_test.go:Example_writeOnly`. Reject. |
| `sync.Mutex` | `sync.RWMutex` | `Publish` holds the lock while iterating and calling `select` (a write-equivalent hot path). RLock buys nothing; adds cost. Reject. |
| Two close callbacks on `subscriber` | Single `closeWith(code StatusCode, reason string)` | Single closure needs a branching input and conditional binding, which moves the close-code decision away from the `Subscribe` boundary. Two pre-bound callbacks keep `Publish` branch-free. CONTEXT.md lock. |
| Exporting `Stats()` for tests | Internal tests in `package wshub` touching unexported fields | Exported stats leak product-visible API for test-only concerns. CONTEXT.md lock. |
| `time.NewTicker` | `time.After` in `select` | `time.After` allocates a new timer on every iteration (minor GC pressure under high-frequency test pings); `time.NewTicker` with `defer tick.Stop()` is the canonical Go idiom. Keep ticker. |

**Installation:**

```bash
go get github.com/coder/websocket@v1.8.14
go mod tidy
```

**Version verification:**

```
$ go list -m -versions github.com/coder/websocket
github.com/coder/websocket v0.1.0 ... v1.8.13 v1.8.14
```

v1.8.14 is the latest tagged release as of 2026-04-10 (verified via `GOPROXY=https://proxy.golang.org go list -m -versions`). Tag was cut 2024-09-06. Active mainline development continues on `master` (AVX2 masking work in Jan 2026, Go 1.25 compat in Dec 2025) but no newer tag has been cut. Pinning v1.8.14 is the correct choice — never depend on `master` for a reference impl.

## Architecture Patterns

### Recommended Project Structure

```
internal/wshub/
├── doc.go             # package comment (extended from Phase 1 placeholder)
├── hub.go             # Hub, Options, New, Publish, Close, add/removeSubscriber, defaults
├── subscribe.go       # subscriber struct + Subscribe writer loop
├── hub_test.go        # TestHub_* — slow-consumer, Close, Publish, logging
└── subscribe_test.go  # TestSubscribe_* — goroutine-leak, ping, CloseRead
```

**Target:** <250 LoC across `hub.go` + `subscribe.go` + `doc.go` combined. Test files can be larger.

### Pattern 1: The Canonical Chat Hub (Verified Against v1.8.14 Source)

**What:** The `coder/websocket` authors wrote `internal/examples/chat/chat.go` as the reference broadcast-hub pattern. Phase 3 is this pattern with four deltas:

1. Drop `publishLimiter` (no rate limit in v1)
2. Swap `log.Printf` for injected `*slog.Logger`
3. Add a per-subscriber ping ticker inside the writer loop
4. Add a second close callback (`closeGoingAway`) for `Hub.Close`

**When to use:** Every broadcast WebSocket server on `coder/websocket`. This is the library-authored reference — deviation requires strong justification.

**Canonical source (verbatim from [coder/websocket@v1.8.14/internal/examples/chat/chat.go](https://github.com/coder/websocket/blob/v1.8.14/internal/examples/chat/chat.go)):**

```go
// subscriber represents a subscriber.
// Messages are sent on the msgs channel and if the client
// cannot keep up with the messages, closeSlow is called.
type subscriber struct {
    msgs      chan []byte
    closeSlow func()
}

// subscribe subscribes the given WebSocket to all broadcast messages.
func (cs *chatServer) subscribe(w http.ResponseWriter, r *http.Request) error {
    var mu sync.Mutex
    var c *websocket.Conn
    var closed bool
    s := &subscriber{
        msgs: make(chan []byte, cs.subscriberMessageBuffer),
        closeSlow: func() {
            mu.Lock()
            defer mu.Unlock()
            closed = true
            if c != nil {
                c.Close(websocket.StatusPolicyViolation, "connection too slow to keep up with messages")
            }
        },
    }
    cs.addSubscriber(s)
    defer cs.deleteSubscriber(s)

    c2, err := websocket.Accept(w, r, nil)
    if err != nil {
        return err
    }
    mu.Lock()
    if closed {
        mu.Unlock()
        return net.ErrClosed
    }
    c = c2
    mu.Unlock()
    defer c.CloseNow()

    ctx := c.CloseRead(context.Background())

    for {
        select {
        case msg := <-s.msgs:
            err := writeTimeout(ctx, time.Second*5, c, msg)
            if err != nil {
                return err
            }
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}

// publish publishes the msg to all subscribers.
// It never blocks and so messages to slow subscribers are dropped.
func (cs *chatServer) publish(msg []byte) {
    cs.subscribersMu.Lock()
    defer cs.subscribersMu.Unlock()

    cs.publishLimiter.Wait(context.Background())  // DROPPED in Phase 3

    for s := range cs.subscribers {
        select {
        case s.msgs <- msg:
        default:
            go s.closeSlow()
        }
    }
}

func writeTimeout(ctx context.Context, timeout time.Duration, c *websocket.Conn, msg []byte) error {
    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()
    return c.Write(ctx, websocket.MessageText, msg)
}
```

**Key observations** (these are the load-bearing details the planner must preserve):

1. **`defer c.CloseNow()` is the safety-net cleanup.** `CloseNow` skips the close handshake — use it in `defer` so that even if the Subscribe path returns via an unexpected error, the underlying TCP conn is reaped. The actual graceful close happens via `closeSlow` / `closeGoingAway` which call `c.Close(code, reason)`.

2. **The `mu` / `closed` / `c` dance in the subscriber closure** guards a race where `Publish` fires `closeSlow` between `addSubscriber` and `Accept` completing. If `Accept` is slow (unlikely but possible under CI load) and a publish happens fast enough to overflow the buffer in that window, the closeSlow callback would dereference a nil `c`. The chat example solves this with an inner mutex + `closed` boolean. **Phase 3 faces the same race** — the Phase 4 HTTP handler calls `websocket.Accept` BEFORE `hub.Subscribe`, so the conn is already non-nil when `Subscribe` starts. This eliminates the race for Phase 3's API shape (`Subscribe(ctx, conn)`) but creates a different window: `addSubscriber` happens inside `Subscribe`, and `closeSlow` dereferences `conn` directly, so no nil check is needed. **Phase 3 does NOT need the inner mu + closed pattern** because `conn` is a parameter (always non-nil from the caller's perspective).

3. **`ctx := c.CloseRead(context.Background())`** in the chat example uses `context.Background()`. Phase 3 MUST use the `ctx` parameter passed to `Subscribe` — the HTTP handler's `r.Context()` — so that HTTP-level cancellation (shutdown, client disconnect) propagates into the writer loop. Adapted form: `ctx = conn.CloseRead(ctx)`.

4. **The chat example's writer loop has no ping case.** Phase 3 adds `case <-pingTick.C: c.Ping(pingCtx); ...`. This is fine because `CloseRead`'s internal reader goroutine is the "concurrent Reader" that `Ping` requires (see `conn.go:208` comment: "Ping must be called concurrently with Reader as it does not read from the connection").

### Pattern 2: Write-Only Subscribe Loop (from `example_test.go:82`)

The library ships an `Example_writeOnly` test example that is structurally the closest canonical match to Phase 3's `Subscribe` method. Verbatim from `example_test.go:82-118`:

```go
func Example_writeOnly() {
    fn := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        c, err := websocket.Accept(w, r, nil)
        if err != nil {
            log.Println(err)
            return
        }
        defer c.CloseNow()

        ctx, cancel := context.WithTimeout(r.Context(), time.Minute*10)
        defer cancel()

        ctx = c.CloseRead(ctx)

        t := time.NewTicker(time.Second * 30)
        defer t.Stop()

        for {
            select {
            case <-ctx.Done():
                c.Close(websocket.StatusNormalClosure, "")
                return
            case <-t.C:
                err = wsjson.Write(ctx, c, "hi")
                if err != nil {
                    log.Println(err)
                    return
                }
            }
        }
    })
    ...
}
```

**Phase 3's `Subscribe` is the write-only pattern + the chat `subscribers` map fan-out.** The planner can cite this example directly when reviewing the writer loop shape.

### Pattern 3: Two-Callback Subscriber (Phase 3 Delta)

**What:** The `subscriber` struct carries both `closeSlow` and `closeGoingAway`, each pre-bound to the `*websocket.Conn` at `Subscribe` time:

```go
// subscribe.go
type subscriber struct {
    msgs           chan []byte
    closeSlow      func()  // Publish path: StatusPolicyViolation
    closeGoingAway func()  // Hub.Close path: StatusGoingAway
}

func (h *Hub) Subscribe(ctx context.Context, conn *websocket.Conn) error {
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

    // Warn log for slow-drop lives in Publish (not closeSlow) because
    // closeSlow is called via `go` and the hub logger is already reachable
    // without plumbing it into the closure.

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
            pingCtx, cancel := context.WithTimeout(ctx, h.opts.PingTimeout)
            err := conn.Ping(pingCtx)
            cancel()
            if err != nil {
                h.logger.Debug("wshub: subscriber writer loop exited", "error", err.Error())
                return err
            }
        case <-ctx.Done():
            return nil  // normal disconnect, no log
        }
    }
}
```

**Why `MessageText` not `MessageBinary`:** The chat example uses `MessageText`. Phase 4's future payload is JSON (UTF-8), which is a text message by RFC 6455 §5.6. `MessageBinary` would work but `MessageText` is semantically correct and matches browser JS `socket.onmessage` handlers that expect a string (not an `ArrayBuffer`). Phase 3 is byte-oriented at the API boundary, but the wire frame type is locked to `MessageText`.

**Why `errors.Is(err, context.Canceled)` filter:** A clean ctx-cancel during a `Write` in-flight returns a wrapped `context.Canceled` error. CONTEXT.md locks "normal disconnect → silent log" — this filter keeps that promise without swallowing write failures caused by flaky clients.

### Anti-Patterns to Avoid

- **Calling `conn.Close` synchronously in `Publish`:** Would block the publisher for up to 10s (Close's 5s+5s handshake budget). Use `go s.closeSlow()` — the `go` keyword is load-bearing.
- **Holding `h.mu.Lock()` across a call to `conn.Write` or `conn.Close`:** Classic PITFALLS #1 (broadcasting under a lock). The chat example releases the lock only after the non-blocking select, and `closeSlow` fires in a new goroutine specifically so `Close` happens off-mutex.
- **Using `context.Background()` inside `Subscribe`:** The ctx parameter must flow into `CloseRead` so HTTP-layer cancellation reaches the writer loop.
- **Omitting `defer conn.CloseNow()`:** Without the safety-net, an early return from an error path leaks the underlying TCP conn and its finalizer-driven cleanup.
- **Writing to a subscriber's `msgs` channel without `select`-default:** Blocks the publisher on a single slow client. WS-03's entire point.
- **Logging the dropped message content:** PII risk in Phase 4 when messages contain app names; CONTEXT.md locks "no message content in logs."

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Control-frame handling for write-only WS servers | A reader goroutine loop calling `c.Read` and throwing away data | `conn.CloseRead(ctx)` | Library-blessed idiom. Spawns the reader goroutine, handles ping/pong/close for you, cancels ctx on connection drop. `c.Ping` depends on this. |
| WebSocket handshake request | `http.Hijack` + manual upgrade | `websocket.Accept(w, r, nil)` | 200+ lines of RFC 6455 state machine you do not want to own. |
| WebSocket handshake response (client side) | Raw TCP + handshake | `websocket.Dial(ctx, srv.URL, nil)` | Identical reasoning; the test client in WS-10 uses `Dial`. |
| Per-`Write` / per-`Ping` timeout enforcement | Setting `c.SetWriteDeadline` or similar | `context.WithTimeout(ctx, ...)` around each call | `coder/websocket` API is context-first; there is no SetDeadline. Context timeout is idiomatic. |
| Close code/reason extraction from errors | Manual error string parsing | `websocket.CloseStatus(err) StatusCode` | Library helper using `errors.As(err, &CloseError{})`. Returns `-1` if not a CloseError. |
| Subscriber identity tracking | Adding `clientID string` or UUID gen | Don't — hub is byte-oriented | Phase 4's HTTP handler layer is where identity lives (has `r.RemoteAddr`). Hub byte-contract forbids it. |
| Reader goroutine leak detection at test time | Manual goroutine stack inspection | `runtime.NumGoroutine()` baseline + `require.Eventually` polling | Stdlib + testify idiom; CONTEXT.md locks this exact pattern. |

**Key insight:** The hub is 8 API calls thick (`Accept`, `CloseRead`, `Write`, `Ping`, `Close`, `CloseNow`, `Dial` in tests, `CloseStatus` optionally in tests). Every attempt to do more inside the hub violates either the byte-oriented contract or the <250 LoC target. Resist.

## Common Pitfalls

Referenced from `.planning/research/PITFALLS.md`:

### Pitfall 1: Broadcasting under a mutex (slow-client stall) — PITFALLS.md Pitfall 1

**What goes wrong:** Writing directly to `conn.Write` while holding `h.mu.Lock()` inside `Publish` means one slow client stalls every subsequent mutation. Every HTTP handler piles up behind the slow client.

**Why it happens:** The "obvious" loop is `for client := range clients { client.conn.Write(ctx, ..., msg) }` — looks idiomatic, catastrophic in practice.

**How to avoid:** `select { case s.msgs <- msg: default: go s.closeSlow() }`. The `default` branch + the `go` keyword on closeSlow are both load-bearing. The buffered channel absorbs short bursts; the select-default prevents indefinite blocking; the goroutine dispatch prevents `Close` (which has a 5s+5s handshake budget) from blocking under the publisher's mutex.

**Warning signs:**
- `conn.Write` appearing anywhere inside `Publish`
- Missing `default:` branch on the channel send
- `closeSlow` called inline instead of via `go`

### Pitfall 2: Goroutine leak on silent disconnect — PITFALLS.md Pitfall 2

**What goes wrong:** Without `CloseRead`, control frames are never processed. When a client disconnects silently (network drop, tab closed), the writer goroutine is blocked on `<-s.msgs` and the hub never learns. After 10k cycles the process has 10k zombie goroutines and climbing memory.

**How to avoid:** `ctx = conn.CloseRead(ctx)` at the top of `Subscribe` + `defer h.removeSubscriber(s)` + the writer loop selecting on `<-ctx.Done()`. CloseRead's internal reader goroutine cancels ctx when the peer closes, the writer loop observes the cancel and returns, `defer` removes the subscriber.

**Warning signs:**
- No `CloseRead` call in a write-only WS server
- Missing `<-ctx.Done()` branch in the writer select
- `runtime.NumGoroutine()` climbing monotonically under a reconnect loop

**The 1000-cycle test (WS-10) IS the enforcement for this pitfall.** It is the load-bearing correctness test for Phase 3.

### Pitfall 3: ABBA lock ordering deadlock — PITFALLS.md Pitfall 3

**Not applicable to Phase 3** — but the structural prevention lands here. `internal/wshub` imports `internal/registry` zero times. Enforced by the CI gate `go list -deps ./internal/wshub | grep -E 'registry|httpapi'` (must be empty). Phase 4 wires the two packages via the HTTP handler layer with mutation-then-broadcast ordering.

### Pitfall 4: Shutdown does not close hijacked WS conns — PITFALLS.md Pitfall 6

**What goes wrong:** `http.Server.Shutdown()` explicitly excludes hijacked connections. WebSocket conns are hijacked. On SIGTERM, `Shutdown` returns cleanly, the process exits, clients see a TCP reset.

**How Phase 3 addresses it:** `Hub.Close()` iterates subscribers and fires `go s.closeGoingAway()` for each, which calls `conn.Close(StatusGoingAway, "server shutting down")`. Phase 5's `cmd/server/main.go` calls `hub.Close()` AFTER `httpSrv.Shutdown(ctx)` — this is OPS-04 and belongs to Phase 5. Phase 3 provides the API, Phase 5 wires it.

**Warning signs for Phase 3:**
- `Hub.Close()` that doesn't iterate subscribers
- `Close()` that returns an error (graceful shutdown must not report failure)
- `Close()` that isn't idempotent (second call must be a no-op)

### Pitfall 5: `httptest.NewServer` URL rewriting is unnecessary — NEW finding

**What goes wrong:** CONTEXT.md's example test snippet contains `"ws"+strings.TrimPrefix(srv.URL, "http")` to convert the httptest server URL from `http://127.0.0.1:N` to `ws://127.0.0.1:N`. This is a common idiom from the `gorilla/websocket` era.

**Why it's unnecessary in `coder/websocket`:** Verified in `dial.go:199` — `websocket.Dial` accepts `http` and `https` URL schemes as first-class alternatives to `ws`/`wss`:

```go
switch u.Scheme {
case "ws":
    u.Scheme = "http"
case "wss":
    u.Scheme = "https"
case "http", "https":   // <-- accepted directly
default:
    return nil, fmt.Errorf("unexpected url scheme: %q", u.Scheme)
}
```

**How to avoid:** The planner may keep the `strings.TrimPrefix` rewrite for explicit symmetry with WebSocket URL conventions (it harms nothing and signals "this is a WS URL") OR drop it and pass `srv.URL` directly. Both are correct. **CONTEXT.md's example snippet works as written** — this is a clarification, not a bug.

### Pitfall 6: Flaky time-based WebSocket tests — PITFALLS.md Pitfall 16

**What goes wrong:** `time.Sleep(100 * time.Millisecond)` + an assertion. Passes on a laptop, times out on a loaded CI runner, gets quarantined, never re-enabled.

**How Phase 3 avoids it:** No `time.Sleep` anywhere in hub tests. For ping-interval tests, use short `PingInterval` values (10-50ms) in `Options` and `require.Eventually` for polling assertions. The goroutine-leak test uses `require.Eventually(t, func() bool {...}, 2*time.Second, 20*time.Millisecond)` — 100 poll attempts over 2 seconds is plenty of room for CI timing skew.

**Warning signs:**
- Any `time.Sleep` in `*_test.go` under `internal/wshub/`
- Any hard-coded millisecond wait before an assertion without `require.Eventually` or a channel read

### Pitfall 7: `httptest.Server` port/goroutine leak — PITFALLS.md Pitfall 17

**What goes wrong:** Forgetting `defer srv.Close()` after `httptest.NewServer(...)`. Each leaked test server leaks an accept-loop goroutine.

**How to avoid:** Always `defer srv.Close()` immediately after `httptest.NewServer(...)`. The goroutine-leak test baselines `runtime.NumGoroutine()` AFTER starting the server but BEFORE the dial loop, so the server's own goroutines are absorbed into the baseline.

### Pitfall 8: `CloseRead` auto-closes on received data frames — NEW finding

**What goes wrong:** The library's `CloseRead` documentation says: *"If a data message is received, the connection will be closed with StatusPolicyViolation."* Verified in `read.go:76-84`:

```go
go func() {
    defer close(c.closeReadDone)
    defer cancel()
    defer c.close()
    _, _, err := c.Reader(ctx)
    if err == nil {
        c.Close(StatusPolicyViolation, "unexpected data message")
    }
}()
```

**Impact on Phase 3:** If a client sends any text or binary frame to the hub endpoint, the CloseRead reader goroutine calls `c.Close(StatusPolicyViolation, "unexpected data message")`. This is the same status code Phase 3 uses for the slow-consumer kick, but the **reason string differs**. The Warn log differentiates between:
- `"subscriber too slow"` — Phase 3's slow-drop
- `"unexpected data message"` — client protocol violation

**Why this matters:** A curious test that sends an unsolicited frame will observe a `StatusPolicyViolation` close and could mistake it for a slow-drop in an assertion. The Phase 3 tests should not send frames from the test client (the hub is broadcast-only), so this pitfall is theoretical — but the planner should document the close-code overlap so future maintainers don't add a test that trips the distinction.

**Mitigation:** The Warn-log assertion in Plan 03-03 matches on the log message text, not the close code, so the differentiation is automatic.

### Pitfall 9: Ping requires a concurrent reader — NEW finding

**What goes wrong:** `Conn.Ping(ctx)` does not itself read from the connection. It writes a PING frame and then blocks waiting for a corresponding PONG frame to arrive via the reader goroutine. Without a concurrent reader, `Ping` will block until ctx expires.

Verified in `conn.go:206-213`:

```go
// Ping sends a ping to the peer and waits for a pong.
// Use this to measure latency or ensure the peer is responsive.
// Ping must be called concurrently with Reader as it does
// not read from the connection but instead waits for a Reader call
// to read the pong.
```

**How Phase 3 satisfies this:** `CloseRead` spawns the internal reader goroutine that IS the "concurrent Reader" the Ping docs require. As long as `CloseRead(ctx)` is called before the writer loop starts, `Ping` works correctly.

**Warning signs:**
- A hypothetical refactor that drops `CloseRead` "because we're write-only" would break `Ping` silently (it would block until the 10s PingTimeout fires every time).
- The 10-50ms ping interval in tests would make this fail loudly and fast — another reason short ping intervals are in CONTEXT.md's locked defaults.

### Pitfall 10: 1000 rapid Dial+Close cycles on Linux — port exhaustion — NEW finding

**Concern:** 1000 rapid connect/disconnect cycles against an `httptest.NewServer` could theoretically exhaust the ephemeral port range. Each closed TCP conn enters `TIME_WAIT` for ~60 seconds.

**Why it's not actually a problem:**
- Linux's default ephemeral port range is 32768-60999 (~28k ports)
- `httptest.NewServer` binds on loopback (`127.0.0.1`), and loopback TIME_WAIT is typically handled quickly by the kernel
- 1000 sockets is 3.5% of the available range at worst
- The cycles in the test are sequential (one at a time), not concurrent, so at most one socket is actively opening at any moment

**No mitigation needed.** The test as written in CONTEXT.md runs cleanly on Linux CI. No need for `SO_REUSEADDR`, no need to limit cycle count, no need to introduce delays.

**Warning sign (forward-looking):** If a future maintainer parallelizes the 1000 cycles to 10 concurrent workers × 100 cycles each, port exhaustion becomes a theoretical concern at ~10k connections. Keep the test sequential.

## Code Examples

All snippets below are verified against `~/go/pkg/mod/github.com/coder/websocket@v1.8.14/`.

### Example 1: Minimal Subscribe writer loop (Phase 3 shape)

```go
// subscribe.go
package wshub

import (
    "context"
    "errors"
    "time"

    "github.com/coder/websocket"
)

func (h *Hub) Subscribe(ctx context.Context, conn *websocket.Conn) error {
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
            pingCtx, cancel := context.WithTimeout(ctx, h.opts.PingTimeout)
            err := conn.Ping(pingCtx)
            cancel()
            if err != nil {
                h.logger.Debug("wshub: subscriber writer loop exited", "error", err.Error())
                return err
            }
        case <-ctx.Done():
            return nil
        }
    }
}
```

### Example 2: Non-blocking Publish fan-out with Warn logging

```go
// hub.go
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
```

**Note:** Logging inside the lock is acceptable here — slog handlers are designed for fast sync writes, and the drop case is by definition exceptional. If drops become hot-path, the log could move off-mutex via a deferred slice append, but that's v2 concern.

### Example 3: Idempotent Close with StatusGoingAway

```go
// hub.go
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
    // Do NOT clear h.subscribers here — the writer loops observe their
    // conn close as a write error and call removeSubscriber themselves.
    // Clearing the map would race with defer h.removeSubscriber(s) in each
    // Subscribe goroutine.
}
```

**Key detail:** Do NOT set `h.subscribers = nil` or `h.subscribers = make(map[*subscriber]struct{})` in `Close()`. The chat example's `Close()` doesn't do this either. Each writer loop cleans itself up via `defer h.removeSubscriber(s)` when it observes the connection close.

### Example 4: Goroutine-leak test (Verified against library behavior)

```go
// subscribe_test.go
package wshub

import (
    "context"
    "net/http"
    "net/http/httptest"
    "runtime"
    "testing"
    "time"

    "github.com/coder/websocket"
    "github.com/stretchr/testify/require"
)

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

    // Allow the test server's accept goroutine to settle into the baseline.
    runtime.GC()
    baseline := runtime.NumGoroutine()

    for i := 0; i < 1000; i++ {
        ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
        conn, _, err := websocket.Dial(ctx, srv.URL, nil) // http:// scheme accepted directly
        require.NoError(t, err)
        _ = conn.Close(websocket.StatusNormalClosure, "")
        cancel()
    }

    // Poll until the writer loops observe their contexts canceling and exit.
    // runtime.GC() inside the closure is critical — it reaps finalizer-held conns.
    require.Eventually(t, func() bool {
        runtime.GC()
        return runtime.NumGoroutine() <= baseline+5
    }, 2*time.Second, 20*time.Millisecond)
}

func testLogger(t *testing.T) *slog.Logger {
    t.Helper()
    return slog.New(slog.NewTextHandler(io.Discard, nil))
}
```

**Epsilon = +5 rationale:** The writer loop observes `<-ctx.Done()` via `CloseRead`'s cancel propagation. There's unavoidable timing skew between the test's `conn.Close(StatusNormalClosure, "")` and the server-side goroutine exiting. `+5` accounts for in-flight goroutines mid-teardown during `NumGoroutine` read — verified in PITFALLS.md's recommended pattern.

**Why `runtime.GC()` inside the polling closure:** `coder/websocket` uses a `runtime.SetFinalizer` on `*Conn` (verified in `conn.go:138`). Conns that have gone out of scope but haven't been GC'd still hold goroutines (potentially). Forcing a GC cycle inside the poll guarantees finalizers run and the count stabilizes. A single `runtime.GC()` outside the poll is insufficient because it may run before all defer chains complete.

### Example 5: Slow-consumer kick test

```go
// hub_test.go
func TestHub_SlowConsumerDropped(t *testing.T) {
    hub := New(testLogger(t), Options{
        MessageBuffer: 1,
        PingInterval:  10 * time.Millisecond,
    })
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        conn, err := websocket.Accept(w, r, nil)
        if err != nil {
            return
        }
        _ = hub.Subscribe(r.Context(), conn)
    }))
    defer srv.Close()

    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    conn, _, err := websocket.Dial(ctx, srv.URL, nil)
    require.NoError(t, err)
    defer conn.CloseNow()

    // Do NOT read from conn — this is the "slow consumer" simulation.
    // The client simply never drains. TCP backpressure + MessageBuffer=1
    // means the hub's outbound channel fills on the second publish.

    // Wait until the hub has exactly one subscriber.
    require.Eventually(t, func() bool {
        hub.mu.Lock()
        defer hub.mu.Unlock()
        return len(hub.subscribers) == 1
    }, time.Second, 10*time.Millisecond)

    // Publish enough messages to overflow the 1-slot buffer.
    for i := 0; i < 5; i++ {
        hub.Publish([]byte("msg"))
    }

    // Slow subscriber should be kicked.
    require.Eventually(t, func() bool {
        hub.mu.Lock()
        defer hub.mu.Unlock()
        return len(hub.subscribers) == 0
    }, time.Second, 10*time.Millisecond)
}
```

**Note:** This test relies on the fact that `Subscribe` lives in `package wshub` and can touch `hub.mu` / `hub.subscribers` directly. This is the justification for internal tests.

### Example 6: Log-capture test for drop-log format (Plan 03-03)

```go
// hub_test.go
func TestHub_SlowConsumerLogsWarn(t *testing.T) {
    var buf bytes.Buffer
    logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

    hub := New(logger, Options{MessageBuffer: 1, PingInterval: 10 * time.Millisecond})
    // ... (same setup as slow-consumer test) ...

    // After the kick:
    require.Eventually(t, func() bool {
        return strings.Contains(buf.String(), "wshub: subscriber dropped (slow consumer)")
    }, time.Second, 10*time.Millisecond)

    logOutput := buf.String()
    require.Contains(t, logOutput, "level=WARN")
    require.Contains(t, logOutput, "buffer_size=1")
    require.NotContains(t, logOutput, "peer_addr", "no PII: peer address must not appear")
    require.NotContains(t, logOutput, "user_agent", "no PII: user agent must not appear")
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `gorilla/websocket` + per-conn mutex for writes | `coder/websocket` concurrent-write-safe Conn | 2024 (gorilla archived, nhooyr → coder/websocket) | No per-conn write mutex needed; no "hub goroutine" with register/unregister channels needed; fan-out happens directly in Publish on the caller's goroutine |
| `nhooyr.io/websocket` import path | `github.com/coder/websocket` import path | 2024 | STACK.md already locked the new path; Phase 3 imports `github.com/coder/websocket` |
| Manual reader loop `for { c.Read(ctx) }` discarding messages | `c.CloseRead(ctx)` | Was in nhooyr since ~2019, now canonical | Single line replaces a goroutine + loop + error handling |
| `time.Sleep` + `time.After` for test synchronization | `require.Eventually` + channel-based sync | testify 1.3+ | Deterministic polling without flakiness; CONTEXT.md locks this |
| Gorilla-style `hub.run()` goroutine reading from `register chan *Client` | Direct `Publish` iteration under `sync.Mutex` | `coder/websocket` era | Fewer goroutines, no channel serialization bottleneck, simpler code |

**Deprecated / outdated:**
- `gorilla/websocket`: archived. Do not use in new code.
- `nhooyr.io/websocket`: old import path, redirects but stale. Use `github.com/coder/websocket`.
- `golang.org/x/net/websocket`: effectively abandoned per STACK.md.
- Separate reader + writer goroutines per conn: `CloseRead` collapses this to writer-only.
- Hub-as-goroutine pattern: unnecessary given concurrent-write safety.

## Open Questions

None — every ambiguity is locked in CONTEXT.md by Claude's discretion. The planner has zero open decisions to make.

## Validation Architecture

**Nyquist validation enabled** (`workflow.nyquist_validation = true` in `.planning/config.json`).

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` + `github.com/stretchr/testify/require` v1.11.1 + `net/http/httptest` (stdlib) |
| Config file | none — Go `testing` is configured via `go test` flags; no `.go-test.yaml` or similar |
| Quick run command | `go test ./internal/wshub -race -run '^TestHub_\|^TestSubscribe_' -timeout 30s` |
| Full suite command | `go test ./internal/wshub -race -timeout 60s` |
| Dependency gate | `go list -deps ./internal/wshub \| grep -E 'registry\|httpapi'` → must be empty (structural isolation) |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|--------------|
| **WS-02** | `Hub` holds subscribers map, `subscriber` has buffered `msgs chan []byte` defaulting to 16 | unit | `go test ./internal/wshub -race -run '^TestHub_DefaultOptions$\|^TestHub_Publish_FanOut$' -timeout 10s` | ❌ Wave 0 — `hub_test.go` does not exist yet |
| **WS-03** | Non-blocking fan-out drops slow consumer via `closeSlow` instead of blocking publisher | unit (internal test, touches `hub.mu`) | `go test ./internal/wshub -race -run '^TestHub_SlowConsumerDropped$' -timeout 10s` | ❌ Wave 0 — `hub_test.go` does not exist yet |
| **WS-04** | `conn.CloseRead(ctx)` called in `Subscribe`; `defer removeSubscriber` on exit | integration (via goroutine-leak test — if CloseRead is missing, the leak test fails) | `go test ./internal/wshub -race -run '^TestSubscribe_NoGoroutineLeak$' -timeout 30s` | ❌ Wave 0 — `subscribe_test.go` does not exist yet |
| **WS-07** | Periodic ping frames keep connections alive at configured interval | integration | `go test ./internal/wshub -race -run '^TestSubscribe_PingKeepsAlive$' -timeout 10s` | ❌ Wave 0 — `subscribe_test.go` does not exist yet |
| **WS-10** | 1000 connect/disconnect cycles leave `runtime.NumGoroutine()` flat (±5) | integration (the correctness test) | `go test ./internal/wshub -race -run '^TestSubscribe_NoGoroutineLeak$' -timeout 30s -v` | ❌ Wave 0 — `subscribe_test.go` does not exist yet |
| **Architectural gate** | `wshub` imports neither `registry` nor `httpapi` | structural | `go list -deps ./internal/wshub \| grep -E 'registry\|httpapi' && exit 1 \|\| exit 0` | ✅ Existing gate pattern established in Phase 2 |
| **Logging gate** | No `slog.Default()` in `internal/wshub/*.go` production files | grep | `! grep -rE 'log/slog\|slog\.Default' internal/wshub/*.go \| grep -v _test.go` | ✅ Existing gate pattern |
| **No time.Sleep gate** | No `time.Sleep` in hub tests (PITFALLS.md #16) | grep | `! grep -n 'time\.Sleep' internal/wshub/*_test.go` | ❌ Wave 0 — new gate added for Phase 3 |

### Sampling Rate

- **Per task commit:** `go test ./internal/wshub -race -run '^TestHub_\|^TestSubscribe_' -timeout 30s` — runs every unit + integration test in the package with the race detector. This is ~5 seconds on a modern laptop including the 1000-cycle goroutine-leak test.
- **Per wave merge:** `go test ./internal/wshub -race -timeout 60s` — full suite including any benchmark-style tests. Runs the architectural `go list -deps` gate and the `time.Sleep` grep gate as part of the wave's quality-gate script.
- **Phase gate:** Full suite green + all gates (architectural, logging, no-time-Sleep) pass + race detector clean, before `/gsd:verify-work`.

### Wave 0 Gaps

Tests and infrastructure that must exist before implementation lands:

- [ ] `internal/wshub/hub_test.go` — covers WS-02 (default options), WS-03 (slow-consumer drop) + logging assertions for Warn-on-drop
- [ ] `internal/wshub/subscribe_test.go` — covers WS-04 (CloseRead presence via goroutine-leak test), WS-07 (ping keepalive with short PingInterval), WS-10 (1000-cycle goroutine-leak test)
- [ ] `internal/wshub/hub.go` — `Hub`, `Options`, `New`, `Publish`, `Close`, `addSubscriber`, `removeSubscriber`, package-level default constants
- [ ] `internal/wshub/subscribe.go` — `subscriber` struct, `Subscribe` method with writer loop
- [ ] `internal/wshub/doc.go` — extend existing Phase 1 placeholder (remove "Phase 1 ships this file only" sentence)
- [ ] Dependency add: `go get github.com/coder/websocket@v1.8.14` then `go mod tidy`
- [ ] CI gate script update: add `go list -deps ./internal/wshub | grep -E 'registry|httpapi'` to `scripts/phase-3-gates.sh` (or wherever Phase 2's equivalent lives)
- [ ] CI gate script update: add `! grep -n 'time\.Sleep' internal/wshub/*_test.go` to the same gate script

Framework install: not needed. Go `testing` is stdlib; testify is already pulled in by Phase 1.

**No existing infrastructure to mirror** except the Phase 2 pattern (`internal/registry/store_test.go`'s `TestStore_ConcurrentAccess`) which Phase 3's slow-consumer test uses as a style reference.

## Sources

### Primary (HIGH confidence)

- **`github.com/coder/websocket` v1.8.14 source** — verified locally at `/home/ben/go/pkg/mod/github.com/coder/websocket@v1.8.14/`
  - `accept.go:102` — `Accept(w, r, opts *AcceptOptions) (*Conn, error)` signature
  - `dial.go:120` — `Dial(ctx, u string, opts *DialOptions) (*Conn, *http.Response, error)` signature
  - `dial.go:194-202` — URL scheme handling (http/https accepted directly)
  - `read.go:51-86` — `CloseRead(ctx) context.Context` behavior, auto-close on data message
  - `conn.go:206-221` — `Ping(ctx) error`, "must be called concurrently with Reader"
  - `close.go:78-84` — `CloseStatus(err) StatusCode` helper
  - `close.go:86-128` — `Close(code, reason) error` with 5s+5s handshake budget, idempotent
  - `close.go:130-155` — `CloseNow() error` without handshake
  - `write.go:42` — `Write(ctx, typ MessageType, p []byte) error`
  - `example_test.go:82-118` — `Example_writeOnly` canonical pattern
- **[`internal/examples/chat/chat.go` @ v1.8.14](https://github.com/coder/websocket/blob/v1.8.14/internal/examples/chat/chat.go)** — fetched from GitHub, the canonical broadcast hub pattern
- **[pkg.go.dev/github.com/coder/websocket](https://pkg.go.dev/github.com/coder/websocket)** — official API docs
- `.planning/research/STACK.md` — library choice + Go 1.26 stack lock
- `.planning/research/ARCHITECTURE.md` §"Pattern 2: `coder/websocket` Canonical Chat Hub (Adapted)" (lines 205-327)
- `.planning/research/PITFALLS.md` — Pitfalls 1 (slow-client stall), 2 (goroutine leak), 3 (ABBA deadlock), 6 (shutdown), 16 (flaky time-based tests), 17 (httptest.Server leak)
- `.planning/phases/03-websocket-hub/03-CONTEXT.md` — every locked decision
- `internal/registry/store.go` + `internal/registry/store_test.go` — prior-phase style reference
- `internal/config/config.go` — error wrapping voice
- `go list -m -versions github.com/coder/websocket` — verified v1.8.14 is latest tag as of 2026-04-10

### Secondary (MEDIUM confidence)

- [Coder blog: "A New Home for nhooyr/websocket"](https://coder.com/blog/websocket) — nhooyr → coder transition
- PITFALLS.md Pitfall 16 example code — `hub.RegisterTestClient(t)` pattern (not used verbatim; Phase 3 uses internal tests instead)

### Tertiary (LOW confidence)

- None. No claims in this research rely on unverified WebSearch-only sources.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — every library version verified via `go list -m -versions` and/or local module source
- Architecture: HIGH — pattern verified verbatim against v1.8.14 canonical chat example and `Example_writeOnly` in `example_test.go`
- Pitfalls: HIGH — all behavioral claims (CloseRead auto-close on data, Ping requires concurrent reader, Dial accepts http scheme, Close 5s+5s budget) verified by reading the actual source files
- Test patterns: HIGH — the goroutine-leak test pattern is the library's own recommendation (PITFALLS.md #2) + stdlib `runtime.GC()` + `runtime.NumGoroutine()` idiom

**Research date:** 2026-04-10
**Valid until:** 2026-07-10 (90 days — `coder/websocket` is stable, v1.8 line has been active for 2+ years without breaking changes, no v2 roadmap published as of research date)

**Key deltas from CONTEXT.md** (items worth the planner's explicit attention):

1. **`defer conn.CloseNow()` safety net** — CONTEXT.md doesn't mention it but the canonical chat example uses it. Recommend including it.
2. **`websocket.Dial` accepts `http://` URLs directly** — CONTEXT.md's example test code has the `strings.TrimPrefix(srv.URL, "http")` rewrite. It works, but the rewrite is unnecessary. Planner may simplify or keep for WS-URL-convention clarity.
3. **`CloseRead` auto-closes with `StatusPolicyViolation` on received data** — overlaps with the Phase 3 slow-drop close code. Reason strings differ (`"unexpected data message"` vs `"subscriber too slow"`) so log assertions match on message text, not close code. Not a bug, but worth documenting.
4. **`Ping` requires concurrent Reader** — satisfied automatically by `CloseRead`. A future refactor that drops `CloseRead` would break `Ping` silently. The short-ping-interval tests in CONTEXT.md would make this fail fast, which is the correct defense.
5. **Do NOT clear `h.subscribers` in `Close()`** — the chat example doesn't, and clearing races with the writer loops' `defer removeSubscriber`. CONTEXT.md's example snippet doesn't explicitly say "do not clear"; the planner should make this explicit in the plan.
6. **`errors.Is(err, context.Canceled)` filter in the writer loop** — CONTEXT.md's "normal disconnect = silent" locks the behavior but doesn't specify the mechanism. Explicit filter keeps Debug-log noise minimal.
