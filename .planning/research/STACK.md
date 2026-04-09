# Stack Research

**Project:** OpenBuro Server (Go HTTP + WebSocket app registry, reference implementation)
**Domain:** Small-to-medium Go HTTP REST + WebSocket server, file-backed, Basic Auth, single-binary
**Researched:** 2026-04-09
**Confidence:** HIGH (all major choices verified against Context7 / official repos / release pages)

## TL;DR — The Prescriptive Stack

A dependency-minimal, idiomatic Go 1.26 server. Standard library where possible, one dependency each for the things stdlib doesn't cover well: WebSocket, YAML, bcrypt, CORS, testing ergonomics.

```
Go 1.26 (latest stable, 2026-02-10)
├── net/http + http.ServeMux            (routing — no framework)
├── log/slog                             (structured logging — stdlib)
├── encoding/json                        (persistence — stdlib)
├── sync.RWMutex                         (concurrency — stdlib)
├── github.com/coder/websocket  v1.8.x   (WebSocket)
├── go.yaml.in/yaml/v3         v3.0.x   (YAML config + credentials)
├── golang.org/x/crypto/bcrypt           (password hashing)
├── github.com/rs/cors          v1.11.x  (CORS middleware)
└── github.com/stretchr/testify v1.11.x  (test assertions — require only)
```

Total direct dependencies: **5**. No framework, no viper, no ORM, no logger library.

## Recommended Stack

### Core Technologies

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| **Go** | **1.26** (latest: 1.26.2, 2026-04-07) | Language & runtime | Latest stable; target per `PROJECT.md` constraint "Go (latest stable)". Go 1.26 released 2026-02-10 brings GC/alloc/cgo perf work; minimum viable for this project is Go 1.22 (for `ServeMux` method/pattern matching) but pin the newest. |
| **`net/http` + `http.ServeMux`** | stdlib (Go 1.22+) | HTTP server and router | Since Go 1.22, `ServeMux` supports method matching (`POST /api/v1/registry`) and path wildcards (`GET /api/v1/registry/{appId}`) — exactly what this project needs. A reference implementation should prefer stdlib for clarity; readers don't need to learn a framework to read the code. Ben Hoyt, Alex Edwards, Eli Bendersky, and the Go blog all now recommend "start with ServeMux, reach for chi only when you need something it doesn't provide." This project needs nothing chi provides. |
| **`log/slog`** | stdlib (Go 1.21+) | Structured logging | Hard requirement from `PROJECT.md` ("`log/slog` only — no metrics backend"). Stdlib, zero deps, structured, handler-based. Slower than zerolog/zap but that's irrelevant for a reference implementation where throughput is not the bottleneck. Use `slog.NewJSONHandler(os.Stdout, ...)` in production, `slog.NewTextHandler` during dev. |
| **`encoding/json`** | stdlib | `registry.json` serialization | In-memory registry → JSON file on disk after each mutation. Stdlib JSON is fast enough; no need for goccy/go-json or sonic at this scale. Use `json.MarshalIndent` for human-readable on-disk format. |
| **`sync.RWMutex`** | stdlib | Thread-safe registry mutations | Explicit project constraint. Wrap the registry map; read-heavy workload (many GETs, occasional writes) is exactly the RWMutex sweet spot. |
| **`github.com/coder/websocket`** | **v1.8.14** (2024-09-06, actively maintained 2025-2026) | WebSocket server | **This is the big 2025 shift.** `gorilla/websocket` was archived; `nhooyr/websocket` was adopted by Coder and rebranded as `coder/websocket`. It is now the idiomatic choice: `context.Context` throughout, concurrent-write safety (eliminates the #1 prod bug with gorilla where two goroutines calling `WriteMessage` panic), built-in ping/pong, smaller API surface (~20 funcs vs gorilla's 50+). Perfect fit for a hub/client pattern. |

### Supporting Libraries

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| **`go.yaml.in/yaml/v3`** | **v3.0.x** | Parse `config.yaml` and `credentials.yaml` | **Note the new import path.** The original `go-yaml/yaml` repo was labeled unmaintained in April 2025; the YAML organization took over and the canonical path is now `go.yaml.in/yaml/v3`. `gopkg.in/yaml.v3` still works (points to the same code) but new projects should use the canonical path. API is identical. |
| **`golang.org/x/crypto/bcrypt`** | tracks Go toolchain | Bcrypt hash/verify for Basic Auth credentials | Correct import path confirmed still valid in 2026. Hard project requirement (bcrypt cost ≥ 12). Single function call: `bcrypt.CompareHashAndPassword(hash, pwd)`. Pull via `go get golang.org/x/crypto@latest`. |
| **`github.com/rs/cors`** | **v1.11.x** | CORS middleware for browser clients | Hand-rolled CORS is the #1 source of subtle bugs (wildcard + credentials, preflight caching, Vary header). `rs/cors` is a standard `http.Handler` wrapper, integrates cleanly with `ServeMux`, and handles WebSocket origin checks consistently. Alternative `jub0bs/cors` is technically better but less established; for a reference implementation, `rs/cors` is the safer "nobody gets surprised" pick. |
| **`github.com/stretchr/testify`** | **v1.11.1** (2025-08-27) | Test assertions (require package only) | Use `testify/require` exclusively (not `assert`). `require` stops the test on failure which is almost always what you want — continuing after a failed assertion in Go tests just produces noise. `httptest` from stdlib handles HTTP test servers; `require` handles the "is this body equal to that JSON" ergonomics. Do NOT pull in testify/mock or testify/suite — stick to `require` + table-driven `t.Run`. |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| `go test ./...` | Test runner | Table-driven pattern: `tests := []struct{ name string; ...}{...}; for _, tc := range tests { t.Run(tc.name, func(t *testing.T) { ... }) }`. Use `net/http/httptest.NewServer` for integration tests; `httptest.NewRecorder` for handler-level tests. |
| `go vet ./...` | Static analysis | Run in CI alongside `go test`. |
| `gofmt` / `go fmt ./...` | Formatter | Non-negotiable. |
| `staticcheck` | Extra linting | `honnef.co/go/tools/cmd/staticcheck@latest` — catches things `go vet` misses. Optional but recommended for a public reference impl. |
| `golangci-lint` | Aggregated linter | Optional. If used, keep the config minimal (govet, staticcheck, errcheck, ineffassign, gofmt) — don't bury readers in lint noise on a reference project. |
| `go mod tidy` | Dependency hygiene | Run before every commit. |

## Installation

```bash
# Initialize module (project root)
go mod init github.com/<org>/openburo-server

# Core (one go get per line for clarity in the module graph)
go get github.com/coder/websocket@latest
go get go.yaml.in/yaml/v3@latest
go get golang.org/x/crypto@latest          # brings in .../crypto/bcrypt
go get github.com/rs/cors@latest

# Test dependency
go get -t github.com/stretchr/testify/require@latest

# Verify
go mod tidy
go mod verify
```

Expected `go.mod` direct-dependency count: **5**. If it grows, question every addition.

## Project Layout (recommended)

For a single-binary server of this size, avoid the full `golang-standards/project-layout` ceremony. A flat-ish layout is clearer:

```
openburo-server/
├── cmd/
│   └── server/
│       └── main.go              # wiring only: load config, build deps, start server
├── internal/
│   ├── config/                  # config.yaml + credentials.yaml loading
│   ├── registry/                # registry core: manifest struct, store, JSON persistence
│   ├── httpapi/                 # HTTP handlers, routing, middleware (auth, CORS, logging)
│   └── wshub/                   # WebSocket hub + client pattern
├── config.example.yaml
├── credentials.example.yaml
├── go.mod
├── go.sum
└── README.md
```

**Rationale:**
- `cmd/server/` — conventional entrypoint directory. Only one binary for v1, but `cmd/` leaves room for a future `cmd/gen-credentials` helper without restructuring.
- `internal/` — everything else. Using `internal/` prevents external imports (this is a reference impl to *read*, not a library to *import*). It also signals "the public API is HTTP + WebSocket, not Go packages."
- **No `pkg/`** — nothing here is meant to be imported by third parties. Empty `pkg/` directories are a code smell.
- Four internal packages map 1:1 to the four domains in `PROJECT.md`: config, registry (storage + domain), httpapi (transport), wshub (real-time).
- `config.example.yaml` + `credentials.example.yaml` at the root make the "how do I run this" experience obvious.

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| `net/http` ServeMux | `go-chi/chi` v5.2.5 | Choose chi if you need nested route groups, per-group middleware stacks, URL-reversal, or more than ~5 middleware layers. **For this project, stdlib is sufficient** — there are ~5 routes and two middleware concerns (auth on writes, CORS everywhere). A reference implementation benefits from readers not having to learn chi. |
| `net/http` ServeMux | `gin` / `echo` / `fiber` | Never for this project. Gin/Echo impose their own request/response types (`gin.Context`), breaking the `http.Handler` contract — which is the universal language of Go HTTP. A reference implementation MUST speak stdlib idioms. |
| `coder/websocket` | `gorilla/websocket` | Only if maintaining an existing gorilla-based codebase. `gorilla/websocket` was archived; do not start new projects on it. Its concurrent-write-panic footgun is a known production hazard. |
| `coder/websocket` | `golang.org/x/net/websocket` | Never. The `x/net/websocket` package is effectively abandoned, doesn't implement modern WebSocket features, and the Go team explicitly directs users to third-party libraries. |
| `go.yaml.in/yaml/v3` | `spf13/viper` | Choose viper if you need: multi-format config (YAML + TOML + env + flags + remote), live reload, hierarchical key lookups, or `viper.Get("database.connection.timeout")`-style dotted access. **For this project**, config is two small YAML files loaded once at startup with no merging — viper is 10× more machinery than needed and pulls in ~20 transitive deps. |
| `go.yaml.in/yaml/v3` | `knadh/koanf` | A good lightweight alternative to viper with fewer deps. If you did need multi-source config, koanf > viper. Still overkill here — plain yaml.v3 `Unmarshal` into a struct is 15 lines. |
| `log/slog` | `zerolog` / `zap` | Only when you need sub-microsecond logging latency or ~100k+ logs/sec. Neither applies to a reference registry server. Also: `PROJECT.md` explicitly mandates `log/slog`. |
| `rs/cors` | `jub0bs/cors` | `jub0bs/cors` has better config validation, a debug mode, and a cleaner API. Choose it if you want strict CORS validation and are willing to pick the less-established library. `rs/cors` wins on "nobody will question this choice." |
| `rs/cors` | Hand-rolled CORS middleware | Only if you need exactly one origin and no preflight cache/credentials quirks. The three-line "set `Access-Control-Allow-Origin: *`" is famously wrong for anything real — don't start there. |
| `testify/require` | stdlib `testing` only | If you want zero test dependencies and your assertions are simple (`if got != want { t.Errorf(...) }`). Totally legitimate for a reference impl, and some Go purists prefer it. The project spec mentions testify as an option — pull it in *only* if table-driven tests start producing noisy boilerplate. Start with stdlib, add require when the boilerplate becomes painful. |
| `internal/` layout | Flat package (everything in `main`) | Never for a server with four domains. A flat layout encourages crossing concerns (WebSocket hub reaching into handler types, config types leaking into storage). The four-internal-package split enforces boundaries. |

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| **`gorilla/websocket`** | Archived; concurrent-write panic footgun. Starting a new project on an archived dep is a bad signal for a reference implementation. | `github.com/coder/websocket` |
| **`gin-gonic/gin`** | Non-`http.Handler` contract (`gin.Context`), middleware is gin-specific, route definitions look nothing like stdlib. Wrong idiom for a reference server. | `net/http` + `http.ServeMux` |
| **`labstack/echo`** | Same reason as gin — custom context, custom handler type. Also shipped a v5 dependency-conflict issue with yaml in 2025. | `net/http` + `http.ServeMux` |
| **`gofiber/fiber`** | Not built on `net/http` (uses `fasthttp`). Drops features like HTTP/2, `http.Flusher`, streaming. Alien to readers learning Go. | `net/http` + `http.ServeMux` |
| **`spf13/viper`** for this project | 20+ transitive deps for config that's ~30 lines of YAML loaded once. Concurrent reads+writes can panic (per maintainer note). | `go.yaml.in/yaml/v3` |
| **`sirupsen/logrus`** | Unstructured-first; active development slowed after slog landed. Maintainer recommends slog for new projects. | `log/slog` |
| **`uber-go/zap`** for this project | Excellent library, wrong project. Reference impl priority is *clarity*; zap's sugared vs. core distinction adds conceptual overhead that's not paying its keep here. | `log/slog` |
| **`gopkg.in/yaml.v2`** | Old v2 is missing fixes and has quirky bool parsing ("yes"/"no" = true/false). | `go.yaml.in/yaml/v3` |
| **`golang-standards/project-layout` (full form)** for this project | That repo is *not* an official Go standard (the README even says so). The full `cmd/ pkg/ internal/ api/ web/ configs/ test/ docs/ tools/ examples/ scripts/ build/ deployments/` structure is overkill for a 4-domain server and obscures the code. | Flat-ish `cmd/ + internal/` layout as above |
| **`testify/mock` / `testify/suite`** | `mock` encourages brittle mock-heavy tests instead of fakes; `suite` fights Go's idiomatic `t.Run` subtest pattern. | Hand-written fakes + table-driven `t.Run` + `testify/require` |
| **ORM libraries (`gorm`, `ent`, `sqlc`)** | No database in v1 (explicit PROJECT.md constraint). | `encoding/json` + `sync.RWMutex` + file persistence |

## Stack Patterns by Variant

**If the registry grows beyond ~10MB on disk:**
- Replace whole-file `registry.json` rewrite with an append-only log + periodic snapshot
- Or: migrate to embedded SQLite via `modernc.org/sqlite` (pure Go, no cgo)
- Rationale: Rewriting a multi-MB file on every mutation becomes the bottleneck long before RWMutex contention does

**If v2 needs multi-instance / HA:**
- Replace in-memory + JSON with Postgres via `jackc/pgx/v5`
- Replace the in-process hub with a Redis pub/sub fan-out
- Keep `coder/websocket`, `net/http`, `slog` — only the persistence layer changes

**If v2 needs OAuth/OIDC instead of Basic Auth:**
- Add `github.com/coreos/go-oidc/v3` for token validation
- Keep bcrypt around only for service-account credentials
- Consider a dedicated auth middleware package under `internal/auth/`

**If the WebSocket client count grows past ~10k concurrent:**
- Profile with `pprof` before optimizing
- `coder/websocket` scales well on its own; the hub fan-out is usually the bottleneck
- Consider sharding the hub by appId hash

## Version Compatibility

| Package | Compatible With | Notes |
|---------|-----------------|-------|
| Go 1.26 | All recommended deps | Go 1.22+ is the floor (required for `ServeMux` method matching). Go 1.21+ required for `log/slog`. Pin `go 1.26` in `go.mod`. |
| `coder/websocket` v1.8.x | Go 1.22+ | Uses `context.AfterFunc` (Go 1.21+). No breaking changes expected before v2. |
| `go.yaml.in/yaml/v3` v3.0.x | Any Go | API-identical to `gopkg.in/yaml.v3`. Don't mix both import paths in one module — pick one (the new one). |
| `rs/cors` v1.11.x | `net/http` (including wrapped handlers from ServeMux) | Works as a standard `http.Handler` wrapper; no router-specific coupling. |
| `testify/require` v1.11.x | Go 1.18+ | Stable API; v2 is in long-term planning but not shipping. Safe to pin v1.11. |
| `golang.org/x/crypto` | tracks recent Go releases | Always pull `@latest` with recent Go toolchain; it's part of the extended standard library. |

**Known gotcha:** If a transitive dependency pulls in `gopkg.in/yaml.v3`, you may end up with both `gopkg.in/yaml.v3` and `go.yaml.in/yaml/v3` in `go.sum`. They're compatible, but run `go mod why gopkg.in/yaml.v3` to identify the transitive source and decide whether to care. For this project's dep set, only direct yaml usage should appear.

## Confidence Breakdown

| Choice | Confidence | Basis |
|--------|------------|-------|
| Go 1.26 as target | HIGH | Verified via `endoflife.date/go` and `go.dev/doc/devel/release` (1.26.2 released 2026-04-07) |
| `net/http` ServeMux over chi/gin | HIGH | Alex Edwards, Ben Hoyt, Eli Bendersky, Go blog all converged on this post-1.22; and this project's routing needs are trivially covered |
| `coder/websocket` | HIGH | Confirmed: gorilla archived, nhooyr transferred to coder org, actively maintained through 2026 (AVX2 masking PR Jan 2026, Go 1.25 update Dec 2025). Latest tagged v1.8.14 (2024-09-06) with ongoing mainline work |
| `log/slog` | HIGH | Stdlib since 1.21; explicit project constraint |
| `go.yaml.in/yaml/v3` canonical path | HIGH | Confirmed: YAML org took over after go-yaml was marked unmaintained in April 2025; new canonical path is `go.yaml.in/yaml/v3` |
| `golang.org/x/crypto/bcrypt` import path | HIGH | Verified current in pkg.go.dev as of March 2026 |
| `rs/cors` over `jub0bs/cors` | MEDIUM | `jub0bs/cors` is technically better per 2024-2025 analyses, but less established. `rs/cors` is the conservative choice for a reference impl. Either would work. |
| `testify/require` (v1.11.1) | HIGH | Latest release 2025-08-27 verified via GitHub releases |
| Flat `cmd/` + `internal/` layout | MEDIUM-HIGH | Consensus across 2024-2026 Go layout discussions is "don't use the full golang-standards repo for small projects." The specific four-package split is opinionated but maps cleanly to the PROJECT.md domains |
| Not using chi | MEDIUM | Defensible for this project specifically. If readers expect chi (common in tutorials), they may ask "why not chi?" — the answer is in this doc |
| Not using viper | HIGH | Config surface is tiny; viper is obviously too much |

## Sources

**Primary (HIGH confidence):**
- [Go release history — go.dev](https://go.dev/doc/devel/release) — verified Go 1.26.2 as latest stable (2026-04-07)
- [endoflife.date/go](https://endoflife.date/go) — verified 1.26 release date (2026-02-10) and support window
- [coder/websocket on GitHub](https://github.com/coder/websocket) — verified active maintenance, v1.8.14 latest tag, Go 1.25 support
- [A New Home for nhooyr/websocket — Coder blog](https://coder.com/blog/websocket) — verified the nhooyr → coder transition
- [go-chi/chi releases](https://github.com/go-chi/chi/releases) — verified chi v5.2.5 (2025-02-05) as latest
- [testify releases](https://github.com/stretchr/testify/releases) — verified v1.11.1 (2025-08-27)
- [pkg.go.dev: golang.org/x/crypto/bcrypt](https://pkg.go.dev/golang.org/x/crypto/bcrypt) — verified current import path
- [pkg.go.dev: go.yaml.in/yaml/v3](https://pkg.go.dev/go.yaml.in/yaml/v3) — verified new canonical YAML path
- [Routing Enhancements for Go 1.22 — Go blog](https://go.dev/blog/routing-enhancements) — ServeMux method + pattern matching

**Secondary (MEDIUM confidence — opinion/analysis pieces):**
- [Which Go Router Should I Use? — Alex Edwards](https://www.alexedwards.net/blog/which-go-router-should-i-use) — post-1.22 ServeMux-first recommendation
- [Different approaches to HTTP routing in Go — Ben Hoyt](https://benhoyt.com/writings/go-routing/) — comparative analysis
- [Go's 1.22+ ServeMux vs Chi Router — Calhoun.io](https://www.calhoun.io/go-servemux-vs-chi/)
- [WebSocket.org: Go WebSocket Server Guide — coder/websocket vs Gorilla](https://websocket.org/guides/languages/go/)
- [jub0bs/cors vs rs/cors analysis](https://jub0bs.com/posts/2024-04-27-jub0bs-cors-a-better-cors-middleware-library-for-go/)
- [Assert vs require in testify — YellowDuck.be](https://www.yellowduck.be/posts/assert-vs-require-in-testify)
- [Go Ecosystem Trends 2025 — JetBrains GoLand blog](https://blog.jetbrains.com/go/2025/11/10/go-language-trends-ecosystem-2025/)
- [labstack/echo yaml dependency conflict issue #2806](https://github.com/labstack/echo/issues/2806) — confirmed the yaml import path migration is affecting real projects

**Project inputs:**
- `.planning/PROJECT.md` — hard constraints (slog, bcrypt, Basic Auth, file persistence, coder/websocket implied by "WebSocket hub pattern"), explicit out-of-scope list

---
*Stack research for: Go HTTP REST + WebSocket reference server (OpenBuro)*
*Researched: 2026-04-09*
