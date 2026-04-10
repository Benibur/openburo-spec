---
plan: 05-01
title: Compose-root + two-phase graceful shutdown + optional TLS + README
wave: 1
depends_on: []
autonomous: true
gap_closure: false
files_modified:
  - cmd/server/main.go
  - cmd/server/main_test.go
  - README.md
requirements_addressed: [OPS-02, OPS-03, OPS-04, OPS-05, TEST-03]
---

# Plan 05-01 — Compose-root + Two-Phase Shutdown + TLS + README

<objective>
Replace the minimal Phase 4 `cmd/server/main.go` with the full compose-root (≤100 lines): signal-aware context, real `LoadCredentials`, HTTP server in a goroutine, two-phase graceful shutdown (`httpSrv.Shutdown` → `hub.Close`), optional TLS via `ListenAndServeTLS`. Add a whole-module race-clean test (`TestGracefulShutdown`) that proves the shutdown path works end-to-end. Ship a README with quickstart. After this plan the milestone v1.0 is complete.
</objective>

<must_haves>
## Load-bearing truths (goal-backward)

1. SIGTERM/SIGINT triggers `httpSrv.Shutdown(ctx)` BEFORE `hub.Close()` — two-phase ordering is verified by line-order grep in main.go.
2. On clean shutdown, the process exits with code 0 and an active WebSocket subscriber observes a `StatusGoingAway` close frame (not a TCP reset).
3. `cmd/server/main.go` source excluding blank lines and comments is ≤100 lines (OPS-02 polish criterion).
4. When `cfg.Server.TLS.Enabled` is true, the server calls `ListenAndServeTLS` with the cert/key from config; otherwise `ListenAndServe`.
5. `LoadCredentials(cfg.CredentialsFile)` is called and errors surface as startup failures.
6. `~/sdk/go1.26.2/bin/go test ./... -race -count=1` is green across the whole module.
</must_haves>

<tasks>

<task id="05-01-01" tdd="true" name="RED — graceful shutdown test">
<read_first>
- cmd/server/main.go (existing)
- .planning/phases/05-wiring-shutdown-polish/05-CONTEXT.md
- .planning/phases/03-websocket-hub/03-CONTEXT.md (Hub.Close contract)
- internal/httpapi/integration_test.go (testdata pattern for credentials fixture)
- internal/httpapi/testdata/credentials-valid.yaml (existing cost-12 hash we can reuse)
</read_first>

<action>
Create `cmd/server/main_test.go` with the following test:

```go
package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGracefulShutdown(t *testing.T) {
	dir := t.TempDir()

	// Copy the httpapi testdata credentials-valid.yaml (cost-12 hash) into a
	// temp file in the form main's config expects.
	credsPath := filepath.Join(dir, "credentials.yaml")
	credsContent := `users:
  testuser: "$2a$12$3rxAwpdXPdxbsrOPg3P/LuSHwLnaMAAeKnoBILOMB.4.mGX.ehFkG"
`
	require.NoError(t, os.WriteFile(credsPath, []byte(credsContent), 0o600))

	regPath := filepath.Join(dir, "registry.json")
	cfgPath := filepath.Join(dir, "config.yaml")
	cfgContent := fmt.Sprintf(`server:
  port: 18089
  tls:
    enabled: false
credentials_file: %q
registry_file: %q
websocket:
  ping_interval_seconds: 30
logging:
  format: text
  level: error
cors:
  allowed_origins:
    - "http://localhost:3000"
`, credsPath, regPath)
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfgContent), 0o600))

	// Override os.Args so the -config flag picks up our temp file.
	oldArgs := os.Args
	os.Args = []string{"openburo-server-test", "-config", cfgPath}
	defer func() { os.Args = oldArgs }()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- run(ctx) }()

	// Wait for the listener to be ready (retry 1 s total).
	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:18089", 50*time.Millisecond)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, 2*time.Second, 25*time.Millisecond, "server never bound 127.0.0.1:18089")

	// Trigger shutdown.
	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err, "run() should return nil on clean shutdown")
	case <-time.After(20 * time.Second):
		t.Fatal("run() did not return within 20s of ctx cancel")
	}
}
```

**Why this test matters:** it's the only test that actually exercises `run()` end-to-end including signal handling, listener binding, and two-phase shutdown. It's the TEST-03 anchor.

**Expected RED behavior:** the test will fail to compile because `run` currently doesn't accept a `context.Context` argument — Task 2 GREEN refactors `run` to `run(ctx context.Context) error`.
</action>

<verify>
<automated>~/sdk/go1.26.2/bin/go test ./cmd/server -race -run '^TestGracefulShutdown$' -timeout 30s 2>&1 | tail -20</automated>
</verify>

<acceptance_criteria>
- File `cmd/server/main_test.go` exists
- File compiles OR fails with `run(ctx)`-signature mismatch — that's the RED signal
- File imports `context`, `net`, `time`, `testify/require`
- Test function named `TestGracefulShutdown`
- Test uses port 18089 and `require.Eventually` (no `time.Sleep`)
</acceptance_criteria>

<done>
Task 1 RED complete when the new test file exists and `go test ./cmd/server -run TestGracefulShutdown` fails (any failure mode — compile error, timeout, or runtime panic — is acceptable RED).
</done>
</task>

<task id="05-01-02" tdd="true" name="GREEN — compose-root rewrite + two-phase shutdown + TLS + README">
<read_first>
- cmd/server/main.go (current — to be replaced)
- cmd/server/main_test.go (just created — must compile and pass)
- .planning/phases/05-wiring-shutdown-polish/05-CONTEXT.md
- internal/httpapi/credentials.go (LoadCredentials signature)
- internal/wshub/hub.go (Close signature)
- .planning/REQUIREMENTS.md §Operations (OPS-02..05)
</read_first>

<action>
**Step 1: Rewrite `cmd/server/main.go`** to this exact structure:

```go
// Command openburo-server runs the OpenBuro capability broker.
//
// The compose-root wires config → logger → registry store → websocket hub →
// httpapi server → http.Server, then blocks on a signal-aware context and
// performs a two-phase graceful shutdown on SIGINT/SIGTERM.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/openburo/openburo-server/internal/config"
	"github.com/openburo/openburo-server/internal/httpapi"
	"github.com/openburo/openburo-server/internal/registry"
	"github.com/openburo/openburo-server/internal/version"
	"github.com/openburo/openburo-server/internal/wshub"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
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
		"registry_file", cfg.RegistryFile,
		"ping_interval", cfg.WebSocket.PingInterval.String(),
	)

	store, err := registry.NewStore(cfg.RegistryFile)
	if err != nil {
		return fmt.Errorf("load registry: %w", err)
	}

	creds, err := httpapi.LoadCredentials(cfg.CredentialsFile)
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}

	hub := wshub.New(logger, wshub.Options{PingInterval: cfg.WebSocket.PingInterval})

	srv, err := httpapi.New(logger, store, hub, creds, httpapi.Config{
		AllowedOrigins: cfg.CORS.AllowedOrigins,
		WSPingInterval: cfg.WebSocket.PingInterval,
	})
	if err != nil {
		return fmt.Errorf("build httpapi server: %w", err)
	}

	httpSrv := &http.Server{Addr: cfg.Server.Addr(), Handler: srv.Handler()}

	serveErr := make(chan error, 1)
	go func() {
		if cfg.Server.TLS.Enabled {
			serveErr <- httpSrv.ListenAndServeTLS(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile)
			return
		}
		serveErr <- httpSrv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http serve: %w", err)
		}
		return nil
	case <-ctx.Done():
		logger.Info("server shutting down")
	}

	// Phase A: stop accepting new connections, drain in-flight HTTP requests.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	shutdownErr := httpSrv.Shutdown(shutdownCtx)

	// Phase B: close WebSocket subscribers with StatusGoingAway (PITFALLS #6).
	hub.Close()

	if shutdownErr != nil {
		return fmt.Errorf("http shutdown: %w", shutdownErr)
	}
	logger.Info("server stopped cleanly")
	return nil
}

// newLogger builds a *slog.Logger from config.
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

**Step 2: Write `README.md`** at repo root with:

- Title + 1-paragraph overview pointing to the OpenBuro spec
- **Quickstart** (5 steps): copy configs, generate bcrypt hash, build, run, test with curl
- **Configuration** (table of config.yaml fields)
- **API Reference** (one-liner per REST endpoint + WS endpoint)
- **Development** (Makefile targets + gsd workflow note)
- **Architecture** (4-package internal/ layout text diagram)
- **Known Limitations** (reference-impl caveats: trust X-Forwarded-For, no hot-reload, no metrics, etc.)
- **License** placeholder

Target: ~200 lines.

**Step 3: Run the full module test suite** to verify TEST-03:

```
~/sdk/go1.26.2/bin/go test ./... -race -count=1 -timeout 240s
```

Expected: all 5 packages green (config, httpapi, registry, wshub, cmd/server).

**Step 4: Verify main.go is ≤100 lines** (excluding blank lines and comments):

```
grep -cvE '^\s*(//|$)' cmd/server/main.go
```

Should return ≤100.
</action>

<verify>
<automated>~/sdk/go1.26.2/bin/go test ./cmd/server -race -run '^TestGracefulShutdown$' -timeout 30s && ~/sdk/go1.26.2/bin/go test ./... -race -count=1 -timeout 240s 2>&1 | tail -10</automated>
</verify>

<acceptance_criteria>
- `cmd/server/main.go` compiles under `go build ./...`
- `grep -cvE '^\s*(//|$)' cmd/server/main.go → ≤100`
- `grep -c "signal.NotifyContext" cmd/server/main.go → 1`
- `grep -c "syscall.SIGTERM" cmd/server/main.go → 1`
- `grep -c "httpapi.LoadCredentials" cmd/server/main.go → 1`
- `grep -c "httpSrv.Shutdown" cmd/server/main.go → 1`
- `grep -c "hub.Close()" cmd/server/main.go → 1`
- Line-order: `httpSrv.Shutdown` line number < `hub.Close()` line number (two-phase ordering)
- `grep -c "ListenAndServeTLS" cmd/server/main.go → 1`
- `README.md` exists at repo root
- `README.md` contains sections: Quickstart, Configuration, API Reference, Development, Architecture, Known Limitations
- `~/sdk/go1.26.2/bin/go test ./cmd/server -race -run TestGracefulShutdown` exits 0
- `~/sdk/go1.26.2/bin/go test ./... -race -count=1` exits 0 for all 4 test packages
- `~/sdk/go1.26.2/bin/go vet ./...` clean
- `gofmt -l cmd/server/` empty
- `! grep -rE 'slog\.Default' cmd/server/*.go | grep -v _test.go` (no slog.Default in production)
</acceptance_criteria>

<done>
Task 2 GREEN complete when all acceptance criteria pass, `TestGracefulShutdown` is green under `-race`, and the whole-module race test is clean.
</done>
</task>

</tasks>

<verification>
Phase 5 is complete when:
- `~/sdk/go1.26.2/bin/go test ./... -race -count=1` returns 0 across all packages including `cmd/server`
- `TestGracefulShutdown` passes and proves the two-phase shutdown ordering
- `cmd/server/main.go` is ≤100 non-blank-non-comment lines
- `README.md` exists with the listed sections
- OPS-02, OPS-03, OPS-04, OPS-05, TEST-03 all marked Complete in REQUIREMENTS.md
</verification>
