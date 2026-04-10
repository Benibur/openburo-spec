---
phase: 1
slug: foundation
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-10
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go stdlib `testing` + `github.com/stretchr/testify/require` v1.11.x |
| **Config file** | None (stdlib `go test` uses `go.mod` for discovery) |
| **Quick run command** | `go test ./internal/... -race -count=1 -short` |
| **Full suite command** | `go test ./... -race -count=1` |
| **Estimated runtime** | ~5 seconds (small phase, few tests) |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/... -race -count=1 -short`
- **After every plan wave:** Run `go test ./... -race -count=1`
- **Before `/gsd:verify-work`:** Full suite must be green + `go build ./...` exits 0 + `go vet ./...` exits 0 + `gofmt -l .` produces no output
- **Max feedback latency:** ~10 seconds

---

## Per-Task Verification Map

Task IDs follow the convention `{phase}-{plan}-{task}`. Plan and task numbers will be finalized by the planner; the requirement→verification mapping below is authoritative regardless of how tasks are sliced.

| Requirement | Behavior | Test Type | Automated Command | Wave 0 File | Status |
|-------------|----------|-----------|-------------------|-------------|--------|
| FOUND-01 | `go build ./...` succeeds on Go 1.26 with pinned deps | Build | `go build ./...` (exit 0) | ✅ scaffold + go.mod | ⬜ pending |
| FOUND-01 | `go.mod` declares `go 1.26` | Static (grep) | `grep -q '^go 1.26' go.mod` | ✅ go.mod | ⬜ pending |
| FOUND-01 | 5-dep ceiling not breached in Phase 1 (only go.yaml.in/yaml/v3 + testify expected) | Static (grep) | `grep -c '^require' go.mod \|\| grep -c '    [a-z]' go.mod` (visual review acceptable) | ✅ go.mod | ⬜ pending |
| FOUND-02 | `config.Load` parses valid YAML into `Config` struct | Unit (table) | `go test ./internal/config -run TestLoad -race` | ✅ `internal/config/config_test.go` + `testdata/valid.yaml` | ⬜ pending |
| FOUND-02 | `config.Load` rejects invalid enum for `logging.format` | Unit | `go test ./internal/config -run TestLoad_InvalidFormat` | ✅ `testdata/invalid_format.yaml` | ⬜ pending |
| FOUND-02 | `config.Load` rejects invalid enum for `logging.level` | Unit | `go test ./internal/config -run TestLoad_InvalidLevel` | ✅ `testdata/invalid_level.yaml` | ⬜ pending |
| FOUND-02 | `config.Load` rejects missing required fields | Unit | `go test ./internal/config -run TestLoad_MissingRequired` | ✅ `testdata/missing_required.yaml` | ⬜ pending |
| FOUND-02 | `config.Load` returns friendly error on missing file | Unit | `go test ./internal/config -run TestLoad_MissingFile` | ✅ `internal/config/config_test.go` | ⬜ pending |
| FOUND-02 | `config.Load` converts `ping_interval_seconds` to `time.Duration` | Unit | `go test ./internal/config -run TestLoad_PingInterval` | ✅ `internal/config/config_test.go` | ⬜ pending |
| FOUND-03 | `*slog.Logger` constructed from `logging.format`/`logging.level` without panic | Static / smoke | `go vet ./... && go run ./cmd/server -config config.example.yaml` (first stderr line is the banner) | ✅ `cmd/server/main.go` | ⬜ pending |
| FOUND-03 | No `slog.Default()` or bare `slog.Info/Debug/Warn/Error` in `internal/` | Static (grep gate) | `! grep -rE 'slog\.(Info\|Debug\|Warn\|Error\|Default)' internal/` (Phase 1 only — Phase 4 will use injected loggers) | ✅ Grep gate, no test file | ⬜ pending |
| FOUND-03 | `*slog.Logger` is a field on every component that logs (injection, not `slog.Default()`) | Static review | Manual code review + the grep gate above | — | ⬜ pending |
| FOUND-04 | `GET /health` returns 200 with no Authorization header | Unit (httptest) | `go test ./internal/httpapi -run TestHealth -race` | ✅ `internal/httpapi/health_test.go` | ⬜ pending |
| FOUND-04 | `POST /health` returns 405 Method Not Allowed (method patterns wired correctly) | Unit (httptest) | `go test ./internal/httpapi -run TestHealth_RejectsWrongMethod` | ✅ `internal/httpapi/health_test.go` | ⬜ pending |
| FOUND-04 | `/health` response body is JSON with `Content-Type: application/json` | Unit (httptest) | Part of `TestHealth` (checks `Content-Type` header and body shape) | ✅ same file | ⬜ pending |
| FOUND-05 | Startup banner emits one `slog.Info` line with all 10 required keys (version, go_version, listen_addr, tls_enabled, config_file, credentials_file, registry_file, ping_interval, log_format, log_level) | Manual / smoke | Run `make run` (or `go run ./cmd/server -config config.yaml`), grep stderr for `"openburo server starting"`, visually confirm all 10 key names | ✅ `cmd/server/main.go` | ⬜ pending (manual) |
| FOUND-06 | `.github/workflows/ci.yml` exists and contains all required steps | Static (file + grep) | `test -f .github/workflows/ci.yml && grep -q 'go test ./\.\.\. -race' .github/workflows/ci.yml && grep -q 'go vet ./\.\.\.' .github/workflows/ci.yml && grep -q 'gofmt -l \.' .github/workflows/ci.yml && grep -q 'staticcheck' .github/workflows/ci.yml && grep -q 'go-version' .github/workflows/ci.yml` | ✅ `.github/workflows/ci.yml` | ⬜ pending |
| FOUND-06 | CI workflow targets Go 1.26 | Static (grep) | `grep -q \"go-version: '1.26'\" .github/workflows/ci.yml` | ✅ same file | ⬜ pending |
| FOUND-06 | Makefile provides `build`, `run`, `test`, `lint`, `fmt`, `ci`, `clean` targets | Static (grep) | `for t in build run test lint fmt ci clean; do grep -q "^$t:" Makefile \|\| exit 1; done` | ✅ `Makefile` | ⬜ pending |
| FOUND-06 | `make ci` runs lint + test + build locally (mirrors GitHub Actions) | Smoke | `make ci` exit 0 | ✅ `Makefile` | ⬜ pending |
| FOUND-06 | `make test` runs under `-race` | Static (grep) | `grep -q -- '-race' Makefile` | ✅ `Makefile` | ⬜ pending |
| FOUND-07 | `config.example.yaml` exists at repo root | Static (file check) | `test -f config.example.yaml` | ✅ `config.example.yaml` | ⬜ pending |
| FOUND-07 | `credentials.example.yaml` exists at repo root | Static (file check) | `test -f credentials.example.yaml` | ✅ `credentials.example.yaml` | ⬜ pending |
| FOUND-07 | `config.example.yaml` contains every required top-level key | Static (grep) | `grep -q '^server:' config.example.yaml && grep -q '^credentials_file:' config.example.yaml && grep -q '^registry_file:' config.example.yaml && grep -q '^websocket:' config.example.yaml && grep -q '^logging:' config.example.yaml && grep -q '^cors:' config.example.yaml` | ✅ `config.example.yaml` | ⬜ pending |
| FOUND-07 | `credentials.example.yaml` contains `admins:` with a bcrypt placeholder | Static (grep) | `grep -q '^admins:' credentials.example.yaml && grep -q 'password_hash' credentials.example.yaml` | ✅ `credentials.example.yaml` | ⬜ pending |
| FOUND-07 | Copy-run flow works: `cp config.example.yaml config.yaml && cp credentials.example.yaml credentials.yaml && go run ./cmd/server` starts and `curl /health` returns 200 | Smoke (manual) | Manual quickstart | — | ⬜ pending (manual) |
| TEST-07 | Directory layout matches `cmd/server/` + `internal/{config,registry,wshub,httpapi,version}/` | Static (file check) | `test -d cmd/server && test -d internal/config && test -d internal/registry && test -d internal/wshub && test -d internal/httpapi && test -d internal/version` | ✅ scaffold | ⬜ pending |
| TEST-07 | `internal/registry/doc.go` and `internal/wshub/doc.go` stubs exist | Static (file check) | `test -f internal/registry/doc.go && test -f internal/wshub/doc.go` | ✅ scaffold | ⬜ pending |
| TEST-07 | `cmd/server/main.go` exists and `package main` | Static (grep) | `test -f cmd/server/main.go && grep -q '^package main' cmd/server/main.go` | ✅ scaffold | ⬜ pending |
| TEST-07 | Tests live in the same package (white-box) by default; `httpapi_test` package allowed for integration tests | Static (grep) | `grep -q '^package config$' internal/config/config_test.go` | ✅ test files | ⬜ pending |
| TEST-07 | `.gitignore` excludes `config.yaml`, `credentials.yaml`, `registry.json`, `bin/` | Static (grep) | `grep -q '^config.yaml$' .gitignore && grep -q '^credentials.yaml$' .gitignore && grep -q '^registry.json$' .gitignore && grep -q '^bin/$' .gitignore` | ✅ `.gitignore` | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Wave 0 is the "scaffolding wave" — create the files and directories so later waves have something to modify.

- [ ] `go.mod` and `go.sum` (via `go mod init github.com/openburo/openburo-server`)
- [ ] Directory layout: `cmd/server/`, `internal/{config,registry,wshub,httpapi,version}/`
- [ ] `internal/registry/doc.go` and `internal/wshub/doc.go` stubs (one-line package doc)
- [ ] `internal/config/config.go` with `Config`, `Credentials` types (initially empty bodies, filled in unit task)
- [ ] `internal/config/config_test.go` with table-driven test skeleton
- [ ] `internal/config/testdata/{valid,invalid_format,invalid_level,missing_required}.yaml` fixtures
- [ ] `internal/version/version.go` with `var Version = "dev"`
- [ ] `internal/httpapi/server.go` with `Server` struct + `New()` + `Handler()` scaffold
- [ ] `internal/httpapi/health.go` + `internal/httpapi/health_test.go`
- [ ] `cmd/server/main.go` with `-config` flag + `newLogger` helper + compose-root
- [ ] `Makefile` with all 7 targets + `LDFLAGS` version injection
- [ ] `.github/workflows/ci.yml`
- [ ] `config.example.yaml`
- [ ] `credentials.example.yaml`
- [ ] `.gitignore`
- [ ] `testify/require` dep pulled via `go get github.com/stretchr/testify/require`
- [ ] `go.yaml.in/yaml/v3` dep pulled via `go get go.yaml.in/yaml/v3`

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Startup banner emits 10 named keys in a single `slog.Info` line | FOUND-05 | Phase 1 has no slog-capture infrastructure — that ships in Phase 4 for the "no credentials in logs" test. Writing a subprocess test for Phase 1 alone is disproportionate and will be superseded. | `cp config.example.yaml config.yaml && cp credentials.example.yaml credentials.yaml && go run ./cmd/server 2>&1 \| head -1` and visually confirm all 10 keys present |
| GitHub Actions workflow YAML parses on GitHub | FOUND-06 | Full validation requires actually running on GitHub or installing `actionlint`. Phase 1 relies on grep-based structural checks + visual review. | Push to a branch and watch the CI run succeed, OR install `actionlint` locally and run `actionlint .github/workflows/ci.yml` |
| Quickstart flow: clean clone → `make run` → `curl localhost:8080/health` returns 200 | FOUND-07 / FOUND-04 | Exercises copy-the-examples UX that no automated test captures end-to-end | From a fresh working copy: `cp config.example.yaml config.yaml && cp credentials.example.yaml credentials.yaml && make run &` then `curl -i localhost:8080/health` |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags (Go `go test` is one-shot — N/A)
- [ ] Feedback latency < 10s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
