---
phase: 4
slug: http-api
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-10
---

# Phase 4 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> Derived from `04-RESEARCH.md` §Validation Architecture.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go stdlib `testing` + `github.com/stretchr/testify/require` v1.11.1 + `net/http/httptest` (stdlib) + `github.com/coder/websocket` v1.8.14 for WS round-trips |
| **Config file** | none — Go `testing` configured via `go test` flags |
| **Quick run command** | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run <TestName> -timeout 30s` |
| **Full suite command** | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -count=1 -timeout 120s` |
| **Estimated runtime** | ~10–15 seconds (REST round-trip + WS round-trip dominate) |

---

## Sampling Rate

- **After every task commit:** quick run for the plan's affected test set
- **After every plan wave:** full httpapi suite + architectural gates + grep gates
- **Before `/gsd:verify-work`:** full suite green, all 8 gates pass, race detector clean, whole-module `go test ./...` clean
- **Max feedback latency:** ~15 seconds

---

## Per-Task Verification Map

| Req ID | Behavior | Plan | Wave | Test | Automated Command |
|--------|----------|------|------|------|-------------------|
| AUTH-01 | `LoadCredentials` validates YAML, bcrypt cost ≥ 12, rejects malformed/missing | 04-02 | 2 | `TestLoadCredentials_*` | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestLoadCredentials' -timeout 10s` |
| AUTH-02 | Basic Auth protects POST/DELETE; 401 without auth | 04-02 / 04-03 | 2, 3 | `TestHandleRegistry*_Auth` | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '_Auth$' -timeout 10s` |
| AUTH-03 | Read routes + /health return 200 without auth | 04-02 / 04-03 | 2, 3 | `TestPublicRoutes` | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestPublicRoutes' -timeout 10s` |
| AUTH-04 | Timing-safe: bcrypt runs unconditionally via dummyHash fallback | 04-02 | 2 | `TestAuth_TimingSafe` | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestAuth_TimingSafe' -timeout 10s` |
| AUTH-05 | No credential material in captured slog output | 04-02 / 04-05 | 2, 5 | `TestAuth_NoCredentialsInLogs` | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestAuth_NoCredentialsInLogs' -timeout 10s` |
| API-01 | POST: 201/200/400/401 | 04-03 | 3 | `TestHandleRegistryUpsert_*` | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestHandleRegistryUpsert' -timeout 10s` |
| API-02 | DELETE: 204/404/401 | 04-03 | 3 | `TestHandleRegistryDelete_*` | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestHandleRegistryDelete' -timeout 10s` |
| API-03 | GET /registry returns `{manifests, count}` | 04-03 | 3 | `TestHandleRegistryList` | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestHandleRegistryList' -timeout 10s` |
| API-04 | GET /registry/{appId} returns one or 404 | 04-03 | 3 | `TestHandleRegistryGet` | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestHandleRegistryGet' -timeout 10s` |
| API-05 | GET /capabilities with `?action=` and `?mimeType=` filters | 04-04 | 4 | `TestHandleCapabilities` | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestHandleCapabilities$' -timeout 10s` |
| API-06 | Go 1.22 method patterns; wrong method → 405 | 04-01 | 1 | `TestServer_MethodNotAllowed` | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestServer_MethodNotAllowed' -timeout 10s` |
| API-07 | Middleware chain: recover → log → CORS → auth → handler | 04-01 | 1 | `TestMiddleware_ChainOrder` | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestMiddleware_ChainOrder' -timeout 10s` |
| API-08 | Handler panic caught; 500 returned; server survives | 04-01 | 1 | `TestRecover_PanicCaught` | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestRecover_PanicCaught' -timeout 10s` |
| API-09 | `{error, details}` envelope on 4xx/5xx | 04-01 | 1 | `TestErrors_Envelope` | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestErrors_Envelope' -timeout 10s` |
| API-10 | `Content-Type: application/json` everywhere | 04-01..04 | 1-4 | `TestHandlers_ContentType` | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestHandlers_ContentType' -timeout 10s` |
| API-11 | Request bodies closed for connection reuse | 04-03 | 3 | `TestHandlers_BodyClosed` | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestHandlers_BodyClosed' -timeout 10s` |
| WS-01 | `GET /capabilities/ws` upgrades successfully | 04-04 | 4 | `TestHandleCapabilitiesWS_Upgrade` | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestHandleCapabilitiesWS_Upgrade' -timeout 15s` |
| WS-05 | Upsert + delete broadcast `REGISTRY_UPDATED` with correct change | 04-03 / 04-05 | 3, 5 | `TestServer_Integration_WebSocketRoundTrip` | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run 'Integration_WebSocketRoundTrip' -timeout 15s` |
| WS-06 | First WS message on connect is full `SNAPSHOT` | 04-04 | 4 | `TestHandleCapabilitiesWS_Snapshot` | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestHandleCapabilitiesWS_Snapshot' -timeout 15s` |
| WS-08 | Disallowed Origin returns 403; same-host bypass documented | 04-05 | 5 | `TestServer_WebSocket_RejectsDisallowedOrigin` | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run 'RejectsDisallowedOrigin' -timeout 10s` |
| WS-09 | Architectural: `go list -deps ./internal/registry \| grep wshub` empty | — | all | gate | `! ~/sdk/go1.26.2/bin/go list -deps ./internal/registry \| grep -E 'wshub\|httpapi'` |
| OPS-01 | CORS constructor rejects `"*"` + `AllowCredentials` and empty allow-list | 04-01 / 04-05 | 1, 5 | `TestServer_New_RejectsWildcardWithCredentials`, `TestServer_New_RejectsEmptyAllowList` | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run 'TestServer_New_Rejects' -timeout 10s` |
| OPS-06 | Writes emit structured audit log (`user`, `action`, `appId`, no PII) | 04-03 | 3 | `TestServer_AuditLog` | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestServer_AuditLog' -timeout 10s` |
| TEST-02 | REST round-trip + WS round-trip via `httptest.NewServer` | 04-05 | 5 | `TestServer_Integration_RESTRoundTrip`, `TestServer_Integration_WebSocketRoundTrip` | `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestServer_Integration' -timeout 30s` |
| TEST-05 | WS origin-rejection test | 04-05 | 5 | `TestServer_WebSocket_RejectsDisallowedOrigin` | (same as WS-08) |
| TEST-06 | Credential PII test across full middleware chain | 04-05 | 5 | `TestAuth_NoCredentialsInLogs` (final end-to-end) | (same as AUTH-05) |

**Sampling continuity:** every plan has ≥1 automated verification. No three consecutive tasks without an automated check. ✓

---

## Wave 0 Requirements

All Phase 4 test files are new (no existing infrastructure to mirror except the Phase 1 `health_test.go` which will be adapted). The following files must be created as Task 0 (or Task 1 if the plan doesn't have a deps task) of each plan:

**Plan 04-01 (Server + middleware):**
- [ ] `internal/httpapi/server_test.go` — `newTestServer(t)` helper, `TestServer_New_*` constructor tests (including `RejectsWildcardWithCredentials`, `RejectsEmptyAllowList`, `MethodNotAllowed`)
- [ ] `internal/httpapi/middleware_test.go` — `TestMiddleware_ChainOrder`, `TestRecover_PanicCaught`, `TestLogMiddleware_SkipsHealth`
- [ ] `internal/httpapi/errors_test.go` — `TestErrors_Envelope`, `TestWriteUnauthorized_Header`
- [ ] Adapt `internal/httpapi/health_test.go` to use `newTestServer(t)` (the Phase 1 `New(logger)` call will no longer compile)

**Plan 04-02 (Auth + credentials):**
- [ ] `go get golang.org/x/crypto/bcrypt` + `go mod tidy` (Task 0 prereq)
- [ ] `internal/httpapi/credentials_test.go` — `TestLoadCredentials_Valid`, `TestLoadCredentials_Missing`, `TestLoadCredentials_Malformed`, `TestLoadCredentials_LowCost`
- [ ] `internal/httpapi/auth_test.go` — `TestAuth_TimingSafe`, `TestAuth_NoCredentialsInLogs`, `TestAuth_EmptyHeader`, `TestAuth_WrongPassword`, `TestAuth_UnknownUser`
- [ ] `internal/httpapi/testdata/credentials-valid.yaml` (real bcrypt cost-12 hash)
- [ ] `internal/httpapi/testdata/credentials-low-cost.yaml` (cost 10 hash)
- [ ] `internal/httpapi/testdata/credentials-malformed.yaml` (invalid YAML)

**Plan 04-03 (Registry handlers):**
- [ ] `internal/httpapi/events_test.go` — `TestNewRegistryUpdatedEvent_{Added,Updated,Removed}`, `TestNewSnapshotEvent`
- [ ] `internal/httpapi/handlers_registry_test.go` — `TestHandleRegistryUpsert_{Create,Update,InvalidBody,NoAuth,BodyTooLarge,UnknownFields}`, `TestHandleRegistryDelete_{Existing,NonExistent,NoAuth}`, `TestHandleRegistryList`, `TestHandleRegistryGet`, `TestHandlers_BodyClosed`, `TestServer_AuditLog`, `TestPublicRoutes`

**Plan 04-04 (Capabilities + WebSocket):**
- [ ] `internal/httpapi/handlers_caps_test.go` — `TestHandleCapabilities`, `TestHandleCapabilities_ActionFilter`, `TestHandleCapabilities_MimeTypeFilter`, `TestHandleCapabilities_MalformedMime`, `TestHandleCapabilitiesWS_Upgrade`, `TestHandleCapabilitiesWS_Snapshot`

**Plan 04-05 (CORS + integration tests):**
- [ ] `go get github.com/rs/cors` + `go mod tidy` (Task 0 prereq)
- [ ] `internal/httpapi/integration_test.go` — `TestServer_Integration_RESTRoundTrip`, `TestServer_Integration_WebSocketRoundTrip`, `TestServer_WebSocket_RejectsDisallowedOrigin`, `TestHandlers_ContentType` (covers API-10 across all routes)

**Framework install:** `testify` already pulled in from Phase 1. No new framework install. Two library adds: `golang.org/x/crypto/bcrypt` (04-02) and `github.com/rs/cors` (04-05).

---

## Architectural / Grep Gates

These run as part of per-plan verification and the Phase 4 gate sweep. Empty output = pass; any output = fail.

| Gate | Command | Enforces | Plan |
|------|---------|----------|------|
| Architectural isolation — registry never imports wshub/httpapi | `! ~/sdk/go1.26.2/bin/go list -deps ./internal/registry \| grep -E 'wshub\|httpapi'` | WS-09, PITFALLS #1 | 04-03 (new), 04-05 (final) |
| Architectural isolation — wshub never imports registry/httpapi | `! ~/sdk/go1.26.2/bin/go list -deps ./internal/wshub \| grep -E 'registry\|httpapi'` | Phase 3 lock (continues) | 04-05 (final) |
| No `slog.Default()` in httpapi production | `! grep -rE 'slog\.Default' internal/httpapi/*.go \| grep -v _test.go` | Phase 1 lock | 04-01, 04-05 |
| No `time.Sleep` in httpapi tests | `! grep -n 'time\.Sleep' internal/httpapi/*_test.go` | PITFALLS #16 | 04-05 |
| No `InsecureSkipVerify` in httpapi production | `! grep -rn 'InsecureSkipVerify' internal/httpapi/*.go \| grep -v _test.go` | PITFALLS #7, WS-08 | 04-04, 04-05 |
| No `internal/config` import in httpapi | `! grep -rn '"github.com/openburo/openburo-server/internal/config"' internal/httpapi/*.go` | CONTEXT.md §"Package layout" | 04-01, 04-05 |
| `go vet ./internal/httpapi/...` clean | `~/sdk/go1.26.2/bin/go vet ./internal/httpapi/...` | standard | all plans |
| `gofmt` clean | `! ~/sdk/go1.26.2/bin/gofmt -l internal/httpapi/ \| grep .` | standard | all plans |

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| — | — | — | — |

**All Phase 4 behaviors have automated verification.** The REST + WebSocket round-trips are exercised via `httptest.NewServer`. Timing-safe auth is asserted via a CPU-time comparison test. Credential PII is grep-asserted across captured slog output. There is no user-facing UI to poke manually; the REST/WS contract is the product surface and it's all grep-verifiable.

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references (12 new test files + 3 testdata fixtures)
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter after Wave 0 lands

**Approval:** pending — flips to `nyquist_compliant: true` once Wave 0 test files exist on disk.
