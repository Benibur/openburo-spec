# Architecture Research

**Project:** OpenBuro Server (Go HTTP REST + WebSocket app registry, reference implementation)
**Domain:** Single-binary Go server with four domains: config, registry (in-memory + JSON file), HTTP API (REST + Basic Auth), WebSocket hub (broadcast)
**Researched:** 2026-04-09
**Confidence:** HIGH (stack is fixed by STACK.md; patterns verified against `coder/websocket` canonical example and Go stdlib idioms)

## TL;DR — The Prescriptive Architecture

- **Four `internal/` packages** matching the four domains — `config`, `registry`, `httpapi`, `wshub`.
- **Strict one-way dependency graph**: `cmd/server` → (`httpapi`, `wshub`, `config`, `registry`); `httpapi` → (`registry`, `wshub`); `wshub` → (`registry` types only, via a tiny `Event` struct); `registry` → **nothing** (pure core).
- **Mutation → broadcast** happens in the HTTP handler layer — *not* inside the registry. Handler calls `registry.Upsert(...)` then `hub.Publish(Event{...})`. Registry stays pure.
- **Persistence is write-through inside the registry**: every successful mutation writes the whole `registry.json` atomically (temp file + rename) while the mutex is held. Simple, correct, fast enough for a reference impl.
- **WebSocket hub = `coder/websocket` canonical chat pattern**: `Hub` holds `map[*subscriber]struct{}` under a mutex; each subscriber has a buffered `msgs chan []byte` and a `closeSlow` callback; `Publish` is non-blocking with drop-slow-consumer semantics. *One* goroutine per connection (the writer loop); no separate "hub goroutine" with register/unregister channels — that's the gorilla pattern and isn't needed with `coder/websocket`'s concurrent-write safety.
- **HTTP handlers live on a `*Server` struct** (`httpapi.Server`) holding dependencies as fields. Methods like `(s *Server) handleRegistryUpsert(w, r)` are returned as `http.HandlerFunc` values during route registration. This is the "handler as method" idiom, testable with `httptest`.
- **Graceful shutdown is two-phase** because `http.Server.Shutdown` does *not* close hijacked (WebSocket) connections: `httpSrv.Shutdown(ctx)` first to stop accepting new work, then `hub.Close()` to close all subscriber channels and let writer goroutines exit.

## Standard Architecture

### System Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                          cmd/server/main.go                         │
│   (wiring only — parse flags, load config, construct, run, shutdown)│
└──────┬─────────────┬─────────────────┬─────────────────┬────────────┘
       │             │                 │                 │
       ▼             ▼                 ▼                 ▼
 ┌──────────┐  ┌──────────┐   ┌──────────────────┐  ┌──────────┐
 │ config   │  │ registry │   │     httpapi      │  │  wshub   │
 │          │  │          │   │                  │  │          │
 │ Load()   │  │ Store    │◄──┤ Server           │──►│ Hub      │
 │ Config{} │  │  Upsert  │   │  router          │  │  Publish │
 │ Creds{}  │  │  Delete  │   │  middleware      │  │  Close   │
 │          │  │  List    │   │  handlers        │  │          │
 │          │  │  Get     │   │   ├─ registry ──►│  │  accept  │
 │          │  │  Save()  │   │   └─ ws upgrade ─┼──►  loop    │
 │          │  └────┬─────┘   └──────────────────┘  └─────┬────┘
 │          │       │                                     │
 └──────────┘       ▼                                     ▼
             ┌─────────────┐                      ┌───────────────┐
             │registry.json│                      │ subscribers   │
             │ (atomic     │                      │  map[*sub]{}  │
             │  rename)    │                      │  msgs chan    │
             └─────────────┘                      └───────────────┘
                                                         ▲
                                                         │ ws clients
                                                         │ (browsers,
                                                         │  CLIs)
                                                   ┌─────┴─────┐
                                                   │  network  │
                                                   └───────────┘
```

### Component Responsibilities

| Package | Responsibility | Does NOT |
|---------|----------------|----------|
| `cmd/server` | Parse `-config` flag, call `config.Load`, construct `registry.Store`, `wshub.Hub`, `httpapi.Server` in that order, start `http.Server`, handle signals, call two-phase shutdown. Nothing else. | Contain business logic, define types that escape `main`, do any HTTP routing itself. |
| `internal/config` | Define `Config` and `Credentials` structs. Load + validate `config.yaml` and `credentials.yaml`. Return `(*Config, error)`. | Touch the network, log anything (return errors), hold mutable state. |
| `internal/registry` | Define the `Manifest` domain type. Provide `Store` with `Upsert/Delete/Get/List` + `Capabilities(filter)` queries. Own the `sync.RWMutex`. Own JSON load at startup and atomic write-through after every mutation. | Know about HTTP, know about WebSocket, log requests, validate Basic Auth, call the hub. |
| `internal/httpapi` | Define `Server` struct holding `*registry.Store`, `*wshub.Hub`, `config.Credentials`, `*slog.Logger`. Build `http.ServeMux` with route patterns. Implement middleware chain (recover → log → CORS → auth-where-needed). Implement handlers that validate input, call `registry`, publish to `wshub`. Upgrade WS requests and hand the conn to `wshub`. | Own registry state, own WebSocket connection management (only upgrade + handoff), persist to disk. |
| `internal/wshub` | Define `Hub` (subscribers map, mutex, `Publish`, `Close`) and `subscriber` (msgs chan, closeSlow). Provide `Subscribe(ctx, conn)` that registers a subscriber, runs the writer loop, and blocks until the context ends or the client is kicked. Define the `Event` payload serialization. | Know about HTTP routes, know about `registry.Store` internals, persist anything, authenticate anyone. |

## Recommended Project Structure

```
openburo-server/
├── cmd/
│   └── server/
│       └── main.go                     # ~80 lines — wiring only
├── internal/
│   ├── config/
│   │   ├── config.go                   # Config struct, Load(path) (*Config, error)
│   │   ├── credentials.go              # Credentials struct, LoadCredentials(path) (Credentials, error)
│   │   └── config_test.go              # table-driven valid/invalid fixtures
│   ├── registry/
│   │   ├── manifest.go                 # Manifest domain type + Validate()
│   │   ├── store.go                    # Store{mu, apps, path}, Upsert/Delete/Get/List
│   │   ├── capabilities.go             # Capability type + filter logic (action, mimeType wildcard)
│   │   ├── persistence.go              # load() / saveLocked() — atomic temp+rename
│   │   ├── store_test.go               # concurrency + persistence tests
│   │   └── capabilities_test.go        # wildcard matching tests
│   ├── httpapi/
│   │   ├── server.go                   # Server struct + New() + Handler() http.Handler
│   │   ├── routes.go                   # route table registration (ServeMux patterns)
│   │   ├── middleware.go               # recover, logging, CORS wiring, auth
│   │   ├── registry_handlers.go        # list / get / upsert / delete / capabilities
│   │   ├── ws_handler.go               # /api/v1/capabilities/ws upgrade + hand to hub
│   │   ├── health_handler.go           # /health
│   │   ├── errors.go                   # writeJSONError helper + error envelope
│   │   └── *_test.go                   # httptest.NewServer integration tests
│   └── wshub/
│       ├── hub.go                      # Hub + subscriber + Publish + Close
│       ├── event.go                    # Event struct + JSON marshaling
│       ├── subscribe.go                # Subscribe(ctx, conn) writer loop
│       └── hub_test.go                 # broadcast fan-out + slow-consumer tests
├── config.example.yaml
├── credentials.example.yaml
├── registry.example.json
├── go.mod
├── go.sum
└── README.md
```

### Structure Rationale

- **Four packages, one per domain** — matches STACK.md recommendation and the four requirement groups in PROJECT.md. Each package has ~3–6 files; none balloon past ~400 lines.
- **`cmd/server/main.go` is wiring only** — the "compose root" pattern. Constructors take dependencies explicitly. This makes it trivial to stand up a test harness (`httpapi.New(store, hub, creds, logger)`) without running `main`.
- **`internal/` prevents external imports** — this is a reference server to *read*, not a library to *import*. STACK.md covers this.
- **File-per-route-group inside `httpapi/`** — not file-per-handler (too granular) and not one `handlers.go` (too monolithic at ~5 routes × multiple handlers each). Grouping by URL prefix is the Go-idiomatic middle ground.
- **`wshub` is deliberately thin** — ~3 files, <250 LoC total. The temptation on a reference project is to over-engineer the hub; resist it.
- **No `pkg/`** — nothing here is designed for external consumption. STACK.md already establishes this.
- **Tests co-located** (`_test.go` next to source) — Go standard; no separate `test/` tree.

## Dependency Graph (Explicit, Acyclic)

```
                      cmd/server
                          │
         ┌────────────────┼────────────────┬──────────┐
         ▼                ▼                ▼          ▼
     config          httpapi            wshub     registry
                          │                │          ▲
                          │                │          │
                          ├────────────────┘          │
                          │                           │
                          └───────────────────────────┘
                          │
                          ▼
                      (config types for
                       Credentials only)
```

**Rules enforced by this graph:**

| Rule | Enforcement |
|------|-------------|
| `registry` depends on **nothing** internal | It's the pure domain core. Imports only `encoding/json`, `os`, `sync`, `path/filepath`. Makes it trivially unit-testable. |
| `wshub` depends on nothing internal (not even `registry`) | It ships generic `Event` broadcasts — the caller (httpapi) marshals registry data into events. This inversion keeps the hub reusable and keeps the dependency graph acyclic. |
| `httpapi` depends on `registry` + `wshub` + `config` | This is where the wiring happens: validate → mutate store → publish event. |
| `config` depends on nothing internal | Pure data loading. |
| Nothing depends on `httpapi` | It's the outermost layer (adapter). |
| **No package imports `cmd/server`** | Go enforces this — `main` packages can't be imported. |

**What this forbids (and why it matters):**

- Registry calling the hub directly — would create a circular concern (storage layer knowing about transport) and make registry tests require a hub.
- Hub calling registry methods — would couple broadcast plumbing to domain types and make hub tests require a store.
- `httpapi` being imported by anything — would tempt re-using handler helpers in wrong places (like in-package tests of the hub); keep the adapter an adapter.

**Acyclic verification:** run `go list -deps ./... | grep internal` after scaffolding and confirm the DAG.

## Architectural Patterns

### Pattern 1: Handler as Method on a Server Struct

**What:** All HTTP handlers are methods on a single `httpapi.Server` struct. Dependencies (store, hub, creds, logger) are struct fields set at construction time. Routes are registered in one place.

**When to use:** Any Go HTTP server with >2 handlers that share dependencies. This is *the* idiomatic pattern (popularized by Mat Ryer's "How I write HTTP services after eight years" and reinforced by every modern Go post-2022).

**Trade-offs:**
- Pro: Handlers get dependencies via `s.store` / `s.hub` without package-level globals or parameter bloat.
- Pro: Tests construct `Server` with fakes or real instances freely: `srv := httpapi.New(store, hub, creds, logger); httptest.NewServer(srv.Handler())`.
- Pro: `Handler() http.Handler` returns the fully-wrapped root handler (middleware applied), which is the single testing seam.
- Con: The struct can grow too large if you add fields carelessly. Rule: if the list exceeds ~6 fields, split into sub-servers by concern.

**Example:**

```go
// internal/httpapi/server.go
type Server struct {
    store  *registry.Store
    hub    *wshub.Hub
    creds  config.Credentials
    logger *slog.Logger
    mux    *http.ServeMux
}

func New(store *registry.Store, hub *wshub.Hub, creds config.Credentials, logger *slog.Logger) *Server {
    s := &Server{store: store, hub: hub, creds: creds, logger: logger, mux: http.NewServeMux()}
    s.registerRoutes()
    return s
}

// Handler returns the fully-wrapped http.Handler (the only public surface).
func (s *Server) Handler() http.Handler {
    return s.withMiddleware(s.mux)
}

// internal/httpapi/routes.go
func (s *Server) registerRoutes() {
    s.mux.HandleFunc("GET    /health",                        s.handleHealth)
    s.mux.HandleFunc("GET    /api/v1/registry",               s.handleRegistryList)
    s.mux.HandleFunc("GET    /api/v1/registry/{appId}",       s.handleRegistryGet)
    s.mux.HandleFunc("POST   /api/v1/registry",               s.requireAuth(s.handleRegistryUpsert))
    s.mux.HandleFunc("DELETE /api/v1/registry/{appId}",       s.requireAuth(s.handleRegistryDelete))
    s.mux.HandleFunc("GET    /api/v1/capabilities",           s.handleCapabilities)
    s.mux.HandleFunc("GET    /api/v1/capabilities/ws",        s.handleWSUpgrade)
}
```

Note the Go 1.22 `ServeMux` method-prefixed patterns — no third-party router needed (per STACK.md).

### Pattern 2: `coder/websocket` Canonical Chat Hub (Adapted)

**What:** A `Hub` holding `map[*subscriber]struct{}` under a `sync.Mutex`. Each subscriber has a buffered `msgs chan []byte` and a `closeSlow()` callback. `Publish(msg)` iterates subscribers with a non-blocking `select { case s.msgs <- msg: default: go s.closeSlow() }` — slow consumers are dropped, not back-pressured.

**When to use:** Every broadcast WebSocket server in Go built on `coder/websocket`. This is the pattern the library's own `internal/examples/chat` codifies.

**Trade-offs:**
- Pro: `coder/websocket` guarantees concurrent-write safety on a single `*websocket.Conn`, so writers don't need per-conn mutexes (this is the big simplification vs gorilla).
- Pro: No "hub goroutine" needed. The gorilla pattern runs one `hub.run()` goroutine reading from `register/unregister/broadcast` channels because gorilla requires write serialization. With `coder/websocket`, fan-out can happen directly in `Publish()` on the caller's goroutine.
- Pro: Slow-consumer drop is bounded — the publisher never blocks, and misbehaving clients don't poison the fan-out.
- Con: Dropped messages aren't replayed. For this project that's *fine* — clients refetch `/api/v1/capabilities` on reconnect; `REGISTRY_UPDATED` is a hint, not a delivery guarantee.
- Con: Unbounded subscriber map growth if clients never disconnect. Bounded by `subscriberMessageBuffer` (default 16) and slow-consumer kick.

**Example:**

```go
// internal/wshub/hub.go
type Hub struct {
    logger          *slog.Logger
    msgBuffer       int                // default 16
    publishLimiter  *rate.Limiter      // optional; safe to remove for v1

    mu              sync.Mutex
    subscribers     map[*subscriber]struct{}
    closed          bool
}

type subscriber struct {
    msgs      chan []byte
    closeSlow func()
}

func New(logger *slog.Logger) *Hub {
    return &Hub{
        logger:      logger,
        msgBuffer:   16,
        subscribers: make(map[*subscriber]struct{}),
    }
}

// Publish fans out msg to all current subscribers. Never blocks.
// Slow subscribers are kicked (their context cancels and the writer loop exits).
func (h *Hub) Publish(msg []byte) {
    h.mu.Lock()
    defer h.mu.Unlock()
    if h.closed {
        return
    }
    for s := range h.subscribers {
        select {
        case s.msgs <- msg:
        default:
            go s.closeSlow()
        }
    }
}

// Close shuts the hub: no more publishes, all subscribers kicked.
// Writer loops exit when their contexts are cancelled.
func (h *Hub) Close() {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.closed = true
    for s := range h.subscribers {
        go s.closeSlow()
    }
    h.subscribers = nil
}

func (h *Hub) addSubscriber(s *subscriber) {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.subscribers[s] = struct{}{}
}

func (h *Hub) removeSubscriber(s *subscriber) {
    h.mu.Lock()
    defer h.mu.Unlock()
    delete(h.subscribers, s)
}
```

```go
// internal/wshub/subscribe.go
//
// Subscribe is called by the httpapi WS handler after websocket.Accept().
// It blocks until the client disconnects or ctx is cancelled.
func (h *Hub) Subscribe(ctx context.Context, c *websocket.Conn) error {
    ctx = c.CloseRead(ctx) // discard any incoming frames; this is broadcast-only

    s := &subscriber{
        msgs: make(chan []byte, h.msgBuffer),
        closeSlow: func() {
            c.Close(websocket.StatusPolicyViolation, "subscriber too slow")
        },
    }
    h.addSubscriber(s)
    defer h.removeSubscriber(s)

    pingTick := time.NewTicker(30 * time.Second) // TODO: from config
    defer pingTick.Stop()

    for {
        select {
        case msg := <-s.msgs:
            writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
            err := c.Write(writeCtx, websocket.MessageText, msg)
            cancel()
            if err != nil {
                return err
            }
        case <-pingTick.C:
            pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
            err := c.Ping(pingCtx)
            cancel()
            if err != nil {
                return err
            }
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}
```

### Pattern 3: Mutation → Broadcast in the Handler Layer (Not the Registry)

**What:** After a successful `store.Upsert(...)` or `store.Delete(...)`, the HTTP handler explicitly calls `hub.Publish(event)`. The registry has *no* knowledge of the hub.

**When to use:** Any time you have a pure storage layer and a separate notification/transport layer. This is the "keep the domain core dependency-free" principle (a lightweight flavor of hexagonal architecture) — appropriate here because the registry is genuinely simple and doesn't need observer-pattern machinery.

**Trade-offs:**
- Pro: Registry stays pure — no callback registration, no `chan Event`, no test setup requiring a hub. Its tests are `store_test.go` with just `store := registry.NewStore(tempfile); store.Upsert(...); got := store.List(); require.Equal(...)`.
- Pro: The "what triggers a broadcast?" question has exactly one answer: "a handler that returned 2xx after mutating." No surprise broadcasts from internal registry code paths.
- Pro: Easy to add handler-level concerns (only broadcast if content actually changed; skip broadcast during bulk imports) without touching the registry.
- Con: If a second mutation path appears (e.g., a CLI that mutates via direct Go calls), that path must also remember to publish. Mitigation: for v1 the HTTP API is the only mutation path; if that changes, introduce an event bus then.

**Example:**

```go
// internal/httpapi/registry_handlers.go
func (s *Server) handleRegistryUpsert(w http.ResponseWriter, r *http.Request) {
    var m registry.Manifest
    if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
        s.writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
        return
    }
    if err := m.Validate(); err != nil {
        s.writeJSONError(w, http.StatusBadRequest, err.Error())
        return
    }

    created, err := s.store.Upsert(m)
    if err != nil {
        s.logger.Error("registry upsert failed", "appId", m.AppID, "err", err)
        s.writeJSONError(w, http.StatusInternalServerError, "persistence failed")
        return
    }

    // Broadcast AFTER successful persist. Fire-and-forget; never blocks
    // the HTTP response because Hub.Publish is non-blocking.
    s.hub.Publish(wshub.NewRegistryUpdatedEvent(m.AppID, "upserted"))

    status := http.StatusOK
    if created {
        status = http.StatusCreated
    }
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(m)
}
```

Note: `Upsert` returns `(created bool, err error)`. The handler uses `created` for 201 vs 200. Registry returns a flag, not an HTTP status — the HTTP concern stays in `httpapi`.

### Pattern 4: Atomic Write-Through Persistence Inside Registry

**What:** `Store.Upsert` and `Store.Delete` hold the mutex, update the in-memory map, then call an internal `saveLocked()` that marshals the entire map to a `*.tmp` file in the same directory and `os.Rename`s it over `registry.json`. Atomic on POSIX; the reader either sees the old file or the new file, never a partial write.

**When to use:** Small registries (< few MB), low-to-moderate write rates, single-writer processes. Perfect fit for OpenBuro's expected scale.

**Trade-offs:**
- Pro: Durability is correct by construction. No WAL, no recovery procedure, no "what if we crashed mid-write?" debugging.
- Pro: Simple to implement (~20 lines in `persistence.go`).
- Pro: Holding the mutex during disk I/O is acceptable at this scale — registry fits in memory, mutations are rare (human-speed CLI calls, not a firehose).
- Con: Write latency = serialize entire map + fsync + rename. Fine for manual admin use; would become a bottleneck at hundreds of writes/second (not in scope).
- Con: Long mutex hold during disk I/O blocks concurrent reads. At this scale, negligible; measurable only if you ran a benchmark.
- Trade: Considered **write-behind with debounce**. Rejected for v1: makes crash semantics subtle (last few mutations lost), adds a goroutine + timer, and doesn't pay for itself until write rates climb.
- Trade: Considered **append-only log + snapshot**. Rejected for v1: overkill for a reference implementation; STACK.md already flags this as the v2 migration path if the registry grows past ~10MB.

**Example:**

```go
// internal/registry/persistence.go
func (s *Store) saveLocked() error {
    // Caller must hold s.mu (write lock).
    tmp := s.path + ".tmp"
    f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
    if err != nil {
        return fmt.Errorf("open temp: %w", err)
    }
    enc := json.NewEncoder(f)
    enc.SetIndent("", "  ")
    if err := enc.Encode(s.apps); err != nil {
        _ = f.Close()
        _ = os.Remove(tmp)
        return fmt.Errorf("encode: %w", err)
    }
    if err := f.Sync(); err != nil {
        _ = f.Close()
        _ = os.Remove(tmp)
        return fmt.Errorf("fsync: %w", err)
    }
    if err := f.Close(); err != nil {
        _ = os.Remove(tmp)
        return fmt.Errorf("close: %w", err)
    }
    if err := os.Rename(tmp, s.path); err != nil {
        return fmt.Errorf("rename: %w", err)
    }
    return nil
}

// store.go
func (s *Store) Upsert(m Manifest) (created bool, err error) {
    s.mu.Lock()
    defer s.mu.Unlock()
    _, existed := s.apps[m.AppID]
    s.apps[m.AppID] = m
    if err := s.saveLocked(); err != nil {
        // Roll back in-memory state on persistence failure to stay consistent.
        if existed {
            // We need the previous value; see note below.
        } else {
            delete(s.apps, m.AppID)
        }
        return false, err
    }
    return !existed, nil
}
```

**Rollback nuance:** On persistence failure for an update, we need the prior value to roll back. Either keep a snapshot before mutation (`prev := s.apps[m.AppID]`) or document that a persistence failure means the server should exit (caller sees 500, operator restarts, on-disk file is still consistent, in-memory state is discarded). For a reference impl, either is defensible — prefer the explicit pre-mutation snapshot.

### Pattern 5: Middleware Chain as Function Composition

**What:** Middleware are `func(http.Handler) http.Handler` decorators. The server's `withMiddleware` method composes them in a fixed order.

**When to use:** Any stdlib-based Go HTTP server with cross-cutting concerns (logging, CORS, auth, recovery).

**Trade-offs:**
- Pro: Idiomatic Go — matches `net/http` contract, works with anything that returns `http.Handler`.
- Pro: Order is explicit and in one place (no hidden registration order bugs).
- Pro: `rs/cors` (from STACK.md) returns `http.Handler`, slotting straight in.
- Con: Per-route middleware (auth only on writes) is handled via wrapping the specific `HandlerFunc` at registration time, not via grouping — this is slightly less elegant than chi's `Route/Group`, but honest. For 2 auth'd routes, it's fine.

**Example:**

```go
// internal/httpapi/middleware.go
func (s *Server) withMiddleware(h http.Handler) http.Handler {
    // Innermost → outermost. Recovery runs first (outermost).
    h = s.corsMiddleware(h)
    h = s.logMiddleware(h)
    h = s.recoverMiddleware(h)
    return h
}

func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if rec := recover(); rec != nil {
                s.logger.Error("panic in handler", "err", rec, "path", r.URL.Path)
                http.Error(w, "internal server error", http.StatusInternalServerError)
            }
        }()
        next.ServeHTTP(w, r)
    })
}

// Per-route auth wrapper — applied at registration time for write routes only.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        user, pass, ok := r.BasicAuth()
        if !ok || !s.creds.Verify(user, pass) {
            w.Header().Set("WWW-Authenticate", `Basic realm="openburo"`)
            s.writeJSONError(w, http.StatusUnauthorized, "authentication required")
            return
        }
        next(w, r)
    }
}
```

### Pattern 6: Two-Phase Graceful Shutdown

**What:** On SIGINT/SIGTERM: (1) call `httpSrv.Shutdown(ctx)` to stop accepting new connections and drain in-flight HTTP requests; (2) call `hub.Close()` to kick all WebSocket subscribers (their contexts cancel, writer loops exit, connections close).

**When to use:** **Any Go server that mixes `http.Server` and WebSocket.** This is a correctness requirement, not a style preference: `http.Server.Shutdown` explicitly does *not* close hijacked connections (per Go docs). Without `hub.Close()`, WebSocket goroutines leak and the process hangs on exit.

**Trade-offs:**
- Pro: Clean shutdown on SIGTERM enables Kubernetes/systemd rolling restarts, which is still nice to have even for a reference impl.
- Pro: Two-phase is explicit about the two kinds of connections the server owns.
- Con: Slightly more ceremony than "just Shutdown" — but the alternative is incorrect.

**Example:**

```go
// cmd/server/main.go (shutdown sequence)
func main() {
    // ... parse flags, load config, build store/hub/server ...

    httpSrv := &http.Server{
        Addr:    cfg.Server.Addr,
        Handler: apiServer.Handler(),
    }

    // Signal-aware root context.
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    // Run ListenAndServe in a goroutine; send errors to a channel.
    errCh := make(chan error, 1)
    go func() {
        logger.Info("http listening", "addr", cfg.Server.Addr)
        if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
            errCh <- err
        }
    }()

    select {
    case err := <-errCh:
        logger.Error("http server error", "err", err)
        os.Exit(1)
    case <-ctx.Done():
        logger.Info("shutdown signal received")
    }

    // Phase 1: stop accepting new HTTP/WS connections, drain in-flight HTTP.
    shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer cancel()
    if err := httpSrv.Shutdown(shutCtx); err != nil {
        logger.Error("http shutdown", "err", err)
    }

    // Phase 2: kick all active WebSocket subscribers. This is required because
    // Shutdown does NOT close hijacked connections.
    hub.Close()

    logger.Info("shutdown complete")
}
```

## Data Flow

### Flow 1: `POST /api/v1/registry` → Broadcast

```
Browser/CLI                                                    WebSocket clients
    │                                                                │
    │ POST /api/v1/registry                                          │
    │ Authorization: Basic ...                                       │
    │ { "appId": "drive-x", ... }                                    │
    │                                                                │
    ▼                                                                │
┌─────────────────────┐                                              │
│ httpapi.Server      │                                              │
│ (recover → log →    │                                              │
│  CORS → requireAuth)│                                              │
└────────┬────────────┘                                              │
         │                                                           │
         │ 1. r.BasicAuth() → creds.Verify()                         │
         │                                                           │
         │ 2. json.Decode(&manifest); manifest.Validate()            │
         │                                                           │
         ▼                                                           │
┌─────────────────────┐                                              │
│ registry.Store      │                                              │
│  .Upsert(m)         │                                              │
│                     │                                              │
│  s.mu.Lock()        │                                              │
│  _, existed := ...  │                                              │
│  s.apps[m.ID] = m   │                                              │
│  saveLocked() ──────┼──► registry.json (write temp + rename)       │
│  s.mu.Unlock()      │                                              │
│                     │                                              │
│  return (!existed, nil)                                            │
└────────┬────────────┘                                              │
         │                                                           │
         │ 3. hub.Publish(Event{type: "REGISTRY_UPDATED",            │
         │                      appId: "drive-x", op: "upserted"})   │
         │                                                           │
         ▼                                                           │
┌─────────────────────┐                                              │
│ wshub.Hub           │                                              │
│  h.mu.Lock()        │                                              │
│  for s := range     │                                              │
│      subscribers:   │                                              │
│    select {         │                                              │
│      case s.msgs <- msg: ─────────┐                                │
│      default: go s.closeSlow()    │                                │
│    }                              │                                │
│  h.mu.Unlock()                    │                                │
│                                   │                                │
│ (non-blocking, returns            │                                │
│  immediately)                     │                                │
└────────┬──────────────────────────┘                                │
         │                          │                                │
         │ 4. 201 Created            │                                │
         │    { manifest JSON }     │                                │
         │                          ▼                                │
         ▼                  ┌───────────────┐                        │
    HTTP response           │ subscriber    │                        │
                            │ writer loop   │                        │
                            │ (one per WS)  │                        │
                            │               │                        │
                            │ case msg:     │                        │
                            │   c.Write(    │                        │
                            │     ctx,      │                        │
                            │     Text, msg)├────────────────────────►
                            └───────────────┘              WS frame
                                                    { "type": "REGISTRY_UPDATED",
                                                      "appId": "drive-x", ... }
```

**Key properties of this flow:**
- Registry persistence happens **before** the broadcast, so subscribers who react by calling `GET /api/v1/capabilities` always see the new state.
- `hub.Publish` is **non-blocking** — the HTTP response isn't held up waiting for slow WS clients.
- Slow WS clients are **dropped**, not back-pressured — they get `StatusPolicyViolation` and reconnect.
- Registry mutex is released **before** the broadcast, so a slow WS fan-out (which shouldn't happen, but...) can't block concurrent registry reads.

### Flow 2: WebSocket Client Subscribe

```
Browser                              httpapi                wshub
   │                                    │                     │
   │ GET /api/v1/capabilities/ws        │                     │
   │ Upgrade: websocket                 │                     │
   ├───────────────────────────────────►│                     │
   │                                    │                     │
   │                       websocket.Accept(w, r, opts)       │
   │◄───────────────────────────────────│                     │
   │  101 Switching Protocols           │                     │
   │                                    │                     │
   │                                    │ hub.Subscribe(      │
   │                                    │   r.Context(), c)   │
   │                                    ├────────────────────►│
   │                                    │                     │ addSubscriber
   │                                    │                     │ c.CloseRead(ctx)
   │                                    │                     │ loop:
   │                                    │                     │   msgs or ping
   │ (receive broadcasts as they        │                     │   or ctx.Done
   │  arrive via subscriber.msgs)       │                     │
   │◄───────────────────────────────────┤◄────────────────────┤
   │                                    │                     │
   │ close / network drop               │                     │
   ├───────────────────────────────────►│                     │
   │                                    │                     │ ctx cancels
   │                                    │                     │ loop exits
   │                                    │                     │ removeSubscriber
   │                                    │                     │ defer c.CloseNow
   │                                    │                     │
```

### Flow 3: Startup

```
main()
  ├─ flag.Parse(-config)
  ├─ cfg, _   := config.Load(cfg.Path)          ← fails fast on bad YAML
  ├─ creds, _ := config.LoadCredentials(cfg.CredentialsPath)
  ├─ logger   := slog.New(slog.NewJSONHandler(...))
  ├─ store, _ := registry.NewStore(cfg.RegistryPath)
  │                 └─ load registry.json if exists, else empty map
  ├─ hub      := wshub.New(logger)
  ├─ api      := httpapi.New(store, hub, creds, logger)
  ├─ httpSrv  := &http.Server{Addr: cfg.Addr, Handler: api.Handler()}
  ├─ ctx, _   := signal.NotifyContext(bg, SIGINT, SIGTERM)
  ├─ go httpSrv.ListenAndServe()
  ├─ <-ctx.Done()
  ├─ httpSrv.Shutdown(shutCtx)                  ← phase 1
  └─ hub.Close()                                ← phase 2
```

## Build Order (Phase-Relevant)

The recommended build order is a direct consequence of the dependency graph — each package depends only on the ones before it.

```
1. config         → nothing depends on it yet
2. registry       → depends on nothing internal; can test in isolation
3. wshub          → depends on nothing internal; can test with a mock *websocket.Conn or a real httptest server
4. httpapi        → depends on all three; integration test with httptest.NewServer
5. cmd/server     → wiring only; smoke test by running the binary
```

**Rationale:**

| Order | Rationale |
|-------|-----------|
| `config` first | Needed to parameterize everything else. Pure data structures — no blockers. Smallest, done in a sitting. |
| `registry` second | The domain core. Can be built + tested end-to-end (including JSON persistence) with zero knowledge of HTTP or WebSocket. Highest "information density per LoC" — once this is right, business correctness is locked in. |
| `wshub` third | Independent of `registry` by design. Can be tested with a fake event payload and one real `coder/websocket` connection via `httptest`. Building it before `httpapi` means the HTTP layer can wire it on day one. |
| `httpapi` fourth | Needs both `registry` and `wshub` to exist. This is where the integration happens and where most of the "does the whole thing work?" tests live. |
| `cmd/server` last | Pure wiring. Writing `main.go` when `httpapi.New(...)` already exists is ~50 lines. |

**Alternative orders considered:**
- `httpapi` before `wshub`: rejected — you'd have to stub broadcasts or leave the WS handler as a 501 for a while. The hub is small; build it up-front.
- `cmd/server` first (skeleton walking): tempting but creates a moving target because `main.go` changes every time a package's constructor signature changes. Better to write `main.go` once, at the end, from a position of knowing all the constructors.

**Parallelizable work:**
- `config`, `registry`, and `wshub` can be built in parallel once the public signatures are pinned (which they are, above). A team of 2 could split `registry` + `wshub`.

## Testing Strategy & Seams

| Layer | Test Type | Seam | Tool |
|-------|-----------|------|------|
| `config` | Table-driven unit | `Load(path)` with fixture files in `testdata/` | stdlib `testing` + `testify/require` |
| `registry.Store` | Table-driven unit, including concurrency | `NewStore(tempFile)`; exercise `Upsert/Delete/Get/List` directly; for concurrency, spawn N goroutines and verify final state | stdlib `testing`, `t.TempDir()`, `sync.WaitGroup` |
| `registry` persistence | Round-trip test | Upsert → close store → reopen from same file → assert state | `t.TempDir()` |
| `wshub.Hub` | Unit + one integration | Unit: call `Publish` and inspect subscriber channels with a "fake subscriber" helper. Integration: `httptest.NewServer` wrapping a handler that calls `Subscribe` → connect two real `coder/websocket` clients → publish → both receive | `httptest`, `coder/websocket` (same lib on both sides) |
| `httpapi` handlers | Integration via `httptest.NewServer(srv.Handler())` | Construct a real `Server` with a real `Store` (tmp file) and a real `Hub`. Send HTTP requests with `http.Client`. Assert JSON responses. | `httptest.NewServer`, stdlib `net/http` client, `require` |
| End-to-end WS | Integration | Same `httptest.NewServer`, upgrade with `coder/websocket.Dial`, POST to `/api/v1/registry` with `http.Client`, assert the WS client received the event | `httptest`, `coder/websocket` |
| `cmd/server` | Smoke only | Build the binary, run with a fixture config, curl `/health`, kill it | shell or `t.Skip` unless CI is configured |

**Testability seams designed in:**

- `httpapi.New(store, hub, creds, logger)` — **every dependency injected**. No globals. Tests construct real or fake versions freely.
- `httpapi.Server.Handler()` returns `http.Handler` — the single point that `httptest.NewServer` needs.
- `registry.NewStore(path)` takes an explicit path, so `t.TempDir()` just works.
- `wshub.Hub.Publish` takes `[]byte`, not a registry type — so hub tests don't import `registry`.
- The middleware chain is a single function (`withMiddleware`) — easy to bypass in unit tests of individual handlers by calling the method directly with `httptest.NewRecorder`.

**Anti-pattern avoided:** testify/mock for dependencies. For this project, **hand-written fakes or real-with-tempfile is strictly simpler** than generated mocks, and test failures point at real code instead of mock expectations. STACK.md already establishes this.

## Scaling Considerations

| Scale | Architecture Adjustments |
|-------|--------------------------|
| **0–1,000 manifests, <100 WS clients** (v1 target) | No changes. Everything as described above. Expected hackathon load. |
| **1k–10k manifests, ~1k WS clients** | Still fine. Profile `saveLocked` duration — if it exceeds ~50ms, switch persistence to write-behind with debounce or to a batched append log. Hub fan-out is still cheap (`O(subscribers)` non-blocking). |
| **10k+ manifests or 10k+ WS clients** | Migrate persistence to embedded SQLite (`modernc.org/sqlite`, per STACK.md) or Postgres. Consider sharding the hub by `appId` hash if broadcasts become CPU-bound. Beyond this point, the architecture is wrong for the load and needs a rewrite — which is fine, because this is explicitly a reference implementation. |

### Scaling Priorities (what breaks first)

1. **`saveLocked` under write pressure** — the first bottleneck. At ~100 writes/sec sustained, the fsync starts to dominate. Fix: debounce + dirty flag (write at most once per 100ms if dirty).
2. **JSON file size** — rewriting a 10MB JSON blob on every mutation is absurd. Fix: STACK.md's SQLite migration path.
3. **Subscriber count × broadcast rate** — hub fan-out is `O(n)` per publish. At 10k subscribers × 10 broadcasts/sec, that's 100k non-blocking channel sends/sec, which is still fine on a modern CPU but worth profiling. Not a v1 concern.
4. **Mutex contention on the registry** — only relevant if reads are in the tens-of-thousands per second *and* writes are frequent. Not a v1 concern; `RWMutex` handles the read-heavy case.

Explicitly **not** a near-term scaling concern: TLS termination (handled by stdlib / reverse proxy), JSON marshaling throughput (stdlib is fine at this scale), Basic Auth bcrypt cost (cost 12 is ~200ms per verify, fine for human rate, would need caching at automated-write rates — but automated writes aren't in scope).

## Anti-Patterns

### Anti-Pattern 1: Hub Reaches Into Registry

**What people do:** `wshub.Hub` holds a `*registry.Store` and, in a goroutine, polls or subscribes to registry changes.

**Why it's wrong:**
- Creates a cyclic-ish concern (storage → transport → storage) even if the package graph is acyclic.
- Forces hub tests to import `registry`, which in turn means hub tests need a tempfile and a fake manifest, just to test broadcast mechanics.
- Forces registry to grow a change-notification API (callbacks, channels) to support the hub, breaking its "pure core" property.

**Do this instead:** Broadcast is triggered in the HTTP handler (Pattern 3), after the mutation succeeds. The hub is a dumb broadcast bus.

### Anti-Pattern 2: Registry Locks Held During Broadcast

**What people do:** Put `hub.Publish(...)` inside `store.Upsert` while `store.mu` is still held.

**Why it's wrong:**
- Even though `Publish` is non-blocking, it acquires `hub.mu`. Now you have a lock order: `store.mu` → `hub.mu`. If any other code ever acquires them in the opposite order, you have a deadlock.
- More importantly, it conflates storage and notification, violating the "registry knows nothing about transport" rule.

**Do this instead:** Return from `Upsert`, release `store.mu`, *then* call `hub.Publish`. Handler-layer sequencing makes this natural.

### Anti-Pattern 3: One Goroutine per Broadcast per Subscriber

**What people do:** `go subscriber.write(msg)` inside the fan-out loop to avoid blocking on any one slow client.

**Why it's wrong:**
- Unbounded goroutine growth under write pressure; a single burst can spawn thousands.
- Messages can arrive at a subscriber out of order (goroutine scheduling is non-deterministic).
- Masks the slow-consumer problem instead of handling it.

**Do this instead:** Buffered channels + non-blocking `select` with drop-slow-consumer semantics (Pattern 2). `coder/websocket`'s chat example is the reference.

### Anti-Pattern 4: Per-Connection Write Mutex

**What people do:** Copy the gorilla pattern of wrapping each `*websocket.Conn` in a struct with a `sync.Mutex` to serialize writes.

**Why it's wrong:** Unnecessary with `coder/websocket` — its `Write` method is concurrent-safe by design. Adding a mutex is cargo-culted gorilla discipline and adds zero value.

**Do this instead:** Trust the library. One writer goroutine per subscriber (the `Subscribe` loop) is still the right pattern, but it's a *design* choice (serialized ordering, one place to handle errors), not a *concurrency-safety* requirement.

### Anti-Pattern 5: `http.Server.Shutdown` Alone

**What people do:** Treat `httpSrv.Shutdown(ctx)` as sufficient for graceful shutdown.

**Why it's wrong:** `Shutdown` explicitly does *not* close hijacked connections (WebSocket upgrades use `http.Hijacker`). WebSocket goroutines leak; the process hangs until context timeout or an orphaned connection finally errors out.

**Do this instead:** Two-phase shutdown (Pattern 6): `httpSrv.Shutdown(ctx)` **then** `hub.Close()`.

### Anti-Pattern 6: Functional Options on Every Constructor

**What people do:** `registry.NewStore(registry.WithPath(...), registry.WithLogger(...), registry.WithAtomicWrite(...))`.

**Why it's wrong:** For a 4-package reference server with 2–3 config knobs per constructor, functional options are ceremony. `registry.NewStore(path string) (*Store, error)` is clearer and one-line shorter than every option variant.

**Do this instead:** Plain struct-literal config or plain constructor args until the arg count genuinely exceeds 4–5. Then reconsider.

### Anti-Pattern 7: Leaking `http.Request.Context()` into Long-Lived Code

**What people do:** Pass `r.Context()` into `hub.Publish` or store it in a subscriber to derive timeouts later.

**Why it's wrong:** The request context is cancelled as soon as the HTTP response is written. Any background work keyed to it gets cancelled prematurely.

**Do this instead:** For WebSocket `Subscribe`, the context is tied to the WS connection lifetime, not the original HTTP upgrade request — derive it from `context.Background()` with a cancel function stored on the subscriber (or use `coder/websocket`'s `CloseRead` which returns a fresh context). For `Publish`, don't take a context at all — broadcast is fire-and-forget and returns immediately.

## Integration Points

### External Services

| "Service" | Integration Pattern | Notes |
|-----------|---------------------|-------|
| File system (`registry.json`) | Atomic temp-file + rename inside `saveLocked()` | POSIX-atomic. On Windows, `os.Rename` is still atomic-ish post-1.5 but less guaranteed — acceptable for reference impl. |
| File system (`config.yaml`, `credentials.yaml`) | Read-once at startup via `os.ReadFile` + `yaml.Unmarshal` | No hot-reload (explicit PROJECT.md decision). |
| Browser clients | CORS via `rs/cors` middleware (STACK.md) | WebSocket origin check handled by `coder/websocket.Accept` options + `rs/cors`. |
| CLI / `curl` clients | Same HTTP API, no special handling | `Authorization: Basic` header on writes. |

### Internal Package Boundaries

| Boundary | Communication | Contract |
|----------|---------------|----------|
| `cmd/server` ↔ `httpapi` | Constructor call: `httpapi.New(store, hub, creds, logger) *Server` | `Server.Handler()` returns `http.Handler`. |
| `cmd/server` ↔ `wshub` | Constructor call: `wshub.New(logger) *Hub`. Shutdown call: `hub.Close()`. | `Hub` is safe for concurrent use. |
| `cmd/server` ↔ `registry` | Constructor call: `registry.NewStore(path) (*Store, error)` | Loads `registry.json` at construction. Thread-safe for all methods. |
| `cmd/server` ↔ `config` | Function calls: `config.Load(path)`, `config.LoadCredentials(path)` | Pure loaders; return `(*Config, error)`. |
| `httpapi` ↔ `registry` | Direct method calls: `store.Upsert(m)`, `store.List()`, etc. | `registry` has no knowledge of HTTP. Errors are domain errors, mapped to HTTP status in `httpapi`. |
| `httpapi` ↔ `wshub` | Direct method calls: `hub.Publish(bytes)`, `hub.Subscribe(ctx, conn)`. | `wshub` has no knowledge of registry types — hub speaks `[]byte`, httpapi marshals events. |
| `httpapi` ↔ `config` | Reads `config.Credentials` from its struct field at request time. | Credentials are immutable after construction (no hot reload). |

## Scaling Considerations (Sanity Check)

**Expected reality for v1:**
- Registry size: dozens to low hundreds of manifests
- Write rate: a handful per hour (human-driven CLI registrations)
- Read rate: browser/CLI polling, low tens per second at peak
- WebSocket clients: dozens concurrent (browser tabs of the demo page)
- Hackathon demo environment; single process; no replicas

Everything in this document is sized for that reality. The "1k–10k" and "10k+" rows in the scaling table exist so that the architecture has a clear evolutionary path, not because v1 needs those capacities.

## Confidence Breakdown

| Decision | Confidence | Basis |
|----------|------------|-------|
| Four-package `internal/` layout | HIGH | Matches STACK.md which already recommends this; each package maps 1:1 to a PROJECT.md requirement group |
| Dependency graph (registry has no internal deps) | HIGH | Direct consequence of keeping the domain core pure; verifiable by `go list -deps` |
| Mutation → broadcast in handler layer (not registry) | HIGH | Standard Go idiom for "pure core + adapters"; alternative (observer in registry) inverts the dependency and has no offsetting benefit at this scale |
| `coder/websocket` canonical chat hub pattern | HIGH | Verified against `coder/websocket`'s own `internal/examples/chat/chat.go` (subscribers map + buffered msgs channel + non-blocking publish + closeSlow) — this is the library-authored reference pattern |
| Concurrent-write safety of `coder/websocket` eliminates gorilla's hub-goroutine | HIGH | Verified via `pkg.go.dev` — "All methods may be called concurrently except for Reader and Read" |
| Handler-as-method on `Server` struct | HIGH | Ubiquitous idiom post-Mat Ryer; supported by Go 1.22 `ServeMux` patterns; matches STACK.md |
| Atomic temp+rename persistence | HIGH | POSIX-guaranteed atomicity; standard Go pattern; matches the scale |
| Two-phase graceful shutdown | HIGH | Verified: `http.Server.Shutdown` godoc explicitly says it does not close hijacked connections; two-phase is a correctness requirement, not a style choice |
| Build order (config → registry → wshub → httpapi → cmd) | HIGH | Direct consequence of the dependency graph; any other order requires stubs |
| Drop-slow-consumer policy | HIGH | Matches `coder/websocket` canonical example; correct for a "hint, not delivery guarantee" event model |
| Middleware-as-function-composition chain | HIGH | Stdlib idiom; works cleanly with `rs/cors` and Go 1.22 `ServeMux` |

## Open Questions (Phase-Specific Research Flags)

- **Event envelope schema**: what exactly goes in a `REGISTRY_UPDATED` event? Minimal (`{type, appId, op}`) or fat (full manifest)? → Flag for "WebSocket hub" phase; reasonable default is minimal + clients refetch, but worth confirming with the File Picker use case.
- **WebSocket origin allowlist**: `coder/websocket.Accept` rejects cross-origin by default; need to thread the CORS config through to `AcceptOptions.OriginPatterns`. → Flag for "HTTP API" phase.
- **Ping interval configurability**: STACK.md implies it comes from config; need to decide whether `wshub` takes it as a constructor arg or reads from a `HubConfig` struct. → Minor; decide during the wshub phase.
- **Config hot reload**: explicitly out of scope per PROJECT.md, but the `config.Credentials` field on `httpapi.Server` is currently immutable. If v2 wants hot reload, the creds field needs to become an atomic pointer or a small accessor. Document this now, don't build it.

## Sources

**Primary (HIGH confidence):**
- [coder/websocket internal/examples/chat/chat.go](https://github.com/coder/websocket/blob/master/internal/examples/chat/chat.go) — canonical hub/subscriber pattern used by the library's maintainers; source of the non-blocking `select` + `closeSlow` + buffered `msgs` channel design
- [pkg.go.dev: coder/websocket](https://pkg.go.dev/github.com/coder/websocket) — confirmed concurrent-write safety ("All methods may be called concurrently except for Reader and Read"), `CloseRead`, ping/pong semantics, and close handshake behavior
- [Go blog: Routing Enhancements for Go 1.22](https://go.dev/blog/routing-enhancements) — `ServeMux` method matching and path wildcards (used in Pattern 1)
- [pkg.go.dev: net/http `Server.Shutdown`](https://pkg.go.dev/net/http#Server.Shutdown) — official doc confirming Shutdown does not close hijacked connections (Pattern 6 + Anti-Pattern 5)
- [Graceful Shutdowns with signal.NotifyContext (millhouse.dev)](https://millhouse.dev/posts/graceful-shutdowns-in-golang-with-signal-notify-context) — idiomatic `signal.NotifyContext` pattern used in Pattern 6

**Secondary (MEDIUM confidence — opinion/analysis pieces):**
- [Graceful Shutdown in Go: Practical Patterns (VictoriaMetrics blog)](https://victoriametrics.com/blog/go-graceful-shutdown/) — corroborates two-phase shutdown for WebSocket servers
- [Mat Ryer: How I write HTTP services after eight years](https://grafana.com/blog/2024/02/09/how-i-write-http-services-in-go-after-13-years/) — the "handler as method on Server struct" idiom (Pattern 1)
- [Alex Edwards: Organising database access in Go](https://www.alexedwards.net/blog/organising-database-access) — dependency injection via struct fields
- [VideoSDK: Go WebSocket guide 2025](https://www.videosdk.live/developer-hub/websocket/go-websocket) — confirms `coder/websocket` adoption and pattern trends
- [WebSocket.org: Go WebSocket Server Guide — coder/websocket vs Gorilla](https://websocket.org/guides/languages/go/) — concurrent-write safety contrast between libraries

**Project inputs:**
- `.planning/PROJECT.md` — four-domain requirement groups (config, registry, HTTP API, WebSocket hub); constraint "WebSocket hub pattern, not per-connection goroutine storms"; `sync.RWMutex` constraint; out-of-scope list
- `.planning/research/STACK.md` — fixes `coder/websocket`, `net/http` ServeMux, `log/slog`, `sync.RWMutex`, and the four-package layout as the target stack

---
*Architecture research for: Go HTTP REST + WebSocket app registry reference server (OpenBuro)*
*Researched: 2026-04-09*
