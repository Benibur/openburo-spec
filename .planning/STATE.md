---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
status: unknown
stopped_at: Completed 04-05-cors-integration-tests-PLAN.md
last_updated: "2026-04-10T20:12:11.102Z"
progress:
  total_phases: 5
  completed_phases: 5
  total_plans: 15
  completed_plans: 15
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-09)

**Core value:** A client app can discover, at any moment, which other apps can fulfill a given intent, and be notified instantly when that set changes.
**Current focus:** Phase 05 — wiring-shutdown-polish

## Current Position

Phase: 05 (wiring-shutdown-polish) — EXECUTING
Plan: 1 of 1

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
| Phase 03-websocket-hub P03 | 5min | 2 tasks | 2 files |
| Phase 04-http-api P01 | 5min | 2 tasks | 8 files |
| Phase 04-http-api P02 | 7min | 3 tasks (chore + TDD RED/GREEN) | 10 files (2 prod, 2 test, 3 fixtures, 1 modified, 2 go.mod/sum) |
| Phase 04-http-api P03 | 7min | 2 tasks (TDD RED/GREEN) | 6 files (2 prod, 2 test, 2 modified) |
| Phase 04-http-api P04 | 8min | 2 tasks (TDD RED/GREEN) tasks | 6 files (1 prod, 1 test, 4 modified) files |
| Phase 04-http-api P05 | 8min | 3 tasks | 6 files |

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
- [Phase 03-websocket-hub]: Plan 03-03: Added syncBuffer (mutex-guarded bytes.Buffer wrapper) because the plan's verbatim `var buf bytes.Buffer` raced under -race — the subscriber writer goroutine logs Debug on exit while the test goroutine reads via buf.String() inside require.Eventually; plain bytes.Buffer is not concurrent-safe
- [Phase 03-websocket-hub]: Plan 03-03: TestHub_Logging_NoPII ctx timeout raised from 3s to 15s to accommodate the 7s slow-consumer drop (same coder/websocket v1.8.14 waitCloseHandshake structural property from 03-02)
- [Phase 03-websocket-hub]: Plan 03-03: Comment "(no time.Sleep)" reworded to "(polling, not blocking)" because the literal substring tripped the Phase 3 no-time.Sleep grep gate — third instance of Phase 3 gates tripping on their own documentation; pattern: always grep-check comments before committing gate-sensitive files
- [Phase 03-websocket-hub]: Plan 03-03: The three logging-capture tests (DropIsWarn, CloseIsInfo, NoPII) freeze the observable log contract by assertion — level=WARN, level=INFO, buffer_size field, subscribers field, exactly-one-line, 11 PII substrings forbidden — so any future refactor that changes format will fail the tests
- [Phase 03-websocket-hub]: Plan 03-03: Zero production code changes — hub.go and subscribe.go are byte-for-byte unchanged from 03-02 commit 9a27fa8; 03-03 is a test-side + docs-side lock-in only
- [Phase 03-websocket-hub]: Phase 3 gate sweep (.planning/phases/03-websocket-hub/03-GATES.md) all 8 gates PASS: full wshub suite (11 tests) + isolated leak test + arch isolation + no slog.Default + no time.Sleep + no TODO(03-02) + build/vet/gofmt + whole-module race-clean
- [Phase 04-http-api]: Plan 04-01: Server.New signature locked at (logger, store, hub, creds, cfg) (*Server, error) — Credentials declared as stub struct so Plan 04-02 replaces type body without changing signature
- [Phase 04-http-api]: Plan 04-01: Constructor validates CORS allow-list at New-time (empty, literal '*', bad path.Match pattern all return errors) because rs/cors v1.11.1 does NOT reject wildcard+credentials — fail-fast anchors PITFALLS #9
- [Phase 04-http-api]: Plan 04-01: Middleware chain composed as recover(log(cors(mux))) in Handler() — recover is OUTERMOST so it catches panics from any inner layer; corsMiddleware is Plan 04-01 pass-through placeholder that Plan 04-05 swaps in place
- [Phase 04-http-api]: Plan 04-01: All 6 Phase 4 routes registered with shared stub501 handler returning 501 envelope so downstream plans replace handler bodies (not route registrations) — the route table stays stable across 04-02..04-04
- [Phase 04-http-api]: Plan 04-01: [Rule 3 deviation] cmd/server/main.go expanded from Phase 1 single-arg New to minimal 5-arg wiring to keep whole-module build green — Phase 5 will replace with full compose-root wiring (LoadCredentials, graceful shutdown, two-phase Close)
- [Phase 04-http-api]: Plan 04-02: Credentials stub replaced in place — real type body lives in credentials.go (byte-identical struct), Server.New signature unchanged; LoadCredentials enforces bcrypt cost >= 12 all-or-nothing at load time with user-named error message
- [Phase 04-http-api]: Plan 04-02: PITFALLS #8 timing-safe Basic Auth — bcrypt.CompareHashAndPassword runs UNCONDITIONALLY on every request, unknown user substitutes precomputed cost-12 dummyHash; final gate is `subtle.ConstantTimeCompare([]byte{foundByte, matchByte}, []byte{1, 1}) != 1` (NOT a short-circuit `if found && matches`)
- [Phase 04-http-api]: Plan 04-02: dummyHash precomputed in package init() via bcrypt.GenerateFromPassword(cost 12) — ~150ms package startup cost paid once, amortized to zero per request
- [Phase 04-http-api]: Plan 04-02: Authenticated username stashed in r.Context() under unexported `type ctxKey int` sentinel (ctxKeyUser iota) — Go context-key convention, prevents cross-package collisions; usernameFromContext helper retrieves it for plan 04-03 audit log
- [Phase 04-http-api]: Plan 04-02: PII-safe Warn log line — `httpapi: basic auth failed` carries exactly {path, method, remote}; never username/password/Authorization/hash. Audit log in plan 04-03 runs AFTER authBasic on SUCCESS only in a separate call
- [Phase 04-http-api]: Plan 04-02: testdata fixtures (credentials-valid.yaml cost-12, credentials-low-cost.yaml cost-10, credentials-malformed.yaml broken YAML) checked into git — fast test startup vs generating hashes at runtime
- [Phase 04-http-api]: Plan 04-02: [Rule 3 deviation, test-runner only] Plan's `-timeout 30s` was insufficient under -race because bcrypt cost-12 takes ~2s per verification under the race detector (vs ~150ms normally); full Auth suite takes ~32s. Raised to 120s for the plan-specific sweep and 180s for the full httpapi sweep. No source code change
- [Phase 04-http-api]: Plan 04-02: golang.org/x/crypto v0.50.0 flipped from indirect to direct in Task 2 GREEN (auth.go + credentials.go import bcrypt from production code); Task 0 `go mod tidy` removed it because fixture generation ran via one-shot `go run /tmp/genhash*.go` outside the module (expected indirect/direct dance, no anchor file needed)
- [Phase 04-http-api]: Plan 04-03: Mutation-then-broadcast (PITFALLS #1) locked at the handler layer — in handleRegistryUpsert, s.store.Upsert(manifest) must return nil BEFORE s.hub.Publish(newRegistryUpdatedEvent(...)) fires; same for handleRegistryDelete. Ordering verified statically via awk+grep over the handler body (Upsert line 30 < Publish line 46; Delete line 10 < Publish line 24) AND behaviorally via TestServer_AuditLog (audit log runs after publish in source order, so observing audit implies publish fired)
- [Phase 04-http-api]: Plan 04-03: eventPayload struct shared between upsert/delete events (this plan) and snapshot events (plan 04-04) via two omitempty fields — AppID set on upsert/delete, Capabilities set on snapshot, changeSnapshot constant pre-declared here so plan 04-04 only writes newSnapshotEvent helper
- [Phase 04-http-api]: Plan 04-03: OPS-06 audit log is a SEPARATE s.logger.Info call from the request log (logMiddleware) — two log lines per authenticated write request (request log: method/path/status/duration, audit log: user/action/appId). Audit fires AFTER publish so observing the audit line is structural evidence that publish ran. PII-free by construction: TestServer_AuditLog asserts NotContains for testpass/YWRtaW46/$2a$/Basic YWRt
- [Phase 04-http-api]: Plan 04-03: API-11 body hygiene pattern — defer r.Body.Close() in every handler + http.MaxBytesReader(w, r.Body, 1<<20) on POST + json.Decoder.DisallowUnknownFields. TestHandlers_BodyClosed does 3 sequential POSTs on the same client (forces connection reuse); TestHandleRegistryUpsert_BodyTooLarge posts 2 MiB and expects 400
- [Phase 04-http-api]: Plan 04-03: 201-vs-200 on upsert is advisory, not linearizable — the existence check runs OUTSIDE the store's mutex and may observe stale state under concurrent Delete; the API-01 contract only requires the status code to be one of {201, 200} on success, not which one in the face of races
- [Phase 04-http-api]: Plan 04-03: [Rule 1 deviation] go vet caught `r, _ := ts.Client().Get(...)` in TestHandleRegistryDelete_Existing; fix was a single-line `, _ :=` -> `, err := ... require.NoError(t, err)` correction in test code. Plan's production code specs were byte-accurate — the only drift was one test line vet flagged
- [Phase 04-http-api]: Plan 04-04: Separate snapshotPayload type rather than reusing eventPayload for SNAPSHOT — Go json omitempty drops empty slices (len==0), not just nil; fix is a dedicated struct without omitempty on Capabilities so SNAPSHOT events always emit `"capabilities":[]` and never null. Two payload types share identical wire shape when populated; only zero-value serialization differs
- [Phase 04-http-api]: Plan 04-04: [Rule 1 deviation] statusCapturingWriter did not implement http.Hijacker, silently breaking every WebSocket upgrade since Plan 04-01. coder/websocket.Accept asserts w.(http.Hijacker) and writes 501 on failure. Fix: 15-line Hijack() shim forwarding to inner writer and promoting status to 101. Generalizable rule: ResponseWriter wrappers in middleware MUST forward http.Hijacker (and Flusher/Pusher) or they'll silently break downstream upgrade handlers
- [Phase 04-http-api]: Plan 04-04: Snapshot-before-subscribe (WS-06) enforced both behaviorally and statically — conn.Write(snapshot) at line 17 of handleCapabilitiesWS body precedes s.hub.Subscribe(r.Context(), conn) at line 28. WS-06 ordering is mechanically auditable via awk+grep, same pattern Plan 04-03 used for mutation-then-broadcast
- [Phase 04-http-api]: Plan 04-04: Deleted stub501 entirely; all 6 Phase 4 routes now real. `grep -c stub501 server.go → 0` is a negative invariant for Plan 04-05. The InsecureSkipVerify grep gate tripped on our own doc comments referencing the knob by name — rule: always grep-check comments before committing gate-sensitive files (4th instance across phases). Fix: reword to 'origin-skip knob'
- [Phase 04-http-api]: Plan 04-05: Real corsMiddleware shipped — cors.New(Options{AllowedOrigins: s.cfg.AllowedOrigins, AllowCredentials: true, AllowedMethods: [GET,POST,DELETE,OPTIONS], AllowedHeaders: [Authorization, Content-Type], MaxAge: 300}).Handler(next). Shared allow-list with handleCapabilitiesWS OriginPatterns — WS-08 contract fully landed (single source of truth for REST CORS and WS origin)
- [Phase 04-http-api]: Plan 04-05: rs/cors v1.11.1 Access-Control-Request-Headers parsing is STRICT — requires lowercase (per Fetch spec normalization) AND sorted lexicographically. Test initially sent 'Authorization,Content-Type' (stdlib canonical) which rs/cors silently dropped as 'headers not allowed'; fix was 'authorization,content-type' + comment. Browsers always send lowercase; Go stdlib-trained developers will hit this
- [Phase 04-http-api]: Plan 04-05: WS origin-rejection test uses 'https://evil.example' (NOT ts.URL) as Origin header to defeat coder/websocket v1.8.14's strings.EqualFold(r.Host, u.Host) same-host bypass. Using ts.URL would be a false positive — request would pass even with empty OriginPatterns. Documented in test function doc-comment
- [Phase 04-http-api]: Plan 04-05: Integration tests use httptest.NewServer(srv.Handler()), not NewRecorder. NewRecorder does not implement http.Hijacker so WS upgrades can never succeed via Recorder — the Plan 04-04 Hijacker shim bug was ONLY observable through NewServer. Convention: all WS-involving tests use NewServer
- [Phase 04-http-api]: Plan 04-05: TestAuth_NoCredentialsInLogs upgraded in place (not sibling) — Plan 04-02 tested authBasic in isolation; Plan 04-05 drives through full chain via httptest.NewServer with 8 forbidden substrings (+anotherSecret, $2a$ bcrypt prefix, nonexistent_user). Audit log line with user=admin is ALLOWED; forbidden list is strictly credential material
- [Phase 04-http-api]: Phase 4 COMPLETE: all 26 requirements closed (AUTH-01..05, API-01..11, WS-01/05/06/08/09, OPS-01/06, TEST-02/05/06), all 8 architectural gates green (registry/wshub isolation, no slog.Default, no time.Sleep, no InsecureSkipVerify, no internal/config import, go vet, gofmt). See .planning/phases/04-http-api/04-GATES.md. Full httpapi suite 93.7s race-clean with 70 tests

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

Last session: 2026-04-10T13:53:15.122Z
Stopped at: Completed 04-05-cors-integration-tests-PLAN.md
Resume file: None
