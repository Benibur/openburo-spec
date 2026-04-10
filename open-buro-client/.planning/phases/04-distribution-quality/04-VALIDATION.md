---
phase: 4
slug: distribution-quality
status: draft
nyquist_compliant: true
wave_0_complete: false
created: 2026-04-10
---

# Phase 4 — Validation Strategy

## Test Infrastructure

| Property | Value |
|----------|-------|
| Framework | Vitest 4.1.4 |
| Quick run | `pnpm vitest run src/client.test.ts -t "same-origin"` |
| Full suite | `pnpm run ci` |
| Pack gate | `pnpm pack && pnpm dlx @arethetypeswrong/cli ./openburo-client-*.tgz` |
| Estimated runtime | ~25s full gate |

## Sampling Rate

- After every task: `pnpm vitest run src/client.test.ts`
- Before phase gate: `pnpm run ci && pnpm pack && attw on tarball`
- Max feedback latency: 25 seconds

## Per-Task Verification Map

| Task | Plan | Wave | Requirement(s) | Test Type | Automated Command |
|------|------|------|----------------|-----------|-------------------|
| 4-01-01 | audit | 1 | QA-01..08 audit | grep / file inspection | `pnpm vitest run` + file-exists checks |
| 4-01-02 | audit | 1 | QA-09 (same-origin integration) | unit (happy-dom) | `pnpm vitest run src/client.test.ts -t "same-origin"` |
| 4-01-03 | docs | 1 | PKG-08 (capability-author guide) | file exists + content | `test -f docs/capability-authors.md && grep -q "Penpal" docs/capability-authors.md` |
| 4-01-04 | packaging | 1 | PKG-01..07 (build + metadata + LICENSE) | file exists | `test -f LICENSE && test -f README.md && pnpm pack` |
| 4-01-05 | phase gate | 2 | composite | composite | `pnpm run ci && pnpm pack && pnpm dlx @arethetypeswrong/cli ./openburo-client-*.tgz` |

## Wave 0 Requirements

- [ ] `docs/capability-authors.md` (new)
- [ ] `LICENSE` (new, MIT)
- [ ] `README.md` (new or extended)
- [ ] `src/client.test.ts` extended with QA-09 same-origin test
- [ ] `package.json` metadata polished (description, keywords, repository, homepage, bugs, license)

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Actual npm publish | PKG (partial) | Requires credentials, user decision | User runs `npm publish` manually when ready |
| Real browser smoke test | IFR-07, UI-08 | No Playwright setup in v1 | User opens `dist/obc.umd.js` in a demo HTML page |

## Validation Sign-Off

- [x] All tasks have `<automated>` verify
- [x] Sampling continuity OK
- [ ] Wave 0 covers MISSING refs *(set on execution)*
- [x] No watch-mode flags
- [x] Feedback latency < 25s
- [x] `nyquist_compliant: true`

**Approval:** pending
