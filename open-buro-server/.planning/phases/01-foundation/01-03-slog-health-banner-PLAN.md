---
phase: 01-foundation
plan: 03
type: execute
wave: 3
depends_on:
  - 01-01
  - 01-02
files_modified:
  - internal/httpapi/server.go
  - internal/httpapi/health.go
  - internal/httpapi/health_test.go
  - internal/httpapi/.gitkeep
  - cmd/server/main.go
autonomous: true
requirements:
  - FOUND-03
  - FOUND-04
  - FOUND-05
must_haves:
  truths:
    - "The binary starts when given a valid `config.yaml` path via `-config`"
    - "`GET /health` returns 200 with `Content-Type: application/json` and body `{\"status\":\"ok\"}` — no authentication required"
    - "`POST /health` (and other non-GET methods) returns 405 Method Not Allowed"
    - "One `slog.Info(\"openburo server starting\", ...)` line is emitted at startup with all 10 required keys in the locked order"
    - "A `*slog.Logger` constructed from `logging.format` (json/text) and `logging.level` (debug/info/warn/error) is injected into `httpapi.New`"
    - "No `slog.Default()` call or bare `slog.Info/Debug/Warn/Error` anywhere inside `internal/` packages"
  artifacts:
    - path: "internal/httpapi/server.go"
      provides: "Server struct (logger, mux) + New(logger) constructor + Handler() + registerRoutes()"
      contains: "type Server struct"
    - path: "internal/httpapi/health.go"
      provides: "handleHealth method — GET /health returns 200 JSON"
      contains: "handleHealth"
    - path: "internal/httpapi/health_test.go"
      provides: "TestHealth (200+content-type+body) + TestHealth_RejectsWrongMethod"
      contains: "TestHealth"
    - path: "cmd/server/main.go"
      provides: "Compose-root: flag parsing, config.Load, newLogger, startup banner, httpapi.New, http.Server.ListenAndServe"
      contains: "func run() error"
  key_links:
    - from: "cmd/server/main.go"
      to: "internal/config.Load"
      via: "config.Load(*configPath)"
      pattern: "config\\.Load"
    - from: "cmd/server/main.go"
      to: "internal/httpapi.New"
      via: "httpapi.New(logger)"
      pattern: "httpapi\\.New"
    - from: "cmd/server/main.go"
      to: "internal/version.Version"
      via: "version.Version in banner"
      pattern: "version\\.Version"
    - from: "internal/httpapi/server.go"
      to: "http.ServeMux"
      via: "s.mux.HandleFunc(\"GET /health\", s.handleHealth)"
      pattern: "GET /health"
---

<objective>
Ship the minimum-viable running server: `internal/httpapi.Server` with a `/health` route, the slog construction helper in `cmd/server/main.go`, the startup banner emitting all 10 locked keys, and the compose-root wiring that loads config, builds the logger, constructs the Server, and calls `ListenAndServe`. This plan closes Phase 1 — after it lands, `make run` starts a working binary that serves `GET /health`.

Purpose: Establish the injection-first logging discipline, the ServeMux method-pattern convention, and the compose-root shape that Phases 2-5 will extend. No global loggers, no framework, no premature abstraction.
Output: A working single-binary server with one route, one structured log line at startup, and unit-test coverage for `/health`.

**Pitfall-awareness:** `slog.Default()` is forbidden anywhere inside `internal/` (PITFALLS §Credentials in logs lineage + RESEARCH §Pitfall 1). The only `slog.*` call outside an injected-logger method receiver is the startup banner in `main.go`. The `/health` handler does NOT log — health endpoints are noisy and logging them pollutes logs (RESEARCH §Pitfall 2).
</objective>

<execution_context>
@.planning/phases/01-foundation/01-CONTEXT.md
@.planning/phases/01-foundation/01-RESEARCH.md
@.planning/phases/01-foundation/01-VALIDATION.md
</execution_context>

<context>
@.planning/REQUIREMENTS.md
@.planning/research/PITFALLS.md

<interfaces>
<!-- Contracts this plan consumes from Plan 01-02 and 01-01. -->

From internal/config/config.go (created in Plan 01-02):
```go
package config

type Config struct {
    Server          ServerConfig
    CredentialsFile string
    RegistryFile    string
    WebSocket       WebSocketConfig
    Logging         LoggingConfig
    CORS            CORSConfig
}
type ServerConfig struct {
    Port int
    TLS  TLSConfig
}
func (s ServerConfig) Addr() string // returns ":8080" form
type TLSConfig struct {
    Enabled  bool
    CertFile string
    KeyFile  string
}
type WebSocketConfig struct {
    PingIntervalSeconds int
    PingInterval        time.Duration // populated during Load
}
type LoggingConfig struct {
    Format string // "json" | "text"
    Level  string // "debug" | "info" | "warn" | "error"
}

func Load(path string) (*Config, error)
```

From internal/version/version.go (created in Plan 01-01):
```go
package version

var Version = "dev" // overridden by ldflags
```

<!-- Contracts this plan PRODUCES for Phases 2-5 to extend. -->

From internal/httpapi/server.go (NEW):
```go
package httpapi

type Server struct {
    logger *slog.Logger
    mux    *http.ServeMux
    // Phase 2 adds: store *registry.Store
    // Phase 3 adds: hub *wshub.Hub
    // Phase 4 adds: creds *auth.Credentials
}

func New(logger *slog.Logger) *Server
func (s *Server) Handler() http.Handler  // returns s.mux in Phase 1; Phase 4 wraps with middleware
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Implement internal/httpapi Server scaffold + /health handler + tests</name>
  <files>internal/httpapi/server.go, internal/httpapi/health.go, internal/httpapi/health_test.go</files>
  <read_first>
    - .planning/phases/01-foundation/01-CONTEXT.md (§Package Layout, §Logging Toggle — injection rule)
    - .planning/phases/01-foundation/01-RESEARCH.md (§Pattern 3 httpapi.Server Minimal Scaffold, §Pattern 4 health_test.go Shape, §Pitfall 4 Go 1.22 method patterns, §Pitfall 2 logging r.Header)
    - .planning/phases/01-foundation/01-VALIDATION.md (§Per-Task Verification Map rows for FOUND-04)
    - .planning/research/PITFALLS.md (§13 Credentials in logs — never log r.Header)
    - internal/httpapi/.gitkeep (delete once real files exist)
  </read_first>
  <behavior>
    - TestHealth: GET /health via httptest.NewRecorder returns status 200
    - TestHealth: response Content-Type header equals "application/json"
    - TestHealth: response body contains both `"status"` and `"ok"` substrings
    - TestHealth_RejectsWrongMethod: POST/PUT/DELETE /health each return 405 Method Not Allowed
    - TestHealth: accepts no Authorization header (request is built without one)
    - Logger field is used for construction but handleHealth itself does NOT call s.logger.Info — health routes are intentionally quiet
  </behavior>
  <action>
Write the three files in `internal/httpapi/`. Delete `internal/httpapi/.gitkeep` once the real files exist.

Step 1: Write `internal/httpapi/server.go`:

```go
// Package httpapi owns the HTTP routing layer of the OpenBuro server.
// Phase 1 ships a minimal Server with only /health wired; subsequent
// phases extend it with the registry store, websocket hub, credentials,
// middleware chain, CORS, and the full /api/v1/* route set.
package httpapi

import (
	"log/slog"
	"net/http"
)

// Server owns the HTTP routing and handler implementations for the
// OpenBuro broker. Phase 1 ships a minimal version with /health only;
// subsequent phases will add store, hub, creds fields alongside the
// existing logger and mux.
type Server struct {
	logger *slog.Logger
	mux    *http.ServeMux
}

// New constructs a Server with the given dependencies and registers its routes.
// The *slog.Logger must be constructed by the caller (compose-root) and
// injected here. No internal/ package is permitted to call slog.Default().
func New(logger *slog.Logger) *Server {
	s := &Server{
		logger: logger,
		mux:    http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

// Handler returns the root http.Handler. Phase 1 returns the raw mux;
// Phase 4 will wrap this in the middleware chain (recover → log → CORS → auth).
func (s *Server) Handler() http.Handler {
	return s.mux
}

// registerRoutes wires Phase 1's single route. Future phases add more
// routes here without touching main.go. Always use the Go 1.22+
// method-prefixed pattern ("GET /health", not "/health") so the mux
// rejects wrong methods with 405 instead of silently matching them.
func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /health", s.handleHealth)
}
```

Step 2: Write `internal/httpapi/health.go`:

```go
package httpapi

import (
	"net/http"
)

// handleHealth answers GET /health with 200 and a minimal JSON body.
// No authentication (public per AUTH-03 / FOUND-04).
// Returns application/json to establish the content-type convention
// that future handlers will follow.
//
// Deliberately does NOT log the request: health endpoints are the
// noisiest routes in any HTTP service, and logging them pollutes logs.
// Phase 4's log middleware will skip /health explicitly for the same reason.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
```

Step 3: Write `internal/httpapi/health_test.go`:

```go
package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHealth(t *testing.T) {
	// Use a discard logger so tests don't spew to stderr.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(logger)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	// Critical for FOUND-04: no Authorization header set. The test builds
	// the request without calling req.Header.Set("Authorization", ...).
	rr := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	body, err := io.ReadAll(rr.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), `"status"`)
	require.Contains(t, string(body), `"ok"`)
}

func TestHealth_RejectsWrongMethod(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(logger)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/health", strings.NewReader(""))
			rr := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rr, req)
			require.Equal(t, http.StatusMethodNotAllowed, rr.Code)
		})
	}
}
```

Step 4: Delete `internal/httpapi/.gitkeep` (the three real files now keep the directory tracked).

Step 5: Run tests and verify:

```bash
go test ./internal/httpapi -race -count=1 -v
go vet ./internal/httpapi/...
gofmt -l internal/httpapi/
```

All must exit 0 / produce empty output. `TestHealth_RejectsWrongMethod` must report 3 passing subtests (POST, PUT, DELETE).

**Anti-patterns to avoid (PITFALLS-aligned):**
- Do NOT write `mux.HandleFunc("/health", ...)` without the `GET ` prefix — that matches all methods and breaks the 405 behavior (§Pitfall 4)
- Do NOT call `s.logger.Info("health hit", ...)` in `handleHealth` — health endpoints must not log (§Pitfall 2)
- Do NOT write `s.logger.Any("req", r)` or log `r.Header` anywhere — credentials would leak when Phase 4 adds Authorization headers (PITFALLS.md §13)
- Do NOT call `slog.Info(...)` directly anywhere in `internal/httpapi/` — always via `s.logger`
- Do NOT store `*Config` on the Server struct in Phase 1 — only `logger` and `mux`
- Do NOT use `json.NewEncoder(w).Encode(...)` for the `/health` body — the payload is a constant literal, and encoder adds a trailing newline and can theoretically fail. Phase 4's dynamic error envelope handlers will use json.NewEncoder properly; Phase 1 writes the literal bytes (RESEARCH §Pattern 3 rationale)
  </action>
  <verify>
    <automated>test -f internal/httpapi/server.go &amp;&amp; test -f internal/httpapi/health.go &amp;&amp; test -f internal/httpapi/health_test.go &amp;&amp; grep -q '^package httpapi$' internal/httpapi/server.go &amp;&amp; grep -q '^package httpapi$' internal/httpapi/health.go &amp;&amp; grep -q '^package httpapi$' internal/httpapi/health_test.go &amp;&amp; grep -q 'type Server struct' internal/httpapi/server.go &amp;&amp; grep -q 'logger \*slog.Logger' internal/httpapi/server.go &amp;&amp; grep -q 'func New(logger \*slog.Logger) \*Server' internal/httpapi/server.go &amp;&amp; grep -q 'func (s \*Server) Handler() http.Handler' internal/httpapi/server.go &amp;&amp; grep -q '"GET /health"' internal/httpapi/server.go &amp;&amp; grep -q 'handleHealth' internal/httpapi/health.go &amp;&amp; grep -q 'application/json' internal/httpapi/health.go &amp;&amp; grep -q '"status":"ok"' internal/httpapi/health.go &amp;&amp; ! grep -E 'slog\.(Info|Debug|Warn|Error|Default)' internal/httpapi/server.go internal/httpapi/health.go &amp;&amp; ! grep -q 'r\.Header' internal/httpapi/health.go &amp;&amp; go test ./internal/httpapi -race -count=1 &amp;&amp; go vet ./internal/httpapi/... &amp;&amp; test -z "$(gofmt -l internal/httpapi/)"</automated>
  </verify>
  <acceptance_criteria>
    - `test -f internal/httpapi/server.go` succeeds
    - `test -f internal/httpapi/health.go` succeeds
    - `test -f internal/httpapi/health_test.go` succeeds
    - All three files declare `package httpapi`
    - `grep -q 'type Server struct' internal/httpapi/server.go` succeeds
    - `grep -q 'logger \*slog.Logger' internal/httpapi/server.go` — logger is a struct field
    - `grep -q 'func New(logger \*slog.Logger) \*Server' internal/httpapi/server.go`
    - `grep -q 'func (s \*Server) Handler() http.Handler' internal/httpapi/server.go`
    - `grep -q '"GET /health"' internal/httpapi/server.go` — method-prefixed pattern (§Pitfall 4)
    - `grep -q 'application/json' internal/httpapi/health.go`
    - `grep -q '"status":"ok"' internal/httpapi/health.go` — exact JSON body
    - `! grep -E 'slog\.(Info|Debug|Warn|Error|Default)' internal/httpapi/server.go internal/httpapi/health.go` — NO direct slog calls (only s.logger is allowed)
    - `! grep -q 'r\.Header' internal/httpapi/health.go` — never touch request headers in /health (§PITFALLS.md #13)
    - `! grep -q 's\.logger\.' internal/httpapi/health.go` — /health does NOT log (§Pitfall 2)
    - `go test ./internal/httpapi -race -count=1` passes (TestHealth + 3 subtests in TestHealth_RejectsWrongMethod)
    - `go vet ./internal/httpapi/...` exits 0
    - `gofmt -l internal/httpapi/` produces empty output
  </acceptance_criteria>
  <done>
`internal/httpapi` package ships a minimal `Server` with injected `*slog.Logger`, a `Handler()` method returning the mux, and a `/health` route using the Go 1.22+ method-prefixed pattern. Tests confirm GET returns 200 JSON, POST/PUT/DELETE return 405, and no Authorization header is required. No `slog.Default()` or bare `slog.Info` calls anywhere; no request-header logging.
  </done>
</task>

<task type="auto">
  <name>Task 2: Implement cmd/server/main.go compose-root (flag, config, logger, banner, httpapi, ListenAndServe)</name>
  <files>cmd/server/main.go</files>
  <read_first>
    - .planning/phases/01-foundation/01-CONTEXT.md (§Startup Banner — exact 10 keys in locked order, §Logging Toggle)
    - .planning/phases/01-foundation/01-RESEARCH.md (§Pattern 1 Compose-Root main.go — full ~60-line reference, §Pattern 2 slog constructor location)
    - .planning/phases/01-foundation/01-VALIDATION.md (§Per-Task Verification Map rows for FOUND-03, FOUND-05)
    - internal/config/config.go (confirm Config field names and Addr() signature)
    - internal/version/version.go (confirm the exported Version variable)
    - internal/httpapi/server.go (confirm New(logger) signature)
    - cmd/server/main.go (the Plan 01-01 stub will be fully replaced)
  </read_first>
  <action>
Replace the Plan 01-01 stub `cmd/server/main.go` with the full compose-root. The file must:

1. Parse a `-config` flag (default `./config.yaml`)
2. Call `config.Load(*configPath)` and fail fast with the wrapped error
3. Construct a `*slog.Logger` from `cfg.Logging.Format` and `cfg.Logging.Level` via an inline `newLogger` helper — do NOT create `internal/logging` (RESEARCH §Pattern 2)
4. Emit the startup banner via `logger.Info("openburo server starting", ...)` with exactly these 10 keys in this exact order: `version`, `go_version`, `listen_addr`, `tls_enabled`, `config_file`, `credentials_file`, `registry_file`, `ping_interval`, `log_format`, `log_level`
5. Construct `httpapi.New(logger)` and wrap in `http.Server` with `Addr: cfg.Server.Addr()`, `Handler: srv.Handler()`
6. Call `httpSrv.ListenAndServe()` — Phase 1 has no graceful shutdown; Phase 5 will add `signal.NotifyContext` + two-phase shutdown

Write `cmd/server/main.go` verbatim (body based on RESEARCH §Pattern 1):

```go
// Command openburo-server runs the OpenBuro capability broker.
//
// Phase 1: loads config.yaml, constructs an injected slog logger, emits
// a structured startup banner, and serves GET /health. Phase 5 will add
// signal-aware graceful shutdown and two-phase WebSocket close.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/openburo/openburo-server/internal/config"
	"github.com/openburo/openburo-server/internal/httpapi"
	"github.com/openburo/openburo-server/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "./config.yaml", "path to config.yaml")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger, err := newLogger(cfg.Logging.Format, cfg.Logging.Level)
	if err != nil {
		return fmt.Errorf("build logger: %w", err)
	}

	logger.Info("openburo server starting",
		"version", version.Version,
		"go_version", runtime.Version(),
		"listen_addr", cfg.Server.Addr(),
		"tls_enabled", cfg.Server.TLS.Enabled,
		"config_file", *configPath,
		"credentials_file", cfg.CredentialsFile,
		"registry_file", cfg.RegistryFile,
		"ping_interval", cfg.WebSocket.PingInterval.String(),
		"log_format", cfg.Logging.Format,
		"log_level", cfg.Logging.Level,
	)

	srv := httpapi.New(logger)
	httpSrv := &http.Server{
		Addr:    cfg.Server.Addr(),
		Handler: srv.Handler(),
	}
	return httpSrv.ListenAndServe()
}

// newLogger builds a *slog.Logger from config.
//
// Lives inline in main.go (not in an internal/logging package) because
// it's compose-root wiring, and because keeping it here guarantees no
// internal/ package ever grabs slog.Default() behind the compose root's
// back. See .planning/phases/01-foundation/01-RESEARCH.md §Pattern 2.
func newLogger(format, level string) (*slog.Logger, error) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		return nil, fmt.Errorf("invalid log level %q (want debug|info|warn|error)", level)
	}
	opts := &slog.HandlerOptions{Level: lvl}
	var h slog.Handler
	switch strings.ToLower(format) {
	case "json":
		h = slog.NewJSONHandler(os.Stderr, opts)
	case "text":
		h = slog.NewTextHandler(os.Stderr, opts)
	default:
		return nil, fmt.Errorf("invalid log format %q (want json|text)", format)
	}
	return slog.New(h), nil
}
```

Smoke-test the whole binary by copying the example files and running it briefly:

```bash
# From repo root:
cp config.example.yaml config.yaml
cp credentials.example.yaml credentials.yaml
go build ./...
# Start the server in background, wait for the banner, hit /health, kill it.
go run ./cmd/server -config config.yaml &
SERVER_PID=$!
sleep 1
curl -sS -o /dev/null -w "%{http_code}" http://localhost:8080/health
echo
kill $SERVER_PID 2>/dev/null || true
wait $SERVER_PID 2>/dev/null || true
# Clean up local copies so they don't get committed.
rm -f config.yaml credentials.yaml
```

Expected output: the banner appears as a JSON line on stderr, and `curl` prints `200`. If `curl` prints anything else, the route is wrong or the server didn't start.

**Important verification of the no-slog.Default() rule across the whole `internal/` tree:**

```bash
# This grep must produce ZERO matches. `slog.NewJSONHandler` and
# `slog.NewTextHandler` are allowed (they construct, not log), but
# `slog.Info` / `slog.Default` / etc. are forbidden inside internal/.
! grep -rE 'slog\.(Info|Debug|Warn|Error|Default)\(' internal/
```

The only place `slog.Info` is permitted is in `cmd/server/main.go`'s startup banner (`logger.Info`, which is a method call on an injected logger — grep above treats method calls on `logger.` or `s.logger.` as a different pattern and allows them).

Actually, more precisely — `logger.Info(...)` matches neither `slog.Info(...)` nor `slog.Default()`, so the grep gate above is safe as written. It only rejects `slog.Info(...)`, `slog.Debug(...)`, `slog.Warn(...)`, `slog.Error(...)`, and `slog.Default()`. Method calls on an injected `*slog.Logger` variable are fine.

**Anti-patterns to avoid:**
- Do NOT put `newLogger` in a new `internal/logging` package (RESEARCH §Pattern 2 — inline is the decision)
- Do NOT call `slog.SetDefault(logger)` — that undermines injection (CONTEXT.md §Logging Toggle anti-pattern)
- Do NOT add `signal.NotifyContext` or any graceful shutdown logic — that's Phase 5
- Do NOT reorder the 10 banner keys — the order is locked in CONTEXT.md §Startup Banner
- Do NOT add extra banner keys (no `"pid"`, no `"hostname"`, no `"commit"`) — locked to exactly 10
- Do NOT emit the banner BEFORE the logger is constructed (it requires the logger)
- Do NOT emit the banner AFTER `ListenAndServe` returns (the binary would never log it until shutdown)
  </action>
  <verify>
    <automated>test -f cmd/server/main.go &amp;&amp; grep -q '^package main$' cmd/server/main.go &amp;&amp; grep -q 'func run() error' cmd/server/main.go &amp;&amp; grep -q 'flag.String("config", "./config.yaml"' cmd/server/main.go &amp;&amp; grep -q 'config.Load' cmd/server/main.go &amp;&amp; grep -q 'func newLogger' cmd/server/main.go &amp;&amp; grep -q 'slog.NewJSONHandler(os.Stderr' cmd/server/main.go &amp;&amp; grep -q 'slog.NewTextHandler(os.Stderr' cmd/server/main.go &amp;&amp; grep -q '"openburo server starting"' cmd/server/main.go &amp;&amp; grep -q '"version", version.Version' cmd/server/main.go &amp;&amp; grep -q '"go_version", runtime.Version()' cmd/server/main.go &amp;&amp; grep -q '"listen_addr", cfg.Server.Addr()' cmd/server/main.go &amp;&amp; grep -q '"tls_enabled", cfg.Server.TLS.Enabled' cmd/server/main.go &amp;&amp; grep -q '"config_file", \*configPath' cmd/server/main.go &amp;&amp; grep -q '"credentials_file", cfg.CredentialsFile' cmd/server/main.go &amp;&amp; grep -q '"registry_file", cfg.RegistryFile' cmd/server/main.go &amp;&amp; grep -q '"ping_interval", cfg.WebSocket.PingInterval.String()' cmd/server/main.go &amp;&amp; grep -q '"log_format", cfg.Logging.Format' cmd/server/main.go &amp;&amp; grep -q '"log_level", cfg.Logging.Level' cmd/server/main.go &amp;&amp; grep -q 'httpapi.New(logger)' cmd/server/main.go &amp;&amp; grep -q 'httpSrv.ListenAndServe()' cmd/server/main.go &amp;&amp; ! grep -q 'slog.SetDefault' cmd/server/main.go &amp;&amp; ! grep -rE 'slog\.(Info|Debug|Warn|Error|Default)\(' internal/ &amp;&amp; go build ./... &amp;&amp; go vet ./... &amp;&amp; test -z "$(gofmt -l .)" &amp;&amp; go test ./... -race -count=1</automated>
  </verify>
  <acceptance_criteria>
    - `test -f cmd/server/main.go` succeeds
    - `grep -q '^package main$' cmd/server/main.go`
    - `grep -q 'func run() error' cmd/server/main.go` — error-returning run() for testability
    - `grep -q 'flag.String("config", "./config.yaml"' cmd/server/main.go` — default flag value matches CONTEXT.md
    - `grep -q 'config.Load' cmd/server/main.go`
    - `grep -q 'func newLogger' cmd/server/main.go` — inline helper, NOT internal/logging package
    - `! test -d internal/logging` — the internal/logging package must NOT exist
    - `grep -q 'slog.NewJSONHandler(os.Stderr' cmd/server/main.go` — stderr, not stdout (CONTEXT.md locked)
    - `grep -q 'slog.NewTextHandler(os.Stderr' cmd/server/main.go`
    - `grep -q '"openburo server starting"' cmd/server/main.go` — exact banner message
    - ALL 10 banner keys present in locked order:
      - `"version", version.Version`
      - `"go_version", runtime.Version()`
      - `"listen_addr", cfg.Server.Addr()`
      - `"tls_enabled", cfg.Server.TLS.Enabled`
      - `"config_file", *configPath`
      - `"credentials_file", cfg.CredentialsFile`
      - `"registry_file", cfg.RegistryFile`
      - `"ping_interval", cfg.WebSocket.PingInterval.String()`
      - `"log_format", cfg.Logging.Format`
      - `"log_level", cfg.Logging.Level`
    - `grep -q 'httpapi.New(logger)' cmd/server/main.go` — logger injected into Server
    - `grep -q 'httpSrv.ListenAndServe()' cmd/server/main.go` — Phase 1 uses ListenAndServe, not ListenAndServeTLS (Phase 5 adds TLS)
    - `! grep -q 'slog.SetDefault' cmd/server/main.go` — global default forbidden
    - `! grep -q 'signal.NotifyContext' cmd/server/main.go` — graceful shutdown is Phase 5
    - **Cross-tree grep gate:** `! grep -rE 'slog\.(Info|Debug|Warn|Error|Default)\(' internal/` — NO direct slog function calls anywhere in internal/ (method calls on injected loggers are fine and don't match this pattern)
    - `go build ./...` exits 0
    - `go vet ./...` exits 0
    - `test -z "$(gofmt -l .)"` — the whole tree is gofmt-clean
    - `go test ./... -race -count=1` — the whole suite passes (config + httpapi)
  </acceptance_criteria>
  <done>
`cmd/server/main.go` is the full compose-root: parses `-config`, calls `config.Load`, builds logger via inline `newLogger`, emits the 10-key startup banner in locked order, constructs `httpapi.New(logger)`, and runs `ListenAndServe`. No `slog.Default()` or bare `slog.Info/Debug/Warn/Error` anywhere in `internal/`. Whole module passes `go build`, `go vet`, `gofmt -l`, and `go test ./... -race`.
  </done>
</task>

</tasks>

<verification>
Full Phase 1 gate after all three plans land:

```bash
# Structural
test -d cmd/server
test -d internal/config
test -d internal/registry && test -f internal/registry/doc.go
test -d internal/wshub && test -f internal/wshub/doc.go
test -d internal/httpapi
test -d internal/version
test -f go.mod && test -f go.sum
test -f Makefile && test -f .gitignore && test -f .github/workflows/ci.yml
test -f config.example.yaml && test -f credentials.example.yaml

# Compile + static
go build ./...
go vet ./...
test -z "$(gofmt -l .)"

# No forbidden slog usage in internal/
! grep -rE 'slog\.(Info|Debug|Warn|Error|Default)\(' internal/

# Tests
go test ./... -race -count=1

# Makefile mirror of CI
make build
make clean

# Smoke test: the binary actually starts and serves /health
cp config.example.yaml config.yaml
cp credentials.example.yaml credentials.yaml
go run ./cmd/server -config config.yaml &
PID=$!
sleep 1
STATUS=$(curl -sS -o /dev/null -w "%{http_code}" http://localhost:8080/health)
kill $PID 2>/dev/null || true
wait $PID 2>/dev/null || true
rm -f config.yaml credentials.yaml
test "$STATUS" = "200"
```

Every step must pass for the phase to be green.
</verification>

<success_criteria>
- The binary starts on `config.example.yaml` (copied to `config.yaml`) and serves `GET /health` returning 200 JSON (FOUND-04, FOUND-05)
- Startup banner emits exactly one `slog.Info("openburo server starting", ...)` line with all 10 required keys in the locked order (FOUND-05)
- `*slog.Logger` is constructed in `main.go` and injected into `httpapi.New` — no `internal/` package calls `slog.Default()` or bare `slog.*` (FOUND-03)
- `POST /health` returns 405 Method Not Allowed, proving the Go 1.22 method-pattern pattern is wired correctly (§Pitfall 4)
- `/health` handler does not log the request, establishing the "never log r.Header" convention Phase 4's auth middleware will inherit (§PITFALLS.md #13)
- `go test ./... -race` passes across the whole module (Phase 1 meta — not required by FOUND-06 until CI runs it, but the local gate proves it)
</success_criteria>

<output>
After completion, create `.planning/phases/01-foundation/01-03-SUMMARY.md` documenting:
- Files created (server.go, health.go, health_test.go, main.go)
- The inline-`newLogger` decision (RESEARCH §Pattern 2) — note that adding `internal/logging` later requires consciously rejecting this decision
- The "no direct slog calls inside internal/" invariant — document the exact grep gate for future phases to preserve
- The 10-key banner order (copy from CONTEXT.md) — Phase 2-5 code reviews must not reorder or remove keys without a CONTEXT update
- Smoke-test result (http status code returned by `curl localhost:8080/health`)
- Phase 1 complete marker: `go test ./... -race` passing, `make ci` passing, banner visible
</output>
