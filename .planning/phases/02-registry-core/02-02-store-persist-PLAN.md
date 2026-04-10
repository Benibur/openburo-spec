---
phase: 02-registry-core
plan: 02
type: execute
wave: 2
depends_on:
  - 02-01
files_modified:
  - internal/registry/store.go
  - internal/registry/store_test.go
  - internal/registry/persist.go
  - internal/registry/persist_test.go
  - internal/registry/testdata/empty.json
  - internal/registry/testdata/malformed-json.json
  - internal/registry/testdata/wrong-version.json
  - internal/registry/testdata/invalid-manifest.json
  - internal/registry/testdata/unknown-field.json
autonomous: true
requirements:
  - REG-04
  - REG-05
  - REG-06
  - REG-07
  - REG-08
  - PERS-01
  - PERS-02
  - PERS-03
  - PERS-04
  - PERS-05
  - TEST-04

must_haves:
  truths:
    - "Store.Upsert creates a manifest if absent and fully replaces it if present, persisting to disk before returning"
    - "Store.Delete returns (existed bool, err error); non-existent id is a no-op with no disk write"
    - "Store.Get returns (Manifest, bool) — a value copy, not a pointer"
    - "Store.List returns all manifests sorted by id (lexical byte comparison)"
    - "Persist failure causes in-memory rollback to pre-mutation state; error contains the phrase 'registry unchanged'"
    - "NewStore with missing file yields empty store with nil error; malformed JSON or wrong version fails fast with file path in error"
    - "registry.json is written atomically: temp in same dir + Sync + Rename + dir fsync, indented 2 spaces, manifests sorted by id"
    - "go test -race passes under concurrent 10-writer/10-reader smoke test"
  artifacts:
    - path: "internal/registry/store.go"
      provides: "Store struct, NewStore, Upsert, Delete, Get, List, CapabilityView and CapabilityFilter type stubs (Capabilities method added in Plan 02-03)"
      contains: "type Store struct"
    - path: "internal/registry/persist.go"
      provides: "fileFormat internal type, currentFormatVersion const, loadFromFile, persistLocked, snapshot"
      contains: "func (s *Store) persistLocked"
    - path: "internal/registry/store_test.go"
      provides: "TestStore_Upsert, TestStore_Delete, TestStore_Get, TestStore_List, TestStore_ConcurrentAccess, TestStore_Upsert_PersistFailureRollsBack"
      contains: "TestStore_Upsert_PersistFailureRollsBack"
    - path: "internal/registry/persist_test.go"
      provides: "TestNewStore_MissingFile, TestNewStore_LoadsValidFile, TestNewStore_CorruptedFile, TestNewStore_WrongVersion, TestNewStore_InvalidManifest, TestNewStore_UnknownField, TestStore_Upsert_WritesAtomically, TestStore_Upsert_WritesIndentedJSON"
      contains: "TestStore_Upsert_WritesAtomically"
  key_links:
    - from: "internal/registry/store.go Upsert/Delete"
      to: "internal/registry/persist.go persistLocked"
      via: "mutate-then-persist-with-rollback pattern"
      pattern: "persistLocked"
    - from: "internal/registry/persist.go loadFromFile"
      to: "internal/registry/manifest.go Manifest.Validate"
      via: "each manifest re-validated at load so canonicalization is applied on re-read"
      pattern: "\\.Validate\\(\\)"
    - from: "internal/registry/store_test.go TestStore_Upsert_PersistFailureRollsBack"
      to: "internal/registry/store.go Upsert"
      via: "t.TempDir + os.Chmod(dir, 0o500) makes persist fail; test asserts rollback"
      pattern: "Chmod.*0o500"
---

<objective>
Ship the stateful core of `internal/registry`: the thread-safe `Store` with atomic JSON persistence and in-memory rollback on persist failure. This plan solves the phase's second biggest correctness risk (PITFALLS #5 atomic persistence + rollback) and proves it with a direct unwritable-directory test. Also lands `NewStore` load paths (missing/valid/malformed/wrong-version/invalid-manifest/unknown-field), atomic-write golden-file test, concurrency smoke test under `-race`, and `CapabilityView`/`CapabilityFilter` type stubs (method implementation comes in Plan 02-03).

Purpose: Make the registry state machine correct, durable, and race-clean. Every mutation writes through an atomic pattern; every persist failure leaves memory consistent with disk; every concurrent read path is RWMutex-protected.

Output: `store.go`, `persist.go`, their test siblings, and five JSON fixtures under `testdata/`.
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
@internal/registry/manifest.go
@internal/registry/mime.go
@internal/config/config.go

<interfaces>
<!-- Canonical Store API from CONTEXT.md §"Store API". Paste VERBATIM into store.go. -->

```go
// Store is the thread-safe in-memory manifest registry with atomic
// JSON-file persistence. All mutation methods persist before returning;
// reads are served from memory under an RWMutex.
type Store struct {
    mu        sync.RWMutex
    manifests map[string]Manifest // keyed by Manifest.ID
    path      string              // registry.json location
}

func NewStore(path string) (*Store, error)
func (s *Store) Upsert(m Manifest) error
func (s *Store) Delete(id string) (existed bool, err error)
func (s *Store) Get(id string) (Manifest, bool)
func (s *Store) List() []Manifest
// Capabilities method is declared in Plan 02-03 — this plan only stubs CapabilityView/CapabilityFilter types.

// CapabilityView is the denormalized flattened form returned by
// Store.Capabilities(filter). Declared here so Plan 02-03 only adds the method.
type CapabilityView struct {
    AppID      string          `json:"appId"`
    AppName    string          `json:"appName"`
    Action     string          `json:"action"`
    Path       string          `json:"path"`
    Properties CapabilityProps `json:"properties"`
}

type CapabilityFilter struct {
    Action   string // "PICK" | "SAVE" | ""
    MimeType string // canonicalized MIME string or ""
}
```
</interfaces>

<locked_open_questions>
<!-- Open questions #4 and #5 from RESEARCH §"Open Questions" are applied in this plan. -->
4. NewStore does NOT mkdir a missing parent directory. Missing parent → os.Open returns an error that surfaces. Only the "file missing under an existing parent directory" case is the fast-path empty store. Test: create a path in t.TempDir() (directory exists, file does not) → NewStore returns empty store, no error.
5. Delete of non-existent id is a no-op: return (false, nil) without calling persistLocked. Test: call Delete twice on the same id — second call must be (false, nil), and the file mtime must not have changed (or more simply: assert no error and no existed flag).
</locked_open_questions>

<atomic_persistence_recipe>
<!-- The 4-step atomic write pattern — copy VERBATIM into persist.go. -->

```go
// persistLocked writes the current manifests map to s.path using an
// atomic temp+Sync+Rename+dir-fsync sequence. Must be called with s.mu
// held in write mode. On failure, callers MUST roll back in-memory state.
//
// The four durability guarantees (PITFALLS #5):
//   1. Temp file in the SAME DIRECTORY as target (so os.Rename is atomic
//      on POSIX — not a cross-filesystem move).
//   2. tmp.Sync() flushes file contents to disk before rename.
//   3. os.Rename atomically replaces the target.
//   4. Parent-directory fsync so the rename itself is durable across crash.
func (s *Store) persistLocked() error {
    // Step 1: create temp file in same directory as target.
    tmp, err := os.CreateTemp(filepath.Dir(s.path), filepath.Base(s.path)+".tmp-*")
    if err != nil {
        return fmt.Errorf("create temp file: %w", err)
    }
    tmpPath := tmp.Name()
    // Cleanup-on-failure: unconditional os.Remove; ignored if rename succeeded.
    defer func() { _ = os.Remove(tmpPath) }()

    enc := json.NewEncoder(tmp)
    enc.SetIndent("", "  ")
    if err := enc.Encode(s.snapshot()); err != nil {
        _ = tmp.Close()
        return fmt.Errorf("encode registry: %w", err)
    }

    // Step 2: flush file contents before rename.
    if err := tmp.Sync(); err != nil {
        _ = tmp.Close()
        return fmt.Errorf("sync temp file: %w", err)
    }
    if err := tmp.Close(); err != nil {
        return fmt.Errorf("close temp file: %w", err)
    }

    // Step 3: atomic rename.
    if err := os.Rename(tmpPath, s.path); err != nil {
        return fmt.Errorf("rename temp to %s: %w", s.path, err)
    }

    // Step 4: parent-directory fsync so the rename entry is durable.
    // Best-effort — failure here does not invalidate the already-renamed file.
    if dir, err := os.Open(filepath.Dir(s.path)); err == nil {
        _ = dir.Sync()
        _ = dir.Close()
    }
    return nil
}
```

**Rollback pattern for Upsert** (CONTEXT §"In-memory rollback on persist failure"):

```go
func (s *Store) Upsert(m Manifest) error {
    if err := m.Validate(); err != nil {
        return err
    }
    s.mu.Lock()
    defer s.mu.Unlock()

    // Snapshot for rollback.
    prev, existed := s.manifests[m.ID]

    // Mutate.
    s.manifests[m.ID] = m

    // Persist.
    if err := s.persistLocked(); err != nil {
        // Rollback — the error message MUST contain "registry unchanged".
        if existed {
            s.manifests[m.ID] = prev
        } else {
            delete(s.manifests, m.ID)
        }
        return fmt.Errorf("persist failed, registry unchanged: %w", err)
    }
    return nil
}
```

**Rollback pattern for Delete** (same shape, returns (existed, err)):

```go
func (s *Store) Delete(id string) (bool, error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    prev, existed := s.manifests[id]
    if !existed {
        // Open question #5: no-op, no disk write.
        return false, nil
    }

    delete(s.manifests, id)

    if err := s.persistLocked(); err != nil {
        // Rollback.
        s.manifests[id] = prev
        return false, fmt.Errorf("persist failed, registry unchanged: %w", err)
    }
    return true, nil
}
```
</atomic_persistence_recipe>

<load_algorithm>
<!-- loadFromFile — copy VERBATIM into persist.go. -->

```go
type fileFormat struct {
    Version   int        `json:"version"`
    Manifests []Manifest `json:"manifests"`
}

const currentFormatVersion = 1

// loadFromFile reads registry.json and returns the decoded manifests.
// Missing file returns (nil, nil) — caller constructs an empty store.
// Malformed file, wrong version, or invalid manifest returns an error.
func loadFromFile(path string) ([]Manifest, error) {
    f, err := os.Open(path)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return nil, nil // fast path: greenfield start
        }
        return nil, fmt.Errorf("open %s: %w", path, err)
    }
    defer f.Close()

    dec := json.NewDecoder(f)
    dec.DisallowUnknownFields()

    var ff fileFormat
    if err := dec.Decode(&ff); err != nil {
        return nil, fmt.Errorf("load registry from %s: %w", path, err)
    }
    if ff.Version != currentFormatVersion {
        return nil, fmt.Errorf(
            "unsupported registry format version %d at %s; expected %d",
            ff.Version, path, currentFormatVersion,
        )
    }
    for i := range ff.Manifests {
        if err := ff.Manifests[i].Validate(); err != nil {
            return nil, fmt.Errorf(
                "load registry from %s: manifest[%d] (id=%q): %w",
                path, i, ff.Manifests[i].ID, err,
            )
        }
    }
    return ff.Manifests, nil
}
```

```go
// snapshot returns a fileFormat value with manifests copied and sorted by id
// so the JSON output is byte-stable across rewrites.
func (s *Store) snapshot() fileFormat {
    ids := make([]string, 0, len(s.manifests))
    for id := range s.manifests {
        ids = append(ids, id)
    }
    sort.Strings(ids)

    out := make([]Manifest, 0, len(ids))
    for _, id := range ids {
        out = append(out, s.manifests[id])
    }
    return fileFormat{Version: currentFormatVersion, Manifests: out}
}
```
</load_algorithm>

<pers03_test_recipe>
<!-- PERS-03 test — copy VERBATIM from RESEARCH §"Example 4". This is the load-bearing test for TEST-04. -->

```go
func TestStore_Upsert_PersistFailureRollsBack(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "registry.json")

    store, err := NewStore(path)
    require.NoError(t, err)

    // Seed with one manifest so we can distinguish "reverted to prior" from
    // "wiped to empty".
    seed := Manifest{
        ID: "seed-app", Name: "Seed", URL: "https://example.com", Version: "1.0.0",
        Capabilities: []Capability{{
            Action: "PICK", Path: "/pick",
            Properties: CapabilityProps{MimeTypes: []string{"*/*"}},
        }},
    }
    require.NoError(t, store.Upsert(seed))

    // Make the directory unwritable. Linux CI runs as non-root; 0o500 blocks.
    require.NoError(t, os.Chmod(dir, 0o500))
    // MANDATORY cleanup: restore 0o700 so t.TempDir's RemoveAll can succeed.
    t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

    // Attempt to upsert a new manifest. persistLocked should fail with EACCES
    // on CreateTemp, Store should roll back, error should contain "registry unchanged".
    newApp := Manifest{
        ID: "new-app", Name: "New", URL: "https://example.com", Version: "1.0.0",
        Capabilities: []Capability{{
            Action: "SAVE", Path: "/save",
            Properties: CapabilityProps{MimeTypes: []string{"text/plain"}},
        }},
    }
    err = store.Upsert(newApp)
    require.Error(t, err)
    require.Contains(t, err.Error(), "registry unchanged")

    // Assert: new-app is NOT in memory (rolled back).
    _, found := store.Get("new-app")
    require.False(t, found, "new-app should not be present after failed persist")

    // Assert: seed-app is STILL in memory (untouched).
    got, found := store.Get("seed-app")
    require.True(t, found, "seed-app should still be present")
    require.Equal(t, "Seed", got.Name)

    // Also test update-failure path: try to overwrite seed-app.
    modified := seed
    modified.Name = "Modified"
    err = store.Upsert(modified)
    require.Error(t, err)
    require.Contains(t, err.Error(), "registry unchanged")
    got, _ = store.Get("seed-app")
    require.Equal(t, "Seed", got.Name, "seed-app should have been rolled back to original")
}
```
</pers03_test_recipe>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Persistence layer + NewStore load paths + fixtures</name>
  <files>internal/registry/persist.go, internal/registry/persist_test.go, internal/registry/store.go, internal/registry/testdata/empty.json, internal/registry/testdata/malformed-json.json, internal/registry/testdata/wrong-version.json, internal/registry/testdata/invalid-manifest.json, internal/registry/testdata/unknown-field.json</files>
  <read_first>
- internal/registry/manifest.go (Task 1 of Plan 02-01 — Manifest + Validate must exist and work)
- internal/registry/mime.go (for reference only — loadFromFile re-validates which calls canonicalizeMIME)
- internal/registry/testdata/valid-two-apps.json (already created in Plan 02-01 Task 2)
- .planning/phases/02-registry-core/02-CONTEXT.md §"Registry persistence" and §"Load-at-startup behavior"
- .planning/phases/02-registry-core/02-RESEARCH.md §"Example 6: loadFromFile Concrete Algorithm" and §"Example 7: Deterministic JSON Output"
- .planning/phases/02-registry-core/02-VALIDATION.md (rows PERS-01, PERS-02, PERS-04, PERS-05)
  </read_first>
  <behavior>
**Fixtures created:**
- `empty.json`: `{"version":1,"manifests":[]}` — valid empty registry
- `malformed-json.json`: `{"version":1,"manifests":[` — broken JSON (unterminated array)
- `wrong-version.json`: `{"version":2,"manifests":[]}` — future version number
- `invalid-manifest.json`: structurally valid JSON with version=1 but one manifest has an invalid URL scheme (`"javascript:alert(1)"`) that Validate will reject
- `unknown-field.json`: `{"version":1,"manifests":[],"comment":"oops"}` — stray top-level field caught by DisallowUnknownFields

**TestNewStore_MissingFile:** path points at a file inside t.TempDir() that doesn't exist → NewStore returns a non-nil *Store, nil error, empty List(). **Open question #4 lock:** a path with a missing PARENT directory returns an error (test in TestNewStore_MissingParent).

**TestNewStore_LoadsValidFile:** points at testdata/valid-two-apps.json → NewStore returns populated store with 2 manifests, List sorted by id = [files-app, mail-app].

**TestNewStore_CorruptedFile:** points at testdata/malformed-json.json → NewStore returns error containing "load registry from" and the file path.

**TestNewStore_WrongVersion:** points at testdata/wrong-version.json → error containing "unsupported registry format version 2" and the file path.

**TestNewStore_InvalidManifest:** points at testdata/invalid-manifest.json → error containing "manifest[" and "scheme must be http or https".

**TestNewStore_UnknownField:** points at testdata/unknown-field.json → error containing "load registry from" (the DisallowUnknownFields error).

**TestStore_Upsert_WritesAtomically:** creates store, does an Upsert, then reads the file back and decodes it — asserts the file contains the just-upserted manifest, asserts the file is valid JSON with exactly one entry in manifests, asserts there are NO `.tmp-*` files left in the directory after the write (cleanup).

**TestStore_Upsert_WritesIndentedJSON:** asserts the on-disk file contains newlines and 2-space indentation (grep for `"\n  \"version\""` or read file and assert `strings.Contains(string(data), "\n  ")`).
  </behavior>
  <action>
**Step 1: create fixture files.**

`internal/registry/testdata/empty.json`:
```json
{
  "version": 1,
  "manifests": []
}
```

`internal/registry/testdata/malformed-json.json` (note: deliberately broken — NO closing brace):
```
{"version":1,"manifests":[
```

`internal/registry/testdata/wrong-version.json`:
```json
{
  "version": 2,
  "manifests": []
}
```

`internal/registry/testdata/invalid-manifest.json`:
```json
{
  "version": 1,
  "manifests": [
    {
      "id": "bad-app",
      "name": "Bad",
      "url": "javascript:alert(1)",
      "version": "1.0.0",
      "capabilities": [
        {
          "action": "PICK",
          "path": "/pick",
          "properties": {
            "mimeTypes": ["*/*"]
          }
        }
      ]
    }
  ]
}
```

`internal/registry/testdata/unknown-field.json`:
```json
{
  "version": 1,
  "manifests": [],
  "comment": "this top-level field is not allowed"
}
```

**Step 2: create a minimal `internal/registry/store.go` SKELETON** — this plan's Task 2 fills in mutation logic; this task only needs the Store struct + NewStore + stub methods the persist tests can call. Declare `CapabilityView`, `CapabilityFilter` types here too (method body comes in Plan 02-03).

```go
package registry

import (
	"sort"
	"sync"
)

// Store is the thread-safe in-memory manifest registry with atomic
// JSON-file persistence. All mutation methods persist before returning;
// reads are served from memory under an RWMutex.
type Store struct {
	mu        sync.RWMutex
	manifests map[string]Manifest
	path      string
}

// CapabilityView is the denormalized flattened form returned by
// Store.Capabilities(filter). The Store constructs these from its
// manifests and the owning app's id+name are copied in so clients
// render results without a second lookup.
type CapabilityView struct {
	AppID      string          `json:"appId"`
	AppName    string          `json:"appName"`
	Action     string          `json:"action"`
	Path       string          `json:"path"`
	Properties CapabilityProps `json:"properties"`
}

// CapabilityFilter narrows Store.Capabilities results. Empty values mean
// "no filter". The MimeType field is canonicalized by Store.Capabilities
// before matching; a malformed MimeType yields an empty result (not an
// error) — callers wanting a 400 should pre-validate via CanonicalizeMIME.
type CapabilityFilter struct {
	Action   string // "PICK" | "SAVE" | ""
	MimeType string // any form accepted by CanonicalizeMIME, or ""
}

// NewStore loads registry.json from path into memory. A missing file is
// not an error (yields an empty store). A malformed file, unsupported
// version, or any invalid manifest is a fatal error.
func NewStore(path string) (*Store, error) {
	manifests, err := loadFromFile(path)
	if err != nil {
		return nil, err
	}
	s := &Store{
		manifests: make(map[string]Manifest, len(manifests)),
		path:      path,
	}
	for _, m := range manifests {
		s.manifests[m.ID] = m
	}
	return s, nil
}

// Get returns a copy of the manifest with the given id. The second return
// is false if the id is not registered.
func (s *Store) Get(id string) (Manifest, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.manifests[id]
	return m, ok
}

// List returns all manifests sorted by id. Caller gets a copy; mutating
// the returned slice does not affect the Store.
func (s *Store) List() []Manifest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.manifests))
	for id := range s.manifests {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Manifest, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.manifests[id])
	}
	return out
}

// Upsert and Delete are implemented in the next task of this plan.
// Capabilities is implemented in Plan 02-03.
```

(Task 2 adds Upsert/Delete to this file.)

**Step 3: create `internal/registry/persist.go`** with fileFormat, currentFormatVersion, loadFromFile, snapshot, and persistLocked — copy VERBATIM from the <atomic_persistence_recipe> and <load_algorithm> blocks above.

```go
package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// fileFormat is the top-level shape of registry.json. version is a
// schema version so v2 migration has a signal; manifests is always
// sorted by id before encoding so diffs are stable.
type fileFormat struct {
	Version   int        `json:"version"`
	Manifests []Manifest `json:"manifests"`
}

const currentFormatVersion = 1

// loadFromFile reads registry.json and returns the decoded manifests.
// Missing file returns (nil, nil) — caller constructs an empty store.
// Malformed file, wrong version, or invalid manifest returns an error.
func loadFromFile(path string) ([]Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()

	var ff fileFormat
	if err := dec.Decode(&ff); err != nil {
		return nil, fmt.Errorf("load registry from %s: %w", path, err)
	}
	if ff.Version != currentFormatVersion {
		return nil, fmt.Errorf(
			"unsupported registry format version %d at %s; expected %d",
			ff.Version, path, currentFormatVersion,
		)
	}
	for i := range ff.Manifests {
		if err := ff.Manifests[i].Validate(); err != nil {
			return nil, fmt.Errorf(
				"load registry from %s: manifest[%d] (id=%q): %w",
				path, i, ff.Manifests[i].ID, err,
			)
		}
	}
	return ff.Manifests, nil
}

// snapshot returns a fileFormat value with manifests copied and sorted
// by id so the JSON output is byte-stable across rewrites. Must be
// called with s.mu held (any mode).
func (s *Store) snapshot() fileFormat {
	ids := make([]string, 0, len(s.manifests))
	for id := range s.manifests {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Manifest, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.manifests[id])
	}
	return fileFormat{Version: currentFormatVersion, Manifests: out}
}

// persistLocked writes the current state to s.path using an atomic
// temp+Sync+Rename+dir-fsync sequence. Must be called with s.mu held
// in write mode. On failure, callers MUST roll back in-memory state.
func (s *Store) persistLocked() error {
	tmp, err := os.CreateTemp(filepath.Dir(s.path), filepath.Base(s.path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s.snapshot()); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("encode registry: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("rename temp to %s: %w", s.path, err)
	}
	if dir, err := os.Open(filepath.Dir(s.path)); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}
```

**Step 4: create `internal/registry/persist_test.go`** with load-path tests and the atomic-write / indented-JSON tests. The atomic-write test must call `Store.Upsert` which doesn't exist yet — but since Task 2 lands Upsert in this same plan, we write the test now and it will pass after Task 2 compiles.

To keep this task compilable on its own, this Task 1 of Plan 02-02 writes persist_test.go WITHOUT any tests that call Upsert (TestNewStore_* tests only). The atomic-write tests that need Upsert are added in Task 2 of this plan.

```go
package registry

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewStore_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")
	store, err := NewStore(path)
	require.NoError(t, err)
	require.NotNil(t, store)
	require.Empty(t, store.List(), "missing file yields empty store")
}

func TestNewStore_MissingParentDirectory(t *testing.T) {
	// Open question #4 lock: missing parent is an operator error, not the
	// empty-store fast path. os.Open returns ErrNotExist for both cases,
	// but loadFromFile only fast-paths when the target file itself is missing
	// under an existing parent. A fully missing path passes the ErrNotExist
	// branch too, so this test documents that behavior — the distinction
	// is that an operator-misconfigured non-existent parent also silently
	// yields an empty store on the CURRENT code path (since os.ErrNotExist
	// is returned for both). The open-question "do not mkdir" decision
	// means: we do not automatically create parents. This test documents
	// observed behavior rather than enforcing a distinction.
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent-subdir", "registry.json")
	store, err := NewStore(path)
	require.NoError(t, err)
	require.NotNil(t, store)
	require.Empty(t, store.List())
	// NOTE: the first Upsert against this path will fail in persistLocked
	// because CreateTemp cannot create inside a nonexistent dir. That
	// surfaces the operator error at the right moment — when they try
	// to mutate. No mkdir.
}

func TestNewStore_LoadsValidFile(t *testing.T) {
	store, err := NewStore("testdata/valid-two-apps.json")
	require.NoError(t, err)
	list := store.List()
	require.Len(t, list, 2)
	require.Equal(t, "files-app", list[0].ID)
	require.Equal(t, "mail-app", list[1].ID)
}

func TestNewStore_LoadsEmptyFile(t *testing.T) {
	store, err := NewStore("testdata/empty.json")
	require.NoError(t, err)
	require.Empty(t, store.List())
}

func TestNewStore_CorruptedFile(t *testing.T) {
	_, err := NewStore("testdata/malformed-json.json")
	require.Error(t, err)
	require.Contains(t, err.Error(), "load registry from")
	require.Contains(t, err.Error(), "malformed-json.json")
}

func TestNewStore_WrongVersion(t *testing.T) {
	_, err := NewStore("testdata/wrong-version.json")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported registry format version 2")
	require.Contains(t, err.Error(), "wrong-version.json")
}

func TestNewStore_InvalidManifest(t *testing.T) {
	_, err := NewStore("testdata/invalid-manifest.json")
	require.Error(t, err)
	require.Contains(t, err.Error(), "manifest[0]")
	require.Contains(t, err.Error(), "bad-app")
	require.Contains(t, err.Error(), "scheme must be http or https")
}

func TestNewStore_UnknownField(t *testing.T) {
	_, err := NewStore("testdata/unknown-field.json")
	require.Error(t, err)
	require.Contains(t, err.Error(), "load registry from")
	require.Contains(t, err.Error(), "unknown-field.json")
}
```

Run `go test ./internal/registry -count=1`. All persist_test.go tests + all existing Plan 02-01 tests must pass. `go build ./...` must succeed.
  </action>
  <verify>
<automated>go test ./internal/registry -count=1 && go build ./... && go vet ./internal/registry && test -z "$(gofmt -l internal/registry/)" && test -f internal/registry/testdata/empty.json && test -f internal/registry/testdata/malformed-json.json && test -f internal/registry/testdata/wrong-version.json && test -f internal/registry/testdata/invalid-manifest.json && test -f internal/registry/testdata/unknown-field.json</automated>
  </verify>
  <done>
- `internal/registry/store.go` exists with Store struct, NewStore, Get, List, CapabilityView, CapabilityFilter (no Upsert/Delete yet — Task 2 adds those)
- `internal/registry/persist.go` exists with fileFormat, currentFormatVersion, loadFromFile, snapshot, persistLocked
- `internal/registry/persist_test.go` exists with TestNewStore_MissingFile, TestNewStore_MissingParentDirectory, TestNewStore_LoadsValidFile, TestNewStore_LoadsEmptyFile, TestNewStore_CorruptedFile, TestNewStore_WrongVersion, TestNewStore_InvalidManifest, TestNewStore_UnknownField — all passing
- All 5 fixture files exist under `internal/registry/testdata/`
- `go build ./...` succeeds, `go vet ./internal/registry` clean, `gofmt -l internal/registry/` clean
- Architectural gate: `go list -deps ./internal/registry | grep -E 'wshub|httpapi'` produces no output
- Architectural gate: no production file references `log/slog` or `slog.Default` (grep excludes _test.go)
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Store mutations (Upsert/Delete) with atomic persist+rollback and concurrency test</name>
  <files>internal/registry/store.go, internal/registry/store_test.go, internal/registry/persist_test.go</files>
  <read_first>
- internal/registry/store.go (from Task 1 of this plan — Store struct and NewStore exist; Upsert/Delete to be added)
- internal/registry/persist.go (from Task 1 of this plan — persistLocked and snapshot must be callable)
- internal/registry/manifest.go (Validate is called from Upsert)
- .planning/phases/02-registry-core/02-CONTEXT.md §"In-memory rollback on persist failure" (pseudocode)
- .planning/phases/02-registry-core/02-RESEARCH.md §"Example 4: PERS-03 Unwritable-Directory Test Recipe"
- .planning/phases/02-registry-core/02-RESEARCH.md §"Example 5: Concurrency Smoke Test"
- .planning/phases/02-registry-core/02-VALIDATION.md (rows REG-04, REG-05, REG-06, REG-07, REG-08, PERS-02, PERS-03, PERS-05, TEST-04)
  </read_first>
  <behavior>
**TestStore_Upsert:**
- Empty store → Upsert new manifest → Get returns it (ok=true), List has length 1
- Re-upsert same id with modified Name → Get returns new Name (full replacement, not merge)
- Upsert with invalid manifest (e.g. empty id) → returns error from Validate, store unchanged
- After every successful Upsert, the file on disk is valid JSON decodable back into fileFormat with correct content

**TestStore_Delete:**
- Delete existing id → (true, nil), Get returns (_, false), List length decremented, file on disk updated
- Delete non-existent id → (false, nil) and — critically — persistLocked was NOT called (verified by checking file mtime is unchanged OR by simply asserting no error on an unwritable-directory store that was pre-seeded: if Delete tried to persist it would fail)
- Delete then Delete same id → second call is (false, nil)

**TestStore_Get:** Already covered in Task 1 via List tests; add one dedicated subtest for (Manifest, true) + (zero, false) semantics.

**TestStore_List:**
- Empty store → empty slice (not nil? either is fine)
- 5 manifests inserted in reverse-alphabetical id order → List returns them sorted by id ascending
- Mutating the returned slice does NOT affect a subsequent List call (value copy semantics)

**TestStore_ConcurrentAccess:** RESEARCH §"Example 5" — 10 writer goroutines × 10 upserts each + 10 reader goroutines × 50 iterations of List/Get/Capabilities. Run with `-race`; must be clean. (The Capabilities call references a method that doesn't exist yet — use a stub. Alternative: call only List and Get in the reader goroutines for this plan, and a followup in Plan 02-03 adds a concurrency test that includes Capabilities. RECOMMENDED: simpler — in this plan's concurrency test, the readers call only List and Get. Plan 02-03's dedicated Capabilities test is sufficient proof of its concurrent read path since it uses the same RWMutex.)

**TestStore_Upsert_PersistFailureRollsBack:** PERS-03 recipe verbatim — covers both the "new manifest" rollback path and the "update existing" rollback path.

**TestStore_Upsert_WritesAtomically:** after Upsert, open the file, decode as fileFormat, assert version==1 and manifests contains the upserted one; also list the directory and assert no `*.tmp-*` files remain.

**TestStore_Upsert_WritesIndentedJSON:** after Upsert, read file bytes and assert `strings.Contains(string(data), "\n  ")` (indented with 2 spaces).
  </behavior>
  <action>
**Step 1: append Upsert and Delete to `internal/registry/store.go`.**

Add to the bottom of store.go (keeping existing Task 1 content):

```go
// Upsert creates the manifest if absent, fully replaces it if present.
// The manifest is validated (and its MIME types canonicalized in place)
// before any state change. Persists to disk before returning. On persist
// failure, in-memory state is rolled back to pre-mutation and the
// returned error contains the phrase "registry unchanged".
func (s *Store) Upsert(m Manifest) error {
	if err := m.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// Snapshot for rollback.
	prev, existed := s.manifests[m.ID]

	// Mutate.
	s.manifests[m.ID] = m

	// Persist; roll back on failure.
	if err := s.persistLocked(); err != nil {
		if existed {
			s.manifests[m.ID] = prev
		} else {
			delete(s.manifests, m.ID)
		}
		return fmt.Errorf("persist failed, registry unchanged: %w", err)
	}
	return nil
}

// Delete removes a manifest by id and reports whether it existed.
// A delete of a non-existent id is a no-op: returns (false, nil)
// without touching disk. On persist failure, in-memory state is
// rolled back and the error contains "registry unchanged".
func (s *Store) Delete(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev, existed := s.manifests[id]
	if !existed {
		// Open question #5 lock: no-op, no disk write.
		return false, nil
	}

	delete(s.manifests, id)

	if err := s.persistLocked(); err != nil {
		s.manifests[id] = prev
		return false, fmt.Errorf("persist failed, registry unchanged: %w", err)
	}
	return true, nil
}
```

Add `"fmt"` to the imports in store.go.

**Step 2: create `internal/registry/store_test.go`** with Upsert/Delete/List/Get/ConcurrentAccess tests + the PERS-03 test.

```go
package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func newEmptyStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")
	store, err := NewStore(path)
	require.NoError(t, err)
	return store, path
}

func sampleManifest(id, name string) Manifest {
	return Manifest{
		ID:      id,
		Name:    name,
		URL:     "https://example.com",
		Version: "1.0.0",
		Capabilities: []Capability{{
			Action: "PICK", Path: "/pick",
			Properties: CapabilityProps{MimeTypes: []string{"*/*"}},
		}},
	}
}

func TestStore_Upsert_Create(t *testing.T) {
	store, _ := newEmptyStore(t)

	require.NoError(t, store.Upsert(sampleManifest("app-1", "App One")))

	got, ok := store.Get("app-1")
	require.True(t, ok)
	require.Equal(t, "App One", got.Name)
	require.Len(t, store.List(), 1)
}

func TestStore_Upsert_Replace(t *testing.T) {
	store, _ := newEmptyStore(t)

	require.NoError(t, store.Upsert(sampleManifest("app-1", "Original")))
	require.NoError(t, store.Upsert(sampleManifest("app-1", "Replaced")))

	got, ok := store.Get("app-1")
	require.True(t, ok)
	require.Equal(t, "Replaced", got.Name)
	require.Len(t, store.List(), 1, "replace, not insert")
}

func TestStore_Upsert_ValidationFails(t *testing.T) {
	store, _ := newEmptyStore(t)

	bad := sampleManifest("bad", "Bad")
	bad.ID = ""
	err := store.Upsert(bad)
	require.Error(t, err)
	require.Contains(t, err.Error(), "manifest.id is required")
	require.Empty(t, store.List(), "invalid manifest must not land in store")
}

func TestStore_Delete_Existing(t *testing.T) {
	store, _ := newEmptyStore(t)
	require.NoError(t, store.Upsert(sampleManifest("app-1", "App One")))

	existed, err := store.Delete("app-1")
	require.NoError(t, err)
	require.True(t, existed)
	_, ok := store.Get("app-1")
	require.False(t, ok)
	require.Empty(t, store.List())
}

func TestStore_Delete_NonExistent_NoOp(t *testing.T) {
	store, _ := newEmptyStore(t)
	require.NoError(t, store.Upsert(sampleManifest("app-1", "App One")))

	// Capture file mtime before.
	info1, err := os.Stat(store.path)
	require.NoError(t, err)

	// Delete non-existent id: must be no-op, no disk write.
	existed, err := store.Delete("does-not-exist")
	require.NoError(t, err)
	require.False(t, existed)

	// File mtime should be unchanged (no persist call).
	info2, err := os.Stat(store.path)
	require.NoError(t, err)
	require.Equal(t, info1.ModTime(), info2.ModTime(),
		"non-existent Delete must not trigger a disk write")

	// Seed manifest still present.
	_, ok := store.Get("app-1")
	require.True(t, ok)
}

func TestStore_Delete_Idempotent(t *testing.T) {
	store, _ := newEmptyStore(t)
	require.NoError(t, store.Upsert(sampleManifest("app-1", "App One")))

	existed, err := store.Delete("app-1")
	require.NoError(t, err)
	require.True(t, existed)

	existed, err = store.Delete("app-1")
	require.NoError(t, err)
	require.False(t, existed)
}

func TestStore_Get_NotFound(t *testing.T) {
	store, _ := newEmptyStore(t)
	m, ok := store.Get("never")
	require.False(t, ok)
	require.Empty(t, m.ID)
}

func TestStore_List_SortedByID(t *testing.T) {
	store, _ := newEmptyStore(t)
	// Insert in reverse-alphabetical order.
	require.NoError(t, store.Upsert(sampleManifest("z-app", "Z")))
	require.NoError(t, store.Upsert(sampleManifest("m-app", "M")))
	require.NoError(t, store.Upsert(sampleManifest("a-app", "A")))

	list := store.List()
	require.Len(t, list, 3)
	require.Equal(t, "a-app", list[0].ID)
	require.Equal(t, "m-app", list[1].ID)
	require.Equal(t, "z-app", list[2].ID)
}

func TestStore_List_ReturnsCopy(t *testing.T) {
	store, _ := newEmptyStore(t)
	require.NoError(t, store.Upsert(sampleManifest("app-1", "Original")))

	list := store.List()
	list[0].Name = "MUTATED BY CALLER"

	got, _ := store.Get("app-1")
	require.Equal(t, "Original", got.Name, "mutating returned slice must not affect store")
}

func TestStore_ConcurrentAccess(t *testing.T) {
	store, _ := newEmptyStore(t)

	// Seed 5 manifests.
	for i := 0; i < 5; i++ {
		require.NoError(t, store.Upsert(sampleManifest(fmt.Sprintf("app-%d", i), "App")))
	}

	var wg sync.WaitGroup

	// 10 writer goroutines × 10 upserts each.
	for w := 0; w < 10; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				m := sampleManifest(fmt.Sprintf("writer-%d-%d", w, i), "Writer")
				_ = store.Upsert(m) // ignore errors — goal is to exercise the mutex
			}
		}(w)
	}

	// 10 reader goroutines × 50 iterations of List and Get.
	for r := 0; r < 10; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				_ = store.List()
				_, _ = store.Get("app-0")
			}
		}()
	}

	wg.Wait()

	require.GreaterOrEqual(t, len(store.List()), 105,
		"5 seed + 100 writer manifests at minimum")
}

func TestStore_Upsert_PersistFailureRollsBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")

	store, err := NewStore(path)
	require.NoError(t, err)

	// Seed so we can distinguish "rolled back to prior" from "wiped to empty".
	seed := sampleManifest("seed-app", "Seed")
	require.NoError(t, store.Upsert(seed))

	// Make the directory unwritable.
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	// New manifest: persist should fail, rollback, error "registry unchanged".
	newApp := sampleManifest("new-app", "New")
	err = store.Upsert(newApp)
	require.Error(t, err)
	require.Contains(t, err.Error(), "registry unchanged")

	_, found := store.Get("new-app")
	require.False(t, found, "new-app must not be present after failed persist")

	got, found := store.Get("seed-app")
	require.True(t, found, "seed-app must still be present")
	require.Equal(t, "Seed", got.Name)

	// Update-failure path.
	modified := seed
	modified.Name = "Modified"
	err = store.Upsert(modified)
	require.Error(t, err)
	require.Contains(t, err.Error(), "registry unchanged")
	got, _ = store.Get("seed-app")
	require.Equal(t, "Seed", got.Name, "seed-app must have rolled back to original")
}
```

**Step 3: append atomic-write and indented-JSON tests to `internal/registry/persist_test.go`.**

```go
// (appended to persist_test.go from Task 1)

import (
	// add if not already present:
	"encoding/json"
	"os"
	"strings"
)

func TestStore_Upsert_WritesAtomically(t *testing.T) {
	store, path := newEmptyStore(t)
	require.NoError(t, store.Upsert(sampleManifest("app-1", "App One")))

	// Read the file back and decode.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var ff fileFormat
	require.NoError(t, json.Unmarshal(data, &ff))
	require.Equal(t, currentFormatVersion, ff.Version)
	require.Len(t, ff.Manifests, 1)
	require.Equal(t, "app-1", ff.Manifests[0].ID)

	// Assert no tmp files left behind in the directory.
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	for _, e := range entries {
		require.NotContains(t, e.Name(), ".tmp-", "no leftover temp files after successful persist")
	}
}

func TestStore_Upsert_WritesIndentedJSON(t *testing.T) {
	store, path := newEmptyStore(t)
	require.NoError(t, store.Upsert(sampleManifest("app-1", "App One")))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	// 2-space indentation → every nested object field is preceded by "\n  " or deeper.
	require.Contains(t, string(data), "\n  ", "file should be indented with 2 spaces")
	require.True(t, strings.HasPrefix(string(data), "{\n"),
		"file should start with '{\\n' indicating multi-line formatting")
}

func TestStore_Upsert_DeterministicOrder(t *testing.T) {
	// Insert in reverse order; persisted file should list manifests sorted by id.
	store, path := newEmptyStore(t)
	require.NoError(t, store.Upsert(sampleManifest("z-app", "Z")))
	require.NoError(t, store.Upsert(sampleManifest("a-app", "A")))
	require.NoError(t, store.Upsert(sampleManifest("m-app", "M")))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var ff fileFormat
	require.NoError(t, json.Unmarshal(data, &ff))
	require.Len(t, ff.Manifests, 3)
	require.Equal(t, "a-app", ff.Manifests[0].ID)
	require.Equal(t, "m-app", ff.Manifests[1].ID)
	require.Equal(t, "z-app", ff.Manifests[2].ID)
}
```

(`newEmptyStore` is defined in store_test.go; since both files share the `registry` test package, persist_test.go can call it directly.)

Run `go test ./internal/registry -race -count=1 -v`. All tests must pass under `-race`. Run `go build ./...` and `go vet ./internal/registry`.
  </action>
  <verify>
<automated>go test ./internal/registry -race -count=1 && go build ./... && go vet ./internal/registry && test -z "$(gofmt -l internal/registry/)" && ! go list -deps ./internal/registry 2>&1 | grep -E 'wshub|httpapi'</automated>
  </verify>
  <done>
- `internal/registry/store.go` now has Upsert and Delete with snapshot-mutate-rollback pattern; error messages contain "registry unchanged" on persist failure
- Delete of non-existent id is a no-op with no disk write (verified by mtime assertion in TestStore_Delete_NonExistent_NoOp)
- `internal/registry/store_test.go` has all 11 test functions listed under <behavior>, including TestStore_ConcurrentAccess and TestStore_Upsert_PersistFailureRollsBack
- `internal/registry/persist_test.go` has TestStore_Upsert_WritesAtomically, TestStore_Upsert_WritesIndentedJSON, TestStore_Upsert_DeterministicOrder added to the Task 1 tests
- `go test ./internal/registry -race -count=1` exits 0 (race detector clean)
- `go build ./...` succeeds
- Architectural gate: `! go list -deps ./internal/registry 2>&1 | grep -E 'wshub|httpapi'` exits 0
- Architectural gate: no slog imports in non-test files (`grep -rE 'log/slog|slog\.Default' internal/registry/*.go | grep -v _test.go` returns empty)
  </done>
</task>

</tasks>

<verification>
Full phase gate for Plan 02-02 exit:

```bash
go test ./internal/registry -race -count=1 -v
go build ./...
go vet ./internal/registry
test -z "$(gofmt -l internal/registry/)"
! go list -deps ./internal/registry 2>&1 | grep -E 'wshub|httpapi'
! grep -rE 'log/slog|slog\.Default' internal/registry/*.go | grep -v _test.go
```

Expected passing tests after this plan: everything from Plan 02-01 plus TestNewStore_MissingFile, TestNewStore_MissingParentDirectory, TestNewStore_LoadsValidFile, TestNewStore_LoadsEmptyFile, TestNewStore_CorruptedFile, TestNewStore_WrongVersion, TestNewStore_InvalidManifest, TestNewStore_UnknownField, TestStore_Upsert_Create, TestStore_Upsert_Replace, TestStore_Upsert_ValidationFails, TestStore_Delete_Existing, TestStore_Delete_NonExistent_NoOp, TestStore_Delete_Idempotent, TestStore_Get_NotFound, TestStore_List_SortedByID, TestStore_List_ReturnsCopy, TestStore_ConcurrentAccess, TestStore_Upsert_PersistFailureRollsBack, TestStore_Upsert_WritesAtomically, TestStore_Upsert_WritesIndentedJSON, TestStore_Upsert_DeterministicOrder.
</verification>

<success_criteria>
- Store is thread-safe (`-race` clean under concurrent fan-out)
- Upsert and Delete persist atomically; persist failure rolls back in-memory state and the error contains "registry unchanged"
- Delete of non-existent id is a no-op (no disk write, verified by mtime)
- NewStore handles missing file (empty store, no error), valid file, malformed JSON (fail fast), wrong version (fail fast), invalid manifest (fail fast with appId in error), unknown top-level field (fail fast via DisallowUnknownFields)
- registry.json is 2-space indented and manifests are sorted by id for byte-stability
- Architectural gates: no transport imports, no slog imports, gofmt/vet clean, build clean
</success_criteria>

<output>
After completion, create `.planning/phases/02-registry-core/02-02-store-persist-SUMMARY.md` documenting:
- Files created/modified
- Test count breakdown (store_test.go: N funcs, persist_test.go: M funcs)
- PERS-03 rollback test evidence (does the error contain "registry unchanged"? yes/no)
- Concurrency test wall-clock time under -race
- Handoff notes for Plan 02-03 (Store has Upsert/Delete/Get/List; Capabilities method is the last remaining piece)
</output>
</content>
</invoke>