# Phase 1: Foundation - Context

**Gathered:** 2026-04-10
**Status:** Ready for planning

<domain>
## Phase Boundary

A buildable, CI-green Go 1.26 project skeleton that starts, logs a structured startup banner, serves `GET /health` without auth, and is backed by example config files a developer can copy and run. Delivers the four-package `internal/` layout, config + credentials YAML loading, `log/slog` construction, and CI running `go test -race`, `go vet`, and `gofmt -l`. No domain code (registry, hub, API routes beyond `/health`) — that's Phases 2-4.

</domain>

<decisions>
## Implementation Decisions

All gray areas were resolved by Claude's discretion at the user's direction ("décide tout seul pour ces questions"). Each decision below is locked for downstream planning/implementation.

### Module & Repository

- Module path: `github.com/openburo/openburo-server` — placeholder; adjust once the repo has a real home. Planner should make this a single point of change (not hardcoded in imports beyond what the module path requires).
- `go.mod` declares `go 1.26`.
- `go.sum` committed; no vendor directory.
- No `replace` directives.

### CI & Build Tooling

- **CI platform:** GitHub Actions — `.github/workflows/ci.yml` runs on push and pull_request.
- **CI jobs (single workflow, one job with steps):**
  1. `actions/checkout@v4`
  2. `actions/setup-go@v5` with `go-version: '1.26'`
  3. `go mod download`
  4. `gofmt -l .` — fails if any file needs formatting
  5. `go vet ./...`
  6. `go run honnef.co/go/tools/cmd/staticcheck@latest ./...`
  7. `go build ./...`
  8. `go test ./... -race -count=1`
- **Makefile:** yes, with standard targets. Acts as the canonical interface readers of the reference impl see first.
  - `make build` — `go build -ldflags "$(LDFLAGS)" -o bin/openburo-server ./cmd/server`
  - `make run` — `go run ./cmd/server -config config.yaml`
  - `make test` — `go test ./... -race -count=1`
  - `make lint` — `gofmt -l . && go vet ./... && staticcheck ./...`
  - `make fmt` — `gofmt -w .`
  - `make ci` — runs lint + test + build (mirrors GitHub Actions locally)
  - `make clean` — removes `bin/`
  - `LDFLAGS` variable sources version from `git describe --tags --always --dirty` (see Versioning below)

### Linting Rigor

- **Phase 1 linters:** `gofmt`, `go vet`, and `staticcheck` (honnef.co/go/tools/cmd/staticcheck).
- **NOT using `golangci-lint`** — avoids `.golangci.yml` config sprawl; `staticcheck` alone catches the real bugs for a reference impl.
- **No custom lint rules** — idiomatic Go defaults only.
- `staticcheck` invoked via `go run honnef.co/go/tools/cmd/staticcheck@latest` in CI and via an installed binary locally via Makefile. No need to pin in `go.mod` as a tool dependency for v1.

### Config File Layout

Two YAML files loaded at startup:

- **`config.yaml`** — server-operational settings. Shape matches the original spec:
  ```yaml
  server:
    port: 8080
    tls:
      enabled: false
      cert_file: ""
      key_file: ""

  credentials_file: "./credentials.yaml"
  registry_file: "./registry.json"

  websocket:
    ping_interval_seconds: 30

  # Phase 1 additions:
  logging:
    format: json    # json | text
    level: info     # debug | info | warn | error

  cors:
    allowed_origins: []   # empty in example; populated by operator. Shared with WS OriginPatterns in Phase 4.
  ```
- **`credentials.yaml`** — admin credentials (bcrypt hashes). Loaded by Phase 4; Phase 1 only needs the loader scaffold and example file.
  ```yaml
  admins:
    - username: "admin"
      password_hash: "$2a$12$EXAMPLEEXAMPLEEXAMPLEEXAMPLEEXAMPLEEXAMPLEEXAMPLEEXAMPLEE"
  ```
- **Both files have `.example` siblings committed at repo root:** `config.example.yaml` and `credentials.example.yaml`. The real files are `.gitignore`'d in Phase 1 so developer-local tweaks never get committed.

### Config Discovery

- **Flag:** `-config <path>` — optional, defaults to `./config.yaml` resolved relative to the current working directory.
- **No env-var fallback, no XDG paths, no auto-search.** Reference impl clarity beats convenience — one well-known location, one flag override.
- **Missing file behavior:** fail fast with a clear error ("config file not found: <path>; copy config.example.yaml to config.yaml to get started").
- **Credentials path:** read from `config.yaml`'s `credentials_file` key (not a separate flag). Loaded by Phase 4; Phase 1 only validates that the path is set.

### Logging Toggle (slog)

- **Format:** config key `logging.format` (`json` | `text`). Default: `json`.
  - `json` → `slog.NewJSONHandler(os.Stderr, opts)` (production default)
  - `text` → `slog.NewTextHandler(os.Stderr, opts)` (dev-friendly)
- **Level:** config key `logging.level` (`debug` | `info` | `warn` | `error`). Default: `info`.
- **Output:** stderr only (not stdout). Allows operators to redirect logs independently of any future stdout use.
- **No env-var override** — config.yaml is the single source of truth.
- **Logger is constructed in `main.go`, wrapped in a `*slog.Logger`, and injected into every component** (the config loader, the future httpapi.Server, the future wshub.Hub, the future registry.Store). No `slog.Default()` usage inside `internal/` packages — always accept an injected `*slog.Logger`.

### Versioning

- **Source:** `var Version = "dev"` in `internal/version/version.go` (default, used when running `go run`).
- **Build-time override:** Makefile `LDFLAGS` injects the version via:
  ```makefile
  VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
  LDFLAGS := -X github.com/openburo/openburo-server/internal/version.Version=$(VERSION)
  ```
- **Accessor:** `version.Version` (exported package variable, read at startup banner time). No helper function needed.
- **Go version:** captured via `runtime.Version()` at startup.

### Startup Banner

Exactly one `slog.Info` call at server start, after config is loaded and the logger is constructed. Keys (in this order):

```
slog.Info("openburo server starting",
  "version",        version.Version,
  "go_version",     runtime.Version(),
  "listen_addr",    cfg.Server.Addr(),         // e.g. ":8080" or the full addr if binding explicitly
  "tls_enabled",    cfg.Server.TLS.Enabled,
  "config_file",    configPath,
  "credentials_file", cfg.CredentialsFile,
  "registry_file",  cfg.RegistryFile,
  "ping_interval",  cfg.WebSocket.PingInterval.String(),  // "30s"
  "log_format",     cfg.Logging.Format,
  "log_level",      cfg.Logging.Level,
)
```

- Single structured line, no multi-line ASCII banner — this is a reference impl, not a marketing moment.
- Credentials-related fields (paths only, never contents) are safe to log.

### Package Layout (TEST-07)

```
open-buro-server/
├── cmd/
│   └── server/
│       └── main.go                 # compose-root, Phase 1 ships a minimal version
├── internal/
│   ├── config/                     # Phase 1: full implementation (Config, Credentials types, loaders)
│   │   ├── config.go
│   │   ├── config_test.go
│   │   └── testdata/
│   │       ├── valid.yaml
│   │       └── invalid.yaml
│   ├── version/                    # Phase 1: just var Version = "dev"
│   │   └── version.go
│   ├── registry/                   # Phase 1: empty package with doc.go stub (real impl in Phase 2)
│   │   └── doc.go
│   ├── wshub/                      # Phase 1: empty package with doc.go stub (real impl in Phase 3)
│   │   └── doc.go
│   └── httpapi/                    # Phase 1: minimal — /health handler + Server scaffold
│       ├── server.go               # Server struct scaffold (even if only /health wires up in Phase 1)
│       ├── health.go
│       └── health_test.go
├── config.example.yaml
├── credentials.example.yaml
├── go.mod
├── go.sum
├── Makefile
├── .gitignore
└── .github/
    └── workflows/
        └── ci.yml
```

- **Phase 1 creates empty `doc.go` stubs for `internal/registry` and `internal/wshub`** so the layout requirement (TEST-07) is verifiable on disk from Phase 1 onward — subsequent phases fill in the real code.
- **Phase 1 ships `internal/httpapi/server.go`** as a real (tiny) Server struct with a constructor, a `Handler()` method returning the mux, and `/health` registered. Phase 4 will extend it; Phase 1 establishes the shape.

### Testing Layout (TEST-07)

- Tests live in the **same package** as the code (white-box by default). Exception: integration tests in `internal/httpapi` may use `httpapi_test` package for end-to-end HTTP tests.
- Table-driven tests with `t.Run` subtests.
- Fixtures in `testdata/` subdirectories (standard Go convention — the `go` tool ignores them).
- `testify/require` is pulled in as a dep in Phase 1 (even though the first tests are simple) so future phases don't need to add it as a fresh dep.

### .gitignore

Phase 1 `.gitignore` content:
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

### Claude's Discretion

- Exact field type for durations in config (`time.Duration` via string parse vs. integer seconds). Default: integer seconds in YAML (`ping_interval_seconds: 30`), converted to `time.Duration` during `config.Load`. Keeps YAML human-friendly.
- Internal naming of Config substructs (`ServerConfig`, `TLSConfig`, `LoggingConfig`, `CORSConfig`, `WebSocketConfig`) — planner picks exact names.
- How many test cases the `config_test.go` table covers — planner decides, but must include: valid full file, missing required fields, invalid enum values (log format/level), missing file, unreadable file.
- Whether the Makefile has a `help` target (cosmetic).

</decisions>

<specifics>
## Specific Ideas

- Makefile targets mirror the "canonical developer commands" for a Go project — a reader opening the repo should see `make build / make test / make run` and immediately know what to do.
- The CI workflow is intentionally one job with sequential steps, not a matrix — this is a single-platform Linux reference impl; Windows/macOS concerns are flagged out of scope in PROJECT.md.
- The startup banner is one line, not ASCII art — the goal is to document what's loaded, not to look cool.
- `staticcheck` is invoked via `go run honnef.co/go/tools/cmd/staticcheck@latest` so CI doesn't need a separate install step. Adds ~15s to first CI run but avoids version drift.

</specifics>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project scope & contracts
- `.planning/PROJECT.md` — Reference-impl framing, hard constraints (Go, slog, bcrypt, Basic Auth, file persistence), out-of-scope list
- `.planning/REQUIREMENTS.md` §Foundation (FOUND-01..07) and §Testing (TEST-07) — the 8 requirements this phase must satisfy
- `.planning/ROADMAP.md` §"Phase 1: Foundation" — goal statement + 4 success criteria
- `../open-buro-dossier-technique-file-picker.md` — broader OpenBuro ecosystem context (Android/Cozy/XDG prior art). Not load-bearing for Phase 1 but good background for naming and comments.

### Research
- `.planning/research/STACK.md` — Exact library choices and version pins (Go 1.26, 5 direct deps, flat layout); Phase 1 wires up go.mod from this
- `.planning/research/ARCHITECTURE.md` — Four-package layout, compose-root pattern, dependency direction; Phase 1 creates the skeleton
- `.planning/research/PITFALLS.md` §12 (defer Unlock), §13 (credentials in logs) — the log-safety pitfalls that Phase 1's slog wiring must already respect
- `.planning/research/SUMMARY.md` — Phase 1 section lists the deliverables; this CONTEXT extends those deliverables with tooling details

### Go idioms for scaffolding
- No external spec — conventions are captured in STACK.md and ARCHITECTURE.md

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- None — greenfield repo. The only existing files are `.planning/**` and `.git/`.

### Established Patterns
- None — this is the phase that establishes them.

### Integration Points
- None yet — Phase 1 creates the skeleton that all subsequent phases plug into.

</code_context>

<deferred>
## Deferred Ideas

- **License file (LICENSE / COPYING):** not in Phase 1; add during Phase 5 (Wiring, Shutdown & Polish) alongside the README. Default choice if not otherwise specified: MIT.
- **Full README:** Phase 5 writes the user-facing README. Phase 1 may ship a minimal README stub (one paragraph + `make run` quickstart) or skip it entirely — planner decides.
- **`tool` directive in `go.mod` for staticcheck (Go 1.24+ feature):** could pin staticcheck as a tool dep for version stability. Deferred — `go run honnef.co/go/tools/cmd/staticcheck@latest` is fine for a reference impl.
- **Dockerfile / container image:** out of scope per PROJECT.md (reference impl, not deployed service).
- **Release workflow (goreleaser, tagged builds):** out of scope; this project isn't distributed as binaries.
- **CONTRIBUTING.md, CODE_OF_CONDUCT.md, issue templates:** out of scope for v1.

</deferred>

---

*Phase: 01-foundation*
*Context gathered: 2026-04-10*
