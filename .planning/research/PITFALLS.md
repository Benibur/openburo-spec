# Pitfalls Research

**Domain:** Go HTTP + WebSocket server with in-memory registry, JSON file persistence, Basic Auth + bcrypt, `coder/websocket` hub/client pattern
**Researched:** 2026-04-09
**Confidence:** HIGH for the concurrency, bcrypt, and graceful-shutdown items (verified against Go stdlib docs, `coder/websocket` pkg.go.dev, and Go issue tracker); MEDIUM for the hub drop-on-overflow capacity tuning (pattern is universal, exact buffer sizes are opinion).

This doc is the "don't embarrass us" list. Every pitfall below has either (a) bitten a prior Go server of this exact shape in public, or (b) is a known footgun documented in the upstream library. Generic "write tests" advice is excluded by design.

## How to read this

- **Showstopper** = data loss, panic, security hole, or broken server. Must not ship.
- **Serious** = degrades correctness under realistic load, flags project as amateurish, hard to fix after release.
- **Minor** = cosmetic or only matters at scale beyond reference-impl expectations.

## Critical Pitfalls

### Pitfall 1: Broadcasting directly to a WebSocket from the registry mutation path (slow-client stall)

**Severity:** Showstopper (correctness + availability)

**What goes wrong:**
One browser tab is on hotel wifi. Its TCP send buffer fills. The hub, iterating over connected clients in `RegistryStore.Upsert`, calls `conn.Write(ctx, ...)` synchronously for that client. `Write` blocks until the context deadline (or forever if no deadline). Every subsequent registry mutation is now stalled behind the slow client. Eventually the handler goroutines pile up, `POST /api/v1/registry` starts timing out, and the server looks "broken" to the admin while a healthy curl client can't understand why.

**Why it happens:**
The "obvious" hub implementation is a `map[*Client]bool` that the mutation code iterates and writes to in a loop. It looks idiomatic. The failure mode only appears when one client is genuinely slow — fine in local dev, catastrophic on a conference wifi demo.

**Wrong code:**
```go
// internal/wshub/hub.go — BROKEN
func (h *Hub) Broadcast(ctx context.Context, msg []byte) {
    h.mu.RLock()
    defer h.mu.RUnlock()
    for client := range h.clients {
        // One slow client stalls everyone. No timeout isolation.
        _ = client.conn.Write(ctx, websocket.MessageText, msg)
    }
}
```

**Right code:**
```go
// internal/wshub/client.go
type Client struct {
    conn *websocket.Conn
    send chan []byte // buffered, e.g. 16
}

// internal/wshub/hub.go
func (h *Hub) Broadcast(msg []byte) {
    h.mu.RLock()
    defer h.mu.RUnlock()
    for c := range h.clients {
        select {
        case c.send <- msg:
            // queued
        default:
            // Slow client — drop this client entirely. Do NOT block the hub.
            // Closing send signals the writer goroutine to exit, which
            // triggers unregister via deferred cleanup.
            go h.unregister(c) // async to avoid mu upgrade
        }
    }
}

// Each client has its own writer goroutine doing the actual conn.Write,
// with a per-write context deadline derived from the ping interval.
func (c *Client) writeLoop(ctx context.Context) {
    defer c.conn.Close(websocket.StatusNormalClosure, "")
    for {
        select {
        case <-ctx.Done():
            return
        case msg, ok := <-c.send:
            if !ok {
                return
            }
            wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
            err := c.conn.Write(wctx, websocket.MessageText, msg)
            cancel()
            if err != nil {
                return
            }
        }
    }
}
```

**Warning signs:**
- Hub code that holds a mutex while calling `conn.Write` — instant red flag.
- No `default:` branch on channel sends inside the hub.
- No per-client buffered channel at all.
- A single benchmark `curl --limit-rate 1` to the WS endpoint will reproduce the stall in under a minute.

**Phase to address:** The phase that builds the WebSocket hub (per STACK.md layout: `internal/wshub`). This pattern must be in place before the first broadcast is wired from `internal/registry`.

---

### Pitfall 2: Goroutine leak per disconnected WebSocket client

**Severity:** Showstopper (memory leak → OOM after long uptime)

**What goes wrong:**
A client disconnects silently (network drop, tab closed). The hub's reader goroutine for that client is blocked in `conn.Read`. The writer goroutine is blocked on `c.send`. Neither ever exits. After a few thousand connect/disconnect cycles the binary has 10k+ zombie goroutines and its memory grows monotonically.

**Why it happens:**
Three specific bugs combine:
1. The reader goroutine has no way to exit on writer-initiated shutdown.
2. The writer goroutine has no way to exit on reader-initiated shutdown.
3. The hub never learns about the disconnect because neither goroutine signals unregister.

With `coder/websocket` specifically, you *must always read from the connection* or control frames (including close) are never processed. Per the pkg.go.dev docs: "You must always read from the connection. Otherwise control frames will not be handled."

**Wrong code:**
```go
// Only a writer goroutine — no reader at all. Close frames never processed.
func (c *Client) serve(ctx context.Context) {
    for msg := range c.send {
        _ = c.conn.Write(ctx, websocket.MessageText, msg)
    }
}
```

**Right code:**
```go
func (c *Client) serve(parentCtx context.Context) {
    ctx, cancel := context.WithCancel(parentCtx)
    defer cancel()
    defer c.hub.unregister(c) // guaranteed cleanup

    // Since this hub is broadcast-only (server -> client), use CloseRead.
    // It spawns a goroutine that reads and discards data messages, but
    // processes ping/pong/close control frames — which is what we need.
    // Per pkg.go.dev: "Call CloseRead when you do not expect to read any
    // more messages." It also ensures c.Ping and c.Close still work.
    ctx = c.conn.CloseRead(ctx)

    // The writer loop exits when:
    //  - send channel is closed (hub unregistered us)
    //  - ctx is cancelled (CloseRead detected peer close, or parent shutdown)
    pingTicker := time.NewTicker(c.pingInterval)
    defer pingTicker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case msg, ok := <-c.send:
            if !ok {
                return
            }
            wctx, wcancel := context.WithTimeout(ctx, 5*time.Second)
            err := c.conn.Write(wctx, websocket.MessageText, msg)
            wcancel()
            if err != nil {
                return
            }
        case <-pingTicker.C:
            pctx, pcancel := context.WithTimeout(ctx, 5*time.Second)
            err := c.conn.Ping(pctx)
            pcancel()
            if err != nil {
                return
            }
        }
    }
}
```

**Key insight:** `CloseRead` is *the* idiom for write-only WebSocket servers in `coder/websocket`. Without it, you either (a) write a reader loop that does nothing with messages (verbose) or (b) don't read at all (broken — control frames stop working).

**Warning signs:**
- `runtime.NumGoroutine()` in a `/health` response that climbs monotonically under a reconnect loop.
- No call to `CloseRead` in a write-only WS handler.
- No `defer hub.unregister` on the client serve goroutine.
- Integration test that opens+closes 1000 WS connections and asserts goroutine count doesn't grow — this must exist.

**Phase to address:** WebSocket hub phase. Write the integration test at the same time as the hub.

---

### Pitfall 3: Lock ordering deadlock between registry mutex and hub mutex

**Severity:** Showstopper (hard to reproduce in dev, kills production at random)

**What goes wrong:**
- Goroutine A (HTTP handler): holds `registry.mu.Lock()`, calls `hub.Broadcast()`, which calls `hub.mu.RLock()`.
- Goroutine B (hub unregister): holds `hub.mu.Lock()` on unregister, calls back into `registry.Something()` (for example, to emit a "client disconnected" audit log that reads registry state), which calls `registry.mu.RLock()`.
- Classic ABBA deadlock. `go test -race` does not catch this.

**Why it happens:**
Cross-package callbacks feel natural. The hub "knows" the registry exists because both are dependencies of the HTTP handler. Someone adds a convenience method that reaches across.

**How to avoid:**
Strict unidirectional dependency: **registry knows nothing about the hub; the HTTP handler wires events from registry to hub.** The hub exposes only `Broadcast([]byte)` and takes nothing registry-shaped as input.

**Right architecture:**
```go
// internal/registry/store.go — no hub import, ever.
type Store struct {
    mu       sync.RWMutex
    apps     map[string]Manifest
    onChange func() // optional callback, called WITHOUT holding the lock
}

func (s *Store) Upsert(m Manifest) error {
    s.mu.Lock()
    s.apps[m.AppID] = m
    if err := s.persistLocked(); err != nil {
        s.mu.Unlock()
        return err
    }
    s.mu.Unlock() // <- released BEFORE calling the callback

    if s.onChange != nil {
        s.onChange()
    }
    return nil
}

// cmd/server/main.go wires them:
store.OnChange = func() {
    payload, _ := json.Marshal(RegistryUpdatedEvent{...})
    hub.Broadcast(payload)
}
```

The two golden rules: (1) never hold a lock across a call into another package; (2) one-way dependency (registry → callback → hub, never hub → registry).

**Warning signs:**
- `hub` package imports `registry` package. Should not happen.
- Any function that calls `lock()` on both mutexes without documenting the strict ordering.
- Review: grep for method calls between `hub.Broadcast(...)` and `s.mu.Unlock()` — if there's a call to another package's method while holding the lock, it's a potential deadlock.

**Phase to address:** The phase that first connects registry mutations to the hub (likely the HTTP API phase, since that's where wiring happens per the STACK.md layout). Establish the callback pattern on day one.

---

### Pitfall 4: `registry.json` corruption via non-atomic write

**Severity:** Showstopper (data loss)

**What goes wrong:**
The naive implementation is:
```go
f, _ := os.Create("registry.json")
json.NewEncoder(f).Encode(registry)
f.Close()
```
Process crashes (OOM, kill -9, host reboot, power loss) between `Create` (which truncates) and `Close`. On restart, `registry.json` exists but is empty or half-written. The server either fails to start or starts with a corrupted registry. The admin's manifests are gone.

**Why it happens:**
"It's just a JSON file" thinking. The write-then-rename pattern is universally known but commonly skipped in "reference impl" code for brevity.

**Right code (Linux/macOS — the target per Go conventions):**
```go
// internal/registry/persist.go
func (s *Store) persistLocked() error {
    dir := filepath.Dir(s.path)
    tmp, err := os.CreateTemp(dir, ".registry-*.json.tmp")
    if err != nil {
        return fmt.Errorf("create temp: %w", err)
    }
    tmpPath := tmp.Name()
    // Clean up temp file on any error path.
    defer func() {
        if tmpPath != "" {
            _ = os.Remove(tmpPath)
        }
    }()

    enc := json.NewEncoder(tmp)
    enc.SetIndent("", "  ")
    if err := enc.Encode(s.apps); err != nil {
        tmp.Close()
        return fmt.Errorf("encode: %w", err)
    }
    // Flush kernel buffers to disk BEFORE the rename.
    if err := tmp.Sync(); err != nil {
        tmp.Close()
        return fmt.Errorf("fsync temp: %w", err)
    }
    if err := tmp.Close(); err != nil {
        return fmt.Errorf("close temp: %w", err)
    }
    // Atomic on POSIX: reader sees either old or new, never torn.
    if err := os.Rename(tmpPath, s.path); err != nil {
        return fmt.Errorf("rename: %w", err)
    }
    tmpPath = "" // rename succeeded, don't clean up

    // Optionally fsync the directory to guarantee the rename survives
    // a power loss. For a reference impl targeting Linux, do this.
    if d, err := os.Open(dir); err == nil {
        _ = d.Sync()
        _ = d.Close()
    }
    return nil
}
```

**Key constraints this pattern enforces:**
1. Temp file is in the **same directory** as the target — otherwise `os.Rename` crosses filesystems and falls back to copy, losing atomicity.
2. `tmp.Sync()` before rename — without it a power loss can leave the new file existing but empty.
3. Directory fsync — the strongest guarantee for the rename itself surviving a crash.
4. Defer-based cleanup of the temp file on any error path — otherwise failed writes leave `.registry-*.json.tmp` droppings.

**Disk-full handling:** `Encode` or `Sync` will return an error. The handler must return `500 Internal Server Error` AND the in-memory state must either be reverted or the server should refuse further writes until the operator intervenes. The naive path (update map, then fail to persist) creates a divergence between memory and disk. A safe default: persist first, update map only on success. See Pitfall 5.

**Warning signs:**
- `os.Create` + `Encode` + `Close` in persistence code — stop immediately.
- No `Sync` call anywhere in the persistence path.
- Temp file created in `/tmp` or `os.TempDir()` instead of the registry file's directory.
- `kill -9` on the server between two writes leaves the file empty.

**Phase to address:** The registry persistence phase. This must be the first commit in that package, not an optimization added later.

---

### Pitfall 5: In-memory / on-disk divergence on persist failure

**Severity:** Showstopper (silent data corruption + admin confusion)

**What goes wrong:**
```go
func (s *Store) Upsert(m Manifest) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.apps[m.AppID] = m          // <- memory updated
    return s.persistLocked()     // <- disk write fails (disk full)
}
```
Handler returns 500. Admin retries — gets another 500. But `GET /api/v1/registry` now returns the new manifest (from memory). Meanwhile, if the server restarts, the manifest disappears because disk never saw it. Worse: if the client also broadcast on success, subscribers were told the registry changed, but a restart will silently "undo" it.

**Why it happens:**
The obvious code path is "update state, then persist." Rollback semantics are not obvious and aren't covered by basic table-driven tests.

**Right code:**
```go
func (s *Store) Upsert(m Manifest) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    // Snapshot for rollback.
    prev, existed := s.apps[m.AppID]
    s.apps[m.AppID] = m

    if err := s.persistLocked(); err != nil {
        // Roll back memory to keep memory/disk in sync.
        if existed {
            s.apps[m.AppID] = prev
        } else {
            delete(s.apps, m.AppID)
        }
        return fmt.Errorf("persist: %w", err)
    }
    return nil
}
```

Only broadcast the event **after** `Upsert` returns nil. Returning from `Upsert` on error must leave the in-memory state exactly as it was on entry.

**Warning signs:**
- Persist code that doesn't roll back memory on error.
- Broadcast code inside the mutation path that fires before persistence succeeds.
- No test that simulates a persist failure (inject an error via a mockable persister or a write-only temp directory).

**Phase to address:** Registry persistence phase. Add a test that sets the persist path to an unwritable directory and asserts the in-memory state is unchanged after a failed `Upsert`.

---

### Pitfall 6: Graceful shutdown does not wait for WebSocket (hijacked) connections

**Severity:** Showstopper on a reference impl (looks unprofessional in a demo); serious in production

**What goes wrong:**
`http.Server.Shutdown()` gracefully closes normal HTTP connections but, per the Go docs, **does not wait for hijacked connections**. WebSockets are hijacked. On SIGTERM: `Shutdown` returns "successfully," the process exits, and all WS clients get an abrupt TCP reset. Any in-flight broadcast is lost. Worse, if the registry persist is mid-write during shutdown, you get Pitfall 4.

**Why it happens:**
`http.Server.Shutdown` is marketed as "graceful." Developers don't read the fine print: hijacked connections are explicitly excluded.

**Right code:**
```go
// cmd/server/main.go
func main() {
    // ... build hub, registry, server ...

    // Track WS connections on the hub so we can close them on shutdown.
    ctx, cancel := signal.NotifyContext(context.Background(),
        syscall.SIGINT, syscall.SIGTERM)
    defer cancel()

    // Start HTTP server.
    srvErr := make(chan error, 1)
    go func() {
        srvErr <- srv.ListenAndServe()
    }()

    select {
    case err := <-srvErr:
        if !errors.Is(err, http.ErrServerClosed) {
            slog.Error("server crashed", "err", err)
        }
    case <-ctx.Done():
        slog.Info("shutdown signal received")
    }

    // Phase 1: stop accepting new HTTP connections and wait for in-flight
    // non-hijacked requests to complete.
    shutdownCtx, cancelShutdown := context.WithTimeout(
        context.Background(), 10*time.Second)
    defer cancelShutdown()
    if err := srv.Shutdown(shutdownCtx); err != nil {
        slog.Error("http shutdown", "err", err)
    }

    // Phase 2: tell the hub to close all WebSocket clients with a clean
    // close frame. The hub's CloseAll drains each client's send channel
    // (bounded wait) and calls conn.Close(StatusGoingAway, ...).
    hub.CloseAll(shutdownCtx)

    // Phase 3: flush registry once more if your design doesn't persist
    // synchronously on every write. (With the design in Pitfall 4 this
    // is a no-op — each mutation is already durable.)

    slog.Info("shutdown complete")
}
```

`hub.CloseAll` must:
1. Acquire the hub lock once, copy the client list, release the lock.
2. For each client, close the send channel (triggers writer loop exit).
3. Call `client.conn.Close(websocket.StatusGoingAway, "server shutting down")` with a bounded context so a dead peer can't hang shutdown forever.

**Warning signs:**
- `main()` that calls `srv.Shutdown` and then `os.Exit` without any hub cleanup.
- WS clients report connection resets (not clean closes) on server restart.
- `go vet` clean but `kill -SIGTERM` during an active WS session leaves the client in a weird state.

**Phase to address:** The main-wiring phase (`cmd/server/main.go`), as a Definition-of-Done for "server can be restarted cleanly."

---

### Pitfall 7: WebSocket origin check disabled or misconfigured

**Severity:** Showstopper (cross-site WebSocket hijacking)

**What goes wrong:**
Developer hits "this WS connection fails from my browser" during dev. They set `InsecureSkipVerify: true` on `websocket.AcceptOptions`. It ships. Now any malicious site can open a WS to the server from a victim's browser (the victim's cookies/Basic-Auth aren't needed for read-only WS in this project, but the malicious site still gets registry updates in real time — information leak at minimum, and if v2 adds auth'd WS, it's full hijack).

**Why it happens:**
The `coder/websocket` default is strict: per pkg.go.dev docs, "Accept will not allow cross origin requests by default. See the InsecureSkipVerify and OriginPatterns options to allow cross origin requests." The correct fix is `OriginPatterns`; the easy fix is `InsecureSkipVerify`. Developers reach for the easy fix.

**Wrong code:**
```go
// DO NOT DO THIS.
conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
    InsecureSkipVerify: true,
})
```

**Right code:**
```go
// config.yaml drives the allowed origins for the WS endpoint.
conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
    OriginPatterns: cfg.Server.AllowedOrigins, // e.g. ["app.example.com", "localhost:5173"]
})
```

Note the relationship to CORS: `rs/cors` handles cross-origin for REST (preflight, `Access-Control-Allow-Origin`, etc.) but **does NOT affect WebSocket handshake origin checks** — the WS library does its own origin validation because the browser does not send preflight for WS. You must configure both.

**Warning signs:**
- The string `InsecureSkipVerify` appearing anywhere in production code.
- `OriginPatterns` not derived from config.
- No test that asserts a request with `Origin: https://evil.com` is rejected with 403.

**Phase to address:** WebSocket endpoint phase. Write the origin-rejection test at the same time as the accept handler.

---

### Pitfall 8: CORS misconfiguration — `AllowOrigins: ["*"]` with credentials

**Severity:** Serious (browsers silently refuse requests OR security hole if browser quirks align)

**What goes wrong:**
Developer sets `cors.Options{AllowedOrigins: []string{"*"}, AllowCredentials: true}`. Browsers forbid this combination: if the response sets `Access-Control-Allow-Origin: *` together with `Access-Control-Allow-Credentials: true`, the fetch fails. The admin's curl works (no CORS) but the browser UI can't `POST` with Basic Auth. Debugging consumes hours.

**Why it happens:**
`"*"` is the developer's intuition for "allow everything." The credentials-vs-wildcard conflict is spec-level, not enforced by Go linters.

**Right code:**
```go
// internal/httpapi/cors.go
corsMW := cors.New(cors.Options{
    AllowedOrigins:   cfg.Server.AllowedOrigins, // concrete list, no "*"
    AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodDelete},
    AllowedHeaders:   []string{"Authorization", "Content-Type"},
    AllowCredentials: true,
    MaxAge:           300,
})
```

If the project genuinely needs `*` (public registry reads from anywhere), then:
- `AllowCredentials: false` (browsers will send the preflight through).
- For write routes that need Basic Auth, require a specific origin list and document that curl works but web clients need a configured origin.

**Warning signs:**
- `"*"` and `AllowCredentials: true` in the same `cors.Options`.
- No test that a browser-shaped request (includes `Origin` and `Access-Control-Request-Method` on preflight) gets an `Access-Control-Allow-Origin` header that is *not* `*` when credentials are in play.

**Phase to address:** HTTP API / middleware phase.

---

### Pitfall 9: Basic Auth timing attack via username compare

**Severity:** Serious (feasibility of username enumeration via timing)

**What goes wrong:**
```go
user, pass, ok := r.BasicAuth()
if !ok || user != cfg.AdminUser { // <- early return reveals user mismatch
    http.Error(w, "unauthorized", http.StatusUnauthorized)
    return
}
if err := bcrypt.CompareHashAndPassword(cfg.AdminHash, []byte(pass)); err != nil {
    http.Error(w, "unauthorized", http.StatusUnauthorized)
    return
}
```
Attacker sends `admin:x` — fast 401 (bcrypt not run). Attacker sends `root:x` — also fast 401 (bcrypt not run). Attacker sends the real `admin` name — slow 401 (bcrypt runs). Username is now discovered via timing. With only one admin this is mostly a curiosity, but it's the kind of finding that gets a reference impl ridiculed.

**Why it happens:**
The natural control flow is "cheap check first, expensive check only if needed." That's exactly backwards for timing-safe auth.

**Right code:**
```go
import "crypto/subtle"

func (a *Auth) Verify(r *http.Request) bool {
    user, pass, ok := r.BasicAuth()
    if !ok {
        // Still do a bcrypt to equalize time with the authenticated path.
        // Cost: one bcrypt per unauthenticated request. Acceptable for writes.
        _ = bcrypt.CompareHashAndPassword(a.dummyHash, []byte("dummy"))
        return false
    }
    userOK := subtle.ConstantTimeCompare(
        []byte(user), []byte(a.adminUser)) == 1
    hashErr := bcrypt.CompareHashAndPassword(a.adminHash, []byte(pass))
    // Combine both results so a branch on userOK doesn't short-circuit
    // the bcrypt path.
    return userOK && hashErr == nil
}
```

**Notes:**
- `bcrypt.CompareHashAndPassword` is itself constant-time with respect to password content (it re-derives the hash and compares), so passing it a wrong password is safe.
- `subtle.ConstantTimeCompare` requires equal-length inputs to be meaningful; if lengths differ it returns 0 immediately, which still leaks length. For short admin usernames this is acceptable; for paranoid deployments, pad both sides.
- The "always run a bcrypt" pattern above adds ~150ms to every write request. For a reference impl at expected scale this is fine; at 1k req/s on writes it becomes a concern (out of scope for v1).

**Warning signs:**
- `user == cfg.AdminUser` (bare `==`) anywhere in auth code.
- A 401-ing benchmark (`hey -c 10 -n 1000 -a wrong:wrong`) that is dramatically faster than a 401 with the correct username.

**Phase to address:** Basic Auth middleware phase.

---

### Pitfall 10: bcrypt cost too low (or too high)

**Severity:** Serious (cost too low) / Minor (cost too high)

**What goes wrong:**
- **Too low:** Cost < 12 lets an attacker brute-force a leaked `credentials.yaml` hash in hours on commodity hardware. PROJECT.md explicitly mandates cost ≥ 12; verify the spec is enforced at hash-generation time.
- **Too high:** Cost > 14 makes every write request feel sluggish (each bcrypt check is ~1s at cost 14+). Admins file "server is slow" bugs. Not catastrophic, but looks bad.

**Why it happens:**
No default is enforced by the bcrypt package; the developer picks a number. `bcrypt.DefaultCost` is 10, which is too low per the project spec.

**Right code:**
```go
// cmd/gen-credentials/main.go or a script
const minCost = 12
hash, err := bcrypt.GenerateFromPassword([]byte(password), minCost)

// When loading credentials.yaml, verify the stored cost:
func (a *Auth) LoadCredentials(path string) error {
    // ... parse yaml ...
    cost, err := bcrypt.Cost(a.adminHash)
    if err != nil {
        return fmt.Errorf("invalid bcrypt hash in %s: %w", path, err)
    }
    if cost < 12 {
        return fmt.Errorf("bcrypt cost in %s is %d, minimum is 12", path, cost)
    }
    return nil
}
```

**Also:** bcrypt silently truncates passwords > 72 bytes. The package exposes `ErrPasswordTooLong` from `GenerateFromPassword` but `CompareHashAndPassword` silently accepts and compares only the first 72 bytes. The credential generator should reject > 72-byte passwords with a clear error; the server's verify path should ideally do the same (reject rather than accept), to avoid the "I set a long passphrase and only the first 72 bytes matter" footgun.

**Warning signs:**
- Hardcoded `bcrypt.DefaultCost` anywhere.
- No cost validation on credential load.
- A > 72-character password in the credential generator's test suite that passes silently.

**Phase to address:** Credential loading / Basic Auth phase.

---

### Pitfall 11: Credentials in logs (defense in depth)

**Severity:** Serious (PROJECT.md hard constraint — a leak here is a project-embarrassment event)

**What goes wrong:**
1. Request logging middleware logs `r.Header` — `Authorization: Basic YWRtaW46c2VjcmV0` lands in slog output.
2. An error path logs `r.BasicAuth()` on failure, including the attempted password.
3. `slog.Error("auth failed", "request", r)` — Go's default formatting serializes the whole request struct, headers included.
4. A panic in the auth handler includes `r` in the stack trace, which includes headers.

**Why it happens:**
Go's `slog` doesn't know what's a secret. `r.Header` is a plain `map[string][]string`. Logging "the whole request" is a common debugging shortcut.

**Right code:**
```go
// internal/httpapi/middleware/logging.go
func LogRequests(logger *slog.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
            lrw := &loggingResponseWriter{ResponseWriter: w, status: 200}
            next.ServeHTTP(lrw, r)
            logger.Info("http",
                "method", r.Method,
                "path", r.URL.Path,
                "status", lrw.status,
                "duration_ms", time.Since(start).Milliseconds(),
                "remote", r.RemoteAddr,
                // DO NOT log r.Header, r.Body, r.URL.RawQuery (may contain tokens)
            )
        })
    }
}

// Auth failure log:
// WRONG: slog.Warn("auth failed", "user", user, "pass", pass)
// RIGHT: slog.Warn("auth failed", "remote", r.RemoteAddr, "path", r.URL.Path)
// Never log pass. Never log user on failure (it's an enumeration aid).
```

**Also:**
- `slog` with `slog.Any("req", r)` will default-serialize the request including headers. Never pass `r` directly to slog.
- Panic recovery middleware should NOT include `r` in the logged panic — it should include method/path/remote only.
- A grep-based test in CI: `go test -run TestNoCredentialsInLogs ./...` that exercises a failed auth path and asserts the captured slog output contains neither the attempted password nor the `Authorization` header value.

**Warning signs:**
- `slog.*("...", "request", r)` anywhere.
- `r.Header` referenced inside a log call.
- `r.BasicAuth()` result logged on the failure path.

**Phase to address:** Logging middleware phase. Add the "no credentials in logs" CI test as part of the middleware's Definition of Done.

---

### Pitfall 12: HTTP handler panic kills "just the request" — but leaks locked state

**Severity:** Serious (deadlock on the next request)

**What goes wrong:**
Per Go docs: "While any panic from ServeHTTP aborts the response to the client, panicking with ErrAbortHandler also suppresses logging of a stack trace to the server's error log." Good news: the `net/http` server recovers and the process survives. BAD news: if the handler panicked while holding `s.mu.Lock()`, the mutex is never unlocked. The next request to the same endpoint blocks forever. The process looks alive (`/health` returns 200 — if `/health` doesn't touch that mutex) but the actual API is dead.

**Why it happens:**
`defer s.mu.Unlock()` is the idiomatic fix but only if every mutex acquisition uses defer. A `s.mu.Lock(); doWork(); s.mu.Unlock()` without defer leaks the lock on panic.

**Wrong code:**
```go
func (s *Store) Delete(id string) error {
    s.mu.Lock()
    if _, ok := s.apps[id]; !ok {
        s.mu.Unlock()                  // <- only released on !ok
        return ErrNotFound
    }
    delete(s.apps, id)
    if err := s.persistLocked(); err != nil {
        panic(err)                      // <- mutex leaked on panic
    }
    s.mu.Unlock()
    return nil
}
```

**Right code:**
```go
func (s *Store) Delete(id string) error {
    s.mu.Lock()
    defer s.mu.Unlock()                 // <- always released, even on panic
    if _, ok := s.apps[id]; !ok {
        return ErrNotFound
    }
    delete(s.apps, id)
    return s.persistLocked()
}
```

Plus a top-level panic recovery middleware that at least logs the panic and returns 500:
```go
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            defer func() {
                if v := recover(); v != nil {
                    logger.Error("panic in handler",
                        "method", r.Method, "path", r.URL.Path, "panic", v)
                    http.Error(w, "internal error", http.StatusInternalServerError)
                }
            }()
            next.ServeHTTP(w, r)
        })
    }
}
```

**Warning signs:**
- Any `Lock()` / `RLock()` in the codebase without an immediately-following `defer Unlock()` / `defer RUnlock()`.
- No panic-recovery middleware in the handler chain.
- `go vet` won't catch this — it's structural, not syntactic. Human review required.

**Phase to address:** Registry core phase (defer idiom) + HTTP middleware phase (recovery).

---

## Serious Pitfalls (shipping-quality issues)

### Pitfall 13: `go test -race` not in CI

**What goes wrong:**
Pitfalls 1, 2, 3, 10, 12 all have race conditions at their core. Standard `go test` doesn't enable the race detector. A clean `go test ./...` gives false confidence. The bug ships, appears at demo time.

**How to avoid:**
- CI must run `go test -race ./...` — this is non-negotiable for a concurrent server.
- Every WebSocket and registry test should be designed to exercise concurrent paths (goroutines hitting the same code path simultaneously via `sync.WaitGroup`) so the race detector has something to detect.
- A single "smoke" test like `TestStore_ConcurrentUpserts` that spawns 100 goroutines doing `Upsert` and `Get` in parallel for 1 second will catch 80% of real races.

**Phase:** Set up in the CI phase, required gate before any merge.

---

### Pitfall 14: Handler doesn't drain `r.Body` → connection not reused

**What goes wrong:**
```go
func handleUpsert(w http.ResponseWriter, r *http.Request) {
    var m Manifest
    if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
        http.Error(w, "bad json", 400)
        return
    }
    // ... validate ...
    if validationErr != nil {
        http.Error(w, validationErr.Error(), 422)
        return  // <- body not fully drained if the client sent trailing data
    }
}
```
If the client sent more than the JSON object (e.g., trailing whitespace, chunked transfer), `net/http` has to close the connection instead of reusing it for keep-alive. Symptom: curl --http1.1 benchmarks show 2x latency.

**How to avoid:**
Use `io.Copy(io.Discard, r.Body)` before returning on error paths, or use `http.MaxBytesReader` to cap the body and then let `Decode` see EOF on a well-formed small body:
```go
r.Body = http.MaxBytesReader(w, r.Body, 64*1024) // 64KB is enough for a manifest
defer io.Copy(io.Discard, r.Body) // drain leftovers for connection reuse
defer r.Body.Close()
```

Bonus: `MaxBytesReader` prevents an attacker from uploading a 10GB body to OOM the server.

**Phase:** HTTP API phase (every write handler).

---

### Pitfall 15: No `Content-Type` on JSON responses

**What goes wrong:**
Handler writes JSON with `w.Write(b)` without setting `Content-Type: application/json`. Go's `http.DetectContentType` kicks in and guesses, usually `text/plain`. Browser-side fetch clients that expect `application/json` break. `curl | jq` works (doesn't care). Demo works on the CLI, breaks in the browser UI. Looks like a project that was never browser-tested.

**How to avoid:**
A single JSON helper used everywhere:
```go
func writeJSON(w http.ResponseWriter, status int, v any) {
    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(v)
}
```
Never `w.Write(marshaledBytes)` directly.

**Phase:** HTTP API phase.

---

### Pitfall 16: Flaky time-based WebSocket tests

**What goes wrong:**
A test does `time.Sleep(100 * time.Millisecond)` and asserts the hub broadcast was received. It passes on the dev laptop, times out on a loaded CI runner, is quarantined, and then never re-enabled.

**How to avoid:**
- Never `time.Sleep` in tests. Use synchronization primitives: `sync.WaitGroup`, buffered channels with deadlines, or explicit signals.
- For "did the hub broadcast?" tests, subscribe a test client to the hub and read from its send channel with a generous context deadline (1-2 seconds).
- For ping-interval tests, inject the ticker as a dependency so tests can drive it manually.

```go
// Right shape:
func TestHubBroadcast(t *testing.T) {
    hub := NewHub()
    c := hub.RegisterTestClient(t) // returns a client whose send chan is drained to a test-visible queue
    hub.Broadcast([]byte("hello"))
    select {
    case msg := <-c.received:
        require.Equal(t, "hello", string(msg))
    case <-time.After(time.Second):
        t.Fatal("broadcast not received")
    }
}
```

**Phase:** WebSocket hub phase, enforced as "no `time.Sleep` in hub tests" code review rule.

---

### Pitfall 17: `httptest.Server` not closed → goroutine/port leak across tests

**What goes wrong:**
```go
func TestUpsert(t *testing.T) {
    ts := httptest.NewServer(handler)
    // forgot defer ts.Close()
    ...
}
```
Each test leaks an accept loop goroutine. Run the full suite, goroutine count balloons, tests become slow and order-dependent. The race detector may start reporting unrelated noise.

**How to avoid:**
- `defer ts.Close()` immediately on creation, every time.
- Consider a helper `newTestServer(t *testing.T) *httptest.Server` that calls `t.Cleanup(func() { ts.Close() })` automatically.
- A lint rule or code-review checklist item: "every `httptest.NewServer` has a matching `Close` or `t.Cleanup`."

**Phase:** Testing infrastructure phase.

---

## Technical Debt Patterns

Shortcuts that seem reasonable. Note the "acceptable?" column — for a reference impl with explicit v1 scope, several of these are acceptable.

| Shortcut | Immediate Benefit | Long-term Cost | Acceptable for v1? |
|----------|-------------------|----------------|---------------------|
| Rewrite `registry.json` on every mutation (no append log) | Simplest possible persistence | O(n) write per mutation; breaks past ~10MB registry | **Yes** — explicit scope per PROJECT.md, flagged in STACK.md "Stack Patterns by Variant" |
| Single bcrypt check per request (no session) | No session management | Every write is ~150ms | **Yes** — reference impl, writes are rare |
| Full registry snapshot in memory (no lazy loading) | Trivial code | Startup time grows linearly with registry size | **Yes** — expected registry is <1k manifests |
| Global hub singleton | Simpler wiring | Hard to test in isolation, hard to shard later | **No** — cost is zero, pass the hub explicitly in `cmd/server/main.go` |
| `log.Fatal` in library code instead of returning errors | Shorter error paths | Server crashes on recoverable errors; hard to test | **Never acceptable** — return errors, let main decide |
| `panic` on persistence errors | Hides error handling | See Pitfall 12 (leaks locks); kills in-flight requests | **Never acceptable** — return errors |
| Trusting client-supplied `appId` without validation | No parsing code | Path traversal (`../../../etc/passwd`), JSON injection | **Never acceptable** — whitelist `[a-zA-Z0-9._-]{1,64}` |
| CORS `AllowedOrigins: ["*"]` | "Just works" in dev | See Pitfall 8 (credentials conflict), security hole | **Dev only** — must be a concrete list from config in prod |
| Global `*http.Client` for any outbound calls (not relevant for v1) | - | - | N/A — v1 has no outbound HTTP |

---

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| **`rs/cors` + WebSocket** | Assuming `rs/cors` protects the WS endpoint | It doesn't. WS bypasses CORS; `coder/websocket` handles origin checks via `OriginPatterns`. Configure both. |
| **`coder/websocket` + `http.ServeMux`** | Registering the WS handler on a path with a trailing `/` then reaching the wrong handler | Go 1.22 ServeMux method-aware routing: use `mux.HandleFunc("GET /api/v1/capabilities/ws", wsHandler)` — no trailing slash unless you want a subtree. |
| **`slog` + context** | Using a global logger inside handlers and losing request_id correlation | Inject logger via context or a `middleware.WithLogger(r)` helper; attach request ID in a middleware before the logging middleware. |
| **YAML parsing for `credentials.yaml`** | Using `gopkg.in/yaml.v2` which treats `yes`/`no` as booleans — a password of `yes` becomes `true` | Use `go.yaml.in/yaml/v3` per STACK.md; unmarshal passwords into `string` (the new bcrypt hash is never `yes`/`no`, but the password-generation tooling might be affected). |
| **`http.Server` + `ListenAndServeTLS`** | Passing relative paths to cert files that work in `go run` but break in packaged binary | Resolve cert paths relative to the config file's directory, not `os.Getwd()`. |
| **`encoding/json` unmarshalling manifests** | Unknown fields silently accepted, letting clients slip data through that won't roundtrip | Use `decoder.DisallowUnknownFields()` on the upsert handler to reject schema drift early. |

---

## Performance Traps

At reference-impl scale (expected: <1k manifests, <100 concurrent WS clients, <10 writes/sec) most of these don't bite. Listed for awareness.

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Rewriting entire `registry.json` on every mutation | Disk I/O dominates write latency | Append-only log + snapshot | ~10MB registry or >100 writes/sec |
| Single hub mutex for register/unregister/broadcast | Lock contention on broadcast path | Shard hub by hash(appId) | ~10k concurrent WS clients |
| Holding `RWMutex.RLock` during JSON marshal for list endpoint | GET blocks writes | Copy the map under lock, marshal outside the lock | ~1k manifests with many concurrent GETs |
| Logging every request at INFO with structured fields | slog throughput ~500k/s is enough, but string allocation dominates | Sampled logging for health checks, skip `/health` entirely | ~5k req/s |
| Broadcasting the full registry snapshot on every change (not just the diff) | Network fan-out O(manifests × clients) per mutation | Send just the changed manifest ID + event type; clients re-fetch if they want fresh state | ~100 manifests × 100 clients = 10k events per mutation |
| No `MaxHeaderBytes` on `http.Server` | An attacker can send multi-MB headers and OOM | Set `srv.MaxHeaderBytes = 1 << 16` (64KB) explicitly | Immediately under attack |

---

## Security Mistakes

Beyond the OWASP basics. Domain-specific to this shape of server.

| Mistake | Risk | Prevention |
|---------|------|------------|
| Path traversal via `appId` into `registry.json` path | If `appId` is ever used in a file path (e.g., `data/{appId}.json`), `appId="../../../etc/passwd"` is a read primitive | Whitelist `appId` with a regex; never interpolate into paths |
| `DELETE /api/v1/registry/{appId}` without auth (copy-paste bug) | Public registry wipe | Integration test: DELETE without Basic Auth returns 401; DELETE with wrong Basic Auth returns 401; DELETE with correct Basic Auth returns 204 |
| WebSocket endpoint reachable over plain HTTP in production | TLS missing = credentials on the wire (even though WS itself is read-only, CORS origin headers and cookies still leak) | `config.yaml: server.tls.enabled: true` in prod; fail startup if `tls.enabled: false` and `server.host` is not `localhost` |
| `credentials.yaml` committed to git | Hash leak → offline brute force at bcrypt cost 12 is days, not years, with a GPU farm | `.gitignore` entry; CI check that `credentials.yaml` is NOT tracked; ship `credentials.example.yaml` with an obviously-fake hash |
| Logging `X-Forwarded-For` without sanitization | Log injection via crafted header values (newlines) | `slog` handles this correctly (it escapes values) — but hand-rolled `fmt.Fprintf(w, "%s", header)` does not. Use `slog` exclusively. |
| `json.Unmarshal` into `map[string]any` for manifests | Unknown fields accepted; clients can inject arbitrary shapes | Unmarshal into a concrete struct with explicit JSON tags and `DisallowUnknownFields` |
| TOCTOU on `registry.json` at startup | Someone replaces the file between `os.Stat` and `os.Open` | Don't `Stat` first — just `os.Open` and handle `os.ErrNotExist` |
| Returning the manifest with the bcrypt hash when `GET /api/v1/registry` is called (if manifests ever contain sensitive fields) | Leakage | Manifests in this project contain no credentials, but the code review rule "never return `credentials.*` types from a handler" should exist from day one |
| Basic Auth header accepted over HTTP (not HTTPS) in production | Credential sniffing on the wire | Middleware that inspects `r.TLS == nil` and refuses Basic Auth unless `cfg.Server.AllowBasicAuthOverHTTP` (dev only). Alternative: bind only to localhost when TLS is off. |

---

## UX Pitfalls (Operator & Client Experience)

This server has two UX audiences: the **operator** running the binary and the **client apps** consuming the API.

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| Error messages that don't tell the operator *which file* failed to load | "open: no such file" with no path | Always wrap: `fmt.Errorf("load credentials from %s: %w", path, err)` |
| Startup that doesn't log the bound address until after `ListenAndServe` blocks | Operator can't tell if the server actually started until they make a request | Log "listening on :8080" BEFORE calling `ListenAndServe`, from the goroutine that calls it |
| `config.yaml` errors surface only on the first request, not at startup | Operator learns of typos in prod | Validate config at startup: parse AND construct all downstream objects before `ListenAndServe` |
| 500 errors with no body | Client can't diagnose | Always write a JSON error body: `{"error": "human-readable reason", "code": "REGISTRY_PERSIST_FAILED"}` |
| 404 for "app not found" indistinguishable from 404 for "route not found" | Client retries the wrong thing | Custom NotFound handler with a JSON body; per-handler 404s use a different error code |
| WS endpoint closes without a `Close` frame on shutdown | Client app reports "connection reset" instead of "server is going away" | Per Pitfall 6: `conn.Close(websocket.StatusGoingAway, "server shutting down")` in `hub.CloseAll` |
| No example `config.yaml` shipped | Operator has to reverse-engineer the schema from Go structs | Ship `config.example.yaml` (per STACK.md layout) with every field documented in comments |
| No `curl` examples in README | Client devs can't try the API in 30 seconds | README quick-start with three curl commands: upsert, list, delete |
| Time-based log formats (e.g., `2m30s ago`) that look cute but can't be grepped | Ops tooling breaks | slog with RFC3339 timestamps only |
| `GET /health` that reads the registry mutex | A hung mutex (Pitfall 12) makes `/health` fail too — which is bad for orchestrators that restart on health failure, but *good* for detecting the hang | Depends on orchestration strategy. For a reference impl: make `/health` cheap (no mutex), and add `/ready` that does touch the mutex for deeper checks. |

---

## "Looks Done But Isn't" Checklist

Things that appear complete but are missing critical pieces. Run this before declaring a phase done.

- [ ] **Registry persistence:** Works in normal flow — verify by `kill -9` between two writes and checking the file isn't empty or truncated. Verify temp files aren't left over after repeated failures (unwritable dir test).
- [ ] **WebSocket broadcast:** Works with one client — verify with 100 concurrent clients using `go test -race`, and one slow client (`curl --limit-rate`) that does NOT stall the others.
- [ ] **Goroutine hygiene:** Test opens+closes 1000 WS connections and asserts `runtime.NumGoroutine()` returns to baseline within 1 second.
- [ ] **Graceful shutdown:** `kill -SIGTERM` during an active WS session closes the WS with a `StatusGoingAway` frame, not a TCP reset. In-flight `POST` completes and persists. No "temp file left behind" after shutdown.
- [ ] **Basic Auth:** Test covers: no header (401), malformed header (401), wrong user (401), wrong password (401), correct both (200). Timing for each 401 is within 20ms of the others (loose, just to catch gross disparity).
- [ ] **CORS:** Test covers: preflight `OPTIONS` returns correct headers, actual `POST` from allowed origin succeeds, `POST` from disallowed origin returns without `Access-Control-Allow-Origin` header (browser will reject).
- [ ] **WS origin:** Test covers: `Origin: https://allowed.example.com` → handshake succeeds. `Origin: https://evil.com` → handshake rejected with 403.
- [ ] **Credentials never logged:** CI test that captures slog output during a failed auth and asserts it contains neither the password nor the full `Authorization` header.
- [ ] **Config validation:** Malformed `config.yaml` produces a clear startup error with the file path and line number (yaml.v3 gives line numbers — use them).
- [ ] **Error bodies:** Every non-200 response has a JSON body with at least `{"error": "..."}`. 204 (DELETE success) has no body, correctly.
- [ ] **`-race` in CI:** The CI config file explicitly passes `-race` to `go test`. Not just "tests pass" — "tests pass with `-race`."
- [ ] **`go vet` in CI:** Separate CI step. Must pass.
- [ ] **`go mod tidy` clean:** CI check that `go.mod` and `go.sum` are tidy (no unused deps, no missing deps).
- [ ] **Startup observability:** Logs show, in order: config loaded (with path), credentials loaded (count, NOT values), registry loaded (count), listening on address. Operator can see each step.
- [ ] **Panic recovery middleware:** Installed at the outermost handler wrapper. Tested with a handler that deliberately panics — verify server doesn't crash and returns 500.
- [ ] **Handler body drain:** All write handlers either use `MaxBytesReader` + deferred drain, or are covered by a test that sends trailing bytes and verifies the connection is reused (hard to test; MaxBytesReader is the pragmatic choice).
- [ ] **Atomic persist:** Temp file is in the same directory as target. fsync called on temp before rename. Directory fsync called after rename.
- [ ] **bcrypt cost enforced at load:** `credentials.yaml` with a cost-10 hash produces a clear startup error, not a silent accept.

---

## Recovery Strategies

When pitfalls hit despite prevention, how to recover.

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| `registry.json` corrupted (Pitfall 4) | LOW — if you have a backup | Restore from the last periodic backup; if no backup, manually reconstruct from WS client logs (long shot) or restart with empty registry. Post-incident: implement Pitfall 4 fix immediately. |
| Memory leak from zombie WS clients (Pitfall 2) | LOW | Restart the server. Post-incident: add the goroutine-count test to prevent regression. |
| Deadlock (Pitfall 3 or 12) | LOW | Restart the server. Post-incident: grep the codebase for `mu.Lock()` without immediate `defer mu.Unlock()`; add panic-recovery middleware if missing; verify no cross-package callbacks hold locks. |
| Slow-client broadcast stall (Pitfall 1) | LOW (if caught early) / HIGH (if it's been masking other issues in prod) | Restart. Post-incident: implement the buffered-channel-with-drop pattern. |
| Credentials leaked in logs | HIGH | Rotate the credentials immediately. Purge the logs (they're probably already in a SIEM). Audit for the specific log line that leaked. Post-incident: add the CI test asserting logs don't contain the password. |
| TLS cert expired in prod | MEDIUM | Rotate cert, restart server. Pre-incident: monitor cert expiry (out of v1 scope — flag for v2). |
| Server can't start because `config.yaml` is malformed | LOW | Revert the config change. Post-incident: add config validation to CI if possible. |
| Mass zombie goroutines from a browser reconnect storm | MEDIUM | Restart the server. Rate-limit WS accepts (out of scope for v1, but note the risk). |

---

## Pitfall-to-Phase Mapping

This is the critical table for the roadmap phase. Phase names are inferred from STACK.md's `internal/` layout: `config`, `registry`, `httpapi`, `wshub`, plus a wiring phase in `cmd/server` and a CI/testing phase.

| # | Pitfall | Prevention Phase | Verification |
|---|---------|------------------|--------------|
| 1 | Slow-client stall in broadcast | `wshub` | Integration test with one rate-limited WS client + 10 fast clients; fast clients still receive broadcasts within 100ms |
| 2 | Goroutine leak on disconnect | `wshub` | Reconnect-loop test asserting `runtime.NumGoroutine()` returns to baseline |
| 3 | Registry ↔ hub lock ordering deadlock | `httpapi` (wiring phase) | Architecture test: `internal/wshub` package has zero imports from `internal/registry`. `go test -race` on a combined mutate+broadcast workload |
| 4 | `registry.json` corruption on crash | `registry` | `kill -9` during mutation test; disk-full simulation test |
| 5 | Memory/disk divergence on persist failure | `registry` | Inject persist error, assert in-memory state unchanged |
| 6 | Shutdown doesn't close WS connections | `cmd/server` wiring | SIGTERM test: client receives close frame with `StatusGoingAway` |
| 7 | WS origin check disabled | `wshub` | Test: `Origin: https://evil.com` → 403 |
| 8 | CORS wildcard + credentials | `httpapi` middleware | Test: preflight with `Origin: https://allowed.com` gets correct headers; no `*` in production config |
| 9 | Basic Auth timing attack | `httpapi` middleware (auth) | Timing test (loose bound); code review for `subtle.ConstantTimeCompare` |
| 10 | bcrypt cost too low | `config` (credentials loading) | Test: loading a cost-10 hash fails at startup |
| 11 | Credentials in logs | `httpapi` middleware (logging) | CI test: failed-auth log capture asserts no password present |
| 12 | Handler panic leaks lock | `registry` + `httpapi` middleware | Code review: every `Lock` has `defer Unlock`. Panic-recovery middleware installed |
| 13 | `-race` not in CI | CI/testing phase | CI config explicitly runs `go test -race ./...` |
| 14 | Body not drained | `httpapi` handlers | `MaxBytesReader` + deferred drain in every write handler |
| 15 | Missing `Content-Type` on JSON | `httpapi` handlers | Shared `writeJSON` helper; lint rule against direct `w.Write(jsonBytes)` |
| 16 | Flaky time-based WS tests | `wshub` tests | Code review: no `time.Sleep` in hub tests |
| 17 | `httptest.Server` leaks | CI/testing phase | Code review: every `httptest.NewServer` has `t.Cleanup` or `defer Close` |

---

## Sources

**Primary (HIGH confidence):**
- [coder/websocket pkg.go.dev](https://pkg.go.dev/github.com/coder/websocket) — "All methods may be called concurrently except for Reader and Read"; "You must always read from the connection. Otherwise control frames will not be handled"; default strict origin checking via `OriginPatterns`.
- [coder/websocket Conn.CloseRead docs](https://pkg.go.dev/github.com/coder/websocket#Conn.CloseRead) — the canonical idiom for write-only servers; ensures ping/pong/close control frames continue to be processed.
- [net/http.Server.Shutdown pkg.go.dev](https://pkg.go.dev/net/http#Server.Shutdown) — `Shutdown does not attempt to close nor wait for hijacked connections such as WebSockets`; `RegisterOnShutdown` for cleanup callbacks.
- [net/http ErrAbortHandler docs](https://pkg.go.dev/net/http#ErrAbortHandler) — confirms `net/http` recovers from handler panics (process survives, but locks held at panic time leak).
- [golang.org/x/crypto/bcrypt pkg.go.dev](https://pkg.go.dev/golang.org/x/crypto/bcrypt) — `ErrPasswordTooLong` (72-byte ceiling); `ErrMismatchedHashAndPassword`; `bcrypt.Cost(hash)` for validating stored cost.
- [Go issue #8914 — os: make Rename atomic on Windows](https://github.com/golang/go/issues/8914) — confirms POSIX `os.Rename` is atomic but Windows is not; informs the Linux-first recommendation.

**Secondary (MEDIUM confidence — techniques are universal, exact parameters are opinion):**
- [Atomically writing files in Go — Michael Stapelberg](https://michael.stapelberg.ch/posts/2017-01-28-golang_atomically_writing/) — the write-temp-fsync-rename-fsync-dir pattern, canonical reference.
- [google/renameio](https://pkg.go.dev/github.com/google/renameio) — battle-tested library implementation of the same pattern (informs the correctness of the inlined approach).
- [A way to do atomic writes — LWN.net](https://lwn.net/Articles/789600/) — kernel-level semantics of rename+fsync.
- [What Broke When We Pushed WebSockets From 100k to 1M Users — dev.to](https://dev.to/speed_engineer/what-broke-when-we-pushed-websockets-from-100k-to-1m-users-2i24) — the slow-client buffered-channel-drop pattern in a production context.
- [Go WebSocket Server Guide: coder/websocket vs Gorilla — WebSocket.org](https://websocket.org/guides/languages/go/) — contrasts gorilla's concurrent-write panic (archived) with coder's concurrent-safe semantics; informs Pitfall 1 motivation.
- [Building a Scalable Go WebSocket Service — Leapcell](https://leapcell.io/blog/building-a-scalable-go-websocket-service-for-thousands-of-concurrent-connections) — confirms the per-client goroutine pair + buffered send channel as the standard hub pattern.
- [npm/write-file-atomic issue #64 — Rename atomicity is not enough](https://github.com/npm/write-file-atomic/issues/64) — discussion of directory fsync's role in durability guarantees.
- [assert vs require in testify — YellowDuck.be](https://www.yellowduck.be/posts/assert-vs-require-in-testify) — informs the "table-driven tests without flaky sleeps" guidance.

**Project inputs:**
- `.planning/PROJECT.md` — hard constraints: bcrypt cost ≥ 12, credentials never logged, `log/slog` only, `sync.RWMutex` for registry, WebSocket hub pattern, in-memory + JSON persistence, no stress testing, browser + CLI clients (CORS), single-admin v1 assumption.
- `.planning/research/STACK.md` — `coder/websocket` v1.8.x, `rs/cors` v1.11.x, `go.yaml.in/yaml/v3`, Go 1.26, `internal/{config,registry,httpapi,wshub}` layout. Pitfalls are mapped against this exact dependency set.

---
*Pitfalls research for: Go HTTP + WebSocket reference server (OpenBuro)*
*Researched: 2026-04-09*
