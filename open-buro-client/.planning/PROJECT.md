# OpenBuroClient (OBC)

## What This Is

OpenBuroClient is a framework-agnostic vanilla JavaScript library that lets host applications resolve "intents" (PICK / SAVE files) by discovering available OpenBuro capabilities, letting the user choose one when multiple match, and orchestrating a sandboxed iframe session that round-trips the result back to the caller. It is the browser-side companion to the OpenBuro Server (Go) already in this repo.

## Core Value

A host app can write `obc.castIntent(intent, cb)` once and get a fully orchestrated file-picker / file-save flow — capability discovery, user selection, iframe lifecycle, and PostMessage round-trip — with zero framework lock-in.

## Requirements

### Validated

<!-- Shipped and confirmed valuable. -->

**Foundations (Phase 1)** — tooling, types, and session-id utility in place; other phases can build on them.
- ✓ Scaffold: tsdown + TS 6 + Vitest 4 + Biome 2.4, pnpm workspace — validated in Phase 1
- ✓ `OBCError` + all six `OBCErrorCode` values (incl. `SAME_ORIGIN_CAPABILITY`) — validated in Phase 1
- ✓ Shared types exported from barrel (`Capability`, `IntentRequest`, `IntentResult`, `FileResult`, `IntentCallback`, `OBCOptions`, `CastPlan`) — validated in Phase 1
- ✓ `generateSessionId()` with tested `getRandomValues` fallback — validated in Phase 1
- ✓ `penpal@7.0.6` pinned exact (no caret) — validated in Phase 1
- ✓ `@arethetypeswrong/cli --pack` CI gate green across node10/node16 CJS+ESM/bundler — validated in Phase 1

**Core Implementation (Phase 2)** — every independent layer built, unit-tested, composable by Phase 3. 112 tests across 15 files.
- ✓ Capability layer: pure MIME resolver (9 tests), HTTPS-guarded HTTP loader with AbortSignal (6 tests), WebSocket listener with full-jitter backoff + `destroyed` flag guard (11 tests) — validated in Phase 2
- ✓ Lifecycle: `createAbortContext()` helper for leak-free teardown composition (6 tests) — validated in Phase 2
- ✓ Intent logic: pure `planCast()` discriminated union with `noUncheckedIndexedAccess` guard (8 tests), `ActiveSession` type for Phase 3 session map — validated in Phase 2
- ✓ UI layer: Shadow DOM host (z-index on light DOM), iframe factory with same-origin guard + WCAG `title` + sandbox, Shadow-DOM-aware focus trap, chooser modal with full a11y baseline (role/aria, ESC, backdrop, arrow-keys/Home/End/Enter, focus restore, body scroll lock, 150ms spinner delay) — 41 tests — validated in Phase 2
- ✓ Messaging layer: `BridgeAdapter` interface, `MockBridge` test double, `PenpalBridge` using verified Penpal v7 `connect({ messenger: new WindowMessenger(...) })` API — 15 tests, Penpal imported from exactly one file — validated in Phase 2

**Orchestration (Phase 3)** — public API working end-to-end. 147 tests total, `pnpm run ci` green.
- ✓ `OpenBuroClient` facade class — the only file in the codebase permitted to cross layer boundaries — validated in Phase 3
- ✓ Constructor validation (https/wss + localhost allow); zero async side-effects — validated in Phase 3
- ✓ Lazy capability fetch with single-flight promise cache; WS listener wired on first successful fetch — validated in Phase 3
- ✓ `castIntent()` full orchestration: lazy fetch → planCast → modal/direct → iframe → bridge → session map + watchdog — validated in Phase 3
- ✓ Leak-free `destroy()` via `createAbortContext()`: idempotent, cancels all active sessions, tears down DOM/WS/bridge — validated in Phase 3
- ✓ Post-destroy semantics: `castIntent` rejects with `DESTROYED`, `getCapabilities` returns `[]`, `refreshCapabilities` rejects — validated in Phase 3
- ✓ Two concurrent `OpenBuroClient` instances verified isolated (no shared state) — validated in Phase 3
- ✓ `DESTROYED` added to `OBCErrorCode` union — validated in Phase 3

### Active

<!-- Current scope. Building toward these. -->

**API surface**
- [ ] Public `OpenBuroClient` class constructed with `OBCOptions` (capabilitiesUrl, liveUpdates, wsUrl, container, onCapabilitiesUpdated, onError)
- [ ] `castIntent(intent, callback)` orchestrates the full intent lifecycle and returns a Promise
- [ ] `getCapabilities()` returns the in-memory capability list synchronously
- [ ] `refreshCapabilities()` forces an HTTP reload from `capabilitiesUrl`
- [ ] `destroy()` releases WS, listeners, iframes, and injected DOM with no leaks
- [ ] Multiple concurrent `OpenBuroClient` instances work side-by-side

**Capability loading & resolution**
- [ ] HTTP loader fetches capabilities from `capabilitiesUrl` and handles errors via `onError` / `OBCError`
- [ ] Resolver matches an `IntentRequest` against capabilities by `action` + mime rules (wildcard `*/*`, exact match, absent filter)
- [ ] Detection of OpenBuro Server mode via `X-OpenBuro-Server: true` header (or explicit `liveUpdates: true`)
- [ ] WebSocket listener subscribes to `wsUrl` (auto-derived from `capabilitiesUrl` or explicit) and refreshes on `REGISTRY_UPDATED`
- [ ] WebSocket exponential backoff (1s→2s→4s→8s→30s cap, 5 attempts configurable)

**Intent orchestration**
- [ ] 0 matches → callback with `{ status: 'cancel', results: [] }` and `NO_MATCHING_CAPABILITY` error surfaced via `onError`
- [ ] 1 match → skip modal, open iframe directly
- [ ] N matches → show selection modal with `appName` + optional icon; cancel button returns `cancel`
- [ ] Unique session `id` generated per `castIntent` (UUID v4)
- [ ] Multiple concurrent `castIntent` sessions isolated by id

**Iframe lifecycle**
- [ ] Iframe injected into `options.container` inside a backdrop with configurable z-index (default 9000/9001)
- [ ] Query params `clientUrl`, `id`, `type`, `allowedMimeType`, `multiple` passed to iframe URL
- [ ] Iframe sandbox: `allow-scripts allow-same-origin allow-forms allow-popups`; `allow="clipboard-read; clipboard-write"`
- [ ] Centered responsive styling (`min(90vw, 800px) × min(85vh, 600px)`)
- [ ] Timeout (default 5 min, configurable) closes connection cleanly with `IFRAME_TIMEOUT`

**PostMessage messaging (Penpal)**
- [ ] Parent side uses Penpal `connectToChild` with `allowedOrigins` restricted to `capability.path` origin
- [ ] Parent exposes `resolve(result: IntentResult)` that the iframe calls on done/cancel
- [ ] Messages with unknown/stale session `id` are ignored
- [ ] Connection torn down cleanly on iframe close or `destroy()`

**UI isolation**
- [ ] Modal + backdrop styles injected via Shadow DOM (preferred) or heavily prefixed `.obc-*` scoped `<style>`
- [ ] Zero CSS bleed into host application
- [ ] No external CSS dependencies

**Error model**
- [ ] `OBCError { code, message, cause? }` with codes: `CAPABILITIES_FETCH_FAILED`, `NO_MATCHING_CAPABILITY`, `IFRAME_TIMEOUT`, `WS_CONNECTION_FAILED`, `INTENT_CANCELLED`
- [ ] Errors both invoked via `onError` and propagated through `castIntent` Promise rejection

**Packaging & distribution**
- [ ] Tree-shakable ESM build (`obc.esm.js`)
- [ ] CommonJS build (`obc.cjs.js`)
- [ ] UMD build for CDN usage (`obc.umd.js`)
- [ ] TypeScript type declarations published (`types/index.d.ts`)
- [ ] No global `window` pollution unless opt-in
- [ ] Target: ES2020+, Chrome 90+ / Firefox 88+ / Safari 14+
- [ ] Published as `@openburo/client` on npm

**Quality**
- [ ] Unit tests for resolver matching logic (mime wildcard rules)
- [ ] Unit tests for UUID generation and session isolation
- [ ] Integration test for full `castIntent` happy path with mock iframe
- [ ] Integration test for `destroy()` leak-free teardown

### Out of Scope

- **Framework bindings (React/Vue/Angular wrappers)** — library is vanilla JS; bindings can be published separately later
- **Parent-exposed methods beyond `resolve`** — v2 concern, called out in spec
- **Mobile-native SDKs** — web-only for v1
- **Custom modal theming API** — styles are fixed in v1; consumers can't override beyond Shadow DOM defaults
- **Authentication / authorization between host and capability iframe** — assumed handled by the iframe itself
- **Capability caching / offline mode** — capabilities always fetched live
- **Non-HTTPS `capabilitiesUrl` support in production** — Mixed Content is the host app's problem to avoid

## Context

- **Part of the OpenBuro ecosystem.** A sibling Go server (`../open-buro-server/`) already exists; it exposes `GET /api/v1/capabilities` with the `X-OpenBuro-Server: true` header and a `/capabilities/ws` WebSocket for live registry updates. This library is the browser-side counterpart.
- **Hackathon demo + long-term reference.** The first consumer is a hackathon showcase of the OpenBuro intent/capability flow, but the code is intended to become the canonical client library that third-party host apps adopt afterwards — so code quality, API stability, and docs matter from day one.
- **Domain reference:** `../docs/` contains the ecosystem overview, comparatives, and the full technical dossier (`open-buro-dossier-technique-file-picker.md`) that this library implements.
- **Existing empty scaffold:** `open-buro-client/` is empty — greenfield TypeScript library project.
- **PostMessage library decision already made:** Penpal is selected over post-robot (less maintained, heavier) and Comlink (too abstract, less iframe-lifecycle control). Rationale in spec.

## Constraints

- **Tech stack**: Vanilla TypeScript compiled to ES2020+ — no React/Vue/Angular dependency, no build-time framework assumptions. Why: library must work in any host environment.
- **Dependencies**: Penpal is the only runtime dependency allowed by default. Why: minimize bundle size and supply chain surface for a library that ships to third-party apps.
- **Bundle format**: Must ship ESM + CJS + UMD. Why: support bundler imports, Node tooling, and CDN `<script>` usage simultaneously.
- **CSS isolation**: Must not leak styles into the host page. Why: library is injected into unknown host pages and cannot assume any CSS reset.
- **Browser support**: Chrome 90+ / Firefox 88+ / Safari 14+. Why: ES2020 baseline, no transpilation needed for these targets.
- **Security**: Penpal `allowedOrigins` must be restricted to `capability.path` origin; iframe must be sandboxed. Why: capabilities are third-party — host cannot trust them.
- **Lifecycle**: `destroy()` is mandatory and must leave zero listeners/DOM/WS behind. Why: embedded in SPAs where hot-reload and route changes are common.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Use Penpal for parent↔iframe messaging | Purpose-built for parent↔iframe, Promise-native, origin-restricted, clean teardown — beats post-robot (heavier, less maintained) and Comlink (too abstract) | — Pending |
| Vanilla TS library, no framework bindings in v1 | Maximize reach; bindings are trivial to add later as separate packages | — Pending |
| Shadow DOM for modal styling | Strongest isolation guarantee against host CSS | — Pending |
| Scaffold inside `open-buro-client/`, separate from `open-buro-server/` | Clear separation of concerns; different tech stacks (TS vs Go) | — Pending |
| Dual-audience build: hackathon demo first, reference lib second | User confirmed during init. Implication: prioritize a working slice for the demo, but never cut corners on public API shape, types, or docs since the same code becomes the reference | — Pending |
| UUID v4 for session ids | Stateless, collision-free, no coordination with server required | — Pending |
| Library package name: `@openburo/client` | Matches ecosystem branding; scoped npm package | — Pending |

---
*Last updated: 2026-04-10 after Phase 3 (Orchestration) completion*
