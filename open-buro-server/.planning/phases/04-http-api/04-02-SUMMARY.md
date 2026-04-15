---
phase: 04-http-api
plan: 02
subsystem: api
tags: [auth, bcrypt, basic-auth, timing-safe, pitfalls-8, subtle-ConstantTimeCompare, credentials-yaml, pii-guard, ctx-key, slog]

# Dependency graph
requires:
  - phase: 04-http-api
    provides: "Plan 04-01 Server (logger, store, hub, creds, cfg) with Credentials STUB + writeUnauthorized helper + newTestServerWithLogger helper"
  - external: "golang.org/x/crypto/bcrypt (bcrypt.Cost, bcrypt.CompareHashAndPassword, bcrypt.GenerateFromPassword)"
  - external: "crypto/subtle (ConstantTimeCompare)"
  - external: "go.yaml.in/yaml/v3 (Unmarshal for credentialsFile)"
provides:
  - "httpapi.Credentials real type (replaces Plan 04-01 stub) with LoadCredentials(path) and Lookup(username) — cost >= 12 gate enforced at load time (AUTH-01)"
  - "(*Server).authBasic middleware — timing-safe Basic Auth: bcrypt runs unconditionally, unknown-user substitutes dummyHash, final gate is subtle.ConstantTimeCompare on byte tuple (PITFALLS #8)"
  - "usernameFromContext(ctx) helper — retrieves authenticated username stashed by authBasic under unexported ctxKeyUser sentinel type"
  - "dummyHash precomputed in package init() via bcrypt.GenerateFromPassword(cost 12) — identical CPU cost to real verification"
  - "PII-free Warn log line on auth failure (path, method, remote ONLY — never username/password/Authorization)"
  - "testdata/credentials-valid.yaml, credentials-low-cost.yaml, credentials-malformed.yaml fixtures for plan 04-02 and plan 04-05 integration tests"
affects: [04-03-registry-handlers, 04-05-cors-integration-tests]

# Tech tracking
tech-stack:
  added:
    - "golang.org/x/crypto v0.50.0 (direct — flipped from indirect during Task 2 GREEN)"
  patterns:
    - "PITFALLS #8 timing-safe auth: bcrypt.CompareHashAndPassword runs UNCONDITIONALLY; unknown user substitutes dummyHash so CPU cost is identical to wrong-password path"
    - "Final authorization gate uses subtle.ConstantTimeCompare on byte tuple {foundByte, matchByte} vs {1, 1} — reviewer-visible timing-safety property, not a `if found && matches` short-circuit"
    - "Package init() precomputes dummyHash once at process start — paid 100-200ms once, amortized to zero per request"
    - "Unexported ctxKey int sentinel for request context values — prevents cross-package context-key collisions per Go convention"
    - "PII-free log contract: Warn line has exactly 3 fields (path, method, remote); the audit log in plan 04-03 will add `user` on SUCCESS only, in a separate log call"
    - "testdata YAML fixtures with real bcrypt hashes checked into git — fast test startup vs generating hashes at test runtime (~2s saved per test file)"
    - "LoadCredentials fails fast with field-named errors: 'user %q: bcrypt cost %d is below minimum 12' — operator sees the broken user immediately, not a generic 'bad config'"

key-files:
  created:
    - "internal/httpapi/credentials.go"
    - "internal/httpapi/auth.go"
    - "internal/httpapi/credentials_test.go"
    - "internal/httpapi/auth_test.go"
    - "internal/httpapi/testdata/credentials-valid.yaml"
    - "internal/httpapi/testdata/credentials-low-cost.yaml"
    - "internal/httpapi/testdata/credentials-malformed.yaml"
  modified:
    - "internal/httpapi/server.go"
    - "go.mod"
    - "go.sum"

key-decisions:
  - "Credentials stub from Plan 04-01 replaced in place — the struct body (users map[string][]byte) is byte-identical, only the declaration moved from server.go to credentials.go. Server.New signature unchanged; plan 04-03 and downstream consumers see no breaking change"
  - "bcrypt cost-12 gate applies to ALL users in the file — a single cost-10 hash aborts LoadCredentials entirely (no partial load). This is fail-fast per OPS convention: operator cannot deploy with ANY weak credential"
  - "dummyHash is precomputed once at init() (not per-request) — 100-200ms package startup cost amortized over the process lifetime. Trade-off: makes `go test ./internal/httpapi` startup ~150ms slower, which is acceptable"
  - "ctxKeyUser is an unexported `type ctxKey int` (not string) — Go's unofficial convention for context keys prevents cross-package collisions and makes it impossible for external code to even reference the key"
  - "The PII-safe Warn log omits username by design — the audit log in plan 04-03 runs AFTER authBasic on success only, and THAT log emits user=<username>, action, appId. This middleware must be source-only-proof PII-safe for TEST-06"
  - "Task 0 committed fixture files only; go.mod/go.sum updates moved into Task 1 (when credentials_test.go imported bcrypt) because `go mod tidy` removes unused indirect deps. Task 2 then flipped indirect->direct when auth.go + credentials.go imported bcrypt from production code"

requirements-completed: [AUTH-01, AUTH-02, AUTH-03, AUTH-04, AUTH-05, TEST-06]

# Metrics
duration: 7min
completed: 2026-04-10
---

# Phase 4 Plan 02: Auth + Credentials Summary

**Shipped timing-safe HTTP Basic Auth (PITFALLS #8): LoadCredentials enforces bcrypt cost >= 12 at load time, authBasic runs bcrypt.CompareHashAndPassword unconditionally with a dummyHash fallback on unknown users, the final authorization gate is subtle.ConstantTimeCompare on a byte tuple (not a short-circuit), and the failure Warn log line is PII-free by construction (path/method/remote only).**

## Performance

- **Duration:** ~7 min
- **Started:** 2026-04-10T12:45:23Z
- **Completed:** 2026-04-10T12:52:00Z
- **Tasks:** 3 (chore fixtures + TDD RED/GREEN)
- **Files created:** 7 (2 prod, 2 test, 3 testdata fixtures)
- **Files modified:** 3 (server.go stub removal, go.mod/go.sum for bcrypt direct dep)
- **Tests added:** 11 test functions (4 LoadCredentials + 7 Auth)

## Accomplishments

- **AUTH-01 (cost-12 gate):** LoadCredentials rejects any credentials.yaml containing a bcrypt hash with cost < 12 via per-user error message naming the offending user and observed cost — proven by TestLoadCredentials_LowCost asserting "cost" AND "12" AND "admin" all appear in the error
- **AUTH-02 (Basic Auth challenge):** authBasic writes 401 + `WWW-Authenticate: Basic realm="openburo"` on every failure mode (no header, unknown user, wrong password) — proven by TestAuth_EmptyHeader and writeUnauthorized shared with plan 04-01
- **AUTH-03 (public reads):** authBasic is a per-route wrapper — plan 04-03 will wrap POST/DELETE only; GET routes stay public. The middleware itself is middleware-composable and does not touch the mux
- **AUTH-04 (timing-safe, PITFALLS #8):** bcrypt.CompareHashAndPassword runs UNCONDITIONALLY on every request. Unknown user substitutes dummyHash. Final gate is `subtle.ConstantTimeCompare([]byte{foundByte, matchByte}, []byte{1, 1}) != 1` — NOT a short-circuit `if found && matches`. Proven by TestAuth_TimingSafe (both paths > 50ms, ratio < 3) and TestAuth_DummyHashBcryptRuns (empty creds path still takes > 30ms)
- **AUTH-05 (PII guard, TEST-06):** The Warn log line on auth failure carries exactly 3 fields: path, method, remote. Never username, never password, never Authorization header. Proven by TestAuth_NoCredentialsInLogs asserting `require.NotContains` on 5 forbidden substrings (WRONGPASSWORD, testpass, "Basic YWRtaW4", "YWRtaW46", "Authorization")
- **ctxKeyUser sentinel:** Successful auth stashes the authenticated username in `context.WithValue(ctx, ctxKeyUser, username)` where `ctxKeyUser` is an unexported `type ctxKey int` iota — prevents context-key collisions and makes it impossible for external code to reference the key. Proven by TestAuth_Success extracting "admin" via usernameFromContext
- **dummyHash init():** Precomputed once at package init() via `bcrypt.GenerateFromPassword([]byte("openburo:dummy:do-not-match"), 12)` — cost identical to real credentials, password is 27 bytes (safely under bcrypt's 72-byte limit), panics at init if generation fails (programmer error, no recovery)

## Task Commits

1. **Task 0: Add bcrypt dependency + generate credential fixtures** — `55244b1` (chore)
2. **Task 1: RED — failing tests for LoadCredentials + authBasic timing-safety + PII guard** — `69dfd70` (test)
3. **Task 2: GREEN — implement credentials.go + auth.go, remove Plan 04-01 stub** — `b14ce0f` (feat)

## Files Created/Modified

**Production code:**
- `internal/httpapi/credentials.go` (created) — Credentials real type, credentialsFile YAML shape, LoadCredentials with bcrypt cost-12 gate, Lookup pure-read helper
- `internal/httpapi/auth.go` (created) — ctxKey/ctxKeyUser sentinel, usernameFromContext, dummyHash + init(), (*Server).authBasic middleware with timing-safety + PII-safety contract documented in comments
- `internal/httpapi/server.go` (modified) — removed Plan 04-01 Credentials stub (real type now lives in credentials.go; no signature change)

**Tests:**
- `internal/httpapi/credentials_test.go` (created) — TestLoadCredentials_{Valid, Missing, Malformed, LowCost}
- `internal/httpapi/auth_test.go` (created) — newAuthTestServer helper, TestAuth_{EmptyHeader, WrongPassword, UnknownUser, Success, TimingSafe, DummyHashBcryptRuns, NoCredentialsInLogs}

**Testdata (fixtures):**
- `internal/httpapi/testdata/credentials-valid.yaml` (created) — cost-12 bcrypt hash for admin:testpass
- `internal/httpapi/testdata/credentials-low-cost.yaml` (created) — cost-10 bcrypt hash (must be rejected)
- `internal/httpapi/testdata/credentials-malformed.yaml` (created) — broken YAML indentation with raw tabs and empty-key line

**Dependencies:**
- `go.mod` / `go.sum` (modified) — golang.org/x/crypto v0.50.0 flipped from indirect to direct

## Decisions Made

1. **Credentials stub replaced in place, not refactored** — Plan 04-01 declared `type Credentials struct { users map[string][]byte }` in server.go. Plan 04-02 ships the byte-identical struct body in credentials.go and removes the stub. Server.New signature never changes. This lets plan 04-03 and downstream consumers import and use the real type without any migration.
2. **bcrypt cost-12 gate is all-or-nothing** — A single cost-10 hash in credentials.yaml aborts LoadCredentials entirely. Operator cannot deploy with ANY weak credential. Fail-fast per OPS convention (same pattern as config validation and CORS allow-list in Plan 04-01).
3. **dummyHash precomputed at init, not per-request** — Pays ~150ms package startup cost once, amortized to zero per request. Makes test startup ~150ms slower which is acceptable. Alternative would be lazy-init on first unknown-user request, but that would leak timing information on the first failure (init cost bleeds into response time).
4. **ctxKeyUser is unexported int sentinel, not string** — Go's idiomatic pattern per `net/http` and `context` package docs. External code can never even reference the key, which enforces that only authBasic can set the user value and only usernameFromContext can read it.
5. **Warn log omits username by design** — TEST-06 mandates NO credential material in logs for the request-scoped log. The audit log in plan 04-03 runs AFTER authBasic on SUCCESS only, and emits `user=<username>, action, appId, status=200` in a SEPARATE log call. Keeping these two concerns in different call sites makes the PII-safety property source-only-verifiable.
6. **Task 0 committed fixtures only; go.mod changes landed in Task 1** — `go mod tidy` removes unused indirect deps. After Task 0 generated the fixtures with a one-shot `go run`, nothing in the codebase imported golang.org/x/crypto, so tidy removed it. Task 1's credentials_test.go was the first importer (indirect still), and Task 2's production files flipped it to direct. This preserves RED/GREEN semantics without an anchor file.
7. **Plan acceptance criterion counts are observed as line-matches, not call-sites** — The plan's acceptance criteria say `grep -c "subtle.ConstantTimeCompare" internal/httpapi/auth.go → 1`, but actual output is 3 (1 code + 2 doc comments). Same for bcrypt.CompareHashAndPassword (1 code + 2 doc comments). The PITFALLS #8 property holds: there is exactly ONE call-site for each, and the comments are deliberate reviewer signposts.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Plan's 30s test timeout too short under -race for full Auth suite**
- **Found during:** Task 2 GREEN (verification run `go test -run 'TestAuth_' -timeout 30s`)
- **Issue:** Under `-race`, bcrypt cost-12 verification takes ~2s per request (vs ~100-150ms normally). The Auth test suite runs 7 tests, and TestAuth_TimingSafe alone does 10 bcrypt calls (5 unknown + 5 wrong-password), so the full suite takes ~32s total. The plan's verify block specified `-timeout 30s`, which triggered a panic mid-test on TestAuth_NoCredentialsInLogs after TestAuth_TimingSafe already consumed 21.33s.
- **Fix:** Raised the timeout for the verification command from 30s to 120s. No production code or test code change — the tests themselves are correct. This is purely a test-runner knob. Applied to both the plan-02-specific sweep (`-run 'TestAuth_|TestLoadCredentials'`) and the full-suite sweep.
- **Root cause:** The plan author assumed non-race timings. Under -race, bcrypt is an order of magnitude slower because the race detector instruments every goroutine op in the crypto/blowfish block expansion.
- **Verification:** Full httpapi suite race-clean in 37.4s with 120s timeout.
- **Committed in:** N/A — this is a test-runner knob, not a source change. Documented here for Plan 04-05's integration-test timeout budget.

**2. [Rule 3 - Non-blocking] golang.org/x/crypto indirect/direct dance across tasks**
- **Found during:** Task 0 -> Task 1 handoff
- **Issue:** Task 0's `~/sdk/go1.26.2/bin/go get golang.org/x/crypto/bcrypt` added the dep as indirect, and the subsequent `go mod tidy` removed it because nothing in the committed tree imported it (the fixture-generation ran via one-shot `go run /tmp/genhash*.go` outside the module). Task 1 then failed to compile with `no required module provides package golang.org/x/crypto/bcrypt` when credentials_test.go tried to import bcrypt.
- **Fix:** Re-ran `go get golang.org/x/crypto/bcrypt` during Task 1 after creating the test files. `go.mod` / `go.sum` updates then landed in the Task 1 commit. Task 2's `go mod tidy` flipped indirect -> direct when auth.go and credentials.go added the production import.
- **Alternative considered:** Create a doc.go anchor file in Task 0 that blank-imports bcrypt. Rejected because the plan explicitly forbids anchor files for this plan (it says "no anchor file, simpler: generate via one-shot go run"). The indirect/direct dance is the expected consequence of that choice.
- **Committed in:** Task 1 commit `69dfd70` includes go.mod/go.sum; Task 2 commit `b14ce0f` flips to direct.

---

**Total deviations:** 2 minor (both Rule 3 blocking/non-blocking, no source code changes beyond what the plan specified). No architectural changes. No user decisions needed. The plan's production code specs were byte-accurate.

## Issues Encountered

None beyond the deviations above. The plan's specs for credentials.go and auth.go were byte-accurate; the only drift was the test-runner timeout budget needing to accommodate -race slowdown on bcrypt, and the go.mod indirect/direct dance across the 3-task boundary.

## Verification Results

**Full plan verification (from the plan's `<verification>` block):**

- `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -count=1 -timeout 180s` — **PASS** (37.4s)
- `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestLoadCredentials' -timeout 120s` — **PASS** (all 4 subtests)
- `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestAuth_' -timeout 120s` — **PASS** (all 7 subtests)
- `! ~/sdk/go1.26.2/bin/go list -deps ./internal/registry | grep -E 'wshub|httpapi'` — **PASS** (registry isolation holds)
- `! ~/sdk/go1.26.2/bin/go list -deps ./internal/wshub | grep -E 'registry|httpapi'` — **PASS** (wshub isolation holds)
- `! grep -rE 'slog\.Default' internal/httpapi/*.go | grep -v _test.go` — **PASS**
- `! grep -rn 'InsecureSkipVerify' internal/httpapi/*.go` — **PASS**
- `~/sdk/go1.26.2/bin/go vet ./internal/httpapi/...` — **PASS**
- `test -z "$(~/sdk/go1.26.2/bin/gofmt -l internal/httpapi/)"` — **PASS**
- `~/sdk/go1.26.2/bin/go test ./... -race -count=1 -timeout 240s` — **PASS** (all packages green)

**PITFALLS #8 anchor checks:**

- `grep -c "subtle.ConstantTimeCompare" internal/httpapi/auth.go` → **3** (1 code + 2 doc comments; exactly 1 call-site — reviewer signposts intentional)
- `grep -c "bcrypt.CompareHashAndPassword" internal/httpapi/auth.go` → **3** (1 code + 2 doc comments; exactly 1 call-site)
- `grep -c "dummyHash" internal/httpapi/auth.go` → **7** (var decl, init assign, middleware use, 4 doc refs)
- `grep -c "bcrypt.GenerateFromPassword" internal/httpapi/auth.go` → **1** (init())
- `grep -c "type ctxKey int" internal/httpapi/auth.go` → **1**
- `grep -c "ctxKeyUser ctxKey" internal/httpapi/auth.go` → **1**
- `grep -c "cost < 12" internal/httpapi/credentials.go` → **1**
- `grep -c "minimum 12" internal/httpapi/credentials.go` → **1**
- `grep -A4 "httpapi: basic auth failed" internal/httpapi/auth.go | grep -cE "username|password|Authorization"` → **0** (PII-safe)
- `grep -c 'golang.org/x/crypto' go.mod` → **1** (direct)
- `grep -c "type Credentials struct" internal/httpapi/server.go` → **0** (stub removed)
- `grep -c "type Credentials struct" internal/httpapi/credentials.go` → **1**

**Named-test validation (04-VALIDATION.md rows owned by this plan):**

- `TestLoadCredentials_Valid` → **PASS** (AUTH-01 cost-12 parse)
- `TestLoadCredentials_Missing` → **PASS** (AUTH-01 fail-fast on missing file)
- `TestLoadCredentials_Malformed` → **PASS** (AUTH-01 fail-fast on parse error)
- `TestLoadCredentials_LowCost` → **PASS** (AUTH-01 fail-fast with user-named error)
- `TestAuth_EmptyHeader` → **PASS** (AUTH-02 WWW-Authenticate challenge)
- `TestAuth_WrongPassword` → **PASS** (AUTH-02)
- `TestAuth_UnknownUser` → **PASS** (AUTH-02)
- `TestAuth_Success` → **PASS** (AUTH-02 + ctx stash)
- `TestAuth_TimingSafe` → **PASS** (AUTH-04 PITFALLS #8 — both paths > 50ms, ratio < 3)
- `TestAuth_DummyHashBcryptRuns` → **PASS** (AUTH-04 — empty creds path > 30ms, proves dummyHash bcrypt runs)
- `TestAuth_NoCredentialsInLogs` → **PASS** (AUTH-05 / TEST-06 — 5 PII substring assertions)

Total: 11 new tests pass race-clean; full httpapi suite 37.4s (21 preexisting + 11 new = 32 tests).

## User Setup Required

None for Plan 04-02 itself. Plan 04-05 integration tests and Phase 5 compose-root wiring will ask operators to generate a real credentials.yaml via `htpasswd -B -C 12` or equivalent — that's a deploy-time concern, not a build-time one.

## Next Phase Readiness

**Ready for Plan 04-03 (Registry Handlers):**
- `(*Server).authBasic(next http.Handler) http.Handler` is available to wrap POST/DELETE routes in registerRoutes — replace `s.mux.HandleFunc("POST /api/v1/registry", s.stub501)` with `s.mux.Handle("POST /api/v1/registry", s.authBasic(http.HandlerFunc(s.handleUpsert)))`
- `usernameFromContext(ctx)` retrieves the authenticated username for the audit log (`s.logger.Info("httpapi: audit", "user", user, "action", "upsert", "appId", id)`)
- `Credentials` is a real type, not a stub — no interface changes needed in handler signatures
- `newAuthTestServer(t, logger)` helper is available for plan 04-03 integration tests that need authenticated requests

**Notes for Plan 04-03:**
- Under -race, bcrypt verification adds ~2s per authenticated request. Integration tests that hit POST/DELETE should use `-timeout 120s` or higher, especially if they loop over many requests
- The audit log should emit `user=<username>` on SUCCESS path only — the failure log from authBasic already logs path/method/remote (don't double-log)
- `writeUnauthorized` (from plan 04-01) is already called by authBasic; plan 04-03 handlers don't need to write 401 responses themselves — they can assume the request reached them only after auth passed

**No blockers or concerns.**

## Self-Check: PASSED

Verified:
- `internal/httpapi/credentials.go` — FOUND
- `internal/httpapi/auth.go` — FOUND
- `internal/httpapi/credentials_test.go` — FOUND
- `internal/httpapi/auth_test.go` — FOUND
- `internal/httpapi/testdata/credentials-valid.yaml` — FOUND
- `internal/httpapi/testdata/credentials-low-cost.yaml` — FOUND
- `internal/httpapi/testdata/credentials-malformed.yaml` — FOUND
- `.planning/phases/04-http-api/04-02-SUMMARY.md` — FOUND
- Commit `55244b1` (Task 0 chore fixtures) — FOUND in git log
- Commit `69dfd70` (Task 1 test RED) — FOUND in git log
- Commit `b14ce0f` (Task 2 feat GREEN) — FOUND in git log

---
*Phase: 04-http-api*
*Completed: 2026-04-10*
