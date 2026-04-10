# Changelog

All notable changes to `@openburo/client` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.1] — 2026-04-10

### Fixed

- **Critical: capability iframe clicks no longer pass through to the host page.**
  `buildIframe()` in `src/ui/iframe.ts` now sets `pointer-events: auto` on the
  injected iframe's inline style. Without this, the shadow host wrapper's
  `pointer-events: none` (used so the invisible fullscreen overlay does not
  block clicks on the host page) cascades to the iframe descendant, and CSS
  hit-testing silently routes every click to the element behind the iframe.
  The capability UI appeared frozen: buttons did not respond, Penpal RPC
  never fired, and the only visible feedback was the iframe itself rendering
  correctly. Discovered during manual browser smoke testing after the v1.0
  milestone was marked complete.

- **Iframe now centered in the viewport instead of rendering at the top-left.**
  The inline style now sets `position: fixed; top: 50%; left: 50%;
  transform: translate(-50%, -50%)`. Previously the iframe relied on being
  inside a centered shadow host wrapper, but the wrapper was `inset: 0`
  fullscreen with no flex/grid centering, so the default `position: static`
  iframe landed at the wrapper's top-left corner (which is the viewport
  top-left). `IFR-07` in the requirements explicitly called for "centered
  responsive sizing" — this fix restores the intended behavior.

### Tests

- Added 2 regression tests in `src/ui/iframe.test.ts` that assert the literal
  presence of `pointer-events:auto`, `position:fixed`,
  `transform:translate(-50%,-50%)`, `top:50%`, and `left:50%` in the iframe's
  raw `style` attribute. These are grep-level assertions — happy-dom has no
  layout engine so we cannot test the real hit-testing behavior in unit tests.
  Full fix validation requires a Playwright smoke test, tracked for v2.

### Documentation

- Added Pitfall 10 (iframe positioning without `position: fixed`) and
  Pitfall 11 (`pointer-events: none` cascade to descendants) to
  `.planning/phases/02-core-implementation/02-RESEARCH.md` so this specific
  trap is captured for future maintainers.
- Clarified `IFR-07` in `REQUIREMENTS.md` to explicitly call out both
  `position: fixed` and `pointer-events: auto` as load-bearing.
- Updated `02-VERIFICATION.md` and `04-VERIFICATION.md` with retroactive
  notes. Both remain `passed` because the core logic requirements are still
  covered — only the test coverage gap for real-browser hit-testing is
  newly documented.
- Added a new `## Known Issues & Post-Milestone Fixes` section to
  `PROJECT.md` so the hotfix is discoverable from the top-level project doc.

### Unchanged

- No API changes. Consumers do not need to modify any code.
- No behavior changes beyond the two fixed bugs — this is a pure hotfix.
- Build outputs (ESM, CJS, UMD, type declarations, exports map) are identical
  in shape. `@arethetypeswrong/cli` remains clean across all four resolution
  modes (node10, node16 CJS, node16 ESM, bundler).

## [0.1.0] — 2026-04-10

Initial release. Milestone v1.0 covers:

- `OpenBuroClient` public API: `castIntent`, `getCapabilities`,
  `refreshCapabilities`, `destroy`.
- Capability layer: HTTP loader with `AbortSignal` + HTTPS guard, pure MIME
  resolver with wildcard support, WebSocket listener with full-jitter
  exponential backoff and `destroyed` guard.
- Intent layer: pure `planCast()` discriminated union, `ActiveSession` type
  for orchestrator session map.
- Lifecycle: `createAbortContext()` helper for leak-free teardown composition.
- UI layer: Shadow DOM host, iframe factory with same-origin guard + WCAG
  title, Shadow-DOM-aware focus trap, full-a11y chooser modal with ESC /
  backdrop / arrow keys / Home / End / Enter / focus restore / scroll lock.
- Messaging layer: `BridgeAdapter` interface, `MockBridge` test double,
  `PenpalBridge` using Penpal v7 `connect` + `WindowMessenger`.
- Distribution: ESM + CJS + UMD builds via tsdown, nested `types` per exports
  condition, `@arethetypeswrong/cli` CI gate.
- Capability author integration guide at `docs/capability-authors.md`.

147 unit + integration tests. `pnpm run ci` green end-to-end.
