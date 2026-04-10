---
phase: 1
slug: foundations
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-10
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Vitest 4.1.4 |
| **Config file** | `vitest.config.ts` (Wave 0 — does not exist yet) |
| **Quick run command** | `pnpm vitest run src/intent/id.test.ts` |
| **Full suite command** | `pnpm vitest run` |
| **Estimated runtime** | ~3 seconds (single-file quick), ~8 seconds (full) |

---

## Sampling Rate

- **After every task commit:** Run `pnpm vitest run` (project is small; full suite is fast)
- **After every plan wave:** Run `pnpm vitest run && pnpm tsc --noEmit`
- **Before `/gsd:verify-work`:** Full suite + `pnpm build` + `pnpm dlx @arethetypeswrong/cli --pack` must be green
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 1-01-01 | 01 | 0 | FOUND-01 | wave-0 scaffold | `test -f package.json && test -f tsconfig.json && test -f tsdown.config.ts && test -f vitest.config.ts && test -f biome.json` | ❌ W0 | ⬜ pending |
| 1-01-02 | 01 | 0 | FOUND-01 | wave-0 smoke | `pnpm install && pnpm build && pnpm vitest run` | ❌ W0 | ⬜ pending |
| 1-01-03 | 01 | 1 | FOUND-02 | unit | `pnpm vitest run src/errors.test.ts` | ❌ W0 | ⬜ pending |
| 1-01-04 | 01 | 1 | FOUND-03 | unit (type-level) | `pnpm tsc --noEmit` | ❌ W0 | ⬜ pending |
| 1-01-05 | 01 | 1 | FOUND-04 | unit (type-level) | `pnpm tsc --noEmit && pnpm vitest run src/index.test.ts` | ❌ W0 | ⬜ pending |
| 1-01-06 | 01 | 1 | FOUND-05 | unit | `pnpm vitest run src/intent/id.test.ts` | ❌ W0 | ⬜ pending |
| 1-01-07 | 01 | 1 | FOUND-06 | smoke | `node -e "const p=require('./package.json'); if(p.dependencies.penpal!=='7.0.6') process.exit(1)"` | ❌ W0 | ⬜ pending |
| 1-01-08 | 01 | 2 | FOUND-07 | CI gate | `pnpm build && pnpm dlx @arethetypeswrong/cli --pack` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

All test infrastructure is greenfield. Wave 0 delivers:

- [ ] `package.json` — declares `penpal@7.0.6` (exact), dev deps, scripts `build`/`test`/`typecheck`/`lint`/`attw`
- [ ] `tsconfig.json` — TS 6, target ES2020, rootDir ./src, types: [], noEmit: true, noUncheckedIndexedAccess: true
- [ ] `tsdown.config.ts` — two-array form (ESM+CJS external penpal, UMD bundles penpal)
- [ ] `vitest.config.ts` — Node environment for Phase 1; happy-dom environment switched on in Phase 2 (UI layer)
- [ ] `biome.json` — formatter + linter config
- [ ] `.gitignore` — `node_modules/`, `dist/`, `*.tsbuildinfo`
- [ ] `src/errors.ts` — `OBCError` class + `OBCErrorCode` union (seven codes incl. `SAME_ORIGIN_CAPABILITY`)
- [ ] `src/errors.test.ts` — stubs for FOUND-02, FOUND-03
- [ ] `src/types.ts` — shared interfaces (`Capability`, `IntentRequest`, `IntentResult`, `FileResult`, `IntentCallback`, `OBCOptions`, `CastPlan`)
- [ ] `src/intent/id.ts` — `generateSessionId()` with `crypto.randomUUID` + `getRandomValues` fallback
- [ ] `src/intent/id.test.ts` — stubs for FOUND-05 (both branches)
- [ ] `src/index.ts` — re-exports errors, types; exports `generateSessionId`
- [ ] `src/index.test.ts` — import-surface smoke test for FOUND-04

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| (none) | — | All FOUND-01..07 have automated commands | — |

*All phase behaviors have automated verification.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags (only `vitest run`, never `vitest watch`)
- [ ] Feedback latency < 10s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
