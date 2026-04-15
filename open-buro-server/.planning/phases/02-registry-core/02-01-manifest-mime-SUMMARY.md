---
phase: 02-registry-core
plan: 01
subsystem: registry
tags: [go, manifest, mime, validation, canonicalization, testdata]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: internal/registry package anchor, testify/require pinned in go.mod, Go 1.26.2 toolchain, injection-first slog convention
provides:
  - Manifest, Capability, CapabilityProps domain types (verbatim from CONTEXT.md)
  - Manifest.Validate() with 19-case error catalog, fail-fast, "validate:" prefix
  - In-place canonicalization of Capability MimeTypes + alphabetical sort inside Validate
  - canonicalizeMIME (unexported) + mimeMatch (unexported) pure-function core
  - CanonicalizeMIME exported wrapper for Phase 4 HTTP handler query validation
  - testdata/valid-two-apps.json fixture for Plan 02-02 persistence tests
affects: [02-02-store-persist, 02-03-capabilities, 04-http-api]

# Tech tracking
tech-stack:
  added: []  # zero new dependencies - stdlib-only + pre-pinned testify/require
  patterns:
    - "Fail-fast Validate with 'validate:' error prefix (matches internal/config style)"
    - "Validate mutates receiver: canonicalize MimeTypes in place, then sort.Strings"
    - "Symmetric matching via short-circuit on */* + strings.Cut"
    - "Exported CanonicalizeMIME wrapper so transport layer can reuse domain validation"
    - "Package-doc comment lives on the 'face' file of the domain (manifest.go), not a doc.go stub"
    - "Test file carries the spec verbatim; implementation follows from the table"

key-files:
  created:
    - internal/registry/mime.go
    - internal/registry/mime_test.go
    - internal/registry/manifest.go
    - internal/registry/manifest_test.go
    - internal/registry/testdata/valid-two-apps.json
  modified: []
  deleted:
    - internal/registry/doc.go

key-decisions:
  - "Open Question 1 locked: sort.Strings MimeTypes at end of Validate for byte-stable file representation"
  - "Open Question 2 locked: canonicalizer is lenient with trailing semicolons (text/plain; -> text/plain)"
  - "Open Question 3 locked (partial): CanonicalizeMIME exported now so Phase 4 can pre-validate query MIMEs"
  - "Fixed RESEARCH canonicalizer bug 1: strings.SplitN lets image//png and image/png/extra through; rejected via strings.Contains(parts[1], '/')"
  - "Fixed RESEARCH canonicalizer bug 2: strings.SplitN lets */subtype through; explicit rejection when parts[0]=='*' && parts[1]!='*'"
  - "Package doc moved from doc.go into manifest.go so the face of the domain carries its own documentation; doc.go deleted"
  - "ID regex anchored: ^[a-zA-Z0-9][a-zA-Z0-9._-]*$ (must start alphanumeric, no leading . - _)"

patterns-established:
  - "TDD RED/GREEN workflow with verbatim test paste from plan (spec IS the test table)"
  - "Package doc on first struct-bearing file of the domain, not a separate doc.go"
  - "Validate returns first error with 'validate: field.path ...' field-path prefix"

requirements-completed: [REG-01, REG-02, REG-03, CAP-05, TEST-01]

# Metrics
duration: ~15min
completed: 2026-04-10
---

# Phase 02 Plan 01: Manifest + MIME Summary

**Pure, stateless registry domain core: Manifest.Validate with 19-case error catalog, symmetric 3x3 MIME wildcard matching with exhaustive symmetry assertions, canonicalizer with both RESEARCH bugs fixed, all proven by 64 passing subtests.**

## Performance

- **Duration:** ~15 min
- **Started:** 2026-04-10T09:44:34Z
- **Completed:** 2026-04-10T09:59:00Z
- **Tasks:** 2
- **Files created:** 5
- **Files deleted:** 1

## Accomplishments

- Phase 2's hardest correctness risks (symmetric MIME matching, validation coverage) are frozen by tests before any Store state exists
- 17 mimeMatch subtests (9 positive 3x3 cells + 8 negative/boundary), each asserting `mimeMatch(a,b) == mimeMatch(b,a)` for provable symmetry
- 24 canonicalizeMIME subtests covering normal shapes, whitespace, parameter stripping, and 9 rejection cases
- 23 Manifest.Validate error subtests + 2 happy-path tests (canonicalizes-in-place, sorts alphabetically)
- Two latent canonicalizer bugs from RESEARCH fixed (double-slash and wildcard-type with concrete subtype)
- CanonicalizeMIME exported for Phase 4 to reuse before calling Store.Capabilities
- Package doc relocated from doc.go stub to manifest.go (the face of the domain)
- testdata fixture ready for Plan 02-02 persistence round-trip tests
- Zero new go.mod dependencies; zero transport/slog imports in package

## Task Commits

Each task was executed TDD-style (RED test commit → GREEN implementation commit):

1. **Task 1 RED: failing mime tests** - `18b6da9` (test)
2. **Task 1 GREEN: canonicalizeMIME + symmetric mimeMatch** - `8c4fd35` (feat)
3. **Task 2 RED: failing Manifest.Validate tests** - `a46fff9` (test)
4. **Task 2 GREEN: Manifest types + 19-case Validate + fixture + delete doc.go** - `88530ea` (feat)

_Plan metadata commit follows this summary._

## Files Created/Modified

- `internal/registry/mime.go` - canonicalizeMIME (unexported), CanonicalizeMIME (exported wrapper), mimeMatch (unexported symmetric 3x3 matcher)
- `internal/registry/mime_test.go` - TestMimeMatch (17 subtests with symmetry), TestCanonicalizeMIME (24 subtests), TestCanonicalizeMIME_Exported (smoke test)
- `internal/registry/manifest.go` - package doc + Manifest/Capability/CapabilityProps types + Validate() with 19-case error catalog
- `internal/registry/manifest_test.go` - TestManifestValidate_Happy, TestManifestValidate_CanonicalizesInPlace, TestManifestValidate_Errors (23 subtests)
- `internal/registry/testdata/valid-two-apps.json` - canonical fixture (two apps: files-app with PICK+SAVE */*, mail-app with SAVE image/png+text/plain)
- `internal/registry/doc.go` - **DELETED** (package doc moved to manifest.go)

## Test Count Breakdown

| Test                                   | Subtests | Purpose |
|----------------------------------------|----------|---------|
| TestMimeMatch                          | 17       | 9 positive 3x3 cells + 8 negative/boundary, symmetry asserted for each |
| TestCanonicalizeMIME                   | 24       | Normal shapes, whitespace, param stripping, 9 rejection cases |
| TestCanonicalizeMIME_Exported          | 1        | Phase 4 wrapper smoke test (accept + reject path) |
| TestManifestValidate_Happy             | 1        | Valid manifest returns nil, MimeTypes sorted alphabetically |
| TestManifestValidate_CanonicalizesInPlace | 1     | Uppercase + param-bearing inputs become canonical after Validate |
| TestManifestValidate_Errors            | 23       | 19-rule error catalog (duplicates for whitespace name, zero-length slice, multiple URL schemes) |
| **Total**                              | **67**   | |

## Decisions Made

All 5 RESEARCH open questions addressed (3 fully locked here, 2 applied in later plans):

1. **Sort MimeTypes after canonicalization: YES.** Single `sort.Strings(c.Properties.MimeTypes)` at the end of each capability loop eliminates the last source of file-representation nondeterminism.
2. **Trailing semicolon in canonicalizer: LENIENT.** `text/plain;` canonicalizes to `text/plain` (asserted by `TestCanonicalizeMIME/trailing_semicolon_(accepted,_lenient)`).
3. **Malformed filter MIME in Store.Capabilities: applied in Plan 02-03.** This plan provides the exported `CanonicalizeMIME` wrapper that Phase 4's handler will call first.
4. **NewStore mkdir missing parent: deferred to Plan 02-02.**
5. **Delete of non-existent manifest: deferred to Plan 02-02.**

Two canonicalizer bugs from RESEARCH fixed (both asserted by negative subtests):

- **Bug 1 (double-slash / three-segment):** `strings.SplitN("image//png", "/", 2)` returns `["image", "/png"]` — both parts non-empty, so the CONTEXT sketch would accept it. Fixed via `strings.Contains(parts[1], "/")` rejection after SplitN.
- **Bug 2 (`*/subtype`):** `strings.SplitN("*/png", "/", 2)` returns `["*", "png"]` — both parts non-empty, would incorrectly accept. Fixed via explicit `parts[0] == "*" && parts[1] != "*"` rejection.

## Deviations from Plan

None - plan executed exactly as written.

Both tasks followed the TDD RED→GREEN flow verbatim. Plan specified verbatim test tables and implementation; I pasted them and both went green on first run (no debug loops). `go vet`, `gofmt`, and the architectural gates (`go list -deps | grep -E 'wshub|httpapi'` empty; no `log/slog` / `slog.Default` in non-test files) all passed without modification. gofmt realigned struct-field padding in `manifest_test.go` after write; this is cosmetic and semantically identical.

## Issues Encountered

- Initial `go version` via `/usr/bin/go` tried to auto-fetch Go 1.26 toolchain and failed (no network). Resolved by using the Phase 1-installed toolchain at `$HOME/sdk/go1.26.2/bin/go` (documented in STATE.md Phase 1 decisions). No code changes needed.

## Verification Gate Results

All six commands from the plan's `<verification>` block exit 0:

```
go test ./internal/registry -race -count=1 -v    -> PASS (67 subtests across 6 test funcs)
go build ./...                                   -> OK
go vet ./internal/registry                       -> clean
gofmt -l internal/registry/                      -> empty
go list -deps ./internal/registry | grep wshub|httpapi  -> empty (architectural isolation)
grep -rE 'log/slog|slog\.Default' internal/registry/*.go | grep -v _test.go  -> empty (no global logger)
```

## Handoff Notes for Plan 02-02

The pure domain layer is stable. Plan 02-02 (store-persist) can build on top with high confidence:

- **Manifest type is frozen.** Use it directly in Store; call `m.Validate()` on every Upsert before mutating `s.byID`. After Validate, MimeTypes are guaranteed canonical + sorted, so the file representation will be byte-stable across re-upserts (one hash = one state).
- **mime.go is a pure, stateless dependency.** Store does not need to import mime.go directly — Validate() already canonicalizes. Only the query layer (Plan 02-03) needs to call `mimeMatch` after `canonicalizeMIME(filter)`.
- **CanonicalizeMIME is exported.** Phase 4 handler must call it on `?mimeType=` before delegating to Store.Capabilities. Per Open Question 3, malformed filter MIME → empty result.
- **testdata/valid-two-apps.json is the canonical good fixture.** Plan 02-02 should read this file verbatim for Load round-trip tests. The two manifests (files-app / mail-app) cover both `*/*` wildcard and concrete mime arrays.
- **Architectural invariants locked by gates:** no wshub/httpapi imports in registry (Phase 2/3 stay parallel-safe), no slog imports (injection-first logging contract from Phase 1).

## Next Plan Readiness

- Plan 02-02 (store-persist): READY. Domain types + Validate are stable dependencies; fixture exists on disk.
- Plan 02-03 (capabilities): READY for parallel planning; blocked on Plan 02-02 for Store type.

## Self-Check: PASSED

Verified on disk:
- `internal/registry/mime.go` FOUND
- `internal/registry/mime_test.go` FOUND
- `internal/registry/manifest.go` FOUND
- `internal/registry/manifest_test.go` FOUND
- `internal/registry/testdata/valid-two-apps.json` FOUND
- `internal/registry/doc.go` DELETED (confirmed absent)
- Commit `18b6da9` FOUND (test RED mime)
- Commit `8c4fd35` FOUND (feat GREEN mime)
- Commit `a46fff9` FOUND (test RED manifest)
- Commit `88530ea` FOUND (feat GREEN manifest)

---
*Phase: 02-registry-core*
*Completed: 2026-04-10*
