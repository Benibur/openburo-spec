# Phase 4: HTTP API - Context

**Gathered:** 2026-04-10
**Status:** Ready for planning

<domain>
## Phase Boundary

Implement `internal/httpapi`: the transport layer that wires `internal/registry` and `internal/wshub` together behind the OpenBuro HTTP+WebSocket contract. This is the **sole** package where both domain packages meet — the unidirectional dependency graph (`httpapi → registry`, `httpapi → wshub`, `registry ⊥ wshub`) is enforced here. Phase 4 is the widest phase of the milestone: 26 requirements spanning authentication, the full `/api/v1/*` REST surface, WebSocket upgrade + broadcast, middleware chain, CORS, structured audit logging, and the test suite that proves the full REST round-trip and WebSocket round-trip via `httptest.NewServer`.

**In scope** (26 requirements):
- **Authentication** (AUTH-01..05): `credentials.yaml` loader with bcrypt cost ≥ 12, Basic Auth middleware (timing-safe), public read routes, credential PII guard
- **REST API** (API-01..11): 5 handlers, Go 1.22+ method-prefixed routes, middleware chain, panic recovery, consistent error envelope, Content-Type discipline, request body hygiene
- **WebSocket wiring** (WS-01, WS-05, WS-06, WS-08, WS-09): upgrade handler, mutation-then-broadcast, full-state snapshot on connect, `OriginPatterns` from shared CORS allow-list, architectural gate (`registry` does not import `wshub`)
- **Operations** (OPS-01, OPS-06): CORS middleware config-driven, structured audit log on writes
- **Testing** (TEST-02, TEST-05, TEST-06): integration tests via `httptest.NewServer`, WS origin-rejection, credential PII assertion

**Out of scope** (Phase 5):
- `cmd/server/main.go` compose-root wiring (OPS-02)
- Signal-aware graceful shutdown with two-phase `httpSrv.Shutdown + hub.Close` (OPS-03, OPS-04)
- Optional TLS via `ListenAndServeTLS` (OPS-05)
- Whole-module race gate (TEST-03)

Phase 4 exposes a `Server` type with enough surface that Phase 5's `main.go` can wire it in under 100 lines. No `main.go` changes happen in Phase 4.

</domain>

<decisions>
## Implementation Decisions

All gray areas in Phase 4 are **builder-internal** (route wiring, middleware composition, error envelope shape, test harness structure, package layout) — not product-visible to end users consuming the REST/WebSocket API. The product-visible shapes (JSON envelopes, status codes, event payloads) are already locked in REQUIREMENTS.md and ROADMAP.md. Per user preference, Claude's discretion is used throughout for the rest. Each decision below is locked for the planner.

### Server constructor — expand Phase 1's signature without breaking it

Phase 1 shipped `httpapi.New(logger *slog.Logger) *Server` wired with `/health` only. Phase 4 **replaces** this constructor with:

```go
// New constructs a Server with all domain and transport dependencies.
// The compose-root (cmd/server/main.go in Phase 5) is the only caller.
//
// All dependencies are required; nil values for any of store, hub, creds,
// or logger panic at construction (no silent slog.Default fallback).
func New(logger *slog.Logger, store *registry.Store, hub *wshub.Hub, creds Credentials, cfg Config) *Server
```

Where:
- `store` is the Phase 2 Registry (*registry.Store)
- `hub` is the Phase 3 Hub (*wshub.Hub)
- `creds` is the Phase 4 credential table (see "Credentials loading" below)
- `cfg` is a Phase 4-local config struct carrying the subset of `config.yaml` the handler layer needs: `AllowedOrigins []string`, `WSPingInterval time.Duration`

The compose-root is responsible for translating `config.Config` into `httpapi.Config` — this keeps `internal/httpapi` from depending on `internal/config` (one fewer import edge; simpler test wiring).

**The existing `handleHealth` handler and its tests stay.** Only `Server.New` and `Server.registerRoutes` expand. Phase 1's `TestHealth` and `TestHealth_RejectsWrongMethod` continue to work via a test helper that constructs a minimal Server with no-op store/hub (or via `t.Helper()` factory below).

**Test construction helper** (unexported, test-only file `server_testutil_test.go`):

```go
// newTestServer builds a Server with a temp-dir-backed store, a real wshub.Hub
// with short ping interval, an empty credential table, and a discard logger.
// Returns the Server plus a cleanup function.
func newTestServer(t *testing.T) (*Server, func()) { ... }
```

All integration tests in this phase use `newTestServer(t)` to avoid boilerplate duplication.

### Credentials type and loading (AUTH-01)

**Type shape** (exported):

```go
// Credentials is the parsed bcrypt-hash table loaded from credentials.yaml.
// Values are bcrypt hashes (cost ≥ 12 enforced at load time).
// The zero value is an empty table — no users, all write requests return 401.
type Credentials struct {
    users map[string][]byte // username → bcrypt hash bytes
}

// LoadCredentials reads credentials.yaml and returns a Credentials table.
// Returns an error if the file is missing, malformed, or any bcrypt hash
// has a cost strictly less than 12.
//
// Missing file is an ERROR (not silent empty), because the operator explicitly
// configured credentials_file in config.yaml — a missing file signals a misconfig.
func LoadCredentials(path string) (Credentials, error)

// Lookup returns the bcrypt hash for a username. The second return is false
// if the user does not exist. Callers MUST still run bcrypt.CompareHashAndPassword
// on the dummy-hash fallback (see authBasic middleware) to preserve timing-safety.
func (c Credentials) Lookup(username string) ([]byte, bool)
```

**YAML shape** (matches the existing `credentials.yaml` example from Phase 1):

```yaml
users:
  admin: "$2a$12$abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123"
  ci: "$2a$14$ABCDEFGHIJKLMNOPQRSTuvwxyzabcdefghijklmnopqrstuvwxyz0123"
```

**Cost verification at load time** (AUTH-01):

```go
for username, hashStr := range raw.Users {
    hash := []byte(hashStr)
    cost, err := bcrypt.Cost(hash)
    if err != nil {
        return Credentials{}, fmt.Errorf("credentials: user %q: invalid bcrypt hash: %w", username, err)
    }
    if cost < 12 {
        return Credentials{}, fmt.Errorf("credentials: user %q: bcrypt cost %d is below minimum 12", username, cost)
    }
    users[username] = hash
}
```

**Package path:** `golang.org/x/crypto/bcrypt` (add via `go get` in Task 0 of plan 04-02).

### Timing-safe Basic Auth middleware (AUTH-02, AUTH-04)

The middleware is the PITFALLS #8 anchor — this is the #1 place an implementation goes wrong. The contract is: **no observable timing signal between "wrong username" and "wrong password."**

```go
// authBasic returns a middleware that enforces HTTP Basic Auth using the
// given credential table. On failure, writes 401 + WWW-Authenticate header
// and returns without invoking the next handler.
//
// Timing-safety contract (AUTH-04):
//   - No early return on username mismatch — always runs bcrypt
//   - Username compared via subtle.ConstantTimeCompare
//   - On unknown user, bcrypt runs against a fixed dummy hash so CPU cost
//     is indistinguishable from the "wrong password" path
//   - Dummy hash is computed once at package init, never changes
func authBasic(creds Credentials, logger *slog.Logger) func(http.Handler) http.Handler
```

**Dummy hash construction** (package init):

```go
// dummyHash is the precomputed bcrypt hash of a known-nonsense value.
// Used in authBasic's "user not found" path so bcrypt.CompareHashAndPassword
// always runs, making the unauthenticated path and the wrong-password path
// indistinguishable by wall-clock time.
//
// Cost 12 matches the minimum enforced for real credentials (AUTH-01).
var dummyHash []byte

func init() {
    h, err := bcrypt.GenerateFromPassword([]byte("openburo:dummy:do-not-match"), 12)
    if err != nil {
        panic(fmt.Sprintf("httpapi: failed to generate dummy hash: %v", err))
    }
    dummyHash = h
}
```

**The middleware body**, line by line (this is the reference pattern — the planner should paste this verbatim and adapt only the error envelope call):

```go
func authBasic(creds Credentials, logger *slog.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            username, password, ok := r.BasicAuth()
            if !ok {
                writeUnauthorized(w)
                return
            }

            // Look up the user; if not found, use the dummy hash so bcrypt
            // always runs. Do NOT early-return here.
            storedHash, found := creds.Lookup(username)
            if !found {
                storedHash = dummyHash
            }

            // Constant-time username comparison — defeats CPU-cache timing
            // attacks that could probe which usernames exist.
            // We compare the looked-up-or-dummy hash, which is timing-safe
            // by construction via subtle.ConstantTimeCompare? No — we compare
            // via bcrypt, which is the password check. The username check
            // is implicit in whether the bcrypt result matches the stored
            // hash for THAT username.
            //
            // The subtle.ConstantTimeCompare is on the BOOLEAN combination
            // of (found AND bcryptMatches): we compute them both
            // unconditionally, then combine.
            bcryptErr := bcrypt.CompareHashAndPassword(storedHash, []byte(password))
            bcryptMatches := bcryptErr == nil

            // subtle.ConstantTimeCompare on two int-valued booleans.
            // This is the final gate — authorized iff the user existed AND
            // the password matched. Short-circuiting on "found" would leak
            // timing, which is why we already ran bcrypt above.
            var foundByte, matchByte byte
            if found {
                foundByte = 1
            }
            if bcryptMatches {
                matchByte = 1
            }
            if subtle.ConstantTimeCompare([]byte{foundByte, matchByte}, []byte{1, 1}) != 1 {
                // Audit the failure (AUTH-05: NO credential material in the log)
                logger.Warn("httpapi: basic auth failed",
                    "path", r.URL.Path,
                    "method", r.Method,
                    "remote", clientIP(r))
                writeUnauthorized(w)
                return
            }

            // Success — stash the authenticated username in the request
            // context so downstream audit logging can emit the `user` field.
            ctx := context.WithValue(r.Context(), ctxKeyUser, username)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

**Key points the planner MUST preserve:**
- `bcrypt.CompareHashAndPassword` runs **unconditionally** for every request (with dummyHash on unknown user)
- The final authorization decision uses `subtle.ConstantTimeCompare` on the boolean tuple `(found, bcryptMatches)` — NOT a short-circuit `if found && bcryptMatches`
- On failure, the Warn log line contains: `path`, `method`, `remote` — **NOT** `username`, NEVER `Authorization`, NEVER `password`
- The authenticated username is stashed in request context under an unexported `ctxKeyUser` type (not a string) to avoid context-key collisions (Go convention)

### Credential PII guard — the TEST-06 test

A dedicated test (`TestAuth_NoCredentialsInLogs`) captures slog output across a **successful** and a **failed** authenticated request, then asserts none of the following appear in any captured log line:
- The literal Authorization header value (`Basic ...`)
- The decoded username
- The decoded password
- The raw bcrypt hash

The test uses a `bytes.Buffer` (wrapped in a mutex for race-safety, like Phase 3's syncBuffer) and a `slog.NewTextHandler` to capture all logs during the request. The assertions grep the buffer contents via `require.NotContains`.

### Error envelope JSON shape (API-09)

**Every 4xx and 5xx response** uses this consistent envelope:

```json
{
  "error": "short human-readable message",
  "details": {
    "field": "optional extra context"
  }
}
```

Helpers (unexported, one file `errors.go`):

```go
// writeJSONError writes a JSON error response with the given status code
// and message. details is optional (pass nil for none). Content-Type is
// set to application/json.
func writeJSONError(w http.ResponseWriter, status int, message string, details map[string]any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    body := struct {
        Error   string         `json:"error"`
        Details map[string]any `json:"details,omitempty"`
    }{
        Error:   message,
        Details: details,
    }
    _ = json.NewEncoder(w).Encode(body)
}

// Shortcut constructors — one per status code used in the handler layer.
func writeBadRequest(w http.ResponseWriter, msg string, details map[string]any) { writeJSONError(w, http.StatusBadRequest, msg, details) }
func writeUnauthorized(w http.ResponseWriter) { writeJSONError(w, http.StatusUnauthorized, "unauthorized", nil) }
func writeForbidden(w http.ResponseWriter, msg string) { writeJSONError(w, http.StatusForbidden, msg, nil) }
func writeNotFound(w http.ResponseWriter, msg string) { writeJSONError(w, http.StatusNotFound, msg, nil) }
func writeInternal(w http.ResponseWriter, msg string) { writeJSONError(w, http.StatusInternalServerError, msg, nil) }
```

**401 responses also set `WWW-Authenticate: Basic realm="openburo"`** so browsers and CLI tools prompt for credentials.

### Route registration (API-06)

Use Go 1.22+ method-prefixed `http.ServeMux` patterns. No third-party router.

```go
func (s *Server) registerRoutes() {
    // Phase 1 route (unchanged)
    s.mux.HandleFunc("GET /health", s.handleHealth)

    // Phase 4 write routes (auth required)
    s.mux.Handle("POST /api/v1/registry",
        s.authBasic(http.HandlerFunc(s.handleRegistryUpsert)))
    s.mux.Handle("DELETE /api/v1/registry/{appId}",
        s.authBasic(http.HandlerFunc(s.handleRegistryDelete)))

    // Phase 4 read routes (public)
    s.mux.HandleFunc("GET /api/v1/registry", s.handleRegistryList)
    s.mux.HandleFunc("GET /api/v1/registry/{appId}", s.handleRegistryGet)
    s.mux.HandleFunc("GET /api/v1/capabilities", s.handleCapabilities)
    s.mux.HandleFunc("GET /api/v1/capabilities/ws", s.handleCapabilitiesWS)
}
```

**Critical:** the `{appId}` path parameter uses Go 1.22's wildcard syntax. Extract it in handlers via `r.PathValue("appId")`.

**Method-prefixed patterns auto-reject the wrong method with 405.** No need for manual method checks in handlers.

### Middleware chain order (API-07)

The chain is constructed in `Server.Handler()`:

```go
func (s *Server) Handler() http.Handler {
    var h http.Handler = s.mux
    h = s.corsMiddleware(h)       // outermost
    h = s.logMiddleware(h)        // middle (skips /health)
    h = s.recoverMiddleware(h)    // innermost — closest to the mux
    return h
}
```

**Order justification** (outermost wraps innermost — what the request hits first):
1. **recover** is the INNERMOST wrap, but gets the request LAST. Wait — that's wrong. Let me re-read API-07: "recover → log → CORS → (per-route) auth".

Corrected order — the chain is applied OUTSIDE-IN as it wraps, but the REQUEST travels OUTSIDE-IN too. So the outermost middleware sees the request FIRST:

```go
func (s *Server) Handler() http.Handler {
    var h http.Handler = s.mux    // base — the mux (with per-route auth inside)
    h = s.corsMiddleware(h)        // 3. CORS wraps log+recover+mux
    h = s.logMiddleware(h)         // 2. log wraps recover+mux (skipping /health)
    h = s.recoverMiddleware(h)     // 1. recover is OUTERMOST — catches panics from ALL inner middleware + handlers
    return h
}
```

Request flow: **recover → log → CORS → mux → (per-route auth) → handler**.

This is the correct reading of API-07. The auth middleware is applied per-route inside `registerRoutes()`, not as part of the global chain.

**Why recover is outermost:** if CORS or log panics, recover still catches it. If recover were innermost, it couldn't save a panic in the log middleware.

### Recover middleware (API-08)

```go
func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if rec := recover(); rec != nil {
                s.logger.Error("httpapi: handler panic",
                    "path", r.URL.Path,
                    "method", r.Method,
                    "panic", fmt.Sprintf("%v", rec),
                    "stack", string(debug.Stack()))
                writeInternal(w, "internal server error")
            }
        }()
        next.ServeHTTP(w, r)
    })
}
```

- The panic is rendered as `%v` (not `%+v`) to avoid accidentally including credential-bearing struct dumps.
- `debug.Stack()` is included — this is a reference impl, operator-friendly debugging beats paranoia.
- `writeInternal` writes the error envelope; the server keeps serving.

**Test:** `TestRecover_PanicCaught` registers a handler that panics on `/panic`, hits it via `httptest.NewServer`, asserts 500 + envelope + server alive on next request.

### Log middleware (API-07, OPS-06 tie-in)

```go
// logMiddleware logs every non-/health request with structured fields.
// /health is skipped explicitly (it's the noisiest route and clutters logs).
func (s *Server) logMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path == "/health" {
            next.ServeHTTP(w, r)
            return
        }
        start := time.Now()
        rw := &statusCapturingWriter{ResponseWriter: w, status: http.StatusOK}
        next.ServeHTTP(rw, r)
        s.logger.Info("httpapi: request",
            "method", r.Method,
            "path", r.URL.Path,
            "status", rw.status,
            "duration_ms", time.Since(start).Milliseconds(),
            "remote", clientIP(r))
    })
}
```

**Fields logged:** `method`, `path`, `status`, `duration_ms`, `remote`. Deliberately NO `user` field here — that's the audit log's job (see below), and the request log must not imply user identity for public read routes.

**`statusCapturingWriter`** is a tiny wrapper around `http.ResponseWriter` that captures the status code:

```go
type statusCapturingWriter struct {
    http.ResponseWriter
    status int
}

func (w *statusCapturingWriter) WriteHeader(code int) {
    w.status = code
    w.ResponseWriter.WriteHeader(code)
}
```

### Audit log (OPS-06)

Write handlers emit a SECOND log line — the audit log — AFTER the mutation succeeds and the broadcast publishes. This is distinct from the request log line.

```go
// Inside handleRegistryUpsert, after store.Upsert succeeds and hub.Publish is called:
s.logger.Info("httpapi: audit",
    "user", username, // from ctx
    "action", "upsert",
    "appId", manifest.ID)
```

Audit log fields:
- `user` — from request context (set by authBasic)
- `action` — one of `upsert`, `delete`
- `appId` — the manifest ID being mutated

**Never** log the manifest body, URL, credentials, or authorization header. TEST-06 asserts this.

### clientIP helper

```go
// clientIP returns the client's IP for logging. Respects X-Forwarded-For
// (first entry) when present, falls back to r.RemoteAddr. For the reference
// impl, this is sufficient — production would need a trusted-proxy allow-list.
func clientIP(r *http.Request) string {
    if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
        if i := strings.Index(fwd, ","); i >= 0 {
            return strings.TrimSpace(fwd[:i])
        }
        return strings.TrimSpace(fwd)
    }
    return r.RemoteAddr
}
```

### JSON decoding discipline (API-11)

**All POST handlers use `json.Decoder.DisallowUnknownFields()`** so misspelled fields fail fast with a clear error rather than silently drop:

```go
func (s *Server) handleRegistryUpsert(w http.ResponseWriter, r *http.Request) {
    defer r.Body.Close() // API-11: close body for connection reuse

    dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)) // 1 MiB cap
    dec.DisallowUnknownFields()

    var manifest registry.Manifest
    if err := dec.Decode(&manifest); err != nil {
        writeBadRequest(w, "invalid JSON body", map[string]any{"reason": err.Error()})
        return
    }
    if err := manifest.Validate(); err != nil {
        writeBadRequest(w, "invalid manifest", map[string]any{"reason": err.Error()})
        return
    }

    // ... upsert + broadcast
}
```

**Body size cap:** 1 MiB via `http.MaxBytesReader`. Larger payloads return 400 automatically.

**Body close:** `defer r.Body.Close()` in every handler — required for connection reuse (API-11).

### REST handlers — behavior per route

Each handler is deliberately thin. The planner gets these skeletons verbatim.

**`POST /api/v1/registry`** (upsert):
1. Decode body with DisallowUnknownFields (400 on error)
2. Validate (400 on error)
3. Check if the appId exists (to decide 201 vs 200 later)
4. `store.Upsert(manifest)` (500 on persist error — the Store returns a wrapped `"persist failed, registry unchanged: ..."` error)
5. Broadcast via `hub.Publish(...)` — see "Event shape" below
6. Audit log
7. Write `201 Created` if newly created, `200 OK` if updated

**`DELETE /api/v1/registry/{appId}`**:
1. Extract `appId` via `r.PathValue("appId")`
2. `existed, err := store.Delete(appId)` (500 on err; 404 if `!existed && err == nil`)
3. If `existed`: broadcast + audit log
4. Write `204 No Content`

**`GET /api/v1/registry`** (public):
1. `manifests := store.List()`
2. Write `200 OK` with `{"manifests": [...], "count": N}`

**`GET /api/v1/registry/{appId}`** (public):
1. `manifest, ok := store.Get(appId)`
2. If `!ok`: 404 with envelope
3. Write `200 OK` with the manifest JSON

**`GET /api/v1/capabilities`** (public):
1. Parse `?action=` and `?mimeType=` query params
2. If `mimeType` is set, call `registry.CanonicalizeMIME` — on error, 400
3. `caps := store.Capabilities(filter)`
4. Write `200 OK` with `{"capabilities": [...], "count": N}`

**`GET /api/v1/capabilities/ws`** (public): see "WebSocket upgrade" below.

### Event shape (WS-05)

Every broadcast is a **single event type** `REGISTRY_UPDATED`. The payload carries just enough to identify what changed; clients refetch `/api/v1/capabilities` to rebuild their view (per FEATURES.md "the broker has one thing to say to everyone").

```json
{
  "event": "REGISTRY_UPDATED",
  "timestamp": "2026-04-10T12:34:56.789Z",
  "payload": {
    "appId": "mail-app",
    "change": "ADDED"
  }
}
```

**Change values:**
- `"ADDED"` — upsert of a new appId
- `"UPDATED"` — upsert of an existing appId
- `"REMOVED"` — delete

**Timestamp:** UTC, RFC 3339 with millisecond precision (`time.Now().UTC().Format(time.RFC3339Milli)` — Go 1.17+).

**Marshalling helper** (unexported, `events.go`):

```go
type changeType string

const (
    changeAdded   changeType = "ADDED"
    changeUpdated changeType = "UPDATED"
    changeRemoved changeType = "REMOVED"
)

type registryUpdatedEvent struct {
    Event     string    `json:"event"`
    Timestamp string    `json:"timestamp"`
    Payload   struct {
        AppID  string     `json:"appId"`
        Change changeType `json:"change"`
    } `json:"payload"`
}

func newRegistryUpdatedEvent(appID string, change changeType) []byte {
    evt := registryUpdatedEvent{
        Event:     "REGISTRY_UPDATED",
        Timestamp: time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00"),
    }
    evt.Payload.AppID = appID
    evt.Payload.Change = change
    b, _ := json.Marshal(evt)
    return b
}
```

(The `json.Marshal` error is discarded because the struct is fixed and cannot fail.)

### WebSocket upgrade handler (WS-01, WS-06, WS-08)

```go
func (s *Server) handleCapabilitiesWS(w http.ResponseWriter, r *http.Request) {
    conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
        OriginPatterns: s.cfg.AllowedOrigins,
        // InsecureSkipVerify is NEVER set. Reviewers should grep for it.
    })
    if err != nil {
        // websocket.Accept already wrote the rejection (403 on origin mismatch);
        // just return.
        return
    }

    // WS-06: send the full-state snapshot BEFORE entering Subscribe.
    // This eliminates the connect-then-fetch race: clients that receive
    // the snapshot then observe subsequent events in order.
    snapshot := s.buildFullStateSnapshot()
    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    err = conn.Write(ctx, websocket.MessageText, snapshot)
    cancel()
    if err != nil {
        conn.Close(websocket.StatusInternalError, "snapshot write failed")
        return
    }

    // Hand off to the hub. Subscribe blocks until disconnect.
    _ = s.hub.Subscribe(r.Context(), conn)
}

// buildFullStateSnapshot returns a REGISTRY_UPDATED event whose payload
// contains a synthetic "FULL_STATE" change type and no appId. Clients
// observe this as their initial "here's everything" message.
//
// Wait — this contradicts the fixed event shape above. Re-read WS-06:
// "A full-state REGISTRY_UPDATED snapshot". Let me reconsider.
```

**Reconsideration:** WS-06 says the snapshot is "a full-state `REGISTRY_UPDATED` snapshot." The canonical interpretation (from FEATURES.md "broker has one thing to say") is that the snapshot IS a `REGISTRY_UPDATED` event — just with a sentinel `change` value that signals "full state, not a diff."

**Locked decision:** The snapshot event uses `change: "SNAPSHOT"` and carries the full capability list in a new `payload.capabilities` field. This is a deliberate deviation from the single-event-shape for upsert/delete:

```json
{
  "event": "REGISTRY_UPDATED",
  "timestamp": "...",
  "payload": {
    "change": "SNAPSHOT",
    "capabilities": [ /* full Store.Capabilities(filter{}) output */ ]
  }
}
```

On upsert/delete, `payload` has `{appId, change: ADDED|UPDATED|REMOVED}`. On initial snapshot, `payload` has `{change: SNAPSHOT, capabilities: [...]}`.

**Rationale:** clients do `state = event.payload.capabilities` on snapshot, then on every subsequent event refetch `/api/v1/capabilities` (or, if they're clever, just refetch on any subsequent event regardless of appId). This keeps the broker's "one thing to say" property while still giving clients a usable initial state.

**Alternative considered and rejected:** a separate `REGISTRY_SNAPSHOT` event type. Rejected because it doubles the event-type surface clients have to handle, contradicting FEATURES.md.

**Payload struct update:**

```go
type eventPayload struct {
    Change       changeType                `json:"change"`
    AppID        string                    `json:"appId,omitempty"`
    Capabilities []registry.CapabilityView `json:"capabilities,omitempty"`
}
```

`omitempty` on both `AppID` and `Capabilities` so upsert events have no `capabilities` field and snapshot events have no `appId` field.

### Mutation-then-broadcast wiring (WS-05, WS-09)

The architectural rule from PITFALLS #1: **broadcast happens in the handler layer AFTER the store mutation succeeds, NEVER inside the registry package.** Enforced by the gate `go list -deps ./internal/registry | grep wshub` → must be empty.

```go
// inside handleRegistryUpsert, AFTER store.Upsert returns nil:
change := changeAdded
if alreadyExisted { // captured before the Upsert call
    change = changeUpdated
}
s.hub.Publish(newRegistryUpdatedEvent(manifest.ID, change))
```

**Ordering is load-bearing:** the store mutation MUST succeed before the broadcast. If the broadcast happens first and the mutation fails, subscribers see a phantom event for state that doesn't exist.

### CORS middleware (OPS-01)

Use `github.com/rs/cors` (add via `go get` in Task 0 of plan 04-05):

```go
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
    c := cors.New(cors.Options{
        AllowedOrigins:   s.cfg.AllowedOrigins,
        AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodOptions},
        AllowedHeaders:   []string{"Authorization", "Content-Type"},
        AllowCredentials: true,
        MaxAge:           300,
    })
    return c.Handler(next)
}
```

**Critical:** `AllowedOrigins` is the same slice passed into `websocket.AcceptOptions.OriginPatterns`. The allow-list is **shared** — a browser client that can call the REST API can also connect to the WebSocket endpoint.

**`AllowCredentials: true` combined with `AllowedOrigins: ["*"]` is rejected by rs/cors at construction time** (as of rs/cors v1.11.x). This is a good thing — prevents the PITFALLS #9 footgun. The planner should NOT attempt to bypass this; if the operator sets `allowed_origins: ["*"]` in config.yaml, the constructor panics with a clear message.

**Allow-list fallback:** if the operator leaves `allowed_origins: []` empty in config.yaml, the Server constructor returns an error. Phase 5's main.go surfaces that error and refuses to start. Rationale: empty allow-list means all WebSocket connections would be rejected anyway (since `AcceptOptions.OriginPatterns` would be empty), so failing fast at startup is operator-friendly.

### Package layout

```
internal/httpapi/
├── doc.go              // extended from Phase 1
├── server.go           // Server, Config, New, Handler, registerRoutes (EXPAND Phase 1)
├── credentials.go      // Credentials type + LoadCredentials + Lookup + YAML struct
├── auth.go             // authBasic middleware + dummyHash + ctxKeyUser
├── middleware.go       // recoverMiddleware, logMiddleware, corsMiddleware, clientIP, statusCapturingWriter
├── errors.go           // writeJSONError + writeBadRequest/Unauthorized/etc
├── events.go           // registryUpdatedEvent type + newRegistryUpdatedEvent + newSnapshotEvent
├── handlers_registry.go // handleRegistryUpsert, handleRegistryDelete, handleRegistryList, handleRegistryGet
├── handlers_caps.go    // handleCapabilities, handleCapabilitiesWS + buildFullStateSnapshot
├── health.go           // EXISTING from Phase 1 — do not modify (will be re-verified by tests)
│
├── server_test.go      // newTestServer helper + Server lifecycle tests
├── credentials_test.go // LoadCredentials path coverage (missing/malformed/low-cost/valid)
├── auth_test.go        // timing-safe auth test + PII-free auth log test
├── middleware_test.go  // recover middleware test + log middleware skips /health
├── errors_test.go      // error envelope JSON shape
├── events_test.go      // event marshal tests + snapshot shape
├── handlers_registry_test.go // REST handler integration via httptest.NewServer
├── handlers_caps_test.go // capabilities + WS handler integration
├── health_test.go      // EXISTING from Phase 1 — may need newTestServer adaptation
│
└── testdata/
    └── credentials-valid.yaml
    └── credentials-low-cost.yaml
    └── credentials-malformed.yaml
```

Target: ~800-1200 LoC production + ~1500-2000 LoC tests (this is a BIG phase; cost is appropriate).

### Integration test strategy (TEST-02)

**One `TestServer_Integration_RESTRoundTrip`** uses `httptest.NewServer(s.Handler())` and exercises the full cycle:

```go
func TestServer_Integration_RESTRoundTrip(t *testing.T) {
    srv, cleanup := newTestServer(t)
    defer cleanup()

    ts := httptest.NewServer(srv.Handler())
    defer ts.Close()

    client := ts.Client()

    // 1. POST /api/v1/registry — 201 on create
    manifest := `{"id":"mail-app","name":"Mail","url":"https://mail.example/","version":"1.0","capabilities":[{"action":"PICK","path":"/pick","properties":{"mimeTypes":["*/*"]}}]}`
    req, _ := http.NewRequest("POST", ts.URL+"/api/v1/registry", strings.NewReader(manifest))
    req.SetBasicAuth("testuser", "testpass")
    req.Header.Set("Content-Type", "application/json")
    resp, err := client.Do(req)
    require.NoError(t, err)
    require.Equal(t, http.StatusCreated, resp.StatusCode)
    resp.Body.Close()

    // 2. GET /api/v1/registry — list returns the manifest
    resp, err = client.Get(ts.URL + "/api/v1/registry")
    require.NoError(t, err)
    require.Equal(t, http.StatusOK, resp.StatusCode)
    require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
    // ... parse, assert count==1, assert manifest fields

    // 3. GET /api/v1/registry/mail-app — fetch single
    // 4. POST /api/v1/registry with same id — 200 on update
    // 5. GET /api/v1/capabilities?action=PICK — returns 1 capability
    // 6. GET /api/v1/capabilities?mimeType=image/png — returns 1 (wildcard match)
    // 7. DELETE /api/v1/registry/mail-app — 204
    // 8. GET /api/v1/registry/mail-app — 404 with envelope
    // 9. POST without auth — 401 with WWW-Authenticate
    // 10. POST with invalid JSON — 400 with envelope
    // 11. POST with missing required field — 400 with envelope
}
```

**One `TestServer_Integration_WebSocketRoundTrip`** covers the WS path:

```go
func TestServer_Integration_WebSocketRoundTrip(t *testing.T) {
    srv, cleanup := newTestServer(t)
    defer cleanup()

    ts := httptest.NewServer(srv.Handler())
    defer ts.Close()

    // 1. Connect to /api/v1/capabilities/ws
    wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/capabilities/ws"
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    conn, _, err := websocket.Dial(ctx, wsURL, nil)
    require.NoError(t, err)
    defer conn.Close(websocket.StatusNormalClosure, "")

    // 2. First message is a SNAPSHOT event with an empty capabilities list
    _, msg, err := conn.Read(ctx)
    require.NoError(t, err)
    var snap registryUpdatedEvent // or a testing struct
    require.NoError(t, json.Unmarshal(msg, &snap))
    require.Equal(t, "REGISTRY_UPDATED", snap.Event)
    require.Equal(t, "SNAPSHOT", string(snap.Payload.Change))

    // 3. Upsert a manifest via REST (separate client, authed)
    // 4. Next WS message is an UPDATED event for that appId, received within 1s
    // 5. Delete — next WS message is a REMOVED event
}
```

**Use `require.Eventually` with a 2s timeout for WS event arrival** — no `time.Sleep`.

### WebSocket origin rejection test (TEST-05)

```go
func TestServer_WebSocket_RejectsDisallowedOrigin(t *testing.T) {
    srv, cleanup := newTestServer(t)
    defer cleanup()

    ts := httptest.NewServer(srv.Handler())
    defer ts.Close()

    // Dial with an Origin header NOT in the allow-list
    wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/capabilities/ws"
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()

    _, resp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
        HTTPHeader: http.Header{"Origin": []string{"https://evil.example"}},
    })
    require.Error(t, err)
    require.NotNil(t, resp)
    require.Equal(t, http.StatusForbidden, resp.StatusCode)
}
```

The `newTestServer` helper configures `AllowedOrigins: []string{"https://allowed.example"}` so the allow-list is non-empty and the disallowed origin is specifically rejected.

### Plan breakdown — honor ROADMAP sketch

ROADMAP.md pre-sketched 5 plans. Honor this breakdown, with the Wave assignments calibrated to dependencies:

- **04-01-server-middleware-PLAN.md** (Wave 1) — Server struct expansion with `store`, `hub`, `creds`, `cfg` fields; Config type; middleware chain (recover + log + statusCapturingWriter + clientIP); error envelope helpers (errors.go); extended registerRoutes stubs (handlers can be `http.NotFound` placeholders at this point). **First commit honors critical research flag:** the middleware chain order + `registerRoutes` method-pattern convention + the "never use `slog.Default`" constraint.
  - Requirements landed: API-06, API-07, API-08, API-09, API-10, API-11 (error envelope shape, content type, body close)

- **04-02-auth-credentials-PLAN.md** (Wave 2) — Credentials type + LoadCredentials (with bcrypt cost check) + YAML fixtures; authBasic middleware with timing-safety (dummyHash + subtle.ConstantTimeCompare tuple); PII-guard log line on auth failure.
  - Requirements landed: AUTH-01, AUTH-02, AUTH-03, AUTH-04, AUTH-05, TEST-06
  - **Depends on:** 04-01 (uses the error envelope helpers, writes to the log middleware)

- **04-03-registry-handlers-PLAN.md** (Wave 3) — handleRegistryUpsert, handleRegistryDelete, handleRegistryList, handleRegistryGet + events.go (registryUpdatedEvent + newRegistryUpdatedEvent) + mutation-then-broadcast wiring in upsert/delete handlers + audit log.
  - Requirements landed: API-01, API-02, API-03, API-04, WS-05, WS-09, OPS-06
  - **Depends on:** 04-02 (uses authBasic on write routes)

- **04-04-capabilities-ws-PLAN.md** (Wave 4) — handleCapabilities + handleCapabilitiesWS + buildFullStateSnapshot + WS upgrade via websocket.Accept + full-state snapshot on connect wired through hub.Subscribe.
  - Requirements landed: API-05, WS-01, WS-06
  - **Depends on:** 04-03 (uses the events.go helpers, reuses the broadcast plumbing)

- **04-05-cors-integration-tests-PLAN.md** (Wave 5) — CORS middleware with shared allow-list (config-driven); OriginPatterns wiring (WS-08); the two big integration tests (REST round-trip + WS round-trip); WS origin rejection test; credential PII test end-to-end (TEST-06 final assertions across the full middleware chain); architectural gate verification.
  - Requirements landed: OPS-01, WS-08, TEST-02, TEST-05, TEST-06 (final end-to-end validation)
  - **Depends on:** 04-04 (needs the WS upgrade handler wired up)

Waves are sequential (plans build on each other). Parallel execution within Phase 4 is NOT possible — each plan layers on the previous.

### Claude's Discretion (for the planner)

- Exact test function names (just mirror Phase 2/3 style: `TestServer_*`, `TestAuth_*`, `TestHandleRegistry_*`)
- Whether to split `handlers_registry.go` and `handlers_caps.go` or keep them in one `handlers.go` — the plan above suggests two files; the planner may consolidate if simpler
- Exact phrasing of error envelope messages (just keep them short + lowercase + no trailing period)
- How to generate bcrypt fixtures for testdata (via a one-shot helper test or a shell script that writes them)
- Whether to add a `t.Cleanup` to `newTestServer` or return a manual `cleanup()` func — either works
- Whether the REST integration test uses a single `t.Run("...", func(t *testing.T){...})` block per sub-step or one monolithic function — either is fine; sub-tests give better failure localization but both match Go conventions

### Go toolchain — CRITICAL

**Every `<automated>` verify command MUST use `~/sdk/go1.26.2/bin/go`.** The system `go` is 1.22 and will fail. This was a gotcha in Phases 2 and 3; the planner has been instructed to preserve it in Phase 4.

### Dependencies to add in Phase 4

- `golang.org/x/crypto/bcrypt` — Task 0 of plan 04-02
- `github.com/rs/cors` — Task 0 of plan 04-05
- `github.com/coder/websocket` is already a direct dependency from Phase 3; Phase 4 imports it again in `handlers_caps.go`

All added via `~/sdk/go1.26.2/bin/go get` + `go mod tidy`. No version pinning beyond what `go get` selects.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Contracts and constraints
- `.planning/PROJECT.md` — core value, shipped reqs (Phases 1-3), active reqs (Phase 4)
- `.planning/REQUIREMENTS.md` §Authentication, §HTTP API, §WebSocket (Phase 4 subset), §Operations (OPS-01, OPS-06), §Testing (TEST-02, TEST-05, TEST-06) — the 26 REQ-IDs Phase 4 must close
- `.planning/ROADMAP.md` §"Phase 4: HTTP API" — goal + 6 success criteria + 5 pre-sketched plans
- `.planning/phases/01-foundation/01-CONTEXT.md` — Go 1.26, testify/require, no `slog.Default()` in internal/, `credentials.yaml` YAML shape with bcrypt hashes
- `.planning/phases/02-registry-core/02-CONTEXT.md` — Store API surface, Manifest struct, Capability response shape, Validate error voice
- `.planning/phases/03-websocket-hub/03-CONTEXT.md` — Hub constructor signature (logger + Options), Subscribe(ctx, conn) contract, two close paths with two status codes, byte-oriented contract
- `.planning/phases/03-websocket-hub/03-RESEARCH.md` — `coder/websocket` v1.8.14 API surface (`Accept`, `AcceptOptions.OriginPatterns`, `websocket.Dial`, `Conn.Write`, `StatusPolicyViolation`, `StatusGoingAway`)

### Research (critical for this phase)
- `.planning/research/STACK.md` — `golang.org/x/crypto/bcrypt`, `github.com/rs/cors`, `github.com/coder/websocket`, stdlib `net/http` 1.22+ method patterns, stdlib `encoding/json`, no framework
- `.planning/research/ARCHITECTURE.md` §"internal/httpapi" — handler layer owns store+hub+creds+logger, middleware chain, no imports from registry ↔ wshub
- `.planning/research/PITFALLS.md` §1 (ABBA deadlock: registry NEVER imports wshub), §7 (WebSocket origin check: OriginPatterns from shared allow-list), §8 (timing-safe Basic Auth), §9 (CORS: explicit origin list, never `*` with credentials), §16 (no time.Sleep in tests — inherited from Phase 3)
- `.planning/research/FEATURES.md` §"the broker has one thing to say to everyone" — single event type, no subscribe/unsubscribe protocol, full state on connect

### Library documentation (for the planner's reference)
- `golang.org/x/crypto/bcrypt` — `bcrypt.GenerateFromPassword`, `bcrypt.CompareHashAndPassword`, `bcrypt.Cost`
- `github.com/rs/cors` — `cors.New(Options{...})`, the `AllowedOrigins + AllowCredentials` interaction
- `github.com/coder/websocket` — `websocket.Accept(w, r, *AcceptOptions)`, `AcceptOptions.OriginPatterns`, `websocket.Dial(ctx, url, *DialOptions)`
- Go stdlib `net/http` §1.22+ mux — `HandleFunc("METHOD /path/{param}")`, `r.PathValue("param")`
- Go stdlib `crypto/subtle` — `subtle.ConstantTimeCompare`

### Prior phase artifacts to mirror
- `internal/httpapi/server.go` — existing Phase 1 Server; extend, don't replace
- `internal/httpapi/health.go` — existing Phase 1 health handler; keep as-is
- `internal/httpapi/health_test.go` — existing Phase 1 test; adapt to `newTestServer` if needed
- `internal/registry/store.go` — Phase 2 Store API used in handlers
- `internal/registry/manifest.go` — Phase 2 Manifest + Validate + CanonicalizeMIME used in handlers
- `internal/wshub/hub.go` — Phase 3 Hub API (`Publish`, `Subscribe`, `Close`, `New`, `Options`)

### Out-of-scope references (Phase 5 — do NOT pull into Phase 4)
- `OPS-02` (main.go compose-root under 100 lines) — Phase 5
- `OPS-03` (signal-aware graceful shutdown) — Phase 5
- `OPS-04` (two-phase shutdown wiring: `httpSrv.Shutdown` → `hub.Close`) — Phase 5
- `OPS-05` (optional TLS via `ListenAndServeTLS`) — Phase 5
- `TEST-03` (whole-module race gate) — Phase 5

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`internal/httpapi/server.go`** — Phase 1 placeholder with `New(logger)` and `registerRoutes()` stubs wired for `/health`. Phase 4 expands the constructor and registerRoutes; the file is extended, not replaced.
- **`internal/httpapi/health.go`** — Phase 1 health handler. Stays as-is; the extended Server still wires `/health` in registerRoutes.
- **`internal/httpapi/health_test.go`** — Phase 1 test calling `New(logger)` with just a logger. After Phase 4's constructor expansion, this test will fail to compile. The planner adapts it to use `newTestServer(t)` (the helper that constructs a full Server with store/hub/creds). Alternatively: keep a backward-compat `NewForHealthOnly(logger)` constructor. **Decision: adapt the test, do NOT add a backward-compat constructor.** Reference impls avoid API sprawl.
- **`internal/registry/store.go`** — Store API (`Upsert`, `Delete`, `Get`, `List`, `Capabilities(filter)`) is the handler-layer dependency. Signature-stable since Phase 2.
- **`internal/registry/manifest.go`** — `Manifest` + `Validate` + `CanonicalizeMIME` + `CapabilityView` + `CapabilityFilter` used in handlers.
- **`internal/wshub/hub.go` + `subscribe.go`** — `Hub.New(logger, opts)`, `Hub.Publish([]byte)`, `Hub.Subscribe(ctx, conn)`, `Hub.Close()`. Signature-stable from Phase 3.

### Established Patterns
- **Error wrapping:** `fmt.Errorf("context: %w", err)` with lowercase context strings, no trailing punctuation. Phase 4 inherits this.
- **Validation errors:** field-path-oriented messages (`"credentials: user %q: bcrypt cost %d is below minimum 12"`).
- **Package doc at top of first file:** `internal/httpapi/server.go` already carries the `// Package httpapi ...` comment. Phase 4 extends this to describe the full responsibility.
- **Testdata convention:** `internal/httpapi/testdata/*.yaml` for credential fixtures. Follow Phase 2's `internal/registry/testdata/*.json` pattern.
- **Table-driven tests via `require`:** `github.com/stretchr/testify/require` for all assertions. `require.Eventually` for WS event arrival (no `time.Sleep`).
- **Architectural isolation via `go list -deps`:** Phase 2 and Phase 3 both have isolation gates. Phase 4 adds the SYMMETRIC gate: `go list -deps ./internal/registry | grep -E 'wshub|httpapi'` must be empty (enforces WS-09). Phase 3's `go list -deps ./internal/wshub | grep -E 'registry|httpapi'` gate continues to apply.
- **Logger injection only:** every middleware and handler takes the logger from `s.logger` (the Server field). No `slog.Default()` anywhere. Gate: `! grep -rE 'log/slog|slog\.Default' internal/httpapi/*.go | grep -v _test.go`.
- **No `time.Sleep` in tests:** inherited from Phase 3 (PITFALLS #16). Use `require.Eventually`. Gate: `! grep -n 'time\.Sleep' internal/httpapi/*_test.go`.

### Integration Points
- **Phase 2 (`internal/registry`)**, shipped: `store.Upsert(manifest) error`, `store.Delete(id) (existed bool, err error)`, `store.Get(id) (Manifest, bool)`, `store.List() []Manifest`, `store.Capabilities(filter CapabilityFilter) []CapabilityView`, `registry.CanonicalizeMIME(s string) (string, error)`. Do NOT modify these signatures.
- **Phase 3 (`internal/wshub`)**, shipped: `wshub.New(logger, opts)`, `hub.Publish([]byte)` (fire-and-forget, non-blocking), `hub.Subscribe(ctx, conn) error`, `hub.Close()`. Do NOT modify these signatures.
- **Phase 5 (`cmd/server/main.go`)** — will call `httpapi.New(logger, store, hub, creds, cfg)` from a compose-root under 100 lines. Phase 4 must ensure the constructor signature is stable before Phase 5 lands. All `Config` fields exposed in Phase 4 must be derivable from `config.Config` + `wshub.Options`.
- **`internal/config` is NOT imported** by Phase 4. The compose-root translates `config.Config` into `httpapi.Config` + `wshub.Options` + the credential path. This keeps `internal/httpapi` from depending on `internal/config`, simplifying test wiring (tests construct `httpapi.Config{AllowedOrigins: []string{"..."}}` directly).

</code_context>

<specifics>
## Specific Ideas

- **PITFALLS #8 is the most critical test in Phase 4.** The timing-safe Basic Auth test is the single hardest correctness property to get right, and it's what reviewers will scrutinize most. The implementation MUST pass bcrypt unconditionally via the dummyHash fallback and combine `(found, matches)` via `subtle.ConstantTimeCompare` on a byte tuple. Anything less is a footgun.
- **The credential PII test (TEST-06) is the second-most-important test.** It captures slog output across a full auth cycle and `require.NotContains`es for every PII field. A reference impl that logs credentials is a reference impl nobody will copy.
- **The REST + WS integration test (TEST-02) is the workhorse.** It runs against `httptest.NewServer(s.Handler())` and exercises every route. If it passes, the middleware chain, handlers, and event plumbing are all wired correctly. Make it comprehensive — the cost of a thick integration test is worth it for the surface area this phase covers.
- **The `SNAPSHOT` change value is load-bearing for WS-06.** Clients receive it as their first message and do `state = event.payload.capabilities`. Subsequent upsert/delete events are just "something changed, refetch" — no diff protocol. This matches FEATURES.md's "broker has one thing to say to everyone" philosophy and eliminates the connect-then-fetch race.
- **Middleware order: recover is OUTERMOST.** If recover were innermost (next to the mux), it couldn't catch panics thrown by CORS or log. The outermost wrap is the safest.
- **The `clientIP` helper deliberately trusts `X-Forwarded-For`.** This is a reference impl detail — a hardened prod service would need a trusted-proxy allow-list. The README should note this in the "Known Limitations" section.
- **No request-body streaming.** All POST bodies are read fully via `json.Decoder` with a 1 MiB cap. No streaming decode, no per-chunk processing. Reference-impl simplicity.
- **No pagination on `GET /api/v1/registry` or `GET /api/v1/capabilities`.** The expected registry size (dozens of apps) makes pagination overkill. Deferred to v2 if ever needed.
- **`handleHealth` continues to skip the log middleware** (explicit `/health` check in `logMiddleware`). Phase 1 implemented this philosophy; Phase 4 preserves it.

</specifics>

<deferred>
## Deferred Ideas

- **Compose-root wiring (`cmd/server/main.go`)** — Phase 5 (OPS-02)
- **Signal-aware graceful shutdown with two-phase `httpSrv.Shutdown` → `hub.Close`** — Phase 5 (OPS-03, OPS-04)
- **Optional TLS via `ListenAndServeTLS`** — Phase 5 (OPS-05)
- **Whole-module `go test ./... -race` gate** — Phase 5 (TEST-03)
- **Authentication on WebSocket and read routes** — v2 (SEC-V2-01)
- **Rate limiting / abuse protection** — v2 (SEC-V2-02)
- **OAuth/OIDC** — v2 (SEC-V2-03)
- **Hot-reload of `credentials.yaml`** — v2 (OPS-V2-01)
- **Prometheus `/metrics`** — v2 (OPS-V2-02)
- **OpenTelemetry tracing** — v2 (OPS-V2-03)
- **Event coalescing / debounce** — v2 (FEAT-V2-01)
- **Additional capability actions beyond `PICK | SAVE`** — v2 (FEAT-V2-02)
- **Trusted-proxy allow-list for `X-Forwarded-For`** — v2; v1 trusts the header directly (reference impl)
- **Request body streaming** — not planned; 1 MiB cap covers the reference-impl workload
- **Pagination on list endpoints** — v2 if registry grows beyond dozens of apps
- **Optimistic concurrency / ETag / 409 on upsert conflict** — v2 (CONC-V2-01)

</deferred>

---

*Phase: 04-http-api*
*Context gathered: 2026-04-10*
