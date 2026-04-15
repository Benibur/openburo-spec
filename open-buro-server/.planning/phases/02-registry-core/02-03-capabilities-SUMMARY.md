---
phase: 02-registry-core
plan: 03
subsystem: registry
tags: [go, store, capabilities, filter, sort, mime, tdd]

# Dependency graph
requires:
  - phase: 02-registry-core
    provides: Plan 02-01 — canonicalizeMIME, CanonicalizeMIME, symmetric mimeMatch (3x3 matrix); Plan 02-02 — Store, NewStore, Upsert, Delete, Get, List, CapabilityView + CapabilityFilter type stubs
provides:
  - Store.Capabilities(filter CapabilityFilter) []CapabilityView — flattened, filtered, deterministically-sorted view over all manifests
  - CAP-01..04 closed (CAP-05 already landed in Plan 02-01)
  - Phase 2 complete: all 20 requirements (REG-01..08, CAP-01..05, PERS-01..05, TEST-01, TEST-04) implemented and tested
affects: [04-http-api]

# Tech tracking
tech-stack:
  added: []  # zero new dependencies — stdlib-only
  patterns:
    - "Read-only RLock-guarded query over Store map; single canonicalization of filter MIME outside the loop"
    - "OR semantics on multi-mime capability: break on first match against symmetric mimeMatch"
    - "4-key stable tiebreaker sort via sort.SliceStable (lower(appName), appID, action, path)"
    - "Malformed filter MIME -> nil/empty slice (not error); callers pre-validate via CanonicalizeMIME"
    - "TDD RED/GREEN with verbatim plan test table"

key-files:
  created: []
  modified:
    - internal/registry/store.go
    - internal/registry/store_test.go

key-decisions:
  - "Open Question 3 LOCKED: malformed filter.MimeType returns an empty slice (no error). Phase 4 handler chooses to pre-validate via CanonicalizeMIME for a 400 response or pass through for empty-result semantics."
  - "Capabilities acquires RLock (not Lock) so multiple concurrent queries are parallel-safe against each other but still serialized against in-flight Upsert/Delete."
  - "Sort uses sort.SliceStable (not sort.Slice) to keep the Plan's locked tiebreaker ordering deterministic even when equal keys appear (e.g. two capabilities at the same path with the same action cannot both exist but the stability guarantee is part of the observable contract)."
  - "Filter MIME is canonicalized exactly ONCE, outside the manifest/capability loop, so repeated queries pay the canonicalization cost O(1) not O(caps)."
  - "Capability-side mimeTypes are already canonical (Validate() sorted and canonicalized them at Upsert time, Plan 02-01), so mimeMatch receives two canonical inputs per comparison — no per-query re-canonicalization cost."
  - "The OR-over-cap-mimes loop breaks on first match (not a count), so a capability with 20 mimeTypes and a wildcard query returns after one comparison."

patterns-established:
  - "Store query methods (Capabilities, future ListByAction, etc.) all follow the RLock-copy-filter-sort pattern established here."
  - "The denormalization (AppID + AppName copied into each CapabilityView) means Phase 4's JSON handler needs ZERO second lookups: Store.Capabilities(...) output maps directly to the API response shape."

requirements-completed: [CAP-01, CAP-02, CAP-03, CAP-04]

# Metrics
duration: ~2min
completed: 2026-04-10
---

# Phase 02 Plan 03: Capabilities Summary

**Store.Capabilities(filter) — flattened, filtered, 4-key deterministically-sorted capability view with symmetric MIME matching OR semantics over multi-mime capabilities; closes CAP-01..04 and ships Phase 2 with all 20 requirements implemented and tested (29 test funcs / 103 subtests passing under -race).**

## Performance

- **Duration:** ~2 min
- **Started:** 2026-04-10T10:00:43Z
- **Completed:** 2026-04-10T10:02:29Z
- **Tasks:** 1 (TDD RED/GREEN)
- **Files modified:** 2 (store.go, store_test.go)

## Accomplishments

- Phase 2's last public-API piece (Store.Capabilities) is frozen with 14 subtests covering flatten, 4-key sort, action filter (PICK/SAVE/case-sensitive), MIME filter (exact/type-wildcard/full-wildcard), symmetric capability-side wildcard, OR semantics over multi-mime, malformed filter, combined filters, and empty store
- The symmetric 3x3 wildcard matching contract (Plan 02-01 TestMimeMatch) is proven at the Store integration level, not just at the pure-function level: `files-app` with `*/*` matches a concrete `image/png` query via one-sided wildcard (subtest `filter by mimeType against capability wildcard (symmetric)`)
- Filter MIME canonicalization happens ONCE outside the loop — `"IMAGE/PNG; charset=utf-8"` is normalized to `"image/png"` before any comparison, verified by the exact-filter subtest which passes both raw and param-bearing forms
- Malformed filter.MimeType (e.g. `"not a valid mime"`, `"*/subtype"`) returns an empty slice, not an error — open question #3 locked. Phase 4 can pre-validate via the already-exported `CanonicalizeMIME` if it wants a 400 response.
- Zero new go.mod dependencies; zero transport/slog imports in the package; `go list -deps ./internal/registry | grep -E 'wshub|httpapi'` still empty
- Phase 2 verification gate fully green: `go test ./... -race -count=1` across the whole module, `go vet`, `gofmt`, architectural gates

## Task Commits

TDD RED/GREEN:

1. **RED: failing TestStore_Capabilities with 14 subtests** — `94464d4` (test)
2. **GREEN: Store.Capabilities filter + sort implementation** — `189abe6` (feat)

_Plan metadata commit follows this summary._

## Files Created/Modified

- `internal/registry/store.go` — added `strings` import; appended `Capabilities(filter CapabilityFilter) []CapabilityView` method (78 new lines, 1 stub comment removed)
- `internal/registry/store_test.go` — appended `TestStore_Capabilities` (200 new lines)

## TestStore_Capabilities Subtest Breakdown

| # | Subtest | Exercises |
|---|---------|-----------|
| 1 | `flatten` | 2 manifests → 3 CapabilityView entries; every field denormalized (AppID, AppName, Action, Path, Properties.MimeTypes non-empty) |
| 2 | `sort by lower appName then appID then action then path` | Case-insensitive name sort + appID tiebreaker: Archive(gamma) < mail(alpha) < Mail(zebra) |
| 3 | `sort action and path tiebreakers` | Same name + same id → action tiebreaker (PICK < SAVE) → path tiebreaker (/a < /b) |
| 4 | `filter by action PICK` | Exact-match, only PICK entries returned |
| 5 | `filter by action SAVE` | Exact-match, only SAVE entries returned |
| 6 | `filter by action is case-sensitive` | Lowercase "pick" → empty (mirrors Validate enum exactness) |
| 7 | `filter by mimeType exact` | `image/png` matches only the png cap; `IMAGE/PNG; charset=utf-8` canonicalizes and matches the same |
| 8 | `filter by mimeType type wildcard` | `image/*` matches `image/png`, excludes `text/plain` |
| 9 | `filter by mimeType full wildcard returns all` | `*/*` matches every capability |
| 10 | `filter by mimeType against capability wildcard (symmetric)` | Capability declares `*/*`, query `image/png` — Files-app pattern — matches via symmetric 3x3 at the Store integration level |
| 11 | `filter by mimeType OR semantics over multi-mime capability` | Single cap with `["text/plain","image/png"]` matches `image/png` (ANY-of), rejects `video/mp4` |
| 12 | `malformed filter mimeType returns empty` | `"not a valid mime"` → empty; `"*/subtype"` → empty (open question 3 locked) |
| 13 | `combined action and mimeType filters` | AND of action=PICK and mime=image/* narrows to 1 capability |
| 14 | `empty store returns empty slice for any filter` | Fresh NewStore → empty for all filter shapes |

**14 subtests, all passing under `-race -count=1`.**

## Phase 2 Test Count (final)

| Test file | Top-level funcs | Notes |
|-----------|-----------------|-------|
| mime_test.go | 3 | TestMimeMatch (17 subtests), TestCanonicalizeMIME (24 subtests), TestCanonicalizeMIME_Exported |
| manifest_test.go | 3 | TestManifestValidate_Happy, TestManifestValidate_CanonicalizesInPlace, TestManifestValidate_Errors (23 subtests) |
| persist_test.go | 11 | 8 NewStore load paths + 3 atomic-write assertions |
| store_test.go | **12** | 11 from Plan 02-02 + **TestStore_Capabilities (14 subtests) added by this plan** |
| **Total** | **29 funcs** | ~103 cases, all passing under `-race -count=1` in ~1.3s |

`ok  github.com/openburo/openburo-server/internal/registry  1.338s`

## Decisions Made

1. **Open Question #3 LOCKED (malformed filter MIME):** Returns empty slice, not an error. Verified by `TestStore_Capabilities/malformed filter mimeType returns empty` with both `"not a valid mime"` and `"*/subtype"` inputs. Phase 4 can pre-validate via `CanonicalizeMIME` (exported in Plan 02-01) for a distinct 400 response.

2. **Single canonicalization outside the loop.** `canonicalizeMIME(filter.MimeType)` is called exactly once before the manifest iteration, stored in `wantMime`, and `wantMimeSet` gates the per-capability loop. This means a query against N manifests with K mimeTypes each pays canonicalization cost O(1), not O(N*K).

3. **RLock (not Lock).** `Capabilities` is a read-only query and acquires `s.mu.RLock()`. The concurrency test for Capabilities is deferred to Phase 4 integration (the RWMutex is the same one proven by Plan 02-02's TestStore_ConcurrentAccess which already covers the read-parallel-write-serialized contract).

4. **sort.SliceStable, not sort.Slice.** Stability is part of the locked plan and guards against future test brittleness if the map iteration order changes.

## Deviations from Plan

None — plan executed exactly as written.

TDD RED/GREEN flow was clean:
- RED: tests pasted verbatim from plan; compile-fail confirmed on `store.Capabilities undefined` (expected — method stub was a comment).
- GREEN: added `strings` import + Capabilities method verbatim from `<capabilities_implementation>` block; all 14 subtests passed on first run.
- `go vet`, `gofmt -l internal/registry/`, architectural gates (`go list -deps | grep wshub|httpapi` empty; no `log/slog` in production files) all passed without modification.
- `go mod tidy && git diff --exit-code go.mod go.sum` clean — zero new dependencies.

**Total deviations:** 0
**Impact on plan:** None.

## Issues Encountered

- `/usr/bin/go` tries to auto-fetch Go 1.26 toolchain and fails (no network). Resolved by using `$HOME/sdk/go1.26.2/bin/go` as in Plans 02-01 and 02-02. No code changes. Same documented workaround.

## Verification Gate Results

All commands from the Phase 2 exit gate exit 0:

```
go test ./internal/registry -race -count=1 -v     -> PASS (29 funcs / ~103 cases, 1.338s)
go test ./... -race -count=1                      -> PASS (config, httpapi, registry all ok)
go build ./...                                    -> OK
go vet ./internal/registry                        -> clean
gofmt -l internal/registry/                       -> empty
go list -deps ./internal/registry | grep wshub|httpapi -> empty (architectural isolation)
grep -rE 'log/slog|slog\.Default' internal/registry/*.go | grep -v _test.go -> empty
go mod tidy && git diff --exit-code go.mod go.sum -> clean (zero new deps)
```

## Phase 2 Requirements Closed

All 20 Phase 2 requirements are now implemented and tested:

| ID | Status | Landed in |
|----|--------|-----------|
| REG-01 Manifest validates required fields | Complete | 02-01 |
| REG-02 action enum PICK/SAVE | Complete | 02-01 |
| REG-03 mimeTypes canonicalized | Complete | 02-01 |
| REG-04 Store guards state with RWMutex | Complete | 02-02 |
| REG-05 Upsert create-or-replace | Complete | 02-02 |
| REG-06 Delete reports existence | Complete | 02-02 |
| REG-07 Get returns manifest-or-not-found | Complete | 02-02 |
| REG-08 List deterministic order | Complete | 02-02 |
| **CAP-01 Capabilities flattened view** | **Complete** | **02-03** |
| **CAP-02 4-key sort (lower appName, appID, action, path)** | **Complete** | **02-03** |
| **CAP-03 action exact-match filter** | **Complete** | **02-03** |
| **CAP-04 symmetric 3x3 MIME filter** | **Complete** | **02-03** |
| CAP-05 exhaustive MIME matching test | Complete | 02-01 |
| PERS-01 missing file -> empty registry | Complete | 02-02 |
| PERS-02 atomic temp+Sync+Rename+dir-fsync | Complete | 02-02 |
| PERS-03 persist failure -> rollback | Complete | 02-02 |
| PERS-04 corrupted file fail-fast | Complete | 02-02 |
| PERS-05 indented JSON | Complete | 02-02 |
| TEST-01 table-driven Validate/Store/MIME | Complete | 02-01/02-02/02-03 |
| TEST-04 unwritable-dir rollback recipe | Complete | 02-02 |

**20 / 20 complete.** Phase 2 is ready for verification.

## Handoff Notes for Phase 4

The `internal/registry` package is fully stable and self-contained. Phase 4's HTTP handler layer can consume it as follows:

### Import and construct

```go
import "github.com/openburo/openburo-server/internal/registry"

store, err := registry.NewStore(cfg.RegistryFile)
if err != nil {
    return fmt.Errorf("load registry: %w", err)
}
```

### Write routes (POST/DELETE)

```go
// POST /api/v1/registry
var m registry.Manifest
if err := json.NewDecoder(r.Body).Decode(&m); err != nil { /* 400 */ }
if err := store.Upsert(m); err != nil {
    if strings.Contains(err.Error(), "validate:") { /* 400 */ }
    if strings.Contains(err.Error(), "registry unchanged") { /* 500 */ }
    /* 500 */
}
// 201 if first time, 200 if replace — compute via Get() before Upsert, or check List size change.

// DELETE /api/v1/registry/{appId}
existed, err := store.Delete(appID)
if err != nil { /* 500 registry unchanged */ }
if !existed { /* 404 */ }
// 204
```

### Read routes (GET)

```go
// GET /api/v1/registry
out := store.List()  // already sorted by id, already a copy — safe to JSON-encode directly

// GET /api/v1/registry/{appId}
m, ok := store.Get(appID)
if !ok { /* 404 */ }
// 200 JSON

// GET /api/v1/capabilities?action=PICK&mimeType=image/png
filter := registry.CapabilityFilter{Action: r.URL.Query().Get("action")}
if raw := r.URL.Query().Get("mimeType"); raw != "" {
    canon, err := registry.CanonicalizeMIME(raw)
    if err != nil { /* 400 — don't pass through */ }
    filter.MimeType = canon
}
caps := store.Capabilities(filter)
// 200: {"capabilities": caps, "count": len(caps)}
```

### Key contracts to honor

- **Never import wshub from registry** — enforced by the architectural gate; broadcast logic lives in the handler layer (Phase 4 WS-09).
- **Mutation-then-broadcast ordering** — call `Upsert`/`Delete` first, THEN publish the `REGISTRY_UPDATED` event to the hub. The registry package already returns rollback errors with `"registry unchanged"` as the observable signal.
- **CanonicalizeMIME for query pre-validation** — if you want a distinct 400 on malformed `?mimeType=`, call `registry.CanonicalizeMIME` before passing into `CapabilityFilter`. Otherwise pass-through and let Store.Capabilities return an empty result.
- **All reads return copies** — caller can mutate the returned slices without affecting the Store, so no locking concerns on the handler side.
- **Manifest.Validate mutates in place** — after Upsert, stored manifests have canonicalized + sorted MimeTypes. Read paths return already-canonical data.
- **Single file persistence, byte-stable** — `registry.json` is diffable in git: 2-space indent, manifests sorted by id, mimeTypes sorted alphabetically.

### Phase 4 will need new test files (Phase 4 scope, not here)

- `httpapi/registry_handler_test.go` — httptest round-trips for POST/DELETE/GET
- `httpapi/capabilities_handler_test.go` — `?action=` and `?mimeType=` query params, 400 on malformed filter
- `httpapi/auth_test.go` — Basic Auth + timing-safe comparison
- Each handler test constructs its own `registry.NewStore(t.TempDir()+"/reg.json")` — no shared state

## Next Plan Readiness

- **Phase 2 exit:** COMPLETE. All 20 requirements implemented and tested. `go test ./... -race -count=1` green across the whole module. Architectural gates green. No new dependencies.
- **Phase 3 (wshub):** unblocked and parallel-safe (disjoint dependency graph — wshub does not import registry).
- **Phase 4 (httpapi):** unblocked. The registry public surface (`NewStore`, `Upsert`, `Delete`, `Get`, `List`, `Capabilities`, `CanonicalizeMIME`) is frozen. Handler implementation can begin immediately.

## Self-Check: PASSED

Verified on disk:
- `internal/registry/store.go` FOUND (contains `func (s *Store) Capabilities`)
- `internal/registry/store_test.go` FOUND (contains `TestStore_Capabilities`)
- Commit `94464d4` FOUND (test RED TestStore_Capabilities)
- Commit `189abe6` FOUND (feat GREEN Store.Capabilities)

---
*Phase: 02-registry-core*
*Plan: 03-capabilities*
*Completed: 2026-04-10*
