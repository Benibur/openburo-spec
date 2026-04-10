# Phase 1: Foundations - Research

**Researched:** 2026-04-10
**Domain:** TypeScript library scaffolding — tsdown + Vitest 4 + Biome + TypeScript 6 + Penpal v7 pin + UUID fallback + attw CI gate
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

All implementation choices are at Claude's discretion — pure infrastructure phase. Research (`.planning/research/STACK.md`, `SUMMARY.md`) and the critical cross-cutting findings in REQUIREMENTS.md already lock:
- **Bundler**: tsdown (not tsup — deprecated)
- **Test runner**: Vitest 4
- **Lint/format**: Biome
- **TypeScript**: 6.x, target `ES2020`, `moduleResolution: bundler`, `noEmit: true`
- **Penpal**: v7, pinned to exact version (no `^` range)
- **UUID**: `crypto.randomUUID()` with inline `getRandomValues` fallback (covers Chrome 90 / FF 88 / Safari 14 floor)
- **Exports map**: nested `types` field per `import`/`require` condition
- **CI gate**: `@arethetypeswrong/cli --pack`
- **Package name**: `@openburo/client`

### Claude's Discretion

Claude chooses all remaining details: file naming conventions, test file layout, Biome config strictness, tsconfig fine print, repo layout within `open-buro-client/`.

### Deferred Ideas (OUT OF SCOPE)

- Vendoring Penpal source (decided: pin exact version first, vendor only if supply-chain posture tightens)
- CI workflow definition (GitHub Actions, etc.) — delivered alongside the `attw` gate in Phase 4 Distribution, not Phase 1 scaffold
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| FOUND-01 | Project scaffolds with tsdown, TypeScript 6, Vitest 4, Biome, npm package name `@openburo/client` | tsdown 0.21.7, TS 6.0.2, Vitest 4.1.4, Biome 2.4.11 verified on npm; exact config files provided in Code Examples section |
| FOUND-02 | `OBCError` class exported with code + message + optional cause | Standard `Error` subclass pattern; exact implementation in Code Examples |
| FOUND-03 | `OBCErrorCode` union type covers `CAPABILITIES_FETCH_FAILED`, `NO_MATCHING_CAPABILITY`, `IFRAME_TIMEOUT`, `WS_CONNECTION_FAILED`, `INTENT_CANCELLED`, `SAME_ORIGIN_CAPABILITY` | Six codes; `SAME_ORIGIN_CAPABILITY` added during research; all defined in `src/errors.ts` |
| FOUND-04 | Shared types exported: `Capability`, `IntentRequest`, `IntentResult`, `FileResult`, `IntentCallback`, `OBCOptions`, `CastPlan` | All seven interfaces documented in Code Examples for `src/types.ts` |
| FOUND-05 | `generateSessionId()` uses `crypto.randomUUID()` with inline `getRandomValues` fallback | Chrome 90-91 / FF 88-94 / Safari 14-15.3 gap confirmed via caniuse; exact 3-line fallback snippet provided |
| FOUND-06 | Penpal pinned to exact version (no `^` range) in `package.json` | Penpal 7.0.6 is latest stable (published 2026-02-13); exact pin syntax shown |
| FOUND-07 | `@arethetypeswrong/cli` runs as CI gate on every build | `attw --pack` command verified; version 0.18.2 on npm; local invocation pattern documented |
</phase_requirements>

---

## Summary

Phase 1 is a pure scaffolding phase: no application logic, only infrastructure. Every file it produces is a config file, a type definition, or a one-function utility. The research goal is not "what stack?" (STACK.md already answered that) but "what exactly goes in each file?"

The research confirms all locked stack versions against npm as of 2026-04-10: tsdown 0.21.7, TypeScript 6.0.2, Vitest 4.1.4, Biome 2.4.11, Penpal 7.0.6, `@arethetypeswrong/cli` 0.18.2. The tsdown config requires a two-array form to differentiate ESM/CJS (external Penpal) from UMD (Penpal bundled). The TypeScript 6 tsconfig has three values that differ from TS 5 defaults and must be set explicitly: `target: "ES2020"` (TS6 default shifted to es2025), `rootDir: "./src"` (TS6 default changed), and `types: []` (TS6 no longer auto-discovers `@types/*`). The `getRandomValues` UUID fallback covers the exact browser gap — Chrome 90-91, Firefox 88-94, Safari 14-15.3 all lack `crypto.randomUUID()` but all have `crypto.getRandomValues()` from v37+ / v21+ / v6.1+ respectively.

**Primary recommendation:** Write the eleven deliverable files in order — `package.json` → `tsconfig.json` → `tsdown.config.ts` → `biome.json` → `vitest.config.ts` → `.gitignore` → `src/errors.ts` → `src/types.ts` → `src/intent/id.ts` → `src/index.ts` → `src/intent/id.test.ts`. Run `pnpm install && pnpm build && pnpm test` to verify FOUND-01, then `pnpm dlx @arethetypeswrong/cli --pack` to verify FOUND-07.

---

## Standard Stack

### Core

| Library | Version (npm-verified) | Purpose | Why Standard |
|---------|----------------------|---------|--------------|
| typescript | 6.0.2 | Source language + type-check | TS6 released March 2026; strict mode on by default; `noEmit: true` with tsdown handling emit |
| tsdown | 0.21.7 | Library bundler (ESM + CJS + UMD) | Only tool in 2026 supporting all three formats natively without plugins; Rolldown-based successor to deprecated tsup |
| vitest | 4.1.4 | Unit + integration tests | Native ESM, TS-native, 10-20x faster than Jest; Browser Mode stable in v4 |
| @biomejs/biome | 2.4.11 | Lint + format | Single binary, zero peer-deps, 450+ TS-aware rules; replaces ESLint + Prettier |
| penpal | 7.0.6 | Parent↔iframe PostMessage | Pinned exact (no `^`); published 2026-02-13; v7 `connect` + `WindowMessenger` API |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| @vitest/coverage-v8 | 4.1.4 | Coverage reports | Always — `--coverage` in CI; v8 provider needs no extra deps |
| happy-dom | 20.8.9 | DOM simulation in Vitest | For Phase 2+ tests touching DOM APIs; not needed in Phase 1 (no DOM code yet) |
| @arethetypeswrong/cli | 0.18.2 | Validate published exports map | CI gate — run `attw --pack` before every publish |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| tsdown | tsup | tsup has a "not actively maintained" warning in its own README pointing to tsdown; do not use |
| tsdown | Rollup 4 (raw) | Rollup is the fallback if tsdown pre-1.0 hits a regression; adds ~1 sprint of config |
| Biome | ESLint + Prettier | Only worth it if framework-specific plugins (react-hooks) are needed; OBC has none |
| inline UUID fallback | `uuid` npm package | `uuid` adds a supply-chain dependency to a library that ships to third parties; inline 3 lines instead |

**Installation (pnpm — recommended for exact-pin discipline):**

```bash
cd open-buro-client

# Runtime dependency
pnpm add penpal@7.0.6          # exact, no ^ — FOUND-06

# Dev: build
pnpm add -D tsdown typescript

# Dev: test
pnpm add -D vitest @vitest/coverage-v8 happy-dom

# Dev: lint/format
pnpm add -D @biomejs/biome

# Dev: publish validation
pnpm add -D @arethetypeswrong/cli
```

**Version verification (run before writing package.json):**

```bash
npm view penpal version          # → 7.0.6  (verified 2026-04-10)
npm view tsdown version          # → 0.21.7 (verified 2026-04-10)
npm view typescript version      # → 6.0.2  (verified 2026-04-10)
npm view vitest version          # → 4.1.4  (verified 2026-04-10)
npm view @biomejs/biome version  # → 2.4.11 (verified 2026-04-10)
npm view @arethetypeswrong/cli version  # → 0.18.2 (verified 2026-04-10)
```

---

## Architecture Patterns

### Recommended Project Structure

```
open-buro-client/
├── src/
│   ├── index.ts          # Public barrel — re-exports OBCError, types, generateSessionId
│   ├── errors.ts         # OBCError class + OBCErrorCode union
│   ├── types.ts          # All shared interfaces (Capability, IntentRequest, …)
│   └── intent/
│       ├── id.ts         # generateSessionId() with getRandomValues fallback
│       └── id.test.ts    # Unit tests for FOUND-05 (UUID fallback path)
├── dist/                 # tsdown output (gitignored)
├── tsdown.config.ts      # Build config
├── tsconfig.json         # Type-check only (noEmit: true)
├── vitest.config.ts      # Test runner config
├── biome.json            # Lint + format config
├── package.json          # @openburo/client with exact Penpal pin
└── .gitignore
```

Later phases add subdirectories under `src/`: `capabilities/`, `ui/`, `messaging/`, `session/`. Phase 1 establishes only the shared contracts (errors, types, id).

### Pattern 1: Exact Version Pin in package.json

**What:** Penpal listed in `dependencies` without a version range prefix — `"penpal": "7.0.6"` not `"penpal": "^7.0.6"`.

**When to use:** Any single-maintainer transitive dependency in a library that ships to third-party host apps.

**Why:** npm range resolution means a `^7.0.6` pin can silently upgrade to `7.1.0` if the author publishes it. Supply-chain attacks on npm packages have been documented (CISA 2025 advisory). Exact pin makes the installed version auditable.

### Pattern 2: Two-Array tsdown Config (ESM/CJS vs UMD)

**What:** Export an array of two config objects from `tsdown.config.ts`. The first covers ESM + CJS with `external: ['penpal']` (let host deduplicate). The second covers UMD with Penpal bundled in (self-contained for CDN `<script>` usage).

**When to use:** Any library that targets both npm consumers (who want deduplication) and CDN `<script>` consumers (who want a single file).

### Pattern 3: TS6 Explicit tsconfig (no relying on defaults)

**What:** Set `target`, `rootDir`, and `types` explicitly even though TS6 has defaults for them — because those defaults changed in TS6 and will surprise anyone reading TS5 documentation.

**When to use:** Any new project on TypeScript 6.x.

### Anti-Patterns to Avoid

- **`"penpal": "^7.0.6"` in package.json:** Range pin defeats exact-version discipline. Use `"7.0.6"` literally.
- **`noEmit: false` in tsconfig:** tsdown handles emit; enabling tsc emit produces a parallel output directory that will confuse attw.
- **Top-level `"types"` only in exports map:** attw will flag `FallbackCondition`; nest `"types"` inside both `"import"` and `"require"` branches.
- **`"target": "ES5"` or `"target": "ESNext"` in tsconfig:** TS6 removed ES5 target; ESNext conflicts with the explicit ES2020 browser floor. Use `"ES2020"` literally.
- **`"type": "commonjs"` in package.json:** Conflicts with ESM primary output; use `"type": "module"`.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| PostMessage RPC | Custom `window.message` listener with origin checks + promise tracking | Penpal 7.0.6 (pinned exact) | Penpal handles MessagePort protocol, connection teardown, origin restriction, and Promise lifecycle; DIY version has dozens of edge cases (stale listeners, race on destroy, origin spoofing) |
| UUID generation | Custom random hex string | `crypto.randomUUID()` + inline `getRandomValues` fallback (3 lines) | RFC 4122 compliance, entropy from OS CSPRNG, zero dependencies |
| Lint + format | ESLint config + Prettier config + shared config package | Biome single binary | Two-tool setup adds 127+ packages and config drift; Biome is faster and has no peer-dep churn |
| exports map validation | Manual `node -e "require('@openburo/client')"` smoke tests | `attw --pack` | attw knows all 14 error conditions (CJSResolvesToESM, FallbackCondition, etc.) that consumers encounter; manual smoke tests miss the TypeScript resolution edge cases |

**Key insight:** Phase 1 creates the contracts that every other phase imports. Any hand-rolled UUID or error class that deviates from spec will propagate structural mistakes across all eight phases.

---

## Common Pitfalls

### Pitfall 1: TypeScript 6 Changed Defaults

**What goes wrong:** Developer writes a tsconfig without explicit `target`, `rootDir`, or `types` — trusting TS5-era mental model. TS6 emits to `es2025` target (breaking the ES2020 browser floor), loses `rootDir` inference (declaration paths break), and auto-discovers `@types/*` differently.

**Why it happens:** TS6 release notes mention the defaults change but most "TypeScript starter" templates haven't updated yet.

**How to avoid:** Use the exact tsconfig in the Code Examples section. Never omit `target`, `rootDir`, or `types` in a TS6 project.

**Warning signs:** `tsc --noEmit` passes but `attw --pack` reports `FallbackCondition`; declaration files appear at unexpected paths in `dist/`.

### Pitfall 2: Penpal Version Drift via `^` Range

**What goes wrong:** Package published with `"penpal": "^7.0.6"`. Author publishes `7.1.0` with a breaking internal change (even a non-semver-major one). Consumer installs OBC and gets `7.1.0` — behavior differs from what OBC was tested against.

**Why it happens:** npm range syntax defaults feel "safe" but are not for single-maintainer packages.

**How to avoid:** Use `"penpal": "7.0.6"` (no caret) in `package.json`. Document this in a comment.

**Warning signs:** `pnpm list penpal` shows a version other than `7.0.6` in consumer projects.

### Pitfall 3: UUID Fallback Not Tested (Coverage Gap)

**What goes wrong:** `generateSessionId()` works in modern test environments (Node 22 has `crypto.randomUUID()`). The fallback branch never executes during test runs. The fallback has a subtle operator-precedence bug and ships broken.

**Why it happens:** Test environments always have `crypto.randomUUID()`, so the `if (typeof crypto.randomUUID === 'function')` branch is always taken.

**How to avoid:** In the test file, delete `crypto.randomUUID` before importing the module (see Validation Architecture section for the exact technique). Assert that the output still matches the UUID v4 format regex.

**Warning signs:** Coverage report shows the fallback lines as uncovered; no test fails when the fallback snippet is corrupted.

### Pitfall 4: Penpal Bundled in ESM/CJS (Deduplication Failure)

**What goes wrong:** tsdown config has no `external` list. Penpal is bundled into `obc.esm.js`. A host app that also uses Penpal gets two Penpal instances, causing the handshake to fail with `CHILD_DESTROYED` or message routing errors.

**Why it happens:** tsdown bundles everything by default; `external` must be set explicitly for library builds.

**How to avoid:** First tsdown config object (ESM + CJS) must set `external: ['penpal']`. Only the UMD config object omits it.

**Warning signs:** `attw --pack` or `publint` may not catch this; it surfaces as a runtime `IFRAME_TIMEOUT` in host apps that have Penpal in their own bundle.

### Pitfall 5: `biome init` Overwrites Custom Config

**What goes wrong:** Developer runs `pnpm biome init` after manually writing `biome.json`. Biome resets all settings to defaults, removing the strict rules chosen for OBC.

**How to avoid:** Write `biome.json` from scratch using the content in Code Examples. Do not run `biome init` on a non-empty directory.

---

## Code Examples

Verified patterns from official sources and npm-verified versions.

### `package.json`

```jsonc
{
  "name": "@openburo/client",
  "version": "0.1.0",
  "description": "Framework-agnostic browser SDK for the OpenBuro intent/capability protocol",
  "type": "module",
  "main": "./dist/obc.cjs.js",
  "module": "./dist/obc.esm.js",
  "types": "./dist/types/index.d.ts",
  "exports": {
    ".": {
      "import": {
        "types": "./dist/types/index.d.ts",
        "default": "./dist/obc.esm.js"
      },
      "require": {
        "types": "./dist/types/index.d.cts",
        "default": "./dist/obc.cjs.js"
      }
    }
  },
  "files": ["dist"],
  "sideEffects": false,
  "scripts": {
    "build":      "tsdown",
    "typecheck":  "tsc --noEmit",
    "test":       "vitest run",
    "test:watch": "vitest",
    "coverage":   "vitest run --coverage",
    "lint":       "biome check .",
    "format":     "biome format --write .",
    "attw":       "attw --pack",
    "ci":         "tsc --noEmit && biome check . && vitest run && attw --pack"
  },
  "dependencies": {
    "penpal": "7.0.6"
  },
  "devDependencies": {
    "@arethetypeswrong/cli": "^0.18.2",
    "@biomejs/biome": "^2.4.11",
    "@vitest/coverage-v8": "^4.1.4",
    "happy-dom": "^20.8.9",
    "tsdown": "^0.21.7",
    "typescript": "^6.0.2",
    "vitest": "^4.1.4"
  },
  "engines": {
    "node": ">=22.18.0"
  }
}
```

Key notes:
- `"penpal": "7.0.6"` — exact pin, no `^` (FOUND-06)
- `"type": "module"` — `.js` files treated as ESM; CJS output uses `.cjs` extension
- `"types"` nested inside both `"import"` and `"require"` branches (FOUND-07 / attw gate)
- `"sideEffects": false` — enables tree-shaking for bundler consumers

### `tsconfig.json`

```jsonc
// tsconfig.json — type-checking only; tsdown handles emit
{
  "compilerOptions": {
    "target": "ES2020",            // explicit — TS6 default shifted to es2025
    "lib": ["ES2020", "DOM"],      // ES2020 + browser APIs
    "module": "ESNext",            // required with moduleResolution: bundler
    "moduleResolution": "bundler", // correct for tsdown / Vite / esbuild consumers
    "strict": true,                // TS6 default, but explicit for clarity
    "declaration": true,
    "declarationMap": true,
    "sourceMap": true,
    "outDir": "./dist",
    "rootDir": "./src",            // explicit — TS6 default rootDir changed
    "types": [],                   // TS6 no longer auto-discovers @types; opt in per-file
    "esModuleInterop": true,       // TS6 always-on; stating explicitly is harmless
    "skipLibCheck": true,
    "noEmit": true                 // tsdown handles emit; tsc only type-checks
  },
  "include": ["src"]
}
```

### `tsdown.config.ts`

```typescript
// tsdown.config.ts
import { defineConfig } from 'tsdown';

export default defineConfig([
  // ESM + CJS: Penpal external — host project provides it (deduplication)
  {
    entry: ['src/index.ts'],
    format: ['esm', 'cjs'],
    outDir: 'dist',
    target: 'es2020',
    platform: 'browser',
    dts: true,          // emit .d.ts + .d.cts declarations
    sourcemap: true,
    clean: true,
    treeshake: true,
    external: ['penpal'],
  },
  // UMD: bundle Penpal in — self-contained for CDN <script> usage
  {
    entry: ['src/index.ts'],
    format: ['umd'],
    outDir: 'dist',
    target: 'es2020',
    platform: 'browser',
    globalName: 'OpenBuroClient',
    minify: true,
    sourcemap: false,
    // penpal NOT in external → bundled into UMD
  },
]);
```

Note: `dts: true` in the first config causes tsdown to emit `.d.ts` alongside `.esm.js` and `.d.cts` alongside `.cjs.js` — matching the `exports` map in `package.json`.

### `biome.json`

```jsonc
{
  "$schema": "https://biomejs.dev/schemas/2.4.11/schema.json",
  "organizeImports": {
    "enabled": true
  },
  "linter": {
    "enabled": true,
    "rules": {
      "recommended": true,
      "correctness": {
        "noUnusedVariables": "error",
        "noUnusedImports": "error"
      },
      "style": {
        "useConst": "error",
        "noVar": "error"
      },
      "suspicious": {
        "noExplicitAny": "error"
      }
    }
  },
  "formatter": {
    "enabled": true,
    "indentStyle": "space",
    "indentWidth": 2,
    "lineWidth": 100
  },
  "javascript": {
    "formatter": {
      "quoteStyle": "single",
      "semicolons": "always",
      "trailingCommas": "all"
    }
  },
  "files": {
    "ignore": ["dist", "node_modules", "coverage"]
  }
}
```

### `vitest.config.ts`

```typescript
// vitest.config.ts
import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    environment: 'node',   // Phase 1 has no DOM tests; later phases override per-file
    include: ['src/**/*.test.ts'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'lcov'],
      include: ['src/**/*.ts'],
      exclude: ['src/**/*.test.ts'],
    },
  },
});
```

Note: `environment: 'node'` is correct for Phase 1. Phase 4 (UI layer) will add `environment: 'happy-dom'` to specific test files via `// @vitest-environment happy-dom` file-level pragma — no global vitest.config change needed.

### `.gitignore`

```gitignore
# Build output
dist/
coverage/

# Dependencies
node_modules/

# TypeScript
*.tsbuildinfo

# Editor
.vscode/
.idea/
*.swp

# OS
.DS_Store
Thumbs.db

# Pack artifacts (attw --pack creates these)
*.tgz
```

### `src/errors.ts`

```typescript
// src/errors.ts

export type OBCErrorCode =
  | 'CAPABILITIES_FETCH_FAILED'
  | 'NO_MATCHING_CAPABILITY'
  | 'IFRAME_TIMEOUT'
  | 'WS_CONNECTION_FAILED'
  | 'INTENT_CANCELLED'
  | 'SAME_ORIGIN_CAPABILITY';

export class OBCError extends Error {
  readonly code: OBCErrorCode;
  override readonly cause?: unknown;

  constructor(code: OBCErrorCode, message: string, cause?: unknown) {
    super(message, cause !== undefined ? { cause } : undefined);
    this.name = 'OBCError';
    this.code = code;
    this.cause = cause;
    // Fix prototype chain for instanceof checks across transpiled environments
    Object.setPrototypeOf(this, new.target.prototype);
  }
}
```

Note: `override readonly cause` re-declares the built-in `Error.cause` field with a more specific type. `Object.setPrototypeOf` ensures `instanceof OBCError` works even after transpilation via older toolchains that still touch prototype chains.

### `src/types.ts`

```typescript
// src/types.ts

export interface Capability {
  /** Unique identifier for this capability provider */
  id: string;
  /** Display name shown in the chooser modal */
  appName: string;
  /** Action this capability handles, e.g. "PICK" or "SAVE" */
  action: string;
  /** Full URL to the capability iframe endpoint */
  path: string;
  /** Optional URL to a capability icon (shown in chooser modal) */
  iconUrl?: string;
  properties: {
    /** Supported MIME types; '*/*' means any type */
    mimeTypes: string[];
  };
}

export interface IntentRequest {
  /** Action to perform, e.g. "PICK" or "SAVE" */
  action: string;
  args: {
    /** MIME type filter; absent or '*/*' matches all capabilities */
    allowedMimeType?: string;
    /** Whether to allow selecting multiple files */
    multiple?: boolean;
  };
}

export interface FileResult {
  /** Name of the selected/saved file */
  name: string;
  /** MIME type of the file */
  type: string;
  /** Size in bytes */
  size: number;
  /** URL or data URI for the file content */
  url: string;
}

export interface IntentResult {
  /** Unique session identifier (UUID v4) */
  id: string;
  /** 'done' when files were selected/saved; 'cancel' when dismissed */
  status: 'done' | 'cancel';
  results: FileResult[];
}

export type IntentCallback = (result: IntentResult) => void;

export interface OBCOptions {
  /** HTTPS URL to the OpenBuro capabilities endpoint */
  capabilitiesUrl: string;
  /** Whether to subscribe to live registry updates via WebSocket (default: auto-detect) */
  liveUpdates?: boolean;
  /** Explicit WebSocket URL; auto-derived from capabilitiesUrl if omitted */
  wsUrl?: string;
  /** DOM element into which the modal/backdrop is injected (default: document.body) */
  container?: HTMLElement;
  /** Called when capabilities are refreshed via WebSocket event */
  onCapabilitiesUpdated?: (capabilities: Capability[]) => void;
  /** Called on any OBCError */
  onError?: (error: import('./errors').OBCError) => void;
}

/** Discriminated union produced by planCast() in Phase 2 */
export type CastPlan =
  | { kind: 'no-match' }
  | { kind: 'direct'; capability: Capability }
  | { kind: 'select'; capabilities: Capability[] };
```

### `src/intent/id.ts`

```typescript
// src/intent/id.ts
// FOUND-05: crypto.randomUUID() with getRandomValues fallback
// Chrome 90-91, Firefox 88-94, Safari 14-15.3 lack randomUUID but have getRandomValues.

/**
 * Generate a UUID v4 string.
 * Uses crypto.randomUUID() when available (Chrome 92+, Firefox 95+, Safari 15.4+).
 * Falls back to crypto.getRandomValues() for older browsers in the stated support range.
 */
export function generateSessionId(): string {
  if (typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  // RFC 4122 v4 via getRandomValues — covers Chrome 90-91, FF 88-94, Safari 14-15.3
  return '10000000-1000-4000-8000-100000000000'.replace(/[018]/g, (c) =>
    (
      Number(c) ^
      (crypto.getRandomValues(new Uint8Array(1))[0] & (15 >> (Number(c) / 4)))
    ).toString(16),
  );
}
```

**Operator-precedence note:** The expression `15 >> Number(c) / 4` is parsed as `15 >> (Number(c) / 4)` in JavaScript (division binds tighter than shift). The parenthesization in the snippet above makes this explicit and matches the intended RFC 4122 v4 bit manipulation.

### `src/index.ts` (Phase 1 barrel)

```typescript
// src/index.ts — public API barrel (Phase 1 scope only)
// Later phases add their own exports here.

export { OBCError } from './errors';
export type { OBCErrorCode } from './errors';

export type {
  Capability,
  CastPlan,
  FileResult,
  IntentCallback,
  IntentRequest,
  IntentResult,
  OBCOptions,
} from './types';

export { generateSessionId } from './intent/id';
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| tsup | tsdown | 2025-2026 | tsup README now shows "not actively maintained" notice; tsdown is the Rolldown-based successor |
| `connectToChild` / `connectToParent` (Penpal v6) | `connect({ messenger: new WindowMessenger(...), methods })` | Penpal v7.0.0 (Nov 2024) | v6 API no longer exists in the package; spec pseudocode is wrong and will not compile |
| TypeScript 5 default `target: "ES3"` / `target: "ES5"` | TS6 default `target: "es2025"` | TS 6.0 (March 2026) | Must set `target: "ES2020"` explicitly or the output will use ES2025 features |
| `"types"` at top level of `exports` only | `"types"` nested inside each condition branch | TypeScript 4.7+ / enforced by attw | Top-level only is the `FallbackCondition` attw error; nested is the correct pattern |

**Deprecated/outdated:**
- `tsup`: Do not use. Deprecated in its own README.
- `"outFile"` tsconfig option: Removed in TypeScript 6.0. tsdown handles output concatenation.
- `"target": "ES5"` in tsconfig: Removed in TypeScript 6.0 (minimum is now ES2015).
- Penpal `connectToChild` / `connectToParent`: These exports do not exist in `penpal@7.x`. Any spec pseudocode using them is Phase 5's problem to translate; Phase 1 just pins the version.

---

## Open Questions

1. **pnpm vs npm for the project package manager**
   - What we know: `pnpm` enforces exact hoisting and works well with exact-pin dependencies; STACK.md installation commands use `npm install`
   - What's unclear: No `package-lock.json` or `pnpm-lock.yaml` preference was expressed
   - Recommendation: Use `pnpm` — its strict hoisting model is a better fit for a library with a single exact-pinned runtime dep; generate `pnpm-lock.yaml`

2. **`noUncheckedIndexedAccess` strictness**
   - What we know: TS6 strict mode does not enable `noUncheckedIndexedAccess` by default; it is a separate flag
   - What's unclear: Whether to enable it now (noisier code) or leave for a future tightening pass
   - Recommendation: Enable it in Phase 1 tsconfig — easier to enforce from day one than retrofit across 8 phases

3. **`publint` as a second CI gate alongside `attw`**
   - What we know: STACK.md mentions `publint` as a companion to `attw`; FOUND-07 only mentions `attw`
   - What's unclear: Whether `publint` is in scope for Phase 1 or Phase 4
   - Recommendation: Install `publint` in Phase 1 (zero cost) but only add it to the `ci` script in Phase 4 when the full exports map is wired

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Vitest 4.1.4 |
| Config file | `vitest.config.ts` (Wave 0 — does not exist yet) |
| Quick run command | `pnpm vitest run src/intent/id.test.ts` |
| Full suite command | `pnpm vitest run` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| FOUND-01 | `pnpm build` and `pnpm test` complete without errors on scaffolded src/ | smoke | `pnpm build && pnpm vitest run` | ❌ Wave 0 |
| FOUND-02 | `OBCError` is instanceof `Error`; `.code` and `.message` are set; `.cause` is optional | unit | `pnpm vitest run src/errors.test.ts` | ❌ Wave 0 |
| FOUND-03 | All six `OBCErrorCode` values constructible without TypeScript error | unit (type-level) | `pnpm tsc --noEmit` (type error = failure) | ❌ Wave 0 |
| FOUND-04 | All seven type interfaces exported from `src/index.ts` and importable without error | unit (type-level) | `pnpm tsc --noEmit` | ❌ Wave 0 |
| FOUND-05 | `generateSessionId()` returns UUID v4 format; fallback path executes when `crypto.randomUUID` is deleted | unit | `pnpm vitest run src/intent/id.test.ts` | ❌ Wave 0 |
| FOUND-06 | `package.json` `dependencies.penpal` equals `"7.0.6"` (no caret) | smoke | `node -e "const p=require('./package.json'); if(p.dependencies.penpal!=='7.0.6') process.exit(1)"` | ❌ Wave 0 |
| FOUND-07 | `attw --pack` exits 0 after `pnpm build` | CI gate | `pnpm build && pnpm dlx @arethetypeswrong/cli --pack` | ❌ Wave 0 |

### FOUND-05 UUID Fallback Test Pattern

The key challenge: Vitest runs in Node 22, which has `crypto.randomUUID`. To exercise the fallback branch:

```typescript
// src/intent/id.test.ts
import { describe, it, expect, afterEach } from 'vitest';

describe('generateSessionId', () => {
  const UUID_V4_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

  it('returns a UUID v4 via crypto.randomUUID when available', async () => {
    // Re-import fresh module (randomUUID is present in Node 22)
    const { generateSessionId } = await import('./id');
    expect(generateSessionId()).toMatch(UUID_V4_RE);
  });

  it('returns a UUID v4 via getRandomValues fallback when randomUUID is absent', async () => {
    // Temporarily remove randomUUID to force the fallback branch
    const original = crypto.randomUUID;
    // @ts-expect-error — deliberately clobbering for test
    delete crypto.randomUUID;
    try {
      // Dynamic import after deletion so the if-check re-runs
      // Because modules are cached, we test the function directly
      // by calling through a wrapper that evaluates the condition at call-time
      const { generateSessionId } = await import('./id');
      expect(generateSessionId()).toMatch(UUID_V4_RE);
    } finally {
      crypto.randomUUID = original;
    }
  });

  it('generates unique IDs across calls', async () => {
    const { generateSessionId } = await import('./id');
    const ids = new Set(Array.from({ length: 100 }, () => generateSessionId()));
    expect(ids.size).toBe(100);
  });
});
```

**Note on module caching:** Because `generateSessionId` checks `typeof crypto.randomUUID` at call-time (inside the function body, not at module load time), deleting `crypto.randomUUID` before calling the already-imported function is sufficient. No Vitest `--isolate` flag or `vi.resetModules()` is needed.

### FOUND-07 `attw --pack` Local Run

```bash
# Build first
pnpm build

# Run attw against the pack tarball (does not publish)
pnpm dlx @arethetypeswrong/cli --pack

# Or if installed as a dev dep:
pnpm attw --pack
```

`attw --pack` calls `npm pack` internally, inspects the resulting `.tgz`, and checks all exports conditions against TypeScript's resolution algorithm. Exits 0 on pass, non-zero on any detected problem. The most common failure for a dual-format library is `CJSResolvesToESM` (CJS consumers get an ESM file) or `FallbackCondition` (types only at top level, not in condition branches).

### FOUND-01 Build + Test Success Criterion

```bash
# In open-buro-client/ directory
pnpm install
pnpm typecheck    # tsc --noEmit — must exit 0
pnpm build        # tsdown — must exit 0, dist/ must contain obc.esm.js, obc.cjs.js, obc.umd.js
pnpm test         # vitest run — must exit 0 (zero test failures)
pnpm attw         # attw --pack — must exit 0
```

Success means all four commands exit 0 with an `src/` that only contains the Phase 1 files (no Phase 2+ code yet).

### Sampling Rate

- **Per task commit:** `pnpm vitest run src/intent/id.test.ts` (quick, targeted)
- **Per wave merge:** `pnpm vitest run` (full suite)
- **Phase gate:** `pnpm typecheck && pnpm build && pnpm test && pnpm attw` all exit 0 before moving to Phase 2

### Wave 0 Gaps

- [ ] `src/intent/id.test.ts` — covers FOUND-05 UUID fallback path
- [ ] `src/errors.test.ts` — covers FOUND-02 `OBCError` class behavior
- [ ] `vitest.config.ts` — test runner config
- [ ] Framework install: `pnpm install` after `package.json` is written

---

## Sources

### Primary (HIGH confidence)

- npm registry `penpal` — version 7.0.6, published 2026-02-13 — version pinning
- npm registry `tsdown` — version 0.21.7, published 2026-03-28 — build config
- npm registry `typescript` — version 6.0.2 — tsconfig TS6 specifics
- npm registry `vitest` — version 4.1.4 — test config
- npm registry `@biomejs/biome` — version 2.4.11 — biome.json schema
- npm registry `@arethetypeswrong/cli` — version 0.18.2 — attw gate
- `.planning/research/STACK.md` — tsconfig verbatim, UUID fallback snippet, exports map pattern, Penpal v7 API shape (all HIGH confidence from project research)
- `.planning/research/SUMMARY.md` — architecture decisions, pitfalls, phase ordering rationale

### Secondary (MEDIUM confidence)

- tsdown documentation: https://tsdown.dev/guide/ — two-array config pattern (verified against STACK.md findings)
- Penpal v7.0.0 release notes: https://github.com/Aaronius/penpal/releases/tag/v7.0.0 — v7 API (cited in STACK.md)
- TypeScript 6.0 announcement: https://devblogs.microsoft.com/typescript/announcing-typescript-6-0/ — default changes (cited in STACK.md)

### Tertiary (LOW confidence — validate during implementation)

- tsdown 0.21 UMD output fidelity: pre-1.0, reported working but not independently verified for this exact OBC build shape

---

## Metadata

**Confidence breakdown:**
- Standard stack versions: HIGH — all verified against npm registry 2026-04-10
- Config file contents: HIGH — tsconfig and exports map from STACK.md (verified against official sources); tsdown config pattern from STACK.md (MEDIUM from tsdown pre-1.0 status)
- UUID fallback snippet: HIGH — MDN-verified browser support, standard RFC 4122 bit manipulation pattern
- Test patterns: HIGH — standard Vitest patterns; fallback test technique relies on call-time evaluation (verified against function implementation)
- Pitfalls: HIGH — all claims cross-referenced against official sources in STACK.md/SUMMARY.md

**Research date:** 2026-04-10
**Valid until:** 2026-05-10 (stable tooling; re-verify tsdown if pre-1.0 issues surface)
