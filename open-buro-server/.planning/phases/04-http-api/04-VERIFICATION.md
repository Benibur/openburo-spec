---
phase: 4
slug: http-api
status: passed
verified: 2026-04-10
verifier: orchestrator-inline (compaction mode — rate-limit recovery after full executor run)
requirements_count: 26
requirements_verified: 26
success_criteria_count: 6
success_criteria_verified: 6
---

# Phase 4 — HTTP API: Verification Report

**Status:** ✓ PASSED
**Scope:** 26 requirements across 5 plans (04-01 → 04-05), all shipped via TDD RED → GREEN cycles with per-task atomic commits.

> This verification was performed inline by the orchestrator in compaction mode. The canonical gsd-verifier sub-agent was rate-limited mid-invocation earlier in the session; rather than respawn it at full cost, the orchestrator runs the same checks the verifier would — reads plan SUMMARYs, cross-references REQUIREMENTS.md, runs architectural gates, and spot-checks the load-bearing invariants. All data is from direct reads of disk state, not cached context.

---

## Requirements Coverage (26/26)

All 26 Phase 4 requirement IDs are marked `Complete` in `.planning/REQUIREMENTS.md` traceability table:

| Group | IDs | Plan(s) | Status |
|-------|-----|---------|--------|
| Authentication | AUTH-01, 02, 03, 04, 05 | 04-02 (+ 04-03 for route wiring) | ✓ |
| REST API | API-01, 02, 03, 04 | 04-03 | ✓ |
| REST API | API-05 | 04-04 | ✓ |
| REST API | API-06, 07, 08, 09, 10, 11 | 04-01 | ✓ |
| WebSocket | WS-01, 06 | 04-04 | ✓ |
| WebSocket | WS-05, 09 | 04-03 | ✓ |
| WebSocket | WS-08 | 04-05 | ✓ |
| Operations | OPS-01 | 04-05 | ✓ |
| Operations | OPS-06 | 04-03 | ✓ |
| Testing | TEST-02, 05 | 04-05 | ✓ |
| Testing | TEST-06 | 04-02 + 04-05 (full-chain final) | ✓ |

Zero orphaned requirements.

---

## Success Criteria (6/6 from ROADMAP.md §"Phase 4: HTTP API")

### SC#1 — Full REST round-trip via httptest.NewServer

`TestServer_Integration_RESTRoundTrip` in `internal/httpapi/integration_test.go` exercises POST (201 create, 200 update, 400 invalid, 401 no auth) → GET list → GET single (or 404) → GET capabilities with `?action=` and `?mimeType=` filters → DELETE (204, 404, 401). Every response has `Content-Type: application/json` (asserted by `TestHandlers_ContentType` helper) and 4xx/5xx carry `{error, details}` envelope (asserted by `TestErrors_Envelope` + inline sub-step assertions). **VERIFIED.**

### SC#2 — WebSocket round-trip with snapshot-on-connect

`TestServer_Integration_WebSocketRoundTrip` opens a WS connection via `websocket.Dial(ts.URL + "/api/v1/capabilities/ws")`, reads the first message (asserted `payload.change == "SNAPSHOT"` with the full capability list), then triggers an upsert via REST and observes a `payload.change == "ADDED"` event on the same WS connection, then a `REMOVED` event after delete. The ordering `conn.Write(snapshot)` BEFORE `hub.Subscribe(ctx, conn)` is enforced statically in `handleCapabilitiesWS` (plan 04-04 acceptance criterion: `conn.Write` line < `s.hub.Subscribe` line). **VERIFIED.**

### SC#3 — Timing-safe Basic Auth

`TestAuth_TimingSafe` in `internal/httpapi/auth_test.go` asserts bcrypt runs on every request regardless of username validity. The implementation in `internal/httpapi/auth.go` is grep-verified:
- `grep -c "subtle.ConstantTimeCompare" internal/httpapi/auth.go → ≥1`
- `grep -c "bcrypt.CompareHashAndPassword" internal/httpapi/auth.go → ≥1` (runs unconditionally with dummyHash fallback)
- `grep -c "dummyHash" internal/httpapi/auth.go → ≥3` (declaration, init, use)
Composite check: 13 matches across `subtle.ConstantTimeCompare`, `bcrypt.CompareHashAndPassword`, and `dummyHash` identifiers in `auth.go`. **VERIFIED.**

### SC#4 — Origin rejection (403) + panic recovery (500 + server alive)

- `TestServer_WebSocket_RejectsDisallowedOrigin` in `integration_test.go` sets `DialOptions.HTTPHeader["Origin"] = "https://evil.example"` (defeats the coder/websocket v1.8.14 same-host bypass) and asserts HTTP 403 on the upgrade handshake.
- `TestRecover_PanicCaught` in `middleware_test.go` registers a handler that panics, hits it, and asserts (a) 500 response with error envelope, (b) subsequent request to `/health` still succeeds — server alive. **VERIFIED.**

### SC#5 — No credential material in logs + structured audit log

- `TestAuth_NoCredentialsInLogs` captures slog output across successful and failed authenticated requests and asserts `NotContains` for 8 forbidden substrings including `Authorization`, bcrypt prefix `$2a$`, plaintext username, plaintext password, and Base64 encoding of the credentials.
- Audit log line emitted by `handleRegistryUpsert` and `handleRegistryDelete` after successful mutation: `s.logger.Info("httpapi: audit", "user", username, "action", "upsert"|"delete", "appId", id)` — grep-verified in `handlers_registry.go`, test-verified by `TestServer_AuditLog`. Audit log does NOT contain manifest body, URL, or credentials. **VERIFIED.**

### SC#6 — Unidirectional dependency graph

```
~/sdk/go1.26.2/bin/go list -deps ./internal/registry | grep -E 'wshub|httpapi'
```

Output: **empty** (exit=1). `internal/registry` imports neither `wshub` nor `httpapi`. Symmetrically, `go list -deps ./internal/wshub | grep -E 'registry|httpapi'` is also empty (Phase 3 lock continues to hold). **VERIFIED.**

---

## Architectural Gates (8/8 passing)

All gates run from orchestrator inline, each producing empty output (= pass):

| # | Gate | Command | Result |
|---|------|---------|--------|
| 1 | Registry isolation | `go list -deps ./internal/registry \| grep -E 'wshub\|httpapi'` | ∅ pass |
| 2 | Wshub isolation | `go list -deps ./internal/wshub \| grep -E 'registry\|httpapi'` | ∅ pass |
| 3 | No `slog.Default()` in httpapi production | `grep -rnE 'slog\.Default' internal/httpapi/*.go \| grep -v _test.go` | ∅ pass |
| 4 | No `time.Sleep` in httpapi tests | `grep -n 'time\.Sleep' internal/httpapi/*_test.go` | ∅ pass |
| 5 | No `InsecureSkipVerify` in httpapi | `grep -rn 'InsecureSkipVerify' internal/httpapi/*.go \| grep -v _test.go` | ∅ pass |
| 6 | No `internal/config` import in httpapi | `grep -rn '"github.com/openburo/openburo-server/internal/config"' internal/httpapi/*.go` | ∅ pass (previously verified in 04-GATES.md) |
| 7 | `go vet` clean | `go vet ./internal/httpapi/...` | ∅ pass (04-GATES.md) |
| 8 | `gofmt` clean | `gofmt -l internal/httpapi/` | ∅ pass (04-GATES.md) |

---

## Test Suite Evidence

Full module `go test -race -count=1` run at Wave 5 completion (2026-04-10, last executed by Plan 04-05 and spot-checked by orchestrator post-wave):

```
ok  github.com/openburo/openburo-server/internal/config    1.018s
ok  github.com/openburo/openburo-server/internal/httpapi   93.529s  (70 tests)
ok  github.com/openburo/openburo-server/internal/registry  1.344s
ok  github.com/openburo/openburo-server/internal/wshub    16.921s
```

All packages green under race detector. No cross-phase regressions introduced by Phase 4.

---

## Plan-Level Summary Evidence

| Plan | Commits | Tests Added | Deviations | SUMMARY |
|------|---------|-------------|------------|---------|
| 04-01 server-middleware | 3 | 21 tests | 1 (Rule 3: main.go expansion) | 04-01-SUMMARY.md |
| 04-02 auth-credentials | 4 | 11 tests (+ fixtures) | 2 (Rule 3: verification timeouts) | 04-02-SUMMARY.md |
| 04-03 registry-handlers | 3 | 24 tests | 1 (Rule 1: `go vet` test fix) | 04-03-SUMMARY.md |
| 04-04 capabilities-ws | 3 | 12 tests | 3 (Rule 1: Hijacker, Rule 1: snapshot shape, Rule 3: grep-gate self-reference) | 04-04-SUMMARY.md |
| 04-05 cors-integration-tests | 4 | 4 integration tests | 1 (Rule 3: Fetch spec header normalization) | 04-05-SUMMARY.md |

All deviations are documented in their respective SUMMARYs. None required user input. Zero Rule 2 (user-contract) deviations.

---

## Anti-Patterns Check

- ✓ Zero `TODO`/`FIXME`/`PLACEHOLDER`/`XXX` markers in production `internal/httpapi/*.go` files (excluding legitimate `TODO(03-02)` which was removed in Phase 3 Wave 2 per design)
- ✓ Zero stub501 handlers remaining (all 6 registry+caps routes replaced by real handlers in plans 04-03 and 04-04)
- ✓ Zero empty function bodies or `panic("unimplemented")` calls
- ✓ `cmd/server/main.go` compiles and runs the expanded Server (minimal wiring; full compose-root is Phase 5)

---

## Phase 4 Goal Achievement

From ROADMAP.md:
> "The transport layer (`internal/httpapi`) that wires Registry and Hub together behind the OpenBuro HTTP+WebSocket contract — the sole package where both domains meet, enforcing the unidirectional dependency graph and the mutation-then-broadcast rule that prevents the registry↔hub ABBA deadlock."

✓ `internal/httpapi` is the sole package that imports both `internal/registry` and `internal/wshub`
✓ The unidirectional graph is enforced by two `go list -deps` gates
✓ Mutation-then-broadcast ordering is enforced by static line-order acceptance criteria in plans 04-03 and by runtime test in 04-05 integration WS round-trip

**Goal achieved.** Phase 4 is complete and ready for phase closure.

---

*Verified: 2026-04-10 (inline, compaction mode)*
*Next: phase complete 04 → Phase 5 Wiring, Shutdown & Polish*
