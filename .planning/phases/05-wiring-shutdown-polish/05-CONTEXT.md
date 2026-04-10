# Phase 5: Wiring, Shutdown & Polish — Context

**Gathered:** 2026-04-10 (compaction mode)
**Status:** Ready for planning

<domain>
## Phase Boundary

Replace the minimal Phase 4 `cmd/server/main.go` wiring with a full compose-root (≤100 lines) that: loads `config.yaml` + `credentials.yaml`, constructs the logger/store/hub/httpapi.Server, runs the HTTP server in a goroutine so main can wait on a signal-aware context, then performs two-phase graceful shutdown (`httpSrv.Shutdown` → `hub.Close` sending `StatusGoingAway` frames). Add optional TLS via `ListenAndServeTLS` when `server.tls.enabled = true`. Add a README with quickstart. Make the whole-module `go test ./... -race` gate a permanent CI fixture.

**In scope (5 requirements):** OPS-02, OPS-03, OPS-04, OPS-05, TEST-03

**Out of scope:** nothing — this is the last phase of the milestone.

</domain>

<decisions>
## Implementation Decisions

All builder-internal per user delegation. Locked:

### main.go structure (OPS-02)

One `run(ctx) error` function. Signal-aware context via `signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)`. Compose-root wiring order: config → logger → store → hub → credentials → httpapi.Server → http.Server. HTTP server runs in a goroutine; main blocks on `<-ctx.Done()` then runs two-phase shutdown with a 15-second budget.

Target: ≤100 lines total (including the existing `newLogger` helper). Counted excluding blank lines and comments.

### Two-phase graceful shutdown (OPS-03, OPS-04)

1. Signal arrives → `ctx.Done()` fires
2. Log `server shutting down`
3. Phase A: `shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second); httpSrv.Shutdown(shutdownCtx)` — stops accepting new connections, drains in-flight HTTP requests
4. Phase B: `hub.Close()` — sends `StatusGoingAway` close frames to every WebSocket subscriber (each writer loop then exits cleanly)
5. Return nil

**Critical:** `http.Server.Shutdown` does NOT close hijacked WebSocket connections (PITFALLS #6). The two-phase order is load-bearing. Hub.Close MUST run AFTER httpSrv.Shutdown, not before.

If `httpSrv.Shutdown` returns an error (e.g. timeout), log it but still call `hub.Close()`, then return the error.

### Optional TLS (OPS-05)

```go
if cfg.Server.TLS.Enabled {
    err := httpSrv.ListenAndServeTLS(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile)
    ...
} else {
    err := httpSrv.ListenAndServe()
    ...
}
```

Both branches ignore `http.ErrServerClosed` (normal shutdown path). Any other error is fatal.

### LoadCredentials wiring

Replace `httpapi.Credentials{}` with `httpapi.LoadCredentials(cfg.CredentialsFile)` — a real call, fail-fast if the file is missing or has cost < 12. This closes the gap from Phase 4 where main.go passed an empty table.

### Test strategy for graceful shutdown

`cmd/server/main.go` is a command package — testing `main()` directly is awkward. Solution: extract the compose-root logic into a testable `run(ctx context.Context) error` function, then write `cmd/server/main_test.go` with a `TestGracefulShutdown` test that:
1. Creates a temp config file pointing to a temp credentials file (cost-12 hash)
2. Spawns `run(ctx)` in a goroutine
3. Waits until the server is listening (retry loop with tight timeout via `net.Dial`)
4. Cancels ctx
5. `require.Eventually`s that `run` returned nil within 20 seconds
6. Asserts no goroutine leak via the same pattern as Phase 3's leak test

**Alternative considered, rejected:** a pure `run_test.go` that doesn't actually bind a socket — rejected because TEST-03 explicitly wants "whole-module -race" coverage including the compose-root, and a network-less test would skip the real shutdown path.

### README content (part of OPS-02 polish per ROADMAP success criteria)

Sections: Overview, Quickstart (5 steps), Configuration, API Reference (one-liner per endpoint), Development (Makefile targets), Architecture diagram (text), Known Limitations, License placeholder.

Target length: ~200-300 lines. Not a book; a reference-impl README.

### Port handling for tests

The test uses port 0 (ephemeral) by writing a test-only config with `server.port: 0`. The `httpSrv.Addr` field is overridden to `:0` in `run` OR we inject a test hook. **Simpler decision:** test uses a fixed high port (e.g. 18089) and relies on `require.Eventually` to tolerate concurrent test runs. If that flakes, switch to an `httptest.NewServer` pattern in a follow-up.

### Error-wrapping voice

Match Phases 1-4: `fmt.Errorf("context: %w", err)` with lowercase context strings, no trailing punctuation.

### Logger on shutdown

Log at Info on clean exit, Error on shutdown failures. Never log credentials.

### What stays in main.go vs moves to an internal package

Everything stays in `cmd/server/main.go`. No new internal package. The compose-root is allowed to be the only place that imports `config`, `httpapi`, `registry`, `wshub` simultaneously — this is the whole point of compose-root pattern.

</decisions>

<canonical_refs>
## Canonical References

- `.planning/REQUIREMENTS.md` §Operations (OPS-02..05), §Testing (TEST-03)
- `.planning/ROADMAP.md` §"Phase 5: Wiring, Shutdown & Polish"
- `.planning/research/PITFALLS.md` §6 (graceful shutdown does NOT close hijacked WS — two-phase required)
- `.planning/phases/01-foundation/01-CONTEXT.md` — logger pattern, no slog.Default
- `.planning/phases/04-http-api/04-CONTEXT.md` — httpapi.New signature (*Server, error)
- `.planning/phases/03-websocket-hub/03-CONTEXT.md` — Hub.Close contract (sends StatusGoingAway)
- `cmd/server/main.go` — existing Phase 4 minimal wiring to EXPAND

</canonical_refs>

<code_context>

### Reusable Assets
- `cmd/server/main.go` — Phase 4 minimal version; `newLogger` helper stays as-is
- `internal/httpapi` — `New(logger, store, hub, creds, cfg) (*Server, error)`, `Handler() http.Handler`, `LoadCredentials(path) (Credentials, error)`
- `internal/registry` — `NewStore(path) (*Store, error)` (idempotent)
- `internal/wshub` — `New(logger, opts)`, `Close()` (idempotent, sends StatusGoingAway)
- `internal/config` — `Load(path) (*Config, error)` validates everything including TLS cert/key when enabled

### Established Patterns
- Logger injection only, no `slog.Default()` (Phase 1 lock)
- Error wrapping via `fmt.Errorf("context: %w", err)`
- Tests use testify/require + `require.Eventually` (no `time.Sleep`)
- Go 1.26 toolchain via `~/sdk/go1.26.2/bin/go`

### Integration Points
- Phase 4 httpapi.LoadCredentials is the credential loader — Phase 5 calls it
- Phase 3 wshub.Hub.Close is the shutdown sink for WS subscribers
- config.Config is the single source of truth for TLS, ports, paths

</code_context>

<specifics>
- Two-phase shutdown is THE load-bearing invariant for Phase 5. The test must prove the WS client receives a clean StatusGoingAway frame, not a TCP reset.
- `http.Server.Shutdown` explicitly does NOT close hijacked connections — this is why Hub.Close is Phase B.
- `cmd/server/main.go` is the FIRST file a reviewer reads. It must be pristine and ≤100 lines.

</specifics>

<deferred>
- Hot-reload of credentials.yaml (v2 OPS-V2-01)
- Prometheus /metrics (v2 OPS-V2-02)
- OpenTelemetry tracing (v2 OPS-V2-03)
- Rate limiting (v2 SEC-V2-02)
- OAuth/OIDC (v2 SEC-V2-03)

</deferred>

---

*Phase: 05-wiring-shutdown-polish*
*Context gathered: 2026-04-10 (compaction mode — no researcher, no plan-checker, execution inline)*
