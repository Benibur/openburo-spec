# Requirements: OpenBuroClient (OBC)

**Defined:** 2026-04-10
**Core Value:** A host app can call `obc.castIntent(intent, cb)` once and get a fully orchestrated file-picker / file-save flow — capability discovery, user selection, sandboxed iframe lifecycle, and PostMessage round-trip — with zero framework lock-in.

## v1 Requirements

Requirements for initial release. Each maps to a roadmap phase.
Requirements flagged **[research]** were added after the research phase surfaced gaps in the original spec.

### Foundations

- [x] **FOUND-01**: Project scaffolds with tsdown, TypeScript 6, Vitest 4, Biome, npm package name `@openburo/client`
- [x] **FOUND-02**: `OBCError` class exported with code + message + optional cause
- [x] **FOUND-03**: `OBCErrorCode` union type covers `CAPABILITIES_FETCH_FAILED`, `NO_MATCHING_CAPABILITY`, `IFRAME_TIMEOUT`, `WS_CONNECTION_FAILED`, `INTENT_CANCELLED`, and `SAME_ORIGIN_CAPABILITY` **[research]**
- [x] **FOUND-04**: Shared types exported: `Capability`, `IntentRequest`, `IntentResult`, `FileResult`, `IntentCallback`, `OBCOptions`, `CastPlan`
- [x] **FOUND-05**: `generateSessionId()` uses `crypto.randomUUID()` with inline `getRandomValues` fallback to cover Chrome 90 / Firefox 88 / Safari 14 floor **[research]**
- [x] **FOUND-06**: Penpal pinned to exact version (no `^` range) in `package.json` **[research]**
- [x] **FOUND-07**: `@arethetypeswrong/cli` runs as CI gate on every build **[research]**

### Capability Loading

- [x] **CAP-01**: HTTP loader fetches capabilities from `options.capabilitiesUrl` and returns `Capability[]`
- [x] **CAP-02**: Loader detects OpenBuro Server via `X-OpenBuro-Server: true` response header
- [x] **CAP-03**: Loader errors surface as `OBCError { code: 'CAPABILITIES_FETCH_FAILED', cause }` through `onError` and a rejected `refreshCapabilities()` Promise
- [x] **CAP-04**: Loader accepts `AbortController.signal` so in-flight requests are cancellable from `destroy()` **[research]**
- [x] **CAP-05**: Loader refuses non-HTTPS `capabilitiesUrl` at runtime with a clear error (mixed-content guard) **[research]**
- [x] **CAP-06**: `getCapabilities()` returns the in-memory capability list synchronously
- [x] **CAP-07**: `refreshCapabilities()` forces an HTTP reload and returns the new list

### Capability Resolution

- [x] **RES-01**: Resolver is a pure function: `(capabilities, intent) => Capability[]`
- [x] **RES-02**: Match rule: `C.action === I.action`
- [x] **RES-03**: Match rule: empty/absent `I.args.allowedMimeType` matches all
- [x] **RES-04**: Match rule: `C.properties.mimeTypes` contains `*/*` matches any mime
- [x] **RES-05**: Match rule: exact mime string match
- [x] **RES-06**: Match rule: `I.args.allowedMimeType === '*/*'` matches any capability
- [x] **RES-07**: Pure `planCast()` function returns `CastPlan` discriminated union (`no-match` | `direct` | `select`) — unit-testable without DOM **[research]**

### WebSocket Live Updates

- [x] **WS-01**: When `liveUpdates` is enabled (default true when server header detected), open WebSocket to `wsUrl`
- [x] **WS-02**: `wsUrl` auto-derived from `capabilitiesUrl` if not provided (replace `/capabilities` with `/capabilities/ws`)
- [x] **WS-03**: On `REGISTRY_UPDATED` event, trigger `refreshCapabilities()` and invoke `onCapabilitiesUpdated`
- [x] **WS-04**: Full-jitter exponential backoff: 1s → 2s → 4s → 8s → 30s cap, max 5 attempts (configurable) **[research: jitter added]**
- [x] **WS-05**: Internal `destroyed` flag prevents post-`destroy()` reconnect timers from opening new sockets **[research]**
- [x] **WS-06**: WS failure after exhausted retries surfaces `OBCError { code: 'WS_CONNECTION_FAILED' }`
- [x] **WS-07**: Non-`wss://` protocol rejected at runtime to match HTTPS mixed-content policy **[research]**

### Intent Orchestration

- [x] **INT-01**: `castIntent(intent, callback)` returns a Promise and fires the callback exactly once per session
- [x] **INT-02**: Zero matches → callback `{ status: 'cancel', id, results: [] }` + `onError({ code: 'NO_MATCHING_CAPABILITY' })`
- [x] **INT-03**: One match → open iframe directly, skip modal
- [x] **INT-04**: Multiple matches → show chooser modal
- [x] **INT-05**: Each session generates a unique UUID v4 and is tracked in an internal `Map<string, ActiveSession>`
- [x] **INT-06**: Multiple concurrent sessions are isolated — results route to the correct callback even with overlapping iframes
- [x] **INT-07**: Modal cancel button returns `{ status: 'cancel', id, results: [] }`
- [x] **INT-08**: Timeout watchdog (default 5 min, configurable) closes the iframe and emits `OBCError { code: 'IFRAME_TIMEOUT' }`
- [x] **INT-09**: Messages with unknown/stale session `id` are silently ignored **[research]**

### Iframe Lifecycle

- [x] **IFR-01**: Iframe injected into `options.container` (default `document.body`) inside a backdrop element
- [x] **IFR-02**: Backdrop + iframe use configurable z-index (defaults 9000/9001)
- [x] **IFR-03**: Query params passed to iframe URL: `clientUrl`, `id`, `type`, `allowedMimeType`, `multiple`
- [x] **IFR-04**: Sandbox attribute set to `allow-scripts allow-same-origin allow-forms allow-popups`
- [x] **IFR-05**: `allow="clipboard-read; clipboard-write"`
- [x] **IFR-06**: Iframe `title` attribute set from `capability.appName` (WCAG 2.4.1, ACT cae760) **[research]**
- [x] **IFR-07**: Centered responsive styles: `min(90vw, 800px) × min(85vh, 600px)`, `border-radius: 8px`, subtle shadow
- [x] **IFR-08**: Constructor throws `OBCError { code: 'SAME_ORIGIN_CAPABILITY' }` if `new URL(capability.path).origin === location.origin` to prevent sandbox escape **[research]**
- [x] **IFR-09**: Loading indicator overlay shown inside host backdrop until Penpal handshake completes **[research]**
- [x] **IFR-10**: Body scroll lock while iframe/modal is open; restored on every close path **[research]**

### Chooser Modal & Accessibility

- [x] **UI-01**: Modal lists matching capabilities with `appName` + optional icon
- [x] **UI-02**: Modal has a visible cancel button
- [x] **UI-03**: Root element has `role="dialog"` + `aria-modal="true"` + `aria-labelledby` pointing at a title node (WCAG 4.1.2) **[research]**
- [x] **UI-04**: ESC key dismisses modal → `cancel` callback **[research]**
- [x] **UI-05**: Backdrop click dismisses modal → `cancel` callback **[research]**
- [x] **UI-06**: Focus trap confines keyboard focus inside modal while open (WCAG 2.1.2) **[research]**
- [x] **UI-07**: Focus restored to the element that triggered `castIntent` on close (WCAG 2.4.3) **[research]**
- [x] **UI-08**: Shadow DOM (`attachShadow({ mode: 'open' })`) isolates modal styles from host CSS **[research: mode specified]**
- [x] **UI-09**: CSS reset inside Shadow DOM prevents host style inheritance leaks
- [x] **UI-10**: Capability metadata rendered via DOM APIs (`textContent`), never `innerHTML` **[research]**
- [x] **UI-11**: No external CSS dependencies

### Messaging Layer (Penpal)

- [x] **MSG-01**: `BridgeAdapter` interface defined; Penpal is imported only inside `messaging/penpal-bridge.ts` **[research]**
- [x] **MSG-02**: `PenpalBridge` uses Penpal v7 API: `connect({ messenger: new WindowMessenger({ remoteWindow, allowedOrigins }), methods })` **[research: v7 API]**
- [x] **MSG-03**: `allowedOrigins` restricted to `[new URL(capability.path).origin]` per session
- [x] **MSG-04**: Parent exposes a `resolve(result: IntentResult)` method bound to the specific session id via closure
- [x] **MSG-05**: `MockBridge` available for unit tests so orchestrator tests run without real iframes **[research]**
- [x] **MSG-06**: Connection torn down on iframe close, timeout, `destroy()`, and session resolve

### OpenBuroClient Orchestrator

- [x] **ORCH-01**: `new OpenBuroClient(options)` constructs an instance; constructor validates `capabilitiesUrl` is `https://`
- [x] **ORCH-02**: Constructor performs no async side effects; capability fetch happens on first use or explicit `refreshCapabilities()`
- [x] **ORCH-03**: Multiple OBC instances can exist simultaneously without cross-talk
- [x] **ORCH-04**: `destroy()` aborts all in-flight fetches, closes the WebSocket, tears down every Penpal connection, removes every injected DOM element, restores body scroll, and nulls session state
- [x] **ORCH-05**: `destroy()` uses `AbortController` as the single teardown mechanism; all listeners attach with `{ signal }` **[research]**
- [x] **ORCH-06**: After `destroy()`, calls to public methods throw or no-op predictably (no silent failure)

### Packaging & Distribution

- [ ] **PKG-01**: ESM build output `dist/obc.esm.js`
- [ ] **PKG-02**: CommonJS build output `dist/obc.cjs.js`
- [ ] **PKG-03**: UMD build output `dist/obc.umd.js` — importable via `<script>` from a CDN
- [ ] **PKG-04**: TypeScript declarations published under `types/index.d.ts`
- [ ] **PKG-05**: `package.json` `exports` map has nested `types` per condition (import/require) **[research]**
- [ ] **PKG-06**: No global `window` pollution; UMD opt-in via `window.OpenBuroClient` only when used as `<script>`
- [ ] **PKG-07**: Target: ES2020+; verified for Chrome 90+, Firefox 88+, Safari 14+
- [ ] **PKG-08**: Capability-author integration guide documents required Penpal v7 compatibility **[research]**

### Quality Gates

- [ ] **QA-01**: Unit tests for resolver mime-matching rules (wildcard, exact, absent filter)
- [ ] **QA-02**: Unit tests for `planCast()` discriminated union branches
- [ ] **QA-03**: Unit tests for UUID generation and the `getRandomValues` fallback path
- [ ] **QA-04**: Unit tests for WebSocket backoff + `destroyed` guard
- [ ] **QA-05**: Unit tests for modal focus trap, ESC key, backdrop click
- [ ] **QA-06**: Integration test (happy-dom + `MockBridge`): full `castIntent` happy path
- [ ] **QA-07**: Integration test: two concurrent sessions route results to the correct callbacks
- [ ] **QA-08**: Integration test: `destroy()` leaves zero listeners, closed WS, zero OBC DOM nodes
- [ ] **QA-09**: Integration test: same-origin capability path rejected at cast time
- [ ] **QA-10**: `@arethetypeswrong/cli --pack` passes in CI before publish

## v2 Requirements

Deferred to future releases. Tracked but not in current roadmap.

### Framework Bindings

- **FWK-01**: `@openburo/react` hook wrapper
- **FWK-02**: `@openburo/vue` composable wrapper

### Advanced Messaging

- **MSG2-01**: Parent-to-iframe method calls beyond `resolve` (v2 extension surface)
- **MSG2-02**: `intent:resize` dynamic iframe sizing protocol
- **MSG2-03**: Penpal version-negotiation ping (detect v6 child, emit clearer error)

### UX

- **UX2-01**: CSS custom-properties theming inside Shadow DOM
- **UX2-02**: Capability icon lazy-loading and fallback
- **UX2-03**: Recent / preferred capability ordering in chooser

### Infrastructure

- **INF2-01**: Capability manifest schema versioning (`schemaVersion` field)
- **INF2-02**: Capability caching / offline mode
- **INF2-03**: Mobile-native SDKs

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| React/Vue/Angular bindings in core | Library must be framework-agnostic; bindings ship as separate packages in v2 |
| Custom modal theming API | Fixed styles in v1; Shadow DOM default only |
| OAuth / SSO between host and capability | The iframe handles its own auth; OBC is a broker, not an auth gateway |
| Global `window.OpenBuro` singleton | Causes multi-instance conflicts; UMD `window.OpenBuroClient` is the only opt-in touchpoint |
| Non-HTTPS `capabilitiesUrl` in production | Mixed Content is the host app's problem; rejected at runtime |
| Capability caching / offline | Capabilities are always live; caching is v2 |
| tsup build tooling | Deprecated in 2026; tsdown is the successor |
| Penpal v6 `connectToChild` API | Spec pseudocode is obsolete; v7 is the only supported version |
| Vendored Penpal source (decision: pin exact version instead) | Pinning is enough for v1; vendor only if supply-chain posture tightens |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| FOUND-01 | Phase 1 | Complete |
| FOUND-02 | Phase 1 | Complete |
| FOUND-03 | Phase 1 | Complete |
| FOUND-04 | Phase 1 | Complete |
| FOUND-05 | Phase 1 | Complete |
| FOUND-06 | Phase 1 | Complete |
| FOUND-07 | Phase 1 | Complete |
| CAP-01 | Phase 2 | Complete |
| CAP-02 | Phase 2 | Complete |
| CAP-03 | Phase 2 | Complete |
| CAP-04 | Phase 2 | Complete |
| CAP-05 | Phase 2 | Complete |
| CAP-06 | Phase 2 | Complete |
| CAP-07 | Phase 2 | Complete |
| RES-01 | Phase 2 | Complete |
| RES-02 | Phase 2 | Complete |
| RES-03 | Phase 2 | Complete |
| RES-04 | Phase 2 | Complete |
| RES-05 | Phase 2 | Complete |
| RES-06 | Phase 2 | Complete |
| RES-07 | Phase 2 | Complete |
| WS-01 | Phase 2 | Complete |
| WS-02 | Phase 2 | Complete |
| WS-03 | Phase 2 | Complete |
| WS-04 | Phase 2 | Complete |
| WS-05 | Phase 2 | Complete |
| WS-06 | Phase 2 | Complete |
| WS-07 | Phase 2 | Complete |
| INT-01 | Phase 2 | Complete |
| INT-02 | Phase 2 | Complete |
| INT-03 | Phase 2 | Complete |
| INT-04 | Phase 2 | Complete |
| INT-05 | Phase 2 | Complete |
| INT-06 | Phase 2 | Complete |
| INT-07 | Phase 2 | Complete |
| INT-08 | Phase 2 | Complete |
| INT-09 | Phase 2 | Complete |
| IFR-01 | Phase 2 | Complete |
| IFR-02 | Phase 2 | Complete |
| IFR-03 | Phase 2 | Complete |
| IFR-04 | Phase 2 | Complete |
| IFR-05 | Phase 2 | Complete |
| IFR-06 | Phase 2 | Complete |
| IFR-07 | Phase 2 | Complete |
| IFR-08 | Phase 2 | Complete |
| IFR-09 | Phase 2 | Complete |
| IFR-10 | Phase 2 | Complete |
| UI-01 | Phase 2 | Complete |
| UI-02 | Phase 2 | Complete |
| UI-03 | Phase 2 | Complete |
| UI-04 | Phase 2 | Complete |
| UI-05 | Phase 2 | Complete |
| UI-06 | Phase 2 | Complete |
| UI-07 | Phase 2 | Complete |
| UI-08 | Phase 2 | Complete |
| UI-09 | Phase 2 | Complete |
| UI-10 | Phase 2 | Complete |
| UI-11 | Phase 2 | Complete |
| MSG-01 | Phase 2 | Complete |
| MSG-02 | Phase 2 | Complete |
| MSG-03 | Phase 2 | Complete |
| MSG-04 | Phase 2 | Complete |
| MSG-05 | Phase 2 | Complete |
| MSG-06 | Phase 2 | Complete |
| ORCH-01 | Phase 3 | Complete |
| ORCH-02 | Phase 3 | Complete |
| ORCH-03 | Phase 3 | Complete |
| ORCH-04 | Phase 3 | Complete |
| ORCH-05 | Phase 3 | Complete |
| ORCH-06 | Phase 3 | Complete |
| PKG-01 | Phase 4 | Pending |
| PKG-02 | Phase 4 | Pending |
| PKG-03 | Phase 4 | Pending |
| PKG-04 | Phase 4 | Pending |
| PKG-05 | Phase 4 | Pending |
| PKG-06 | Phase 4 | Pending |
| PKG-07 | Phase 4 | Pending |
| PKG-08 | Phase 4 | Pending |
| QA-01 | Phase 4 | Pending |
| QA-02 | Phase 4 | Pending |
| QA-03 | Phase 4 | Pending |
| QA-04 | Phase 4 | Pending |
| QA-05 | Phase 4 | Pending |
| QA-06 | Phase 4 | Pending |
| QA-07 | Phase 4 | Pending |
| QA-08 | Phase 4 | Pending |
| QA-09 | Phase 4 | Pending |
| QA-10 | Phase 4 | Pending |

**Coverage:**
- v1 requirements listed: 88 total (71 base + 17 [research] additions folded in during research phase)
- Mapped to phases: 88/88
- Unmapped: 0

---
*Requirements defined: 2026-04-10*
*Last updated: 2026-04-10 — traceability populated by roadmapper*
