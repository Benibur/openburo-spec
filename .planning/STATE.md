---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
status: unknown
stopped_at: Completed 02-01-manifest-mime-PLAN.md
last_updated: "2026-04-10T09:49:42.962Z"
progress:
  total_phases: 5
  completed_phases: 1
  total_plans: 6
  completed_plans: 4
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-09)

**Core value:** A client app can discover, at any moment, which other apps can fulfill a given intent, and be notified instantly when that set changes.
**Current focus:** Phase 02 — registry-core

## Current Position

Phase: 02 (registry-core) — EXECUTING
Plan: 2 of 3 (02-01-manifest-mime complete)

## Performance Metrics

**Velocity:**

- Total plans completed: 0
- Average duration: —
- Total execution time: 0.0 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 1. Foundation | 0 | — | — |
| 2. Registry Core | 0 | — | — |
| 3. WebSocket Hub | 0 | — | — |
| 4. HTTP API | 0 | — | — |
| 5. Wiring, Shutdown & Polish | 0 | — | — |

**Recent Trend:**

- Last 5 plans: none
- Trend: —

*Updated after each plan completion*
| Phase 01-foundation P01 | 8min | 2 tasks | 12 files |
| Phase 01-foundation P02 | 12min | 2 tasks | 12 files |
| Phase 01-foundation P03 | 3min | 2 tasks | 4 files |
| Phase 02-registry-core P01 | 15min | 2 tasks (TDD RED/GREEN) | 5 files (1 deleted) |

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- Reference-implementation framing — clarity beats completeness; every feature must be essential to the broker pattern or illustrate a protocol decision
- Five-dependency stdlib-first stack — coder/websocket, go.yaml.in/yaml/v3, golang.org/x/crypto/bcrypt, rs/cors, testify/require
- Four-package layout with unidirectional dependency graph — registry never imports wshub; httpapi is the sole wiring point
- Phase 2 and Phase 3 are parallel-safe (disjoint dependency graphs)
- [Phase 01-foundation]: Go 1.26.2 toolchain installed to $HOME/sdk/go1.26.2 (not system-wide) because Go 1.22 couldn't auto-fetch 1.26+ toolchains
- [Phase 01-foundation]: Replaced plan's .gitkeep files with package-anchor stubs (internal/config/doc.go blank-imports yaml/v3; internal/httpapi/doc_test.go imports testify/require) so go mod tidy retains pinned direct deps
- [Phase 01-foundation]: Deleted internal/config/doc.go anchor (Plan 01-01 blank-import) once config.go imports yaml/v3 directly
- [Phase 01-foundation]: Followed RESEARCH Config struct skeleton verbatim; validate() fails fast with field-named errors (no silent defaults for logging.format/level)
- [Phase 01-foundation]: newLogger lives inline in cmd/server/main.go (not in an internal/logging package) to make it physically impossible for any internal/ package to grab a global logger; injection-first slog is enforced by a cross-tree grep gate
- [Phase 01-foundation]: Startup banner contract frozen: 10 keys (version, go_version, listen_addr, tls_enabled, config_file, credentials_file, registry_file, ping_interval, log_format, log_level) in locked order; reordering requires CONTEXT.md update
- [Phase 01-foundation]: handleHealth deliberately does not log and does not touch r.Header — locks in the never-log-health convention that Phase 4 middleware will inherit (PITFALLS #13 credential leak prevention)
- [Phase 01-foundation]: Go 1.22 method-prefixed ServeMux patterns (mux.HandleFunc("GET /health", ...)) — the METHOD prefix is load-bearing for automatic 405s on wrong methods
- [Phase 02-registry-core P01]: Locked Open Question 1 — sort.Strings MimeTypes at end of Validate so file representation is byte-stable across re-upserts
- [Phase 02-registry-core P01]: Locked Open Question 2 — canonicalizer is lenient with trailing semicolons (text/plain; -> text/plain)
- [Phase 02-registry-core P01]: Fixed two RESEARCH canonicalizer bugs: (1) strings.SplitN accepts image//png and image/png/extra — rejected via strings.Contains(parts[1], "/"); (2) strings.SplitN accepts */subtype — rejected explicitly when parts[0]=="*" && parts[1]!="*"
- [Phase 02-registry-core P01]: Deleted internal/registry/doc.go — package doc moved into manifest.go (the face of the domain carries its own documentation; repeats the Phase 1 pattern of deleting doc.go stubs once the real code arrives)
- [Phase 02-registry-core P01]: Manifest.Validate mutates receiver in place (canonicalizes MimeTypes, sorts alphabetically) so stored manifests carry already-canonical MIME strings and mimeMatch stays a pure comparison with no re-canonicalization cost per query
- [Phase 02-registry-core P01]: Exported CanonicalizeMIME wrapper lands in this plan (not Phase 4) so Phase 4 has no registry-internal plumbing to worry about and Open Question 3 (malformed filter MIME -> empty result) can be implemented cleanly in Plan 02-03

### Critical Research Flags (must land in first commit of their phase)

- **Phase 2:** Symmetric 3×3 wildcard MIME matching with exhaustive test (PITFALLS #2); atomic persistence + in-memory rollback (PITFALLS #5)
- **Phase 3:** `conn.CloseRead(ctx)` + `defer removeSubscriber` (PITFALLS #3); non-blocking drop-slow-consumer fan-out (PITFALLS #4)
- **Phase 4:** Timing-safe Basic Auth (PITFALLS #8); OriginPatterns from shared config, no InsecureSkipVerify (PITFALLS #7); mutation-then-broadcast in handler layer (PITFALLS #1)
- **Phase 5:** Two-phase graceful shutdown (PITFALLS #6)

### Pending Todos

None yet.

### Blockers/Concerns

None yet.

## Session Continuity

Last session: 2026-04-10T09:59:00Z
Stopped at: Completed 02-01-manifest-mime-PLAN.md
Resume file: .planning/phases/02-registry-core/02-02-store-persist-PLAN.md
