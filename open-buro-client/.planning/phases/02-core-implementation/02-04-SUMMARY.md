---
phase: 02-core-implementation
plan: "04"
subsystem: messaging
tags: [penpal, penpal-v7, bridge-adapter, iframe, postmessage, vitest, happy-dom]

# Dependency graph
requires:
  - phase: 01-foundations
    provides: "src/types.ts (IntentResult), src/errors.ts (OBCError), base project scaffold"
provides:
  - "BridgeAdapter + ConnectionHandle + ParentMethods pure-type interfaces (src/messaging/bridge-adapter.ts)"
  - "MockBridge test double implementing BridgeAdapter with counters + lastMethods (src/messaging/mock-bridge.ts)"
  - "PenpalBridge production implementation using Penpal v7 connect() + WindowMessenger (src/messaging/penpal-bridge.ts)"
affects:
  - "02-orchestration (Phase 3) — imports BridgeAdapter; injects PenpalBridge or MockBridge"
  - "02-intent — Session type uses ConnectionHandle from this plan"

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "BridgeAdapter interface as seam — orchestrator depends on interface, not Penpal directly"
    - "vi.mock('penpal') with constructor-compatible vi.fn() for Vitest 4 unit testing of Penpal classes"
    - "happy-dom environment for messaging tests that use HTMLIFrameElement"
    - "TDD for MockBridge and PenpalBridge — RED (test written first) → GREEN (implementation)"

key-files:
  created:
    - src/messaging/bridge-adapter.ts
    - src/messaging/bridge-adapter.test.ts
    - src/messaging/mock-bridge.ts
    - src/messaging/mock-bridge.test.ts
    - src/messaging/penpal-bridge.ts
    - src/messaging/penpal-bridge.test.ts
  modified: []

key-decisions:
  - "WindowMessenger mock must use vi.fn(function(opts){...}) not vi.fn().mockImplementation(arrow) — arrow functions cannot be used as constructors with 'new'"
  - "penpal-bridge.test.ts imports penpal (for vi.mock spy assertions) — this is expected and excluded from the grep-enforced single-import-site check"
  - "Pre-existing lint/typecheck errors in parallel-plan files (src/ui/styles.ts, src/capabilities/loader.test.ts, src/ui/iframe.test.ts) are out-of-scope"

patterns-established:
  - "Messaging layer never touches document or window.document — receives remoteWindow as parameter"
  - "PenpalBridge is the ONLY file importing 'penpal'; all other layers use BridgeAdapter interface"
  - "MockBridge exposes lastMethods so orchestrator tests can simulate child calling parent.resolve()"

requirements-completed: [MSG-01, MSG-02, MSG-03, MSG-04, MSG-05, MSG-06]

# Metrics
duration: 4min
completed: 2026-04-10
---

# Phase 02 Plan 04: Messaging Layer Summary

**BridgeAdapter interface + PenpalBridge (Penpal v7 connect/WindowMessenger) + MockBridge test double with grep-enforced single import site**

## Performance

- **Duration:** ~4 min
- **Started:** 2026-04-10T12:21:00Z
- **Completed:** 2026-04-10T12:24:10Z
- **Tasks:** 3
- **Files modified:** 6 created, 0 modified

## Accomplishments
- Defined `BridgeAdapter` interface as the sole seam between orchestrator and Penpal — compile-time enforced
- Implemented `MockBridge` with `lastMethods`, `connectCallCount`, `destroyCallCount` for full orchestrator test coverage
- Implemented `PenpalBridge` using Penpal v7 `connect({ messenger: new WindowMessenger({ remoteWindow, allowedOrigins }), methods, timeout })` — v6 `connectToChild` never appears
- 15 tests pass (3 bridge-adapter + 6 mock-bridge + 6 penpal-bridge); `grep -r "from 'penpal'" src/ | grep -v penpal-bridge` returns nothing

## Task Commits

Each task was committed atomically:

1. **Task 1: BridgeAdapter interface + ConnectionHandle + ParentMethods** - `0419e49` (feat)
2. **Task 2: MockBridge test double implementing BridgeAdapter** - `61c9f6d` (feat)
3. **Task 3: PenpalBridge using Penpal v7 connect + WindowMessenger** - `7769cb0` (feat)

## Files Created/Modified
- `src/messaging/bridge-adapter.ts` — Pure-type interfaces: BridgeAdapter, ConnectionHandle, ParentMethods. No runtime code.
- `src/messaging/bridge-adapter.test.ts` — 3 compile-time smoke tests; Node env (no happy-dom needed)
- `src/messaging/mock-bridge.ts` — MockBridge implements BridgeAdapter; exposes lastMethods, connectCallCount, destroyCallCount
- `src/messaging/mock-bridge.test.ts` — 6 tests; `// @vitest-environment happy-dom`
- `src/messaging/penpal-bridge.ts` — PenpalBridge sole Penpal import site; connect() uses Penpal v7 API; default 10s timeout
- `src/messaging/penpal-bridge.test.ts` — 6 tests with vi.mock('penpal'); `// @vitest-environment happy-dom`

## Decisions Made
- `WindowMessenger` in `vi.mock()` must use a regular function (`vi.fn(function(opts){...})`) not an arrow function — arrow functions are not valid constructors and `new WindowMessenger(...)` would throw at runtime in tests. Fixed during Task 3 GREEN phase.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed WindowMessenger mock constructor incompatibility**
- **Found during:** Task 3 (PenpalBridge GREEN phase)
- **Issue:** Plan template used `vi.fn().mockImplementation((opts) => ({ _mockOpts: opts }))` — arrow function cannot be called with `new`, causing `TypeError: ... is not a constructor` for all 4 tests relying on `new WindowMessenger()`
- **Fix:** Changed mock to `vi.fn(function(opts) { this._mockOpts = opts; })` which is constructor-compatible
- **Files modified:** `src/messaging/penpal-bridge.test.ts`
- **Verification:** All 6 penpal-bridge tests pass after fix
- **Committed in:** `7769cb0` (Task 3 commit)

---

**Total deviations:** 1 auto-fixed (Rule 1 — bug)
**Impact on plan:** Necessary for test correctness. No scope change.

## Penpal v7 Specific Notes

- **happy-dom `iframe.contentWindow`:** In happy-dom 20.8.9, iframes appended to `document.body` do provide a non-null `contentWindow` stub. The null-contentWindow test uses `Object.defineProperty(detachedIframe, 'contentWindow', { value: null })` as documented in the plan.
- **`vi.mock('penpal')` interception:** Works correctly in Vitest 4 — `vi.isMockFunction(connect)` returns `true`, confirming the mock is active when PenpalBridge is imported.
- **Penpal v7 type mismatches:** None found. `connect<TMethods>({ messenger, methods, timeout })` signature, `Connection<TMethods>.promise`, `Connection.destroy()`, and `WindowMessenger({ remoteWindow, allowedOrigins })` all match the installed `penpal@7.0.6` type declarations.

## Issues Encountered
- Pre-existing lint errors in parallel-plan files (`src/ui/styles.ts`, `src/capabilities/loader.test.ts`, `src/ui/iframe.test.ts`) caused `pnpm lint` and `pnpm typecheck` to exit non-zero. These are out-of-scope per deviation rules — messaging files themselves are clean.

## Next Phase Readiness
- `BridgeAdapter` interface ready for Phase 3 orchestrator injection
- `MockBridge` ready for orchestrator unit tests (Phase 3)
- `PenpalBridge` ready for production use in Phase 3 and Playwright validation in Phase 4
- Penpal v7 single-import-site contract enforced via grep (`grep -r "from 'penpal'" src/ | grep -v penpal-bridge` → empty)

---
*Phase: 02-core-implementation*
*Completed: 2026-04-10*
