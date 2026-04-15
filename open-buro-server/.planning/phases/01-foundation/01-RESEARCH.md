# Phase 1: Foundation - Research

**Researched:** 2026-04-09
**Domain:** Go 1.26 project scaffolding (module init, config loader, slog construction, minimal HTTP server, CI, Makefile)
**Confidence:** HIGH

## Summary

Phase 1 is pure scaffolding: wire up the module, four-package `internal/` layout, a working `config.Load`, a safe `*slog.Logger` constructor, a minimal `httpapi.Server` serving `GET /health`, CI (GitHub Actions), a Makefile, and example YAML files. No domain code (no registry, no hub, no auth, no routes beyond `/health`) — those are Phases 2-5.

All strategic choices are already locked by CONTEXT.md (GitHub Actions + Makefile + staticcheck + `-config` flag + `logging.format/level` in YAML + `var Version` + single-line banner). Domain-level research (STACK.md, ARCHITECTURE.md, PITFALLS.md) is stable and not re-investigated here. This document is narrow: it drills into the exact shape of the files Phase 1 must create so the planner can write tasks without ambiguity.

**Primary recommendation:** Build top-down from `cmd/server/main.go` — decide the compose-root signature first, then implement `internal/config`, `internal/version`, the slog constructor (inline helper in `main.go` or tiny `internal/logging` package), and `internal/httpapi` (Server scaffold + `/health`). Every piece must accept an injected `*slog.Logger`; nothing uses `slog.Default()`. The two pitfalls that MUST land in Phase 1's logging code (even though no credentials flow yet) are: never log `r.Header` or `r` directly, and always use `defer mu.Unlock()` (there is no mutex in Phase 1 code, but the health handler establishes the idiom).

## User Constraints (from CONTEXT.md)

### Locked Decisions

**Module & Repository**
- Module path: `github.com/openburo/openburo-server` (placeholder; single point of change)
- `go.mod` declares `go 1.26`; `go.sum` committed; no vendor; no `replace` directives

**CI & Build Tooling**
- CI platform: **GitHub Actions**, single workflow `.github/workflows/ci.yml`, one job with sequential steps, runs on push + pull_request
- CI steps in order: checkout → setup-go 1.26 → `go mod download` → `gofmt -l .` → `go vet ./...` → `go run honnef.co/go/tools/cmd/staticcheck@latest ./...` → `go build ./...` → `go test ./... -race -count=1`
- **Makefile is required** with: `build`, `run`, `test`, `lint`, `fmt`, `ci`, `clean`
- `LDFLAGS := -X github.com/openburo/openburo-server/internal/version.Version=$(VERSION)` with `VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")`

**Linting**
- Phase 1 linters: `gofmt`, `go vet`, `staticcheck` only
- **NOT using `golangci-lint`** — no `.golangci.yml`
- `staticcheck` via `go run honnef.co/go/tools/cmd/staticcheck@latest` (no tool-dep pin)

**Config Files**
- Two YAML files: `config.yaml` + `credentials.yaml`
- Both have committed `.example` siblings at repo root; the real files are `.gitignore`d
- `config.yaml` shape is locked (see CONTEXT.md for exact schema — `server.port/tls`, `credentials_file`, `registry_file`, `websocket.ping_interval_seconds`, `logging.format/level`, `cors.allowed_origins`)

**Config Discovery**
- Flag: `-config <path>`, defaults to `./config.yaml`
- **No env var, no XDG, no auto-search**
- Missing file: fail fast with `"config file not found: <path>; copy config.example.yaml to config.yaml to get started"`
- Credentials path: read from `config.yaml`'s `credentials_file` key; Phase 1 only validates the path is set (doesn't load credentials)

**Logging (slog)**
- `logging.format` = `json` | `text` (default `json`) → `slog.NewJSONHandler(os.Stderr, opts)` or `slog.NewTextHandler(os.Stderr, opts)`
- `logging.level` = `debug` | `info` | `warn` | `error` (default `info`)
- **Output: stderr only**
- No env-var override
- Logger constructed in `main.go`, wrapped in `*slog.Logger`, **injected into every component**. No `slog.Default()` inside `internal/`.

**Versioning**
- `var Version = "dev"` in `internal/version/version.go`
- Makefile `LDFLAGS` injects via `git describe --tags --always --dirty`
- Accessor: `version.Version` (exported variable, no helper function)
- Go version captured via `runtime.Version()` at startup

**Startup Banner** (exact shape locked — see CONTEXT.md ### Startup Banner section)
- Single `slog.Info("openburo server starting", ...)` call with specific key order: `version`, `go_version`, `listen_addr`, `tls_enabled`, `config_file`, `credentials_file`, `registry_file`, `ping_interval`, `log_format`, `log_level`
- No ASCII art, no multi-line banner

**Package Layout (TEST-07)**
```
cmd/server/main.go
internal/config/        (config.go, config_test.go, testdata/{valid,invalid}.yaml)
internal/version/       (version.go — just var Version = "dev")
internal/registry/      (doc.go stub only)
internal/wshub/         (doc.go stub only)
internal/httpapi/       (server.go, health.go, health_test.go)
config.example.yaml
credentials.example.yaml
go.mod, go.sum, Makefile, .gitignore
.github/workflows/ci.yml
```

**Testing Layout**
- Tests in the **same package** as code (white-box by default); `httpapi_test` external package allowed for E2E
- Table-driven with `t.Run` subtests
- Fixtures in `testdata/` subdirs
- `testify/require` pulled in as dep in Phase 1 even though initial tests are simple

**.gitignore content** (locked verbatim — see CONTEXT.md)

### Claude's Discretion

- Exact duration field type in config (`time.Duration` parsed string vs. integer seconds) — **Default locked: integer seconds in YAML (`ping_interval_seconds: 30`), converted to `time.Duration` during `Load`**
- Internal naming of Config substructs (`ServerConfig`, `TLSConfig`, `LoggingConfig`, `CORSConfig`, `WebSocketConfig`) — planner picks exact names
- `config_test.go` table size — planner decides, but MUST include: valid full file, missing required fields, invalid enum values (format/level), missing file, unreadable file
- Whether Makefile has a `help` target (cosmetic)
- Whether `.example` comments in `config.example.yaml` include per-field descriptions
- Whether the slog construction helper lives inline in `main.go` or in a tiny `internal/logging` package (research recommendation: **inline helper in `cmd/server/main.go`** — see Architecture section)
- Whether `internal/httpapi` exposes a `NewServer` function or a value type; research recommendation: pointer receiver `*Server` with `func New(...) *Server`
- Whether `/health` returns a JSON body `{"status":"ok"}` or just a 200 with empty body; research recommendation: **JSON body** with `Content-Type: application/json` to establish the pattern Phase 4 will extend

### Deferred Ideas (OUT OF SCOPE)

- LICENSE file (Phase 5)
- Full README (Phase 5; optional stub in Phase 1)
- `tool` directive in `go.mod` for staticcheck (Go 1.24+ feature)
- Dockerfile / container image (out of scope entirely per PROJECT.md)
- Release workflow (goreleaser, tagged builds)
- CONTRIBUTING.md, CODE_OF_CONDUCT.md, issue templates
- Graceful shutdown (`signal.NotifyContext`, two-phase shutdown) — Phase 5
- CORS middleware — Phase 4
- Basic Auth middleware — Phase 4
- Any registry or WebSocket code — Phases 2-3

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| **FOUND-01** | Project builds with `go build ./...` on Go 1.26 with pinned `go.mod` deps | Standard Stack section (go.mod shape); Makefile targets; CI pipeline |
| **FOUND-02** | Config loaded from `config.yaml` (port, TLS, credential path, registry path, WS ping interval) | Config Package Shape section (Config struct + Load + validation + duration conversion) |
| **FOUND-03** | Structured logging via `log/slog` (JSON prod, text dev) injected everywhere | slog Constructor Pattern section (including PITFALLS #13 safety) |
| **FOUND-04** | `GET /health` returns 200 without auth | httpapi.Server Scaffold section (Server struct + New + Handler() + health.go + health_test.go using httptest) |
| **FOUND-05** | Startup banner log line captures version, config path, listen addr, TLS state, registry path, ping interval | Startup Banner (locked verbatim from CONTEXT.md) |
| **FOUND-06** | CI runs `go test -race`, `go vet`, `gofmt` check | GitHub Actions Workflow section (exact ci.yml body) |
| **FOUND-07** | Example `config.yaml` and `credentials.yaml` exist at repo root | Example YAML Files section (full file bodies) |
| **TEST-07** | Idiomatic Go layout: `cmd/server/` + `internal/{config,registry,httpapi,wshub}/` | Directory Scaffold Commands section + doc.go stubs for registry and wshub |

## Standard Stack

Phase 1 adds these direct dependencies to `go.mod`. Domain-level research (STACK.md) already vetted each choice; Phase 1 only wires them up.

### Phase 1 Direct Dependencies

| Library | Version | Purpose | Phase 1 Usage |
|---------|---------|---------|---------------|
| **Go toolchain** | **1.26** (`go 1.26` directive) | Language/runtime | `go.mod` directive; CI pins same version |
| **`go.yaml.in/yaml/v3`** | **v3.0.x** | YAML unmarshaling for config.yaml and credentials.yaml | `config.Load` only. Phase 1 imports this even though credentials are loaded in Phase 4 — it's in `go.mod` from the start so Phase 4 doesn't touch deps. |
| **`github.com/stretchr/testify`** | **v1.11.1** (test-only) | `require` package for tests | `config_test.go` and `health_test.go`. Pulled in Phase 1 so future phases don't add deps. |

**Phase 1 does NOT import yet** (deferred to later phases):
- `github.com/coder/websocket` — Phase 3
- `golang.org/x/crypto/bcrypt` — Phase 4
- `github.com/rs/cors` — Phase 4

Phase 1's `go.mod` should have **2 direct dependencies** (yaml + testify). When Phase 4 lands, the direct count climbs to 5 — STACK.md's target.

### Installation (one-shot, during module init task)

```bash
# From repo root, after `git init`
go mod init github.com/openburo/openburo-server
# → writes go.mod with `module github.com/openburo/openburo-server` and `go 1.26`

go get go.yaml.in/yaml/v3@latest
go get -t github.com/stretchr/testify/require@latest

go mod tidy
go mod verify
```

### Version Verification

Planner must run `go list -m -versions go.yaml.in/yaml/v3` and `go list -m -versions github.com/stretchr/testify` during scaffolding and record the exact versions landed in go.mod. Training data says `yaml/v3 v3.0.x` and `testify v1.11.1` (2025-08-27); registry may have newer versions by April 2026. **Pin whatever `go get @latest` resolves; don't hand-edit go.mod.**

### GitHub Actions (tool dependencies)

| Action | Version | Notes |
|--------|---------|-------|
| `actions/checkout` | **`@v6`** | v5 still works; v6 is current as of 2026 and aligns with Node.js 24 runner migration |
| `actions/setup-go` | **`@v6`** | v6 released 2025, supports Go 1.26 and the `toolchain` directive; v5 runs on Node 20 (deprecated June 2026) |

**Finding: CONTEXT.md specifies `@v4` and `@v5` for the two actions.** Current state of the world (April 2026): **`actions/checkout@v6` and `actions/setup-go@v6`** are the current majors. v4/v5 still work but will hit Node 20 deprecation in June 2026.

**Recommendation for planner:** Use `@v6` for both. This is a small tooling drift from CONTEXT.md and falls under "current-state correction, not strategic change." If the user had strong reasons for v4/v5, they'd have said so; the discussion was about "GitHub Actions" as a platform, not specific action versions. Planner should surface this in the Plan doc as a note, but ship v6.

Sources (MEDIUM — verified via web search April 2026):
- [actions/setup-go releases](https://github.com/actions/setup-go/releases)
- [actions/checkout releases](https://github.com/actions/checkout/releases)

### Alternatives Considered

Not applicable — Phase 1's tooling is locked by CONTEXT.md (GH Actions + Makefile + staticcheck, not golangci-lint). Any alternative would contradict a locked decision.

## Architecture Patterns

### Phase 1 Dependency Graph (what exists at end of phase)

```
                  cmd/server/main.go
                   │
                   ├──► internal/config      (Load)
                   ├──► internal/version     (Version var)
                   └──► internal/httpapi     (New, Handler, health handler)

              internal/registry/             ← doc.go stub only
              internal/wshub/                ← doc.go stub only
```

Nothing imports nothing else yet except `main.go` → {config, version, httpapi}. The stub packages exist for `go build ./...` to succeed and to verify TEST-07 structurally.

### Pattern 1: Compose-Root `cmd/server/main.go`

**What:** `main.go` parses the `-config` flag, calls `config.Load`, constructs a `*slog.Logger` from config, logs the startup banner, constructs `httpapi.New(logger)`, wraps in `http.Server`, calls `ListenAndServe`. No business logic. No graceful shutdown yet (Phase 5 adds `signal.NotifyContext` + two-phase shutdown). Phase 1 just needs "starts, serves /health, exits on Ctrl-C with an unhandled error, which is fine for a reference impl at this stage."

**Reference skeleton** (~60-80 lines):

```go
// cmd/server/main.go
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
// Lives in main.go (not in internal/) because it's compose-root wiring,
// and because keeping it here guarantees no internal/ package ever
// grabs slog.Default() behind the compose root's back.
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

**Notes for planner:**
- `run()` returns error; `main()` prints and exits. Standard Go idiom for testable main.
- `newLogger` lives in `main.go` — recommendation section below argues this vs an `internal/logging` package.
- No `context.Background()` yet; no signal handling yet. Phase 5 refactors this.
- `ListenAndServe` returns `http.ErrServerClosed` on graceful exit; Phase 5 will filter that. Phase 1 just lets it propagate as an error on Ctrl-C — acceptable because Phase 1 is scaffolding, not production.
- `cfg.Server.Addr()` is a method on the Config struct that returns `":8080"` (or whatever is configured). See Config Package Shape below.

### Pattern 2: slog Constructor Location — Inline vs. Package

**Decision point:** Does `newLogger` live in `cmd/server/main.go` or in a new `internal/logging` package?

**Recommendation: inline in `main.go`.**

| Option | Pros | Cons |
|--------|------|------|
| **Inline in `main.go`** (recommended) | Zero new package. Compose-root owns the construction. No chance an `internal/` package accidentally calls a `logging.Default()` helper. ~20 lines of code. | Slightly harder to unit-test the format/level switch logic (but it's trivially simple — table test it in `main_test.go` or skip entirely and rely on the validation that bad config values are rejected). |
| **`internal/logging` package** | Unit-testable in isolation. Could be reused by a future `cmd/gen-credentials` helper. | Adds a package for ~20 lines. Creates the temptation to add a `logging.Default()` or package-level logger variable, which violates the "inject everywhere" rule. |

**Why inline wins for Phase 1:** CONTEXT.md says "No `slog.Default()` usage inside `internal/` packages — always accept an injected `*slog.Logger`." The safest way to enforce that is to make it physically impossible: the constructor lives in `main` and the logger is passed by reference to every component. No `internal/logging` package exists to tempt a future contributor into calling `logging.Default()`.

If Phase 5 or later adds a second binary that needs logger construction, refactor then. YAGNI applies.

### Pattern 3: `internal/httpapi.Server` Minimal Scaffold

**What:** A `Server` struct with `logger` + `mux` fields, a `New(logger)` constructor that builds the mux and registers `/health`, and a `Handler() http.Handler` method returning the mux. Phase 2-4 will add `store`, `hub`, `creds` fields and more routes; Phase 1 establishes the shape.

```go
// internal/httpapi/server.go
package httpapi

import (
    "log/slog"
    "net/http"
)

// Server owns the HTTP routing and handler implementations for the
// OpenBuro broker. Phase 1 ships a minimal version with /health only;
// subsequent phases extend it with the registry store, websocket hub,
// credentials, and full middleware chain.
type Server struct {
    logger *slog.Logger
    mux    *http.ServeMux
}

// New constructs a Server with the given dependencies and registers its routes.
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
// routes here without touching main.go.
func (s *Server) registerRoutes() {
    s.mux.HandleFunc("GET /health", s.handleHealth)
}
```

```go
// internal/httpapi/health.go
package httpapi

import (
    "net/http"
)

// handleHealth answers GET /health with 200 and a minimal JSON body.
// No authentication (public per AUTH-03 / FOUND-04).
// Returns application/json to establish the content-type convention
// that future handlers will follow.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    _, _ = w.Write([]byte(`{"status":"ok"}`))
}
```

**Why `GET /health` (not `/health`) in the pattern:** Go 1.22 `ServeMux` method-prefixed patterns. Without `GET`, the route matches any method, which is valid but less explicit and locks in bad habits for Phase 4 where method distinctions matter.

**Why no request body read:** `/health` is idempotent and takes no input. Phase 4's middleware will handle body drain for the routes that take bodies (API-11).

**Why writing the body directly (not via `json.NewEncoder`) in Phase 1:** The payload is a constant. `json.NewEncoder(w).Encode(...)` adds a trailing newline and can fail (although for a map it can't). A bare `w.Write([]byte(...))` is simpler and shows intent. Phase 4's error envelope handlers will use `json.NewEncoder` proper.

### Pattern 4: `internal/httpapi/health_test.go` Shape

```go
// internal/httpapi/health_test.go
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
    // Critical for FOUND-04: no Authorization header set.
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

**Why two tests:**
1. `TestHealth` proves FOUND-04: GET /health returns 200, no auth required, JSON content type.
2. `TestHealth_RejectsWrongMethod` proves the Go 1.22 ServeMux method-prefix pattern is actually working (otherwise `POST /health` would also return 200, and we'd ship a subtle bug that only bites in Phase 4 when we add POST routes).

**Why `httptest.NewRecorder` instead of `httptest.NewServer`:** NewRecorder is in-process, faster, no network port involved, perfect for unit-testing a single handler. NewServer is for E2E tests that exercise the full stack; Phase 4 uses it. Phase 1 stays light.

**Why `io.Discard` for the logger:** Otherwise every test run dumps slog output to stderr, making `go test` output noisy. The discarded logger still satisfies the injection contract.

### Pattern 5: `internal/config.Config` Package Shape

```go
// internal/config/config.go
package config

import (
    "errors"
    "fmt"
    "net"
    "os"
    "strconv"
    "time"

    "go.yaml.in/yaml/v3"
)

// Config is the root of config.yaml. Fields are populated from the YAML
// document; validation runs in Load after unmarshal.
type Config struct {
    Server          ServerConfig    `yaml:"server"`
    CredentialsFile string          `yaml:"credentials_file"`
    RegistryFile    string          `yaml:"registry_file"`
    WebSocket       WebSocketConfig `yaml:"websocket"`
    Logging         LoggingConfig   `yaml:"logging"`
    CORS            CORSConfig      `yaml:"cors"`
}

type ServerConfig struct {
    Port int       `yaml:"port"`
    TLS  TLSConfig `yaml:"tls"`
}

// Addr returns the listen address in ":port" form for http.Server.Addr.
// A zero port yields ":0" which http.Server treats as "pick any free port",
// which is surprising and not what the operator means — validate nonzero in Load.
func (s ServerConfig) Addr() string {
    return net.JoinHostPort("", strconv.Itoa(s.Port))
}

type TLSConfig struct {
    Enabled  bool   `yaml:"enabled"`
    CertFile string `yaml:"cert_file"`
    KeyFile  string `yaml:"key_file"`
}

// WebSocketConfig holds websocket-specific settings. PingInterval is derived
// from PingIntervalSeconds during Load — YAML uses a human-friendly integer,
// Go code uses time.Duration.
type WebSocketConfig struct {
    PingIntervalSeconds int           `yaml:"ping_interval_seconds"`
    PingInterval        time.Duration `yaml:"-"`
}

type LoggingConfig struct {
    Format string `yaml:"format"` // "json" | "text"
    Level  string `yaml:"level"`  // "debug" | "info" | "warn" | "error"
}

type CORSConfig struct {
    AllowedOrigins []string `yaml:"allowed_origins"`
}

// Load reads, parses, and validates config.yaml. On any error it returns a
// wrapped error explaining what failed in operator-friendly language.
func Load(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return nil, fmt.Errorf(
                "config file not found: %s; copy config.example.yaml to config.yaml to get started",
                path,
            )
        }
        return nil, fmt.Errorf("read %s: %w", path, err)
    }

    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, fmt.Errorf("parse %s: %w", path, err)
    }

    if err := cfg.validate(); err != nil {
        return nil, fmt.Errorf("invalid config in %s: %w", path, err)
    }

    // Derive runtime-friendly fields after validation.
    cfg.WebSocket.PingInterval = time.Duration(cfg.WebSocket.PingIntervalSeconds) * time.Second

    return &cfg, nil
}

func (c *Config) validate() error {
    if c.Server.Port <= 0 || c.Server.Port > 65535 {
        return fmt.Errorf("server.port must be between 1 and 65535, got %d", c.Server.Port)
    }
    if c.Server.TLS.Enabled {
        if c.Server.TLS.CertFile == "" {
            return errors.New("server.tls.cert_file required when server.tls.enabled is true")
        }
        if c.Server.TLS.KeyFile == "" {
            return errors.New("server.tls.key_file required when server.tls.enabled is true")
        }
    }
    if c.CredentialsFile == "" {
        return errors.New("credentials_file is required")
    }
    if c.RegistryFile == "" {
        return errors.New("registry_file is required")
    }
    if c.WebSocket.PingIntervalSeconds <= 0 {
        return fmt.Errorf("websocket.ping_interval_seconds must be > 0, got %d", c.WebSocket.PingIntervalSeconds)
    }
    switch c.Logging.Format {
    case "json", "text":
    default:
        return fmt.Errorf("logging.format must be json or text, got %q", c.Logging.Format)
    }
    switch c.Logging.Level {
    case "debug", "info", "warn", "error":
    default:
        return fmt.Errorf("logging.level must be debug|info|warn|error, got %q", c.Logging.Level)
    }
    return nil
}
```

**Notes for planner:**
- Substruct names (`ServerConfig`, `TLSConfig`, etc.) are planner's choice per CONTEXT.md; the shape above is one defensible option.
- `PingIntervalSeconds int` is the YAML-facing field; `PingInterval time.Duration` with `yaml:"-"` is the Go-facing field populated during Load. This keeps YAML human-friendly while giving downstream code a `time.Duration`.
- `validate()` is a method on `*Config`. Called from `Load` after unmarshal and before deriving `PingInterval`. Returns wrapped errors with field names so operators can fix their config.
- `Addr()` on `ServerConfig` returns `":8080"` format. `net.JoinHostPort` handles IPv6 correctly if we ever add a `host` field. Empty host = bind all interfaces, which is the desired default for a reference impl.
- **No defaults applied.** If a field is missing from YAML, Go's zero value survives and `validate()` catches it. This is intentional: operators should see a clear error, not silent defaults that mask typos. If the discussion wants defaults, add them explicitly in Load after unmarshal (e.g., `if cfg.Logging.Format == "" { cfg.Logging.Format = "json" }`), but the current recommendation is to require explicit values in `config.yaml` and rely on `config.example.yaml` to show what they look like.
- **Credentials struct is NOT in Phase 1.** CONTEXT.md is explicit: "Phase 1 only validates the path is set" for credentials. Defer the `Credentials` struct, `LoadCredentials`, and bcrypt wiring to Phase 4. Phase 1's config.go is a single file.

### Pattern 6: `internal/config/config_test.go` Shape

Table-driven with `testdata/` fixtures. Must cover (per CONTEXT.md): valid full file, missing required fields, invalid enum values (format/level), missing file, unreadable file.

```go
// internal/config/config_test.go
package config

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
    tests := []struct {
        name        string
        fixture     string // path relative to testdata/
        wantErr     bool
        errContains string
    }{
        {
            name:    "valid full config",
            fixture: "valid.yaml",
            wantErr: false,
        },
        {
            name:        "invalid log format",
            fixture:     "invalid-log-format.yaml",
            wantErr:     true,
            errContains: "logging.format",
        },
        {
            name:        "invalid log level",
            fixture:     "invalid-log-level.yaml",
            wantErr:     true,
            errContains: "logging.level",
        },
        {
            name:        "missing credentials_file",
            fixture:     "missing-credentials-file.yaml",
            wantErr:     true,
            errContains: "credentials_file",
        },
        {
            name:        "zero port",
            fixture:     "zero-port.yaml",
            wantErr:     true,
            errContains: "server.port",
        },
        {
            name:        "tls enabled without cert",
            fixture:     "tls-no-cert.yaml",
            wantErr:     true,
            errContains: "tls.cert_file",
        },
        {
            name:        "zero ping interval",
            fixture:     "zero-ping.yaml",
            wantErr:     true,
            errContains: "ping_interval_seconds",
        },
        {
            name:        "malformed yaml",
            fixture:     "malformed.yaml",
            wantErr:     true,
            errContains: "parse",
        },
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            cfg, err := Load(filepath.Join("testdata", tc.fixture))
            if tc.wantErr {
                require.Error(t, err)
                require.Contains(t, err.Error(), tc.errContains)
                return
            }
            require.NoError(t, err)
            require.NotNil(t, cfg)
        })
    }
}

func TestLoad_MissingFile(t *testing.T) {
    _, err := Load(filepath.Join("testdata", "does-not-exist.yaml"))
    require.Error(t, err)
    require.Contains(t, err.Error(), "config file not found")
    require.Contains(t, err.Error(), "copy config.example.yaml")
}

func TestLoad_UnreadableFile(t *testing.T) {
    // Unreadable file test: create a temp file with no read perms.
    // Skip on non-Unix where chmod doesn't work the same way.
    tmp := filepath.Join(t.TempDir(), "unreadable.yaml")
    require.NoError(t, os.WriteFile(tmp, []byte("server:\n  port: 8080\n"), 0o000))
    t.Cleanup(func() { _ = os.Chmod(tmp, 0o600) })

    _, err := Load(tmp)
    require.Error(t, err)
    // The error wraps os.ReadFile's permission error; exact message is OS-dependent.
}

func TestLoad_DerivesPingInterval(t *testing.T) {
    cfg, err := Load(filepath.Join("testdata", "valid.yaml"))
    require.NoError(t, err)
    require.Equal(t, 30, cfg.WebSocket.PingIntervalSeconds)
    require.Equal(t, "30s", cfg.WebSocket.PingInterval.String())
}
```

**testdata/ fixtures to create:**

```
internal/config/testdata/
├── valid.yaml                      (full valid config)
├── invalid-log-format.yaml         (logging.format: "xml")
├── invalid-log-level.yaml          (logging.level: "verbose")
├── missing-credentials-file.yaml   (no credentials_file key)
├── zero-port.yaml                  (server.port: 0)
├── tls-no-cert.yaml                (tls.enabled: true, cert_file: "")
├── zero-ping.yaml                  (ping_interval_seconds: 0)
└── malformed.yaml                  (broken YAML — unclosed bracket)
```

**Why separate `TestLoad_UnreadableFile`:** It uses `t.TempDir()` and chmod, which doesn't fit the table-driven structure cleanly. Keeping it standalone keeps the table readable. Note the test may be skipped on Windows if we care about cross-platform (we don't per PROJECT.md — Linux-first).

### Anti-Patterns to Avoid in Phase 1

- **`slog.Default()` anywhere inside `internal/`** — CONTEXT.md locks "inject everywhere." A single slip creates a testability hole and makes Phase 4's "no credentials in logs" test harder to write.
- **Passing `*Config` to `httpapi.New` in Phase 1** — tempting (httpapi will eventually want `Config.CORS`), but Phase 1's `httpapi.Server` only needs a logger. Adding fields preemptively creates ghost dependencies. Add fields in the phase that uses them.
- **Reading `Authorization` header in the /health handler** — there is no Authorization header to read in Phase 1; don't even mention auth in `handleHealth`. Phase 4 will add middleware.
- **Creating `internal/logging` package for the slog constructor** — see Pattern 2. Inline in main.go.
- **Putting `var Version = "dev"` in `cmd/server/main.go`** — CONTEXT.md says it lives in `internal/version/version.go`. The Makefile `-X` flag targets that exact symbol path.
- **Defaulting missing config values silently** — operator should see explicit errors. `config.example.yaml` is the self-documentation.
- **Auto-creating `config.yaml` from `config.example.yaml` on first run** — this is "clever" and hides what's happening. Fail fast, tell the operator what to copy, let them run the cp themselves.
- **Adding `//go:build` tags anywhere** — not needed in Phase 1.
- **Calling `slog.SetDefault(logger)` in main** — sets a global that undermines injection. Don't. The injected `*slog.Logger` is the only logger.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| YAML parsing | Custom line parser | `go.yaml.in/yaml/v3` | yaml.v3 handles multi-doc, anchors, unicode, type coercion, error locations |
| Struct validation framework | `go-playground/validator` | Plain methods (`validate()`) | Phase 1 has ~7 validation rules; a library is 10x more machinery than hand-written `if` statements |
| Config merging / env overrides | Viper, Koanf | Single YAML + `-config` flag | CONTEXT.md locks "no env var, no auto-search." Viper adds 20+ transitive deps for a decision already made |
| HTTP router | chi, gorilla/mux, gin | `http.ServeMux` (Go 1.22+) | STACK.md already establishes this; `"GET /health"` method patterns cover every Phase 1 need |
| JSON encoder for `/health` | Custom | `json` stdlib or literal bytes | Phase 1 payload is a constant — literal bytes is fine; Phase 4 will use `json.NewEncoder` for dynamic envelopes |
| Test assertion library | Custom diff helpers | `testify/require` | Already in go.mod per CONTEXT.md |
| Version string parsing | Semver parser | `var Version = "dev"` + ldflags | The Makefile does the work; the Go code just reads a string |
| Log format switcher | Handler wrapper lib | `slog.NewJSONHandler` / `slog.NewTextHandler` stdlib | Both are one constructor call; the switch is 10 lines |
| CLI framework | cobra, urfave/cli | `flag.String("config", ...)` stdlib | Phase 1 has ONE flag. `flag` is fine |
| Atomic file writing | Hand-rolled temp+rename | `os.Rename` directly (Phase 2 concern) | Phase 1 doesn't write any files — deferred |

**Key insight:** Phase 1 is a "say no" phase. Every "should we add X?" question should be answered "no, and here's what we're using instead." The fewer decisions made now, the cleaner the planner's job on Phases 2-5.

## Common Pitfalls

These are Phase-1-specific gotchas. Full project pitfalls are in `.planning/research/PITFALLS.md` — do NOT re-document them here.

### Pitfall 1: `slog.Default()` creep

**What goes wrong:** A developer writes `slog.Info("starting up")` in an `internal/` package file. It works. It even produces output. But now that package is implicitly coupled to the global default logger, which CONTEXT.md forbids.

**Why it happens:** `slog.Info` is importable from the `log/slog` package at any callsite. It's frictionless. There's no compile error. The test that would catch it (credentials leaking) doesn't exist yet in Phase 1.

**How to avoid:**
- Grep review: after Phase 1 code lands, `rg 'slog\\.(Info|Debug|Warn|Error)' internal/` should return zero results. Only method calls on an injected logger (`s.logger.Info(...)`, `logger.Info(...)`) should appear.
- Static discipline: the `httpapi.Server` struct has a `logger *slog.Logger` field. Every handler uses `s.logger`, never `slog.*` directly.
- In Phase 1 specifically, the only file that mentions `slog.Info` directly is `cmd/server/main.go` (the startup banner). Everywhere else goes through an injected logger.

**Warning signs:**
- Any `slog.Info` / `slog.Error` / `slog.Debug` / `slog.Warn` call outside `cmd/server/main.go`
- Any package-level `var logger = slog.Default()` declaration
- Any `slog.SetDefault(...)` call anywhere

### Pitfall 2: Logging `r.Header` or `r` itself (PITFALLS.md #13 / §Credentials in logs)

**What goes wrong:** A request logging middleware or a panic recovery path uses `logger.Info("request", "req", r)` or `logger.Error("auth failed", "headers", r.Header)`. The next time someone hits the server with an `Authorization: Basic <base64>` header, the whole credential lands in the log.

**Why it happens:** `slog` can serialize anything. `*http.Request` with `slog.Any("req", r)` dumps the whole struct, including `Header`. Debuggers reach for "log the whole thing" shortcuts.

**How to avoid in Phase 1:**
- Phase 1 has no Authorization header in play, but the pattern MUST land now so Phase 4's auth middleware inherits a clean file.
- The `handleHealth` handler does NOT log the request at all. No `s.logger.Info("health hit", ...)` — health endpoints are the noisiest routes and logging them pollutes logs. Future log middleware will skip `/health`.
- If the planner wants ANY request logging in Phase 1 (not required by FOUND-XX), it must use explicit fields only: `"method"`, `"path"`, `"status"`, `"duration_ms"`, `"remote"` — **never** `"req"`, `"request"`, `"headers"`, or `"body"`.

**Warning signs:**
- `logger.Any("req", r)` or `logger.Any("request", r)` anywhere
- `r.Header` referenced inside any slog call
- `r.URL.RawQuery` logged (Phase 4+ concern, but note it: query strings can contain tokens)

**Phase 4 will add the test** (`TestNoCredentialsInLogs`). Phase 1 doesn't need the test yet because there are no credentials. But Phase 1 **must not write code that the test would fail** when it's added.

### Pitfall 3: `defer mu.Unlock()` discipline (PITFALLS.md #12 / §HTTP handler panic leaks locks)

**What goes wrong:** Phase 1 has no mutex. So this pitfall is technically not in scope. BUT Phase 2 will add `registry.Store.mu` and Phase 3 will add `wshub.Hub.mu`. If Phase 1's code review doesn't establish "we use `defer Unlock` everywhere" as the baseline expectation, Phases 2-3 are more likely to ship a `mu.Lock(); work(); mu.Unlock()` pattern that leaks on panic.

**How to avoid in Phase 1:**
- Phase 1 has no mutex, so there's nothing to defer. But the Plan document for Phase 1 should explicitly note: "Any mutex acquisition added in Phase 2+ MUST use `defer mu.Unlock()` immediately after `mu.Lock()`, without exception. No early returns between Lock and defer Unlock."
- This is a documentation-level win, not a code-level one.

**Phase to enforce in code:** Phase 2 (registry) and Phase 3 (wshub).

### Pitfall 4: Go 1.22 method patterns look like they work without the method prefix

**What goes wrong:** Someone writes `mux.HandleFunc("/health", s.handleHealth)` (no `GET ` prefix). The route matches `GET /health`, `POST /health`, `DELETE /health`, everything. Tests that only exercise GET pass. When Phase 4 adds `POST /api/v1/registry` and the test suite grows, subtle routing surprises start appearing.

**Why it happens:** The method-prefix pattern is optional; ServeMux accepts both forms. Old tutorials and pre-1.22 code uses the unprefixed form.

**How to avoid:** Always use the method-prefixed form: `"GET /health"`, `"POST /api/v1/registry"`, etc. Phase 1's single route establishes the convention. The `TestHealth_RejectsWrongMethod` test enforces it.

**Warning signs:** Any `mux.HandleFunc(pattern, ...)` where `pattern` doesn't start with a known HTTP method word.

### Pitfall 5: `os.ReadFile` error wrapping loses `os.ErrNotExist`

**What goes wrong:** `config.Load` does `data, err := os.ReadFile(path)` and on error returns `fmt.Errorf("read config: %w", err)`. The caller can still `errors.Is(err, os.ErrNotExist)` because `%w` preserves the chain. But a naive implementation uses `fmt.Errorf("read config: %v", err)` (note `%v`, not `%w`), which loses the error type and breaks the "missing file → friendly message" branch.

**How to avoid:** Use `%w`, not `%v`, when wrapping errors that callers might inspect. The `Load` skeleton in Pattern 5 above gets this right: it checks `errors.Is(err, os.ErrNotExist)` **before** wrapping, which is cleaner than hoping the wrap propagates correctly.

**Warning signs:** `fmt.Errorf` with `%v` for error values anywhere in Phase 1 code.

### Pitfall 6: Makefile tabs vs spaces

**What goes wrong:** The developer writes the Makefile with 4-space indentation (because their editor auto-converts). `make build` runs and prints `Makefile:5: *** missing separator.  Stop.` because Makefile recipe lines must start with a tab.

**How to avoid:** Tell the planner explicitly: "the Makefile uses HARD TABS for recipe indentation; configure the editor accordingly." CI will catch this (make would fail), but a wasted round trip is easy to avoid.

**Warning signs:** Any Makefile recipe line that starts with spaces instead of `\t`.

### Pitfall 7: `go mod tidy` reformats `go.mod` unexpectedly

**What goes wrong:** Someone edits `go.mod` by hand to set `go 1.26`. Then `go get` rewrites it. Then `go mod tidy` reformats it again. The file churns and it's unclear which form is canonical.

**How to avoid:** Run `go mod init` → `go get` → `go mod tidy` in one scaffolding task, then commit the result and **never edit `go.mod` or `go.sum` by hand**. Use `go get` / `go get -u` / `go mod tidy` exclusively.

### Pitfall 8: Staticcheck finding in `_test.go` files for `testify/require`

**What goes wrong:** `staticcheck` may flag `require.NoError(t, err)` with ST1000 or similar if the test file doesn't have a package doc comment. Or it may be fine. Depends on staticcheck version.

**How to avoid:** After the first `make ci` run locally, read the output. If staticcheck complains about a test file, add a package doc comment (`// Package httpapi ...`) to the one non-test file in that package. Don't disable checks unless necessary.

**Phase to address:** Phase 1 (first CI run).

## Code Examples

See Architecture Patterns section above for:
- `cmd/server/main.go` (Pattern 1)
- `internal/httpapi/server.go` (Pattern 3)
- `internal/httpapi/health.go` (Pattern 3)
- `internal/httpapi/health_test.go` (Pattern 4)
- `internal/config/config.go` (Pattern 5)
- `internal/config/config_test.go` (Pattern 6)

### Additional concrete files Phase 1 must create

#### `.github/workflows/ci.yml`

```yaml
name: CI

on:
  push:
    branches: [ master, main ]
  pull_request:

jobs:
  test:
    name: Test + Lint + Build
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v6

      - name: Set up Go
        uses: actions/setup-go@v6
        with:
          go-version: '1.26'
          check-latest: true

      - name: Download modules
        run: go mod download

      - name: gofmt check
        run: |
          unformatted="$(gofmt -l .)"
          if [ -n "$unformatted" ]; then
            echo "The following files are not gofmt-clean:"
            echo "$unformatted"
            exit 1
          fi

      - name: go vet
        run: go vet ./...

      - name: staticcheck
        run: go run honnef.co/go/tools/cmd/staticcheck@latest ./...

      - name: go build
        run: go build ./...

      - name: go test (race)
        run: go test ./... -race -count=1
```

**Notes for planner:**
- `@v6` used for both checkout and setup-go per the version finding above. If the user pushes back, downgrade to `@v4`/`@v5` (they still work through June 2026).
- `check-latest: true` ensures the Go toolchain is always the latest 1.26.x patch.
- `gofmt` step uses a shell snippet because `gofmt -l .` returns exit 0 even when it finds unformatted files (it only prints their names). The snippet captures the output and fails on non-empty.
- Runner: `ubuntu-latest` only per PROJECT.md "Linux-first reference impl." No Windows, no macOS.
- Single job, sequential steps per CONTEXT.md ("single workflow, one job with steps").

Source: [GitHub Actions workflow syntax](https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions), [actions/setup-go README](https://github.com/actions/setup-go)

#### `Makefile`

```makefile
# OpenBuro server — developer commands

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -X github.com/openburo/openburo-server/internal/version.Version=$(VERSION)

BIN := bin/openburo-server

.PHONY: all build run test lint fmt ci clean help

all: build

build: ## Compile the server binary to bin/openburo-server
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/server

run: ## Run the server with ./config.yaml
	go run ./cmd/server -config config.yaml

test: ## Run tests with the race detector
	go test ./... -race -count=1

lint: ## Run gofmt check, go vet, and staticcheck
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt issues in:"; echo "$$unformatted"; exit 1; \
	fi
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@latest ./...

fmt: ## Rewrite files with gofmt
	gofmt -w .

ci: lint test build ## Run the full CI pipeline locally

clean: ## Remove build artifacts
	rm -rf bin/

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'
```

**Notes:**
- **Recipe lines use hard tabs**, not spaces. See Pitfall 6.
- `ci` target chains `lint test build` — mirrors the GitHub Actions flow. Running `make ci` locally catches everything CI catches.
- `help` target is included (CONTEXT.md marks it as Claude's discretion).
- `-count=1` in `test` disables Go's test caching so tests always actually run.
- `LDFLAGS` targets the exact symbol `internal/version.Version` per CONTEXT.md.

#### `config.example.yaml`

```yaml
# OpenBuro Server — example configuration
# Copy this file to config.yaml and edit to suit your deployment.

server:
  port: 8080
  tls:
    enabled: false
    cert_file: ""
    key_file: ""

# Path to the credentials file (bcrypt-hashed admin passwords).
# Loaded at startup; see credentials.example.yaml for the expected shape.
credentials_file: "./credentials.yaml"

# Path to the persistent registry store. Created on first write if absent.
registry_file: "./registry.json"

websocket:
  # How often the server sends ping frames to keep connections alive.
  ping_interval_seconds: 30

logging:
  # Format: "json" for production, "text" for local development.
  format: json
  # Level: debug | info | warn | error
  level: info

cors:
  # Explicit allow-list of origins. Leave empty to block all browser clients.
  # Shared with the WebSocket origin check in the API phase.
  allowed_origins: []
```

#### `credentials.example.yaml`

```yaml
# OpenBuro Server — example credentials
# Copy this file to credentials.yaml and replace the example hash.
#
# Generate a bcrypt hash (cost >= 12):
#   htpasswd -bnBC 12 "" your-password-here | tr -d ':\n'
# Or in Go:
#   bcrypt.GenerateFromPassword([]byte("your-password"), 12)

admins:
  - username: "admin"
    password_hash: "$2a$12$EXAMPLEEXAMPLEEXAMPLEEXAMPLEEXAMPLEEXAMPLEEXAMPLEEXAMPLEE"
```

**Note:** The example hash is a structurally-valid-looking but functionally-invalid bcrypt string. A developer copying this file and running the server in Phase 4 will get an auth error on first login attempt — exactly the desired behavior. Don't ship a hash that actually decrypts to "password" or similar.

#### `.gitignore` (verbatim from CONTEXT.md)

```
# Build outputs
bin/
*.test
*.out

# Local config (operator supplies real values)
config.yaml
credentials.yaml
registry.json

# IDE / OS cruft
.idea/
.vscode/
.DS_Store
```

#### `internal/version/version.go`

```go
// Package version exposes the build-time version string for the OpenBuro
// server. The default value "dev" applies when running via `go run`.
// Release builds inject a real version via ldflags:
//
//	go build -ldflags "-X github.com/openburo/openburo-server/internal/version.Version=$(git describe --tags --always --dirty)" ./cmd/server
package version

// Version is the build-time version string. Overridden via ldflags;
// defaults to "dev" for local `go run` invocations.
var Version = "dev"
```

#### `internal/registry/doc.go` (stub for TEST-07)

```go
// Package registry holds the in-memory manifest store, domain types
// (Manifest, Capability), MIME wildcard matching, and atomic JSON
// persistence. It is the pure domain core and depends on nothing
// from other internal/ packages.
//
// Phase 1 ships this file only; the real implementation lands in Phase 2.
package registry
```

#### `internal/wshub/doc.go` (stub for TEST-07)

```go
// Package wshub implements the WebSocket broadcast hub using the
// coder/websocket library. It holds a map of subscribers under a mutex
// and fans out events non-blockingly with drop-slow-consumer semantics.
//
// wshub intentionally knows nothing about the registry package — events
// are opaque byte slices supplied by the handler layer. This inversion
// keeps the dependency graph acyclic.
//
// Phase 1 ships this file only; the real implementation lands in Phase 3.
package wshub
```

### Directory Scaffold Commands (TEST-07)

The planner can script the layout creation as a single bash block. The commands are idempotent enough to use in a "scaffold" task that can be re-run safely:

```bash
# From repo root (already git-initialized, already has go.mod)
mkdir -p cmd/server
mkdir -p internal/config/testdata
mkdir -p internal/version
mkdir -p internal/registry
mkdir -p internal/wshub
mkdir -p internal/httpapi
mkdir -p .github/workflows

# Stub files are written via the planner's edit/write tasks, not touch.
# Listed here for clarity only:
#   cmd/server/main.go
#   internal/config/config.go
#   internal/config/config_test.go
#   internal/config/testdata/valid.yaml
#   internal/config/testdata/invalid-log-format.yaml
#   ... (other testdata fixtures)
#   internal/version/version.go
#   internal/registry/doc.go
#   internal/wshub/doc.go
#   internal/httpapi/server.go
#   internal/httpapi/health.go
#   internal/httpapi/health_test.go
#   config.example.yaml
#   credentials.example.yaml
#   Makefile
#   .gitignore
#   .github/workflows/ci.yml
```

**Important:** Use individual write tasks (not `touch` commands) for any file that has content. `touch` creates empty files; empty Go files don't compile and will break `go build`. Only `testdata/*.yaml` fixtures could be touched then filled, but even then, write the full content in one shot.

## State of the Art

Phase 1 is scaffolding, so "state of the art" mostly means "current tool versions." Domain patterns are stable.

| Old Approach | Current Approach (April 2026) | When Changed | Impact on Phase 1 |
|--------------|-------------------------------|--------------|-------------------|
| `actions/checkout@v4` + `setup-go@v5` | `actions/checkout@v6` + `actions/setup-go@v6` | 2025-2026 (Node 24 migration; forced by June 2026 Node 20 deprecation) | Use v6 in `ci.yml` |
| `gopkg.in/yaml.v3` | `go.yaml.in/yaml/v3` | April 2025 (original repo unmaintained, YAML org took over) | Use new canonical path in imports |
| `gorilla/websocket` | `coder/websocket` | 2023-2024 (gorilla archived) | Not Phase 1 concern (Phase 3) |
| Pre-1.22 router libs (chi, mux) | stdlib `http.ServeMux` with method patterns | Go 1.22, Feb 2024 | Use method-prefix form (`"GET /health"`) |
| `golangci-lint` aggregated | `gofmt` + `go vet` + `staticcheck` individually | CONTEXT.md decision | Three separate CI steps |
| `gopkg.in/yaml.v2` | `go.yaml.in/yaml/v3` | Pre-2020 already deprecated | Never use v2 |
| Package-level loggers (`logrus` style) | Injected `*slog.Logger` | Go 1.21 (slog landed), 2023 onward | CONTEXT.md hard rule |

**Deprecated / outdated — do NOT use in Phase 1:**
- `actions/checkout@v3` or earlier (Node 16, EOL)
- `actions/setup-go@v4` or earlier (Node 16)
- `gopkg.in/yaml.v2` (ancient, quirky)
- `log` stdlib package (use `log/slog`)
- `interface{}` for "any value" (use `any` — Go 1.18+; cosmetic but Phase 1's code should look modern)

## Open Questions

1. **GitHub Actions version: v4/v5 vs v6?**
   - What we know: CONTEXT.md specifies `@v4` / `@v5`; current state of the world is `@v6` for both
   - What's unclear: Whether the user specified v4/v5 intentionally or because they were the defaults at the time of discussion
   - Recommendation: **Plan with `@v6` and note the drift in the Plan document.** v6 is backward-compatible for the features Phase 1 uses, and v4/v5 will hit Node 20 EOL in June 2026. If the user pushes back during review, it's a one-character change in `ci.yml`.

2. **Does `/health` return `{"status":"ok"}` or just 200 with empty body?**
   - What we know: FOUND-04 says "returns 200 without requiring auth" — doesn't specify body
   - What's unclear: Whether a JSON body is expected for consistency with future routes
   - Recommendation: **Return `{"status":"ok"}` with `Content-Type: application/json`.** Establishes the convention other handlers will follow. ~3 extra lines. Easy to accept or reject in review.

3. **Does Phase 1 ship a minimal README?**
   - What we know: CONTEXT.md marks it as "planner decides" (deferred section: "Phase 1 may ship a minimal README stub or skip it entirely")
   - What's unclear: Whether a placeholder README is more or less confusing than its absence
   - Recommendation: **Ship a 5-10 line README.md with `make run` quickstart.** A GitHub repo with no README looks abandoned. A tiny stub that says "see .planning/ROADMAP.md and run `make run`" is more professional than nothing and costs 1 minute.

4. **Does `cmd/server/main.go` have a `main_test.go`?**
   - What we know: CONTEXT.md doesn't mention one
   - What's unclear: Whether the `newLogger` helper needs its own test
   - Recommendation: **Skip it for Phase 1.** `newLogger` is 20 lines of straightforward switch statements. The config_test.go already catches invalid format/level values before they reach `newLogger`. If Phase 5 adds graceful shutdown logic to `main.go`, a test becomes more valuable; add it then.

5. **Exact testify version to pull?**
   - What we know: STACK.md recommends v1.11.1 (2025-08-27); training data may be stale
   - What's unclear: Whether a newer version has shipped by April 2026
   - Recommendation: **Run `go get -t github.com/stretchr/testify/require@latest` and commit whatever lands.** Don't hand-pin; let `go.mod` record reality.

6. **Should `logger *slog.Logger` be the first or last field of `httpapi.Server`?**
   - What we know: Cosmetic question; no functional impact
   - Recommendation: **First.** Convention in Go is "context/logger first, then domain deps, then internal state." Phase 4 will extend the struct to `{logger, store, hub, creds, mux}`.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` + `github.com/stretchr/testify/require` v1.11.x |
| Config file | None (stdlib `go test` uses `go.mod` for discovery) |
| Quick run command | `go test ./internal/... -race -count=1 -short` |
| Full suite command | `go test ./... -race -count=1` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| **FOUND-01** | `go build ./...` succeeds on Go 1.26 with pinned deps | Build/static | `go build ./...` (exit 0) | Wave 0 — scaffold creates files |
| **FOUND-02** | `config.Load` parses valid YAML, rejects invalid | Unit (table) | `go test ./internal/config -run TestLoad -race` | Wave 0 — `internal/config/config_test.go` + `testdata/` |
| **FOUND-02** | `config.Load` returns friendly error on missing file | Unit | `go test ./internal/config -run TestLoad_MissingFile` | Wave 0 |
| **FOUND-03** | `*slog.Logger` is constructible from `logging.format`/`logging.level` without panic | Static / smoke | `go vet ./...` + successful `go run ./cmd/server -config testdata/valid.yaml` (first line is banner) | Wave 0 — implicit via TestLoad (validates format/level before construction) |
| **FOUND-03** | No `slog.Default()` or bare `slog.*` calls in `internal/` | Grep check | `! rg 'slog\\.(Info\|Debug\|Warn\|Error\|Default)' internal/` | Static (no test file — validation is a grep gate) |
| **FOUND-04** | `GET /health` returns 200 with no Authorization header | Unit (httptest) | `go test ./internal/httpapi -run TestHealth -race` | Wave 0 — `internal/httpapi/health_test.go` |
| **FOUND-04** | `POST /health` returns 405 (method patterns wired correctly) | Unit (httptest) | `go test ./internal/httpapi -run TestHealth_RejectsWrongMethod` | Wave 0 |
| **FOUND-05** | Startup banner logs the 10 required keys with correct names | Manual / smoke | Run `make run` against `config.example.yaml` (copied), grep output for `"openburo server starting"` and the 10 key names | Manual-only (no test captures the banner in Phase 1; Phase 4 adds slog capture infrastructure for the "no credentials in logs" test) |
| **FOUND-06** | CI runs `go test -race`, `go vet`, `gofmt` | Static (file check) | `test -f .github/workflows/ci.yml && grep -q 'go test ./... -race' .github/workflows/ci.yml && grep -q 'go vet ./...' .github/workflows/ci.yml && grep -q 'gofmt -l' .github/workflows/ci.yml` | Wave 0 — `.github/workflows/ci.yml` |
| **FOUND-06** | CI workflow YAML is valid GitHub Actions syntax | Static | (GitHub parses it on push; locally verifiable with `actionlint` if installed, otherwise visual review) | Manual-only (no actionlint dep added in Phase 1) |
| **FOUND-07** | `config.example.yaml` exists at repo root | Static (file check) | `test -f config.example.yaml` | Static |
| **FOUND-07** | `credentials.example.yaml` exists at repo root | Static (file check) | `test -f credentials.example.yaml` | Static |
| **FOUND-07** | `config.example.yaml` contains all required keys | Static (grep) | `grep -q 'server:' config.example.yaml && grep -q 'credentials_file:' config.example.yaml && grep -q 'registry_file:' config.example.yaml && grep -q 'websocket:' config.example.yaml && grep -q 'logging:' config.example.yaml && grep -q 'cors:' config.example.yaml` | Static |
| **TEST-07** | Directory layout matches spec | Static (file checks) | `test -d cmd/server && test -d internal/config && test -d internal/registry && test -d internal/wshub && test -d internal/httpapi && test -d internal/version` | Static |
| **TEST-07** | `internal/registry/doc.go` exists (stub) | Static | `test -f internal/registry/doc.go` | Static |
| **TEST-07** | `internal/wshub/doc.go` exists (stub) | Static | `test -f internal/wshub/doc.go` | Static |
| **Meta** | `gofmt` check passes | Static | `test -z "$(gofmt -l .)"` | Static |
| **Meta** | `go vet` passes | Static | `go vet ./...` (exit 0) | Static |
| **Meta** | `staticcheck` passes | Static | `go run honnef.co/go/tools/cmd/staticcheck@latest ./...` (exit 0) | Static |

### Sampling Rate

- **Per task commit:** `go build ./... && go test ./internal/... -race -count=1` (completes in <10s on Phase 1's tiny codebase)
- **Per wave merge:** `make ci` (runs lint + test + build — mirrors GitHub Actions locally, ~30s first run due to staticcheck download, <15s subsequent)
- **Phase gate:** `make ci` green + all structural checks (file existence, grep patterns) pass + one manual smoke run of `make run` confirming the banner appears

### Wave 0 Gaps

Phase 1 IS Wave 0 — there is no pre-existing code. Every file listed below must be created before validation can run:

- [ ] `go.mod` — from `go mod init github.com/openburo/openburo-server`
- [ ] `go.sum` — from `go mod tidy` after adding yaml + testify
- [ ] `cmd/server/main.go` — compose root (FOUND-02, FOUND-03, FOUND-05)
- [ ] `internal/config/config.go` — Config struct, Load, validate (FOUND-02)
- [ ] `internal/config/config_test.go` — table-driven tests (FOUND-02)
- [ ] `internal/config/testdata/valid.yaml` — fixture (FOUND-02)
- [ ] `internal/config/testdata/invalid-log-format.yaml` — fixture
- [ ] `internal/config/testdata/invalid-log-level.yaml` — fixture
- [ ] `internal/config/testdata/missing-credentials-file.yaml` — fixture
- [ ] `internal/config/testdata/zero-port.yaml` — fixture
- [ ] `internal/config/testdata/tls-no-cert.yaml` — fixture
- [ ] `internal/config/testdata/zero-ping.yaml` — fixture
- [ ] `internal/config/testdata/malformed.yaml` — fixture
- [ ] `internal/version/version.go` — `var Version = "dev"` + package doc
- [ ] `internal/registry/doc.go` — package stub (TEST-07)
- [ ] `internal/wshub/doc.go` — package stub (TEST-07)
- [ ] `internal/httpapi/server.go` — Server struct + New + Handler + registerRoutes
- [ ] `internal/httpapi/health.go` — handleHealth (FOUND-04)
- [ ] `internal/httpapi/health_test.go` — TestHealth + TestHealth_RejectsWrongMethod (FOUND-04)
- [ ] `config.example.yaml` — example config (FOUND-07)
- [ ] `credentials.example.yaml` — example credentials (FOUND-07)
- [ ] `Makefile` — build, run, test, lint, fmt, ci, clean, help
- [ ] `.gitignore` — verbatim from CONTEXT.md
- [ ] `.github/workflows/ci.yml` — CI workflow (FOUND-06)

No existing test infrastructure — everything is greenfield. The validation strategy is heavily structural (file existence, grep for expected content, `go build` exit code, `go vet` exit code) plus one behavioral check: `TestHealth` proves `/health` returns 200 without an Authorization header via `httptest.NewRecorder`.

**Key insight for the validation step:** Phase 1's "am I done?" signal is 80% structural and 20% behavioral. The downstream validation-strategy document should weight static checks (file exists, file contains expected strings, `go build` exit 0, `go test ./internal/... -race` exit 0) heavily, and rely on the single `TestHealth` behavioral test for FOUND-04. The startup banner (FOUND-05) intentionally lacks an automated behavioral test in Phase 1 — writing a main_test.go that captures slog output from a subprocess is disproportionate effort for a reference impl at this stage. Phase 4 will build the slog-capture test infrastructure (for the "no credentials in logs" test), at which point FOUND-05 can be back-tested cheaply if desired.

## Sources

### Primary (HIGH confidence)
- `.planning/phases/01-foundation/01-CONTEXT.md` — locked decisions
- `.planning/REQUIREMENTS.md` — FOUND-01..07 and TEST-07 definitions
- `.planning/research/STACK.md` — library choices (yaml v3, testify, Go 1.26)
- `.planning/research/ARCHITECTURE.md` — package layout, Server struct pattern, dependency graph
- `.planning/research/PITFALLS.md` §12 (defer Unlock) and §11/§13 (credentials in logs) — log-safety rules
- [Go blog: Routing Enhancements for Go 1.22](https://go.dev/blog/routing-enhancements) — method-prefixed ServeMux patterns
- [pkg.go.dev: log/slog](https://pkg.go.dev/log/slog) — handler construction, `*slog.Logger`
- [pkg.go.dev: go.yaml.in/yaml/v3](https://pkg.go.dev/go.yaml.in/yaml/v3) — canonical YAML import path

### Secondary (MEDIUM confidence — verified via web search April 2026)
- [actions/setup-go releases](https://github.com/actions/setup-go/releases) — verified `@v6` is the current major, supports Go 1.26 and the `toolchain` directive
- [actions/checkout releases](https://github.com/actions/checkout/releases) — verified `@v6` is the current major
- [dominikh/go-tools releases](https://github.com/dominikh/go-tools/releases) — verified staticcheck has Feb 2026 release with Go 1.26 support
- [GitHub Actions workflow syntax docs](https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions)

### Tertiary (LOW confidence — flag for validation during implementation)
- Exact latest patch versions of `go.yaml.in/yaml/v3` and `stretchr/testify` — resolve via `go get @latest` during scaffolding; don't trust training data versions
- Exact staticcheck output on Phase 1's code — staticcheck may flag something pedantic (e.g., a missing package comment); address during first CI run

## Metadata

**Confidence breakdown:**
- User constraints extraction: HIGH — copied verbatim from CONTEXT.md
- Config package shape: HIGH — standard yaml.v3 + struct validation pattern, no novel ground
- slog constructor location: HIGH — follows CONTEXT.md's injection rule with the cleanest enforcement
- httpapi.Server scaffold: HIGH — directly follows ARCHITECTURE.md Pattern 1
- health handler + test: HIGH — httptest is stdlib, the pattern is canonical
- GitHub Actions workflow: MEDIUM — version pinning (@v6 vs @v5) is a current-state call; everything else is locked
- Makefile shape: HIGH — standard Go project targets + LDFLAGS pattern from CONTEXT.md
- Example YAML files: HIGH — shape is locked in CONTEXT.md
- Directory scaffold: HIGH — layout is locked in CONTEXT.md; mkdir commands are mechanical
- `.gitignore`: HIGH — verbatim from CONTEXT.md
- Validation architecture: HIGH — Phase 1's structural-over-behavioral weighting is self-evident from the requirement set

**Research date:** 2026-04-09
**Valid until:** 2026-05-09 (30 days — stable scaffolding domain; only GitHub Actions versions move faster)
