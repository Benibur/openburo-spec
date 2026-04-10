---
phase: 01-foundations
plan: 01
subsystem: infra
tags: [typescript, tsdown, vitest, biome, penpal, uuid, attw, esm, cjs, umd]

# Dependency graph
requires: []
provides:
  - "@openburo/client npm package scaffold with tsdown ESM+CJS+UMD build"
  - "OBCError class with 6 OBCErrorCode values (FOUND-02, FOUND-03)"
  - "7 shared type contracts: Capability, IntentRequest, IntentResult, FileResult, IntentCallback, OBCOptions, CastPlan (FOUND-04)"
  - "generateSessionId() with crypto.randomUUID primary + getRandomValues fallback (FOUND-05)"
  - "Biome lint+format config, Vitest 4 node-env test runner, TypeScript 6 strict config"
  - "attw gate: pnpm dlx @arethetypeswrong/cli --pack exits 0 (FOUND-07)"
affects: [02-core-layers, 03-orchestrator, 04-distribution]

# Tech tracking
tech-stack:
  added:
    - "tsdown 0.21.7 (Rolldown-based bundler; successor to deprecated tsup)"
    - "typescript 6.0.2 (strict, ES2020 target, noUncheckedIndexedAccess)"
    - "vitest 4.1.4 + @vitest/coverage-v8 4.1.4 (node environment)"
    - "@biomejs/biome 2.4.11 (lint + format)"
    - "penpal 7.0.6 (exact pin, no caret)"
    - "@arethetypeswrong/cli 0.18.2 (run via pnpm dlx)"
  patterns:
    - "Exact version pin for single-maintainer deps (penpal: 7.0.6 not ^7.0.6)"
    - "Two-array tsdown config: ESM+CJS with deps.neverBundle penpal; UMD with penpal bundled"
    - "Nested types in exports map (import.types + require.types) — required for attw FOUND-07"
    - "Call-time typeof check for crypto.randomUUID (not module-load-time) enables test fallback coverage"
    - "Object.setPrototypeOf(this, new.target.prototype) in OBCError for instanceof safety across transpilation"

key-files:
  created:
    - "open-buro-client/package.json"
    - "open-buro-client/tsconfig.json"
    - "open-buro-client/tsdown.config.ts"
    - "open-buro-client/vitest.config.ts"
    - "open-buro-client/biome.json"
    - "open-buro-client/.gitignore"
    - "open-buro-client/src/errors.ts"
    - "open-buro-client/src/errors.test.ts"
    - "open-buro-client/src/types.ts"
    - "open-buro-client/src/intent/id.ts"
    - "open-buro-client/src/intent/id.test.ts"
    - "open-buro-client/src/index.ts"
    - "open-buro-client/src/index.test.ts"
    - "open-buro-client/pnpm-lock.yaml"
  modified: []

key-decisions:
  - "Exports map reconciled to actual tsdown 0.21.7 output filenames (index.js/index.cjs/index.d.ts/index.d.cts) — research assumed obc.esm.js prefix which tsdown does not produce"
  - "tsdown external: ['penpal'] replaced with deps.neverBundle: ['penpal'] to eliminate deprecation warning in tsdown 0.21.7"
  - "biome.json updated from research template to Biome 2.4.11 actual API: organizeImports moved to assist.actions.source, noVar removed (not in style rules), files.includes with bare folder negation (no /** suffix)"
  - "OBCError.cause declared as plain property (not override) — ES2020 lib does not include Error.cause; override would fail tsc"
  - "Error constructor called as super(message) not super(message, { cause }) — ErrorOptions.cause is ES2022+ and absent from ES2020 lib"
  - "noExplicitAny in id.test.ts fallback covered with biome-ignore comment (cast is intentional for delete on crypto)"

patterns-established:
  - "Pattern 1: pnpm run ci runs typecheck + lint + test + attw in one invocation"
  - "Pattern 2: biome check --write for auto-sorting imports (replaces manual organizeImports)"
  - "Pattern 3: biome-ignore format comment protects load-bearing operator precedence in UUID fallback"

requirements-completed: [FOUND-01, FOUND-02, FOUND-03, FOUND-04, FOUND-05, FOUND-06, FOUND-07]

# Metrics
duration: 6min
completed: 2026-04-10
---

# Phase 01 Plan 01: Scaffold Summary

**TypeScript library scaffold with tsdown ESM+CJS+UMD build, OBCError + 7 shared type contracts, and crypto.randomUUID/getRandomValues UUID utility — all verified by attw, Vitest 4 (10/10 tests), and Biome lint**

## Performance

- **Duration:** ~6 min
- **Started:** 2026-04-10T09:29:32Z
- **Completed:** 2026-04-10T09:35:44Z
- **Tasks:** 3 of 3
- **Files modified:** 14 (13 source + pnpm-lock.yaml)

## Accomplishments

- Complete `@openburo/client` library scaffold with pnpm, tsdown 0.21.7, TypeScript 6.0.2, Vitest 4.1.4, Biome 2.4.11, penpal pinned at exact 7.0.6
- OBCError class + 6 OBCErrorCode values; 7 shared interfaces (Capability, IntentRequest, IntentResult, FileResult, IntentCallback, OBCOptions, CastPlan); generateSessionId() with fallback
- Full gate passing: `pnpm run ci` (typecheck + biome check + vitest run + attw) exits 0; attw reports "No problems found" for node10, node16 CJS/ESM, bundler

## Task Commits

1. **Task 1: Config files + pnpm install** - `3579d2b` (chore)
2. **Task 2: Source files + tests** - `ac2bb30` (feat)
3. **Task 3: Build gate + lint reconciliation** - `2ffd110` (fix)

## Files Created/Modified

- `package.json` - @openburo/client manifest, penpal 7.0.6 exact, nested exports map, pnpm dlx attw scripts
- `tsconfig.json` - TS6 explicit ES2020 target, rootDir ./src, types [], noEmit true, noUncheckedIndexedAccess true
- `tsdown.config.ts` - two-array config: ESM+CJS with deps.neverBundle penpal; UMD with penpal bundled, globalName OpenBuroClient
- `vitest.config.ts` - node environment, v8 coverage
- `biome.json` - Biome 2.4.11 API with assist.actions.source organizeImports, strict rules
- `.gitignore` - dist/, node_modules/, *.tsbuildinfo, *.tgz
- `src/errors.ts` - OBCError class (6 codes), Object.setPrototypeOf for instanceof, cause as plain property (ES2020 compat)
- `src/errors.test.ts` - 4 tests: instanceof, all 6 codes, cause propagation
- `src/types.ts` - 7 exported interfaces/types
- `src/intent/id.ts` - generateSessionId() with call-time typeof check + getRandomValues fallback (load-bearing parens preserved)
- `src/intent/id.test.ts` - 3 tests: happy path, delete (globalThis.crypto as any).randomUUID fallback, uniqueness 100x
- `src/index.ts` - public barrel, all symbols re-exported
- `src/index.test.ts` - public API smoke test + type-level compile check
- `pnpm-lock.yaml` - generated, penpal 7.0.6 locked

## Decisions Made

- Exports map filenames reconciled to tsdown 0.21.7 actual output (index.js/index.cjs/index.d.ts/index.d.cts) vs the obc.esm/obc.cjs prefix in research
- `external` replaced with `deps.neverBundle` (tsdown 0.21.7 deprecation)
- biome.json rewritten from research template to actual Biome 2.4.11 API
- `OBCError.cause` declared as plain property (not `override`) — ES2020 lib lacks `Error.cause`; `super()` called with 1 argument

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] TypeScript 6 / ES2020 lib lacks Error.cause and ErrorOptions**
- **Found during:** Task 2 (errors.ts + typecheck)
- **Issue:** `override readonly cause` fails — `Error.cause` is not in ES2020 lib. `super(message, { cause })` fails — `ErrorOptions` is ES2022+
- **Fix:** Removed `override`, declared `cause` as plain own property; called `super(message)` only
- **Files modified:** `src/errors.ts`
- **Verification:** `pnpm typecheck` exits 0; instanceof and cause tests pass
- **Committed in:** `ac2bb30` (Task 2)

**2. [Rule 1 - Bug] Single quotes inside JSDoc comments caused TS1002 parse errors**
- **Found during:** Task 2 (typecheck on src/types.ts)
- **Issue:** `'*/*'` inside JSDoc block comments caused "Unterminated string literal" in TypeScript 6
- **Fix:** Replaced `'*/*'` in JSDoc comments with escaped HTML entities / different phrasing
- **Files modified:** `src/types.ts`
- **Verification:** `pnpm typecheck` exits 0
- **Committed in:** `ac2bb30` (Task 2)

**3. [Rule 3 - Blocking] biome.json used keys not in Biome 2.4.11 schema**
- **Found during:** Task 3 (pnpm lint)
- **Issue:** `organizeImports` top-level key, `style.noVar` rule, `files.ignore` key all absent from Biome 2.4.11 API
- **Fix:** Rewrote biome.json to use `assist.actions.source.organizeImports`, removed `noVar`, changed `files.ignore` to `files.includes` with negation patterns
- **Files modified:** `biome.json`
- **Verification:** `pnpm lint` exits 0
- **Committed in:** `2ffd110` (Task 3)

**4. [Rule 3 - Blocking] tsdown 0.21.7 outputs index.js/index.cjs, not obc.esm.js/obc.cjs.js**
- **Found during:** Task 3 (build + exports map reconciliation)
- **Issue:** Research-provided exports map used obc.esm.js/obc.cjs.js naming; tsdown produces index.js/index.cjs
- **Fix:** Updated package.json exports map to match actual tsdown output; attw verified green
- **Files modified:** `package.json`
- **Verification:** `node -e` check confirms all exports targets exist on disk; attw "No problems found"
- **Committed in:** `2ffd110` (Task 3)

---

**Total deviations:** 4 auto-fixed (2 Rule 1 bugs, 2 Rule 3 blocking)
**Impact on plan:** All auto-fixes required for correctness and build functionality. No scope creep; all 13 required files delivered.

## Issues Encountered

None beyond the deviations above.

## User Setup Required

None - no external service configuration required. `pnpm install` pulls everything from npm.

## Next Phase Readiness

- All shared type contracts (`Capability`, `IntentRequest`, `IntentResult`, `FileResult`, `IntentCallback`, `OBCOptions`, `CastPlan`) stable and importable from `@openburo/client`
- `OBCError` and `OBCErrorCode` (6 codes) ready for Phase 2 error handling
- `generateSessionId()` available for session tracking in Phase 2
- Build pipeline (tsdown ESM+CJS+UMD), test pipeline (Vitest 4), lint (Biome) all operational
- Phase 2 can add subdirectories under `src/` without config changes

---
*Phase: 01-foundations*
*Completed: 2026-04-10*
