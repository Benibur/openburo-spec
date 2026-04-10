# Phase 4: Distribution & Quality - Context

**Gathered:** 2026-04-10
**Status:** Ready for planning

<domain>
## Phase Boundary

Close out `@openburo/client` v1: verify every packaging/quality requirement has a concrete artifact, fill the two remaining gaps (same-origin integration test + capability-author integration guide), polish the package metadata (README, LICENSE, description/keywords), and confirm the full CI gate runs end-to-end. No actual `npm publish` — we prepare the tarball, run `attw --pack`, and stop before pushing credentials.

</domain>

<decisions>
## Implementation Decisions (user-confirmed)

### Integration Guide + Packaging Polish

- **Capability-author integration guide** lives at `docs/capability-authors.md` inside `open-buro-client/`. It documents:
  - Required Penpal version for capability iframes (v7 minimum — document the compatibility contract)
  - The `resolve(result: IntentResult)` method contract from the iframe side
  - Query parameters OBC passes (`clientUrl`, `id`, `type`, `allowedMimeType`, `multiple`)
  - Example capability implementation skeleton (HTML + minimal JS)
  - Security expectations (CSP, HTTPS, sandbox implications)
- **README.md** — minimal: install, 10-line usage example, link to `docs/capability-authors.md`, link to main spec
- **`npm publish`** — NOT executed. This phase prepares the tarball via `pnpm pack` and runs `attw --pack` on it. Stop before `npm publish`.
- **LICENSE** — MIT (added in this phase)
- **`package.json` polish** — add `description`, `keywords`, `repository`, `homepage`, `bugs`, `license` fields (Phase 1 may already have some; extend as needed)

### Remaining QA gaps to fill

- **QA-09**: Integration test asserting that `castIntent()` against a same-origin `capability.path` rejects with `OBCError('SAME_ORIGIN_CAPABILITY')`. Test lives in `src/client.test.ts` and uses a capability whose URL matches `location.origin` in happy-dom.
- Confirm QA-01 through QA-10 each have at least one passing test that maps to it.

### Audit

- Run `pnpm run ci` once more end-to-end → must exit 0
- Run `pnpm pack` to produce `openburo-client-0.1.0.tgz` → verify file list (tarball contents)
- Run `pnpm dlx @arethetypeswrong/cli ./openburo-client-0.1.0.tgz` → must report "No problems found"
- Grep audit: verify `src/` contains no TODO/FIXME markers (or document any intentional ones)
- Confirm `package.json` `files` field includes only what should ship (dist + types + README + LICENSE)

### Claude's Discretion

- README wording, LICENSE year/holder (assume 2026 / "OpenBuro contributors" unless the user specifies)
- Example capability skeleton code exact content
- Whether to split the integration guide into multiple files or keep it in one

</decisions>

<code_context>
## Existing Code Insights

### What's already shipping

- All 17 source files across 6 layers + barrel
- 147 passing tests (unit + integration)
- ESM/CJS/UMD builds via tsdown two-array config
- `attw --pack` green across node10/node16 CJS+ESM/bundler
- TypeScript declarations with nested exports conditions
- Biome lint + format clean

### What's missing

- `docs/capability-authors.md`
- `README.md` (may exist from Phase 1 scaffold but minimal)
- `LICENSE`
- QA-09 integration test for same-origin rejection
- Possibly package.json metadata polish

</code_context>

<specifics>
## Specific Ideas

- The capability-author guide is the ONE user-facing artifact that external developers will read — it's the difference between "the lib exists" and "people can actually build capabilities for it". Worth writing well.
- Same-origin test pattern: in happy-dom, `location.origin` is `http://localhost:3000` by default. A capability with `path: 'http://localhost:3000/cap'` will trigger the guard.
- `pnpm pack` output path is `./openburo-client-<version>.tgz` — clean it up afterward.

</specifics>

<deferred>
## Deferred Ideas

- Actual npm publish (requires credentials; not part of autonomous mode)
- CI workflow file (`.github/workflows/*.yml`) — can be added post-milestone
- Playwright end-to-end tests with real cross-origin iframes — v2 (documented in Phase 2 as deferred)
- Changelog automation / semantic-release — v2

</deferred>
