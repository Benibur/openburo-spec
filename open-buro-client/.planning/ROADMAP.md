# Roadmap: OpenBuroClient (@openburo/client)

## Overview

Four phases take a greenfield TypeScript directory to a published, fully-tested intent-brokering library. Phase 1 locks shared types and tooling so Phase 2 can build all independent implementation layers in parallel (capability loading, resolution, WebSocket, intent orchestration, iframe lifecycle, chooser modal, messaging). Phase 3 wires those layers into the `OpenBuroClient` facade. Phase 4 validates the integrated system and ships the three-format build to npm.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 1: Foundations** - Tooling scaffold, shared types, error model, and session-id utility — everything else depends on this (completed 2026-04-10)
- [ ] **Phase 2: Core Implementation** - All independent layers built in parallel: capability loading/resolution/WS, intent orchestration, iframe lifecycle, chooser modal/a11y, and Penpal messaging bridge
- [ ] **Phase 3: Orchestration** - `OpenBuroClient` facade wires Phase 2 layers into the public API with lifecycle management and session isolation
- [ ] **Phase 4: Distribution & Quality** - Build outputs validated, exports map verified, integration tests pass, library published as `@openburo/client`

## Phase Details

### Phase 1: Foundations
**Goal**: Developers (and Claude) can write and test typed OBC code — the project scaffold, shared type contracts, error model, and session-id utility are all in place
**Depends on**: Nothing (first phase)
**Requirements**: FOUND-01, FOUND-02, FOUND-03, FOUND-04, FOUND-05, FOUND-06, FOUND-07
**Success Criteria** (what must be TRUE):
  1. `npm run build` and `npm test` complete without errors on an empty `src/` — tooling (tsdown, Vitest 4, Biome, TypeScript 6) is wired correctly
  2. `import { OBCError, OBCErrorCode } from '@openburo/client'` resolves all six error codes at compile time with no type errors
  3. All shared interfaces (`Capability`, `IntentRequest`, `IntentResult`, `OBCOptions`, `CastPlan`) are exported and type-check in an isolated consumer file
  4. `generateSessionId()` returns a valid UUID v4 string when called in an environment that lacks `crypto.randomUUID()` (verified via Vitest test)
  5. `@arethetypeswrong/cli --pack` exits 0 in CI, and Penpal is pinned to an exact version in `package.json`
**Plans**: 1 plan

Plans:
- [ ] 01-01-SCAFFOLD-PLAN.md — Scaffold config files, source (errors/types/id/index), tests; verify build + attw gate green

### Phase 2: Core Implementation
**Goal**: Every independent implementation layer exists, is unit-tested, and can be composed by Phase 3 — capability loading, MIME resolution, WebSocket live updates, intent orchestration logic, iframe DOM factory, chooser modal with full a11y, and the Penpal messaging bridge are all complete
**Depends on**: Phase 1
**Requirements**: CAP-01, CAP-02, CAP-03, CAP-04, CAP-05, CAP-06, CAP-07, RES-01, RES-02, RES-03, RES-04, RES-05, RES-06, RES-07, WS-01, WS-02, WS-03, WS-04, WS-05, WS-06, WS-07, INT-01, INT-02, INT-03, INT-04, INT-05, INT-06, INT-07, INT-08, INT-09, IFR-01, IFR-02, IFR-03, IFR-04, IFR-05, IFR-06, IFR-07, IFR-08, IFR-09, IFR-10, UI-01, UI-02, UI-03, UI-04, UI-05, UI-06, UI-07, UI-08, UI-09, UI-10, UI-11, MSG-01, MSG-02, MSG-03, MSG-04, MSG-05, MSG-06
**Success Criteria** (what must be TRUE):
  1. `planCast(capabilities, intent)` returns the correct discriminated union branch (`no-match`, `direct`, or `select`) for all MIME wildcard combinations — verified by unit tests with zero DOM or network setup
  2. The capability HTTP loader cancels its in-flight fetch when the supplied `AbortSignal` fires, and rejects with `CAPABILITIES_FETCH_FAILED` on non-HTTPS URLs
  3. The WebSocket listener reconnects with full-jitter exponential backoff and never opens a new socket after `stop()` is called, even if a retry timer was already scheduled — verified by unit test using a fake timer
  4. A chooser modal rendered in a happy-dom document traps keyboard focus inside the Shadow DOM, dismisses on ESC or backdrop click, and restores focus to the trigger element on close — verified by unit tests without Penpal
  5. Calling `PenpalBridge.connect()` with a same-origin `remoteWindow` resolves a `ConnectionHandle`; calling it with `allowedOrigins` set to a mismatched origin causes the Penpal handshake to reject — verified via `MockBridge`
**Plans**: 5 plans

Plans:
- [ ] 02-01-capabilities-PLAN.md — Pure MIME resolver, HTTPS-guarded HTTP loader with AbortSignal, WebSocket listener with full-jitter backoff + destroyed guard (Wave 1)
- [ ] 02-02-intent-PLAN.md — Pure planCast() discriminated union + ActiveSession type for Phase 3 session Map (Wave 1)
- [ ] 02-03-ui-PLAN.md — Shadow DOM host + CSS reset, iframe factory with same-origin guard, Shadow-DOM-aware focus trap, chooser modal with full a11y baseline (Wave 1)
- [ ] 02-04-messaging-PLAN.md — BridgeAdapter interface, MockBridge for tests, PenpalBridge via Penpal v7 connect + WindowMessenger (Wave 1)
- [ ] 02-05-integration-PLAN.md — Extend public barrel, verify layer isolation, full pnpm run ci gate (Wave 2)

### Phase 3: Orchestration
**Goal**: `new OpenBuroClient(options)` is a fully working public API — `castIntent`, `getCapabilities`, `refreshCapabilities`, and `destroy` all behave as specified, multiple concurrent instances do not interfere, and `destroy()` leaves zero leaks
**Depends on**: Phase 2
**Requirements**: ORCH-01, ORCH-02, ORCH-03, ORCH-04, ORCH-05, ORCH-06
**Success Criteria** (what must be TRUE):
  1. A host page can call `obc.castIntent(intent, cb)` against a mock capability server and receive the iframe result in the callback — the full round-trip works end-to-end in a browser-like environment
  2. Two `OpenBuroClient` instances created with different `capabilitiesUrl` values fetch independently and never share session state or capability lists
  3. After `obc.destroy()` returns, all injected DOM nodes are gone, the WebSocket is closed, and every AbortController-registered listener has fired — verified by the integration test asserting zero OBC artifacts remain
  4. Calling any public method after `destroy()` throws a predictable error or no-ops without silent failure
**Plans**: TBD

### Phase 4: Distribution & Quality
**Goal**: `@openburo/client` is ready to publish — ESM/CJS/UMD builds pass type validation, full integration tests cover happy path and edge cases, and the capability-author integration guide documents the Penpal v7 contract
**Depends on**: Phase 3
**Requirements**: PKG-01, PKG-02, PKG-03, PKG-04, PKG-05, PKG-06, PKG-07, PKG-08, QA-01, QA-02, QA-03, QA-04, QA-05, QA-06, QA-07, QA-08, QA-09, QA-10
**Success Criteria** (what must be TRUE):
  1. `attw --pack` exits 0 — no `CJSResolvesToESM` or `FallbackCondition` errors; `"types"` is nested correctly inside each `exports` condition
  2. The UMD build loads via a `<script>` tag in a static HTML page and exposes `window.OpenBuroClient` without polluting any other globals
  3. All four integration tests pass: happy-path `castIntent` with `MockBridge`, two concurrent sessions routing to correct callbacks, `destroy()` leaving zero artifacts, and same-origin capability rejection
  4. A capability author reading `PKG-08` integration guide knows exactly which Penpal version their iframe must use and what the `resolve(result)` method signature is
**Plans**: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Foundations | 1/1 | Complete    | 2026-04-10 |
| 2. Core Implementation | 4/5 | In Progress|  |
| 3. Orchestration | 0/TBD | Not started | - |
| 4. Distribution & Quality | 0/TBD | Not started | - |
