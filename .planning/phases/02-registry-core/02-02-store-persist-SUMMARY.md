---
phase: 02-registry-core
plan: 02
subsystem: registry
tags: [go, store, persistence, atomic-write, rollback, rwmutex, race, tdd]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: internal/registry package anchor, testify/require pinned, Go 1.26.2 toolchain, injection-first slog convention
  - phase: 02-registry-core
    provides: Plan 02-01 — Manifest/Capability/CapabilityProps types, Manifest.Validate (19 rules + in-place MIME canonicalization + alphabetical sort), canonicalizeMIME, CanonicalizeMIME, testdata/valid-two-apps.json fixture
provides:
  - Store struct (sync.RWMutex + manifests map + path) and NewStore
  - Read paths: Get (value copy, (Manifest, bool)) and List (sorted by id, caller-safe copy)
  - Mutation paths: Upsert (Validate -> snapshot prev -> mutate -> persistLocked -> rollback-on-fail) and Delete (no-op on missing id, rollback-on-fail)
  - persist.go: fileFormat{Version,Manifests}, currentFormatVersion=1, loadFromFile (DisallowUnknownFields, per-manifest Validate), snapshot (sorted-by-id), persistLocked (temp+Sync+Rename+dir-fsync)
  - CapabilityView and CapabilityFilter type stubs (Capabilities method lands in Plan 02-03)
  - 5 load-path fixtures under internal/registry/testdata/
  - "persist failed, registry unchanged" error phrase as the observable rollback contract
affects: [02-03-capabilities, 04-http-api, 05-wiring-shutdown]

# Tech tracking
tech-stack:
  added: []  # zero new dependencies — stdlib only + pre-pinned testify/require
  patterns:
    - "Snapshot-mutate-persist-rollback pattern for every state change"
    - "Atomic file write: CreateTemp in same dir -> Encode -> Sync -> Close -> Rename -> dir fsync"
    - "Delete of non-existent id is a no-op with NO disk write (mtime-verified)"
    - "All reads return value copies; callers can safely mutate without affecting Store"
    - "fileFormat wraps version + manifests so v2 migration has a clean signal"
    - "loadFromFile re-validates every manifest at read time so re-loaded state matches a freshly-upserted state byte-for-byte"
    - "TDD RED/GREEN with verbatim plan test tables"

key-files:
  created:
    - internal/registry/store.go
    - internal/registry/store_test.go
    - internal/registry/persist.go
    - internal/registry/persist_test.go
    - internal/registry/testdata/empty.json
    - internal/registry/testdata/malformed-json.json
    - internal/registry/testdata/wrong-version.json
    - internal/registry/testdata/invalid-manifest.json
    - internal/registry/testdata/unknown-field.json
  modified: []

key-decisions:
  - "Open Question 4 locked: NewStore does NOT mkdir a missing parent — missing-file fast-path returns empty store, but the first Upsert against a non-existent parent will fail in CreateTemp, surfacing the operator error when mutation is attempted"
  - "Open Question 5 locked: Delete of non-existent id is a (false, nil) no-op with no disk write, verified by mtime assertion in TestStore_Delete_NonExistent_NoOp"
  - "Rollback error phrase frozen: 'persist failed, registry unchanged' — the test asserts Contains on this exact substring so any future refactor preserves the contract"
  - "persistLocked step order: CreateTemp same-dir -> Encode (SetIndent 2 spaces) -> Sync -> Close -> Rename -> dir fsync (best-effort); temp file Remove deferred unconditionally so failed writes never leak .tmp-* files"
  - "snapshot() copies + sorts by id so registry.json is byte-stable across rewrites; combined with Plan 02-01's in-Validate sort.Strings of mimeTypes, the entire file is diff-stable"
  - "Concurrency test uses List/Get readers only (not Capabilities which doesn't exist yet) — Plan 02-03 adds the Capabilities concurrency test since it's the one that most needs the RWMutex under read load"

patterns-established:
  - "Rollback contract: error.Error() must contain 'registry unchanged' when in-memory state is consistent with disk state after a persist failure"
  - "Fixture-based fail-fast testing: each error path has a dedicated minimal fixture file so the test asserts behavior, not implementation"
  - "Race-detector-first: every goroutine-involving test runs under -race in CI by default (go test -race -count=1)"

requirements-completed: [REG-04, REG-05, REG-06, REG-07, REG-08, PERS-01, PERS-02, PERS-03, PERS-04, PERS-05, TEST-04]

# Metrics
duration: ~3min
completed: 2026-04-10
---

# Phase 02 Plan 02: Store + Persist Summary

**Thread-safe Store with atomic temp+Sync+Rename+dir-fsync persistence and observable in-memory rollback on persist failure, proven by 22 registry tests passing under -race including the PERS-03 unwritable-directory rollback recipe.**

## Performance

- **Duration:** ~3 min
- **Started:** 2026-04-10T09:52:13Z
- **Completed:** 2026-04-10T09:55:33Z
- **Tasks:** 2 (each TDD RED/GREEN)
- **Files created:** 9 (2 production, 2 test, 5 testdata fixtures)

## Accomplishments

- Phase 2's second-biggest correctness risk (PITFALLS #5 atomic persistence + in-memory rollback) is frozen by a direct unwritable-directory test that covers BOTH the new-manifest rollback path and the update-existing rollback path
- `-race` clean under a 10-writer x 10-upsert fan-out against 10-reader x 50-iteration List/Get readers (wall-clock ~0.28s)
- NewStore covers 8 load paths: missing file, missing parent dir, valid file, empty file, malformed JSON, wrong version, invalid manifest, unknown top-level field
- Atomic-write recipe copied verbatim from research: temp-in-same-dir (so Rename is atomic), Encode+SetIndent, Sync (contents), Close, Rename, dir fsync (directory entry durable); failed writes never leak .tmp-* files (deferred Remove)
- `registry.json` is byte-stable across rewrites: manifests sorted by id in snapshot(), mimeTypes sorted in Validate(), 2-space indentation — diffable in git
- Zero new go.mod dependencies; zero transport/slog imports; architectural gates still clean
- CapabilityView + CapabilityFilter type stubs landed so Plan 02-03 only has to add the `Capabilities(filter)` method body

## Task Commits

Each task executed TDD-style (RED test commit -> GREEN implementation commit):

1. **Task 1 RED: failing NewStore load-path tests + fixtures** - `c826f44` (test)
2. **Task 1 GREEN: Store skeleton + persist.go (loadFromFile, snapshot, persistLocked)** - `5b8512e` (feat)
3. **Task 2 RED: failing Upsert/Delete/concurrency/rollback tests** - `8d736cf` (test)
4. **Task 2 GREEN: Upsert + Delete with snapshot-mutate-rollback** - `424b012` (feat)

_Plan metadata commit follows this summary._

## Files Created/Modified

- `internal/registry/store.go` — Store struct, NewStore, Get, List, Upsert, Delete, CapabilityView, CapabilityFilter
- `internal/registry/store_test.go` — 11 test funcs: Upsert_{Create,Replace,ValidationFails}, Delete_{Existing,NonExistent_NoOp,Idempotent}, Get_NotFound, List_{SortedByID,ReturnsCopy}, ConcurrentAccess, Upsert_PersistFailureRollsBack
- `internal/registry/persist.go` — fileFormat, currentFormatVersion=1, loadFromFile, snapshot, persistLocked
- `internal/registry/persist_test.go` — 11 test funcs: NewStore_{MissingFile,MissingParentDirectory,LoadsValidFile,LoadsEmptyFile,CorruptedFile,WrongVersion,InvalidManifest,UnknownField}, Upsert_{WritesAtomically,WritesIndentedJSON,DeterministicOrder}
- `internal/registry/testdata/empty.json` — valid empty registry fixture
- `internal/registry/testdata/malformed-json.json` — unterminated array (load error fixture)
- `internal/registry/testdata/wrong-version.json` — version=2 (version-mismatch fixture)
- `internal/registry/testdata/invalid-manifest.json` — version=1 + one manifest with `javascript:` URL (Validate error fixture)
- `internal/registry/testdata/unknown-field.json` — version=1 + stray top-level `comment` (DisallowUnknownFields fixture)

## Test Count Breakdown

| Test file          | Funcs | Purpose                                                                               |
| ------------------ | ----- | ------------------------------------------------------------------------------------- |
| store_test.go      | 11    | Upsert/Delete/Get/List semantics, concurrent access, persist-failure rollback         |
| persist_test.go    | 11    | 8 NewStore load paths + 3 atomic-write assertions (decode, indent, deterministic order) |
| **Plan 02-02 new** | **22**| All passing under `-race -count=1`                                                    |
| Plan 02-01 carried | 6 funcs / 67 subtests | mime + manifest (all still passing)                                        |
| **Package total**  | **28 funcs / 89 cases** | `ok github.com/openburo/openburo-server/internal/registry 1.344s`        |

## PERS-03 Rollback Evidence

`TestStore_Upsert_PersistFailureRollsBack` exercises the contract exactly:

1. Seeds one manifest (`seed-app`), persist succeeds
2. `os.Chmod(dir, 0o500)` makes the directory unwritable (CreateTemp inside fails with EACCES)
3. Attempt to upsert a NEW manifest (`new-app`) → error returned, asserted `Contains "registry unchanged"` → `Get("new-app")` returns `(_, false)`, `Get("seed-app")` returns `(_, true)` with name `"Seed"` (rollback to pre-mutation state)
4. Attempt to UPDATE the existing `seed-app` with new name `"Modified"` → error returned, asserted `Contains "registry unchanged"` → `Get("seed-app").Name == "Seed"` (rolled back to prior value, not wiped)
5. `t.Cleanup` restores `0o700` so `t.TempDir`'s RemoveAll can clean up

**Rollback error message contains "registry unchanged": YES.** Both new-insert and update rollback paths covered.

## Concurrency Test

`TestStore_ConcurrentAccess` — 10 writer goroutines × 10 upserts + 10 reader goroutines × 50 List/Get iterations, all against one Store.

**Wall-clock under `-race`:** 0.28s
**Race detector:** clean
**Final state assertion:** `len(store.List()) >= 105` (5 seed + 100 unique writer ids)

## Decisions Made

1. **Open Question #4 (missing parent dir): NO mkdir.** NewStore against a path whose PARENT directory does not exist succeeds silently (both cases return `os.ErrNotExist` from os.Open and take the fast-path empty-store branch). The operator error surfaces on the first Upsert because `os.CreateTemp` cannot create inside a non-existent dir. Documented by `TestNewStore_MissingParentDirectory`. This is intentional: we do not silently create directories outside the operator's stated intent.

2. **Open Question #5 (Delete non-existent id): NO-OP, no disk write.** Verified by `TestStore_Delete_NonExistent_NoOp` which captures `os.Stat().ModTime()` before and after — asserted equal. Returns `(false, nil)`.

3. **Rollback error phrase is observable contract.** Chose `"persist failed, registry unchanged"` and the tests assert `require.Contains(err.Error(), "registry unchanged")` — so future refactors can change the prefix but cannot drop the phrase without failing tests.

4. **snapshot() over the map (not append-as-we-go).** Kept the snapshot step as a separate method taking a read of the map so persistLocked is one linear flow with no implicit ordering dependency; also lets Plan 02-03's Capabilities method potentially reuse it if needed.

5. **Best-effort dir fsync.** Parent-directory fsync failure is ignored (`_ = dir.Sync()`). The rename has already succeeded and the file contents are already fsynced; dir fsync is about crash-durability of the rename entry. On systems that don't support dir fsync (or where Open on a dir returns error), we still return success.

6. **Concurrency test uses List/Get only, not Capabilities.** Capabilities lands in Plan 02-03, which will add its own concurrency test. The RWMutex is the same mutex, so correctness transfers.

## Deviations from Plan

None — plan executed exactly as written.

Both tasks followed the TDD RED→GREEN flow verbatim. Plan specified verbatim test tables and implementation; both went green on first run (no debug loops). `go vet`, `gofmt -l`, architectural gates (`go list -deps | grep -E 'wshub|httpapi'` empty; no `log/slog` / `slog.Default` in non-test files) all passed without modification.

**Total deviations:** 0
**Impact on plan:** None.

## Issues Encountered

- `go version` via `/usr/bin/go` tried to auto-fetch Go 1.26 toolchain and failed (no network). Resolved by using the Phase 1-installed toolchain at `$HOME/sdk/go1.26.2/bin/go` (documented in STATE.md Phase 1 decisions). No code changes needed. Same workaround as Plan 02-01.

## Verification Gate Results

All six commands from the plan's `<verification>` block exit 0:

```
go test ./internal/registry -race -count=1 -v          -> PASS (28 test funcs / 89 cases; ConcurrentAccess 0.28s)
go build ./...                                         -> OK
go vet ./internal/registry                             -> clean
gofmt -l internal/registry/                            -> empty
go list -deps ./internal/registry | grep wshub|httpapi -> empty (architectural isolation)
grep -rE 'log/slog|slog\.Default' internal/registry/*.go | grep -v _test.go -> empty (no global logger)
```

## Handoff Notes for Plan 02-03

The Store is stateful, thread-safe, durable, and race-clean. Plan 02-03 (capabilities) only has to add the Capabilities method:

- **Store is frozen except for the Capabilities method.** Add `func (s *Store) Capabilities(filter CapabilityFilter) []CapabilityView` — the type stubs exist in store.go.
- **Capabilities implementation guidance:**
  - Acquire `s.mu.RLock()` for the duration; iterate the manifests map deterministically (sort ids first, just like List/snapshot) so output order is stable
  - For each manifest and each of its capabilities, if `filter.Action != ""` and `filter.Action != c.Action` skip; if `filter.MimeType != ""`, canonicalize the filter once (if canonicalization fails → return empty slice, not an error), then for each of the capability's already-canonical mimeTypes call `mimeMatch(canonFilter, mt)` and emit a CapabilityView if any match
  - Return `[]CapabilityView` (empty slice, not nil, if no matches — easier for JSON handlers)
- **Phase 4 surface is covered.** `CanonicalizeMIME` is already exported so Phase 4's handler can pre-validate `?mimeType=` and return 400 on malformed input before calling Store.Capabilities.
- **Concurrency test for Capabilities:** add a dedicated one in Plan 02-03 mirroring the shape of TestStore_ConcurrentAccess but with Capabilities calls interleaved with Upsert/Delete, to close the loop on the RWMutex read contract.
- **testdata/valid-two-apps.json still the canonical fixture.** Reuse it for Capabilities tests — files-app has `*/*` so every filter matches; mail-app has concrete `image/png`+`text/plain` so filter tests have clear positive and negative cases.
- **File output is byte-stable.** Tests can grep the on-disk `registry.json` without needing to reparse.

## Next Plan Readiness

- **Plan 02-03 (capabilities):** READY. Store + all underlying types + canonicalization are stable; only the one method body remains.
- **Phase 02 exit criteria:** Plan 02-03 is the last plan in Phase 2. After it completes, Phase 2 ships.
- **Phase 3 (wshub) and Phase 4 (httpapi)** remain unblocked by Phase 2's dependency-graph contract — registry imports nothing from wshub or httpapi.

## Self-Check: PASSED

Verified on disk:
- `internal/registry/store.go` FOUND
- `internal/registry/store_test.go` FOUND
- `internal/registry/persist.go` FOUND
- `internal/registry/persist_test.go` FOUND
- `internal/registry/testdata/empty.json` FOUND
- `internal/registry/testdata/malformed-json.json` FOUND
- `internal/registry/testdata/wrong-version.json` FOUND
- `internal/registry/testdata/invalid-manifest.json` FOUND
- `internal/registry/testdata/unknown-field.json` FOUND
- Commit `c826f44` FOUND (test RED persist)
- Commit `5b8512e` FOUND (feat GREEN persist/store skeleton)
- Commit `8d736cf` FOUND (test RED store mutations)
- Commit `424b012` FOUND (feat GREEN Upsert/Delete)

---
*Phase: 02-registry-core*
*Completed: 2026-04-10*
