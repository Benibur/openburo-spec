# Phase 2: Core Implementation - Research

**Researched:** 2026-04-10
**Domain:** Vanilla TypeScript browser library — MIME resolver, WebSocket backoff, Shadow DOM modal (a11y), Penpal v7 bridge
**Confidence:** HIGH

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Chooser Modal Visual Defaults (user-confirmed)**
- Modal card: `max-width: 480px`, `min-width: 320px`, responsive
- Backdrop: `rgba(0, 0, 0, 0.5)`
- Accent color (primary action, focus ring): `#2563eb` (Tailwind blue-600)
- Loading spinner: pure CSS `@keyframes` border-rotate (no SVG, no emoji, no external font)

**Chooser Keyboard Interaction (user-confirmed)**
- Arrow keys (↑/↓) navigate capability items; `Home`/`End` jump to first/last
- `Enter` selects the focused item
- `Escape` dismisses → `cancel`
- Auto-focus the first capability item on open (focus trap pulls keyboard into modal)
- Single capability match → skip chooser entirely, open iframe directly (per IFR-/INT- requirements)
- Backdrop click dismisses → `cancel`
- Focus is restored to the element that held focus before `castIntent` on every close path

**Timing & Reconnect Knobs (user-confirmed)**
- Penpal handshake timeout: **10 seconds** (slow capability servers during a hackathon demo)
- Intent session watchdog timeout: **5 minutes** default (configurable via options — per INT-08)
- WebSocket max reconnect attempts: **5** before emitting `WS_CONNECTION_FAILED`
- Iframe loading spinner delay: **150 ms** (spinner appears only if handshake takes longer)

**Architecture (locked by research SUMMARY.md)**
- Layer strictness: orchestrator is the ONLY file that imports across layers. UI layer must not import Penpal. Messaging layer must not touch DOM. Capability layer must not touch DOM or Penpal.
- Session state lives exclusively on `OpenBuroClient` in a `Map<string, ActiveSession>` — no event bus. Phase 2 defines the `ActiveSession` type; Phase 3 holds the Map.
- `planCast()` is a pure function returning a discriminated union: `{ kind: 'no-match' } | { kind: 'direct'; capability } | { kind: 'select'; capabilities }`.
- `BridgeAdapter` interface + `PenpalBridge` implementation — Penpal is imported only from `src/messaging/penpal-bridge.ts`.
- `MockBridge` lives in `src/messaging/mock-bridge.ts` for unit tests.
- Shadow DOM mode: `attachShadow({ mode: 'open' })` (open, not closed).
- Same-origin capability guard: thrown before iframe creation, not lazily.
- AbortController pattern: every listener/fetch/WS registration uses `{ signal }` from a phase-2-provided helper so Phase 3 can call `controller.abort()` for leak-free `destroy()`.

**Test Environment (locked by Phase 1 choices)**
- Vitest 4 with Node environment for pure-logic modules (resolver, planCast, id, errors)
- Vitest 4 with **happy-dom** environment for UI layer (modal, iframe factory, Shadow DOM) and messaging (MockBridge usage)
- happy-dom added as devDependency in Phase 2 Wave 0 (not present yet)
- WebSocket mocking via a tiny in-test fake (no external msw-ws dependency unless necessary)

### Claude's Discretion
- Exact file split inside each module (e.g., whether to put focus-trap logic in `ui/focus-trap.ts` or inline in `ui/modal.ts`)
- Test naming convention (`*.test.ts` co-located)
- Internal type names that don't appear in the public API
- Whether to export `planCast` publicly or keep it internal (recommend internal for v1)
- Exact WebSocket event payload schema beyond `REGISTRY_UPDATED` — follow project-level research SUMMARY.md

### Deferred Ideas (OUT OF SCOPE)
- Parent-exposed methods beyond `resolve` (v2 — `MSG2-01`)
- `intent:resize` dynamic iframe sizing (v2 — `MSG2-02`)
- Capability icon lazy-loading (v2 — `UX2-02`)
- Recent/preferred capability ordering (v2 — `UX2-03`)
- CSS custom properties for theming (v2 — `UX2-01`)
- WebSocket auto-wiring into orchestrator options — that lives in Phase 2 scope, but the `liveUpdates: true` wire-up to the constructor happens in Phase 3 orchestrator; Phase 2 only ships the listener module
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| CAP-01 | HTTP loader fetches capabilities from `options.capabilitiesUrl`, returns `Capability[]` | Standard `fetch` + `AbortSignal`; `response.json()` pattern |
| CAP-02 | Loader detects OpenBuro Server via `X-OpenBuro-Server: true` response header | `response.headers.get('X-OpenBuro-Server')` |
| CAP-03 | Loader errors → `OBCError { code: 'CAPABILITIES_FETCH_FAILED', cause }` via `onError` | Existing `OBCError` class in `src/errors.ts` |
| CAP-04 | Loader accepts `AbortController.signal` for cancellable in-flight requests | `fetch(url, { signal })` pattern |
| CAP-05 | Non-HTTPS `capabilitiesUrl` rejected at runtime | URL protocol check before fetch |
| CAP-06 | `getCapabilities()` returns in-memory list synchronously | Simple getter, no I/O |
| CAP-07 | `refreshCapabilities()` forces HTTP reload, returns new list | Calls loader, updates internal array |
| RES-01 | Resolver is a pure function: `(capabilities, intent) => Capability[]` | No I/O, fully unit-testable |
| RES-02 | Match rule: `C.action === I.action` | Simple equality check |
| RES-03 | Match rule: empty/absent `I.args.allowedMimeType` matches all | Guard + early return |
| RES-04 | Match rule: `C.properties.mimeTypes` contains `*/*` matches any mime | `Array.includes('*/*')` |
| RES-05 | Match rule: exact mime string match | `Array.includes(allowedMimeType)` |
| RES-06 | Match rule: `I.args.allowedMimeType === '*/*'` matches any capability | Guard before mime-level checks |
| RES-07 | `planCast()` returns `CastPlan` discriminated union | Pure function, uses `CastPlan` type already in `src/types.ts` |
| WS-01 | Open WebSocket to `wsUrl` when `liveUpdates` enabled | Native `WebSocket` constructor |
| WS-02 | `wsUrl` auto-derived from `capabilitiesUrl` (replace `/capabilities` with `/capabilities/ws`) | URL manipulation |
| WS-03 | On `REGISTRY_UPDATED` event, trigger `refreshCapabilities()` and invoke `onCapabilitiesUpdated` | `WebSocket.onmessage` handler |
| WS-04 | Full-jitter exponential backoff: 1s→2s→4s→8s→30s cap, max 5 attempts | Jitter formula: `Math.random() * min(cap, base * 2^attempt)` |
| WS-05 | `destroyed` flag prevents post-`destroy()` reconnect | Boolean guard in reconnect callback |
| WS-06 | WS failure after exhausted retries → `OBCError { code: 'WS_CONNECTION_FAILED' }` | After attempt 5 |
| WS-07 | Non-`wss://` protocol rejected at runtime | URL protocol check |
| INT-01 | `castIntent(intent, callback)` returns Promise, fires callback exactly once | Promise + callback resolved in `onChildResolve` |
| INT-02 | Zero matches → callback `{ status: 'cancel', id, results: [] }` + `onError` | `planCast` 'no-match' branch |
| INT-03 | One match → open iframe directly, skip modal | `planCast` 'direct' branch |
| INT-04 | Multiple matches → show chooser modal | `planCast` 'select' branch |
| INT-05 | Each session generates a UUID v4, tracked in `Map<string, ActiveSession>` | `generateSessionId()` already in `src/intent/id.ts` |
| INT-06 | Multiple concurrent sessions isolated — results route to correct callback | Session Map keyed by id; `resolve` closes over `sessionId` |
| INT-07 | Modal cancel → `{ status: 'cancel', id, results: [] }` | `onCancel` callback in modal |
| INT-08 | Timeout watchdog (default 5 min) closes iframe, emits `OBCError { code: 'IFRAME_TIMEOUT' }` | `setTimeout` handle stored in `ActiveSession` |
| INT-09 | Messages with unknown/stale session `id` silently ignored | `sessions.has(id)` guard |
| IFR-01 | Iframe injected into `options.container` inside a backdrop element | DOM factory pattern |
| IFR-02 | Backdrop + iframe use configurable z-index (defaults 9000/9001) | CSS on shadow host in light DOM |
| IFR-03 | Query params: `clientUrl`, `id`, `type`, `allowedMimeType`, `multiple` | `URLSearchParams` |
| IFR-04 | Sandbox: `allow-scripts allow-same-origin allow-forms allow-popups` | Verified safe for cross-origin only |
| IFR-05 | `allow="clipboard-read; clipboard-write"` | `permissions` attribute |
| IFR-06 | `title` attribute from `capability.appName` (WCAG 2.4.1, ACT cae760) | One-liner: `iframe.title = capability.appName` |
| IFR-07 | Centered responsive styles: `min(90vw, 800px) × min(85vh, 600px)`, `border-radius: 8px` | CSS inside Shadow DOM |
| IFR-08 | Constructor throws `OBCError { code: 'SAME_ORIGIN_CAPABILITY' }` if same-origin | Pre-creation origin check |
| IFR-09 | Loading indicator overlay until Penpal handshake completes | Spinner shown after 150ms delay; hidden on connect |
| IFR-10 | Body scroll lock while iframe/modal is open; restored on every close path | `document.body.style.overflow = 'hidden'` + restore |
| UI-01 | Modal lists matching capabilities with `appName` + optional icon | DOM factory |
| UI-02 | Modal has a visible cancel button | Button element with `textContent` |
| UI-03 | `role="dialog"` + `aria-modal="true"` + `aria-labelledby` on root | ARIA attributes on dialog element |
| UI-04 | ESC key dismisses modal → `cancel` | `keydown` listener inside shadow root |
| UI-05 | Backdrop click dismisses modal → `cancel` | `click` listener, check target is backdrop |
| UI-06 | Focus trap confines keyboard focus inside modal (WCAG 2.1.2) | Shadow DOM focus-trap spike (see below) |
| UI-07 | Focus restored to trigger element on close (WCAG 2.4.3) | `document.activeElement` captured before open |
| UI-08 | Shadow DOM `attachShadow({ mode: 'open' })` | Locked decision |
| UI-09 | CSS reset inside Shadow DOM | `:host { all: initial }` or explicit resets |
| UI-10 | Capability metadata via DOM APIs (`textContent`), never `innerHTML` | XSS prevention |
| UI-11 | No external CSS dependencies | All styles inline in Shadow DOM |
| MSG-01 | `BridgeAdapter` interface; Penpal only in `messaging/penpal-bridge.ts` | Interface + implementation pattern |
| MSG-02 | `PenpalBridge` uses `connect({ messenger: new WindowMessenger({ remoteWindow, allowedOrigins }), methods })` | Verified against actual Penpal 7.0.6 types |
| MSG-03 | `allowedOrigins` restricted to `[new URL(capability.path).origin]` per session | Per-session origin restriction |
| MSG-04 | Parent exposes `resolve(result: IntentResult)` bound to specific `sessionId` via closure | Closure over `sessionId` |
| MSG-05 | `MockBridge` for unit tests — no real iframes needed | `MockBridge implements BridgeAdapter` |
| MSG-06 | Connection torn down on iframe close, timeout, `destroy()`, and session resolve | `connection.destroy()` in all close paths |
</phase_requirements>

---

## Summary

Phase 2 builds seven independent modules that Phase 3's orchestrator wires together. The modules are grouped into four layers with strict no-upward-dependency rules: capability layer (loader, resolver, WS listener), intent layer (planCast), UI layer (iframe factory, chooser modal, Shadow DOM host), and messaging layer (BridgeAdapter + PenpalBridge + MockBridge). Each module is independently unit-testable — pure functions in a Node environment, DOM factories in happy-dom.

The two highest-risk items are the Shadow DOM focus trap and the Penpal v7 handshake timing. Both have pre-phase research flags in SUMMARY.md and require working spike patterns before implementation. This document provides concrete, verified spike solutions for both. The Penpal 7.0.6 TypeScript declarations have been read directly from the installed package and confirm the API: `connect({ messenger: new WindowMessenger({ remoteWindow, allowedOrigins }), methods, timeout })` returns `{ promise: Promise<RemoteProxy>, destroy: () => void }`. There is no `connectToChild` or `connectToParent` — the spec pseudocode is dead.

The test infrastructure already has happy-dom at 20.8.9 (present in `devDependencies` from Phase 1's package.json, confirmed installed). Vitest 4.1.4 supports per-file `@vitest-environment` docblock comments, so no second workspace config is needed — add `// @vitest-environment happy-dom` at the top of each UI/messaging test file. A `FakeWebSocket` class (in-process) is sufficient for WS backoff tests; no external msw-ws dependency is needed.

**Primary recommendation:** Implement modules in dependency order within each wave — resolver and planCast first (pure, zero setup), then loader and WS listener (fake WebSocket), then UI layer with focus-trap spike result baked in, then PenpalBridge last (most complex teardown). Each module ships with co-located `*.test.ts` passing before the wave closes.

---

## Standard Stack

### Core (all already installed — verified from Phase 1 Summary)

| Library | Version | Purpose | Status |
|---------|---------|---------|--------|
| `penpal` | 7.0.6 (exact pin) | Parent→iframe PostMessage RPC | Installed |
| `typescript` | 6.0.2 | Source language; ES2020 target | Installed |
| `vitest` | 4.1.4 | Test runner, Node + happy-dom envs | Installed |
| `happy-dom` | 20.8.9 | DOM environment for UI tests | Installed |
| `@biomejs/biome` | 2.4.11 | Lint + format | Installed |

### What Phase 2 Adds (Wave 0)

happy-dom is already in `devDependencies` per `package.json` inspection — no installation needed in Wave 0. The only Wave 0 gap is a Vitest workspace split or per-file environment annotation.

**Confirmed from package.json:**
```json
"devDependencies": {
  "happy-dom": "^20.8.9"
}
```

### Vitest Environment Switch

Two options; option A is recommended for this project's single `vitest.config.ts` setup:

**Option A — Per-file docblock annotation (recommended, zero config change):**
```typescript
// src/ui/modal.test.ts
// @vitest-environment happy-dom

import { describe, it, expect } from 'vitest';
// ... tests use document, HTMLElement, ShadowRoot freely
```

**Option B — Workspace projects (adds complexity, use only if env switching causes issues):**
```typescript
// vitest.config.ts with projects
import { defineConfig } from 'vitest/config';
export default defineConfig({
  test: {
    projects: [
      {
        test: {
          name: 'node',
          environment: 'node',
          include: ['src/capabilities/**/*.test.ts', 'src/intent/**/*.test.ts'],
        },
      },
      {
        test: {
          name: 'happy-dom',
          environment: 'happy-dom',
          include: ['src/ui/**/*.test.ts', 'src/messaging/**/*.test.ts'],
        },
      },
    ],
  },
});
```

Option A is simpler and already supported in Vitest 4. Use it. Only switch to Option B if per-file annotations cause ordering issues with the coverage reporter.

---

## Architecture Patterns

### Recommended Project Structure

```
src/
├── index.ts                       # barrel (Phase 1; Phase 2 adds new exports)
├── errors.ts                      # OBCError (Phase 1 — do not modify)
├── types.ts                       # shared types (Phase 1 — do not modify)
│
├── capabilities/
│   ├── loader.ts                  # fetchCapabilities(url, signal) → Promise<Capability[]>
│   ├── loader.test.ts
│   ├── resolver.ts                # resolve(caps, intent): Capability[]
│   ├── resolver.test.ts
│   ├── ws-listener.ts             # WsListener class: start()/stop()
│   └── ws-listener.test.ts
│
├── intent/
│   ├── id.ts                      # Phase 1 — do not modify
│   ├── id.test.ts                 # Phase 1 — do not modify
│   ├── cast.ts                    # planCast(caps, intent): CastPlan
│   └── cast.test.ts
│
├── ui/
│   ├── shadow-host.ts             # createShadowHost(container, zIndex): ShadowRoot
│   ├── shadow-host.test.ts        # @vitest-environment happy-dom
│   ├── iframe.ts                  # buildIframe(cap, params): HTMLIFrameElement
│   ├── iframe.test.ts             # @vitest-environment happy-dom
│   ├── modal.ts                   # buildModal(caps, callbacks): { element, destroy }
│   ├── modal.test.ts              # @vitest-environment happy-dom
│   ├── focus-trap.ts              # trapFocus(root: ShadowRoot | HTMLElement): () => void
│   └── focus-trap.test.ts        # @vitest-environment happy-dom
│
└── messaging/
    ├── bridge-adapter.ts          # BridgeAdapter + ConnectionHandle interfaces
    ├── penpal-bridge.ts           # PenpalBridge implements BridgeAdapter
    ├── penpal-bridge.test.ts      # @vitest-environment happy-dom
    ├── mock-bridge.ts             # MockBridge implements BridgeAdapter
    └── mock-bridge.test.ts        # @vitest-environment happy-dom
```

**Note on focus-trap.ts:** Claude's Discretion allows either a separate file or inline in modal.ts. Given the Shadow DOM spike complexity documented below, a separate `ui/focus-trap.ts` is recommended — it enables isolated testing of the focus-trap logic without the full modal setup and makes the spike's working pattern easy to slot in.

### Pattern 1: Pure Resolver + planCast (no DOM, no I/O)

Both the resolver and planCast are pure functions. No environment setup needed.

**resolver.ts — MIME match logic:**

```typescript
// src/capabilities/resolver.ts
import type { Capability, IntentRequest } from '../types.js';

export function resolve(capabilities: Capability[], intent: IntentRequest): Capability[] {
  const requestedMime = intent.args.allowedMimeType;

  return capabilities.filter((cap) => {
    // RES-02: action must match
    if (cap.action !== intent.action) return false;

    // RES-03: absent or empty allowedMimeType matches all capabilities
    if (!requestedMime) return true;

    // RES-06: intent requests */* — matches any capability
    if (requestedMime === '*/*') return true;

    // RES-04: capability supports */* — matches any mime request
    if (cap.properties.mimeTypes.includes('*/*')) return true;

    // RES-05: exact mime string match
    return cap.properties.mimeTypes.includes(requestedMime);
  });
}
```

**cast.ts — CastPlan discriminated union:**

```typescript
// src/intent/cast.ts
import type { Capability, CastPlan } from '../types.js';

// Note: CastPlan is defined in types.ts as:
// | { kind: 'no-match' }
// | { kind: 'direct'; capability: Capability }
// | { kind: 'select'; capabilities: Capability[] }

export function planCast(matches: Capability[]): CastPlan {
  if (matches.length === 0) return { kind: 'no-match' };
  const first = matches[0];
  if (matches.length === 1 && first !== undefined) {
    return { kind: 'direct', capability: first };
  }
  return { kind: 'select', capabilities: matches };
}
```

**IMPORTANT — `noUncheckedIndexedAccess: true` is active.** `matches[0]` returns `Capability | undefined`. The guard `first !== undefined` before using `first` is not stylistic — it is required by the TypeScript compiler. Omitting it causes a type error.

### Pattern 2: HTTP Loader with AbortSignal

```typescript
// src/capabilities/loader.ts
import { OBCError } from '../errors.js';
import type { Capability } from '../types.js';

export interface LoaderResult {
  capabilities: Capability[];
  isOpenBuroServer: boolean;
}

export async function fetchCapabilities(
  url: string,
  signal?: AbortSignal
): Promise<LoaderResult> {
  // CAP-05: reject non-HTTPS at runtime
  if (new URL(url).protocol !== 'https:' && typeof location !== 'undefined' && location.protocol === 'https:') {
    throw new OBCError('CAPABILITIES_FETCH_FAILED', `capabilitiesUrl must use HTTPS when host page is HTTPS: ${url}`);
  }

  let response: Response;
  try {
    response = await fetch(url, { signal });
  } catch (cause) {
    throw new OBCError('CAPABILITIES_FETCH_FAILED', `Failed to fetch capabilities from ${url}`, cause);
  }

  if (!response.ok) {
    throw new OBCError('CAPABILITIES_FETCH_FAILED', `Capabilities endpoint returned ${response.status}`);
  }

  const isOpenBuroServer = response.headers.get('X-OpenBuro-Server') === 'true';
  const capabilities = (await response.json()) as Capability[];
  return { capabilities, isOpenBuroServer };
}
```

**Testing the loader:** Use `vi.stubGlobal('fetch', ...)` in Vitest to mock the global fetch. Test the AbortSignal by passing a pre-aborted signal (via `AbortSignal.abort()`) and asserting the fetch rejects.

### Pattern 3: WebSocket Listener with Full-Jitter Backoff + Destroyed Guard

This is the most operationally complex module. The `destroyed` flag and jitter must ship together in the initial implementation.

```typescript
// src/capabilities/ws-listener.ts
import { OBCError } from '../errors.js';

export interface WsListenerOptions {
  wsUrl: string;
  maxAttempts?: number;           // default 5 (WS-04)
  onUpdate: () => void;           // fires on REGISTRY_UPDATED
  onError: (err: OBCError) => void;
}

export class WsListener {
  private destroyed = false;      // WS-05: guards post-destroy reconnect
  private attempt = 0;
  private socket: WebSocket | null = null;
  private retryTimer: ReturnType<typeof setTimeout> | null = null;
  private readonly maxAttempts: number;
  private readonly wsUrl: string;
  private readonly onUpdate: () => void;
  private readonly onError: (err: OBCError) => void;

  constructor(opts: WsListenerOptions) {
    this.wsUrl = opts.wsUrl;
    this.maxAttempts = opts.maxAttempts ?? 5;
    this.onUpdate = opts.onUpdate;
    this.onError = opts.onError;
  }

  start(): void {
    if (this.destroyed) return;
    this.connect();
  }

  stop(): void {
    this.destroyed = true;            // WS-05
    if (this.retryTimer !== null) {
      clearTimeout(this.retryTimer); // cancel pending reconnect
      this.retryTimer = null;
    }
    if (this.socket) {
      this.socket.close();
      this.socket = null;
    }
  }

  private connect(): void {
    if (this.destroyed) return;       // WS-05: double-check after async gap

    // WS-07: reject non-wss in HTTPS context
    if (new URL(this.wsUrl).protocol !== 'wss:' && typeof location !== 'undefined' && location.protocol === 'https:') {
      this.onError(new OBCError('WS_CONNECTION_FAILED', 'WebSocket URL must use wss:// when host page is HTTPS'));
      return;
    }

    this.socket = new WebSocket(this.wsUrl);

    this.socket.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data as string) as { type?: string };
        if (data.type === 'REGISTRY_UPDATED') {
          this.onUpdate();           // WS-03
        }
      } catch {
        // ignore malformed messages
      }
    };

    this.socket.onopen = () => {
      this.attempt = 0;             // reset backoff on successful connection
    };

    this.socket.onerror = () => {
      // onclose fires after onerror; handle retry there
    };

    this.socket.onclose = () => {
      if (this.destroyed) return;   // WS-05: guard
      this.attempt += 1;
      if (this.attempt >= this.maxAttempts) {
        // WS-06: exhausted retries
        this.onError(new OBCError('WS_CONNECTION_FAILED', `WebSocket failed after ${this.maxAttempts} attempts`));
        return;
      }
      // WS-04: full-jitter delay = random(0, min(30000, 1000 * 2^attempt))
      const cap = 30_000;
      const base = 1_000 * Math.pow(2, this.attempt);
      const delay = Math.random() * Math.min(cap, base);
      this.retryTimer = setTimeout(() => {
        this.retryTimer = null;
        this.connect();             // WS-05: connect() re-checks destroyed
      }, delay);
    };
  }
}
```

**WS URL derivation (WS-02):**
```typescript
export function deriveWsUrl(capabilitiesUrl: string): string {
  const u = new URL(capabilitiesUrl);
  u.protocol = u.protocol === 'https:' ? 'wss:' : 'ws:';
  u.pathname = u.pathname.replace(/\/capabilities$/, '/capabilities/ws');
  return u.toString();
}
```

### Pattern 4: Shadow DOM Host + CSS Reset

```typescript
// src/ui/shadow-host.ts
export interface ShadowHostResult {
  host: HTMLElement;
  root: ShadowRoot;
}

export function createShadowHost(
  container: HTMLElement,
  zIndex = 9000
): ShadowHostResult {
  const host = document.createElement('div');
  // z-index and position MUST be on the light-DOM host element, NOT inside the shadow root
  // (pitfall: Shadow DOM does not create a new stacking context by itself)
  host.style.cssText = `position:fixed;inset:0;z-index:${zIndex};pointer-events:none;`;
  host.dataset['obcHost'] = '';  // selector for destroy() cleanup

  const root = host.attachShadow({ mode: 'open' });

  // CSS reset: prevent host-page font/color inheritance from leaking in
  const style = document.createElement('style');
  style.textContent = `
    :host {
      all: initial;
      display: block;
    }
    *, *::before, *::after {
      box-sizing: border-box;
      margin: 0;
      padding: 0;
    }
  `;
  root.appendChild(style);

  container.appendChild(host);
  return { host, root };
}
```

**Key pitfall from PITFALLS.md:** The host element's `z-index` and `position` live in the **light DOM**, not the shadow root. Setting `z-index` inside the shadow root does nothing for stacking context relative to the host page.

### Pattern 5: Iframe Factory with Same-Origin Guard

```typescript
// src/ui/iframe.ts
import { OBCError } from '../errors.js';
import type { Capability } from '../types.js';

export interface IframeParams {
  id: string;
  clientUrl: string;
  type: string;
  allowedMimeType?: string;
  multiple?: boolean;
}

export function buildIframe(
  capability: Capability,
  params: IframeParams
): HTMLIFrameElement {
  // IFR-08: same-origin guard — throw BEFORE creating the iframe
  const capOrigin = new URL(capability.path).origin;
  if (typeof location !== 'undefined' && capOrigin === location.origin) {
    throw new OBCError(
      'SAME_ORIGIN_CAPABILITY',
      `Capability "${capability.appName}" (${capability.path}) shares origin with host page. ` +
      'Same-origin capabilities are blocked because allow-scripts + allow-same-origin nullifies sandboxing.'
    );
  }

  const iframe = document.createElement('iframe');

  // IFR-03: query params
  const url = new URL(capability.path);
  url.searchParams.set('clientUrl', params.clientUrl);
  url.searchParams.set('id', params.id);
  url.searchParams.set('type', params.type);
  if (params.allowedMimeType) url.searchParams.set('allowedMimeType', params.allowedMimeType);
  if (params.multiple !== undefined) url.searchParams.set('multiple', String(params.multiple));
  iframe.src = url.toString();

  // IFR-04: sandbox
  iframe.setAttribute('sandbox', 'allow-scripts allow-same-origin allow-forms allow-popups');

  // IFR-05: permissions policy
  iframe.allow = 'clipboard-read; clipboard-write';

  // IFR-06: WCAG 2.4.1 accessible name — ONE LINE, hard fail without it
  iframe.title = capability.appName;

  // IFR-07: centered responsive sizing via inline style (lives in shadow root CSS)
  iframe.style.cssText = `
    display:block;
    width:min(90vw,800px);
    height:min(85vh,600px);
    border:none;
    border-radius:8px;
    box-shadow:0 4px 32px rgba(0,0,0,0.24);
  `;

  return iframe;
}
```

### Pattern 6: Chooser Modal with Full A11y Baseline

The modal is a DOM factory. Callbacks are injected by the caller (orchestrator). The modal returns an object with the element and a `destroy` method for cleanup.

```typescript
// src/ui/modal.ts
import type { Capability } from '../types.js';
import { trapFocus } from './focus-trap.js';

export interface ModalCallbacks {
  onSelect: (capability: Capability) => void;
  onCancel: () => void;
}

export interface ModalResult {
  element: HTMLElement;        // the dialog element (appended into shadow root by caller)
  destroy: () => void;         // removes listeners, releases focus trap
}

export function buildModal(
  capabilities: Capability[],
  callbacks: ModalCallbacks,
  shadowRoot: ShadowRoot       // shadow root receives the style tag + dialog
): ModalResult {
  // Inject modal-specific styles into the shadow root
  const style = document.createElement('style');
  style.textContent = getModalStyles();
  shadowRoot.appendChild(style);

  // UI-03: role="dialog" + aria-modal + aria-labelledby
  const dialog = document.createElement('div');
  dialog.setAttribute('role', 'dialog');
  dialog.setAttribute('aria-modal', 'true');
  dialog.setAttribute('aria-labelledby', 'obc-modal-title');
  dialog.className = 'obc-modal';

  const title = document.createElement('h2');
  title.id = 'obc-modal-title';
  title.textContent = 'Choose an application';   // UI-10: textContent, never innerHTML
  dialog.appendChild(title);

  const list = document.createElement('ul');
  list.setAttribute('role', 'listbox');
  list.setAttribute('aria-label', 'Available applications');
  dialog.appendChild(list);

  for (const cap of capabilities) {
    const item = document.createElement('li');
    item.setAttribute('role', 'option');
    item.setAttribute('tabindex', '0');
    item.dataset['capId'] = cap.id;

    const name = document.createElement('span');
    name.textContent = cap.appName;    // UI-10: textContent, not innerHTML
    item.appendChild(name);

    item.addEventListener('click', () => callbacks.onSelect(cap));
    item.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') callbacks.onSelect(cap);
    });
    list.appendChild(item);
  }

  const cancelBtn = document.createElement('button');
  cancelBtn.type = 'button';
  cancelBtn.textContent = 'Cancel';   // UI-02
  cancelBtn.addEventListener('click', callbacks.onCancel);
  dialog.appendChild(cancelBtn);

  // Backdrop
  const backdrop = document.createElement('div');
  backdrop.className = 'obc-backdrop';
  backdrop.setAttribute('aria-hidden', 'true');

  // UI-05: backdrop click dismisses
  backdrop.addEventListener('click', (e) => {
    if (e.target === backdrop) callbacks.onCancel();
  });

  const wrapper = document.createElement('div');
  wrapper.className = 'obc-wrapper';
  wrapper.appendChild(backdrop);
  wrapper.appendChild(dialog);

  // UI-04: ESC key — listen on shadow root, not document (stays inside shadow boundary)
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Escape') {
      e.preventDefault();
      callbacks.onCancel();
    }
    // Arrow key navigation (user-confirmed)
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      navigateItems(list, e.key);
    }
    if (e.key === 'Home') focusItem(list, 0);
    if (e.key === 'End') focusItem(list, -1);
  };
  shadowRoot.addEventListener('keydown', onKeyDown);

  // UI-06: focus trap (see spike below)
  const releaseTrap = trapFocus(shadowRoot, dialog);

  const destroy = () => {
    shadowRoot.removeEventListener('keydown', onKeyDown);
    releaseTrap();
    style.remove();
    wrapper.remove();
  };

  return { element: wrapper, destroy };
}
```

**Arrow key navigation helpers (pure, no DOM side effects):**
```typescript
function navigateItems(list: HTMLElement, direction: 'ArrowDown' | 'ArrowUp'): void {
  const items = Array.from(list.querySelectorAll<HTMLElement>('[role="option"]'));
  const current = list.querySelector<HTMLElement>('[role="option"]:focus');
  const idx = current ? items.indexOf(current) : -1;
  const next = direction === 'ArrowDown'
    ? Math.min(idx + 1, items.length - 1)
    : Math.max(idx - 1, 0);
  items[next]?.focus();
}

function focusItem(list: HTMLElement, index: number): void {
  const items = Array.from(list.querySelectorAll<HTMLElement>('[role="option"]'));
  const target = index === -1 ? items[items.length - 1] : items[index];
  target?.focus();
}
```

### Pattern 7: Penpal v7 BridgeAdapter + PenpalBridge

**Verified from installed `penpal@7.0.6` type declarations** (read directly from `node_modules/penpal/dist/penpal.d.ts`):

```typescript
// Penpal v7 actual API — confirmed from installed package types:
// connect<TMethods>({ messenger, methods, timeout, channel, log }): Connection<TMethods>
// Connection<TMethods> = { promise: Promise<RemoteProxy<TMethods>>, destroy: () => void }
// WindowMessenger constructor: ({ remoteWindow, allowedOrigins })
// allowedOrigins: (string | RegExp)[]
```

**bridge-adapter.ts:**
```typescript
// src/messaging/bridge-adapter.ts
import type { IntentResult } from '../types.js';

export interface ConnectionHandle {
  destroy(): void;
}

export interface ParentMethods {
  resolve(result: IntentResult): void;
}

export interface BridgeAdapter {
  connect(
    iframe: HTMLIFrameElement,
    allowedOrigin: string,
    methods: ParentMethods,
    timeoutMs?: number
  ): Promise<ConnectionHandle>;
}
```

**penpal-bridge.ts:**
```typescript
// src/messaging/penpal-bridge.ts
// ONLY file in the codebase that imports penpal (MSG-01)
import { connect, WindowMessenger } from 'penpal';
import type { BridgeAdapter, ConnectionHandle, ParentMethods } from './bridge-adapter.js';

export class PenpalBridge implements BridgeAdapter {
  async connect(
    iframe: HTMLIFrameElement,
    allowedOrigin: string,
    methods: ParentMethods,
    timeoutMs = 10_000   // user-confirmed 10s handshake timeout
  ): Promise<ConnectionHandle> {
    const remoteWindow = iframe.contentWindow;
    if (!remoteWindow) {
      throw new Error('iframe has no contentWindow — ensure it is attached to the DOM before connecting');
    }

    // MSG-03: restrict to specific capability origin per session
    const messenger = new WindowMessenger({
      remoteWindow,
      allowedOrigins: [allowedOrigin],   // (string | RegExp)[] per Penpal v7 types
    });

    // MSG-02: Penpal v7 connect() API — NOT connectToChild
    const connection = connect<Record<string, never>>({
      messenger,
      methods,        // MSG-04: parent exposes resolve(); child calls parent.resolve(result)
      timeout: timeoutMs,
    });

    // Wait for handshake to complete
    await connection.promise;

    return {
      destroy: () => connection.destroy(),   // MSG-06
    };
  }
}
```

**CRITICAL — Penpal v7 handshake timing rule (from PITFALLS.md Pitfall 1):**
The iframe MUST be appended to the DOM and `connect()` called in the **same synchronous block**. Never call `connect()` inside an `await`, `setTimeout`, or `iframe.onload` callback. The orchestrator (Phase 3) must follow this exact sequence:

```typescript
// In the orchestrator — CORRECT ORDER (same sync block):
const iframe = buildIframe(capability, params);
shadowRoot.appendChild(iframe);             // append first
const handle = await bridge.connect(       // then connect — these two lines must be in the same tick
  iframe,
  new URL(capability.path).origin,
  { resolve: (result) => onChildResolve(sessionId, result) }
);
```

**mock-bridge.ts:**
```typescript
// src/messaging/mock-bridge.ts
import type { BridgeAdapter, ConnectionHandle, ParentMethods } from './bridge-adapter.js';

export class MockBridge implements BridgeAdapter {
  /** Exposes the last-injected methods so tests can call parent.resolve() manually */
  public lastMethods: ParentMethods | null = null;
  public connectCallCount = 0;
  public destroyCallCount = 0;

  async connect(
    _iframe: HTMLIFrameElement,
    _allowedOrigin: string,
    methods: ParentMethods,
    _timeoutMs?: number
  ): Promise<ConnectionHandle> {
    this.connectCallCount += 1;
    this.lastMethods = methods;
    return {
      destroy: () => {
        this.destroyCallCount += 1;
      },
    };
  }
}
```

**MockBridge usage in orchestrator tests (Phase 3 preview):**
```typescript
const bridge = new MockBridge();
const obc = new OpenBuroClient({ capabilitiesUrl: 'https://example.com', _bridge: bridge });

const resultPromise = obc.castIntent({ action: 'PICK', args: {} }, cb);

// Simulate the child iframe calling parent.resolve():
bridge.lastMethods?.resolve({ id: sessionId, status: 'done', results: [] });

await resultPromise;
expect(cb).toHaveBeenCalledWith({ status: 'done', ... });
```

---

## Spike 1: Shadow DOM Focus Trap under Sandbox Constraints

**Pre-phase research flag from SUMMARY.md:** "Shadow DOM + iframe focus delegation under sandbox constraints (WCAG compliance spike needed)"

### The Problem

Standard focus-trap libraries (focus-trap, tabbable) assume `document.querySelectorAll` reaches all focusable elements. Inside a Shadow DOM, `document.querySelectorAll` does NOT cross the shadow boundary — you must query from the shadow root. Additionally, once the user opens the capability iframe, focus moves into a cross-origin sandboxed document. Keyboard focus cannot be programmatically moved back from inside the sandboxed iframe to the shadow modal.

### The Split

There are two separate focus contexts in OBC:

1. **Chooser modal focus trap** — keyboard inside the Shadow DOM modal while the user picks a capability. This is fully controllable.
2. **Capability iframe focus** — once the iframe is open, focus is inside a cross-origin sandboxed document. OBC cannot trap focus at the modal level at this point because the modal is dismissed when the iframe opens.

For v1, the focus trap applies **only** to the chooser modal (1). The iframe receives focus after the modal closes; restoring focus to the trigger element happens on iframe close (UI-07), not on modal-to-iframe transition.

### Working Spike Pattern

```typescript
// src/ui/focus-trap.ts
// Focus trap confined to a ShadowRoot — does NOT use document.querySelectorAll

const FOCUSABLE_SELECTORS = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
  '[role="option"][tabindex="0"]',
].join(',');

/**
 * Installs a focus trap inside `container` queried from `root`.
 * Returns a cleanup function that removes the trap.
 *
 * WCAG 2.1 SC 2.1.2 compliance: focus cannot leave the modal while it is open.
 * Shadow DOM note: querySelectorAll on `root` (ShadowRoot) correctly finds
 * focusable elements inside the shadow boundary.
 */
export function trapFocus(
  root: ShadowRoot,
  container: HTMLElement
): () => void {
  function getFocusableElements(): HTMLElement[] {
    // Query from shadow root, not from document
    return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTORS)).filter(
      (el) => container.contains(el)
    );
  }

  function onKeyDown(e: KeyboardEvent): void {
    if (e.key !== 'Tab') return;

    const focusable = getFocusableElements();
    if (focusable.length === 0) {
      e.preventDefault();
      return;
    }

    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    // root.activeElement traverses into shadow DOM correctly
    const active = root.activeElement;

    if (e.shiftKey) {
      if (active === first || active === container) {
        e.preventDefault();
        last?.focus();
      }
    } else {
      if (active === last) {
        e.preventDefault();
        first?.focus();
      }
    }
  }

  // Listen on the shadow root (not document) — events bubble inside shadow boundary
  root.addEventListener('keydown', onKeyDown);

  // Auto-focus first item on open (user-confirmed decision)
  const firstFocusable = getFocusableElements()[0];
  // Use rAF to ensure the element is rendered before focusing
  requestAnimationFrame(() => {
    firstFocusable?.focus();
  });

  return () => {
    root.removeEventListener('keydown', onKeyDown);
  };
}
```

**Why `root.activeElement` and not `document.activeElement`:**
When focus is inside a shadow root, `document.activeElement` returns the shadow host element, not the focused element inside it. `shadowRoot.activeElement` (i.e., `root.activeElement`) returns the actual focused element within the shadow tree. This is a WCAG-critical distinction.

### Testing Focus Trap in happy-dom

happy-dom 20.8.9 supports Shadow DOM, `attachShadow`, and `dispatchEvent` with keyboard events. The focus trap test pattern:

```typescript
// src/ui/focus-trap.test.ts
// @vitest-environment happy-dom

import { describe, it, expect, beforeEach } from 'vitest';
import { trapFocus } from './focus-trap.js';

describe('trapFocus', () => {
  let host: HTMLElement;
  let root: ShadowRoot;
  let container: HTMLElement;

  beforeEach(() => {
    host = document.createElement('div');
    document.body.appendChild(host);
    root = host.attachShadow({ mode: 'open' });

    container = document.createElement('div');
    const btn1 = document.createElement('button');
    btn1.textContent = 'First';
    const btn2 = document.createElement('button');
    btn2.textContent = 'Last';
    container.appendChild(btn1);
    container.appendChild(btn2);
    root.appendChild(container);
  });

  it('wraps Tab from last to first element', () => {
    const release = trapFocus(root, container);
    const buttons = container.querySelectorAll<HTMLElement>('button');
    const last = buttons[buttons.length - 1]!;
    last.focus();

    const tab = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true });
    root.dispatchEvent(tab);

    expect(root.activeElement).toBe(buttons[0]);
    release();
  });

  it('wraps Shift+Tab from first to last element', () => {
    const release = trapFocus(root, container);
    const buttons = container.querySelectorAll<HTMLElement>('button');
    buttons[0]!.focus();

    const shiftTab = new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true });
    root.dispatchEvent(shiftTab);

    expect(root.activeElement).toBe(buttons[buttons.length - 1]);
    release();
  });

  it('release() removes the keydown listener', () => {
    const release = trapFocus(root, container);
    release();
    // After release, Tab should not be intercepted — no assertion on focus position needed
    // Verify no error thrown
    const tab = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true });
    root.dispatchEvent(tab);
    expect(true).toBe(true); // reaches here = no error
  });
});
```

**happy-dom caveat:** happy-dom's `focus()` implementation may not update `root.activeElement` synchronously in all cases. If assertions on `root.activeElement` fail, assert via `document.activeElement` or check `el.matches(':focus')` as a fallback.

---

## Spike 2: Penpal v7 Handshake Test Fixture

**Pre-phase research flag from SUMMARY.md:** "Penpal v7 MessagePort handshake timing under restrictive CSP (validate early)"

### The Problem

Penpal v7 uses a SYN→ACK1→ACK2 handshake protocol. After ACK2, all RPC traffic moves to MessagePorts (not `window.postMessage` on the window object). In a restrictive CSP (`default-src 'self'`), the `postMessage` SYN frame may be blocked if the CSP restricts `frame-src` or `connect-src` for `blob:` or `data:` URLs.

In happy-dom, there is no real cross-origin context. A second `Window` object can be created with `new Window()` from happy-dom, but Penpal's `WindowMessenger` communicates via `postMessage` which happy-dom simulates in-process.

### Resolution Strategy

**For unit tests of PenpalBridge:** Use `MockBridge` — there is no point testing Penpal internals in a unit context. `MockBridge` gives the orchestrator tests 100% coverage of the session lifecycle without real iframes.

**For integration tests of Penpal handshake:** Use two happy-dom `Window` objects connected via a shared `MessageChannel`. This simulates the parent-child postMessage flow without real iframes.

```typescript
// src/messaging/penpal-bridge.test.ts
// @vitest-environment happy-dom

import { describe, it, expect } from 'vitest';
import { connect, WindowMessenger } from 'penpal';

// Spike: verify Penpal v7 handshake between two in-process Window objects
describe('Penpal v7 handshake spike', () => {
  it('establishes connection between two Window objects via postMessage', async () => {
    // Use happy-dom's global window as "parent"
    // Create a second window object to act as "child"
    // NOTE: happy-dom supports window.open() returning a Window; use that
    const childWindow = window.open('about:blank')!;

    // Parent side
    const parentMessenger = new WindowMessenger({
      remoteWindow: childWindow,
      allowedOrigins: ['*'],  // test only — production always restricts
    });

    const parentConnection = connect<{ ping(): string }>({
      messenger: parentMessenger,
      timeout: 3000,
    });

    // Child side — must connect() in same tick as parent (or before)
    const childMessenger = new WindowMessenger({
      remoteWindow: window,    // child's "parent" is the opener
      allowedOrigins: ['*'],
    });

    connect({
      messenger: childMessenger,
      methods: {
        ping: () => 'pong',
      },
    });

    const remote = await parentConnection.promise;
    const result = await remote.ping();
    expect(result).toBe('pong');

    parentConnection.destroy();
  });
});
```

**Honest assessment:** happy-dom's `window.open()` behavior is limited. If `window.open()` does not return a proper Window with a working `postMessage`, the Penpal handshake test will fail in the test environment even if it works in a real browser. In that case:

1. Accept the limitation and test `PenpalBridge` only via `MockBridge` in unit tests.
2. Add a single Playwright test for the real handshake (out of scope for Phase 2, but documented here so Phase 3/4 planning accounts for it).

The recommended Phase 2 approach: `penpal-bridge.test.ts` tests that `connect()` is called with the right arguments (spy on the Penpal `connect` import using `vi.mock`), not that the handshake completes. The actual handshake correctness is validated by the Phase 3 orchestrator integration test using `MockBridge`.

**CSP note:** The Penpal 7.0.6 `WindowMessenger` uses `window.postMessage()` for the SYN frame and then transitions to MessagePort. If the host CSP has `frame-ancestors` restrictions, those affect whether the iframe loads at all — not the postMessage protocol itself. The `connect-src` CSP directive does not restrict `postMessage`. The real CSP risk is `frame-src` blocking the capability iframe URL; OBC has no control over that (it is the capability server's CSP). Document this in the capability author guide.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Parent→iframe RPC | Custom `window.addEventListener('message', ...)` with session routing | Penpal v7 `connect()` + `WindowMessenger` | Origin restriction, connection lifecycle, MessagePort transition, timeout — all solved |
| MIME type matching | External `mime-match` npm package | Inline four-rule logic in `resolver.ts` | The four rules are closed-form; no edge cases require external parsing |
| Focus trap | `focus-trap` npm package | `ui/focus-trap.ts` (spike above) | The spike works; `focus-trap` package queries `document`, not `ShadowRoot` — will not work inside Shadow DOM |
| UUID v4 | `uuid` npm package | `generateSessionId()` already in `src/intent/id.ts` | Already implemented and tested in Phase 1 |
| CSS-in-JS | Any CSS-in-JS runtime | Inline `<style>` tag inside Shadow DOM | Shadow DOM eliminates all need for scoping; no runtime overhead |
| WebSocket reconnect library | `reconnecting-websocket` npm package | `WsListener` class (pattern above) | Single-maintainer deps; the jitter + destroyed-guard is 40 lines |

---

## Common Pitfalls

### Pitfall 1: Calling Penpal `connect()` After an `await`

**What goes wrong:** If the orchestrator awaits anything (e.g., `await buildIframe()`) before calling `bridge.connect()`, the iframe may have already sent its SYN handshake frame. Penpal v7 is more resilient than v6 but the race is real on fast loads.

**How to avoid:** The sequence must be synchronous: `buildIframe()` → `shadowRoot.appendChild(iframe)` → `bridge.connect(...)` — all in the same synchronous block, before any `await`.

**Warning sign:** `connection.promise` hangs indefinitely; no timeout fires.

### Pitfall 2: `noUncheckedIndexedAccess` Breaks Array Index Access

**What goes wrong:** `matches[0]` returns `Capability | undefined` in this TypeScript config. Code like `return { kind: 'direct', capability: matches[0] }` fails tsc when the type expects `Capability`.

**How to avoid:** Always guard: `const first = matches[0]; if (first !== undefined) return { kind: 'direct', capability: first }`.

**Warning sign:** TypeScript error on array access anywhere in the codebase.

### Pitfall 3: `document.activeElement` Inside Shadow DOM Returns Shadow Host, Not Focused Element

**What goes wrong:** The focus-trap checks `document.activeElement === lastFocusable` — but `document.activeElement` inside a shadow root returns the shadow host element, not the actual focused button. The trap logic fails silently.

**How to avoid:** Use `shadowRoot.activeElement` (i.e., `root.activeElement`) inside any focus comparison logic within the trap.

### Pitfall 4: `z-index` on Shadow DOM Root Has No Effect

**What goes wrong:** Setting `z-index: 9999` inside the shadow root's CSS does not affect the modal's stacking order relative to the host page. The host page may render content above the modal.

**How to avoid:** Set `z-index` and `position: fixed` on the shadow host element in the light DOM (standard DOM styles). The shadow root's `z-index` is relative to the shadow DOM's own stacking context.

### Pitfall 5: ESC Key Listener on `document` Not Fired Inside Shadow DOM

**What goes wrong:** Adding `document.addEventListener('keydown', handler)` — the event does fire (events bubble up from shadow DOM to document), but best practice is to listen on the `shadowRoot` directly to avoid processing events from outside the modal when multiple shadows exist.

**How to avoid:** Attach the ESC keydown listener to the `shadowRoot`, not `document`. Events still bubble but are scoped to the shadow context.

### Pitfall 6: `innerHTML` for Capability `appName`

**What goes wrong:** A malicious capability registry returns `appName: '<img src=x onerror=alert(1)>'`. If rendered via `element.innerHTML = cap.appName`, this is an XSS vector.

**How to avoid:** Always `element.textContent = cap.appName`. Never `innerHTML` for any untrusted capability metadata (UI-10).

### Pitfall 7: WS `destroyed` Flag Not Checked Inside `setTimeout` Callback

**What goes wrong:** `stop()` is called. The pending `setTimeout` fires anyway (the timer was not cleared, or was cleared but race). Inside the callback, `this.connect()` runs, opening a new WebSocket on a destroyed instance.

**How to avoid:** Two guards: (1) `clearTimeout(this.retryTimer)` in `stop()`, and (2) check `if (this.destroyed) return;` at the top of both `connect()` and the `setTimeout` callback. The double-check is required.

### Pitfall 8: Biome's `noParameterAssign` Rejects Direct Parameter Mutation

**What goes wrong:** Code like `url.protocol = 'wss:';` inside a URL object is fine, but reassigning a parameter directly (e.g., `url = new URL(...)`) triggers Biome's `noParameterAssign` rule.

**How to avoid:** Use `const derived = new URL(url)` and mutate the copy, not the parameter.

### Pitfall 9: Single-Quote String Literals Rejected by Biome

**Established in Phase 1 Summary:** Biome is configured to prefer double quotes. All string literals in new files must use double quotes. Exception: JSDoc comment strings (single quotes inside JSDoc triggered TS1002 in Phase 1 — use double quotes or avoid single quotes in JSDoc entirely).

---

## ActiveSession Type

Phase 2 defines this type (Phase 3 holds the Map):

```typescript
// src/intent/types.ts (new file in Phase 2)
import type { Capability, IntentResult } from '../types.js';
import type { ConnectionHandle } from '../messaging/bridge-adapter.js';

export interface ActiveSession {
  id: string;
  capability: Capability;
  iframe: HTMLIFrameElement;
  shadowHost: HTMLElement;
  connectionHandle: ConnectionHandle;
  timeoutHandle: ReturnType<typeof setTimeout>;
  resolve: (result: IntentResult) => void;
  reject: (err: Error) => void;
  callback: (result: IntentResult) => void;
}
```

---

## Loading Spinner Pattern (IFR-09)

The spinner appears after 150ms delay and is hidden when Penpal handshake completes.

```typescript
// Pattern for orchestrator use (Phase 3), documented here for Phase 2 implementation:
// The shadow-host or iframe module exposes a showSpinner/hideSpinner pair.

export function createSpinnerOverlay(): { element: HTMLElement; show(): void; hide(): void } {
  const overlay = document.createElement('div');
  overlay.className = 'obc-spinner-overlay';
  overlay.style.cssText = 'display:none; position:absolute; inset:0; ...';

  const spinner = document.createElement('div');
  spinner.className = 'obc-spinner';
  overlay.appendChild(spinner);

  let timer: ReturnType<typeof setTimeout> | null = null;

  return {
    element: overlay,
    show() {
      // IFR-09: 150ms delay before spinner appears
      timer = setTimeout(() => {
        overlay.style.display = 'flex';
      }, 150);
    },
    hide() {
      if (timer !== null) {
        clearTimeout(timer);
        timer = null;
      }
      overlay.style.display = 'none';
    },
  };
}
```

**CSS border-rotate spinner (user-confirmed: no SVG, no emoji):**
```css
@keyframes obc-spin {
  to { transform: rotate(360deg); }
}
.obc-spinner {
  width: 32px;
  height: 32px;
  border: 3px solid rgba(37,99,235,0.2);  /* accent #2563eb at 20% */
  border-top-color: #2563eb;
  border-radius: 50%;
  animation: obc-spin 0.8s linear infinite;
}
```

---

## Body Scroll Lock (IFR-10)

Applied by the orchestrator whenever the modal or iframe is open. Phase 2 exports the helper:

```typescript
// src/ui/scroll-lock.ts
export function lockBodyScroll(): () => void {
  const prev = document.body.style.overflow;
  document.body.style.overflow = 'hidden';
  return () => {
    document.body.style.overflow = prev;
  };
}
```

Every close path (modal cancel, ESC, backdrop click, timeout, `destroy()`) must call the returned restorer. Store it in `ActiveSession`.

---

## State of the Art

| Old Approach | Current Approach | Impact |
|--------------|------------------|--------|
| Penpal v6 `connectToChild()` / `connectToParent()` | Penpal v7 `connect({ messenger: new WindowMessenger(...), methods })` | All spec pseudocode is invalid; implement from verified types only |
| `focus-trap` npm package | Shadow-root-aware manual trap (spike above) | focus-trap queries `document`, not `ShadowRoot` |
| `tsup` bundler | `tsdown` (Rolldown-based successor) | tsup is deprecated; already established in Phase 1 |
| `uuid` npm package | `generateSessionId()` in `src/intent/id.ts` | Already done in Phase 1; do not add uuid dependency |

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Vitest 4.1.4 |
| Config file | `vitest.config.ts` (existing, `environment: 'node'` default) |
| Environment switch | Per-file `// @vitest-environment happy-dom` docblock |
| Quick run command | `pnpm test` (runs `vitest run`) |
| Full suite command | `pnpm run ci` (typecheck + lint + test + attw) |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | Notes |
|--------|----------|-----------|-------------------|-------|
| RES-01..07 | MIME resolver all 4 rules + absent + wildcard | Unit (node) | `pnpm test -- resolver` | Pure function, no mocks needed |
| RES-07 | `planCast()` all 3 branches | Unit (node) | `pnpm test -- cast` | Pure function |
| CAP-01,03,04,05 | Loader fetch, error mapping, AbortSignal, HTTPS guard | Unit (node) | `pnpm test -- loader` | `vi.stubGlobal('fetch', ...)` |
| CAP-02 | Header detection `X-OpenBuro-Server` | Unit (node) | included in loader tests | Mock response headers |
| WS-01..07 | WS reconnect, jitter, destroyed guard, protocol validation | Unit (node) | `pnpm test -- ws-listener` | `FakeWebSocket` class |
| INT-01..09 | `planCast` branches, session isolation | Unit (node) | `pnpm test -- cast` | Pure, no DOM |
| IFR-01..10 | iframe DOM structure, sandbox, title, same-origin guard | Unit (happy-dom) | `pnpm test -- iframe` | `// @vitest-environment happy-dom` |
| UI-01..11 | Modal DOM, ARIA attrs, ESC, backdrop, keyboard nav | Unit (happy-dom) | `pnpm test -- modal` | Shadow DOM in happy-dom |
| UI-06 | Focus trap wraps Tab/Shift+Tab | Unit (happy-dom) | `pnpm test -- focus-trap` | `root.activeElement` assertions |
| UI-07 | Focus restore to trigger element | Unit (happy-dom) | included in modal tests | Save/restore `document.activeElement` |
| MSG-01..06 | `BridgeAdapter` interface, `MockBridge` behavior | Unit (happy-dom) | `pnpm test -- mock-bridge` | No real iframe |
| MSG-02 | `PenpalBridge.connect()` calls Penpal with correct args | Unit (happy-dom) | `pnpm test -- penpal-bridge` | `vi.mock('penpal', ...)` |
| IFR-09 | Spinner shows after 150ms, hides on connect | Unit (happy-dom) | included in spinner tests | Vitest fake timers |
| IFR-10 | Body scroll lock applied + restored on all close paths | Unit (happy-dom) | included in modal/iframe tests | Assert `document.body.style.overflow` |

### FakeWebSocket Pattern

In-process fake — no external dependency:

```typescript
// Used inside ws-listener.test.ts
class FakeWebSocket {
  static instance: FakeWebSocket | null = null;
  readyState = WebSocket.CONNECTING;
  onopen: ((e: Event) => void) | null = null;
  onclose: ((e: CloseEvent) => void) | null = null;
  onerror: ((e: Event) => void) | null = null;
  onmessage: ((e: MessageEvent) => void) | null = null;

  constructor(public url: string) {
    FakeWebSocket.instance = this;
    // Auto-open in next tick
    Promise.resolve().then(() => {
      this.readyState = WebSocket.OPEN;
      this.onopen?.(new Event('open'));
    });
  }

  close() { this.readyState = WebSocket.CLOSED; }
  send(_data: unknown) {}

  // Test helper: simulate server message
  simulateMessage(data: unknown) {
    this.onmessage?.(new MessageEvent('message', { data: JSON.stringify(data) }));
  }

  // Test helper: simulate close
  simulateClose() {
    this.readyState = WebSocket.CLOSED;
    this.onclose?.(new CloseEvent('close'));
  }
}

// In test:
vi.stubGlobal('WebSocket', FakeWebSocket);
```

### AbortController Teardown Test Pattern

Assert "zero listeners remain" after `destroy()`:

```typescript
// Test pattern for loader AbortController
it('aborts in-flight fetch when signal is aborted', async () => {
  let capturedSignal: AbortSignal | undefined;
  vi.stubGlobal('fetch', (url: string, init?: RequestInit) => {
    capturedSignal = init?.signal;
    return new Promise(() => {}); // never resolves
  });

  const controller = new AbortController();
  const promise = fetchCapabilities('https://example.com/caps', controller.signal);
  controller.abort();

  await expect(promise).rejects.toThrow(); // fetch rejects with AbortError

  // Signal is aborted
  expect(capturedSignal?.aborted).toBe(true);
});
```

For WebSocket signal teardown, assert `FakeWebSocket.instance?.readyState === WebSocket.CLOSED` after `ws.stop()`.

For DOM listener teardown, use Vitest spies:
```typescript
const spy = vi.spyOn(shadowRoot, 'removeEventListener');
modal.destroy();
expect(spy).toHaveBeenCalledWith('keydown', expect.any(Function));
```

### Wave 0 Gaps

- [ ] `src/ui/shadow-host.ts` — does not exist yet (create in Phase 2)
- [ ] `src/ui/iframe.ts` — does not exist yet
- [ ] `src/ui/modal.ts` — does not exist yet
- [ ] `src/ui/focus-trap.ts` — does not exist yet
- [ ] `src/ui/scroll-lock.ts` — does not exist yet
- [ ] `src/capabilities/loader.ts` — does not exist yet
- [ ] `src/capabilities/resolver.ts` — does not exist yet
- [ ] `src/capabilities/ws-listener.ts` — does not exist yet
- [ ] `src/intent/cast.ts` — does not exist yet
- [ ] `src/intent/types.ts` — does not exist yet (ActiveSession type)
- [ ] `src/messaging/bridge-adapter.ts` — does not exist yet
- [ ] `src/messaging/penpal-bridge.ts` — does not exist yet
- [ ] `src/messaging/mock-bridge.ts` — does not exist yet
- [ ] `vitest.config.ts` — currently `environment: 'node'`; no workspace config needed if per-file docblock is used, but confirm happy-dom environment resolves correctly with `// @vitest-environment happy-dom`
- [ ] `src/index.ts` — add Phase 2 exports (resolver, planCast, WsListener, BridgeAdapter, MockBridge, OBCOptions public types)

No new devDependencies needed: happy-dom 20.8.9 is already installed.

---

## Sources

### Primary (HIGH confidence)

- Penpal 7.0.6 TypeScript declarations — read directly from `node_modules/penpal/dist/penpal.d.ts` — `connect()`, `WindowMessenger`, `Connection` type, `allowedOrigins: (string | RegExp)[]`, `timeout` parameter all verified
- Phase 1 Summary `01-01-SUMMARY.md` — confirmed installed packages, established patterns (biome quotes, noUncheckedIndexedAccess, ES2020 Error.cause absence)
- `src/types.ts` (read) — `CastPlan` discriminated union uses `capabilities: Capability[]` (not `candidates`) in `{ kind: 'select' }` branch
- `src/errors.ts` (read) — `OBCErrorCode` union, `OBCError` constructor signature `(code, message, cause?)`
- `vitest.config.ts` (read) — current `environment: 'node'`, per-file docblock override is valid in Vitest 4
- `package.json` (read) — `happy-dom: ^20.8.9` is already in devDependencies; no Wave 0 install needed
- MDN — Shadow DOM `activeElement`, `attachShadow`, `querySelectorAll` on ShadowRoot
- WCAG 2.1 SC 2.1.2, 2.4.3, 4.1.2 — focus trap, focus restore, ARIA dialog requirements
- ACT rule cae760 — iframe accessible name via `title` attribute

### Secondary (MEDIUM confidence)

- SUMMARY.md research flags — Shadow DOM focus spike and Penpal CSP timing validated; spike solutions above are original based on MDN specs
- PITFALLS.md — Pitfalls 1-11 inform the Common Pitfalls section; original text consulted
- ARCHITECTURE.md — layer boundaries, component responsibilities, data flow diagram

### Tertiary (LOW confidence)

- happy-dom `root.activeElement` behavior in focus-trap tests — happy-dom 20.8.9 supports Shadow DOM but `focus()` synchronicity in test context varies; the spike test notes this caveat

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — verified from installed packages and Phase 1 SUMMARY
- Architecture: HIGH — derived from ARCHITECTURE.md + CONTEXT.md locked decisions
- Penpal API: HIGH — read directly from installed 7.0.6 type declarations
- Focus-trap spike: MEDIUM-HIGH — based on MDN ShadowRoot API; happy-dom compatibility caveat noted
- Pitfalls: HIGH — sourced from PITFALLS.md (verified research) + Phase 1 SUMMARY deviations

**Research date:** 2026-04-10
**Valid until:** 2026-05-10 (stable APIs; happy-dom major version change would require re-check)
