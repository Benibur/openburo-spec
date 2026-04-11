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

**Registry Core (Phase 2, shipped 2026-04-10)**
- ✓ In-memory `Store` with `sync.RWMutex`-protected mutations (Upsert/Delete/Get/List) — Phase 2
- ✓ Manifest domain type + `Validate()` (required fields, `action` enum, non-empty `mimeTypes`) — Phase 2
- ✓ Atomic JSON persistence to `registry.json` (CreateTemp→Sync→Rename→dir fsync) with in-memory rollback on persist failure — Phase 2
- ✓ Load existing `registry.json` at startup (empty/missing/valid/malformed/wrong-version/invalid-manifest/unknown-field paths) — Phase 2
- ✓ Capability aggregation view `Store.Capabilities(filter)` with symmetric `*/*` MIME wildcard matching and deterministic sort — Phase 2

**HTTP API (Phase 4, shipped 2026-04-10)**
- ✓ REST handlers for `POST/DELETE /api/v1/registry[/{appId}]` (Basic Auth, 201/200/400/401/204/404) — Phase 4
- ✓ Public read routes `GET /api/v1/registry`, `GET /api/v1/registry/{appId}`, `GET /api/v1/capabilities?action=&mimeType=` — Phase 4
- ✓ `GET /api/v1/capabilities/ws` WebSocket upgrade with full-state `SNAPSHOT` on connect — Phase 4
- ✓ `REGISTRY_UPDATED` event broadcast on every upsert/delete with `{event, timestamp, payload: {appId, change}}` shape — Phase 4
- ✓ Timing-safe Basic Auth (bcrypt unconditional via dummyHash + `subtle.ConstantTimeCompare` tuple, PITFALLS #8) — Phase 4
- ✓ `LoadCredentials` from `credentials.yaml` with bcrypt cost ≥ 12 enforced at load — Phase 4
- ✓ Structured audit log on writes (`user`, `action`, `appId`) with zero PII — Phase 4
- ✓ Middleware chain: recover → log → CORS → per-route auth; handler panics caught, server stays alive — Phase 4
- ✓ `rs/cors` middleware with config-driven allow-list, shared with WebSocket `OriginPatterns` — Phase 4
- ✓ Error envelope `{error, details}` on all 4xx/5xx responses — Phase 4
- ✓ 70 httpapi tests + full REST/WS round-trip via `httptest.NewServer`, race-clean — Phase 4

**WebSocket Hub (Phase 3, shipped 2026-04-10)**
- ✓ Leak-free `internal/wshub` hub on `coder/websocket` v1.8.14 with `Hub` + `subscriber` + buffered outbound channel (default 16) — Phase 3
- ✓ Non-blocking `Publish([]byte)` fan-out with drop-slow-consumer via `StatusPolicyViolation` — Phase 3
- ✓ `Subscribe(ctx, conn)` writer loop with `conn.CloseRead(ctx)` + `defer removeSubscriber` (goroutine-leak prevention) — Phase 3
- ✓ Periodic ping keepalive (default 30s, configurable via `Options.PingInterval`) — Phase 3
- ✓ `Hub.Close()` sends `StatusGoingAway` close frames to every subscriber (ready for Phase 5 two-phase shutdown) — Phase 3
- ✓ Correctness oracle: 1000-cycle goroutine-leak test against `httptest.NewServer` with `runtime.NumGoroutine()` flat ±5 — Phase 3
- ✓ Byte-oriented contract: `wshub` does not import `internal/registry` or `internal/httpapi` (ABBA deadlock structurally impossible) — Phase 3
- ✓ Logging contract: Warn on slow-consumer drop (no PII), Info on hub close, silent on fan-out — Phase 3

### Active

<!-- Current scope. Building toward these. -->

**Wiring, Shutdown & Polish (Phase 5, shipped 2026-04-10)**
- ✓ `cmd/server/main.go` compose-root at 99 non-blank-non-comment lines, wiring config → logger → store → credentials → hub → httpapi → http.Server — Phase 5
- ✓ Signal-aware graceful shutdown via `signal.NotifyContext(SIGINT, SIGTERM)` — Phase 5
- ✓ Two-phase shutdown: `httpSrv.Shutdown` drains HTTP, then `hub.Close()` sends `StatusGoingAway` to WS subscribers (PITFALLS #6) — Phase 5
- ✓ Optional TLS via `ListenAndServeTLS` when `server.tls.enabled = true` — Phase 5
- ✓ `TestGracefulShutdown` binds a real listener, cancels ctx, asserts clean exit within 20 s — Phase 5
- ✓ Whole-module `go test ./... -race` green across all 5 packages (119 s) — Phase 5
- ✓ `README.md` with Quickstart, Configuration, API Reference, Development, Architecture, Known Limitations — Phase 5

### Milestone v1.0 — COMPLETE (2026-04-10)

64/64 requirements shipped and tested. Audit: `.planning/v1.0-MILESTONE-AUDIT.md` — passed. Binary ready for public demo.

### Active

*(Out of scope for v1.0 — tracked in REQUIREMENTS.md §v2 Requirements)*

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
| Symmetric `*/*` MIME wildcard matching (both sides can wildcard) | Required by CAP-05 so capability providers and requesters can each express openness | ✓ Good — `mimeMatch` + 3×3 matrix test landed in Phase 2 |
| Persist failure → in-memory rollback (error contains `"registry unchanged"`) | Prevents divergence between disk and memory state under disk-full / permission errors | ✓ Good — `TestStore_Upsert_PersistFailureRollsBack` landed in Phase 2 |
| `NewStore` does NOT mkdir missing parent directory | Operator error (bad config) should surface on first write, not be silently papered over | ✓ Good — Phase 2, Open Question 4 |
| Deleting a non-existent id is `(false, nil)` no-op with no disk write | Idempotent DELETE semantics; avoids spurious persist churn on retries | ✓ Good — Phase 2, Open Question 5 |
| `capability.path` accepts absolute http(s) URLs in addition to `/`-relative paths | v1.0 shipped assuming every provider serves its capability endpoints on the same host as its manifest URL; the TechSprint-01 FilePicker registry revealed real providers split these across hosts. Widening the accepted input is strictly backward-compatible. | ✓ Good — hotfix 2026-04-11 (post-v1.0) |

---
## Current State

**Milestone v1.0 is COMPLETE** — 64/64 requirements shipped, 5/5 phases verified, full module race-clean. The server is feature-complete and audited. Next step is public demo deployment via ngrok, then planning milestone v2 (v2 requirements already scoped in REQUIREMENTS.md — pluggable storage, hot-reload credentials, metrics, OAuth/OIDC, event coalescing).

---
*Last updated: 2026-04-10 — milestone v1.0 complete*
