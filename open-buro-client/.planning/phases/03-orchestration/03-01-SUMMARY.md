---
phase: "03-orchestration"
plan: "01"
subsystem: "orchestrator"
tags: ["openburoclient", "orchestration", "capability-fetch", "session-management", "destroy", "iframe"]
dependency_graph:
  requires:
    - "02-01 (capabilities loader, resolver, ws-listener)"
    - "02-02 (intent: cast, session, id)"
    - "02-03 (ui: styles, iframe, modal)"
    - "02-04 (messaging: bridge-adapter, mock-bridge, penpal-bridge)"
    - "02-05 (lifecycle: abort-context)"
  provides:
    - "OpenBuroClient public facade"
    - "DESTROYED error code"
    - "Phase 3 barrel exports"
  affects:
    - "src/index.ts (Phase 4 bundles this)"
tech_stack:
  added:
    - "window.happyDOM.settings.disableIframePageLoading mutation in test beforeEach (happy-dom iframe suppression)"
  patterns:
    - "Single-flight fetch Promise (inflightFetch: Promise<LoaderResult> | null)"
    - "AbortContext LIFO cleanup stack for leak-free teardown"
    - "Synchronicity pitfall: buildIframe → appendChild → bridgeFactory.connect in one sync block"
    - "Post-destroy guard on all public methods"
key_files:
  created:
    - "src/client.ts"
    - "src/client.test.ts"
  modified:
    - "src/errors.ts"
    - "src/errors.test.ts"
    - "src/index.ts"
    - "src/index.test.ts"
    - "src/vitest-env.d.ts"
    - "vitest.config.ts"
decisions:
  - "Post-destroy castIntent throws as rejected Promise (async function) — not sync throw — matching spec intent"
  - "window.happyDOM.settings.disableIframePageLoading set per-test in beforeEach to prevent happy-dom network requests without breaking penpal-bridge tests"
  - "Watchdog test uses real timers with 50ms sessionTimeoutMs instead of vi.useFakeTimers() — avoids microtask drain complexity with fake timers"
  - "Tasks 2-3-4 implemented together in one commit (client.ts + client.test.ts built incrementally)"
metrics:
  duration: "17 min"
  completed_date: "2026-04-10"
  tasks_completed: 5
  files_created: 2
  files_modified: 6
  tests_added: 35
  total_tests: 147
---

# Phase 3 Plan 01: OpenBuroClient Orchestrator Summary

**One-liner:** OpenBuroClient facade wiring all Phase 2 layers with lazy single-flight capability fetch, castIntent orchestration (no-match/direct/select), AbortContext LIFO teardown, and post-destroy invariants.

## Requirement Coverage

| Requirement | Description | Test |
|-------------|-------------|------|
| ORCH-01 | Constructor validates capabilitiesUrl (https or localhost only) | "throws CAPABILITIES_FETCH_FAILED on non-HTTPS" |
| ORCH-02 | Constructor performs no async side effects | "constructs with https URL without calling fetch" |
| ORCH-03 | Two concurrent instances have isolated session maps | "two separate OpenBuroClient instances maintain isolated capability state" |
| ORCH-04 | destroy() tears down every active session, DOM, WS, fetch | "destroy during active session cancels callback and removes DOM nodes" |
| ORCH-05 | destroy() uses AbortContext as single teardown mechanism | grep createAbortContext/addCleanup in src/client.ts |
| ORCH-06 | Post-destroy behavior: castIntent rejected, getCapabilities [], refreshCapabilities rejected, destroy idempotent | 4 dedicated tests |
| FOUND-03+ | DESTROYED added to OBCErrorCode union | src/errors.ts + src/errors.test.ts |

## Tasks Completed

| Task | Description | Commit |
|------|-------------|--------|
| 1 | Add DESTROYED to OBCErrorCode, extend errors.test.ts | 7af0ebe |
| 2-4 | OpenBuroClient scaffold + castIntent + destroy (Tasks 2, 3, 4 implemented together) | 3ae94eb |
| 5 | Export from barrel, run pnpm run ci (phase gate) | dc89c44 |

## Key Design Choices

### Single-flight fetch
`inflightFetch: Promise<LoaderResult> | null` is set before the fetch call and cleared in `finally`. Two concurrent `castIntent` or `refreshCapabilities` calls await the same Promise.

### Synchronicity pitfall (documented inline)
`buildIframe() → shadowRoot.appendChild(iframe) → bridgeFactory.connect()` run in ONE synchronous block before any `await`. If an `await` is inserted between appendChild and connect, the child frame may postMessage before Penpal is listening.

### destroy() via AbortContext
`abortContext.abort()` drains the LIFO cleanup stack — each session, modal, and WsListener registered its own cleanup via `addCleanup`. destroy() sets `this.destroyed = true` first, then aborts, then defensively clears the map.

### Post-destroy castIntent
Since `castIntent` is declared `async`, a `throw` before the first `await` produces a rejected Promise (not a synchronous throw). The test asserts `.rejects.toMatchObject({ code: 'DESTROYED' })`.

### Happy-dom iframe suppression
`window.happyDOM.settings.disableIframePageLoading = true` is set in `beforeEach` to prevent happy-dom from making real HTTPS requests when iframes are appended to the shadow DOM. This is NOT set globally in vitest.config.ts (which would break penpal-bridge tests that need `iframe.contentWindow`).

## Test Metrics

- **Total tests:** 147 (112 Phase 2 + 35 Phase 3)
- **client.test.ts:** 31 tests across 3 describe blocks
- **errors.test.ts:** 2 new tests (DESTROYED construction + cause)
- **index.test.ts:** 2 new tests (Phase 3 API surface)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Vitest global environmentOptions for happyDOM settings**
- **Found during:** Tasks 2-4
- **Issue:** Setting `environmentOptions.happyDOM.settings.disableIframePageLoading` in vitest.config.ts breaks penpal-bridge tests by preventing `iframe.contentWindow` from being set. The setting needs to be scoped to client.test.ts only.
- **Fix:** Moved iframe loading suppression to `beforeEach` in client.test.ts via `window.happyDOM.settings.disableIframePageLoading = true`
- **Files modified:** `src/client.test.ts`, `vitest.config.ts` (reverted to no environmentOptions)

**2. [Rule 1 - Bug] `vi.useFakeTimers()` in watchdog test caused microtask drain complexity**
- **Found during:** Task 3/4
- **Issue:** With fake timers, `await Promise.resolve()` x2 was insufficient to drain the fetch → ensureCapabilities → openIframeSession → connectPromise microtask chain. Test timed out.
- **Fix:** Replaced fake timer test with real timers + `sessionTimeoutMs: 50` + real 50ms wait via `await castPromise`
- **Commit:** 3ae94eb

**3. [Rule 2 - Missing] Post-destroy guard after async awaits**
- **Found during:** Tasks 3/4 integration
- **Issue:** destroy() called while `openIframeSession` was mid-flight (after fetch, before connect resolved) left a dangling session — the abortContext cleanup ran before the session was registered.
- **Fix:** Added `if (this.destroyed)` check after `await connectPromise` to short-circuit the session creation when destroy was called during the connection phase.
- **Files modified:** `src/client.ts`

## Self-Check: PASSED

- `src/errors.ts` contains 'DESTROYED': confirmed
- `src/client.ts` contains 'class OpenBuroClient': confirmed (min_lines 350+)
- `src/client.test.ts` contains 'MockBridge': confirmed (31 tests)
- `src/index.ts` contains 'OpenBuroClient': confirmed
- `pnpm run ci` exits 0: confirmed (147 tests, attw 🟢)
- Commits exist: 7af0ebe, 3ae94eb, dc89c44 confirmed via git log
