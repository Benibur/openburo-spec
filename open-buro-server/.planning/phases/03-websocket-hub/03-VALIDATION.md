---
phase: 3
slug: websocket-hub
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-10
---

# Phase 3 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

Derived from `03-RESEARCH.md` §Validation Architecture. The research agent
verified every `coder/websocket` v1.8.14 API claim against the local module
cache; this validation strategy operationalizes those findings into concrete
test commands and Wave 0 gaps.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go stdlib `testing` + `github.com/stretchr/testify/require` v1.11.1 + `net/http/httptest` (stdlib) |
| **Config file** | none — Go `testing` is configured via `go test` flags |
| **Quick run command** | `go test ./internal/wshub -race -run '^TestHub_\|^TestSubscribe_' -timeout 30s` |
| **Full suite command** | `go test ./internal/wshub -race -timeout 60s` |
| **Estimated runtime** | ~5 seconds (package is deliberately thin; 1000-cycle leak test is the dominant cost) |
| **Dependency gate** | `go list -deps ./internal/wshub \| grep -E 'registry\|httpapi'` must be empty |

---

## Sampling Rate

- **After every task commit:** `go test ./internal/wshub -race -run '^TestHub_\|^TestSubscribe_' -timeout 30s`
- **After every plan wave:** `go test ./internal/wshub -race -timeout 60s` + all three gates (architectural, logging, no-time-Sleep)
- **Before `/gsd:verify-work`:** Full suite green, all gates pass, race detector clean
- **Max feedback latency:** ~5 seconds

---

## Per-Task Verification Map

Concrete `TaskID → Plan → Wave → Req → Test` mapping. Plans 03-01 / 03-02 / 03-03 are locked in CONTEXT.md; task IDs below follow the Phase 2 numbering convention (`{phase}-{plan}-{task}`).

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 03-01-01 | 01 | 1 | WS-02 (Hub + subscriber + buffer defaults) | unit | `go test ./internal/wshub -race -run '^TestHub_DefaultOptions$' -timeout 10s` | ❌ W0 — `hub_test.go` | ⬜ pending |
| 03-01-02 | 01 | 1 | WS-04 (CloseRead + defer removeSubscriber) | integration (leak test) | `go test ./internal/wshub -race -run '^TestSubscribe_NoGoroutineLeak$' -timeout 30s` | ❌ W0 — `subscribe_test.go` | ⬜ pending |
| 03-02-01 | 02 | 2 | WS-03 (non-blocking fan-out + drop-slow-consumer) | unit (internal test) | `go test ./internal/wshub -race -run '^TestHub_SlowConsumerDropped$\|^TestHub_Publish_FanOut$' -timeout 10s` | ❌ W0 — `hub_test.go` | ⬜ pending |
| 03-02-02 | 02 | 2 | WS-07 (periodic ping keepalive) | integration | `go test ./internal/wshub -race -run '^TestSubscribe_PingKeepsAlive$' -timeout 10s` | ❌ W0 — `subscribe_test.go` | ⬜ pending |
| 03-02-03 | 02 | 2 | WS-03 (two close paths: slow kick vs Hub.Close) | unit | `go test ./internal/wshub -race -run '^TestHub_Close_GoingAway$' -timeout 10s` | ❌ W0 — `hub_test.go` | ⬜ pending |
| 03-03-01 | 03 | 3 | WS-10 (1000-cycle goroutine-leak test) | integration (correctness test) | `go test ./internal/wshub -race -run '^TestSubscribe_NoGoroutineLeak$' -timeout 30s -v` | ❌ W0 — `subscribe_test.go` | ⬜ pending |
| 03-03-02 | 03 | 3 | Logging contract (Warn-on-drop, Info-on-close, no PII) | unit (captured slog output) | `go test ./internal/wshub -race -run '^TestHub_Logging_DropIsWarn$\|^TestHub_Logging_CloseIsInfo$\|^TestHub_Logging_NoPII$' -timeout 10s` | ❌ W0 — `hub_test.go` | ⬜ pending |
| 03-03-03 | 03 | 3 | Architectural gates (all) | structural + grep | `go list -deps ./internal/wshub \| grep -E 'registry\|httpapi' && exit 1 \|\| exit 0 && ! grep -rE 'log/slog\|slog\.Default' internal/wshub/*.go \| grep -v _test.go && ! grep -n 'time\.Sleep' internal/wshub/*_test.go` | ✅ Phase 2 pattern | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

**Sampling continuity check:** Every plan has ≥1 automated verification. No three consecutive tasks lack an automated check. ✓

---

## Wave 0 Requirements

Tests and infrastructure that must exist before implementation lands. The planner's first task in each plan should address the relevant Wave 0 items (RED-phase tests or stubs).

- [ ] `internal/wshub/hub_test.go` — covers WS-02 (default options), WS-03 (slow-consumer drop + two close paths), plus logging assertions (Warn on drop, Info on close, no PII)
- [ ] `internal/wshub/subscribe_test.go` — covers WS-04 (CloseRead presence, validated via the leak test), WS-07 (ping keepalive with short PingInterval), WS-10 (1000-cycle goroutine-leak test)
- [ ] `internal/wshub/hub.go` — `Hub`, `Options`, `New`, `Publish`, `Close`, `addSubscriber`, `removeSubscriber`, package-level default constants
- [ ] `internal/wshub/subscribe.go` — `subscriber` struct, `Subscribe` method with writer loop
- [ ] `internal/wshub/doc.go` — extend existing Phase 1 placeholder (remove "Phase 1 ships this file only" sentence)
- [ ] Dependency add: `go get github.com/coder/websocket@v1.8.14 && go mod tidy`
- [ ] CI gate: `go list -deps ./internal/wshub | grep -E 'registry|httpapi'` (symmetric to Phase 2's registry gate)
- [ ] CI gate: `! grep -n 'time\.Sleep' internal/wshub/*_test.go` (new gate for Phase 3, guards PITFALLS #16)

Framework install: **not needed**. Go `testing` is stdlib; testify is already pulled in by Phase 1; `net/http/httptest` is stdlib.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| — | — | — | — |

**All Phase 3 behaviors have automated verification.** The hub is byte-oriented, deterministic, and headless — there is no user-facing surface to manually poke. The goroutine-leak test is the correctness oracle.

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 10s
- [ ] `nyquist_compliant: true` set in frontmatter after Wave 0 lands

**Approval:** pending — will flip to `nyquist_compliant: true` once Wave 0 test files exist on disk.
