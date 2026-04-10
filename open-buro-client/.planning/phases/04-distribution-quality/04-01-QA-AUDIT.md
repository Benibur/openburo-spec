# QA Audit — Phase 4

Generated: 2026-04-10

| REQ | Description | Covered by | Status |
|-----|-------------|------------|--------|
| QA-01 | Unit tests for resolver mime-matching rules (wildcard, exact, absent filter) | `src/capabilities/resolver.test.ts` (9 tests: empty array, action mismatch, absent mime, empty string mime, intent `*/*`, cap `*/*`, exact match, exact mismatch, mixed list) | ✓ |
| QA-02 | Unit tests for `planCast()` discriminated union branches | `src/intent/cast.test.ts` (8 tests: no-match, direct, reference equality, select-2, select-3, select preserves order, compile smoke, select reference equality) | ✓ |
| QA-03 | Unit tests for UUID generation and the `getRandomValues` fallback path | `src/intent/id.test.ts` (3 tests: randomUUID happy path, getRandomValues fallback, uniqueness across calls — fallback branch exercised) | ✓ |
| QA-04 | Unit tests for WebSocket backoff + `destroyed` guard | `src/capabilities/ws-listener.test.ts` (11 tests: deriveWsUrl, start constructs socket, REGISTRY_UPDATED, non-matching message, malformed JSON, onopen resets counter, reconnect backoff, stop during retry, exhausted retries fires onError, WS-07 non-wss rejection) | ✓ |
| QA-05 | Unit tests for modal focus trap, ESC key, backdrop click | `src/ui/focus-trap.test.ts` (5 tests: Tab wrap, Shift+Tab wrap, release removes listener, no focusables, auto-focus via rAF) + `src/ui/modal.test.ts` (14 tests including ESC key, backdrop click, cancel button, focus restore) | ✓ |
| QA-06 | Integration test (happy-dom + `MockBridge`): full `castIntent` happy path | `src/client.test.ts` — "happy-path: one match → direct iframe → resolve routes to callback" | ✓ |
| QA-07 | Integration test: two concurrent sessions route results to the correct callbacks | `src/client.test.ts` — "two concurrent sessions route resolve to the correct callback by id" | ✓ |
| QA-08 | Integration test: `destroy()` leaves zero listeners, closed WS, zero OBC DOM nodes | `src/client.test.ts` — "destroy during active session cancels callback and removes DOM nodes", "destroy removes all [data-obc-host] nodes from document.body", "destroy stops WsListener when one is active", "destroy aborts in-flight fetch (signal.aborted is true)" | ✓ |
| QA-09 | Integration test: same-origin capability path rejected at cast time | `src/client.test.ts` — "same-origin capability: castIntent fires cancel callback and SAME_ORIGIN_CAPABILITY error" (asserts `onError` called with `OBCError { code: 'SAME_ORIGIN_CAPABILITY' }` and callback with `{ status: 'cancel' }`) | ✓ |
| QA-10 | `@arethetypeswrong/cli --pack` passes in CI before publish | `package.json` `scripts.ci` runs `pnpm dlx @arethetypeswrong/cli --pack` as the final step; verified clean in Task 5 phase gate | ✓ |

**Result:** 10/10 QA requirements covered. All 147 tests pass (`pnpm run ci` exits 0).
