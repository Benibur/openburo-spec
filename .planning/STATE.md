---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
status: unknown
stopped_at: Completed 03-02-publish-close-PLAN.md
last_updated: "2026-04-10T11:27:18Z"
progress:
  total_phases: 5
  completed_phases: 2
  total_plans: 9
  completed_plans: 8
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-09)

**Core value:** A client app can discover, at any moment, which other apps can fulfill a given intent, and be notified instantly when that set changes.
**Current focus:** Phase 03 — websocket-hub

## Current Position

Phase: 03 (websocket-hub) — EXECUTING
Plan: 3 of 3

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
| Phase 02-registry-core P02 | 3min | 2 tasks (TDD RED/GREEN) | 9 files (2 prod, 2 test, 5 fixtures) |
| Phase 02-registry-core P03 | 2min | 1 (TDD RED/GREEN) tasks | 2 files files |
| Phase 03-websocket-hub P01 | 5min | 3 tasks | 5 files |
| Phase 03-websocket-hub P02 | 8min | 2 tasks (TDD RED/GREEN) | 4 files (1 created, 3 modified) |

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
- [Phase 02-registry-core P02]: Locked Open Question 4 — NewStore does NOT mkdir a missing parent; the first Upsert against a path with a non-existent parent directory fails in CreateTemp, surfacing the operator error at mutation time rather than silently creating directories
- [Phase 02-registry-core P02]: Locked Open Question 5 — Delete of non-existent id is a (false, nil) no-op with NO disk write, verified by os.Stat().ModTime() assertion in TestStore_Delete_NonExistent_NoOp
- [Phase 02-registry-core P02]: Rollback error phrase frozen as observable contract — error.Error() MUST contain "registry unchanged" when in-memory state is consistent with disk state after a persist failure; tests assert require.Contains on this exact substring so future refactors cannot drop the contract
- [Phase 02-registry-core P02]: persistLocked step order — CreateTemp-in-same-dir -> Encode(SetIndent 2 spaces) -> Sync (contents) -> Close -> Rename -> dir fsync (best-effort); temp file Remove deferred unconditionally so failed writes never leak .tmp-* files
- [Phase 02-registry-core P02]: Plan 02-02 concurrency test uses List/Get readers only (Capabilities doesn't exist yet) — Plan 02-03 will add the Capabilities concurrency test using the same RWMutex so correctness transfers
- [Phase 02-registry-core]: Plan 02-03: Open Question 3 LOCKED — malformed filter.MimeType returns empty slice (not error); Phase 4 pre-validates via CanonicalizeMIME for 400 response
- [Phase 02-registry-core]: Plan 02-03: Single canonicalization outside loop — filter.MimeType canonicalized once before manifest iteration; capability-side mimeTypes already canonical from Validate so mimeMatch compares two canonical inputs per call
- [Phase 03-websocket-hub]: Plan 03-01: Task 0 deferred go mod tidy until test file exists; coder/websocket v1.8.14 flips from indirect to direct after Task 2
- [Phase 03-websocket-hub]: Plan 03-01: Hub comment reworded from 'slog.Default()' to 'global default logger' so literal substring does not trip the grep gate; semantic meaning preserved
- [Phase 03-websocket-hub]: Plan 03-01: Publish/Close shipped as TODO(03-02) stubs; ping ticker wired with empty case body so 03-02 only fills the ping case, not the select shape
- [Phase 03-websocket-hub]: Plan 03-01: The three PITFALLS #3 research flags (conn.CloseRead(ctx), defer h.removeSubscriber(s), defer conn.CloseNow()) land as code in the first commit of Phase 3 and are guarded by the 1000-cycle TestSubscribe_NoGoroutineLeak that passes in 0.6s under -race
- [Phase 03-websocket-hub]: Plan 03-02: Publish uses non-blocking `select { case s.msgs <- msg: default: Warn + go s.closeSlow() }` under h.mu; the `go` keyword on closeSlow is load-bearing because conn.Close has a 5s+5s handshake budget that must NOT run under the publisher's mutex
- [Phase 03-websocket-hub]: Plan 03-02: Hub.Close is idempotent via h.closed flag, logs Info once ("wshub: closing hub" with subscribers count), then iterates firing `go s.closeGoingAway()` off-mutex; Close does NOT clear h.subscribers (writer loops self-cleanup via defer h.removeSubscriber)
- [Phase 03-websocket-hub]: Plan 03-02: Publish-after-Close is a silent no-op so Phase 5's two-phase shutdown can race with in-flight HTTP handlers without spurious Warn spam
- [Phase 03-websocket-hub]: Plan 03-02: Slow-consumer and Close-GoingAway test timeouts raised from 1s to 7s to accommodate coder/websocket v1.8.14's hardcoded 5s waitCloseHandshake timeout that fires when the peer never reads (the slow-consumer simulation). This is a structural library property, not a bug — production code is byte-for-byte per plan
- [Phase 03-websocket-hub]: Plan 03-02: TestSubscribe_PingKeepsAlive uses require.Never over 300ms (30+ ping cycles at 10ms PingInterval) as a positive-by-negative oracle — if pings silently break, the writer loop errors out and h.subscribers shrinks to 0

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

Last session: 2026-04-10T11:27:18Z
Stopped at: Completed 03-02-publish-close-PLAN.md
Resume file: None
