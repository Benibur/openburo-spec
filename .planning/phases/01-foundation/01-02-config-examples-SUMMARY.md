---
phase: 01-foundation
plan: 02
subsystem: infra
tags: [go, yaml, config, validation, testify, time.Duration]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: "Go module + go.yaml.in/yaml/v3 + testify pinned; internal/config/ directory with disposable doc.go anchor"
provides:
  - "internal/config package with Config type tree (Server, TLS, WebSocket, Logging, CORS) + Load() + validate() + ServerConfig.Addr()"
  - "11 unit test cases under -race covering valid config, 6 validation failures, missing file, unreadable file, and ping interval derivation"
  - "config.example.yaml and credentials.example.yaml at repo root ready for cp-and-run quickstart"
  - "time.Duration derivation pattern for ping_interval_seconds -> PingInterval"
  - "Friendly missing-file error message pointing operators to config.example.yaml"
affects:
  - 01-03-slog-health-banner (main.go will call config.Load(*configPath) and pass *slog.Logger + *Config to httpapi.Server)
  - Phase 4 (will add LoadCredentials + parse credentials.yaml using the same yaml/v3 dep)

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "YAML-integer -> time.Duration conversion done post-validation in Load, keeping YAML human-readable while exposing a Duration to callers"
    - "Table-driven unit tests with testdata/ fixtures (Go convention: go tool skips testdata/)"
    - "Error wrapping with %w for Load() so callers can errors.Is(err, os.ErrNotExist)"
    - "Friendly operator-facing missing-file message: 'copy config.example.yaml to config.yaml to get started'"
    - "white-box same-package tests (package config, not package config_test) per TEST-07"
    - "Explicit enum errors over silent defaults (logging.format/logging.level must be declared)"

key-files:
  created:
    - internal/config/config.go
    - internal/config/config_test.go
    - internal/config/testdata/valid.yaml
    - internal/config/testdata/invalid-log-format.yaml
    - internal/config/testdata/invalid-log-level.yaml
    - internal/config/testdata/missing-credentials-file.yaml
    - internal/config/testdata/zero-port.yaml
    - internal/config/testdata/tls-no-cert.yaml
    - internal/config/testdata/zero-ping.yaml
    - internal/config/testdata/malformed.yaml
    - config.example.yaml
    - credentials.example.yaml
  modified: []
  deleted:
    - internal/config/doc.go

key-decisions:
  - "config.go imports go.yaml.in/yaml/v3 directly, so the Plan 01-01 doc.go blank-import anchor was deleted; yaml/v3 is now retained by real usage."
  - "Followed RESEARCH skeleton verbatim — no shape changes to Config/ServerConfig/TLSConfig/WebSocketConfig/LoggingConfig/CORSConfig."
  - "PingInterval derivation placed AFTER validate() so zero-or-negative PingIntervalSeconds is caught first and Duration is only set on known-good input."
  - "Fixtures use hyphens (invalid-log-format.yaml, zero-ping.yaml) per plan's actual naming — VALIDATION.md footnote declares -run patterns illustrative."

patterns-established:
  - "Config loader pattern: os.ReadFile -> errors.Is(ErrNotExist) special-case -> yaml.Unmarshal -> validate() -> derive runtime fields. Every future loader in the codebase should mirror this sequence."
  - "Validation errors name the offending YAML key verbatim (server.port, logging.format, websocket.ping_interval_seconds) so operators can fix their YAML without reading Go source."
  - "Example YAML files at repo root are the authoritative schema documentation; the Config struct's YAML tags are the contract with operators."

requirements-completed:
  - FOUND-02
  - FOUND-07

# Metrics
duration: 12min
completed: 2026-04-10
---

# Phase 01 Plan 02: Config Package + Example YAML Files Summary

**Typed YAML config loader (internal/config) with 7-rule validator, 11-test suite, and quickstart-ready config.example.yaml + credentials.example.yaml at repo root.**

## Performance

- **Duration:** ~12 min
- **Started:** 2026-04-10T08:34:30Z
- **Completed:** 2026-04-10T08:38:32Z
- **Tasks:** 2
- **Files created:** 12
- **Files modified:** 0
- **Files deleted:** 1 (`internal/config/doc.go` anchor stub replaced by real `config.go`)

## Accomplishments

- Wrote `internal/config/config.go` (140 lines) with the full Config type tree — `Config`, `ServerConfig`, `TLSConfig`, `WebSocketConfig`, `LoggingConfig`, `CORSConfig` — plus `Load(path string) (*Config, error)`, unexported `validate()`, and `ServerConfig.Addr()`.
- Implemented 7 validation rules (see §Validation Rules below), every one emitting an operator-friendly error naming the offending YAML key.
- Wrote table-driven `TestLoad` with 8 subtests + 3 standalone tests (`TestLoad_MissingFile`, `TestLoad_UnreadableFile`, `TestLoad_DerivesPingInterval`) — all 11 pass under `-race` in ~1s.
- Wrote 8 YAML fixtures in `internal/config/testdata/` covering the happy path, 6 validation-fail cases, and one malformed-YAML case.
- Wrote `config.example.yaml` and `credentials.example.yaml` at repo root with every Phase 1 schema key, fully commented for developer-facing documentation.
- Verified `config.example.yaml` parses through `Load()` via a throwaway round-trip test (removed before commit).
- Error wrapping uses `%w` throughout — `errors.Is(err, os.ErrNotExist)` survives the wrap chain.
- Zero `slog.*` imports inside `internal/config` (FOUND-03 injection rule) and zero `Credentials` struct (deferred to Phase 4).

## Task Commits

Each task was committed atomically:

1. **Task 1: Implement internal/config package (types + Load + validate + Addr) + fixtures + tests** - `8749e48` (feat)
2. **Task 2: Write config.example.yaml and credentials.example.yaml at repo root** - `d8c64ff` (feat)

## Config Struct Shape

```go
type Config struct {
    Server          ServerConfig    `yaml:"server"`
    CredentialsFile string          `yaml:"credentials_file"`
    RegistryFile    string          `yaml:"registry_file"`
    WebSocket       WebSocketConfig `yaml:"websocket"`
    Logging         LoggingConfig   `yaml:"logging"`
    CORS            CORSConfig      `yaml:"cors"`
}

type ServerConfig    struct { Port int; TLS TLSConfig }        // +Addr() string
type TLSConfig       struct { Enabled bool; CertFile, KeyFile string }
type WebSocketConfig struct { PingIntervalSeconds int; PingInterval time.Duration `yaml:"-"` }
type LoggingConfig   struct { Format, Level string }
type CORSConfig      struct { AllowedOrigins []string }
```

## Validation Rules Enforced

The unexported `validate()` method enforces 7 rules. Each error names the YAML key verbatim so operators can fix their config without reading Go source:

1. `server.port` must be in `[1, 65535]` (rejects 0, negatives, overflow).
2. If `server.tls.enabled: true`, `server.tls.cert_file` must be non-empty.
3. If `server.tls.enabled: true`, `server.tls.key_file` must be non-empty.
4. `credentials_file` must be non-empty.
5. `registry_file` must be non-empty.
6. `websocket.ping_interval_seconds` must be `> 0`.
7. `logging.format` must be `"json"` or `"text"` (no silent default).
8. `logging.level` must be `"debug"`, `"info"`, `"warn"`, or `"error"` (no silent default).

(8 checks total; the 7-rule count in the plan was approximate — the TLS check splits into cert/key for precise error messages.)

## Test Coverage

- **TestLoad** — 8 subtests: `valid full config`, `invalid log format`, `invalid log level`, `missing credentials_file`, `zero port`, `tls enabled without cert`, `zero ping interval`, `malformed yaml`.
- **TestLoad_MissingFile** — asserts both `"config file not found"` and `"copy config.example.yaml"` substrings are present in the error.
- **TestLoad_UnreadableFile** — creates a mode-0 file in `t.TempDir()` and asserts Load returns an error (cleanup via `t.Cleanup` + `os.Chmod 0o600`).
- **TestLoad_DerivesPingInterval** — asserts `cfg.WebSocket.PingIntervalSeconds == 30` AND `cfg.WebSocket.PingInterval.String() == "30s"`.

**Total:** 11 passing assertions in ~1.0s wall time under `-race -count=1`.

## Files Created/Modified

### Created

- `internal/config/config.go` — Config type tree, `Load()`, `validate()`, `ServerConfig.Addr()`. Imports `errors`, `fmt`, `net`, `os`, `strconv`, `time`, and `go.yaml.in/yaml/v3`. No slog, no Credentials.
- `internal/config/config_test.go` — White-box same-package tests (`package config`). Uses `testify/require` and `t.TempDir`.
- `internal/config/testdata/valid.yaml` — Happy-path fixture with every Phase 1 key populated.
- `internal/config/testdata/invalid-log-format.yaml` — `logging.format: xml` (triggers format validator).
- `internal/config/testdata/invalid-log-level.yaml` — `logging.level: verbose` (triggers level validator).
- `internal/config/testdata/missing-credentials-file.yaml` — omits the `credentials_file:` key entirely.
- `internal/config/testdata/zero-port.yaml` — `server.port: 0`.
- `internal/config/testdata/tls-no-cert.yaml` — `tls.enabled: true` with empty `cert_file`/`key_file`.
- `internal/config/testdata/zero-ping.yaml` — `websocket.ping_interval_seconds: 0`.
- `internal/config/testdata/malformed.yaml` — intentionally unterminated YAML string on the `credentials_file` line.
- `config.example.yaml` — Repo-root quickstart template, every schema key + operator-facing comments.
- `credentials.example.yaml` — Repo-root template with `admins:` + username/password_hash + bcrypt generation instructions + functionally-invalid placeholder hash (fails loudly in Phase 4 if not replaced).

### Deleted

- `internal/config/doc.go` — The Plan 01-01 blank-import anchor for `go.yaml.in/yaml/v3` is no longer needed because `config.go` imports the package directly. Removal was staged via `git rm` in the Task 1 commit.

## Decisions Made

- **Anchor file disposal:** Deleted `internal/config/doc.go` rather than keeping it as a pure package-doc file. Rationale: having both `doc.go` (with a misleading "Phase 1 only ships this stub" comment) and `config.go` (the real implementation) would confuse future readers. The package doc-comment now lives at the top of `config.go`.
- **Test fixture naming:** Used hyphenated filenames (`invalid-log-format.yaml`, `zero-ping.yaml`) exactly as the PLAN's action steps specify. VALIDATION.md's `-run TestLoad_InvalidFormat`-style patterns are declared illustrative in a footnote at line 41, so table-driven subtests are compliant.
- **Round-trip smoke test for `config.example.yaml`:** Created a throwaway `internal/config/example_smoke_test.go` that calls `Load("../../config.example.yaml")`, ran it green, then deleted it before the Task 2 commit. This is the "truest proof" of schema match mentioned in the plan's Task 2 Step 3 alternative. Not kept as a permanent test because it couples the package to the repo layout; Plan 01-03's `make run` smoke pass will exercise the same path.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Missing internal/config/.gitkeep (plan referenced a file Plan 01-01 never created)**
- **Found during:** Task 1 Step 2 ("Delete `internal/config/.gitkeep` once `config.go` exists")
- **Issue:** The plan's files_modified list includes `internal/config/.gitkeep` and Task 1 Step 2 says to delete it, but Plan 01-01's deviation notes reveal that `.gitkeep` was NEVER created — it was replaced with a blank-import `doc.go` anchor. So the plan's delete step targets a nonexistent file.
- **Fix:** Deleted `internal/config/doc.go` instead (the actual anchor file in the repo). The semantic intent is the same — remove the disposable Phase 1 anchor now that real code lives in `config.go`. Staged via `git rm internal/config/doc.go` in the Task 1 commit.
- **Files modified:** `internal/config/doc.go` (deleted).
- **Verification:** `git status` shows `D internal/config/doc.go` staged; `go build ./...`, `go vet ./...`, `go test ./internal/config -race -count=1` all green after deletion.
- **Committed in:** `8749e48` (Task 1 commit).

---

**Total deviations:** 1 auto-fixed (1 blocking).
**Impact on plan:** Zero scope change. The PLAN and the Plan 01-01 SUMMARY disagreed on the anchor file's name (`.gitkeep` vs `doc.go`); followed Plan 01-01's actual filesystem state, which is the ground truth. All acceptance criteria met.

## Issues Encountered

- **Cannot import `internal/config` from outside the module tree.** When attempting a round-trip smoke test for `config.example.yaml` via `go run /tmp/load_example.go`, Go refused: `use of internal package not allowed`. Resolved by creating a throwaway test file inside `internal/config/` (`example_smoke_test.go`), running it, and deleting it before commit. This is a one-time verification, not a permanent test.

No other issues. All 11 primary tests passed on the first run, `gofmt -l .` produced no output, `go vet ./...` exited 0, `go build ./...` exited 0.

## User Setup Required

None — this plan adds a pure-code YAML loader and two example files. No external service configuration, no environment variables, no credentials.

Local developers can already run:
```bash
cp config.example.yaml config.yaml
cp credentials.example.yaml credentials.yaml
```

but the binary won't actually serve traffic until Plan 01-03 wires `main.go`.

## Next Phase Readiness

- **Plan 01-03 unblocked:** `main.go` can now call `cfg, err := config.Load(*configPath)` and pass `cfg` to `httpapi.NewServer(cfg, logger)`. The startup banner can read `cfg.Server.Addr()`, `cfg.Server.TLS.Enabled`, `cfg.CredentialsFile`, `cfg.RegistryFile`, `cfg.WebSocket.PingInterval.String()`, `cfg.Logging.Format`, and `cfg.Logging.Level` directly.
- **Phase 4 unblocked for credentials loader:** When Phase 4 writes `LoadCredentials(cfg.CredentialsFile)`, the path validation is already guaranteed by `config.Load`'s `credentials_file is required` check.
- **Phase 2 / Phase 3 unaffected:** Registry and wshub packages don't touch config directly; they receive injected settings from the compose root in `main.go`.

## Verification Sample

```
$ export PATH="$HOME/sdk/go1.26.2/bin:$PATH"
$ go test ./internal/config -race -count=1
ok  	github.com/openburo/openburo-server/internal/config	1.018s
$ go test ./... -race -count=1
?   	github.com/openburo/openburo-server/cmd/server	[no test files]
ok  	github.com/openburo/openburo-server/internal/config	1.019s
ok  	github.com/openburo/openburo-server/internal/httpapi	1.014s
?   	github.com/openburo/openburo-server/internal/registry	[no test files]
?   	github.com/openburo/openburo-server/internal/version	[no test files]
?   	github.com/openburo/openburo-server/internal/wshub	[no test files]
$ go vet ./...          # exit 0
$ go build ./...        # exit 0
$ gofmt -l .            # (empty)
$ grep -c 'yaml:' internal/config/config.go
14
$ grep -q 'copy config.example.yaml' internal/config/config.go && echo "friendly msg OK"
friendly msg OK
$ grep -q 'yaml:"-"' internal/config/config.go && echo "PingInterval excluded from YAML"
PingInterval excluded from YAML
```

## Self-Check: PASSED

All 13 claimed files exist on disk:
- `internal/config/config.go`, `internal/config/config_test.go`
- 8 fixture files under `internal/config/testdata/`
- `config.example.yaml`, `credentials.example.yaml`
- This `SUMMARY.md`

Both task commit hashes (`8749e48`, `d8c64ff`) present in `git log --all`.

`internal/config/doc.go` (the disposable Plan 01-01 anchor) no longer exists, confirming the Task 1 deletion stuck.

---
*Phase: 01-foundation*
*Plan: 01-02-config-examples*
*Completed: 2026-04-10*
