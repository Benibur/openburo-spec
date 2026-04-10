---
phase: 02-core-implementation
plan: 01
subsystem: capabilities
tags: [websocket, abort-controller, mime, fetch, vitest, fake-websocket]

# Dependency graph
requires:
  - phase: 01-foundations
    provides: OBCError, Capability, IntentRequest types; Biome/TypeScript config
provides:
  - "Pure MIME resolver: resolve(capabilities, intent) -> Capability[]"
  - "HTTP capability loader: fetchCapabilities(url, signal) -> Promise<LoaderResult>"
  - "WsListener class with full-jitter exponential backoff and destroyed guard"
  - "createAbortContext() lifecycle helper for Phase 3 destroy() composition"
affects:
  - 02-02-intent (planCast uses resolve() output)
  - 03-orchestration (fetchCapabilities + WsListener + createAbortContext composed)

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "FakeWebSocket in-test class pattern — no external msw-ws dependency"
    - "vi.stubGlobal('fetch') for HTTP mocking without external libraries"
    - "Math.random spy (mockReturnValue(0)) for deterministic jitter tests"
    - "noUncheckedIndexedAccess guard: check !== undefined before array[0] use"
    - "AbortController + LIFO cleanup stack pattern for leak-free teardown"

key-files:
  created:
    - src/capabilities/resolver.ts
    - src/capabilities/resolver.test.ts
    - src/capabilities/loader.ts
    - src/capabilities/loader.test.ts
    - src/capabilities/ws-listener.ts
    - src/capabilities/ws-listener.test.ts
    - src/lifecycle/abort-context.ts
    - src/lifecycle/abort-context.test.ts
    - src/vitest-env.d.ts
  modified:
    - src/messaging/penpal-bridge.ts
    - src/ui/focus-trap.ts

key-decisions:
  - "Single quotes used throughout (Biome quoteStyle: single), not double quotes as noted in critical rules"
  - "Template literals preferred over string concatenation (Biome lint/style/useTemplate)"
  - "** exponentiation operator used instead of Math.pow (Biome lint/style/useExponentiationOperator)"
  - "FakeWebSocket static constants (CONNECTING=0, OPEN=1, CLOSED=3) defined in test to avoid Node-env WebSocket absence"
  - "simulateClose() uses plain Event('close') not CloseEvent (not available in Node env)"
  - "Deterministic jitter tests use vi.spyOn(Math, 'random').mockReturnValue(0) for delay=0"
  - "WS-05 destroyed guard implemented in three locations: start(), connect(), and setTimeout callback body"

patterns-established:
  - "Pattern: FakeWebSocket with static instance tracking — stub WebSocket global and close immediately before auto-open microtask to test exhaustion"
  - "Pattern: vi.stubGlobal / vi.unstubAllGlobals in beforeEach/afterEach — never share global stubs across tests"
  - "Pattern: AbortContext LIFO cleanup stack drains on single 'abort' event (once:true listener)"

requirements-completed:
  - CAP-01
  - CAP-02
  - CAP-03
  - CAP-04
  - CAP-05
  - CAP-06
  - CAP-07
  - RES-01
  - RES-02
  - RES-03
  - RES-04
  - RES-05
  - RES-06
  - WS-01
  - WS-02
  - WS-03
  - WS-04
  - WS-05
  - WS-06
  - WS-07
  - LIFECYCLE-01

# Metrics
duration: 11min
completed: 2026-04-10
---

# Phase 02 Plan 01: Capabilities Layer Summary

**Pure MIME resolver, HTTPS-guarded fetch loader with AbortSignal, WebSocket listener with full-jitter backoff and destroyed-flag guard, and createAbortContext() LIFO cleanup helper — 32 tests across 4 modules, all in Node env with zero DOM/Penpal imports**

## Performance

- **Duration:** 11 min
- **Started:** 2026-04-10T10:20:27Z
- **Completed:** 2026-04-10T10:32:11Z
- **Tasks:** 4
- **Files created:** 9 (8 source/test + vitest-env.d.ts shim)
- **Files modified:** 2 (pre-existing penpal-bridge.ts, focus-trap.ts)

## Accomplishments
- Delivered all 4 capability layer modules with TDD (RED → GREEN for each)
- 32 tests pass: 9 resolver + 6 loader + 11 ws-listener + 6 abort-context
- Full CI (`pnpm run ci`) passes including typecheck + lint + attw
- WsListener destroyed-guard verified by explicit test advancing fake timers after stop()
- createAbortContext() ready for Phase 3 orchestrator destroy() composition

## Task Commits

1. **Task 1: Pure MIME resolver** - `801b117` (feat)
2. **Task 2: HTTP capability loader** - committed in `1ecd3ba` (pre-existing metadata commit)
3. **Task 3: WsListener with full-jitter backoff** - `9944ff1` (feat)
4. **Task 4: createAbortContext lifecycle helper** - `f0232bf` (feat)

## Files Created/Modified
- `src/capabilities/resolver.ts` — Pure MIME filter, 5-rule match chain (RES-02..06)
- `src/capabilities/resolver.test.ts` — 9 tests covering all action+mime match rules
- `src/capabilities/loader.ts` — fetchCapabilities with HTTPS guard, AbortSignal, OBCError mapping
- `src/capabilities/loader.test.ts` — 6 tests: happy path, header, non-200, network error, HTTPS guard, abort
- `src/capabilities/ws-listener.ts` — WsListener class + deriveWsUrl helper
- `src/capabilities/ws-listener.test.ts` — 11 tests using FakeWebSocket; covers reconnect, jitter, WS-05 guard
- `src/lifecycle/abort-context.ts` — createAbortContext() with LIFO cleanup stack
- `src/lifecycle/abort-context.test.ts` — 6 tests: initial state, abort, LIFO order, error-swallow, idempotency, post-abort addCleanup
- `src/vitest-env.d.ts` — Global type shim for `window.happyDOM.setURL` (unblocks typecheck)
- `src/messaging/penpal-bridge.ts` — Fixed: cast `ParentMethods as unknown as Methods` for Penpal compatibility
- `src/ui/focus-trap.ts` — Fixed: cast `onKeyDown as EventListener` for ShadowRoot.addEventListener overload

## Decisions Made
- Biome enforces single quotes (not double as in critical rules), template literals, and `**` operator — all auto-fixed
- `CloseEvent` unavailable in Node env; used plain `Event('close')` for FakeWebSocket.simulateClose()
- Jitter tests stub `Math.random` to 0 for deterministic delay=0 — avoids fake timer race conditions
- `vi.runAllMicrotasksAsync` not available in Vitest 4; used `await Promise.resolve()` instead
- FakeWebSocket defined `static CONNECTING/OPEN/CLOSED` constants to be usable in Node without global WebSocket

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Biome requires single quotes, not double quotes**
- **Found during:** Task 1 (lint check)
- **Issue:** Files written with double quotes; Biome formatter mandates single quotes
- **Fix:** `biome check --write` auto-fixed all new files
- **Files modified:** resolver.ts, resolver.test.ts, loader.ts, loader.test.ts, ws-listener.ts, ws-listener.test.ts
- **Verification:** `pnpm lint` passes with no errors

**2. [Rule 3 - Blocking] Missing `src/ui/styles.ts` caused typecheck failure**
- **Found during:** Task 2 (typecheck check)
- **Issue:** Pre-existing untracked `src/ui/styles.test.ts` imports `./styles.js` but `styles.ts` existed already (untracked). TypeCheck showed `happyDOM` property error
- **Fix:** Created `src/vitest-env.d.ts` shim to declare `window.happyDOM.setURL`; `styles.ts` already existed
- **Files modified:** src/vitest-env.d.ts (created)
- **Verification:** `pnpm typecheck` exits 0

**3. [Rule 1 - Bug] `src/messaging/penpal-bridge.ts` type error: ParentMethods not assignable to Penpal Methods**
- **Found during:** Task 2 (typecheck)
- **Issue:** Penpal's `Methods` type requires `[index: string]: Methods | Function`; `ParentMethods` lacks index signature
- **Fix:** Cast `methods as unknown as Methods` in connect() call; imported `type Methods` from penpal
- **Files modified:** src/messaging/penpal-bridge.ts
- **Verification:** `pnpm typecheck` exits 0

**4. [Rule 1 - Bug] `src/ui/focus-trap.ts` ShadowRoot.addEventListener overload mismatch**
- **Found during:** Task 2 (typecheck)
- **Issue:** ShadowRoot.addEventListener strict overloads reject `(e: KeyboardEvent) => void` listener type
- **Fix:** Cast `onKeyDown as EventListener` in addEventListener and removeEventListener calls
- **Files modified:** src/ui/focus-trap.ts
- **Verification:** `pnpm typecheck` exits 0

**5. [Rule 1 - Bug] `vi.runAllMicrotasksAsync` not available in Vitest 4**
- **Found during:** Task 3 (test run)
- **Issue:** API does not exist; causes TypeError at runtime
- **Fix:** Replaced with `await Promise.resolve()` to flush microtask queue
- **Files modified:** ws-listener.test.ts
- **Verification:** 11 tests pass

**6. [Rule 1 - Bug] `CloseEvent` not defined in Node environment**
- **Found during:** Task 3 (test run)
- **Issue:** Node env lacks DOM's CloseEvent; `new CloseEvent('close')` throws ReferenceError
- **Fix:** Changed FakeWebSocket.simulateClose() to use `new Event('close')` (onclose handler ignores event data)
- **Files modified:** ws-listener.test.ts
- **Verification:** 11 tests pass

---

**Total deviations:** 6 auto-fixed (4 Rule 1 bugs, 1 Rule 3 blocking, 1 formatting)
**Impact on plan:** All fixes were for pre-existing issues uncovered by typecheck/lint/test, or for Node env limitations not mentioned in plan. No scope creep.

## Issues Encountered
- Vitest 4 fake timers + Promise.resolve() microtask interaction required careful ordering: close before microtask flushes (before `await Promise.resolve()`) so `attempt` counter isn't reset by `onopen`
- Math.pow rejected by Biome in favor of `**` operator (auto-fixable, no logic change)
- noUncheckedIndexedAccess: no additional guards needed in this plan beyond what was designed

## Next Phase Readiness
- Phase 3 orchestrator can import `fetchCapabilities`, `resolve`, `WsListener`, `createAbortContext`
- `createAbortContext()` provides the AbortController primitive required by CONTEXT.md locked decision
- Layer isolation verified: zero penpal/document/window imports in capabilities/ or lifecycle/

---
*Phase: 02-core-implementation*
*Completed: 2026-04-10*

## Self-Check: PASSED

- [x] src/capabilities/resolver.ts — FOUND
- [x] src/capabilities/resolver.test.ts — FOUND
- [x] src/capabilities/loader.ts — FOUND
- [x] src/capabilities/loader.test.ts — FOUND
- [x] src/capabilities/ws-listener.ts — FOUND
- [x] src/capabilities/ws-listener.test.ts — FOUND
- [x] src/lifecycle/abort-context.ts — FOUND
- [x] src/lifecycle/abort-context.test.ts — FOUND
- [x] Commit 801b117 — FOUND (resolver)
- [x] Commit 9944ff1 — FOUND (ws-listener)
- [x] Commit f0232bf — FOUND (abort-context)
- [x] pnpm vitest run src/capabilities/ src/lifecycle/ — 32 tests pass
- [x] pnpm run ci — exits 0
