# OpenBuro Server

## What This Is

OpenBuro Server is a Go-based app registry and capability broker for the OpenBuro ecosystem — an open standard for inter-app communication inspired by Android intents. It stores application manifests, exposes their declared capabilities (e.g. `PICK`, `SAVE`) via a REST API, and notifies connected clients of registry changes in real time over WebSocket. It is the **reference implementation** of the OpenBuro platform layer (the mediator between client apps and capability providers like drives or editors).

## Core Value

A client app can discover, at any moment, which other apps can fulfill a given intent (e.g. "pick a file of MIME type X"), and be notified instantly when that set changes.

## Requirements

### Validated

<!-- Shipped and confirmed valuable. -->

**Foundation (Phase 1, shipped 2026-04-10)**
- ✓ Load `config.yaml` at startup (port, TLS, credential file path, registry file path, WS ping interval) — Phase 1
- ✓ `GET /health` endpoint (no auth) for liveness checks — Phase 1
- ✓ Structured logging via `log/slog` with injected `*slog.Logger` (no `slog.Default()` in internal/) — Phase 1
- ✓ Project structure follows idiomatic Go server layout (`cmd/server/` + `internal/{config,registry,wshub,httpapi,version}/`) — Phase 1
- ✓ Go 1.26 build with pinned deps, `go test -race` green, `gofmt`/`vet`/`staticcheck` CI, Makefile — Phase 1

### Active

<!-- Current scope. Building toward these. -->

**Registry CRUD**
- [ ] Upsert an app manifest via `POST /api/v1/registry` (Basic Auth, returns 201/200)
- [ ] Delete an app manifest via `DELETE /api/v1/registry/{appId}` (Basic Auth, returns 204/404)
- [ ] List all manifests via `GET /api/v1/registry` (public)
- [ ] Fetch a single manifest via `GET /api/v1/registry/{appId}` (public)
- [ ] Validate manifest payload (required fields, `action` enum, non-empty `mimeTypes`)

**Capabilities aggregation**
- [ ] Aggregate all capabilities across manifests via `GET /api/v1/capabilities`
- [ ] Filter capabilities by `action` query param
- [ ] Filter capabilities by `mimeType` query param with `*/*` wildcard matching

**Real-time notifications**
- [ ] WebSocket endpoint `GET /api/v1/capabilities/ws` broadcasts `REGISTRY_UPDATED` events on any manifest change
- [ ] WebSocket hub pattern (centralized, thread-safe fan-out to connected clients)
- [ ] Periodic ping frames (configurable, default 30s) to keep connections alive

**Authentication**
- [ ] HTTP Basic Auth on write routes (`POST`, `DELETE`)
- [ ] Credentials loaded from `credentials.yaml` with bcrypt-hashed passwords (cost ≥ 12)
- [ ] Credentials never appear in logs

**Persistence**
- [ ] In-memory registry with `sync.RWMutex`-protected mutations
- [ ] Serialization to `registry.json` on disk after each mutation
- [ ] Load existing `registry.json` at startup

**Configuration**
- [ ] Optional TLS termination when `server.tls.enabled = true`

**Ops**
- [ ] CORS configured to allow browser clients (REST + WebSocket origin)

**Quality**
- [ ] Standard Go test suite: table-driven unit tests for core logic + HTTP/WS integration tests

### Out of Scope

<!-- Explicit boundaries. Includes reasoning to prevent re-adding. -->

- **Pluggable storage backend (SQLite/Postgres)** — v2 evolution; in-memory + JSON file is sufficient for the reference implementation
- **Optimistic concurrency / version conflict (`409`)** — v2 evolution; single-admin assumption for v1
- **Hot-reload of `credentials.yaml`** — v1 requires restart; hot-reload adds complexity without hackathon value
- **Authentication on read routes (including WS)** — v1 registry is publicly readable by design; restrict in v2 if needed
- **Multi-tenant / namespacing of registries** — single registry per server instance for v1
- **Rate limiting / abuse protection** — reference implementation, not hardened service
- **Stress testing for concurrency** — standard test coverage is sufficient; no dedicated stress suite
- **Prometheus metrics / full observability stack** — structured logs only for v1

## Context

- **Ecosystem:** Part of the broader OpenBuro project, an open standard for inter-app communication modeled on Android intents / Cozy Cloud intents / Freedesktop portals. This server implements the "Plateforme" layer in the three-tier architecture (App cliente ↔ Plateforme ↔ Source/Capability).
- **First concrete use case:** File Picker — any client app (mail, docs, chat) can discover and invoke a file picker exposed by any drive (TDrive, Fichier DINUM, Nextcloud, etc.) via standardized capabilities.
- **Prior art informing design:** Android intent-filters, Cozy Cloud intents, XDG portals (notably `org.freedesktop.portal.FileChooser`). See `../open-buro-dossier-technique-file-picker.md` and `../docs/etat-de-lart/`.
- **Target consumers:** Browser-based client apps (via fetch + WebSocket) and CLI tools / Go clients. CORS must be configured accordingly.
- **Hackathon context:** Open Buro hackathon, April 2026 — this server is the platform layer demonstrated during the event.

## Constraints

- **Tech stack:** Go (latest stable) — chosen to match the broader OpenBuro reference stack
- **Auth:** HTTP Basic Auth over TLS only in production; no OAuth/OIDC in v1
- **Persistence:** File-based (`registry.json`), no database in v1
- **Thread safety:** All registry mutations must be thread-safe (`sync.RWMutex` or equivalent)
- **WebSocket pattern:** Centralized hub/client pattern, not per-connection goroutine storms
- **Security:** Credentials never logged, bcrypt cost ≥ 12, passwords never stored in plaintext
- **Observability:** `log/slog` only — no metrics backend, no tracing

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Reference implementation focus (not production deployment) | OpenBuro is a spec; this server demonstrates it clearly over being hardened | — Pending |
| In-memory + JSON file persistence for v1 | Zero external dependencies, trivial to run for demos, sufficient for expected registry size | — Pending |
| HTTP Basic Auth only on write routes | Reads are public by design in the OpenBuro model; keeps the demo path friction-free | — Pending |
| Browser + CLI clients supported (CORS enabled) | Matches expected consumer set (web apps, curl, Go clients) | — Pending |
| Structured logs via `log/slog`, no metrics | Sufficient for reference impl; avoids pulling in Prometheus/OTel | — Pending |
| Standard test rigor (unit + integration, no stress) | Correctness matters; concurrency stress testing is overkill for the expected load | — Pending |
| Project layout decided during planning | Apply idiomatic Go patterns once scope is clearer; no premature structuring | ✓ Good — 4-package `internal/` layout landed in Phase 1 |
| `log/slog` injected everywhere, no `slog.Default()` in `internal/` | Structural enforcement of "credentials never logged" via a grep gate | ✓ Good — gate passing in Phase 1 |
| GitHub Actions `@v6`/`@v6` (revised from `@v4`/`@v5`) | Node 20 EOL June 2026 | ✓ Good — CI pipeline using current majors |

---
*Last updated: 2026-04-10 after Phase 1 completion*
