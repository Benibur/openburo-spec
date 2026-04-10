---
phase: 2
slug: registry-core
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-10
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go stdlib `testing` + `github.com/stretchr/testify/require` v1.11.x (established in Phase 1) |
| **Config file** | None (`go test` discovers `*_test.go` automatically) |
| **Quick run command** | `go test ./internal/registry -count=1` |
| **Full suite command** | `go test ./... -race -count=1` |
| **Per-test filter** | `go test ./internal/registry -run TestName -v` |
| **Estimated runtime** | ~5 seconds for the registry package, ~10s for the full module under `-race` |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/registry -count=1`
- **After every plan wave:** Run `go test ./internal/registry -race -count=1` + architectural gates below
- **Before `/gsd:verify-work`:** Full suite `go test ./... -race -count=1` + `go build ./...` + `go vet ./...` + `gofmt -l .` all green
- **Max feedback latency:** ~15 seconds

---

## Per-Requirement Verification Map

> **Note on test naming:** Plans are free to consolidate related subtests under a shared parent test (e.g. `TestManifestValidate` with table-driven subtests). The `-run` patterns below are illustrative — the authoritative invocation for this phase is `go test ./internal/registry -race -count=1`, which runs every subtest regardless of naming.

| Requirement | Behavior | Test Type | Automated Command | Wave 0 File | Status |
|-------------|----------|-----------|-------------------|-------------|--------|
| REG-01 | Manifest validates required fields (id, name, url, version, capabilities) | unit (table) | `go test ./internal/registry -run TestManifestValidate` | ✅ `internal/registry/manifest_test.go` | ⬜ pending |
| REG-02 | `capabilities[].action` validated against `PICK\|SAVE` enum (case-sensitive) | unit (subtest of REG-01) | `go test ./internal/registry -run TestManifestValidate` | ✅ same file | ⬜ pending |
| REG-03 | `mimeTypes` non-empty, canonicalized (lowercased, params stripped) | unit | `go test ./internal/registry -run 'TestManifestValidate\|TestCanonicalizeMIME'` | ✅ `manifest_test.go` + `mime_test.go` | ⬜ pending |
| REG-04 | `Store` RWMutex: mutations serialize, reads parallelize, no races | unit (race) | `go test ./internal/registry -run TestStore_ConcurrentAccess -race` | ✅ `store_test.go` | ⬜ pending |
| REG-05 | `Store.Upsert` creates-if-absent, fully-replaces-if-present | unit | `go test ./internal/registry -run TestStore_Upsert` | ✅ `store_test.go` | ⬜ pending |
| REG-06 | `Store.Delete` returns `(existed bool, err error)` | unit | `go test ./internal/registry -run TestStore_Delete` | ✅ `store_test.go` | ⬜ pending |
| REG-07 | `Store.Get` returns `(Manifest, bool)` | unit | `go test ./internal/registry -run TestStore_Get` | ✅ `store_test.go` | ⬜ pending |
| REG-08 | `Store.List` returns all manifests sorted by id | unit | `go test ./internal/registry -run TestStore_List` | ✅ `store_test.go` | ⬜ pending |
| CAP-01 | `Store.Capabilities(filter)` returns flattened `CapabilityView` with denormalized `appId`+`appName` | unit | `go test ./internal/registry -run TestStore_Capabilities` | ✅ `store_test.go` | ⬜ pending |
| CAP-02 | Capability results sorted by `(lower(appName), appId, action, path)` | unit | `go test ./internal/registry -run TestStore_Capabilities` | ✅ same file | ⬜ pending |
| CAP-03 | Filter by `action` returns only exact matches | unit | `go test ./internal/registry -run TestStore_Capabilities` | ✅ same file | ⬜ pending |
| CAP-04 | Filter by `mimeType` supports symmetric 3×3 wildcard matching | unit | `go test ./internal/registry -run 'TestStore_Capabilities\|TestMimeMatch'` | ✅ `store_test.go` + `mime_test.go` | ⬜ pending |
| CAP-05 | Exhaustive table-driven test over all 9 wildcard combinations + malformed input rejection | unit | `go test ./internal/registry -run 'TestMimeMatch\|TestCanonicalizeMIME' -v` | ✅ `mime_test.go` | ⬜ pending |
| PERS-01 | Missing `registry.json` at startup → empty store, no error | unit | `go test ./internal/registry -run TestNewStore_MissingFile` | ✅ `persist_test.go` | ⬜ pending |
| PERS-02 | Every mutation persists via temp+fsync+rename+dir-fsync | unit (golden-file) | `go test ./internal/registry -run TestStore_Upsert_WritesAtomically` | ✅ `persist_test.go` | ⬜ pending |
| PERS-03 | Persist failure rolls back in-memory state (unwritable directory test) | unit | `go test ./internal/registry -run TestStore_Upsert_PersistFailureRollsBack` | ✅ `store_test.go` | ⬜ pending |
| PERS-04 | Corrupted `registry.json` fails fast with clear error | unit (fixture) | `go test ./internal/registry -run TestNewStore_CorruptedFile` | ✅ `persist_test.go` + corrupted fixtures | ⬜ pending |
| PERS-05 | `registry.json` written human-readable (2-space indented) | unit (golden-file) | `go test ./internal/registry -run TestStore_Upsert_WritesIndentedJSON` | ✅ `persist_test.go` | ⬜ pending |
| TEST-01 | Table-driven tests cover Validate, Store, MIME matrix | meta | `go test ./internal/registry -v` (all tests above pass) | ✅ all test files | ⬜ pending |
| TEST-04 | Rollback test uses unwritable directory | (same as PERS-03) | `go test ./internal/registry -run TestStore_Upsert_PersistFailureRollsBack` | ✅ same as PERS-03 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Architectural Gates (Non-REQ but phase-level acceptance criteria)

| Gate | Command | Purpose |
|------|---------|---------|
| No transport imports | `! go list -deps ./internal/registry 2>&1 \| grep -E 'wshub\|httpapi'` | Enforces PITFALLS #1 — ABBA deadlock prevention by construction |
| No logger imports | `! grep -rE 'log/slog\|slog\.Default' internal/registry/*.go` (non-test files only) | Enforces "Store is logger-free" — errors bubble up to handler layer |
| Race detector clean | `go test ./internal/registry -race -count=1` | Catches any concurrency bugs in Store mutation/read paths |
| gofmt clean | `test -z "$(gofmt -l internal/registry/)"` | Project-wide convention |
| go vet clean | `go vet ./internal/registry` | Project-wide convention |
| Package builds | `go build ./internal/registry` | Standalone build check (phase core) |
| Full module builds | `go build ./...` | Ensures Phase 2 doesn't break Phase 1 consumers (none yet, but will matter once Phase 4 lands) |

---

## Wave 0 Requirements

Wave 0 is the "create the files" wave. Plans land files incrementally but Wave 0 must finish with every file below present (even if initially empty stubs that later tasks fill in).

- [ ] `internal/registry/manifest.go` — `Manifest`, `Capability`, `CapabilityProps` types + `Validate()` method
- [ ] `internal/registry/manifest_test.go` — `TestManifestValidate` table-driven
- [ ] `internal/registry/mime.go` — `canonicalizeMIME`, `mimeMatch`, exported `CanonicalizeMIME` wrapper for Phase 4
- [ ] `internal/registry/mime_test.go` — `TestMimeMatch` (3×3 matrix + symmetry check) + `TestCanonicalizeMIME` (edge-case table)
- [ ] `internal/registry/store.go` — `Store`, `NewStore`, `Upsert`, `Delete`, `Get`, `List`, `Capabilities`, `CapabilityView`, `CapabilityFilter`
- [ ] `internal/registry/store_test.go` — `TestStore_Upsert`, `TestStore_Delete`, `TestStore_Get`, `TestStore_List`, `TestStore_Capabilities`, `TestStore_ConcurrentAccess`, `TestStore_Upsert_PersistFailureRollsBack`
- [ ] `internal/registry/persist.go` — `persistLocked`, `loadFromFile`, `fileFormat` internal type, `currentFormatVersion` constant
- [ ] `internal/registry/persist_test.go` — `TestNewStore_MissingFile`, `TestNewStore_CorruptedFile`, `TestStore_Upsert_WritesAtomically`, `TestStore_Upsert_WritesIndentedJSON`
- [ ] `internal/registry/testdata/empty.json` — `{"version":1,"manifests":[]}`
- [ ] `internal/registry/testdata/valid-two-apps.json` — canonical valid file with 2 manifests
- [ ] `internal/registry/testdata/malformed-json.json` — broken JSON (missing brace) for PERS-04
- [ ] `internal/registry/testdata/wrong-version.json` — `{"version":2,...}` for PERS-04
- [ ] `internal/registry/testdata/invalid-manifest.json` — structurally valid JSON, semantically invalid manifest (bad action or missing url)
- [ ] `internal/registry/testdata/unknown-field.json` — has a top-level field that isn't `version`/`manifests`
- [ ] `internal/registry/doc.go` — can be deleted/replaced once `manifest.go` or `store.go` carries the canonical package doc

**Framework install:** None — stdlib `testing` and `testify/require v1.11` are already in `go.mod` from Phase 1.

**First task side-effect:** The first task of the first plan must also update `.planning/REQUIREMENTS.md` CAP-02 to mention the final sort key. This is already done (committed in `aa2c1ef` as part of CONTEXT capture), so the planner should verify the text matches the Store behavior and not re-edit.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| `registry.json` golden-file output is human-readable and diffable across restarts | PERS-05 (partial) | Automated test compares bytes; visual review confirms the file is actually pleasant to inspect | `cat internal/registry/testdata/expected-after-upsert.json` during development |
| `go list -deps ./internal/registry` produces no surprising transitive imports | PITFALLS #1 | Automated grep catches `wshub`/`httpapi` but subtle future regressions (e.g. a dep pulling in an HTTP helper) are easier to spot visually | Run `go list -deps ./internal/registry \| sort` once at phase close; cross-check against expected stdlib-only list |

---

## Validation Sign-Off

- [ ] All tasks have automated verify commands or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags (`go test` is one-shot — N/A)
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
