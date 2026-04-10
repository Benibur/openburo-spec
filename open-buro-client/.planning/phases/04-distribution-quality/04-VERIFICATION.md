---
phase: 04-distribution-quality
verified: 2026-04-10T13:50:00Z
status: passed
score: 18/18 requirements verified
re_verification: false
---

# Phase 4: Distribution & Quality Verification Report

**Phase Goal:** `@openburo/client` is ready to publish — ESM/CJS/UMD builds pass type validation, full integration tests cover happy path and edge cases, and the capability-author integration guide documents the Penpal v7 contract.

**Verified:** 2026-04-10
**Status:** PASSED
**Re-verification:** No — initial verification

---

## Observable Truths (from ROADMAP.md Success Criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | `attw --pack` exits 0 — no CJSResolvesToESM or FallbackCondition errors; "types" nested correctly inside each exports condition | VERIFIED | `pnpm run ci` ran attw as final step: all 4 resolution modes green (node10, node16 CJS, node16 ESM, bundler). `pnpm dlx @arethetypeswrong/cli ./openburo-client-0.1.0.tgz` confirms "No problems found". |
| 2 | UMD build loads via `<script>` tag and exposes `window.OpenBuroClient` without polluting other globals | VERIFIED | `dist/index.umd.js` opens with IIFE that sets `e.OpenBuroClient = {}` where `e` is `globalThis` only when used as a script tag. No other globals written. |
| 3 | All four integration tests pass: happy-path castIntent, two concurrent sessions, destroy() leaving zero artifacts, same-origin rejection | VERIFIED | 147 tests pass across 16 test files (`pnpm run ci` exit 0). Specific tests confirmed in `src/client.test.ts`: lines 192, 317, 510, 378. |
| 4 | A capability author reading the integration guide knows exactly which Penpal version their iframe must use and what the `resolve(result)` method signature is | VERIFIED | `docs/capability-authors.md` (209 lines, >80 threshold). Penpal v7 explicitly named at top. `resolve(result: IntentResult)` documented with full TypeScript interfaces and working code example. |

**Score:** 4/4 truths verified

---

## Packaging Requirements (PKG-01..08)

| Req | Description | Expected | Actual | Status | Notes |
|-----|-------------|----------|--------|--------|-------|
| PKG-01 | ESM build | `dist/index.js` | `dist/index.js` exists | VERIFIED | REQUIREMENTS.md lists `dist/obc.esm.js` but tsdown convention is `index.js`; exports map and attw confirm correct ESM resolution |
| PKG-02 | CJS build | `dist/index.cjs` | `dist/index.cjs` exists | VERIFIED | REQUIREMENTS.md lists `dist/obc.cjs.js`; actual file is `index.cjs` per tsdown defaults; exports map and attw confirm correct CJS resolution |
| PKG-03 | UMD build | `dist/index.umd.js` | `dist/index.umd.js` exists | VERIFIED | Loads via `<script>` with `window.OpenBuroClient` global |
| PKG-04 | TypeScript declarations | `dist/index.d.ts` + `dist/index.d.cts` | both exist | VERIFIED | REQUIREMENTS.md says `types/index.d.ts` (outdated); actual output is `dist/index.d.ts` + `dist/index.d.cts` per tsdown; attw verifies correct type resolution for both import and require conditions |
| PKG-05 | `exports` map has nested `types` per condition | `"import": { "types": ..., "default": ... }` and `"require": { "types": ..., "default": ... }` | present in package.json | VERIFIED | `package.json` exports map: import condition has `types: ./dist/index.d.ts`, require condition has `types: ./dist/index.d.cts` |
| PKG-06 | No global window pollution; UMD opt-in via `window.OpenBuroClient` only | UMD IIFE assigns only `OpenBuroClient` key | confirmed | VERIFIED | UMD wrapper: `t(e.OpenBuroClient={},e.penpal)` — only `OpenBuroClient` assigned on globalThis when script-tag loaded |
| PKG-07 | ES2020+ target for Chrome 90+, Firefox 88+, Safari 14+ | `tsconfig.json` `"target": "ES2020"` | confirmed | VERIFIED | tsconfig.json has `"target": "ES2020"`, `"lib": ["ES2020", "DOM"]` |
| PKG-08 | Capability-author guide documents Penpal v7 contract (>=80 lines) | `docs/capability-authors.md` exists, 80+ lines, covers Penpal v7 API | 209 lines | VERIFIED | Covers: Penpal v7 requirement, WindowMessenger, `connect()` API, `resolve(result)` method signature with full IntentResult/FileResult types, query params, minimal skeleton, security expectations |

---

## Quality Gate Requirements (QA-01..10)

| Req | Description | Test File | Test Name/Count | Status |
|-----|-------------|-----------|-----------------|--------|
| QA-01 | Unit tests for resolver mime-matching (wildcard, exact, absent) | `src/capabilities/resolver.test.ts` | 9 tests covering empty array, action mismatch, absent mime, empty string, intent `*/*`, cap `*/*`, exact match, exact mismatch, mixed list | VERIFIED |
| QA-02 | Unit tests for `planCast()` discriminated union branches | `src/intent/cast.test.ts` | 8 tests: no-match, direct, reference equality, select-2, select-3, select preserves order, compile smoke, select reference equality | VERIFIED |
| QA-03 | Unit tests for UUID + `getRandomValues` fallback | `src/intent/id.test.ts` | 3 tests: randomUUID happy path, getRandomValues fallback (exercises fallback branch), uniqueness | VERIFIED |
| QA-04 | Unit tests for WebSocket backoff + destroyed guard | `src/capabilities/ws-listener.test.ts` | 11 tests: deriveWsUrl, start constructs socket, REGISTRY_UPDATED, non-matching message, malformed JSON, onopen resets counter, reconnect backoff, stop during retry, exhausted retries onError, WS-07 non-wss rejection | VERIFIED |
| QA-05 | Unit tests for modal focus trap, ESC, backdrop click | `src/ui/focus-trap.test.ts` (5 tests) + `src/ui/modal.test.ts` (14 tests) | Focus trap: Tab wrap, Shift+Tab, release, no focusables, rAF auto-focus. Modal: ESC key, backdrop click, cancel button, focus restore | VERIFIED |
| QA-06 | Integration test: happy-path `castIntent` with MockBridge | `src/client.test.ts` line 192 | "happy-path: one match → direct iframe → resolve routes to callback" — verifies fetch, MockBridge.connect called, resolve routed, callback fired, destroy called | VERIFIED |
| QA-07 | Integration test: two concurrent sessions route to correct callbacks | `src/client.test.ts` line 317 | "two concurrent sessions route resolve to the correct callback by id" — resolves session 2 first, cb1 not called, cb2 called once, then session 1 resolves correctly | VERIFIED |
| QA-08 | Integration test: `destroy()` leaves zero listeners, closed WS, zero DOM nodes | `src/client.test.ts` lines 510, 596, 560, 539 | Four tests: cancel + remove nodes on destroy during session, destroy removes all `[data-obc-host]` nodes, WsListener.stop() called, AbortSignal.aborted=true | VERIFIED |
| QA-09 | Integration test: same-origin capability rejected at cast time | `src/client.test.ts` line 378 | "same-origin capability: castIntent fires cancel callback and SAME_ORIGIN_CAPABILITY error" — asserts onError with `SAME_ORIGIN_CAPABILITY`, callback with `{ status: 'cancel' }` | VERIFIED |
| QA-10 | `@arethetypeswrong/cli --pack` passes in CI before publish | `package.json` scripts.ci | `ci` script ends with `pnpm dlx @arethetypeswrong/cli --pack`; CI confirmed passing; `attw` script also available standalone | VERIFIED |

---

## Package Polish Verification

| Item | Status | Evidence |
|------|--------|----------|
| `README.md` exists with install + usage + link to capability-authors.md | VERIFIED | 63 lines; install (npm + pnpm + CDN UMD), usage TypeScript example, link `./docs/capability-authors.md` |
| `LICENSE` exists with MIT text | VERIFIED | Standard MIT license, copyright 2026 OpenBuro contributors |
| `package.json` `description` field | VERIFIED | "Framework-agnostic browser library for OpenBuro intent brokering — discover capabilities, pick, round-trip via sandboxed iframe." |
| `package.json` `keywords` field | VERIFIED | 8 keywords: openburo, intent, iframe, file-picker, capability, penpal, postmessage, typescript |
| `package.json` `license` field | VERIFIED | "MIT" |
| `package.json` `repository` field | VERIFIED | git + url + directory |
| `package.json` `files` field | VERIFIED | `["dist", "README.md", "LICENSE"]` — docs/ intentionally excluded (decision documented in SUMMARY) |
| `package.json` `homepage` and `bugs` fields | VERIFIED | Both present |

---

## Final CI Gate Results

| Check | Command | Result |
|-------|---------|--------|
| TypeScript type check | `tsc --noEmit` | Exit 0 |
| Biome lint | `biome check .` | Exit 0 |
| Vitest | `vitest run` | 147/147 tests passed, 16 test files |
| attw in-process | `pnpm dlx @arethetypeswrong/cli --pack` | "No problems found", all 4 resolution modes green |
| `pnpm pack` | `pnpm pack` | Tarball `openburo-client-0.1.0.tgz` produced; contents: dist/ (ESM + CJS + UMD + all .d.ts + maps), README.md, LICENSE, package.json |
| attw on tarball | `pnpm dlx @arethetypeswrong/cli ./openburo-client-0.1.0.tgz` | "No problems found", all 4 resolution modes green |
| Tarball cleanup | `rm openburo-client-0.1.0.tgz` | Cleaned up |

---

## Anti-Patterns Scan

No stub implementations, TODO/FIXME markers, or empty handlers found in phase-4-created files (`docs/capability-authors.md`, `README.md`, `LICENSE`). The `package.json` `scripts.ci` is a real, executing command that was verified to pass.

No blockers or warnings found.

---

## Notable Discrepancies (Non-Blocking)

**PKG-01, PKG-02, PKG-04: File name mismatch vs REQUIREMENTS.md**

REQUIREMENTS.md was written before tsdown conventions were applied:
- PKG-01 says `dist/obc.esm.js`; actual: `dist/index.js`
- PKG-02 says `dist/obc.cjs.js`; actual: `dist/index.cjs`
- PKG-04 says `types/index.d.ts`; actual: `dist/index.d.ts` + `dist/index.d.cts`

These are naming differences only. The exports map is correct, the attw gate passes all 4 resolution modes with zero errors, and the tarball contents are correct. The intent of the requirements is fully satisfied. REQUIREMENTS.md was not updated to reflect tsdown file naming conventions — this is a documentation drift issue, not an implementation gap.

---

## Human Verification Required

### 1. UMD Browser Script Tag Test

**Test:** Load `dist/index.umd.js` via `<script>` in a real static HTML page (no bundler). Check `window.OpenBuroClient` is defined and `window.penpal` is NOT exposed as a standalone global.

**Expected:** `typeof window.OpenBuroClient === 'object'` with exported names visible; no `window.penpal` pollution.

**Why human:** The IIFE structure is verified in code, but confirming actual browser behavior with a CDN script tag requires a manual smoke test.

---

## Requirements Coverage Summary

| Requirement | Status | Evidence |
|-------------|--------|----------|
| PKG-01 | SATISFIED | `dist/index.js` exists; attw node10/bundler green |
| PKG-02 | SATISFIED | `dist/index.cjs` exists; attw node16 CJS green |
| PKG-03 | SATISFIED | `dist/index.umd.js` exists; IIFE with OpenBuroClient global |
| PKG-04 | SATISFIED | `dist/index.d.ts` + `dist/index.d.cts` exist; attw type resolution green |
| PKG-05 | SATISFIED | exports map has nested types per import/require condition |
| PKG-06 | SATISFIED | UMD assigns only `OpenBuroClient` key on globalThis |
| PKG-07 | SATISFIED | tsconfig.json `"target": "ES2020"` |
| PKG-08 | SATISFIED | docs/capability-authors.md 209 lines, full Penpal v7 contract documented |
| QA-01 | SATISFIED | resolver.test.ts 9 tests passing |
| QA-02 | SATISFIED | cast.test.ts 8 tests passing |
| QA-03 | SATISFIED | id.test.ts 3 tests including fallback branch |
| QA-04 | SATISFIED | ws-listener.test.ts 11 tests passing |
| QA-05 | SATISFIED | focus-trap.test.ts (5) + modal.test.ts (14) passing |
| QA-06 | SATISFIED | client.test.ts happy-path integration test passing |
| QA-07 | SATISFIED | client.test.ts concurrent sessions test passing |
| QA-08 | SATISFIED | client.test.ts 4 destroy() tests passing |
| QA-09 | SATISFIED | client.test.ts same-origin rejection test passing |
| QA-10 | SATISFIED | `ci` script includes attw gate; verified passing |

**Coverage: 18/18 requirements satisfied**

---

## Goal Assessment

The phase goal is achieved:

1. **ESM/CJS/UMD builds pass type validation** — `attw --pack` runs clean (no CJSResolvesToESM, no FallbackCondition), all 4 resolution modes green, exports map has nested `types` per condition.

2. **Full integration tests cover happy path and edge cases** — 147 tests pass including all 4 explicitly required integration scenarios (happy-path castIntent, concurrent sessions, destroy() leak-free, same-origin rejection).

3. **Capability-author integration guide documents the Penpal v7 contract** — `docs/capability-authors.md` (209 lines) covers Penpal version requirement, WindowMessenger API, `resolve()` method signature with TypeScript types, query parameter contract, minimal skeleton, and security expectations.

`@openburo/client` is ready to publish.

---

_Verified: 2026-04-10_
_Verifier: Claude (gsd-verifier)_
