---
phase: 01-foundation
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - go.mod
  - go.sum
  - Makefile
  - .gitignore
  - .github/workflows/ci.yml
  - cmd/server/main.go
  - internal/registry/doc.go
  - internal/wshub/doc.go
  - internal/version/version.go
autonomous: true
requirements:
  - FOUND-01
  - FOUND-06
  - TEST-07
must_haves:
  truths:
    - "`go build ./...` succeeds on Go 1.26 from a clean checkout"
    - "`go vet ./...`, `gofmt -l .`, and `staticcheck ./...` all pass on the scaffold"
    - "CI workflow `.github/workflows/ci.yml` runs every required step in order"
    - "`make build`, `make test`, `make lint`, `make ci`, `make fmt`, `make run`, `make clean` all resolve via Makefile"
    - "Directory layout matches TEST-07: `cmd/server/` + `internal/{config,registry,wshub,httpapi,version}/`"
  artifacts:
    - path: "go.mod"
      provides: "Module path `github.com/openburo/openburo-server` + `go 1.26` directive + pinned deps"
      contains: "module github.com/openburo/openburo-server"
    - path: "Makefile"
      provides: "Developer command interface"
      contains: "LDFLAGS"
    - path: ".github/workflows/ci.yml"
      provides: "GitHub Actions CI pipeline"
      contains: "actions/setup-go@v6"
    - path: ".gitignore"
      provides: "Ignores build outputs + local config + IDE cruft"
      contains: "config.yaml"
    - path: "cmd/server/main.go"
      provides: "package main stub that compiles (body filled in Plan 03)"
      contains: "package main"
    - path: "internal/registry/doc.go"
      provides: "Phase 1 stub package for registry (TEST-07 layout verification)"
      contains: "package registry"
    - path: "internal/wshub/doc.go"
      provides: "Phase 1 stub package for wshub (TEST-07 layout verification)"
      contains: "package wshub"
    - path: "internal/version/version.go"
      provides: "`var Version = \"dev\"` overridden at build time via ldflags"
      contains: "var Version"
  key_links:
    - from: "Makefile"
      to: "internal/version/version.go"
      via: "LDFLAGS -X symbol path"
      pattern: "internal/version.Version="
    - from: ".github/workflows/ci.yml"
      to: "Makefile"
      via: "Same command sequence (lint -> test -> build)"
      pattern: "go test ./\\.\\.\\. -race"
---

<objective>
Bootstrap the repository skeleton: initialize the Go module with pinned dependencies, create the four-package `internal/` layout, ship the Makefile and GitHub Actions CI pipeline, and commit a `.gitignore` that keeps operator-local files out of version control. This plan produces a buildable, CI-ready, but functionally empty scaffold.

Purpose: Establish the project's physical shape before any business logic is written so Plans 02 and 03 can drop their files into a layout that already compiles and already has CI green.
Output: A repository where `go build ./...`, `go vet ./...`, `gofmt -l .`, `staticcheck ./...`, and `make ci` all succeed, even though the server does nothing meaningful yet.

**Note on GitHub Actions versions:** CONTEXT.md specifies `actions/checkout@v4` and `actions/setup-go@v5`. This plan uses `actions/checkout@v6` and `actions/setup-go@v6` per RESEARCH.md recommendation — v4/v5 hit Node 20 EOL in June 2026. This is a current-state correction, not a strategic change. If the user prefers v4/v5, it is a one-line edit in `.github/workflows/ci.yml`.
</objective>

<execution_context>
@.planning/phases/01-foundation/01-CONTEXT.md
@.planning/phases/01-foundation/01-RESEARCH.md
@.planning/phases/01-foundation/01-VALIDATION.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/REQUIREMENTS.md
@.planning/research/STACK.md
@.planning/research/ARCHITECTURE.md
@.planning/research/PITFALLS.md
</context>

<tasks>

<task type="auto">
  <name>Task 1: Initialize Go module, scaffold directory layout, pull dependencies</name>
  <files>go.mod, go.sum, cmd/server/main.go, internal/version/version.go, internal/registry/doc.go, internal/wshub/doc.go, internal/config/.gitkeep, internal/httpapi/.gitkeep</files>
  <read_first>
    - .planning/phases/01-foundation/01-CONTEXT.md (§Module & Repository, §Package Layout)
    - .planning/phases/01-foundation/01-RESEARCH.md (§Standard Stack, §Directory Scaffold Commands, §Additional concrete files)
    - .planning/phases/01-foundation/01-VALIDATION.md (§Wave 0 Requirements, §Per-Task Verification Map rows for FOUND-01, TEST-07)
    - .planning/research/STACK.md (TL;DR pinned deps block)
  </read_first>
  <action>
Initialize the Go module and scaffold the complete TEST-07 directory layout.

Step 1: From the repo root, run:

```bash
go mod init github.com/openburo/openburo-server
```

This must produce a `go.mod` whose first two non-blank lines are:

```
module github.com/openburo/openburo-server

go 1.26
```

If `go mod init` writes a different `go` directive (e.g. `go 1.25`), edit the file so it reads `go 1.26` exactly — the CI workflow pins `go-version: '1.26'` and mismatched directives cause toolchain warnings.

Step 2: Create the directory scaffold:

```bash
mkdir -p cmd/server
mkdir -p internal/config/testdata
mkdir -p internal/version
mkdir -p internal/registry
mkdir -p internal/wshub
mkdir -p internal/httpapi
mkdir -p .github/workflows
```

Step 3: Write `internal/version/version.go` verbatim (the Makefile LDFLAGS target this exact symbol path):

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

Step 4: Write `internal/registry/doc.go` verbatim:

```go
// Package registry holds the in-memory manifest store, domain types
// (Manifest, Capability), MIME wildcard matching, and atomic JSON
// persistence. It is the pure domain core and depends on nothing
// from other internal/ packages.
//
// Phase 1 ships this file only; the real implementation lands in Phase 2.
package registry
```

Step 5: Write `internal/wshub/doc.go` verbatim:

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

Step 6: Write `cmd/server/main.go` as a minimal but compilable stub. Plan 03 will replace the body; for now it must compile cleanly so `go build ./...` passes:

```go
// Package main is the compose-root of the OpenBuro server.
// Plan 01-03 replaces this stub with the real main() that loads config,
// constructs a slog logger, logs the startup banner, and serves /health.
package main

func main() {
	// Plan 01-03 fills in: flag parsing, config.Load, newLogger, startup
	// banner, httpapi.New, http.Server.ListenAndServe.
}
```

Step 7: Create empty `.gitkeep` files in `internal/config/` and `internal/httpapi/` so the directories are tracked by git even before Plans 02 and 03 add real files:

```bash
touch internal/config/.gitkeep
touch internal/httpapi/.gitkeep
```

These will be deleted automatically when Plans 02 and 03 write real files into those directories.

Step 8: Pull the two Phase 1 direct dependencies (latest versions — do NOT hand-edit go.mod):

```bash
go get go.yaml.in/yaml/v3@latest
go get -t github.com/stretchr/testify/require@latest
go mod tidy
go mod verify
```

Step 9: Verify the scaffold compiles and the module graph is clean:

```bash
go build ./...
go vet ./...
gofmt -l .
```

All three commands must exit 0, and `gofmt -l .` must produce no output. If `gofmt` flags a file, run `gofmt -w .` and re-verify.

**Phase 1 does NOT pull:**
- `github.com/coder/websocket` (Phase 3)
- `golang.org/x/crypto/bcrypt` (Phase 4)
- `github.com/rs/cors` (Phase 4)

If `go mod tidy` somehow adds these, the scaffold is wrong — investigate and remove them before committing.
  </action>
  <verify>
    <automated>test -f go.mod &amp;&amp; test -f go.sum &amp;&amp; grep -q '^module github.com/openburo/openburo-server$' go.mod &amp;&amp; grep -q '^go 1.26$' go.mod &amp;&amp; grep -q 'go.yaml.in/yaml/v3' go.mod &amp;&amp; grep -q 'github.com/stretchr/testify' go.mod &amp;&amp; ! grep -q 'coder/websocket' go.mod &amp;&amp; ! grep -q 'crypto/bcrypt' go.mod &amp;&amp; ! grep -q 'rs/cors' go.mod &amp;&amp; test -d cmd/server &amp;&amp; test -d internal/config &amp;&amp; test -d internal/registry &amp;&amp; test -d internal/wshub &amp;&amp; test -d internal/httpapi &amp;&amp; test -d internal/version &amp;&amp; test -f internal/registry/doc.go &amp;&amp; test -f internal/wshub/doc.go &amp;&amp; test -f internal/version/version.go &amp;&amp; test -f cmd/server/main.go &amp;&amp; grep -q '^package registry' internal/registry/doc.go &amp;&amp; grep -q '^package wshub' internal/wshub/doc.go &amp;&amp; grep -q '^package version' internal/version/version.go &amp;&amp; grep -q 'var Version = "dev"' internal/version/version.go &amp;&amp; grep -q '^package main' cmd/server/main.go &amp;&amp; go build ./... &amp;&amp; go vet ./... &amp;&amp; test -z "$(gofmt -l .)"</automated>
  </verify>
  <acceptance_criteria>
    - `test -f go.mod` succeeds
    - `test -f go.sum` succeeds
    - `grep -q '^module github.com/openburo/openburo-server$' go.mod` succeeds
    - `grep -q '^go 1.26$' go.mod` succeeds
    - `grep -q 'go.yaml.in/yaml/v3' go.mod` succeeds (yaml v3 dep pinned)
    - `grep -q 'github.com/stretchr/testify' go.mod` succeeds (testify dep pinned)
    - `! grep -q 'coder/websocket' go.mod` — NOT pulled in Phase 1
    - `! grep -q 'golang.org/x/crypto' go.mod` — NOT pulled in Phase 1
    - `! grep -q 'rs/cors' go.mod` — NOT pulled in Phase 1
    - `test -d cmd/server && test -d internal/config && test -d internal/registry && test -d internal/wshub && test -d internal/httpapi && test -d internal/version` succeeds
    - `test -f internal/registry/doc.go && grep -q '^package registry' internal/registry/doc.go`
    - `test -f internal/wshub/doc.go && grep -q '^package wshub' internal/wshub/doc.go`
    - `test -f internal/version/version.go && grep -q 'var Version = "dev"' internal/version/version.go`
    - `test -f cmd/server/main.go && grep -q '^package main' cmd/server/main.go`
    - `go build ./...` exits 0
    - `go vet ./...` exits 0
    - `gofmt -l .` produces empty output
  </acceptance_criteria>
  <done>
Module initialized at `github.com/openburo/openburo-server` with `go 1.26`, exactly two direct deps (`go.yaml.in/yaml/v3` + `github.com/stretchr/testify`), complete TEST-07 directory layout, stub `doc.go` files in registry + wshub, minimal compilable `cmd/server/main.go`, `internal/version/version.go` with `var Version = "dev"`. `go build ./...` and `go vet ./...` both green; `gofmt -l .` produces no output.
  </done>
</task>

<task type="auto">
  <name>Task 2: Write Makefile, .gitignore, and GitHub Actions CI workflow</name>
  <files>Makefile, .gitignore, .github/workflows/ci.yml</files>
  <read_first>
    - .planning/phases/01-foundation/01-CONTEXT.md (§CI & Build Tooling, §.gitignore block)
    - .planning/phases/01-foundation/01-RESEARCH.md (§.github/workflows/ci.yml, §Makefile, §Pitfall 6 Makefile tabs)
    - .planning/phases/01-foundation/01-VALIDATION.md (§Per-Task Verification Map rows for FOUND-06)
    - go.mod (confirm module path matches the LDFLAGS symbol in Makefile)
  </read_first>
  <action>
Create the three scaffolding files that live at the repo root: `Makefile`, `.gitignore`, and `.github/workflows/ci.yml`.

**CRITICAL — Makefile indentation:** Makefile recipe lines MUST start with a HARD TAB character (`\t`), NOT spaces. Any editor or clipboard transformation that substitutes spaces for tabs will cause `make build` to fail with `Makefile:N: *** missing separator.  Stop.`. Verify with `cat -A Makefile | grep -P '^\s' | grep -v '^\^I'` — any match is a bug.

Step 1: Write `Makefile` verbatim. The symbol path in `LDFLAGS` must match the Go import path for `internal/version`, so `github.com/openburo/openburo-server/internal/version.Version` must appear exactly:

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

Every indented recipe line (`go build ...`, `go run ...`, `@unformatted=...`, the `if` block continuations, `go vet ...`, etc.) must begin with a literal tab character.

Step 2: Write `.gitignore` verbatim (per CONTEXT.md, no deviations):

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

Step 3: Write `.github/workflows/ci.yml` verbatim:

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

Step 4: Verify the Makefile parses and that the minimum viable targets resolve. Since Plans 02 and 03 haven't written `internal/config` or `internal/httpapi` yet, `make test` will currently only exercise stub packages — that's expected. What must work NOW is:

```bash
make build    # Produces bin/openburo-server
make lint     # All three sub-steps pass
./bin/openburo-server --help 2>&1 | grep -q "flag provided but not defined\|Usage" || true
```

The binary may not have any flags yet (that's Plan 03), but it must build and be executable. After verification, run `make clean` to remove the binary so the scaffold commit doesn't include build artifacts.

**Do NOT run `make run`** — there is no `config.yaml` yet and Plan 03 hasn't wired the config loader. That target is for future use.
  </action>
  <verify>
    <automated>test -f Makefile &amp;&amp; test -f .gitignore &amp;&amp; test -f .github/workflows/ci.yml &amp;&amp; grep -q '^config.yaml$' .gitignore &amp;&amp; grep -q '^credentials.yaml$' .gitignore &amp;&amp; grep -q '^registry.json$' .gitignore &amp;&amp; grep -q '^bin/$' .gitignore &amp;&amp; grep -q 'internal/version.Version' Makefile &amp;&amp; for t in build run test lint fmt ci clean; do grep -q "^$t:" Makefile || exit 1; done &amp;&amp; grep -q -- '-race' Makefile &amp;&amp; grep -q 'actions/checkout@v6' .github/workflows/ci.yml &amp;&amp; grep -q 'actions/setup-go@v6' .github/workflows/ci.yml &amp;&amp; grep -q "go-version: '1.26'" .github/workflows/ci.yml &amp;&amp; grep -q 'go test ./\.\.\. -race' .github/workflows/ci.yml &amp;&amp; grep -q 'go vet ./\.\.\.' .github/workflows/ci.yml &amp;&amp; grep -q 'gofmt -l \.' .github/workflows/ci.yml &amp;&amp; grep -q 'staticcheck' .github/workflows/ci.yml &amp;&amp; make build &amp;&amp; test -x bin/openburo-server &amp;&amp; make clean &amp;&amp; test ! -d bin</automated>
  </verify>
  <acceptance_criteria>
    - `test -f Makefile` succeeds
    - `test -f .gitignore` succeeds
    - `test -f .github/workflows/ci.yml` succeeds
    - All 7 Makefile targets exist: `for t in build run test lint fmt ci clean; do grep -q "^$t:" Makefile || exit 1; done` succeeds
    - `grep -q 'internal/version.Version' Makefile` — LDFLAGS targets correct symbol path
    - `grep -q -- '-race' Makefile` — test target uses race detector
    - `.gitignore` contains: `config.yaml`, `credentials.yaml`, `registry.json`, `bin/` (one per line, at line start)
    - `.github/workflows/ci.yml` contains `actions/checkout@v6` (NOT v4)
    - `.github/workflows/ci.yml` contains `actions/setup-go@v6` (NOT v5)
    - `.github/workflows/ci.yml` contains `go-version: '1.26'`
    - `.github/workflows/ci.yml` contains all required step commands: `gofmt -l .`, `go vet ./...`, `staticcheck`, `go build ./...`, `go test ./... -race`
    - `make build` succeeds (exit 0) and produces `bin/openburo-server`
    - `make clean` removes `bin/`
    - Makefile recipe lines use HARD TABS (verified by successful `make build`; if they were spaces, `make` would fail with `missing separator`)
  </acceptance_criteria>
  <done>
Repo root has Makefile (7 targets, hard-tab recipes, LDFLAGS pointing at `internal/version.Version`), .gitignore (build outputs + local config + IDE cruft), and `.github/workflows/ci.yml` (single job, 8 steps, `@v6` actions, Go 1.26). `make build` produces the binary, `make clean` removes it, and `make lint` would pass on the empty scaffold (verified via the lint step of the individual commands if the executor chooses to run it — not required because it redownloads staticcheck on each cold run).
  </done>
</task>

</tasks>

<verification>
Run the full scaffold smoke pass after both tasks are complete:

```bash
go build ./...                            # exit 0
go vet ./...                              # exit 0
test -z "$(gofmt -l .)"                   # empty output
make build                                # produces bin/openburo-server
make clean                                # removes bin/
test -f go.mod && test -f go.sum
test -f Makefile && test -f .gitignore && test -f .github/workflows/ci.yml
test -d cmd/server
test -d internal/config
test -d internal/registry
test -d internal/wshub
test -d internal/httpapi
test -d internal/version
test -f internal/registry/doc.go
test -f internal/wshub/doc.go
test -f internal/version/version.go
test -f cmd/server/main.go
```

All commands must exit 0 and produce no unexpected output.
</verification>

<success_criteria>
- Repo scaffold compiles cleanly on Go 1.26 (FOUND-01)
- TEST-07 layout is verifiable on disk: `cmd/server/` + `internal/{config,registry,wshub,httpapi,version}/`
- Makefile provides 7 canonical developer commands and CI mirrors them exactly (FOUND-06)
- `go.mod` has exactly 2 direct dependencies (`go.yaml.in/yaml/v3`, `github.com/stretchr/testify`) — the 5-dep ceiling from STACK.md has headroom for Phases 3 and 4
- `.gitignore` prevents operator-local files from being committed
- GitHub Actions workflow is structurally complete and uses current action majors (`@v6`)
</success_criteria>

<output>
After completion, create `.planning/phases/01-foundation/01-01-SUMMARY.md` documenting:
- Module path chosen and `go 1.26` directive
- Exact versions landed for `yaml/v3` and `testify` (read from `go.mod`)
- Files created (full list)
- Note on the `@v6` vs `@v4`/`@v5` GitHub Actions version choice
- `make ci` expected runtime (first run downloads staticcheck ~20s)
</output>
