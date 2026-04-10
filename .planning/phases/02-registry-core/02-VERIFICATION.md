---
phase: 02-registry-core
verified: 2026-04-10T10:15:00Z
status: passed
score: 5/5 success criteria verified, 20/20 requirements verified
re_verification:
  previous_status: none
  previous_score: none
  gaps_closed: []
  gaps_remaining: []
  regressions: []
---

# Phase 02: Registry Core Verification Report

**Phase Goal:** A pure domain package (`internal/registry`) owning manifest validation, thread-safe in-memory state, atomic file persistence, and symmetric MIME matching — independently testable with zero transport dependencies.

**Verified:** 2026-04-10T10:15:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths (from ROADMAP Success Criteria)

| #   | Truth                                                                                                                                                             | Status     | Evidence                                                                                                                                                                                                              |
| --- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | `Store.Upsert`, `Delete`, `Get`, `List`, `Capabilities(filter)` behave per contract under concurrent access, proven by `go test -race` on the registry package.  | VERIFIED | `TestStore_ConcurrentAccess` (store_test.go:150) runs 10 writers × 10 upserts + 10 readers × 50 List/Get under `-race`. `go test ./internal/registry -race -count=1` → ok 1.360s. 29 funcs / 107 runs, 0 failures. |
| 2   | A `mimeType` query matches symmetrically across all 9 wildcard combinations; malformed MIME strings rejected — verified by exhaustive table-driven test.         | VERIFIED | `TestMimeMatch` (mime_test.go:9) has 17 cases (9 positive 3×3 cells + 8 negative/boundary), each asserts `mimeMatch(a,b) == mimeMatch(b,a)`. `TestCanonicalizeMIME` (mime_test.go:55) has 24 cases including 9 rejection paths. Includes double-slash, `*/subtype`, three-segment bug fixes. |
| 3   | Disk write failure mid-mutation → in-memory `Store` identical to pre-mutation state; mutation returns error (proven by test against unwritable directory).        | VERIFIED | `TestStore_Upsert_PersistFailureRollsBack` (store_test.go:190) uses `os.Chmod(dir, 0o500)` to force CreateTemp failure; asserts `require.Contains(err.Error(), "registry unchanged")` for BOTH new-insert AND update-existing paths. Verifies seed-app name unchanged post-rollback. |
| 4   | Restart vs existing `registry.json` → same `Store.List`; missing file → empty registry; corrupted file → fail fast with clear error.                             | VERIFIED | `TestNewStore_LoadsValidFile` (persist_test.go:45) loads testdata/valid-two-apps.json → 2 manifests in deterministic order. `TestNewStore_MissingFile` (:13) → empty store no error. `TestNewStore_CorruptedFile` (:60), `_WrongVersion` (:67), `_InvalidManifest` (:74), `_UnknownField` (:82) all assert error with contextual message. |
| 5   | `Store.List` and `Store.Capabilities` return results in deterministic order.                                                                                      | VERIFIED | `List` sorts map keys (store.go:72-76). `Capabilities` uses `sort.SliceStable` with 4-key sort: lower(AppName), AppID, Action, Path (store.go:197-210). `TestStore_List_SortedByID` (store_test.go:125), `TestStore_Capabilities/sort...` (store_test.go:269, 283), `TestStore_Upsert_DeterministicOrder` (persist_test.go:122) all pass. |

**Score:** 5/5 Success Criteria truths verified.

### Required Artifacts

| Artifact                                          | Expected                                                          | Status     | Details                                                                                                                |
| ------------------------------------------------- | ----------------------------------------------------------------- | ---------- | ---------------------------------------------------------------------------------------------------------------------- |
| `internal/registry/manifest.go`                   | Manifest/Capability/CapabilityProps types + Validate (19 rules) | VERIFIED | 150 lines; package doc; all 19 validation rules present; in-place canonicalize + `sort.Strings` on MIME (:137-147). |
| `internal/registry/mime.go`                       | canonicalizeMIME, CanonicalizeMIME, mimeMatch (symmetric 3×3)   | VERIFIED | 87 lines; double-slash bug fix (:37-39); `*/subtype` bug fix (:41-43); symmetric implementation via `strings.Cut` (:65-86). |
| `internal/registry/store.go`                      | Store + NewStore/Get/List/Upsert/Delete + Capabilities + types    | VERIFIED | 214 lines; RWMutex; snapshot-mutate-persist-rollback on Upsert (:89-112) and Delete (:118-135); Capabilities with single-canonicalize + RLock + 4-key sort (:147-213). |
| `internal/registry/persist.go`                    | fileFormat, loadFromFile, snapshot, persistLocked (atomic write) | VERIFIED | 115 lines; `DisallowUnknownFields`; per-manifest Validate on load (:48-55); temp+Encode+Sync+Close+Rename+dir-fsync sequence verbatim (:85-114); deferred tmp Remove (:91). |
| `internal/registry/manifest_test.go`              | Happy + canonicalize + 23 error-case subtests                     | VERIFIED | 3 test funcs, 23 error subtests for Validate 19-rule catalog (some rules have multiple duplicates for whitespace/slice/URL scheme).   |
| `internal/registry/mime_test.go`                  | 17 mimeMatch subtests + 24 canonicalizeMIME subtests + export     | VERIFIED | 3 test funcs; symmetry asserted inside every subtest loop (:46-51).                                                   |
| `internal/registry/store_test.go`                 | Upsert/Delete/Get/List + concurrency + rollback + Capabilities    | VERIFIED | 12 top-level funcs; `TestStore_Capabilities` has 14 subtests covering flatten/sort/filter/symmetric/OR/malformed/combined/empty. |
| `internal/registry/persist_test.go`               | 6 NewStore load paths + 3 atomic-write assertions                 | VERIFIED | 11 test funcs including MissingFile, MissingParentDirectory, LoadsValidFile, LoadsEmptyFile, CorruptedFile, WrongVersion, InvalidManifest, UnknownField, Upsert_WritesAtomically, WritesIndentedJSON, DeterministicOrder. |
| `internal/registry/testdata/valid-two-apps.json`  | Canonical fixture (2 apps)                                        | VERIFIED | files-app (PICK+SAVE `*/*`) + mail-app (SAVE `image/png`+`text/plain`); matches SUMMARY claims.                       |
| `internal/registry/testdata/empty.json`           | Empty registry fixture                                            | VERIFIED | Present.                                                                                                              |
| `internal/registry/testdata/malformed-json.json`  | Load-error fixture                                                | VERIFIED | Present.                                                                                                              |
| `internal/registry/testdata/wrong-version.json`   | Version-mismatch fixture                                          | VERIFIED | Present.                                                                                                              |
| `internal/registry/testdata/invalid-manifest.json`| Validate-error fixture                                            | VERIFIED | Present.                                                                                                              |
| `internal/registry/testdata/unknown-field.json`   | DisallowUnknownFields fixture                                     | VERIFIED | Present.                                                                                                              |
| `internal/registry/doc.go`                        | DELETED — package doc moved to manifest.go                        | VERIFIED | Absent on disk. `manifest.go` carries the package doc (:1-9).                                                          |

### Key Link Verification

| From                             | To                             | Via                                                          | Status | Details                                                                                                                          |
| -------------------------------- | ------------------------------ | ------------------------------------------------------------ | ------ | -------------------------------------------------------------------------------------------------------------------------------- |
| `Store.Upsert`                   | `Manifest.Validate`            | direct call before mutation                                  | WIRED  | store.go:90 — `if err := m.Validate(); err != nil { return err }` before acquiring lock.                                         |
| `Manifest.Validate`              | `canonicalizeMIME`             | direct call inside capabilities loop                         | WIRED  | manifest.go:137 — canonicalizes each MIME in place, then `sort.Strings` at :146.                                                 |
| `Store.Capabilities`             | `canonicalizeMIME` + `mimeMatch` | single canonicalize outside loop, mimeMatch per-cap inside | WIRED  | store.go:155 (single canonicalize), store.go:177 (mimeMatch against pre-canonical capability MIMEs).                              |
| `Store.Upsert` / `Delete`        | `persistLocked`                | direct call inside write lock with snapshot rollback         | WIRED  | store.go:103, :130. Rollback restores previous value (store.go:104-108, :131).                                                   |
| `persistLocked`                  | `snapshot`                     | encoded via json.Encoder                                     | WIRED  | persist.go:95 — `enc.Encode(s.snapshot())`.                                                                                      |
| `NewStore`                       | `loadFromFile`                 | direct call in constructor                                   | WIRED  | store.go:44; loadFromFile re-validates every manifest at :49-55.                                                                  |
| `loadFromFile`                   | `Manifest.Validate`            | per-manifest in decode loop                                  | WIRED  | persist.go:48-55 — re-validates so reloaded state matches freshly-upserted state byte-for-byte.                                   |
| `CanonicalizeMIME` (exported)    | `canonicalizeMIME` (unexported)| thin wrapper                                                 | WIRED  | mime.go:50-52. Ready for Phase 4 handler to pre-validate query MIME.                                                              |
| persistLocked atomic write       | same-dir temp + Sync + Rename + dir fsync | sequential calls                                    | WIRED  | persist.go:86 (CreateTemp in `filepath.Dir(s.path)`), :99 (tmp.Sync), :106 (os.Rename), :109-112 (dir.Sync). Temp Remove deferred. |
| Rollback error phrase            | "registry unchanged" substring | error message contract                                       | WIRED  | store.go:109, :132 — `fmt.Errorf("persist failed, registry unchanged: %w", err)`. Tests assert `require.Contains`.                |

### Requirements Coverage

All 20 Phase 2 requirements are implemented and tested. REQUIREMENTS.md already marks them checked; source + test + roadmap traceability verified.

| Requirement                                                    | Source Plan | Description                                                              | Status    | Evidence                                                                                                           |
| -------------------------------------------------------------- | ----------- | ------------------------------------------------------------------------ | --------- | ------------------------------------------------------------------------------------------------------------------ |
| **REG-01** Manifest validates required fields                  | 02-01       | id, name, url, version, non-empty capabilities[]                         | SATISFIED | manifest.go:68-114; tests manifest_test.go:57-70 (empty/missing cases).                                            |
| **REG-02** action enum PICK/SAVE                               | 02-01       | Validated against enum                                                   | SATISFIED | manifest.go:118-123; tests manifest_test.go:72-73 (empty + lowercase rejected).                                    |
| **REG-03** mimeTypes canonicalized                             | 02-01       | Non-empty list; lowercased, parameters stripped                          | SATISFIED | manifest.go:133-147 + mime.go:19-45; tests `TestManifestValidate_CanonicalizesInPlace` + 24 canonicalizeMIME cases. |
| **REG-04** Store guards state with sync.RWMutex                | 02-02       | Mutations serialize, reads parallelize                                   | SATISFIED | store.go:14 RWMutex; Lock on Upsert/Delete, RLock on Get/List/Capabilities; `TestStore_ConcurrentAccess` race-clean.|
| **REG-05** Upsert create-or-replace                            | 02-02       | Create if absent, fully replace if present                               | SATISFIED | store.go:89-112; tests `TestStore_Upsert_Create`, `TestStore_Upsert_Replace`.                                      |
| **REG-06** Delete reports existence                            | 02-02       | Removes by id and reports whether it existed                             | SATISFIED | store.go:118-135 returns `(bool, error)`; tests `TestStore_Delete_Existing`, `_NonExistent_NoOp`, `_Idempotent`.    |
| **REG-07** Get returns manifest-or-not-found                   | 02-02       | Returns single manifest by id or not-found signal                        | SATISFIED | store.go:60-65 returns `(Manifest, bool)`; test `TestStore_Get_NotFound`.                                           |
| **REG-08** List deterministic order                            | 02-02       | Sorted by id                                                             | SATISFIED | store.go:69-82; test `TestStore_List_SortedByID` inserts reverse order, asserts sorted output.                     |
| **CAP-01** Capabilities flattened view                         | 02-03       | appId, appName, action, path, properties                                 | SATISFIED | store.go:23-29 CapabilityView + :147-213 Capabilities; test `TestStore_Capabilities/flatten`.                      |
| **CAP-02** 4-key sort (lower appName, appID, action, path)     | 02-03       | Stable across restarts and platforms                                     | SATISFIED | store.go:197-210 `sort.SliceStable` with 4-key comparator; tests `sort by lower appName...`, `sort action and path tiebreakers`. |
| **CAP-03** action exact-match filter                           | 02-03       | Case-sensitive match                                                     | SATISFIED | store.go:170-172; tests `filter by action PICK`, `SAVE`, `is case-sensitive`.                                       |
| **CAP-04** symmetric 3×3 MIME filter                           | 02-03       | Full matrix on both sides                                                | SATISFIED | store.go:173-185 uses mimeMatch; tests `filter by mimeType exact/type wildcard/full wildcard/against capability wildcard (symmetric)/OR semantics`. |
| **CAP-05** exhaustive table-driven MIME matching test           | 02-01       | Every 3×3 combo + malformed rejection                                    | SATISFIED | mime_test.go `TestMimeMatch` 17 cases (9 positive matrix + 8 negative/boundary) + symmetry assertion per case. `TestCanonicalizeMIME` 24 cases with 9 rejection paths. |
| **PERS-01** missing file → empty registry                      | 02-02       | No error on missing file                                                 | SATISFIED | persist.go:27-29 `os.ErrNotExist` fast-path; test `TestNewStore_MissingFile`.                                       |
| **PERS-02** atomic temp+Sync+Rename+dir-fsync                  | 02-02       | Temp in same directory                                                   | SATISFIED | persist.go:86 (same-dir CreateTemp), :99 (tmp.Sync), :106 (os.Rename), :109-112 (dir fsync best-effort). Test `TestStore_Upsert_WritesAtomically` asserts no .tmp- leakage. |
| **PERS-03** persist failure → rollback                          | 02-02       | In-memory state rolls back; mutation returns error                       | SATISFIED | store.go:103-110 (Upsert rollback), :130-132 (Delete rollback); test `TestStore_Upsert_PersistFailureRollsBack` exercises both new-insert AND update-existing rollback paths via `os.Chmod(dir, 0o500)`. |
| **PERS-04** corrupted file fail-fast                           | 02-02       | Clear error, no silent data loss                                         | SATISFIED | persist.go:39-41, :42-47 (version mismatch), :48-55 (per-manifest Validate); tests `TestNewStore_CorruptedFile`, `_WrongVersion`, `_InvalidManifest`, `_UnknownField`. |
| **PERS-05** indented JSON                                      | 02-02       | Human-readable, 2-space indent                                           | SATISFIED | persist.go:94 `enc.SetIndent("", "  ")`; test `TestStore_Upsert_WritesIndentedJSON`.                                |
| **TEST-01** table-driven unit tests Validate/Store/MIME        | 02-01..03   | Table-driven coverage                                                    | SATISFIED | TestMimeMatch, TestCanonicalizeMIME, TestManifestValidate_Errors, TestStore_Capabilities all table-driven.         |
| **TEST-04** unwritable-dir rollback recipe                     | 02-02       | Direct test against unwritable directory                                 | SATISFIED | `TestStore_Upsert_PersistFailureRollsBack` uses `os.Chmod(dir, 0o500)` + `t.Cleanup(0o700)`.                        |

**Orphaned requirements:** None. REQUIREMENTS.md traceability table maps exactly 20 requirements to Phase 2, and all 20 appear across the three plans' `requirements-completed` fields (02-01: REG-01..03, CAP-05, TEST-01; 02-02: REG-04..08, PERS-01..05, TEST-04; 02-03: CAP-01..04). TEST-01 is claimed by 02-01 but legitimately satisfied across all three plans (table-driven tests exist in all three test files).

### Anti-Patterns Found

None. Grep for `TODO|FIXME|XXX|HACK|PLACEHOLDER|placeholder|coming soon` across `internal/registry/**/*.go` returned zero matches. No stub functions, no `return nil` placeholders, no empty handlers. All code paths execute real logic.

### Verification Gate Results

All commands from the phase verification gate executed successfully (using `~/sdk/go1.26.2/bin/go`):

| Command                                                                           | Result                                                                                     |
| --------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| `go test ./internal/registry -race -count=1`                                      | **PASS** — ok 1.360s (29 top-level funcs, 107 test runs counted from `-v` output)          |
| `go test ./... -race -count=1`                                                    | **PASS** — config, httpapi, registry all ok; cmd/server, wshub, version no test files     |
| `go build ./...`                                                                  | **OK** — clean exit                                                                        |
| `go vet ./internal/registry`                                                      | **Clean** — no diagnostics                                                                 |
| `gofmt -l internal/registry/`                                                     | **Empty** — zero files need formatting                                                     |
| `go list -deps ./internal/registry \| grep -E 'wshub\|httpapi'`                    | **Empty** — architectural isolation confirmed; registry imports nothing from wshub or httpapi |
| `grep -rE 'log/slog\|slog\.Default' internal/registry/*.go \| grep -v _test.go`   | **Empty** — no global logger / no slog import in production files; injection-first contract upheld |

### Commit Verification

All 13 commits claimed by the three SUMMARYs exist in `git log`:

| Commit   | Type               | Subject (abbreviated)                                     | Plan   |
| -------- | ------------------ | --------------------------------------------------------- | ------ |
| 18b6da9  | test RED           | add failing tests for canonicalizeMIME and mimeMatch      | 02-01  |
| 8c4fd35  | feat GREEN         | implement canonicalizeMIME and symmetric mimeMatch         | 02-01  |
| a46fff9  | test RED           | add failing tests for Manifest.Validate                    | 02-01  |
| 88530ea  | feat GREEN         | add Manifest domain type and 19-case Validate              | 02-01  |
| 643b2c2  | docs               | complete manifest-mime plan                                | 02-01  |
| c826f44  | test RED           | add failing NewStore load-path tests + fixtures            | 02-02  |
| 5b8512e  | feat GREEN         | Store skeleton + atomic persistence layer                  | 02-02  |
| 8d736cf  | test RED           | add failing Upsert/Delete/concurrency/rollback tests       | 02-02  |
| 424b012  | feat GREEN         | Store.Upsert + Delete with atomic persist + rollback       | 02-02  |
| 857c8a6  | docs               | complete store-persist plan                                | 02-02  |
| 94464d4  | test RED           | add failing TestStore_Capabilities with 13 subtests        | 02-03  |
| 189abe6  | feat GREEN         | implement Store.Capabilities filter + sort                 | 02-03  |
| df64260  | docs               | complete capabilities plan                                 | 02-03  |

Note: The 02-03 RED commit message says "13 subtests" but the actual test function contains 14 subtests. This is a cosmetic discrepancy in the commit subject; the code and summary both correctly reflect 14 subtests. Not a verification gap.

### Research-Flagged Pitfalls Honored

Both critical Phase 2 research flags from ROADMAP are implemented and asserted:

1. **PITFALLS #2 — Symmetric 3×3 wildcard MIME matching with exhaustive test** — VERIFIED. `mimeMatch` (mime.go:65-86) handles `*/*` short-circuit, splits via `strings.Cut`, wildcard subtype rule. Every `TestMimeMatch` subtest asserts `mimeMatch(a,b) == mimeMatch(b,a)` explicitly (mime_test.go:48-51).

2. **PITFALLS #5 — Atomic persistence with in-memory rollback** — VERIFIED. `persistLocked` (persist.go:85-114) uses the full recipe: same-dir CreateTemp + Encode + Sync + Close + Rename + dir fsync, with deferred temp Remove. Upsert rollback restores prev or deletes new-ID (store.go:104-109). Delete rollback restores prev (store.go:131). `TestStore_Upsert_PersistFailureRollsBack` exercises both rollback branches.

### Gaps Summary

**No gaps found.**

Every Success Criterion from ROADMAP.md is observably true in the codebase. Every requirement ID declared for Phase 2 has implementation evidence (source code path + test function). All architectural gates pass. All verification commands exit clean. The registry package is the pure, transport-agnostic domain core that Phase 4 HTTP handlers can consume via the frozen public API (`NewStore`, `Get`, `List`, `Upsert`, `Delete`, `Capabilities`, `CanonicalizeMIME`).

The phase goal — "A pure domain package owning manifest validation, thread-safe in-memory state, atomic file persistence, and symmetric MIME matching — independently testable with zero transport dependencies" — is fully achieved:

- **Pure domain package:** zero transport imports; `go list -deps` confirms no wshub/httpapi imports; grep confirms no slog imports in production files.
- **Manifest validation:** 19-rule Validate with fail-fast "validate:" prefix; 23 error subtests + 2 happy-path tests.
- **Thread-safe in-memory state:** sync.RWMutex with write-lock on mutations, read-lock on reads; race-clean under concurrent workload.
- **Atomic file persistence:** full temp+Sync+Rename+dir-fsync recipe; byte-stable output (sorted manifests, sorted MIME types, 2-space indent); corrupted/version-wrong/unknown-field fail-fast.
- **Symmetric MIME matching:** 3×3 matrix with per-case symmetry assertion; canonicalizer bug fixes for double-slash and `*/subtype`; exported wrapper for Phase 4 query pre-validation.
- **Independently testable:** all 29 test funcs run from within the package; no external process, no network, no transport dependencies.

Phase 2 is ready to ship. Phase 4 (HTTP API) can consume the registry public surface per the handoff notes in 02-03-SUMMARY.md.

---

*Verified: 2026-04-10T10:15:00Z*
*Verifier: Claude (gsd-verifier)*
