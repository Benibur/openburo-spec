---
phase: 04-http-api
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/httpapi/server.go
  - internal/httpapi/middleware.go
  - internal/httpapi/errors.go
  - internal/httpapi/server_test.go
  - internal/httpapi/middleware_test.go
  - internal/httpapi/errors_test.go
  - internal/httpapi/health_test.go
autonomous: true
requirements_addressed: [API-06, API-07, API-08, API-09, API-10, API-11]
gap_closure: false
user_setup: []

must_haves:
  truths:
    - "httpapi.New validates Config (rejects empty AllowedOrigins and '*' + credentials) and returns (*Server, error)"
    - "Middleware chain wraps mux in the order recover(log(cors(mux))) so recover catches panics from CORS/log/handler"
    - "Handler panics on any route are caught, logged, converted to 500 with a JSON envelope, and the server stays alive for the next request"
    - "Every 4xx/5xx response carries Content-Type: application/json and a body shaped {error, details?}"
    - "Route wiring uses Go 1.22+ method-prefixed patterns and wrong method returns 405 automatically"
    - "Placeholder Phase 4 API routes register under the mux returning 501 Not Implemented (handler stubs) so the chain is exercisable end-to-end"
  artifacts:
    - path: "internal/httpapi/server.go"
      provides: "Server struct with store/hub/creds/cfg fields, New(*,*,*,*,*) (*Server, error), Config type, Handler() middleware chain, registerRoutes with stubs"
      contains: "type Config struct"
    - path: "internal/httpapi/middleware.go"
      provides: "recoverMiddleware, logMiddleware, corsMiddleware placeholder, statusCapturingWriter, clientIP"
      contains: "func (s *Server) recoverMiddleware"
    - path: "internal/httpapi/errors.go"
      provides: "writeJSONError + writeBadRequest/Unauthorized/Forbidden/NotFound/Internal helpers"
      contains: "func writeJSONError"
    - path: "internal/httpapi/health_test.go"
      provides: "Phase 1 health tests adapted to newTestServer(t)"
  key_links:
    - from: "internal/httpapi/server.go (Handler)"
      to: "internal/httpapi/middleware.go (recover, log, cors)"
      via: "Server.Handler() wraps s.mux with three middlewares"
      pattern: "recoverMiddleware.*logMiddleware.*corsMiddleware"
    - from: "internal/httpapi/server.go (New)"
      to: "internal/httpapi/server.go (Config validation)"
      via: "New validates cfg.AllowedOrigins emptiness, '*' reject, path.Match probe"
      pattern: "AllowedOrigins.*empty|contains.*\\*"
---

<objective>
Expand the Phase 1 httpapi.Server skeleton into the full Phase 4 transport shell: Config type, validated constructor returning `(*Server, error)`, middleware chain (recover → log → CORS placeholder → mux), error envelope helpers, 501-stub handlers for every Phase 4 route, and the test helper `newTestServer(t)` used by every subsequent plan. This is the scaffolding every later plan (04-02..04-05) layers onto — no auth, no real handlers, no CORS library yet.

Purpose: Lock the constructor signature and middleware shape first so downstream plans slot into stable contracts. API-06, API-07, API-08, API-09, API-10, API-11 all land in this plan.

Output: A compilable httpapi package where `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race` passes with newTestServer-based tests plus the adapted Phase 1 health tests.
</objective>

<execution_context>
@/home/ben/.claude/get-shit-done/workflows/execute-plan.md
@/home/ben/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/STATE.md
@.planning/REQUIREMENTS.md
@.planning/phases/04-http-api/04-CONTEXT.md
@.planning/phases/04-http-api/04-RESEARCH.md
@.planning/phases/04-http-api/04-VALIDATION.md
@.planning/phases/01-foundation/01-CONTEXT.md
@.planning/research/PITFALLS.md
@internal/httpapi/server.go
@internal/httpapi/health.go
@internal/httpapi/health_test.go
@internal/registry/store.go
@internal/wshub/hub.go

<interfaces>
<!-- Stable Phase 2/3 contracts this plan embeds. Do NOT modify. -->

From internal/registry/store.go:
```go
type Store struct { /* ... */ }
func NewStore(path string) (*Store, error)
func (s *Store) Get(id string) (Manifest, bool)
func (s *Store) List() []Manifest
func (s *Store) Upsert(m Manifest) error
func (s *Store) Delete(id string) (bool, error)
func (s *Store) Capabilities(filter CapabilityFilter) []CapabilityView
```

From internal/wshub/hub.go:
```go
type Options struct {
    MessageBuffer int
    PingInterval  time.Duration
    WriteTimeout  time.Duration
    PingTimeout   time.Duration
}
func New(logger *slog.Logger, opts Options) *Hub
func (h *Hub) Publish(msg []byte)
func (h *Hub) Close()
func (h *Hub) Subscribe(ctx context.Context, conn *websocket.Conn) error
```

Phase 1 Server (to be replaced by this plan):
```go
// current: func New(logger *slog.Logger) *Server
// new:     func New(logger *slog.Logger, store *registry.Store, hub *wshub.Hub, creds Credentials, cfg Config) (*Server, error)
```

**Note:** `Credentials` is a type this plan declares as a stub (empty struct with unexported `users` field) so the New signature compiles. Plan 04-02 fills in `LoadCredentials`, `Lookup`, etc. This plan ships an empty Credentials literal in the test helper.
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: RED — write failing tests for Server.New validation, middleware chain, recover, error envelope, method-not-allowed, and adapt health_test.go</name>
  <files>
    internal/httpapi/server_test.go (create),
    internal/httpapi/middleware_test.go (create),
    internal/httpapi/errors_test.go (create),
    internal/httpapi/health_test.go (modify)
  </files>
  <read_first>
    .planning/phases/04-http-api/04-CONTEXT.md,
    .planning/phases/04-http-api/04-RESEARCH.md,
    .planning/phases/04-http-api/04-VALIDATION.md,
    .planning/phases/01-foundation/01-CONTEXT.md,
    internal/httpapi/server.go,
    internal/httpapi/health.go,
    internal/httpapi/health_test.go
  </read_first>
  <behavior>
    - TestServer_New_RejectsEmptyAllowList: New with cfg.AllowedOrigins == nil returns error containing "AllowedOrigins is empty"
    - TestServer_New_RejectsWildcardWithCredentials: New with cfg.AllowedOrigins == []string{"*"} returns error containing `"*"` AND "AllowCredentials"
    - TestServer_New_RejectsBadPattern: New with cfg.AllowedOrigins == []string{"[invalid"} returns error wrapping path.ErrBadPattern
    - TestServer_New_PanicsOnNilDeps: calling New with nil logger, nil store, or nil hub panics (require.Panics per-subtest)
    - TestServer_New_Valid: New(logger, store, hub, Credentials{}, Config{AllowedOrigins:[]string{"https://allowed.example"}, WSPingInterval: 30*time.Second}) returns (srv, nil); srv.Handler() != nil
    - TestServer_MethodNotAllowed: POST /health via the wrapped Handler returns 405 (method-prefixed pattern auto-rejects)
    - TestMiddleware_ChainOrder: hit a route whose handler writes a marker header; assert recover→log→cors→handler order by observing that a panic in the CORS stage is caught by recover (register a test-only middleware at the right layer using a custom Server, OR simpler: panic from the registered route handler, assert recover catches it — this is the outer-most-is-recover property)
    - TestRecover_PanicCaught: mount a panicking test route, hit it via the Server.Handler() chain, assert response is 500 + envelope `{"error":"internal server error"}`, assert a subsequent request to /health still returns 200 (server survived)
    - TestLogMiddleware_SkipsHealth: capture logger output via slog.NewTextHandler(&syncBuffer, nil); GET /health; assert buffer.String() does NOT contain `"httpapi: request"` (health is skipped by the log middleware)
    - TestLogMiddleware_LogsOtherRoute: capture logger output; GET /api/v1/registry (stub returns 501); assert buffer contains `"httpapi: request"` AND `"path"` AND `"/api/v1/registry"` AND `"status"` AND `501` AND `"duration_ms"` AND `"remote"`
    - TestErrors_Envelope: call writeBadRequest(rr, "bad input", map[string]any{"field":"name"}); assert rr.Code == 400, Content-Type == application/json, body unmarshals to {Error:"bad input", Details:{field:"name"}}
    - TestErrors_Envelope_NoDetails: call writeNotFound(rr, "not found"); assert body has NO `"details"` key (omitempty)
    - TestWriteUnauthorized_Header: writeUnauthorized(rr); assert rr.Code == 401 AND rr.Header().Get("WWW-Authenticate") == `Basic realm="openburo"`
    - Add a syncBuffer helper (mutex-guarded bytes.Buffer) per Phase 3 precedent because the log middleware writes from a request-handling goroutine while the test reads from the test goroutine

    Adapt internal/httpapi/health_test.go:
    - Delete the `srv := New(logger)` calls (they no longer compile — New now takes 5 args and returns (*Server, error))
    - Replace with `srv := newTestServer(t)` (the unexported helper defined in server_test.go)
    - Keep the existing assertions unchanged
  </behavior>
  <action>
    Create `internal/httpapi/server_test.go` with the `newTestServer(t *testing.T) *Server` helper per CONTEXT.md §"Test construction helper":

    ```go
    package httpapi

    import (
        "bytes"
        "io"
        "log/slog"
        "path/filepath"
        "sync"
        "testing"
        "time"

        "github.com/openburo/openburo-server/internal/registry"
        "github.com/openburo/openburo-server/internal/wshub"
        "github.com/stretchr/testify/require"
    )

    // syncBuffer wraps bytes.Buffer with a mutex so the log middleware's
    // write goroutine does not race the test's read goroutine. Mirrors
    // the Phase 3 pattern from wshub/subscribe_test.go.
    type syncBuffer struct {
        mu  sync.Mutex
        buf bytes.Buffer
    }

    func (b *syncBuffer) Write(p []byte) (int, error) {
        b.mu.Lock()
        defer b.mu.Unlock()
        return b.buf.Write(p)
    }

    func (b *syncBuffer) String() string {
        b.mu.Lock()
        defer b.mu.Unlock()
        return b.buf.String()
    }

    // newTestServer constructs a Server backed by a temp-dir registry store,
    // a short-ping wshub.Hub, an empty credential table, and a discard logger.
    // Callers that need captured logs should override srv.logger themselves.
    // Uses t.Cleanup to shut the hub down automatically.
    func newTestServer(t *testing.T) *Server {
        t.Helper()
        logger := slog.New(slog.NewTextHandler(io.Discard, nil))
        return newTestServerWithLogger(t, logger)
    }

    func newTestServerWithLogger(t *testing.T, logger *slog.Logger) *Server {
        t.Helper()
        storePath := filepath.Join(t.TempDir(), "registry.json")
        store, err := registry.NewStore(storePath)
        require.NoError(t, err)
        hub := wshub.New(logger, wshub.Options{
            PingInterval: 50 * time.Millisecond,
        })
        t.Cleanup(func() { hub.Close() })
        srv, err := New(logger, store, hub, Credentials{}, Config{
            AllowedOrigins: []string{"https://allowed.example"},
            WSPingInterval: 30 * time.Second,
        })
        require.NoError(t, err)
        return srv
    }
    ```

    Then write the constructor-validation tests in the same file:

    ```go
    func TestServer_New_RejectsEmptyAllowList(t *testing.T) {
        logger := slog.New(slog.NewTextHandler(io.Discard, nil))
        storePath := filepath.Join(t.TempDir(), "registry.json")
        store, err := registry.NewStore(storePath)
        require.NoError(t, err)
        hub := wshub.New(logger, wshub.Options{PingInterval: 50 * time.Millisecond})
        defer hub.Close()

        _, err = New(logger, store, hub, Credentials{}, Config{
            AllowedOrigins: nil,
            WSPingInterval: 30 * time.Second,
        })
        require.Error(t, err)
        require.Contains(t, err.Error(), "AllowedOrigins is empty")
    }

    func TestServer_New_RejectsWildcardWithCredentials(t *testing.T) {
        // ... same setup ...
        _, err = New(logger, store, hub, Credentials{}, Config{
            AllowedOrigins: []string{"*"},
            WSPingInterval: 30 * time.Second,
        })
        require.Error(t, err)
        require.Contains(t, err.Error(), `"*"`)
        require.Contains(t, err.Error(), "AllowCredentials")
    }

    func TestServer_New_RejectsBadPattern(t *testing.T) {
        // ... same setup ...
        _, err = New(logger, store, hub, Credentials{}, Config{
            AllowedOrigins: []string{"[invalid"},
            WSPingInterval: 30 * time.Second,
        })
        require.Error(t, err)
        // path.Match returns ErrBadPattern wrapped by our error
    }

    func TestServer_New_PanicsOnNilDeps(t *testing.T) {
        logger := slog.New(slog.NewTextHandler(io.Discard, nil))
        store, _ := registry.NewStore(filepath.Join(t.TempDir(), "r.json"))
        hub := wshub.New(logger, wshub.Options{PingInterval: 50 * time.Millisecond})
        defer hub.Close()
        cfg := Config{AllowedOrigins: []string{"https://allowed.example"}, WSPingInterval: 30 * time.Second}
        t.Run("nil logger", func(t *testing.T) {
            require.Panics(t, func() { _, _ = New(nil, store, hub, Credentials{}, cfg) })
        })
        t.Run("nil store", func(t *testing.T) {
            require.Panics(t, func() { _, _ = New(logger, nil, hub, Credentials{}, cfg) })
        })
        t.Run("nil hub", func(t *testing.T) {
            require.Panics(t, func() { _, _ = New(logger, store, nil, Credentials{}, cfg) })
        })
    }

    func TestServer_MethodNotAllowed(t *testing.T) {
        srv := newTestServer(t)
        req := httptest.NewRequest(http.MethodPost, "/health", nil)
        rr := httptest.NewRecorder()
        srv.Handler().ServeHTTP(rr, req)
        require.Equal(t, http.StatusMethodNotAllowed, rr.Code)
    }
    ```

    Create `internal/httpapi/middleware_test.go` with:

    ```go
    package httpapi

    import (
        "log/slog"
        "net/http"
        "net/http/httptest"
        "strings"
        "testing"

        "github.com/stretchr/testify/require"
    )

    func TestRecover_PanicCaught(t *testing.T) {
        srv := newTestServer(t)
        // Register a panicking handler on the mux directly (test-only).
        srv.mux.HandleFunc("GET /panic", func(w http.ResponseWriter, r *http.Request) {
            panic("boom")
        })

        rr := httptest.NewRecorder()
        req := httptest.NewRequest(http.MethodGet, "/panic", nil)
        srv.Handler().ServeHTTP(rr, req)
        require.Equal(t, http.StatusInternalServerError, rr.Code)
        require.Equal(t, "application/json", rr.Header().Get("Content-Type"))
        require.Contains(t, rr.Body.String(), `"error":"internal server error"`)

        // Server survives: next request works
        rr2 := httptest.NewRecorder()
        req2 := httptest.NewRequest(http.MethodGet, "/health", nil)
        srv.Handler().ServeHTTP(rr2, req2)
        require.Equal(t, http.StatusOK, rr2.Code)
    }

    func TestLogMiddleware_SkipsHealth(t *testing.T) {
        buf := &syncBuffer{}
        logger := slog.New(slog.NewTextHandler(buf, nil))
        srv := newTestServerWithLogger(t, logger)

        req := httptest.NewRequest(http.MethodGet, "/health", nil)
        srv.Handler().ServeHTTP(httptest.NewRecorder(), req)
        require.NotContains(t, buf.String(), "httpapi: request")
    }

    func TestLogMiddleware_LogsOtherRoute(t *testing.T) {
        buf := &syncBuffer{}
        logger := slog.New(slog.NewTextHandler(buf, nil))
        srv := newTestServerWithLogger(t, logger)

        req := httptest.NewRequest(http.MethodGet, "/api/v1/registry", nil)
        srv.Handler().ServeHTTP(httptest.NewRecorder(), req)
        out := buf.String()
        require.Contains(t, out, "httpapi: request")
        require.Contains(t, out, "path=/api/v1/registry")
        require.Contains(t, out, "status=501")
        require.Contains(t, out, "duration_ms=")
        require.Contains(t, out, "remote=")
    }

    func TestMiddleware_ChainOrder(t *testing.T) {
        // Register a panicking handler at the mux level. The panic must be
        // caught by recover (outermost), which proves recover wraps log+cors
        // (i.e. recover is on the outside). We assert 500 + envelope, which
        // only happens if recover is correctly the outermost wrapper.
        srv := newTestServer(t)
        srv.mux.HandleFunc("GET /chain-panic", func(w http.ResponseWriter, r *http.Request) {
            panic("from handler, should be caught by recover at top")
        })
        rr := httptest.NewRecorder()
        req := httptest.NewRequest(http.MethodGet, "/chain-panic", nil)
        req.Header.Set("Origin", "https://allowed.example")
        srv.Handler().ServeHTTP(rr, req)
        require.Equal(t, http.StatusInternalServerError, rr.Code)
        require.True(t, strings.HasPrefix(rr.Header().Get("Content-Type"), "application/json"))
    }
    ```

    Create `internal/httpapi/errors_test.go` with:

    ```go
    package httpapi

    import (
        "encoding/json"
        "net/http"
        "net/http/httptest"
        "testing"

        "github.com/stretchr/testify/require"
    )

    func TestErrors_Envelope(t *testing.T) {
        rr := httptest.NewRecorder()
        writeBadRequest(rr, "bad input", map[string]any{"field": "name"})
        require.Equal(t, http.StatusBadRequest, rr.Code)
        require.Equal(t, "application/json", rr.Header().Get("Content-Type"))

        var body struct {
            Error   string         `json:"error"`
            Details map[string]any `json:"details"`
        }
        require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
        require.Equal(t, "bad input", body.Error)
        require.Equal(t, "name", body.Details["field"])
    }

    func TestErrors_Envelope_NoDetails(t *testing.T) {
        rr := httptest.NewRecorder()
        writeNotFound(rr, "not found")
        require.Equal(t, http.StatusNotFound, rr.Code)
        // omitempty: no "details" key in the body at all
        require.NotContains(t, rr.Body.String(), `"details"`)
        require.Contains(t, rr.Body.String(), `"error":"not found"`)
    }

    func TestWriteUnauthorized_Header(t *testing.T) {
        rr := httptest.NewRecorder()
        writeUnauthorized(rr)
        require.Equal(t, http.StatusUnauthorized, rr.Code)
        require.Equal(t, `Basic realm="openburo"`, rr.Header().Get("WWW-Authenticate"))
    }
    ```

    Modify `internal/httpapi/health_test.go`:
    - Delete the `logger := slog.New(...)` and `srv := New(logger)` lines in both TestHealth and TestHealth_RejectsWrongMethod
    - Replace with a single `srv := newTestServer(t)` call
    - Remove the `io` and `log/slog` imports if no longer used
    - Keep every assertion exactly as-is

    Run tests — they MUST fail to compile because Credentials, Config, New signature, writeBadRequest, writeNotFound, writeUnauthorized, syncBuffer usage, registerRoutes stubs, and middleware methods don't exist yet. That IS the RED state.

    Commit: `test(04-01): add failing tests for Server.New validation + middleware chain + error envelope`
  </action>
  <verify>
    <automated>cd /home/ben/Dev-local/openburo-spec/open-buro-server &amp;&amp; ~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -timeout 30s 2>&amp;1 | head -40 ; echo "EXPECT: compile errors (RED state)"</automated>
  </verify>
  <acceptance_criteria>
    - Files exist: `test -f internal/httpapi/server_test.go && test -f internal/httpapi/middleware_test.go && test -f internal/httpapi/errors_test.go`
    - syncBuffer declared once: `grep -c "type syncBuffer struct" internal/httpapi/server_test.go → 1`
    - newTestServer helper exists: `grep -c "func newTestServer" internal/httpapi/server_test.go → ≥1`
    - Constructor tests reference the 5-arg New: `grep -c 'New(logger, store, hub, Credentials{}, Config{' internal/httpapi/server_test.go → ≥3`
    - health_test.go no longer calls `New(logger)`: `! grep -n 'srv := New(logger)$' internal/httpapi/health_test.go`
    - health_test.go uses the helper: `grep -c 'newTestServer(t)' internal/httpapi/health_test.go → ≥2`
    - Test file fails to compile (RED state proof): `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -timeout 30s 2>&1 | grep -E 'undefined|undeclared' → ≥1 line`
    - gofmt clean on new files: `~/sdk/go1.26.2/bin/gofmt -l internal/httpapi/server_test.go internal/httpapi/middleware_test.go internal/httpapi/errors_test.go internal/httpapi/health_test.go` → empty
  </acceptance_criteria>
  <done>RED committed: failing tests for constructor validation, middleware chain order, recover, log-skips-health, error envelope, and the newTestServer helper are all on disk and the test file fails to compile because the production code doesn't exist yet.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: GREEN — implement Server + Config + middleware chain + errors.go + 501 route stubs to make Task 1 tests pass</name>
  <files>
    internal/httpapi/server.go (modify — replace Phase 1 version),
    internal/httpapi/middleware.go (create),
    internal/httpapi/errors.go (create)
  </files>
  <read_first>
    .planning/phases/04-http-api/04-CONTEXT.md,
    .planning/phases/04-http-api/04-RESEARCH.md,
    .planning/phases/04-http-api/04-VALIDATION.md,
    .planning/research/PITFALLS.md,
    internal/httpapi/server.go,
    internal/httpapi/health.go,
    internal/httpapi/server_test.go,
    internal/httpapi/middleware_test.go,
    internal/httpapi/errors_test.go,
    internal/registry/store.go,
    internal/wshub/hub.go
  </read_first>
  <action>
    Replace the body of `internal/httpapi/server.go` with the Phase 4 version:

    ```go
    // Package httpapi owns the HTTP routing layer of the OpenBuro server.
    // It is the SOLE package where internal/registry and internal/wshub meet:
    // registry state lives here behind HTTP handlers, and every mutation
    // broadcasts via the hub from the handler layer (not from inside registry).
    // This enforces the unidirectional dependency graph that prevents the
    // registry<->hub ABBA deadlock (see .planning/research/PITFALLS.md #1).
    package httpapi

    import (
        "errors"
        "fmt"
        "log/slog"
        "net/http"
        "path"
        "time"

        "github.com/openburo/openburo-server/internal/registry"
        "github.com/openburo/openburo-server/internal/wshub"
    )

    // Config carries the subset of config.yaml that the handler layer needs.
    // The compose-root (Phase 5 cmd/server/main.go) translates config.Config
    // into this struct so internal/httpapi does NOT import internal/config.
    type Config struct {
        // AllowedOrigins is the CORS + WebSocket OriginPatterns allow-list.
        // Must be non-empty; must not contain "*" because AllowCredentials=true.
        AllowedOrigins []string

        // WSPingInterval is exposed for the compose-root to pass through to
        // wshub.New. The Server itself does not use this field; it is kept
        // on Config so the hub and the cors allow-list come from one struct.
        WSPingInterval time.Duration
    }

    // Credentials is a stub declared here so Server.New compiles. The full
    // type (with users map, LoadCredentials, Lookup) ships in Plan 04-02.
    // An empty Credentials literal is legal and causes every authenticated
    // request to return 401 (see Plan 04-02's authBasic middleware).
    type Credentials struct {
        users map[string][]byte
    }

    // Server owns the HTTP routing, middleware chain, and handler implementations.
    // Dependencies (store, hub, credentials, logger, config) are injected by the
    // compose-root — internal packages NEVER grab global state.
    type Server struct {
        logger *slog.Logger
        store  *registry.Store
        hub    *wshub.Hub
        creds  Credentials
        cfg    Config
        mux    *http.ServeMux
    }

    // New constructs a Server with all domain and transport dependencies.
    //
    // Returns (*Server, error) — NOT *Server alone — because the Phase 4
    // research revealed that rs/cors v1.11.1 does NOT reject the
    // `AllowedOrigins: ["*"] + AllowCredentials: true` combination at
    // construction time. This constructor performs that validation itself
    // and returns a clear error if the operator misconfigures the allow-list.
    //
    // Nil logger/store/hub panic at construction (programmer error, not
    // operator error — there is no recovery path).
    func New(logger *slog.Logger, store *registry.Store, hub *wshub.Hub, creds Credentials, cfg Config) (*Server, error) {
        if logger == nil {
            panic("httpapi.New: logger is nil")
        }
        if store == nil {
            panic("httpapi.New: store is nil")
        }
        if hub == nil {
            panic("httpapi.New: hub is nil")
        }
        if len(cfg.AllowedOrigins) == 0 {
            return nil, errors.New("httpapi: cfg.AllowedOrigins is empty (no CORS allow-list; WebSocket handshakes would be rejected)")
        }
        for _, pattern := range cfg.AllowedOrigins {
            if pattern == "*" {
                return nil, errors.New(`httpapi: cfg.AllowedOrigins contains "*" which is incompatible with AllowCredentials=true (PITFALLS #9)`)
            }
            // Probe that the pattern is valid path.Match syntax. coder/websocket
            // uses path.Match for OriginPatterns; rs/cors has no equivalent check
            // but an invalid glob here would silently fail-open on the WS side.
            if _, err := path.Match(pattern, "probe"); err != nil {
                return nil, fmt.Errorf("httpapi: cfg.AllowedOrigins pattern %q: %w", pattern, err)
            }
        }

        s := &Server{
            logger: logger,
            store:  store,
            hub:    hub,
            creds:  creds,
            cfg:    cfg,
            mux:    http.NewServeMux(),
        }
        s.registerRoutes()
        return s, nil
    }

    // Handler returns the root http.Handler wrapped in the middleware chain.
    // Order (outermost first — the request hits the outermost wrapper first):
    //   recover -> log -> cors -> mux -> (per-route auth) -> handler
    // Recover is outermost so it catches panics from log, cors, or any inner
    // middleware/handler. Per-route auth is attached inside registerRoutes.
    func (s *Server) Handler() http.Handler {
        var h http.Handler = s.mux
        h = s.corsMiddleware(h)     // innermost wrap (closest to mux)
        h = s.logMiddleware(h)      // wraps cors
        h = s.recoverMiddleware(h)  // OUTERMOST — catches panics from all inner layers
        return h
    }

    // stub501 is the Phase 4 placeholder handler. Every Phase 4 route that
    // is not wired yet (auth, registry handlers, caps, ws) is registered with
    // this stub, returning 501 Not Implemented with the envelope shape so
    // integration tests exercise the full middleware chain from day one.
    // Later plans (04-02..04-05) replace each stub with the real handler.
    func (s *Server) stub501(w http.ResponseWriter, r *http.Request) {
        writeJSONError(w, http.StatusNotImplemented, "not implemented", nil)
    }

    // registerRoutes wires every route Phase 4 is responsible for. Plans
    // 04-02..04-04 replace the stub501 calls with real handlers in-place.
    func (s *Server) registerRoutes() {
        // Phase 1 route (unchanged)
        s.mux.HandleFunc("GET /health", s.handleHealth)

        // Phase 4 write routes (auth added in plan 04-02; 501 stubs for now)
        s.mux.HandleFunc("POST /api/v1/registry", s.stub501)
        s.mux.HandleFunc("DELETE /api/v1/registry/{appId}", s.stub501)

        // Phase 4 read routes (public)
        s.mux.HandleFunc("GET /api/v1/registry", s.stub501)
        s.mux.HandleFunc("GET /api/v1/registry/{appId}", s.stub501)
        s.mux.HandleFunc("GET /api/v1/capabilities", s.stub501)
        s.mux.HandleFunc("GET /api/v1/capabilities/ws", s.stub501)
    }
    ```

    Create `internal/httpapi/errors.go` exactly as spec'd in CONTEXT.md:

    ```go
    package httpapi

    import (
        "encoding/json"
        "net/http"
    )

    // writeJSONError writes a JSON error response with the given status code
    // and message. details is optional (pass nil for none). Content-Type is
    // set to application/json. This is the single source of error-envelope
    // truth — all 4xx/5xx handlers MUST funnel through here or the shortcuts
    // below for consistency (API-09).
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

    // Shortcuts for each status code the handler layer uses.
    func writeBadRequest(w http.ResponseWriter, msg string, details map[string]any) {
        writeJSONError(w, http.StatusBadRequest, msg, details)
    }

    func writeUnauthorized(w http.ResponseWriter) {
        // WWW-Authenticate prompts browsers and CLI tools for credentials.
        w.Header().Set("WWW-Authenticate", `Basic realm="openburo"`)
        writeJSONError(w, http.StatusUnauthorized, "unauthorized", nil)
    }

    func writeForbidden(w http.ResponseWriter, msg string) {
        writeJSONError(w, http.StatusForbidden, msg, nil)
    }

    func writeNotFound(w http.ResponseWriter, msg string) {
        writeJSONError(w, http.StatusNotFound, msg, nil)
    }

    func writeInternal(w http.ResponseWriter, msg string) {
        writeJSONError(w, http.StatusInternalServerError, msg, nil)
    }
    ```

    Create `internal/httpapi/middleware.go`:

    ```go
    package httpapi

    import (
        "fmt"
        "net/http"
        "runtime/debug"
        "strings"
        "time"
    )

    // statusCapturingWriter is a tiny wrapper around http.ResponseWriter that
    // captures the status code so logMiddleware can log it. Handlers that call
    // WriteHeader multiple times are caught by the first call only (same as
    // stdlib behavior).
    type statusCapturingWriter struct {
        http.ResponseWriter
        status int
    }

    func (w *statusCapturingWriter) WriteHeader(code int) {
        w.status = code
        w.ResponseWriter.WriteHeader(code)
    }

    // recoverMiddleware is the OUTERMOST middleware. It catches panics from
    // any inner middleware or handler, logs them (with stack), emits a 500
    // envelope, and returns — the server stays alive for the next request.
    // API-08 anchor.
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

    // logMiddleware logs every non-/health request with structured fields.
    // /health is skipped explicitly (it's the noisiest route and clutters
    // logs — inherited from Phase 1's "never log health" convention).
    //
    // Deliberately NO `user` field here — that's the audit log's job
    // (Plan 04-03 OPS-06) and the request log must not imply user identity
    // for public read routes.
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

    // corsMiddleware is a Plan 04-01 PLACEHOLDER pass-through. Plan 04-05
    // replaces this with the real rs/cors wrap driven by s.cfg.AllowedOrigins.
    // It is declared here so the middleware chain order is locked from day one.
    func (s *Server) corsMiddleware(next http.Handler) http.Handler {
        return next
    }

    // clientIP returns the client's IP for logging. Respects X-Forwarded-For
    // (first entry) when present, falls back to r.RemoteAddr. Reference-impl
    // only — a hardened prod service would need a trusted-proxy allow-list.
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

    Run the full httpapi suite. Everything from Task 1 should now pass.

    Run `~/sdk/go1.26.2/bin/go vet ./internal/httpapi/...` and `~/sdk/go1.26.2/bin/gofmt -l internal/httpapi/` — both clean.

    Run the architectural gate: `! grep -rE 'slog\.Default' internal/httpapi/*.go | grep -v _test.go` — empty.

    Run the isolation gate: `! ~/sdk/go1.26.2/bin/go list -deps ./internal/registry | grep -E 'wshub|httpapi'` — empty (still passes because registry still imports neither).

    Commit: `feat(04-01): implement Server + Config + middleware chain + error envelope + 501 stubs`
  </action>
  <verify>
    <automated>cd /home/ben/Dev-local/openburo-spec/open-buro-server &amp;&amp; ~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -timeout 60s &amp;&amp; ~/sdk/go1.26.2/bin/go vet ./internal/httpapi/... &amp;&amp; test -z "$(~/sdk/go1.26.2/bin/gofmt -l internal/httpapi/)"</automated>
  </verify>
  <acceptance_criteria>
    - Files exist: `test -f internal/httpapi/server.go && test -f internal/httpapi/middleware.go && test -f internal/httpapi/errors.go`
    - `grep -c "func New(logger \*slog.Logger, store \*registry.Store, hub \*wshub.Hub, creds Credentials, cfg Config) (\*Server, error)" internal/httpapi/server.go → 1`
    - Server struct has all 6 fields: `grep -c "store  \*registry.Store" internal/httpapi/server.go → 1` AND `grep -c "hub    \*wshub.Hub" internal/httpapi/server.go → 1` AND `grep -c "creds  Credentials" internal/httpapi/server.go → 1` AND `grep -c "cfg    Config" internal/httpapi/server.go → 1`
    - Middleware chain order is recover-outermost: `grep -A3 "func (s \*Server) Handler()" internal/httpapi/server.go | grep -c "s.recoverMiddleware(h)" → 1` AND that line appears LAST in the Handler body (verified by the TestMiddleware_ChainOrder test passing)
    - Error envelope helpers present: `grep -c "^func writeJSONError" internal/httpapi/errors.go → 1` AND `grep -c "^func writeBadRequest" internal/httpapi/errors.go → 1` AND `grep -c "^func writeUnauthorized" internal/httpapi/errors.go → 1` AND `grep -c "^func writeNotFound" internal/httpapi/errors.go → 1` AND `grep -c "^func writeInternal" internal/httpapi/errors.go → 1` AND `grep -c "^func writeForbidden" internal/httpapi/errors.go → 1`
    - WWW-Authenticate header set: `grep -c 'Basic realm="openburo"' internal/httpapi/errors.go → 1`
    - 6 Phase 4 routes registered: `grep -c "s.mux.HandleFunc" internal/httpapi/server.go → ≥7` (health + 6 new)
    - Method-prefixed patterns used exclusively: `grep -E "s.mux.HandleFunc\(\"(GET|POST|DELETE) " internal/httpapi/server.go | wc -l → ≥7`
    - recoverMiddleware emits "httpapi: handler panic": `grep -c '"httpapi: handler panic"' internal/httpapi/middleware.go → 1`
    - logMiddleware emits "httpapi: request": `grep -c '"httpapi: request"' internal/httpapi/middleware.go → 1`
    - logMiddleware skips /health: `grep -c 'r.URL.Path == "/health"' internal/httpapi/middleware.go → 1`
    - clientIP respects X-Forwarded-For: `grep -c "X-Forwarded-For" internal/httpapi/middleware.go → 1`
    - No slog.Default in production: `! grep -rE 'slog\.Default' internal/httpapi/*.go | grep -v _test.go` exits 0
    - No InsecureSkipVerify anywhere: `! grep -rn 'InsecureSkipVerify' internal/httpapi/*.go` exits 0
    - No internal/config import: `! grep -rn '"github.com/openburo/openburo-server/internal/config"' internal/httpapi/*.go` exits 0
    - Architectural isolation holds: `! ~/sdk/go1.26.2/bin/go list -deps ./internal/registry | grep -E 'wshub|httpapi'` exits 0
    - Full suite green: `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -timeout 60s` exits 0
    - go vet clean: `~/sdk/go1.26.2/bin/go vet ./internal/httpapi/...` exits 0
    - gofmt clean: `test -z "$(~/sdk/go1.26.2/bin/gofmt -l internal/httpapi/)"` exits 0
  </acceptance_criteria>
  <done>GREEN: all Task 1 tests pass. Server struct is at the Phase 4 shape, Handler() returns the middleware chain in recover→log→cors→mux order, Config validation rejects empty/wildcard/bad-pattern allow-lists, error envelope helpers exist and set the right headers, all 6 Phase 4 routes register with stub501 placeholders, health_test.go still passes, architectural gates all pass, go vet and gofmt clean.</done>
</task>

</tasks>

<verification>
Full plan verification — run from the repo root:

```bash
# 1. Full httpapi suite race-clean
~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -count=1 -timeout 60s

# 2. Architectural gates
! ~/sdk/go1.26.2/bin/go list -deps ./internal/registry | grep -E 'wshub|httpapi'
! ~/sdk/go1.26.2/bin/go list -deps ./internal/wshub | grep -E 'registry|httpapi'
! grep -rE 'slog\.Default' internal/httpapi/*.go | grep -v _test.go
! grep -rn 'InsecureSkipVerify' internal/httpapi/*.go
! grep -rn '"github.com/openburo/openburo-server/internal/config"' internal/httpapi/*.go

# 3. Format + vet
~/sdk/go1.26.2/bin/go vet ./internal/httpapi/...
test -z "$(~/sdk/go1.26.2/bin/gofmt -l internal/httpapi/)"

# 4. Named tests from 04-VALIDATION.md that this plan owns
~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestServer_New_' -timeout 10s
~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestServer_MethodNotAllowed' -timeout 10s
~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestMiddleware_ChainOrder' -timeout 10s
~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestRecover_PanicCaught' -timeout 10s
~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestErrors_Envelope' -timeout 10s
~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestLogMiddleware' -timeout 10s
~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestHealth' -timeout 10s  # adapted Phase 1 tests still pass
```
</verification>

<success_criteria>
- Server.New signature is `(*slog.Logger, *registry.Store, *wshub.Hub, Credentials, Config) (*Server, error)` — stable for all downstream plans
- Server.Handler() returns `recover(log(cors(mux)))` and recover catches panics from ANY layer
- Config validation surfaces clear errors for `[]`, `["*"]`, and `["[invalid"]` allow-lists
- Error envelope is `{"error": "...", "details": {...}}` with `omitempty` details and Content-Type application/json
- 401 responses set `WWW-Authenticate: Basic realm="openburo"`
- Phase 1 health tests still pass via the adapted newTestServer(t) helper
- 6 Phase 4 routes are registered with stub501 placeholders (501 envelope responses) so downstream plans replace handlers in place
- All architectural gates pass: no slog.Default, no InsecureSkipVerify, no internal/config import, registry-isolation intact
- Full suite green under -race
</success_criteria>

<output>
After completion, create `.planning/phases/04-http-api/04-01-SUMMARY.md` mirroring the Phase 2/3 SUMMARY conventions (decisions locked, files touched, test counts, gate results, notes for plan 04-02).
</output>
