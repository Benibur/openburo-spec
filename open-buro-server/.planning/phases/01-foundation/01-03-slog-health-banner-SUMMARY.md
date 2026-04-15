---
phase: 01-foundation
plan: 03
subsystem: infra
tags: [go, slog, http, servemux, httpapi, health, logging, compose-root]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: "Go module + internal/config Load() + Config type tree + internal/version.Version + internal/httpapi package stub"
provides:
  - "internal/httpapi.Server minimal scaffold (logger + mux) with New(logger) constructor and Handler() method"
  - "GET /health handler returning 200 application/json {\"status\":\"ok\"} with no auth requirement (FOUND-04)"
  - "Go 1.22+ method-prefixed ServeMux pattern locked in: POST/PUT/DELETE /health -> 405"
  - "cmd/server/main.go compose-root: flag parsing, config.Load, inline newLogger, 10-key startup banner, httpapi.New, http.Server.ListenAndServe"
  - "Injection-first slog discipline: no bare slog.(Info|Debug|Warn|Error|Default)( anywhere in internal/ (grep gate enforced)"
  - "Startup banner contract: single slog.Info(\"openburo server starting\", ...) with 10 locked keys in exact order"
affects:
  - "Phase 2 (registry handlers will be added to httpapi.Server as new fields + new routes in registerRoutes)"
  - "Phase 3 (wshub will be added to httpapi.Server as hub field + /api/v1/subscribe route)"
  - "Phase 4 (middleware chain wraps srv.Handler(); credentials field added; full route set registered)"
  - "Phase 5 (replaces ListenAndServe with signal.NotifyContext + two-phase graceful shutdown)"

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Compose-root main.go: run() returns error, main() is a fatal-exit shim; standard testable-main idiom"
    - "Injection-first slog: logger constructed in main.go only, passed by reference into every component that logs; no internal/logging package"
    - "httpapi.Server shape: logger + mux fields, New(logger) constructor, Handler() -> http.Handler, registerRoutes() method"
    - "Go 1.22 method-prefixed ServeMux patterns: mux.HandleFunc(\"GET /health\", ...) — the METHOD prefix is load-bearing for automatic 405s"
    - "Literal-body JSON for constant responses (w.Write([]byte(`{\"status\":\"ok\"}`))), not json.NewEncoder — encoder is reserved for dynamic envelopes in Phase 4"
    - "Health handler silence: handleHealth deliberately does NOT call s.logger.* or touch r.Header (noise reduction + credential-leak prevention per PITFALLS #13)"

key-files:
  created:
    - internal/httpapi/server.go
    - internal/httpapi/health.go
    - internal/httpapi/health_test.go
  modified:
    - cmd/server/main.go
  deleted:
    - internal/httpapi/doc.go
    - internal/httpapi/doc_test.go

key-decisions:
  - "newLogger lives inline in cmd/server/main.go, NOT in a new internal/logging package (RESEARCH Pattern 2). The physical absence of an internal/logging package makes it impossible for a future contributor to add a Default() helper that would undermine the inject-everywhere rule."
  - "handleHealth does not log. Health endpoints are the noisiest routes in any service; logging them pollutes logs and Phase 4's log middleware will explicitly skip /health for the same reason. This is a permanent convention."
  - "Server struct starts with only logger + mux. store (Phase 2), hub (Phase 3), creds (Phase 4) fields will be added incrementally — the minimal Phase 1 shape locks the constructor pattern but not the full dependency list."
  - "httpapi.Server does not own *config.Config. Only the compose-root reads config; every setting that httpapi needs is passed as an explicit constructor argument. Keeps the package decoupled from YAML shape."
  - "Used w.Write([]byte(`{\"status\":\"ok\"}`)) for the health body, not json.NewEncoder, per RESEARCH Pattern 3 rationale (constant payload, no trailing newline, no theoretical error path)."
  - "Phase 1 intentionally has no graceful shutdown. ListenAndServe blocks and returns on Ctrl-C with an unhandled error — Phase 5 adds signal.NotifyContext + two-phase shutdown."

patterns-established:
  - "10-key startup banner contract (frozen): version, go_version, listen_addr, tls_enabled, config_file, credentials_file, registry_file, ping_interval, log_format, log_level. Reordering or removing keys requires a CONTEXT.md update."
  - "Grep gate enforced in every future plan: ! grep -rE 'slog\\.(Info|Debug|Warn|Error|Default)\\(' internal/ — catches any regression into bare slog calls. Method calls on injected loggers (logger.Info, s.logger.Debug) don't match this pattern and remain allowed."
  - "TDD discipline for httpapi handlers: test file lands RED first (test-only commit), then server.go + handler.go land GREEN. Phase 4's route additions should follow the same test-first sequence."
  - "Anchor-file disposal: Plan 01-01's internal/httpapi/doc.go + doc_test.go were deleted once the real server.go/health.go/health_test.go files exist. No future phase should keep 'Phase 1 anchor' files alive after the real code lands."

requirements-completed:
  - FOUND-03
  - FOUND-04
  - FOUND-05

# Metrics
duration: 3min
completed: 2026-04-10
---

# Phase 01 Plan 03: slog + /health + Startup Banner Summary

**Ships a working single-binary OpenBuro server: httpapi.Server with GET /health, compose-root main.go with a 10-key slog startup banner, and the injection-first logging discipline locked in via a cross-tree grep gate.**

## Performance

- **Duration:** ~3 min
- **Started:** 2026-04-10T08:43:49Z
- **Completed:** 2026-04-10T08:46:55Z
- **Tasks:** 2 (one TDD, one straight auto)
- **Commits:** 3 (RED test commit + GREEN Task 1 + Task 2)
- **Files created:** 3 (server.go, health.go, health_test.go)
- **Files modified:** 1 (cmd/server/main.go)
- **Files deleted:** 2 (internal/httpapi/doc.go, internal/httpapi/doc_test.go — Plan 01-01 anchors)

## Accomplishments

- **Working binary:** `go run ./cmd/server -config <path>` loads config, constructs an injected slog logger, emits the 10-key startup banner as a single JSON line on stderr, and serves GET /health on the configured port.
- **Smoke-tested live:** GET /health returned `HTTP/1.1 200 OK` with `Content-Type: application/json` and body `{"status":"ok"}`; POST /health returned 405; the banner JSON line contained all 10 keys in locked order including `version=dev`, `go_version=go1.26.2`, `ping_interval=30s`, `log_format=json`, `log_level=info`.
- **httpapi.Server scaffold shipped** with the shape Phases 2-4 will extend: `logger + mux` fields today, `store/hub/creds` fields to be added in Phases 2/3/4 without touching the constructor contract.
- **Go 1.22 method-pattern discipline locked in:** `mux.HandleFunc("GET /health", ...)` — TestHealth_RejectsWrongMethod confirms POST/PUT/DELETE all yield 405 (a subtle-but-important check: without the `GET ` prefix the mux silently accepts all methods).
- **Injection-first slog invariant enforced by grep gate:** `! grep -rE 'slog\\.(Info|Debug|Warn|Error|Default)\\(' internal/` passes. The only slog.* function calls in the entire tree are `slog.NewJSONHandler` / `slog.NewTextHandler` constructors in `cmd/server/main.go`.
- **Test coverage:** `TestHealth` + 3-case `TestHealth_RejectsWrongMethod` (POST/PUT/DELETE) subtests all pass under `go test ./internal/httpapi -race -count=1`.
- **Full Phase 1 gate green:** `go build`, `go vet`, `gofmt -l`, `go test ./... -race`, `make build`, `make clean` all exit 0. TEST-07 layout complete.

## Task Commits

Each task was committed atomically:

1. **Task 1 RED: add failing health handler tests** — `01ad9f2` (test)
2. **Task 1 GREEN: implement httpapi.Server scaffold with /health handler** — `e36b3e0` (feat)
3. **Task 2: wire compose-root main.go with config, logger, banner, httpapi** — `7c499d0` (feat)

_Task 1 used TDD so it produced two commits (RED + GREEN). Task 2 is a straight implementation of the compose-root per RESEARCH Pattern 1._

## Files Created/Modified

### Created

- `internal/httpapi/server.go` — Server struct (logger + mux), `New(logger *slog.Logger) *Server`, `Handler() http.Handler`, `registerRoutes()`. The logger field is load-bearing for Phase 4's middleware; the mux field is load-bearing for Phase 2-3 route additions.
- `internal/httpapi/health.go` — `handleHealth(w, r)` method writing `Content-Type: application/json` + 200 + literal `{"status":"ok"}` body. Deliberately does NOT call `s.logger.*` and does NOT touch `r.Header` (PITFALLS #13 credential-leak prevention).
- `internal/httpapi/health_test.go` — `TestHealth` (200 + Content-Type + body substrings, no Authorization header), `TestHealth_RejectsWrongMethod` (POST/PUT/DELETE subtests each asserting 405). Uses `io.Discard` logger so `go test` output stays clean.

### Modified

- `cmd/server/main.go` — Replaced the Plan 01-01 stub with the full compose-root: `-config` flag (defaults to `./config.yaml`), `config.Load`, inline `newLogger` helper (format + level switch), 10-key startup banner, `httpapi.New(logger)`, `http.Server{Addr, Handler}`, `httpSrv.ListenAndServe()`. The file is ~95 lines including the inline `newLogger`.

### Deleted

- `internal/httpapi/doc.go` — Plan 01-01 package-doc stub, no longer needed once `server.go` provides the package doc comment.
- `internal/httpapi/doc_test.go` — Plan 01-01 testify-anchor test, replaced by the real `health_test.go` which imports testify directly.

## Startup Banner Contract (frozen)

```go
logger.Info("openburo server starting",
    "version",          version.Version,             // "dev" by default; LDFLAGS injects release version
    "go_version",       runtime.Version(),           // "go1.26.2" in current environment
    "listen_addr",      cfg.Server.Addr(),           // ":8080" form from net.JoinHostPort
    "tls_enabled",      cfg.Server.TLS.Enabled,      // false in Phase 1
    "config_file",      *configPath,                 // the -config flag value
    "credentials_file", cfg.CredentialsFile,         // path only, never contents
    "registry_file",    cfg.RegistryFile,            // path only
    "ping_interval",    cfg.WebSocket.PingInterval.String(), // "30s"
    "log_format",       cfg.Logging.Format,          // "json" | "text"
    "log_level",        cfg.Logging.Level,           // "debug" | "info" | "warn" | "error"
)
```

**Order is locked.** Phases 2-5 must not reorder keys or add new ones without a CONTEXT.md update. Credentials-related fields (`credentials_file`) are path-only and safe to log; the contents of `credentials.yaml` must never appear in logs.

## Verification Sample

```
$ export PATH="$HOME/sdk/go1.26.2/bin:$PATH"

$ go test ./internal/httpapi -race -count=1 -v
=== RUN   TestHealth
--- PASS: TestHealth (0.00s)
=== RUN   TestHealth_RejectsWrongMethod
=== RUN   TestHealth_RejectsWrongMethod/POST
--- PASS: TestHealth_RejectsWrongMethod/POST (0.00s)
=== RUN   TestHealth_RejectsWrongMethod/PUT
--- PASS: TestHealth_RejectsWrongMethod/PUT (0.00s)
=== RUN   TestHealth_RejectsWrongMethod/DELETE
--- PASS: TestHealth_RejectsWrongMethod/DELETE (0.00s)
--- PASS: TestHealth_RejectsWrongMethod (0.00s)
PASS
ok  github.com/openburo/openburo-server/internal/httpapi  1.013s

$ go test ./... -race -count=1
?     github.com/openburo/openburo-server/cmd/server  [no test files]
ok    github.com/openburo/openburo-server/internal/config   1.017s
ok    github.com/openburo/openburo-server/internal/httpapi  1.014s

$ go build ./... && go vet ./... && gofmt -l .        # all exit 0 / empty

$ ! grep -rE 'slog\.(Info|Debug|Warn|Error|Default)\(' internal/ && echo OK
OK

$ make build && make clean
go build -ldflags "-X github.com/openburo/openburo-server/internal/version.Version=7c499d0-dirty" -o bin/openburo-server ./cmd/server
rm -rf bin/

$ # Smoke test on port 18080 (8080 busy with cozy-stack on dev machine):
$ go run ./cmd/server -config /tmp/config-smoke.yaml 2>/tmp/server.log &
$ curl -sS -i http://localhost:18080/health
HTTP/1.1 200 OK
Content-Type: application/json
Content-Length: 15

{"status":"ok"}

$ curl -sS -o /dev/null -w "%{http_code}\n" -X POST http://localhost:18080/health
405

$ head -1 /tmp/server.log
{"time":"2026-04-10T10:46:06.616187339+02:00","level":"INFO","msg":"openburo server starting","version":"dev","go_version":"go1.26.2","listen_addr":":18080","tls_enabled":false,"config_file":"/tmp/config-smoke.yaml","credentials_file":"/tmp/credentials-smoke.yaml","registry_file":"/tmp/registry-smoke.json","ping_interval":"30s","log_format":"json","log_level":"info"}
```

All 10 banner keys present in locked order. `curl localhost:18080/health` returned `200` with the correct JSON body; POST returned `405`. The banner appeared as a single JSON line on stderr.

## Decisions Made

- **newLogger stays inline in main.go.** RESEARCH Pattern 2 makes the argument explicit: creating an `internal/logging` package would tempt a future contributor to add a `logging.Default()` helper, which is the exact pattern the inject-everywhere rule forbids. Keeping the constructor in `main` makes the violation physically impossible.
- **Health handler is logger-blind.** `handleHealth` deliberately does not accept or use `s.logger`. This locks in the "never log health endpoints" convention that Phase 4's log middleware will also inherit — the handler itself cannot accidentally grow a `s.logger.Info("health hit", ...)` call during a future refactor because the handler isn't wired to think about logging at all.
- **Literal JSON body, not `json.NewEncoder`.** The /health payload is a constant. `w.Write([]byte(`{"status":"ok"}`))` is two lines and has no theoretical error path. Phase 4's dynamic error envelopes will use `json.NewEncoder` properly when the payload is actually computed.
- **No graceful shutdown in Phase 1.** `httpSrv.ListenAndServe()` blocks and returns an error on Ctrl-C. Phase 5 will replace this with `signal.NotifyContext` + two-phase shutdown per PITFALLS #6. Phase 1 stays a scaffold, not a production shim.
- **Anchor file cleanup is atomic with the first real code.** The Plan 01-01 `doc.go` and `doc_test.go` were deleted in the Task 1 RED commit — before the real server.go exists — because the RED test already replaces the testify-anchor role of `doc_test.go`. This keeps every intermediate commit buildable (the RED commit fails the health tests, but the rest of the tree compiles).

## Deviations from Plan

None functional. The plan executed exactly as written — every task followed RESEARCH Pattern 1/3/4 verbatim, every banner key landed in the locked order, every acceptance grep gate passes on the first run.

**One environmental note (not a deviation):** the smoke-test step ran on port **18080** instead of the plan's suggested **8080** because port 8080 on this dev machine is held by `cozy-stack` (unrelated dev service). Switched by editing a temp config, not the committed `config.example.yaml`. This had no effect on the server, its banner, or its behavior — it only moved the smoke test to a free port. The committed `config.example.yaml` still ships with `port: 8080` as specified by CONTEXT.md.

## Issues Encountered

- **Port 8080 already in use on the dev machine.** First smoke-test attempt hit a Cozy-stack 404. Diagnosed via `ss -ltnp` (cozy-stack holding :8080), then re-ran on :18080 with a temporary copy of the example config. No code changes required — the committed defaults are untouched and the scenario is strictly local to this machine.
- **One grep-gate miss during Task 1.** The initial draft of `server.go` referenced `slog.Default()` in a prose comment (warning against it), which made `! grep -E 'slog\.(Info|Debug|Warn|Error|Default)' internal/httpapi/server.go internal/httpapi/health.go` fail. Fix: rewrote the comment to "No internal/ package is permitted to grab a global logger" without naming the forbidden function. This is a lesson for Phase 2+: the grep gate does not distinguish warnings from real calls, so comments must paraphrase. The correction landed in the Task 1 GREEN commit (`e36b3e0`) — no separate fix commit was needed.

## User Setup Required

None — everything is pure code and local filesystem. To run the binary locally:

```bash
cp config.example.yaml config.yaml
cp credentials.example.yaml credentials.yaml
go run ./cmd/server -config config.yaml
```

On dev machines where port 8080 is occupied, edit `config.yaml` to pick a different port before running.

## Next Phase Readiness

**Phase 1 is complete.** All 8 requirements (FOUND-01 through FOUND-07 + TEST-07) are satisfied across the three plans:

- FOUND-01, FOUND-06, TEST-07 — Plan 01-01 (scaffold + CI + layout)
- FOUND-02, FOUND-07 — Plan 01-02 (config loader + examples)
- FOUND-03, FOUND-04, FOUND-05 — Plan 01-03 (slog + /health + banner) ← **this plan**

**Ready for Phase 2 (Registry Core):**

- `httpapi.Server` has the exact shape Phase 2 needs to extend — add a `store *registry.Store` field to the struct, pass it into `New`, and register new routes in `registerRoutes()` without touching `main.go`'s compose logic.
- `internal/registry/doc.go` anchor from Plan 01-01 is still in place, ready to be replaced with the real store implementation.
- The inject-first discipline means Phase 2's `registry.Store` must accept a `*slog.Logger` in its constructor (not call `slog.Default`). The grep gate will catch any regression.

**Ready for Phase 3 (WebSocket Hub) in parallel with Phase 2:**

- `internal/wshub/doc.go` anchor in place.
- Hub will get its own injected `*slog.Logger` from the compose root.

**Ready for Phase 4 (HTTP API + Middleware):**

- `srv.Handler()` returns `http.Handler` — Phase 4 wraps it with `recover → log → CORS → auth` without changing the `httpapi.Server` internals.
- Phase 4's log middleware must explicitly skip `/health` (not just `/health` exact match, but the full method-prefixed pattern) to preserve the "health stays silent" convention established here.

**Ready for Phase 5 (Graceful Shutdown):**

- `main.go`'s `run()` function is the single refactor site. Replace `httpSrv.ListenAndServe()` with `signal.NotifyContext` + goroutine-scoped `httpSrv.Shutdown(ctx)` + `hub.Close()` sequence per PITFALLS #6.

## Self-Check: PASSED

All claimed files exist on disk:
- `internal/httpapi/server.go` (FOUND)
- `internal/httpapi/health.go` (FOUND)
- `internal/httpapi/health_test.go` (FOUND)
- `cmd/server/main.go` (FOUND — modified from Plan 01-01 stub)
- `internal/httpapi/doc.go` (GONE — correctly deleted)
- `internal/httpapi/doc_test.go` (GONE — correctly deleted)

All claimed task commits present in `git log`:
- `01ad9f2` — test(01-03): add failing health handler tests (FOUND)
- `e36b3e0` — feat(01-03): implement httpapi.Server scaffold with /health handler (FOUND)
- `7c499d0` — feat(01-03): wire compose-root main.go with config, logger, banner, httpapi (FOUND)

Full Phase 1 gate verified: `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./... -race -count=1`, `make build`, and the `! grep -rE 'slog\.(Info|Debug|Warn|Error|Default)\(' internal/` gate all pass. Live smoke test confirmed GET /health returns 200 JSON and POST /health returns 405 with the 10-key banner emitted as the first stderr line.

---
*Phase: 01-foundation*
*Plan: 01-03-slog-health-banner*
*Completed: 2026-04-10*
