---
phase: 02-registry-core
plan: 03
type: execute
wave: 3
depends_on:
  - 02-01
  - 02-02
files_modified:
  - internal/registry/store.go
  - internal/registry/store_test.go
autonomous: true
requirements:
  - CAP-01
  - CAP-02
  - CAP-03
  - CAP-04

must_haves:
  truths:
    - "Store.Capabilities returns a flattened CapabilityView slice with AppID and AppName denormalized from the owning Manifest"
    - "Results are sorted by lower(AppName) with AppID, Action, Path tiebreakers using sort.SliceStable"
    - "Filter by action is exact-match case-sensitive; only PICK or SAVE narrows results"
    - "Filter by mimeType uses symmetric mimeMatch against EVERY MIME in the capability (OR semantics)"
    - "Malformed filter.MimeType returns an empty slice (not an error) — callers use CanonicalizeMIME to pre-validate if they want a 400"
    - "Capabilities is read-only under RLock and safe for concurrent callers (race-clean)"
  artifacts:
    - path: "internal/registry/store.go"
      provides: "Capabilities(filter CapabilityFilter) []CapabilityView method appended to existing Store"
      contains: "func (s *Store) Capabilities"
    - path: "internal/registry/store_test.go"
      provides: "TestStore_Capabilities with flatten / sort / filter_action / filter_mime / malformed_filter subtests"
      contains: "TestStore_Capabilities"
  key_links:
    - from: "internal/registry/store.go Capabilities"
      to: "internal/registry/mime.go mimeMatch"
      via: "OR-over-capability-mimeTypes loop against canonicalized query"
      pattern: "mimeMatch\\("
    - from: "internal/registry/store.go Capabilities sort"
      to: "strings.ToLower(appName) comparator"
      via: "sort.SliceStable with 4-key tiebreaker"
      pattern: "sort\\.SliceStable"
---

<objective>
Complete `internal/registry` by adding `Store.Capabilities(filter)` — the flattened, filtered, deterministically-sorted capability view that Phase 4's `GET /api/v1/capabilities` handler will consume. This plan closes CAP-01..04, leaving all 20 Phase 2 requirements implemented and tested. CAP-05 (the exhaustive 3x3 matrix test) already landed in Plan 02-01 as `TestMimeMatch`; this plan wires `mimeMatch` into `Store.Capabilities` and proves the integration.

Purpose: Ship the last piece of the registry's public API. After this plan, Phase 4 can import `internal/registry` and build HTTP handlers with zero blockers.

Output: `Store.Capabilities` method appended to `store.go`; new `TestStore_Capabilities` with flatten/sort/filter_action/filter_mime subtests added to `store_test.go`.
</objective>

<execution_context>
@/home/ben/.claude/get-shit-done/workflows/execute-plan.md
@/home/ben/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/REQUIREMENTS.md
@.planning/phases/02-registry-core/02-CONTEXT.md
@.planning/phases/02-registry-core/02-RESEARCH.md
@.planning/phases/02-registry-core/02-VALIDATION.md
@.planning/phases/02-registry-core/02-01-manifest-mime-SUMMARY.md
@.planning/phases/02-registry-core/02-02-store-persist-SUMMARY.md
@internal/registry/store.go
@internal/registry/store_test.go
@internal/registry/mime.go
@internal/registry/manifest.go

<interfaces>
<!-- These types already exist from Plan 02-02 — quoting for executor convenience. -->

```go
// From store.go (Plan 02-02):
type CapabilityView struct {
    AppID      string          `json:"appId"`
    AppName    string          `json:"appName"`
    Action     string          `json:"action"`
    Path       string          `json:"path"`
    Properties CapabilityProps `json:"properties"`
}

type CapabilityFilter struct {
    Action   string // "PICK" | "SAVE" | ""
    MimeType string // any form accepted by CanonicalizeMIME, or ""
}

// From mime.go (Plan 02-01):
func mimeMatch(cap, q string) bool
func canonicalizeMIME(s string) (string, error)
func CanonicalizeMIME(s string) (string, error) // exported wrapper
```
</interfaces>

<capabilities_implementation>
<!-- Copy VERBATIM from RESEARCH §"Example 8: Store.Capabilities Filter+Sort Implementation". -->

```go
// Capabilities returns all capabilities across all manifests as
// CapabilityView entries, filtered by filter and sorted per Phase 2
// rules: (lower(AppName), AppID, Action, Path). Filter is applied before
// sort. An empty filter returns every capability. A malformed
// filter.MimeType yields an empty slice (no error); callers wanting a
// 400-on-malformed-query should pre-validate via CanonicalizeMIME.
//
// OR semantics on MIME filter: a capability matches if ANY of its declared
// MIME types matches the query under symmetric 3x3 wildcard matching.
func (s *Store) Capabilities(filter CapabilityFilter) []CapabilityView {
    s.mu.RLock()
    defer s.mu.RUnlock()

    // Canonicalize the query MIME once, outside the loop.
    var wantMime string
    var wantMimeSet bool
    if filter.MimeType != "" {
        canon, err := canonicalizeMIME(filter.MimeType)
        if err != nil {
            // Open question #3 lock: malformed filter.MimeType → empty result,
            // not an error. Callers pre-validate with CanonicalizeMIME if they
            // want a distinct 400 response.
            return nil
        }
        wantMime = canon
        wantMimeSet = true
    }

    out := make([]CapabilityView, 0)
    for _, m := range s.manifests {
        for _, c := range m.Capabilities {
            // Action filter: exact match, case-sensitive.
            if filter.Action != "" && c.Action != filter.Action {
                continue
            }
            // MIME filter: OR over the capability's declared mimeTypes.
            if wantMimeSet {
                matched := false
                for _, capMime := range c.Properties.MimeTypes {
                    if mimeMatch(capMime, wantMime) {
                        matched = true
                        break
                    }
                }
                if !matched {
                    continue
                }
            }
            out = append(out, CapabilityView{
                AppID:      m.ID,
                AppName:    m.Name,
                Action:     c.Action,
                Path:       c.Path,
                Properties: c.Properties,
            })
        }
    }

    // 4-key stable sort: (lower(appName), appID, action, path).
    sort.SliceStable(out, func(i, j int) bool {
        ai := strings.ToLower(out[i].AppName)
        aj := strings.ToLower(out[j].AppName)
        if ai != aj {
            return ai < aj
        }
        if out[i].AppID != out[j].AppID {
            return out[i].AppID < out[j].AppID
        }
        if out[i].Action != out[j].Action {
            return out[i].Action < out[j].Action
        }
        return out[i].Path < out[j].Path
    })

    return out
}
```
</capabilities_implementation>

<locked_open_questions>
<!-- Open question #3 from RESEARCH is applied in this plan. -->
3. Malformed filter.MimeType → empty result, not an error. The exported `CanonicalizeMIME` wrapper (created in Plan 02-01) lets Phase 4 pre-validate and choose whether to 400 or pass-through. Locked in the action text above.
</locked_open_questions>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Store.Capabilities filter + sort + tests</name>
  <files>internal/registry/store.go, internal/registry/store_test.go</files>
  <read_first>
- internal/registry/store.go (Plan 02-02 — Store struct, NewStore, Upsert, Delete, Get, List all exist; CapabilityView and CapabilityFilter types already declared; add Capabilities method)
- internal/registry/store_test.go (Plan 02-02 — has `newEmptyStore`, `sampleManifest` helpers; add TestStore_Capabilities using them)
- internal/registry/mime.go (Plan 02-01 — mimeMatch and canonicalizeMIME)
- internal/registry/manifest.go (Plan 02-01 — Manifest types)
- .planning/phases/02-registry-core/02-CONTEXT.md §"Capability response shape" + §"Capability sort order"
- .planning/phases/02-registry-core/02-RESEARCH.md §"Example 8: Store.Capabilities Filter+Sort Implementation"
- .planning/phases/02-registry-core/02-VALIDATION.md (rows CAP-01, CAP-02, CAP-03, CAP-04)
  </read_first>
  <behavior>
**TestStore_Capabilities** with named subtests:

- **`flatten`**: seed 2 manifests, one with 2 capabilities (PICK, SAVE) and one with 1 capability (PICK). Call `Capabilities(CapabilityFilter{})` → expect 3 CapabilityView entries, each with the correct AppID, AppName (denormalized from the owning Manifest), Action, Path, and Properties.MimeTypes.

- **`sort`**: seed 3 manifests with deliberately crafted AppNames to exercise every tiebreaker:
  - `{ID: "zebra", Name: "Mail"}` capability PICK /pick
  - `{ID: "alpha", Name: "mail"}` capability PICK /pick → same lowercased name, earlier id, should come before `zebra`
  - `{ID: "gamma", Name: "Archive"}` capability PICK /pick → lowercase "archive" < "mail", should come first
  
  Expected order: `Archive (gamma)`, `mail (alpha)`, `Mail (zebra)`.

  Add a second case covering action and path tiebreakers with same name + same id: one manifest declaring two PICK capabilities at different paths (`/a` and `/b`) → expect sorted ascending by path. And one manifest declaring PICK and SAVE at the same path → expect PICK before SAVE (alphabetical).

- **`filter_action_pick`**: seed mixed PICK/SAVE capabilities; filter `CapabilityFilter{Action: "PICK"}` → only PICK entries returned.

- **`filter_action_save`**: same seed; filter `CapabilityFilter{Action: "SAVE"}` → only SAVE entries returned.

- **`filter_action_case_sensitive`**: filter `CapabilityFilter{Action: "pick"}` (lowercase) → empty result (action filter is case-sensitive, mirrors Validate's exact-match rule).

- **`filter_mime_exact`**: seed a manifest with MimeTypes `["image/png"]` and another with `["text/plain"]`; filter `CapabilityFilter{MimeType: "image/png"}` → only the png capability. Also try the filter with `"IMAGE/PNG; charset=utf-8"` — must still match (canonicalization applied inside Capabilities).

- **`filter_mime_type_wildcard`**: seed with `["image/png"]` and `["text/plain"]`; filter `CapabilityFilter{MimeType: "image/*"}` → only image/png.

- **`filter_mime_full_wildcard`**: filter `CapabilityFilter{MimeType: "*/*"}` → all capabilities.

- **`filter_mime_capability_wildcard`**: seed a manifest with `MimeTypes: ["*/*"]` (the "Files" app pattern); filter `CapabilityFilter{MimeType: "image/png"}` → the wildcard capability matches (symmetric matching proven at integration level, not just in TestMimeMatch).

- **`filter_mime_or_semantics`**: seed a single capability with `MimeTypes: ["text/plain", "image/png"]`; filter for `"image/png"` → that capability matches (ANY-of semantics). Then filter for `"video/mp4"` → no match.

- **`filter_mime_malformed_returns_empty`**: seed arbitrary data; filter `CapabilityFilter{MimeType: "not a valid mime"}` → empty slice (not a nil-vs-empty assertion necessarily, but `len == 0`). Also try `CapabilityFilter{MimeType: "*/subtype"}` → empty.

- **`combined_filters`**: filter `{Action: "PICK", MimeType: "image/*"}` → only PICK capabilities whose MIME list intersects with image/*.

- **`empty_store`**: a fresh NewStore returns empty slice for any filter.
  </behavior>
  <action>
**Step 1: append the Capabilities method to `internal/registry/store.go`.**

Add to the imports in store.go: `"sort"` (already present from Plan 02-02 Task 1 via the List method) and `"strings"` (new — needed for `strings.ToLower`). Verify both are present before pasting.

Paste the Capabilities method VERBATIM from the <capabilities_implementation> block above.

```go
// Capabilities returns all capabilities across all manifests as
// CapabilityView entries, filtered by filter and sorted per Phase 2
// rules: (lower(AppName), AppID, Action, Path). Filter is applied before
// sort. An empty filter returns every capability. A malformed
// filter.MimeType yields an empty slice (no error); callers wanting a
// 400-on-malformed-query should pre-validate via CanonicalizeMIME.
//
// OR semantics on MIME filter: a capability matches if ANY of its
// declared MIME types matches the query under symmetric 3x3 wildcard
// matching.
func (s *Store) Capabilities(filter CapabilityFilter) []CapabilityView {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var wantMime string
	var wantMimeSet bool
	if filter.MimeType != "" {
		canon, err := canonicalizeMIME(filter.MimeType)
		if err != nil {
			return nil
		}
		wantMime = canon
		wantMimeSet = true
	}

	out := make([]CapabilityView, 0)
	for _, m := range s.manifests {
		for _, c := range m.Capabilities {
			if filter.Action != "" && c.Action != filter.Action {
				continue
			}
			if wantMimeSet {
				matched := false
				for _, capMime := range c.Properties.MimeTypes {
					if mimeMatch(capMime, wantMime) {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
			out = append(out, CapabilityView{
				AppID:      m.ID,
				AppName:    m.Name,
				Action:     c.Action,
				Path:       c.Path,
				Properties: c.Properties,
			})
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		ai := strings.ToLower(out[i].AppName)
		aj := strings.ToLower(out[j].AppName)
		if ai != aj {
			return ai < aj
		}
		if out[i].AppID != out[j].AppID {
			return out[i].AppID < out[j].AppID
		}
		if out[i].Action != out[j].Action {
			return out[i].Action < out[j].Action
		}
		return out[i].Path < out[j].Path
	})

	return out
}
```

**Step 2: append TestStore_Capabilities to `internal/registry/store_test.go`.**

Use the existing `newEmptyStore(t)` and `sampleManifest(id, name)` helpers from Plan 02-02 Task 2. For test cases that need multi-capability manifests, define a local builder helper at the top of the function body.

```go
// (appended to store_test.go)

func TestStore_Capabilities(t *testing.T) {
	// Local builder — sampleManifest only makes single-capability manifests.
	makeManifest := func(id, name string, caps []Capability) Manifest {
		return Manifest{
			ID:           id,
			Name:         name,
			URL:          "https://example.com",
			Version:      "1.0.0",
			Capabilities: caps,
		}
	}
	capPick := func(path string, mimes ...string) Capability {
		return Capability{Action: "PICK", Path: path, Properties: CapabilityProps{MimeTypes: mimes}}
	}
	capSave := func(path string, mimes ...string) Capability {
		return Capability{Action: "SAVE", Path: path, Properties: CapabilityProps{MimeTypes: mimes}}
	}

	t.Run("flatten", func(t *testing.T) {
		store, _ := newEmptyStore(t)
		require.NoError(t, store.Upsert(makeManifest("mail-app", "Mail", []Capability{
			capPick("/pick", "*/*"),
			capSave("/save", "*/*"),
		})))
		require.NoError(t, store.Upsert(makeManifest("viewer-app", "Viewer", []Capability{
			capPick("/view", "image/png"),
		})))

		got := store.Capabilities(CapabilityFilter{})
		require.Len(t, got, 3)

		// Each entry has denormalized AppID + AppName.
		for _, cv := range got {
			require.NotEmpty(t, cv.AppID)
			require.NotEmpty(t, cv.AppName)
			require.NotEmpty(t, cv.Action)
			require.NotEmpty(t, cv.Path)
			require.NotEmpty(t, cv.Properties.MimeTypes)
		}
	})

	t.Run("sort by lower appName then appID then action then path", func(t *testing.T) {
		store, _ := newEmptyStore(t)
		require.NoError(t, store.Upsert(makeManifest("zebra", "Mail", []Capability{capPick("/p", "*/*")})))
		require.NoError(t, store.Upsert(makeManifest("alpha", "mail", []Capability{capPick("/p", "*/*")})))
		require.NoError(t, store.Upsert(makeManifest("gamma", "Archive", []Capability{capPick("/p", "*/*")})))

		got := store.Capabilities(CapabilityFilter{})
		require.Len(t, got, 3)
		// Expected: Archive (gamma) < mail (alpha) < Mail (zebra)
		require.Equal(t, "gamma", got[0].AppID, "Archive first by lower(name)")
		require.Equal(t, "alpha", got[1].AppID, "alpha before zebra by appID tiebreaker")
		require.Equal(t, "zebra", got[2].AppID)
	})

	t.Run("sort action and path tiebreakers", func(t *testing.T) {
		store, _ := newEmptyStore(t)
		require.NoError(t, store.Upsert(makeManifest("app", "Solo", []Capability{
			capSave("/b", "*/*"),
			capPick("/b", "*/*"),
			capPick("/a", "*/*"),
		})))

		got := store.Capabilities(CapabilityFilter{})
		require.Len(t, got, 3)
		// Same appName + same appId → action tiebreaker (PICK < SAVE).
		// Within PICK → path tiebreaker (/a < /b).
		require.Equal(t, "PICK", got[0].Action)
		require.Equal(t, "/a", got[0].Path)
		require.Equal(t, "PICK", got[1].Action)
		require.Equal(t, "/b", got[1].Path)
		require.Equal(t, "SAVE", got[2].Action)
		require.Equal(t, "/b", got[2].Path)
	})

	t.Run("filter by action PICK", func(t *testing.T) {
		store, _ := newEmptyStore(t)
		require.NoError(t, store.Upsert(makeManifest("app", "App", []Capability{
			capPick("/pick", "*/*"),
			capSave("/save", "*/*"),
		})))
		got := store.Capabilities(CapabilityFilter{Action: "PICK"})
		require.Len(t, got, 1)
		require.Equal(t, "PICK", got[0].Action)
	})

	t.Run("filter by action SAVE", func(t *testing.T) {
		store, _ := newEmptyStore(t)
		require.NoError(t, store.Upsert(makeManifest("app", "App", []Capability{
			capPick("/pick", "*/*"),
			capSave("/save", "*/*"),
		})))
		got := store.Capabilities(CapabilityFilter{Action: "SAVE"})
		require.Len(t, got, 1)
		require.Equal(t, "SAVE", got[0].Action)
	})

	t.Run("filter by action is case-sensitive", func(t *testing.T) {
		store, _ := newEmptyStore(t)
		require.NoError(t, store.Upsert(makeManifest("app", "App", []Capability{
			capPick("/pick", "*/*"),
		})))
		got := store.Capabilities(CapabilityFilter{Action: "pick"})
		require.Empty(t, got)
	})

	t.Run("filter by mimeType exact", func(t *testing.T) {
		store, _ := newEmptyStore(t)
		require.NoError(t, store.Upsert(makeManifest("img", "Img", []Capability{capPick("/p", "image/png")})))
		require.NoError(t, store.Upsert(makeManifest("txt", "Txt", []Capability{capPick("/p", "text/plain")})))

		got := store.Capabilities(CapabilityFilter{MimeType: "image/png"})
		require.Len(t, got, 1)
		require.Equal(t, "img", got[0].AppID)

		// Same query with params + uppercase — canonicalized inside Capabilities.
		got = store.Capabilities(CapabilityFilter{MimeType: "IMAGE/PNG; charset=utf-8"})
		require.Len(t, got, 1)
		require.Equal(t, "img", got[0].AppID)
	})

	t.Run("filter by mimeType type wildcard", func(t *testing.T) {
		store, _ := newEmptyStore(t)
		require.NoError(t, store.Upsert(makeManifest("img", "Img", []Capability{capPick("/p", "image/png")})))
		require.NoError(t, store.Upsert(makeManifest("txt", "Txt", []Capability{capPick("/p", "text/plain")})))

		got := store.Capabilities(CapabilityFilter{MimeType: "image/*"})
		require.Len(t, got, 1)
		require.Equal(t, "img", got[0].AppID)
	})

	t.Run("filter by mimeType full wildcard returns all", func(t *testing.T) {
		store, _ := newEmptyStore(t)
		require.NoError(t, store.Upsert(makeManifest("img", "Img", []Capability{capPick("/p", "image/png")})))
		require.NoError(t, store.Upsert(makeManifest("txt", "Txt", []Capability{capPick("/p", "text/plain")})))

		got := store.Capabilities(CapabilityFilter{MimeType: "*/*"})
		require.Len(t, got, 2)
	})

	t.Run("filter by mimeType against capability wildcard (symmetric)", func(t *testing.T) {
		store, _ := newEmptyStore(t)
		// "Files"-app pattern: declares */* and expects to match any query.
		require.NoError(t, store.Upsert(makeManifest("files", "Files", []Capability{capPick("/p", "*/*")})))
		require.NoError(t, store.Upsert(makeManifest("txt", "Txt", []Capability{capPick("/p", "text/plain")})))

		// Query with exact image/png. Files should match (symmetric), Txt should not.
		got := store.Capabilities(CapabilityFilter{MimeType: "image/png"})
		require.Len(t, got, 1)
		require.Equal(t, "files", got[0].AppID)
	})

	t.Run("filter by mimeType OR semantics over multi-mime capability", func(t *testing.T) {
		store, _ := newEmptyStore(t)
		require.NoError(t, store.Upsert(makeManifest("multi", "Multi", []Capability{
			capPick("/p", "text/plain", "image/png"),
		})))

		// Query matches image/png → the capability (which carries both) matches.
		got := store.Capabilities(CapabilityFilter{MimeType: "image/png"})
		require.Len(t, got, 1)

		// Query for an unrelated type → no match.
		got = store.Capabilities(CapabilityFilter{MimeType: "video/mp4"})
		require.Empty(t, got)
	})

	t.Run("malformed filter mimeType returns empty", func(t *testing.T) {
		store, _ := newEmptyStore(t)
		require.NoError(t, store.Upsert(makeManifest("app", "App", []Capability{capPick("/p", "*/*")})))

		got := store.Capabilities(CapabilityFilter{MimeType: "not a valid mime"})
		require.Empty(t, got, "malformed mime filter → empty, not error")

		got = store.Capabilities(CapabilityFilter{MimeType: "*/subtype"})
		require.Empty(t, got, "rejected wildcard form → empty")
	})

	t.Run("combined action and mimeType filters", func(t *testing.T) {
		store, _ := newEmptyStore(t)
		require.NoError(t, store.Upsert(makeManifest("app", "App", []Capability{
			capPick("/pick", "image/png"),
			capSave("/save", "image/png"),
			capPick("/pick-txt", "text/plain"),
		})))

		got := store.Capabilities(CapabilityFilter{Action: "PICK", MimeType: "image/*"})
		require.Len(t, got, 1)
		require.Equal(t, "PICK", got[0].Action)
		require.Equal(t, "/pick", got[0].Path)
	})

	t.Run("empty store returns empty slice for any filter", func(t *testing.T) {
		store, _ := newEmptyStore(t)
		require.Empty(t, store.Capabilities(CapabilityFilter{}))
		require.Empty(t, store.Capabilities(CapabilityFilter{Action: "PICK"}))
		require.Empty(t, store.Capabilities(CapabilityFilter{MimeType: "image/png"}))
	})
}
```

**Step 3: run the gate commands.**

```
go test ./internal/registry -race -count=1 -v
go build ./...
go vet ./internal/registry
test -z "$(gofmt -l internal/registry/)"
! go list -deps ./internal/registry 2>&1 | grep -E 'wshub|httpapi'
! grep -rE 'log/slog|slog\.Default' internal/registry/*.go | grep -v _test.go
```

All must exit 0.
  </action>
  <verify>
<automated>go test ./internal/registry -race -count=1 -run TestStore_Capabilities -v && go test ./internal/registry -race -count=1 && go build ./... && go vet ./internal/registry && test -z "$(gofmt -l internal/registry/)" && ! go list -deps ./internal/registry 2>&1 | grep -E 'wshub|httpapi'</automated>
  </verify>
  <done>
- `internal/registry/store.go` has a `Capabilities(filter CapabilityFilter) []CapabilityView` method under RLock, filtering by action (exact) and mimeType (OR over capability mimes via mimeMatch), sorting by `(lower(AppName), AppID, Action, Path)` via sort.SliceStable
- `store.go` imports `sort` and `strings` (both present in imports)
- Malformed filter.MimeType returns an empty slice — TestStore_Capabilities/malformed_filter_mimeType_returns_empty passes
- `internal/registry/store_test.go` has `TestStore_Capabilities` with 13+ subtests: flatten, sort (name), sort (action/path tiebreakers), filter_action_PICK, filter_action_SAVE, case-sensitive action, filter_mime_exact (with canonicalization), filter_mime_type_wildcard, filter_mime_full_wildcard, capability-side wildcard (symmetric), OR semantics, malformed filter, combined filters, empty store
- `go test ./internal/registry -race -count=1` exits 0
- `go build ./...` succeeds
- Architectural gates: no transport/slog imports in production files; `go list -deps ./internal/registry | grep -E 'wshub|httpapi'` produces no output
  </done>
</task>

</tasks>

<verification>
Full Phase 2 exit gate. Every command MUST exit 0:

```bash
# Tests (all 40+ tests across the package)
go test ./internal/registry -race -count=1 -v
go test ./... -race -count=1

# Build + static checks
go build ./...
go vet ./internal/registry
test -z "$(gofmt -l internal/registry/)"

# Architectural gates
! go list -deps ./internal/registry 2>&1 | grep -E 'wshub|httpapi'
! grep -rE 'log/slog|slog\.Default' internal/registry/*.go | grep -v _test.go

# go mod untouched (no new deps)
go mod tidy
git diff --exit-code go.mod go.sum
```

The phase's 5 ROADMAP success criteria are all satisfied after this plan:
1. Store.Upsert/Delete/Get/List/Capabilities behave per contract under -race ✓ (Plans 02-02 + 02-03)
2. Symmetric 3x3 MIME matching with exhaustive table-driven test ✓ (Plan 02-01 + integration in 02-03)
3. Disk write failure → in-memory unchanged + error ✓ (Plan 02-02 PERS-03 test)
4. Restart against existing file yields same List output; missing file empty; corrupted fail-fast ✓ (Plan 02-02)
5. List and Capabilities return deterministic order ✓ (Plan 02-02 sort-by-id + Plan 02-03 4-key sort)
</verification>

<success_criteria>
- `Store.Capabilities` is implemented, tested, and closes CAP-01..04
- Sort order is (lower(appName), appId, action, path) via sort.SliceStable, proven by dedicated tiebreaker tests
- Filter by action is exact-match case-sensitive
- Filter by mimeType uses symmetric mimeMatch with OR semantics over the capability's mimes
- Malformed query MIME → empty slice, not error
- All 20 Phase 2 requirements implemented (REG-01..08, CAP-01..05, PERS-01..05, TEST-01, TEST-04)
- `go test ./... -race -count=1` green across the whole module
- No new go.mod dependencies; Phase 2 remains stdlib-only
- Architectural gates (no wshub/httpapi imports, no slog imports in production files) all green
</success_criteria>

<output>
After completion, create `.planning/phases/02-registry-core/02-03-capabilities-SUMMARY.md` documenting:
- Files modified (store.go, store_test.go)
- TestStore_Capabilities subtest count and what each exercises
- Final Phase 2 test count (TestMimeMatch cases + TestCanonicalizeMIME cases + TestManifestValidate_Errors cases + Store test count)
- Confirmation that all 20 Phase 2 requirement IDs are closed
- Handoff notes for Phase 4: import `internal/registry`, use `registry.NewStore`, mutate via `Upsert/Delete`, query via `Get/List/Capabilities`, pre-validate `?mimeType=` via `registry.CanonicalizeMIME`
</output>
</content>
</invoke>