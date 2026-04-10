---
phase: 01-foundations
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - open-buro-client/package.json
  - open-buro-client/tsconfig.json
  - open-buro-client/tsdown.config.ts
  - open-buro-client/vitest.config.ts
  - open-buro-client/biome.json
  - open-buro-client/.gitignore
  - open-buro-client/src/errors.ts
  - open-buro-client/src/errors.test.ts
  - open-buro-client/src/types.ts
  - open-buro-client/src/intent/id.ts
  - open-buro-client/src/intent/id.test.ts
  - open-buro-client/src/index.ts
  - open-buro-client/src/index.test.ts
autonomous: true
requirements:
  - FOUND-01
  - FOUND-02
  - FOUND-03
  - FOUND-04
  - FOUND-05
  - FOUND-06
  - FOUND-07

must_haves:
  truths:
    - "pnpm install completes with penpal pinned to exact 7.0.6 (no caret) in the installed lockfile"
    - "pnpm typecheck (tsc --noEmit) exits 0 on the scaffolded src/"
    - "pnpm build (tsdown) produces dist/obc.esm.js, dist/obc.cjs.js, dist/obc.umd.js with .d.ts and .d.cts alongside"
    - "pnpm vitest run exits 0 with all FOUND-02, FOUND-05, FOUND-04 tests passing including the UUID fallback branch"
    - "pnpm dlx @arethetypeswrong/cli --pack exits 0 after a successful build"
    - "A consumer can `import { OBCError, generateSessionId, type Capability, type CastPlan } from '@openburo/client'` and TypeScript resolves all types"
  artifacts:
    - path: "open-buro-client/package.json"
      provides: "Package manifest with exact Penpal pin and nested exports map"
      contains: '"penpal": "7.0.6"'
    - path: "open-buro-client/tsconfig.json"
      provides: "TypeScript 6 strict config with explicit ES2020 target and noEmit"
      contains: '"noUncheckedIndexedAccess": true'
    - path: "open-buro-client/tsdown.config.ts"
      provides: "Two-array tsdown config (ESM/CJS external penpal + UMD bundled)"
      contains: "defineConfig(["
    - path: "open-buro-client/vitest.config.ts"
      provides: "Vitest 4 runner config, node environment"
      contains: "environment: 'node'"
    - path: "open-buro-client/biome.json"
      provides: "Biome 2.4 lint + format config"
      contains: "biomejs.dev/schemas"
    - path: "open-buro-client/.gitignore"
      provides: "Ignores for dist/, node_modules/, *.tsbuildinfo, *.tgz"
      contains: "dist/"
    - path: "open-buro-client/src/errors.ts"
      provides: "OBCError class + OBCErrorCode union (six codes)"
      exports: ["OBCError", "OBCErrorCode"]
    - path: "open-buro-client/src/errors.test.ts"
      provides: "Unit tests for OBCError instance shape and all six codes"
    - path: "open-buro-client/src/types.ts"
      provides: "Shared interfaces: Capability, IntentRequest, IntentResult, FileResult, IntentCallback, OBCOptions, CastPlan"
      exports: ["Capability", "IntentRequest", "IntentResult", "FileResult", "IntentCallback", "OBCOptions", "CastPlan"]
    - path: "open-buro-client/src/intent/id.ts"
      provides: "generateSessionId() with crypto.randomUUID + getRandomValues fallback"
      exports: ["generateSessionId"]
    - path: "open-buro-client/src/intent/id.test.ts"
      provides: "UUID v4 regex assertion + delete crypto.randomUUID fallback branch coverage"
    - path: "open-buro-client/src/index.ts"
      provides: "Public barrel: re-exports OBCError, all types, generateSessionId"
    - path: "open-buro-client/src/index.test.ts"
      provides: "Import-surface smoke test for FOUND-04 (all symbols present)"
  key_links:
    - from: "open-buro-client/package.json"
      to: "open-buro-client/src/index.ts"
      via: "exports map entrypoints resolve to dist/obc.esm.js / obc.cjs.js built from src/index.ts"
      pattern: '"default": "./dist/obc\.esm\.js"'
    - from: "open-buro-client/tsdown.config.ts"
      to: "open-buro-client/src/index.ts"
      via: "entry: ['src/index.ts'] in both config objects"
      pattern: "src/index\\.ts"
    - from: "open-buro-client/src/index.ts"
      to: "open-buro-client/src/errors.ts"
      via: "re-export of OBCError and OBCErrorCode"
      pattern: "from './errors'"
    - from: "open-buro-client/src/index.ts"
      to: "open-buro-client/src/types.ts"
      via: "re-export type Capability, CastPlan, etc."
      pattern: "from './types'"
    - from: "open-buro-client/src/index.ts"
      to: "open-buro-client/src/intent/id.ts"
      via: "re-export of generateSessionId"
      pattern: "from './intent/id'"
    - from: "open-buro-client/src/intent/id.test.ts"
      to: "open-buro-client/src/intent/id.ts"
      via: "dynamic import after deleting crypto.randomUUID to exercise fallback"
      pattern: "delete \\(globalThis\\.crypto as any\\)\\.randomUUID"
---

<objective>
Scaffold the `@openburo/client` TypeScript library from a greenfield `open-buro-client/` directory: build tooling (tsdown + TS 6 + Vitest 4 + Biome), shared type contracts, error model, and the `generateSessionId()` utility. Every subsequent phase depends on this scaffold.

Purpose: Satisfy FOUND-01 through FOUND-07 so Phase 2 layers can be built in parallel against stable type contracts and a working build/test pipeline.
Output: Thirteen files on disk, `pnpm install` succeeds, `pnpm typecheck && pnpm build && pnpm test && pnpm dlx @arethetypeswrong/cli --pack` all exit 0.
</objective>

<execution_context>
@/home/ben/.claude/get-shit-done/workflows/execute-plan.md
@/home/ben/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@open-buro-client/.planning/ROADMAP.md
@open-buro-client/.planning/REQUIREMENTS.md
@open-buro-client/.planning/STATE.md
@open-buro-client/.planning/phases/01-foundations/01-CONTEXT.md
@open-buro-client/.planning/phases/01-foundations/01-RESEARCH.md
@open-buro-client/.planning/phases/01-foundations/01-VALIDATION.md
@open-buro-client/.planning/research/STACK.md

<interfaces>
<!-- All file contents are verbatim in 01-RESEARCH.md "Code Examples" section (lines 243-611). -->
<!-- The executor MUST read RESEARCH.md first and copy each snippet literally. -->
<!-- Do NOT reformat the UUID fallback expression — operator precedence matters. -->

Key immutable contracts for downstream phases (Phase 2+):

From src/errors.ts — six error codes (FOUND-03):
```typescript
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
  constructor(code: OBCErrorCode, message: string, cause?: unknown);
}
```

From src/types.ts — shared contracts (FOUND-04):
```typescript
export interface Capability { id; appName; action; path; iconUrl?; properties: { mimeTypes: string[] } }
export interface IntentRequest { action; args: { allowedMimeType?; multiple? } }
export interface FileResult { name; type; size; url }
export interface IntentResult { id; status: 'done'|'cancel'; results: FileResult[] }
export type IntentCallback = (result: IntentResult) => void;
export interface OBCOptions { capabilitiesUrl; liveUpdates?; wsUrl?; container?; onCapabilitiesUpdated?; onError? }
export type CastPlan = { kind: 'no-match' } | { kind: 'direct'; capability } | { kind: 'select'; capabilities }
```

From src/intent/id.ts — session id (FOUND-05):
```typescript
export function generateSessionId(): string;
```
</interfaces>

<critical_rules>
1. Penpal version is `"7.0.6"` — no caret, no tilde, no range prefix. Literal string.
2. tsdown.config.ts MUST use the two-array form (`defineConfig([...])`) — first config externalizes penpal for ESM+CJS, second bundles penpal for UMD. A single-object config is wrong and will break deduplication in consumers.
3. tsconfig MUST set explicitly: `"target": "ES2020"`, `"rootDir": "./src"`, `"types": []`, `"noEmit": true`, `"noUncheckedIndexedAccess": true`. TS6 defaults shifted; do not rely on them.
4. UUID fallback expression: `(crypto.getRandomValues(new Uint8Array(1))[0] & (15 >> (Number(c) / 4)))` — the parentheses around `(15 >> (Number(c) / 4))` and around `(Number(c) / 4)` are LOAD-BEARING. Do NOT reformat, do NOT let Biome auto-fix alter precedence.
5. The UUID fallback test deletes `crypto.randomUUID` BEFORE calling `generateSessionId()` — the check is at call-time, not module-load-time. No `vi.resetModules()` needed.
6. `@arethetypeswrong/cli` runs via `pnpm dlx` (the `--pack` flag invokes `npm pack` and needs the tarball — it is NOT listed as a devDependency for runtime execution, though it's OK to leave it in devDependencies; prefer the `pnpm dlx` invocation in the `attw` script).
7. Package manager is **pnpm**. Do not create `package-lock.json`. Generate `pnpm-lock.yaml`.
8. No nested `git init` — `open-buro-client/` is a subdirectory of the existing `openburo-spec` git repo.
9. `src/errors.ts` — OBCError constructor MUST call `Object.setPrototypeOf(this, new.target.prototype)` for instanceof to survive transpilation.
</critical_rules>
</context>

<tasks>

<task type="auto" tdd="false">
  <name>Task 1 — Wave 0: Write all config files and install dependencies</name>
  <files>
    open-buro-client/package.json,
    open-buro-client/tsconfig.json,
    open-buro-client/tsdown.config.ts,
    open-buro-client/vitest.config.ts,
    open-buro-client/biome.json,
    open-buro-client/.gitignore
  </files>
  <read_first>
    - open-buro-client/.planning/phases/01-foundations/01-RESEARCH.md (Code Examples section, lines 243-455 — contains verbatim package.json, tsconfig.json, tsdown.config.ts, biome.json, vitest.config.ts, .gitignore)
    - open-buro-client/.planning/phases/01-foundations/01-CONTEXT.md (locked decisions — nothing to interpret)
    - open-buro-client/.planning/research/STACK.md (installation commands and version verification)
  </read_first>
  <action>
    Create six files at the paths above. Copy the content **verbatim** from RESEARCH.md "Code Examples" (lines 243-455), with one deviation documented below.

    1. **open-buro-client/package.json** — copy from RESEARCH.md lines 245-294. Verify:
       - `"name": "@openburo/client"`
       - `"type": "module"`
       - `"dependencies": { "penpal": "7.0.6" }` — exact literal, NO caret
       - `"exports"."."` has `"import"` and `"require"` branches, each with nested `"types"` BEFORE `"default"`
       - `"scripts"` includes: `build`, `typecheck`, `test`, `test:watch`, `coverage`, `lint`, `format`, `attw`, `ci`
       - `"engines": { "node": ">=22.18.0" }`
       - Change the `attw` script to use `pnpm dlx`: `"attw": "pnpm dlx @arethetypeswrong/cli --pack"` (and the same in the `ci` script: `... && pnpm dlx @arethetypeswrong/cli --pack`). Remove `@arethetypeswrong/cli` from devDependencies since we run it via `pnpm dlx`.

    2. **open-buro-client/tsconfig.json** — copy from RESEARCH.md lines 306-326. Then ADD one additional field inside `compilerOptions` per Open Question #2 in RESEARCH.md:
       - `"noUncheckedIndexedAccess": true` (add after `"skipLibCheck": true`)
       All other fields exactly as shown: `"target": "ES2020"`, `"lib": ["ES2020", "DOM"]`, `"module": "ESNext"`, `"moduleResolution": "bundler"`, `"strict": true`, `"declaration": true`, `"declarationMap": true`, `"sourceMap": true`, `"outDir": "./dist"`, `"rootDir": "./src"`, `"types": []`, `"esModuleInterop": true`, `"skipLibCheck": true`, `"noEmit": true`. `"include": ["src"]`.

    3. **open-buro-client/tsdown.config.ts** — copy from RESEARCH.md lines 331-360 **exactly**. MUST be the two-array form `defineConfig([{...}, {...}])`. First object: `format: ['esm', 'cjs']`, `dts: true`, `external: ['penpal']`. Second object: `format: ['umd']`, `globalName: 'OpenBuroClient'`, `minify: true`, NO `external` field.

    4. **open-buro-client/vitest.config.ts** — copy from RESEARCH.md lines 412-426 exactly. `environment: 'node'`, `include: ['src/**/*.test.ts']`, coverage provider `v8`.

    5. **open-buro-client/biome.json** — copy from RESEARCH.md lines 367-406 exactly. `"$schema": "https://biomejs.dev/schemas/2.4.11/schema.json"`, strict rules including `noExplicitAny: error`, `noUnusedVariables: error`.

    6. **open-buro-client/.gitignore** — copy from RESEARCH.md lines 433-454 exactly. Must include: `dist/`, `coverage/`, `node_modules/`, `*.tsbuildinfo`, `.vscode/`, `.idea/`, `*.swp`, `.DS_Store`, `Thumbs.db`, `*.tgz`.

    After all six files exist, run (from `open-buro-client/`):
    ```bash
    cd open-buro-client && pnpm install
    ```
    This should generate `pnpm-lock.yaml` and `node_modules/`. It may warn that there's no `src/index.ts` to build yet — that's fine, build is not run here.
  </action>
  <verify>
    <automated>
      cd open-buro-client &&
      test -f package.json &&
      test -f tsconfig.json &&
      test -f tsdown.config.ts &&
      test -f vitest.config.ts &&
      test -f biome.json &&
      test -f .gitignore &&
      node -e "const p=require('./package.json'); if(p.dependencies.penpal!=='7.0.6'){console.error('penpal not pinned exact');process.exit(1)}; if(p.name!=='@openburo/client'){console.error('name wrong');process.exit(1)}; if(p.type!=='module'){console.error('type wrong');process.exit(1)}; const ex=p.exports['.']; if(!ex.import.types||!ex.require.types){console.error('exports map missing nested types');process.exit(1)}" &&
      node -e "const fs=require('fs'); const c=fs.readFileSync('tsconfig.json','utf8'); if(!/\"target\"\s*:\s*\"ES2020\"/.test(c)){console.error('target not ES2020');process.exit(1)}; if(!/\"noEmit\"\s*:\s*true/.test(c)){console.error('noEmit not true');process.exit(1)}; if(!/\"noUncheckedIndexedAccess\"\s*:\s*true/.test(c)){console.error('noUncheckedIndexedAccess not true');process.exit(1)}; if(!/\"rootDir\"\s*:\s*\"\.\/src\"/.test(c)){console.error('rootDir not ./src');process.exit(1)}; if(!/\"types\"\s*:\s*\[\s*\]/.test(c)){console.error('types not []');process.exit(1)}" &&
      node -e "const fs=require('fs'); const c=fs.readFileSync('tsdown.config.ts','utf8'); if(!/defineConfig\(\s*\[/.test(c)){console.error('tsdown config not two-array form');process.exit(1)}; if(!/external:\s*\[\s*'penpal'\s*\]/.test(c)){console.error('penpal not externalized');process.exit(1)}; if(!/globalName:\s*'OpenBuroClient'/.test(c)){console.error('UMD globalName missing');process.exit(1)}" &&
      test -d node_modules/penpal &&
      node -e "const v=require('./node_modules/penpal/package.json').version; if(v!=='7.0.6'){console.error('installed penpal version is',v);process.exit(1)}"
    </automated>
  </verify>
  <acceptance_criteria>
    - package.json exists and `dependencies.penpal === "7.0.6"` (exact, no caret) — FOUND-06
    - package.json name is `@openburo/client`, type is `module`, exports map has nested `types` in both `import` and `require` branches — FOUND-01, FOUND-07 pre-req
    - tsconfig.json has `target: "ES2020"`, `rootDir: "./src"`, `types: []`, `noEmit: true`, `noUncheckedIndexedAccess: true`
    - tsdown.config.ts uses two-array form; first config has `external: ['penpal']`; second has `globalName: 'OpenBuroClient'`
    - vitest.config.ts has `environment: 'node'` and `include: ['src/**/*.test.ts']`
    - biome.json has `$schema` pointing at biomejs.dev and `noExplicitAny: error`
    - .gitignore includes `dist/`, `node_modules/`, `*.tsbuildinfo`, `*.tgz`
    - `pnpm install` completes; `node_modules/penpal/package.json` reports version `7.0.6`
    - `pnpm-lock.yaml` is generated
  </acceptance_criteria>
  <done>
    All six config files exist with exact verified content, pnpm install has produced node_modules with penpal 7.0.6 and pnpm-lock.yaml. Wave 1 source files can now be written against a working install.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2 — Wave 1: Write source files (errors, types, id, index) and their tests</name>
  <files>
    open-buro-client/src/errors.ts,
    open-buro-client/src/errors.test.ts,
    open-buro-client/src/types.ts,
    open-buro-client/src/intent/id.ts,
    open-buro-client/src/intent/id.test.ts,
    open-buro-client/src/index.ts,
    open-buro-client/src/index.test.ts
  </files>
  <read_first>
    - open-buro-client/.planning/phases/01-foundations/01-RESEARCH.md (Code Examples section, lines 457-611 — verbatim src/errors.ts, src/types.ts, src/intent/id.ts, src/index.ts; Validation Architecture section lines 674-713 — verbatim id.test.ts)
    - open-buro-client/package.json (confirm it exists from Task 1)
    - open-buro-client/tsconfig.json (confirm strict + noUncheckedIndexedAccess are live)
  </read_first>
  <behavior>
    - **errors.test.ts**:
      - `new OBCError('CAPABILITIES_FETCH_FAILED', 'msg')` is instanceof `Error` AND instanceof `OBCError`
      - `.code === 'CAPABILITIES_FETCH_FAILED'`, `.message === 'msg'`, `.name === 'OBCError'`
      - All six codes construct without TS error: `CAPABILITIES_FETCH_FAILED`, `NO_MATCHING_CAPABILITY`, `IFRAME_TIMEOUT`, `WS_CONNECTION_FAILED`, `INTENT_CANCELLED`, `SAME_ORIGIN_CAPABILITY`
      - `new OBCError('INTENT_CANCELLED', 'msg', new Error('root'))` sets `.cause` to the inner Error
      - `new OBCError('INTENT_CANCELLED', 'msg')` has `.cause === undefined`
    - **id.test.ts** (three cases from RESEARCH.md lines 681-712):
      - Happy path: `generateSessionId()` returns a UUID v4 matching `/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i`
      - Fallback path: after `delete (globalThis.crypto as any).randomUUID`, calling `generateSessionId()` still returns a valid UUID v4 (and restore `crypto.randomUUID` in `finally`)
      - Uniqueness: 100 calls produce 100 unique ids (`new Set(...).size === 100`)
    - **index.test.ts**:
      - Importing `{ OBCError, generateSessionId }` from `../src/index` (or relative) returns truthy values
      - Type-level: assign `Capability`, `IntentRequest`, `IntentResult`, `FileResult`, `IntentCallback`, `OBCOptions`, `CastPlan` to variables with `satisfies` or explicit annotations; `pnpm tsc --noEmit` must pass
  </behavior>
  <action>
    Copy source files verbatim from RESEARCH.md Code Examples. Do NOT paraphrase. Do NOT let Biome reformat the UUID fallback expression.

    1. **open-buro-client/src/errors.ts** — copy from RESEARCH.md lines 460-482 exactly. `OBCErrorCode` union with the six codes listed. `OBCError` class extends `Error`, `override readonly cause?: unknown`, constructor signature `(code, message, cause?)`, must call `super(message, cause !== undefined ? { cause } : undefined)`, set `this.name = 'OBCError'`, set `this.code = code`, set `this.cause = cause`, and call `Object.setPrototypeOf(this, new.target.prototype)` as the last line.

    2. **open-buro-client/src/errors.test.ts** — create new file following the behaviors above. Example structure:
       ```typescript
       import { describe, it, expect } from 'vitest';
       import { OBCError, type OBCErrorCode } from './errors';

       describe('OBCError', () => {
         it('is an instance of Error and OBCError', () => {
           const e = new OBCError('CAPABILITIES_FETCH_FAILED', 'boom');
           expect(e).toBeInstanceOf(Error);
           expect(e).toBeInstanceOf(OBCError);
           expect(e.code).toBe('CAPABILITIES_FETCH_FAILED');
           expect(e.message).toBe('boom');
           expect(e.name).toBe('OBCError');
         });

         it('accepts all six error codes', () => {
           const codes: OBCErrorCode[] = [
             'CAPABILITIES_FETCH_FAILED',
             'NO_MATCHING_CAPABILITY',
             'IFRAME_TIMEOUT',
             'WS_CONNECTION_FAILED',
             'INTENT_CANCELLED',
             'SAME_ORIGIN_CAPABILITY',
           ];
           for (const code of codes) {
             const e = new OBCError(code, 'x');
             expect(e.code).toBe(code);
           }
         });

         it('preserves cause when provided', () => {
           const root = new Error('root');
           const e = new OBCError('INTENT_CANCELLED', 'wrap', root);
           expect(e.cause).toBe(root);
         });

         it('cause is undefined when omitted', () => {
           const e = new OBCError('INTENT_CANCELLED', 'noCause');
           expect(e.cause).toBeUndefined();
         });
       });
       ```

    3. **open-buro-client/src/types.ts** — copy from RESEARCH.md lines 490-561 exactly. Seven interface/type exports: `Capability`, `IntentRequest`, `FileResult`, `IntentResult`, `IntentCallback`, `OBCOptions`, `CastPlan`. Do NOT alter field names, optionality markers, or JSDoc.

    4. **open-buro-client/src/intent/id.ts** — copy from RESEARCH.md lines 567-587 exactly. CRITICAL: the fallback expression must be:
       ```typescript
       return '10000000-1000-4000-8000-100000000000'.replace(/[018]/g, (c) =>
         (
           Number(c) ^
           (crypto.getRandomValues(new Uint8Array(1))[0] & (15 >> (Number(c) / 4)))
         ).toString(16),
       );
       ```
       The parenthesization `(15 >> (Number(c) / 4))` is load-bearing. If Biome or any formatter attempts to "simplify" this, override with `// biome-ignore format` if needed.

       NOTE: With `noUncheckedIndexedAccess: true` in tsconfig, `crypto.getRandomValues(new Uint8Array(1))[0]` will have type `number | undefined`. Wrap as `(crypto.getRandomValues(new Uint8Array(1))[0] ?? 0)` to satisfy the type checker without changing runtime behavior (the array always has exactly 1 element, so `?? 0` never fires). Final expression:
       ```typescript
       (
         Number(c) ^
         ((crypto.getRandomValues(new Uint8Array(1))[0] ?? 0) & (15 >> (Number(c) / 4)))
       ).toString(16)
       ```

    5. **open-buro-client/src/intent/id.test.ts** — copy from RESEARCH.md lines 680-712 exactly. Three `it()` blocks: happy path, fallback with `delete (globalThis.crypto as any).randomUUID` inside try/finally, uniqueness 100×. Use `const UUID_V4_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i`. Import `generateSessionId` at the top (static import is fine since the check is call-time, not module-load-time).

    6. **open-buro-client/src/index.ts** — copy from RESEARCH.md lines 596-610 exactly:
       ```typescript
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

    7. **open-buro-client/src/index.test.ts** — create a new file that imports the public surface and asserts presence + type-level coverage:
       ```typescript
       import { describe, it, expect } from 'vitest';
       import {
         OBCError,
         generateSessionId,
         type Capability,
         type CastPlan,
         type FileResult,
         type IntentCallback,
         type IntentRequest,
         type IntentResult,
         type OBCOptions,
         type OBCErrorCode,
       } from './index';

       describe('public API surface (FOUND-04)', () => {
         it('exports OBCError as a constructor', () => {
           expect(typeof OBCError).toBe('function');
           expect(new OBCError('INTENT_CANCELLED', 'x')).toBeInstanceOf(Error);
         });

         it('exports generateSessionId as a function returning a UUID-shaped string', () => {
           expect(typeof generateSessionId).toBe('function');
           expect(generateSessionId()).toMatch(/^[0-9a-f-]{36}$/i);
         });

         it('all shared types are assignable (compile-time check)', () => {
           const cap: Capability = { id: 'x', appName: 'X', action: 'PICK', path: 'https://x/', properties: { mimeTypes: ['*/*'] } };
           const req: IntentRequest = { action: 'PICK', args: {} };
           const file: FileResult = { name: 'f', type: 't', size: 0, url: 'u' };
           const res: IntentResult = { id: 'id', status: 'done', results: [file] };
           const cb: IntentCallback = (_r) => {};
           const opts: OBCOptions = { capabilitiesUrl: 'https://x/' };
           const plan: CastPlan = { kind: 'no-match' };
           const code: OBCErrorCode = 'CAPABILITIES_FETCH_FAILED';
           expect([cap, req, file, res, cb, opts, plan, code]).toBeDefined();
         });
       });
       ```

    After all seven source files are written, run:
    ```bash
    cd open-buro-client && pnpm typecheck && pnpm vitest run
    ```
    Both must exit 0.
  </action>
  <verify>
    <automated>
      cd open-buro-client &&
      test -f src/errors.ts &&
      test -f src/errors.test.ts &&
      test -f src/types.ts &&
      test -f src/intent/id.ts &&
      test -f src/intent/id.test.ts &&
      test -f src/index.ts &&
      test -f src/index.test.ts &&
      node -e "const fs=require('fs'); const c=fs.readFileSync('src/errors.ts','utf8'); const codes=['CAPABILITIES_FETCH_FAILED','NO_MATCHING_CAPABILITY','IFRAME_TIMEOUT','WS_CONNECTION_FAILED','INTENT_CANCELLED','SAME_ORIGIN_CAPABILITY']; for(const k of codes){if(!c.includes(k)){console.error('missing code',k);process.exit(1)}}; if(!c.includes('Object.setPrototypeOf')){console.error('missing setPrototypeOf');process.exit(1)}" &&
      node -e "const fs=require('fs'); const c=fs.readFileSync('src/intent/id.ts','utf8'); if(!/15\s*>>\s*\(Number\(c\)\s*\/\s*4\)/.test(c)){console.error('UUID fallback precedence parens missing');process.exit(1)}; if(!c.includes('crypto.randomUUID')){console.error('randomUUID call missing');process.exit(1)}" &&
      node -e "const fs=require('fs'); const c=fs.readFileSync('src/intent/id.test.ts','utf8'); if(!c.includes('delete (globalThis.crypto as any).randomUUID')){console.error('delete randomUUID pattern missing');process.exit(1)}" &&
      node -e "const fs=require('fs'); const c=fs.readFileSync('src/types.ts','utf8'); for(const t of ['Capability','IntentRequest','FileResult','IntentResult','IntentCallback','OBCOptions','CastPlan']){if(!new RegExp('export (interface|type) '+t+'\\\\b').test(c)){console.error('missing export',t);process.exit(1)}}" &&
      node -e "const fs=require('fs'); const c=fs.readFileSync('src/index.ts','utf8'); for(const s of ['OBCError','OBCErrorCode','Capability','CastPlan','FileResult','IntentCallback','IntentRequest','IntentResult','OBCOptions','generateSessionId']){if(!c.includes(s)){console.error('index.ts missing',s);process.exit(1)}}" &&
      pnpm typecheck &&
      pnpm vitest run
    </automated>
  </verify>
  <acceptance_criteria>
    - `src/errors.ts` exports `OBCError` (class) and `OBCErrorCode` (type union with all six codes: CAPABILITIES_FETCH_FAILED, NO_MATCHING_CAPABILITY, IFRAME_TIMEOUT, WS_CONNECTION_FAILED, INTENT_CANCELLED, SAME_ORIGIN_CAPABILITY) — FOUND-02, FOUND-03
    - OBCError constructor calls `Object.setPrototypeOf(this, new.target.prototype)`
    - `src/types.ts` exports all seven: `Capability`, `IntentRequest`, `IntentResult`, `FileResult`, `IntentCallback`, `OBCOptions`, `CastPlan` — FOUND-04
    - `src/intent/id.ts` — `generateSessionId()` with call-time `typeof crypto.randomUUID === 'function'` check, parenthesized `(15 >> (Number(c) / 4))` fallback expression — FOUND-05
    - `src/intent/id.test.ts` — contains literal `delete (globalThis.crypto as any).randomUUID` for fallback coverage
    - `src/index.ts` re-exports all symbols listed in `<interfaces>` block
    - `pnpm typecheck` exits 0 (no TypeScript errors)
    - `pnpm vitest run` exits 0 with all tests passing (errors.test.ts, id.test.ts, index.test.ts)
    - id.test.ts fallback branch test passes — demonstrates `generateSessionId()` still works when `crypto.randomUUID` is deleted
  </acceptance_criteria>
  <done>
    Seven source files exist with verified content, all tests green, tsc --noEmit passes, UUID fallback branch demonstrably exercised. FOUND-02, FOUND-03, FOUND-04, FOUND-05 satisfied.
  </done>
</task>

<task type="auto" tdd="false">
  <name>Task 3 — Wave 2: Full build, lint, attw gate verification</name>
  <files>open-buro-client/dist/</files>
  <read_first>
    - open-buro-client/.planning/phases/01-foundations/01-RESEARCH.md (Validation Architecture section, lines 717-744 — verbatim attw invocation pattern)
    - open-buro-client/package.json (verify scripts from Task 1 are present)
    - open-buro-client/tsdown.config.ts (verify two-array form from Task 1)
  </read_first>
  <action>
    Run the full Phase 1 gate sequence and verify each step. No files are modified here — this task proves the scaffold is production-ready.

    From `open-buro-client/`:

    1. **Lint check** — `pnpm lint` (which runs `biome check .`). Must exit 0. If Biome flags anything in src/, fix with minimal-invasive changes (prefer `biome-ignore` comments over altering load-bearing code like the UUID fallback expression). Re-run until green.

    2. **Full typecheck** — `pnpm typecheck` (`tsc --noEmit`). Must exit 0.

    3. **Full test suite** — `pnpm test` (`vitest run`). Must exit 0 with all tests passing.

    4. **Build** — `pnpm build` (`tsdown`). Must exit 0. Verify `dist/` contains:
       - `dist/obc.esm.js` (or whatever tsdown outputs for the esm format; check `tsdown.config.ts` filename mapping — the default is `index.mjs` or similar, the exports map in `package.json` expects `obc.esm.js`/`obc.cjs.js`/`obc.umd.js`)
       - `dist/obc.cjs.js`
       - `dist/obc.umd.js`
       - `dist/index.d.ts` (ESM types) and `dist/index.d.cts` (CJS types) — or whatever path the exports map `"types"` field points at

       **If the output filenames do not match the exports map:** Update `tsdown.config.ts` to add `outputOptions` / `outExtensions` / `entryFileNames` mapping OR update the `exports` map in `package.json` to match tsdown's default output filenames. Reconcile so that every path referenced in `package.json.exports` actually exists on disk after build. Prefer adjusting tsdown config if possible (to keep the exports map user-facing paths stable). Test by `ls dist/` and `node -e "require.resolve('./dist/obc.cjs.js')"` or equivalent.

       NOTE: tsdown by default emits `index.js` for ESM, `index.cjs` for CJS, and for UMD the filename depends on `globalName`. Check with `pnpm build && ls -la dist/` first, then reconcile. This is a known reconciliation step — research did not lock the exact output filenames, only the exports map targets.

    5. **attw gate** — `pnpm dlx @arethetypeswrong/cli --pack` (or `pnpm attw` if the script wraps it). Must exit 0. If it reports `CJSResolvesToESM`, `FallbackCondition`, or `Masked*` errors, the exports map or the emitted files don't match. Fix and re-run `pnpm build && pnpm dlx @arethetypeswrong/cli --pack` until green.

    6. **Final composite check** — run `pnpm ci` (which is `tsc --noEmit && biome check . && vitest run && pnpm dlx @arethetypeswrong/cli --pack`). Must exit 0 end-to-end in one invocation.
  </action>
  <verify>
    <automated>
      cd open-buro-client &&
      pnpm lint &&
      pnpm typecheck &&
      pnpm vitest run &&
      pnpm build &&
      test -d dist &&
      node -e "const fs=require('fs'); const p=require('./package.json'); const ex=p.exports['.']; const targets=[ex.import.default, ex.import.types, ex.require.default, ex.require.types]; for(const t of targets){if(!fs.existsSync(t.replace(/^\.\//, ''))){console.error('missing exports target:',t);process.exit(1)}}; console.log('all exports targets exist on disk')" &&
      pnpm dlx @arethetypeswrong/cli --pack
    </automated>
  </verify>
  <acceptance_criteria>
    - `pnpm lint` exits 0
    - `pnpm typecheck` exits 0
    - `pnpm vitest run` exits 0 with all tests green
    - `pnpm build` exits 0 and creates `dist/` directory
    - Every path listed in `package.json` `exports` map (both `import.default`, `import.types`, `require.default`, `require.types`) exists on disk inside `dist/`
    - `pnpm dlx @arethetypeswrong/cli --pack` exits 0 — no `CJSResolvesToESM`, no `FallbackCondition`, no `Masked*` errors — FOUND-07
    - `pnpm ci` (composite script) exits 0
  </acceptance_criteria>
  <done>
    Phase 1 scaffold passes every automated gate: lint + typecheck + test + build + attw. FOUND-01 (scaffold boots) and FOUND-07 (attw green) satisfied. Phase 2 can begin.
  </done>
</task>

</tasks>

<verification>
After all three tasks complete, run once more from `open-buro-client/`:

```bash
pnpm ci
```

This single composite command must exit 0. It chains:
1. `tsc --noEmit` (FOUND-01 typecheck)
2. `biome check .` (lint)
3. `vitest run` (FOUND-02, FOUND-03, FOUND-04, FOUND-05 test coverage)
4. `pnpm dlx @arethetypeswrong/cli --pack` (FOUND-07 types gate)

Also spot-check FOUND-06 one more time:
```bash
node -e "const p=require('./open-buro-client/package.json'); console.log('penpal pinned to:',p.dependencies.penpal); process.exit(p.dependencies.penpal==='7.0.6'?0:1)"
```
Must print `penpal pinned to: 7.0.6` and exit 0.
</verification>

<success_criteria>
- All 13 files exist at the paths in `files_modified`
- `pnpm-lock.yaml` exists and lists penpal `7.0.6`
- `pnpm ci` exits 0 on a clean `pnpm install` (FOUND-01, FOUND-07)
- `OBCError` and `OBCErrorCode` are importable from `@openburo/client` and cover all six codes (FOUND-02, FOUND-03)
- All seven shared types are importable from `@openburo/client` (FOUND-04)
- `generateSessionId()` returns a valid UUID v4 both via `crypto.randomUUID` AND via the `getRandomValues` fallback, verified by distinct test cases (FOUND-05)
- `package.json.dependencies.penpal === "7.0.6"` — exact literal, verified by grep and by installed `node_modules/penpal/package.json` (FOUND-06)
- `pnpm dlx @arethetypeswrong/cli --pack` exits 0 (FOUND-07)
- No `.eslintrc*`, no `.prettierrc*`, no `tsup.config.*` files exist (we use Biome and tsdown)
- No `package-lock.json` exists (pnpm only)
</success_criteria>

<output>
After completion, create `.planning/phases/01-foundations/01-01-SUMMARY.md` documenting:
- Final file listing under `open-buro-client/` (13 files + generated dist/, node_modules/, pnpm-lock.yaml)
- Exact versions installed (`pnpm list --depth 0` output or equivalent)
- `pnpm ci` output confirming all gates pass
- Any deviations from the plan (e.g., output filename reconciliation in Task 3)
- Patterns established for Phase 2: file layout (`src/<domain>/<file>.ts` + co-located `.test.ts`), import conventions (`type`-only imports for types), test structure (vitest `describe`/`it`/`expect`)
- Requirement completion table: FOUND-01 through FOUND-07 each marked complete with evidence path
</output>
