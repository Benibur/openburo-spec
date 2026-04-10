---
phase: 02-core-implementation
plan: 03
type: execute
wave: 1
depends_on: []
files_modified:
  - src/ui/styles.ts
  - src/ui/styles.test.ts
  - src/ui/scroll-lock.ts
  - src/ui/iframe.ts
  - src/ui/iframe.test.ts
  - src/ui/focus-trap.ts
  - src/ui/focus-trap.test.ts
  - src/ui/modal.ts
  - src/ui/modal.test.ts
autonomous: true
requirements:
  - IFR-01
  - IFR-02
  - IFR-03
  - IFR-04
  - IFR-05
  - IFR-06
  - IFR-07
  - IFR-08
  - IFR-09
  - IFR-10
  - UI-01
  - UI-02
  - UI-03
  - UI-04
  - UI-05
  - UI-06
  - UI-07
  - UI-08
  - UI-09
  - UI-10
  - UI-11

must_haves:
  truths:
    - "createShadowHost() returns a div with position:fixed and a configurable z-index on the LIGHT DOM host element (not inside the shadow root)"
    - "The shadow host uses attachShadow({ mode: 'open' })"
    - "Shadow root contains a CSS reset (:host { all: initial })"
    - "buildIframe() throws OBCError(SAME_ORIGIN_CAPABILITY) when capability.path origin === location.origin"
    - "buildIframe() sets iframe.title = capability.appName (WCAG 2.4.1 / ACT cae760)"
    - "buildIframe() sets sandbox='allow-scripts allow-same-origin allow-forms allow-popups'"
    - "buildIframe() sets allow='clipboard-read; clipboard-write'"
    - "buildIframe() URL has clientUrl, id, type, allowedMimeType, multiple query params"
    - "buildIframe() inline style uses width:min(90vw,800px); height:min(85vh,600px); border-radius:8px"
    - "trapFocus(root, container) returns a release function; root.addEventListener('keydown', ...) intercepts Tab"
    - "trapFocus uses root.activeElement (not document.activeElement) for focus comparison"
    - "buildModal() creates a div with role='dialog', aria-modal='true', aria-labelledby pointing at a title node"
    - "Modal cancel button is a <button> with textContent 'Cancel'"
    - "Modal renders capability.appName via textContent, never innerHTML"
    - "Modal keydown listener is attached to shadowRoot (not document)"
    - "lockBodyScroll() sets document.body.style.overflow='hidden' and returns a restorer"
    - "Loading spinner overlay hides by default and appears after 150ms setTimeout delay on show()"
    - "Modal destroy() restores focus to the element that held focus before buildModal() was called (WCAG 2.4.3)"
  artifacts:
    - path: "src/ui/styles.ts"
      provides: "createShadowHost(container, zIndex) → { host, root } with CSS reset + createSpinnerOverlay()"
      exports: ["createShadowHost", "ShadowHostResult", "createSpinnerOverlay"]
      min_lines: 60
    - path: "src/ui/scroll-lock.ts"
      provides: "lockBodyScroll() → restorer"
      exports: ["lockBodyScroll"]
      min_lines: 8
    - path: "src/ui/iframe.ts"
      provides: "buildIframe(capability, params) → HTMLIFrameElement with same-origin guard"
      exports: ["buildIframe", "IframeParams"]
      min_lines: 50
    - path: "src/ui/focus-trap.ts"
      provides: "trapFocus(root, container) → release function (Shadow-DOM-aware)"
      exports: ["trapFocus"]
      min_lines: 50
    - path: "src/ui/modal.ts"
      provides: "buildModal(caps, callbacks, shadowRoot) → { element, destroy } with full a11y"
      exports: ["buildModal", "ModalCallbacks", "ModalResult"]
      min_lines: 100
  key_links:
    - from: "src/ui/iframe.ts"
      to: "same-origin guard via location.origin"
      via: "constructor precondition throwing OBCError"
      pattern: "SAME_ORIGIN_CAPABILITY"
    - from: "src/ui/styles.ts"
      to: "z-index on LIGHT DOM host element"
      via: "host.style.cssText"
      pattern: "z-index:"
    - from: "src/ui/focus-trap.ts"
      to: "root.activeElement (not document.activeElement)"
      via: "shadow-root-aware focus query"
      pattern: "root\\.activeElement"
    - from: "src/ui/modal.ts"
      to: "src/ui/focus-trap.ts (trapFocus)"
      via: "import"
      pattern: "import.*trapFocus"
    - from: "src/ui/modal.ts"
      to: "textContent over innerHTML"
      via: "DOM factory pattern"
      pattern: "textContent"
---

<objective>
Build the UI layer — Shadow DOM host factory with CSS reset and spinner overlay, body-scroll-lock helper, iframe factory with same-origin guard and WCAG title, Shadow-DOM-aware focus trap, and the chooser modal with full a11y baseline. This layer has zero Penpal knowledge (strict layer isolation) and zero capability/messaging dependencies.

Purpose: The orchestrator (Phase 3) composes `createShadowHost()`, `buildModal()` (for multi-match), `buildIframe()` (for direct or after modal select), `trapFocus()` (while modal open), and `lockBodyScroll()` (while modal/iframe open) into the `castIntent` flow. Every function in this layer is independently unit-testable under `@vitest-environment happy-dom`.

Output:
- `src/ui/styles.ts` + test (happy-dom)
- `src/ui/scroll-lock.ts` (no dedicated test; verified inside modal tests)
- `src/ui/iframe.ts` + test (happy-dom)
- `src/ui/focus-trap.ts` + test (happy-dom)
- `src/ui/modal.ts` + test (happy-dom)

**All test files MUST start with `// @vitest-environment happy-dom` as the first line** (Vitest docblock; Phase 1 ships a Node-default config, per-file override is the locked approach per 02-CONTEXT.md decisions).
</objective>

<execution_context>
@/home/ben/.claude/get-shit-done/workflows/execute-plan.md
@/home/ben/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/02-core-implementation/02-CONTEXT.md
@.planning/phases/02-core-implementation/02-RESEARCH.md
@.planning/phases/01-foundations/01-01-SUMMARY.md
@src/types.ts
@src/errors.ts

<interfaces>
<!-- Types already exported from Phase 1. -->

From src/types.ts:
```typescript
export interface Capability {
  id: string;
  appName: string;
  action: string;
  path: string;
  iconUrl?: string;
  properties: { mimeTypes: string[] };
}
```

From src/errors.ts:
```typescript
export class OBCError extends Error {
  readonly code: OBCErrorCode;
  readonly cause?: unknown;
  constructor(code: OBCErrorCode, message: string, cause?: unknown);
}
// OBCErrorCode includes "SAME_ORIGIN_CAPABILITY"
```

**DOM globals (happy-dom 20.8.9 already in devDependencies from Phase 1):**
- `document`, `window`, `HTMLElement`, `HTMLIFrameElement`, `ShadowRoot`, `Event`, `KeyboardEvent`, `MouseEvent`
- `document.createElement`, `element.attachShadow({ mode: "open" })`, `element.appendChild`
- `element.dispatchEvent`, `new KeyboardEvent("keydown", { key: "Tab", shiftKey: true, bubbles: true })`
</interfaces>

<locked_visual_defaults>
From 02-CONTEXT.md (user-confirmed):
- Modal card: `max-width: 480px`, `min-width: 320px`
- Backdrop color: `rgba(0, 0, 0, 0.5)`
- Accent color: `#2563eb` (Tailwind blue-600)
- Loading spinner: pure CSS `@keyframes` border-rotate — NO SVG, NO emoji, NO external font
- Spinner delay: 150 ms
- Keyboard: `↑/↓` navigate, `Home/End` jump, `Enter` select, `Escape` cancel, auto-focus first item on open
- z-index defaults: 9000 (backdrop) / 9001 (iframe)
</locked_visual_defaults>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Shadow DOM host + CSS reset + spinner overlay + scroll lock (UI-08, UI-09, UI-11, IFR-02, IFR-09, IFR-10)</name>
  <files>src/ui/styles.ts, src/ui/styles.test.ts, src/ui/scroll-lock.ts</files>

  <read_first>
    - .planning/phases/02-core-implementation/02-RESEARCH.md (Pattern 4: Shadow DOM Host + CSS Reset, Loading Spinner Pattern, Body Scroll Lock, Pitfall 4: z-index on Shadow DOM Root)
    - .planning/phases/02-core-implementation/02-CONTEXT.md (locked visual defaults block)
  </read_first>

  <behavior>
    - createShadowHost(document.body) returns { host: HTMLElement, root: ShadowRoot }
    - host.style.position === "fixed" and host.style.zIndex === "9000" (default)
    - createShadowHost(container, 12345) sets host.style.zIndex === "12345"
    - root contains a <style> tag with the literal ":host { all: initial"
    - host is appended to the supplied container
    - host has a data attribute for destroy() cleanup (e.g. data-obc-host)
    - createSpinnerOverlay() returns { element, show, hide }
    - Immediately after createSpinnerOverlay(), element.style.display === "none"
    - show() does NOT immediately set display — it sets a 150ms setTimeout
    - After 150ms (fake timers), element.style.display === "flex"
    - hide() clears the timer and sets display back to "none"
    - If hide() is called BEFORE 150ms elapses, element.style.display === "none" (never shows)
    - lockBodyScroll() sets document.body.style.overflow = "hidden" and returns a restorer
    - The restorer restores the previous value (empty string or whatever was there)
  </behavior>

  <action>
    **File 1: `src/ui/styles.ts`** — copy Pattern 4 from 02-RESEARCH.md verbatim (lines ~462-501), adapted:

    ```typescript
    export interface ShadowHostResult {
      host: HTMLElement;
      root: ShadowRoot;
    }

    export function createShadowHost(
      container: HTMLElement,
      zIndex = 9000
    ): ShadowHostResult {
      const host = document.createElement("div");
      // IFR-02 + Pitfall 4: z-index and position MUST be on the light-DOM host element,
      // NOT inside the shadow root. Shadow DOM does not create a stacking context by itself.
      host.style.cssText = `position:fixed;inset:0;z-index:${zIndex};pointer-events:none;`;
      host.dataset["obcHost"] = "";

      const root = host.attachShadow({ mode: "open" });

      // UI-09: CSS reset so host-page typography/colors do not leak into the modal
      const style = document.createElement("style");
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
        @keyframes obc-spin {
          to { transform: rotate(360deg); }
        }
        .obc-spinner {
          width: 32px;
          height: 32px;
          border: 3px solid rgba(37,99,235,0.2);
          border-top-color: #2563eb;
          border-radius: 50%;
          animation: obc-spin 0.8s linear infinite;
        }
        .obc-spinner-overlay {
          position: absolute;
          inset: 0;
          display: none;
          align-items: center;
          justify-content: center;
          background: rgba(255,255,255,0.6);
          pointer-events: auto;
        }
      `;
      root.appendChild(style);

      container.appendChild(host);
      return { host, root };
    }

    /** IFR-09: spinner shows after 150ms delay; hidden on connect. */
    export function createSpinnerOverlay(): {
      element: HTMLElement;
      show: () => void;
      hide: () => void;
    } {
      const overlay = document.createElement("div");
      overlay.className = "obc-spinner-overlay";
      overlay.style.display = "none";

      const spinner = document.createElement("div");
      spinner.className = "obc-spinner";
      overlay.appendChild(spinner);

      let timer: ReturnType<typeof setTimeout> | null = null;

      return {
        element: overlay,
        show() {
          timer = setTimeout(() => {
            overlay.style.display = "flex";
          }, 150);
        },
        hide() {
          if (timer !== null) {
            clearTimeout(timer);
            timer = null;
          }
          overlay.style.display = "none";
        },
      };
    }
    ```

    **File 2: `src/ui/scroll-lock.ts`** — copy from 02-RESEARCH.md "Body Scroll Lock" section:

    ```typescript
    /** IFR-10: lock body scroll; return a restorer to call on every close path. */
    export function lockBodyScroll(): () => void {
      const prev = document.body.style.overflow;
      document.body.style.overflow = "hidden";
      return () => {
        document.body.style.overflow = prev;
      };
    }
    ```

    **File 3: `src/ui/styles.test.ts`** — FIRST LINE must be `// @vitest-environment happy-dom`. At least **8 tests**:

    1. createShadowHost default z-index 9000 on host.style.zIndex
    2. createShadowHost custom z-index on host.style.zIndex
    3. host.style.position === "fixed"
    4. root is a ShadowRoot (instanceof or `root.host === host`)
    5. Shadow root contains a <style> with ":host { all: initial" substring
    6. host is appended to the container (`container.contains(host) === true`)
    7. createSpinnerOverlay().element.style.display is initially "none"
    8. createSpinnerOverlay show()/hide() with fake timers: show → advance 150ms → display === "flex"; hide → display === "none"; show then hide BEFORE 150ms → still "none"

    Use `vi.useFakeTimers()` / `vi.useRealTimers()` for spinner tests.

    **Note: `lockBodyScroll` is tested inside modal tests (Task 4)**, not here — it only takes meaning in the context of the modal lifecycle.
  </action>

  <verify>
    <automated>pnpm vitest run src/ui/styles.test.ts</automated>
  </verify>

  <acceptance_criteria>
    - `src/ui/styles.ts` exports `createShadowHost`, `ShadowHostResult`, `createSpinnerOverlay`
    - `src/ui/styles.ts` contains the literal `attachShadow({ mode: "open" })`
    - `src/ui/styles.ts` contains the literal `z-index:${zIndex}`
    - `src/ui/styles.ts` contains the literal `:host {` and `all: initial`
    - `src/ui/styles.ts` contains the literal `@keyframes obc-spin`
    - `src/ui/styles.ts` contains the literal `#2563eb`
    - `src/ui/styles.ts` contains the literal `setTimeout` with `150` arg (spinner delay)
    - `src/ui/styles.ts` does NOT contain `innerHTML`
    - `src/ui/scroll-lock.ts` exports `lockBodyScroll`
    - `src/ui/scroll-lock.ts` contains the literal `document.body.style.overflow = "hidden"`
    - `src/ui/styles.test.ts` first line contains `@vitest-environment happy-dom`
    - `src/ui/styles.test.ts` contains at least 8 `it(` or `test(` calls
    - `pnpm vitest run src/ui/styles.test.ts` exits 0
    - `pnpm typecheck` exits 0
    - `pnpm lint` exits 0
  </acceptance_criteria>

  <done>
    Shadow host with CSS reset, z-index on light DOM, spinner overlay with 150ms delay, scroll-lock helper — all committed with 8+ passing tests.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Iframe factory with same-origin guard, WCAG title, sandbox (IFR-01, IFR-03..08)</name>
  <files>src/ui/iframe.ts, src/ui/iframe.test.ts</files>

  <read_first>
    - .planning/phases/02-core-implementation/02-RESEARCH.md (Pattern 5: Iframe Factory with Same-Origin Guard)
    - .planning/phases/02-core-implementation/02-CONTEXT.md
    - src/errors.ts
  </read_first>

  <behavior>
    - buildIframe(cap, params) returns an HTMLIFrameElement
    - Thrown OBCError(SAME_ORIGIN_CAPABILITY) when new URL(cap.path).origin === location.origin
    - iframe.title === cap.appName (WCAG 2.4.1)
    - iframe.getAttribute("sandbox") === "allow-scripts allow-same-origin allow-forms allow-popups"
    - iframe.getAttribute("allow") === "clipboard-read; clipboard-write"
    - iframe.src URL contains: clientUrl={params.clientUrl}, id={params.id}, type={params.type}
    - When params.allowedMimeType is set, iframe.src URL contains allowedMimeType
    - When params.allowedMimeType is undefined, the param is absent from the URL
    - When params.multiple is true/false, iframe.src URL contains multiple={value}
    - iframe.style.cssText contains "width:min(90vw,800px)" and "height:min(85vh,600px)" and "border-radius:8px"
  </behavior>

  <action>
    Create `src/ui/iframe.ts` — copy Pattern 5 from 02-RESEARCH.md verbatim (lines ~507-565), adjusted for double quotes:

    ```typescript
    import { OBCError } from "../errors.js";
    import type { Capability } from "../types.js";

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
      if (typeof location !== "undefined" && capOrigin === location.origin) {
        throw new OBCError(
          "SAME_ORIGIN_CAPABILITY",
          `Capability "${capability.appName}" (${capability.path}) shares origin with host page. ` +
            "Same-origin capabilities are blocked because allow-scripts + allow-same-origin nullifies sandboxing."
        );
      }

      const iframe = document.createElement("iframe");

      // IFR-03: query params
      const url = new URL(capability.path);
      url.searchParams.set("clientUrl", params.clientUrl);
      url.searchParams.set("id", params.id);
      url.searchParams.set("type", params.type);
      if (params.allowedMimeType !== undefined) {
        url.searchParams.set("allowedMimeType", params.allowedMimeType);
      }
      if (params.multiple !== undefined) {
        url.searchParams.set("multiple", String(params.multiple));
      }
      iframe.src = url.toString();

      // IFR-04: sandbox
      iframe.setAttribute("sandbox", "allow-scripts allow-same-origin allow-forms allow-popups");

      // IFR-05: permissions policy
      iframe.setAttribute("allow", "clipboard-read; clipboard-write");

      // IFR-06: WCAG 2.4.1 accessible name — one line, hard fail without it
      iframe.title = capability.appName;

      // IFR-07: centered responsive sizing
      // Two load-bearing rules (see 02-RESEARCH.md Pitfall 10 + Pitfall 11):
      //   - position:fixed + translate centers in viewport (the shadow host wrapper
      //     is fullscreen but does not center its children)
      //   - pointer-events:auto is MANDATORY to override the shadow host's
      //     pointer-events:none wrapper (hotfix 0.1.1). Without it, clicks pass
      //     through to the body and capability buttons become inert.
      // Use setAttribute('style', ...) not style.cssText — some DOM impls normalize
      // min() values away when assigned via cssText.
      iframe.setAttribute(
        "style",
        "position:fixed;" +
          "top:50%;" +
          "left:50%;" +
          "transform:translate(-50%,-50%);" +
          "display:block;" +
          "width:min(90vw,800px);" +
          "height:min(85vh,600px);" +
          "border:none;" +
          "border-radius:8px;" +
          "box-shadow:0 4px 32px rgba(0,0,0,0.24);" +
          "pointer-events:auto;",
      );

      return iframe;
    }
    ```

    **Biome note:** The same-origin check uses `new URL(capability.path)` (no parameter reassignment — avoids Pitfall 8). Template literals for error messages are fine; concatenation for the inline style avoids multi-line template literal whitespace handling inside `style.cssText`.

    Create `src/ui/iframe.test.ts` with FIRST LINE `// @vitest-environment happy-dom`. At least **8 tests**:

    1. Happy path: cap with cross-origin path returns HTMLIFrameElement
    2. iframe.title === cap.appName
    3. iframe sandbox attribute has exact string `allow-scripts allow-same-origin allow-forms allow-popups`
    4. iframe allow attribute has exact string `clipboard-read; clipboard-write`
    5. Same-origin guard: set `location` (happy-dom allows `window.happyDOM.setURL("https://cap.example.com")` or construct a cap at current origin). Build a capability with `path: "https://cap.example.com/picker"` and assert buildIframe throws `OBCError` with `err.code === "SAME_ORIGIN_CAPABILITY"`.
    6. URL params: construct with `{ clientUrl: "https://host.example.com", id: "sess-1", type: "PICK", allowedMimeType: "image/png", multiple: true }`. Parse `new URL(iframe.src).searchParams` — assert all five values.
    7. URL params omit allowedMimeType when undefined: `.has("allowedMimeType") === false`
    8. iframe.style.cssText contains substrings `"width:min(90vw,800px)"`, `"height:min(85vh,600px)"`, `"border-radius:8px"`

    For the same-origin test, use happy-dom's `window.happyDOM.setURL(url)` (API reference: https://github.com/capricorn86/happy-dom). If that API is unavailable in v20.8.9, fall back to `Object.defineProperty(window, "location", { value: { origin: "https://cap.example.com" }, writable: true })` BEFORE buildIframe is called, and restore in `afterEach`.
  </action>

  <verify>
    <automated>pnpm vitest run src/ui/iframe.test.ts</automated>
  </verify>

  <acceptance_criteria>
    - `src/ui/iframe.ts` exports `buildIframe` and `IframeParams`
    - `src/ui/iframe.ts` contains the literal `SAME_ORIGIN_CAPABILITY`
    - `src/ui/iframe.ts` contains the literal `new URL(capability.path).origin`
    - `src/ui/iframe.ts` contains the literal `iframe.title = capability.appName`
    - `src/ui/iframe.ts` contains the literal `allow-scripts allow-same-origin allow-forms allow-popups`
    - `src/ui/iframe.ts` contains the literal `clipboard-read; clipboard-write`
    - `src/ui/iframe.ts` contains the literal `width:min(90vw,800px)`
    - `src/ui/iframe.ts` contains the literal `border-radius:8px`
    - `src/ui/iframe.ts` contains the literal `position:fixed` (hotfix 0.1.1 — viewport centering)
    - `src/ui/iframe.ts` contains the literal `transform:translate(-50%,-50%)` (hotfix 0.1.1)
    - `src/ui/iframe.ts` contains the literal `pointer-events:auto` (hotfix 0.1.1 — overrides shadow host pointer-events:none)
    - `src/ui/iframe.ts` does NOT contain the literal `innerHTML`
    - `src/ui/iframe.ts` does NOT contain the literal `penpal`
    - `src/ui/iframe.test.ts` first line contains `@vitest-environment happy-dom`
    - `src/ui/iframe.test.ts` contains at least 10 `it(` or `test(` calls (8 original + 2 regression tests for hotfix 0.1.1)
    - `src/ui/iframe.test.ts` contains the literal `pointer-events:auto` (regression guard)
    - `src/ui/iframe.test.ts` contains the literal `position:fixed` (regression guard)
    - `pnpm vitest run src/ui/iframe.test.ts` exits 0
    - `pnpm typecheck` exits 0
    - `pnpm lint` exits 0
  </acceptance_criteria>

  <done>
    Iframe factory with same-origin guard, WCAG title, sandbox + allow attrs, responsive sizing, and query params — all verified in happy-dom.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 3: Shadow-DOM-aware focus trap (UI-06, UI-07)</name>
  <files>src/ui/focus-trap.ts, src/ui/focus-trap.test.ts</files>

  <read_first>
    - .planning/phases/02-core-implementation/02-RESEARCH.md (Spike 1: Shadow DOM Focus Trap under Sandbox Constraints, Pitfall 3: document.activeElement Inside Shadow DOM)
    - .planning/phases/02-core-implementation/02-CONTEXT.md
  </read_first>

  <behavior>
    - trapFocus(root, container) adds a keydown listener on the SHADOW ROOT (not document)
    - trapFocus returns a release function; release() removes the keydown listener
    - Tab at the last focusable element wraps to the first
    - Shift+Tab at the first focusable element wraps to the last
    - Tab with no focusables in container calls e.preventDefault() and does not crash
    - Focus comparison uses root.activeElement (not document.activeElement)
    - trapFocus auto-focuses the first focusable inside rAF
  </behavior>

  <action>
    Create `src/ui/focus-trap.ts` — copy Spike 1 result from 02-RESEARCH.md verbatim (lines ~855-928), adjusted for double quotes:

    ```typescript
    const FOCUSABLE_SELECTORS = [
      "a[href]",
      "button:not([disabled])",
      "input:not([disabled])",
      "select:not([disabled])",
      "textarea:not([disabled])",
      "[tabindex]:not([tabindex=\"-1\"])",
      "[role=\"option\"][tabindex=\"0\"]",
    ].join(",");

    /**
     * Installs a focus trap inside `container` queried from `root` (ShadowRoot).
     * Returns a cleanup function that removes the trap.
     *
     * WCAG 2.1 SC 2.1.2 compliance: focus cannot leave the modal while open.
     * Uses root.querySelectorAll (not document.querySelectorAll) so focusable
     * elements inside the shadow boundary are correctly found.
     * Uses root.activeElement (not document.activeElement) because the latter
     * returns the shadow host, not the focused element inside the shadow tree.
     */
    export function trapFocus(
      root: ShadowRoot,
      container: HTMLElement
    ): () => void {
      function getFocusableElements(): HTMLElement[] {
        return Array.from(
          root.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTORS)
        ).filter((el) => container.contains(el));
      }

      function onKeyDown(e: KeyboardEvent): void {
        if (e.key !== "Tab") return;

        const focusable = getFocusableElements();
        if (focusable.length === 0) {
          e.preventDefault();
          return;
        }

        const first = focusable[0];
        const last = focusable[focusable.length - 1];
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

      root.addEventListener("keydown", onKeyDown);

      const firstFocusable = getFocusableElements()[0];
      requestAnimationFrame(() => {
        firstFocusable?.focus();
      });

      return () => {
        root.removeEventListener("keydown", onKeyDown);
      };
    }
    ```

    **Note on noUncheckedIndexedAccess**: `focusable[0]` and `focusable[focusable.length - 1]` return `HTMLElement | undefined`. The optional chaining `first?.focus()` and `last?.focus()` handles the undefined case. Do NOT remove the `?.` — it's required by the compiler.

    Create `src/ui/focus-trap.test.ts` with FIRST LINE `// @vitest-environment happy-dom`. Copy the test patterns from 02-RESEARCH.md (lines ~938-998) — at least **5 tests**:

    1. Tab wraps from last to first (assert via `root.activeElement === buttons[0]` after dispatchEvent)
    2. Shift+Tab wraps from first to last
    3. release() removes the keydown listener (dispatch a Tab event after release, verify no error thrown)
    4. Tab with empty container (no focusables inside) does not throw
    5. Auto-focus first element on setup — use `vi.useFakeTimers()`, wrap `requestAnimationFrame` in a Promise, or call `vi.advanceTimersByTime(0)` then assert `root.activeElement === firstButton`

    **happy-dom caveat from RESEARCH.md line 1001**: happy-dom's `focus()` may not update `root.activeElement` synchronously in all cases. If tests fail, fall back to checking `document.activeElement` or `el.matches(":focus")`. Document the fallback in the test file comments if used.

    Use a helper `setupShadowFixture()` in the test file: creates a div, appends to document.body, attaches open shadow, appends a container with two buttons, returns `{ host, root, container, buttons }`.
  </action>

  <verify>
    <automated>pnpm vitest run src/ui/focus-trap.test.ts</automated>
  </verify>

  <acceptance_criteria>
    - `src/ui/focus-trap.ts` exports `trapFocus`
    - `src/ui/focus-trap.ts` contains the literal `root.activeElement`
    - `src/ui/focus-trap.ts` does NOT contain the literal `document.activeElement`
    - `src/ui/focus-trap.ts` contains the literal `root.querySelectorAll`
    - `src/ui/focus-trap.ts` contains the literal `root.addEventListener("keydown"`
    - `src/ui/focus-trap.ts` contains the literal `requestAnimationFrame`
    - `src/ui/focus-trap.ts` does NOT contain the literal `document.querySelectorAll`
    - `src/ui/focus-trap.ts` does NOT contain the literal `penpal`
    - `src/ui/focus-trap.test.ts` first line contains `@vitest-environment happy-dom`
    - `src/ui/focus-trap.test.ts` contains at least 5 `it(` or `test(` calls
    - `pnpm vitest run src/ui/focus-trap.test.ts` exits 0
    - `pnpm typecheck` exits 0
    - `pnpm lint` exits 0
  </acceptance_criteria>

  <done>
    Focus trap queries from ShadowRoot, wraps Tab/Shift+Tab correctly, uses shadow-aware activeElement, release function removes listener.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 4: Chooser modal with full a11y baseline (UI-01..11, IFR-10)</name>
  <files>src/ui/modal.ts, src/ui/modal.test.ts</files>

  <read_first>
    - .planning/phases/02-core-implementation/02-RESEARCH.md (Pattern 6: Chooser Modal with Full A11y Baseline, navigateItems/focusItem helpers, Pitfall 5, Pitfall 6)
    - .planning/phases/02-core-implementation/02-CONTEXT.md (keyboard interaction block, visual defaults block)
    - src/ui/focus-trap.ts (for import path verification)
    - src/ui/scroll-lock.ts
  </read_first>

  <behavior>
    - buildModal(caps, callbacks, shadowRoot) returns { element, destroy }
    - element is a wrapper div containing a backdrop and a dialog div
    - dialog div has role="dialog", aria-modal="true", aria-labelledby pointing at a title node
    - Modal has a <h2 id="obc-modal-title"> with textContent "Choose an application"
    - Modal has a <ul role="listbox"> with one <li role="option" tabindex="0"> per capability
    - Each option renders capability.appName via textContent (NOT innerHTML)
    - Modal has a <button> with textContent "Cancel"
    - Cancel button click invokes callbacks.onCancel
    - Option click invokes callbacks.onSelect(capability)
    - Option Enter key invokes callbacks.onSelect(capability)
    - ESC key on shadowRoot invokes callbacks.onCancel
    - Backdrop click (e.target === backdrop) invokes callbacks.onCancel
    - Backdrop click on a child element (not backdrop itself) does NOT invoke onCancel
    - Arrow keys navigate between option items
    - destroy() removes the keydown listener on shadowRoot, releases the focus trap, removes the style tag and wrapper
    - Modal keydown listener is on shadowRoot (not document) per Pitfall 5
    - Body scroll is locked on open, restored on destroy()
  </behavior>

  <action>
    Create `src/ui/modal.ts` — adapt Pattern 6 from 02-RESEARCH.md verbatim (lines ~573-698), with these specific additions:

    1. Import `trapFocus` from `./focus-trap.js`
    2. Import `lockBodyScroll` from `./scroll-lock.js`
    3. Call `lockBodyScroll()` inside `buildModal` BEFORE appending anything; store the restorer; call it in `destroy`
    4. Use **double quotes** everywhere
    5. Use **textContent** everywhere (never innerHTML)
    6. Include the `navigateItems` and `focusItem` helper functions at the bottom of the file (not exported)

    Exact signature:

    ```typescript
    import type { Capability } from "../types.js";
    import { trapFocus } from "./focus-trap.js";
    import { lockBodyScroll } from "./scroll-lock.js";

    export interface ModalCallbacks {
      onSelect: (capability: Capability) => void;
      onCancel: () => void;
    }

    export interface ModalResult {
      element: HTMLElement;
      destroy: () => void;
    }

    export function buildModal(
      capabilities: Capability[],
      callbacks: ModalCallbacks,
      shadowRoot: ShadowRoot
    ): ModalResult {
      // UI-07 (WCAG 2.4.3): capture the element that held focus BEFORE the modal opens
      // so destroy() can restore it when the modal closes. This runs before any DOM
      // mutation or scroll lock so the snapshot is the true pre-modal focus owner.
      const previousFocus = document.activeElement as HTMLElement | null;

      const restoreScroll = lockBodyScroll();

      const style = document.createElement("style");
      style.textContent = getModalStyles();
      shadowRoot.appendChild(style);

      // UI-03: role="dialog" + aria-modal + aria-labelledby
      const dialog = document.createElement("div");
      dialog.setAttribute("role", "dialog");
      dialog.setAttribute("aria-modal", "true");
      dialog.setAttribute("aria-labelledby", "obc-modal-title");
      dialog.className = "obc-modal";

      const title = document.createElement("h2");
      title.id = "obc-modal-title";
      title.textContent = "Choose an application";
      dialog.appendChild(title);

      const list = document.createElement("ul");
      list.setAttribute("role", "listbox");
      list.setAttribute("aria-label", "Available applications");
      dialog.appendChild(list);

      for (const cap of capabilities) {
        const item = document.createElement("li");
        item.setAttribute("role", "option");
        item.setAttribute("tabindex", "0");
        item.dataset["capId"] = cap.id;

        const name = document.createElement("span");
        name.textContent = cap.appName;  // UI-10: textContent, never innerHTML
        item.appendChild(name);

        item.addEventListener("click", () => callbacks.onSelect(cap));
        item.addEventListener("keydown", (e) => {
          if (e.key === "Enter") callbacks.onSelect(cap);
        });
        list.appendChild(item);
      }

      // UI-02: visible cancel button
      const cancelBtn = document.createElement("button");
      cancelBtn.type = "button";
      cancelBtn.textContent = "Cancel";
      cancelBtn.addEventListener("click", callbacks.onCancel);
      dialog.appendChild(cancelBtn);

      // Backdrop (UI-05 dismiss source)
      const backdrop = document.createElement("div");
      backdrop.className = "obc-backdrop";
      backdrop.setAttribute("aria-hidden", "true");

      backdrop.addEventListener("click", (e) => {
        if (e.target === backdrop) callbacks.onCancel();
      });

      const wrapper = document.createElement("div");
      wrapper.className = "obc-wrapper";
      wrapper.appendChild(backdrop);
      wrapper.appendChild(dialog);

      // UI-04: ESC key on shadowRoot (Pitfall 5)
      const onKeyDown = (e: KeyboardEvent) => {
        if (e.key === "Escape") {
          e.preventDefault();
          callbacks.onCancel();
          return;
        }
        if (e.key === "ArrowDown" || e.key === "ArrowUp") {
          navigateItems(list, e.key);
        }
        if (e.key === "Home") focusItem(list, 0);
        if (e.key === "End") focusItem(list, -1);
      };
      shadowRoot.addEventListener("keydown", onKeyDown);

      // UI-06: focus trap
      const releaseTrap = trapFocus(shadowRoot, dialog);

      const destroy = () => {
        shadowRoot.removeEventListener("keydown", onKeyDown);
        releaseTrap();
        // UI-07 (WCAG 2.4.3): restore focus to the element that held it before buildModal().
        // The element may be detached by the time destroy() runs (host page re-rendered,
        // iframe removed, etc.) — swallow any focus() error in that case.
        try {
          previousFocus?.focus?.();
        } catch {
          // element may be detached; ignore
        }
        style.remove();
        wrapper.remove();
        restoreScroll();
      };

      return { element: wrapper, destroy };
    }

    function getModalStyles(): string {
      return `
        .obc-wrapper {
          position: fixed;
          inset: 0;
          pointer-events: auto;
        }
        .obc-backdrop {
          position: absolute;
          inset: 0;
          background: rgba(0, 0, 0, 0.5);
        }
        .obc-modal {
          position: absolute;
          top: 50%;
          left: 50%;
          transform: translate(-50%, -50%);
          min-width: 320px;
          max-width: 480px;
          width: 90vw;
          background: #ffffff;
          border-radius: 8px;
          padding: 24px;
          box-shadow: 0 4px 32px rgba(0, 0, 0, 0.24);
        }
        .obc-modal h2 {
          font-size: 18px;
          margin-bottom: 16px;
        }
        .obc-modal ul {
          list-style: none;
          margin-bottom: 16px;
        }
        .obc-modal li {
          padding: 12px;
          cursor: pointer;
          border-radius: 4px;
        }
        .obc-modal li:hover,
        .obc-modal li:focus {
          background: rgba(37, 99, 235, 0.1);
          outline: 2px solid #2563eb;
          outline-offset: -2px;
        }
        .obc-modal button {
          background: #2563eb;
          color: #ffffff;
          border: none;
          padding: 10px 20px;
          border-radius: 4px;
          cursor: pointer;
        }
        .obc-modal button:focus {
          outline: 2px solid #2563eb;
          outline-offset: 2px;
        }
      `;
    }

    function navigateItems(list: HTMLElement, direction: "ArrowDown" | "ArrowUp"): void {
      const items = Array.from(list.querySelectorAll<HTMLElement>("[role=\"option\"]"));
      const current = list.querySelector<HTMLElement>("[role=\"option\"]:focus");
      const idx = current ? items.indexOf(current) : -1;
      const next = direction === "ArrowDown"
        ? Math.min(idx + 1, items.length - 1)
        : Math.max(idx - 1, 0);
      items[next]?.focus();
    }

    function focusItem(list: HTMLElement, index: number): void {
      const items = Array.from(list.querySelectorAll<HTMLElement>("[role=\"option\"]"));
      const target = index === -1 ? items[items.length - 1] : items[index];
      target?.focus();
    }
    ```

    **Caller contract**: `buildModal` appends the style tag to `shadowRoot` and returns the `element` (wrapper) unattached. The caller (orchestrator) appends `element` to `shadowRoot`. This matches RESEARCH.md.

    Create `src/ui/modal.test.ts` with FIRST LINE `// @vitest-environment happy-dom`. At least **13 tests**:

    Setup helper: `function setup(caps: Capability[] = [...])` that creates a host, attaches shadow, calls `buildModal(caps, { onSelect, onCancel }, root)`, appends `element` to `root`, returns `{ host, root, element, destroy, onSelect, onCancel }`.

    1. role="dialog", aria-modal="true", aria-labelledby="obc-modal-title" all set on dialog element
    2. h2 with id="obc-modal-title" and textContent "Choose an application" exists
    3. One li[role="option"] per capability; textContent of child span === cap.appName
    4. Cancel button exists with textContent "Cancel"
    5. Cancel button click invokes onCancel once
    6. Option click invokes onSelect with the correct capability (spy assertion)
    7. Option Enter key invokes onSelect (dispatch `new KeyboardEvent("keydown", { key: "Enter", bubbles: true })` on the item)
    8. ESC key dispatched on shadowRoot invokes onCancel
    9. Backdrop click (where `e.target === backdrop`) invokes onCancel; click on a child element inside the modal (e.g. dialog) does NOT invoke onCancel
    10. destroy() removes wrapper from shadow (`shadowRoot.contains(wrapper) === false`)
    11. destroy() restores body scroll: before destroy, `document.body.style.overflow === "hidden"`; after destroy, it reverts to the pre-open value (empty string)
    12. Render with capability `appName: "<img src=x onerror=alert(1)>"` — assert the rendered span has `textContent` containing the literal string `"<img"` and the shadow root contains no actual `<img>` element (XSS guard via textContent)
    13. **UI-07 focus restoration (WCAG 2.4.3)**: create a `<button id="trigger">Open</button>` in `document.body`, call `trigger.focus()`, assert `document.activeElement === trigger`. Then call setup() to open the modal; assert focus moved into the modal (either an option item or the dialog, via `root.activeElement`). Then call `destroy()`. Assert `document.activeElement === trigger` — focus returned to the pre-open trigger element. Clean up the trigger in afterEach.

    Also assert that after setup, `document.body.style.overflow === "hidden"` (scroll lock on open).
  </action>

  <verify>
    <automated>pnpm vitest run src/ui/modal.test.ts</automated>
  </verify>

  <acceptance_criteria>
    - `src/ui/modal.ts` exports `buildModal`, `ModalCallbacks`, `ModalResult`
    - `src/ui/modal.ts` contains the literal `role", "dialog"` and `aria-modal", "true"` and `aria-labelledby`
    - `src/ui/modal.ts` contains the literal `textContent = cap.appName`
    - `src/ui/modal.ts` contains the literal `shadowRoot.addEventListener("keydown"`
    - `src/ui/modal.ts` contains the literal `trapFocus(shadowRoot, dialog)`
    - `src/ui/modal.ts` contains the literal `lockBodyScroll()`
    - `src/ui/modal.ts` contains the literal `restoreScroll()`
    - `src/ui/modal.ts` contains the literal `rgba(0, 0, 0, 0.5)` in modal styles
    - `src/ui/modal.ts` contains the literal `#2563eb` in modal styles
    - `src/ui/modal.ts` contains the literal `max-width: 480px` in modal styles
    - `src/ui/modal.ts` does NOT contain the literal `innerHTML`
    - `src/ui/modal.ts` does NOT contain the literal `penpal`
    - `src/ui/modal.ts` does NOT contain the literal `document.addEventListener("keydown"` (Pitfall 5)
    - `src/ui/modal.ts` contains the literal `previousFocus` (UI-07 focus capture)
    - `src/ui/modal.ts` contains the literal `previousFocus?.focus` (UI-07 focus restore)
    - `src/ui/modal.test.ts` contains the literal `restores focus` (UI-07 test name)
    - `src/ui/modal.test.ts` first line contains `@vitest-environment happy-dom`
    - `src/ui/modal.test.ts` contains at least 13 `it(` or `test(` calls
    - `pnpm vitest run src/ui/modal.test.ts` exits 0
    - `pnpm typecheck` exits 0
    - `pnpm lint` exits 0
  </acceptance_criteria>

  <done>
    Modal renders dialog with full ARIA, handles cancel/select/ESC/backdrop/keyboard nav, locks body scroll, uses textContent XSS-safe, attaches keydown to shadowRoot — all verified.
  </done>
</task>

</tasks>

<verification>
All four tasks produce green test runs. No UI file imports `penpal`. `focus-trap.ts` uses `root.activeElement`, never `document.activeElement`. `modal.ts` attaches keydown to shadowRoot, never document. `styles.ts` sets `z-index` on the light DOM host element. `iframe.ts` throws on same-origin BEFORE creating the iframe.
</verification>

<success_criteria>
- `pnpm vitest run src/ui/` exits 0 with 34+ tests (8 styles + 8 iframe + 5 focus-trap + 13 modal)
- `pnpm run ci` exits 0
- `grep -r "penpal" src/ui/` returns nothing
- `grep -r "document.activeElement" src/ui/focus-trap.ts` returns nothing
- `grep -r "innerHTML" src/ui/modal.ts src/ui/iframe.ts` returns nothing
- All 21 requirement IDs (IFR-01..10, UI-01..11) covered
</success_criteria>

<output>
After completion, create `.planning/phases/02-core-implementation/02-03-SUMMARY.md`. Record:
- Per-file test counts
- Whether happy-dom 20.8.9 supported `root.activeElement` synchronously in focus-trap tests, or required the `document.activeElement` / `el.matches(":focus")` fallback
- How the same-origin guard test manipulated `location` (happyDOM.setURL vs Object.defineProperty)
- Any Biome rule violations encountered and resolved (e.g., noParameterAssign on URL objects)
</output>
