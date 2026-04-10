# Phase 2: Registry Core - Research

**Researched:** 2026-04-09
**Domain:** Pure-Go domain package (manifest validation, thread-safe in-memory state, atomic JSON persistence, symmetric MIME wildcard matching)
**Confidence:** HIGH

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Capability response shape (USER DECISION):** `GET /api/v1/capabilities` entries are denormalized — every entry is exactly `{ appId, appName, action, path, properties: { mimeTypes: [...] } }`. No extra fields in v1 (no `match_score`, no `manifest_version`, no `url`). A dedicated Go type (`CapabilityView`) is distinct from `Manifest.Capabilities[i]` because it carries the denormalized `appId`/`appName`.

**Capability sort order (USER DECISION):** Primary sort key is `appName`. Tiebreakers (locked by Claude): `appName` lowercased → `appId` → `action` → `path`, all lexical/byte comparison. Implementation: `sort.SliceStable`. REQUIREMENTS.md CAP-02 says "sorted by (appId, action, mimeType[0])" — **this is stale** and must be updated to "sorted by (appName, appId, action, path) lowercased" in the same commit that lands `Store.Capabilities`.

**Manifest domain types (locked):** `Manifest{ID, Name, URL, Version, Capabilities[]}`, `Capability{Action, Path, Properties}`, `CapabilityProps{MimeTypes[]}`, `CapabilityView{AppID, AppName, Action, Path, Properties}`, `CapabilityFilter{Action, MimeType}`. All value semantics, no pointers.

**Store API (locked):**
- `NewStore(path string) (*Store, error)` — missing file → empty store, malformed → error
- `Upsert(m Manifest) error` — full replace, persist before return, rollback on failure
- `Delete(id string) (existed bool, err error)`
- `Get(id string) (Manifest, bool)` — returns a copy
- `List() []Manifest` — sorted by id, returns copies
- `Capabilities(filter CapabilityFilter) []CapabilityView`

**Validation rules (locked):**
- `id`: required, ASCII, `^[a-zA-Z0-9][a-zA-Z0-9._-]*$`, max 128 chars
- `name`: required, non-empty after TrimSpace, max 200 chars, any Unicode
- `url`: required, `url.Parse` succeeds, Scheme in {http, https}, non-empty Host
- `version`: required, non-empty, max 64 chars, any printable ASCII
- `capabilities`: required, non-empty slice
- `capabilities[i].action`: required, exactly `"PICK"` or `"SAVE"` (case-sensitive)
- `capabilities[i].path`: required, starts with `/`, max 500 chars
- `capabilities[i].properties.mimeTypes`: required, non-empty, each canonicalized `type/subtype` with `*` allowed on either side. `*/subtype` is invalid. Parameters stripped. Case-folded to lowercase.

**MIME matching (locked):** Symmetric 3×3 wildcard matching — `exact`, `type/*`, `*/*` on both sides. Matching is called on canonicalized inputs only. Canonicalization is applied at `Validate()` time (in place) and at query-parse time.

**Atomic persistence (locked):** 4 guarantees — (1) temp file in same directory as target, (2) `tmp.Sync()` before rename, (3) `os.Rename` atomic replace, (4) parent directory fsync. Mutex held through mutate → persist → rollback-on-failure. On persist failure, in-memory state rolls back AND the error message contains the phrase `"registry unchanged"`.

**File format (locked):** JSON, 2-space indented, top-level `{ "version": 1, "manifests": [...] }`. Manifests array sorted by id before encoding. `version: 1` literal; mismatch is a fail-fast error at load time.

**Load-at-startup (locked):** Missing file → empty store, no error. Malformed JSON → fail fast with file path + offset. Wrong `version` → fail fast. Any manifest failing `Validate()` at load → fail fast with offending appId.

**No transport dependencies (locked):** `internal/registry` imports NOTHING from `internal/wshub` or `internal/httpapi`. Enforced by `go list -deps ./internal/registry | grep -E 'wshub|httpapi'` producing no output.

**No logger in Store (locked):** The Store takes no `*slog.Logger` parameter. Errors are returned, callers (Phase 4 HTTP handler) log with request context.

### Claude's Discretion

- Exact file names inside `internal/registry/` — suggested `manifest.go / mime.go / store.go / persist.go` + `_test.go` siblings. See §"File Layout Recommendation" below for final recommendation.
- `map[string]Manifest` under RWMutex vs `sync.Map` — plain map recommended (atomic rollback is harder with `sync.Map`).
- Pre-canonicalization timing: at load/validate vs at match time — recommended at load (`Validate()` canonicalizes in place). Cheaper per query, and since matching assumes canonicalized inputs on both sides the canonicalization must happen somewhere up front anyway.
- Test fixture filenames — descriptive kebab-case JSON in `internal/registry/testdata/` for Validate+load tests; inline Go literals for the MIME 3×3 matrix (no fixture file needed — the matrix IS the test).
- Specific error message wording (subject to "field path + problem" convention from CONTEXT).

### Deferred Ideas (OUT OF SCOPE)

- Pluggable storage backend (SQLite/Postgres) — v2
- Optimistic concurrency / ETag / 409 — v2
- Manifest versioning (keeping old versions on upsert) — v2
- Backup management (registry.json.bak rotation) — operators handle via fs snapshots
- MIME structured-suffix decomposition (`application/vnd.api+json` also matching `application/json`) — out of scope, match literal subtype strings
- Capability wildcards beyond MIME (path wildcards, action wildcards) — out of scope
- Registry-level audit logging — Phase 4 concern, Store stays logger-free
- Per-capability UUIDs — natural key `(appId, action, path)` is identity
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| REG-01 | Manifest domain type validates required fields (id, name, url, version, non-empty capabilities[]) | §"Manifest.Validate() error catalog" below enumerates every error case and exact string pattern |
| REG-02 | capabilities[].action validated against enum PICK \| SAVE | §"Validate error catalog" — exact-match case-sensitive check, error message includes got-value |
| REG-03 | capabilities[].properties.mimeTypes non-empty, each MIME canonicalized | §"canonicalizeMIME edge-case table" — 20+ cases covered |
| REG-04 | Store guards state with sync.RWMutex; mutations serialize, reads parallelize | §"Concurrency test shape" — small-N fan-out test under `-race` |
| REG-05 | Store.Upsert creates if absent, fully replaces if present | CONTEXT has the exact pseudocode with rollback; no further research needed |
| REG-06 | Store.Delete reports whether it existed | Signature locked in CONTEXT: `(existed bool, err error)` |
| REG-07 | Store.Get returns single manifest or not-found signal | Signature locked in CONTEXT: `(Manifest, bool)`; return a copy |
| REG-08 | Store.List returns all manifests in deterministic order | Sort by id (lexical); no locale-sensitive collation |
| CAP-01 | Store.Capabilities(filter) returns flattened list with {appId, appName, action, path, properties} | Denormalized `CapabilityView` type locked in CONTEXT |
| CAP-02 | Capability results sorted by appName (lowercased) with appId/action/path tiebreakers | §"Sort stability & locale" — `strings.ToLower` is the right call, locale-independent for ASCII |
| CAP-03 | Filtering by action returns exact-match only | Trivial guard; part of §"Capability filtering algorithm" |
| CAP-04 | Filtering by mimeType supports symmetric 3×3 wildcard matching | §"Symmetric MIME 3×3 matrix — full Go table literal" — copy-paste ready |
| CAP-05 | MIME matching covered by exhaustive table-driven test over every 3×3 combination + malformed input rejection | §"Symmetric MIME 3×3 matrix" provides the full test table |
| PERS-01 | Registry state loads from registry.json at startup; missing file yields empty registry without error | §"File loading algorithm" — `errors.Is(err, os.ErrNotExist)` fast-path |
| PERS-02 | Each mutation persists atomically: temp in same dir, Sync, Rename, dir fsync | Pseudocode locked in CONTEXT; §"Atomic persistence — Go stdlib notes" confirms POSIX guarantees |
| PERS-03 | On persist failure, in-memory registry rolls back and mutation returns an error | §"PERS-03 unwritable-directory test recipe" — end-to-end test concrete shape |
| PERS-04 | Corrupted registry.json at startup fails fast with clear error | §"File loading algorithm" — error messages tag file path + offset |
| PERS-05 | registry.json is written human-readable (indented JSON) | `json.MarshalIndent(..., "  ")` or `enc.SetIndent("", "  ")`; see §"Deterministic JSON output" |
| TEST-01 | Table-driven unit tests cover Manifest.Validate, Store mutations, and full MIME matching matrix | All three test shapes specified below |
| TEST-04 | Persistence rollback test uses unwritable directory | §"PERS-03 unwritable-directory test recipe" |
</phase_requirements>

## Summary

Phase 2 is a stdlib-only Go package. No new dependencies. The CONTEXT.md already locks the public API, domain types, validation rules, persistence pseudocode, and the abstract description of symmetric MIME matching. This research pass fills the narrow gaps CONTEXT doesn't exhaust:

1. **The exhaustive 3×3 MIME test table** as a copy-paste Go literal (9 positive cells + 8 negative cases + canonicalization-failure cases).
2. **The full `Manifest.Validate` error catalog** — every exact error string pattern a test assertion can grep for.
3. **The `canonicalizeMIME` edge-case table** — every weird input operators might supply.
4. **The PERS-03 unwritable-directory test** end-to-end with cleanup caveats (Linux CI runs as non-root; `0o500` actually blocks).
5. **`loadFromFile` concrete algorithm** — handles missing, malformed JSON, wrong version, and individual-manifest-validate failures.
6. **Concurrency smoke test** — minimal fan-out pattern that exercises the race detector.
7. **Sort stability** — `strings.ToLower` on ASCII display names is fine; `sort.SliceStable` with 4-key comparator is locale-independent and stable across builds.
8. **Deterministic JSON output** — Go's `encoding/json` sorts map keys since 1.12; manifests array is sorted by id upstream of the encoder so the whole file is byte-stable.
9. **Nyquist validation map** — per-REQ automated verify commands for all 20 REQ IDs, plus a grep gate enforcing the no-transport-import rule.
10. **File layout recommendation** — final split with `CapabilityView`/`CapabilityFilter` living in `store.go` (they're Store API types, not Manifest-intrinsic).

**Primary recommendation:** Land the MIME matrix test, the atomic persistence test (PERS-03), and the `go list -deps` grep gate in the **first commit** of the phase. Everything else can follow.

## Standard Stack

### Core (all stdlib, nothing new)
| Package | Purpose | Notes |
|---------|---------|-------|
| `encoding/json` | Load/save `registry.json`, use `json.NewDecoder(f).DisallowUnknownFields()` at load time, `json.MarshalIndent` (or `enc.SetIndent("", "  ")`) for output | Map keys are sorted since Go 1.12 — anonymous inline structs have fixed field order via struct tags |
| `sync` | `sync.RWMutex` for the Store | Read-heavy workload (every GET /capabilities is an RLock); mutations are rare and take the write lock |
| `os` | `os.ReadFile`, `os.CreateTemp`, `os.Rename`, `os.Remove`, `os.Chmod` (test only) | `os.CreateTemp(dir, pattern)` places the temp file in the same directory as the target — load-bearing for rename atomicity |
| `path/filepath` | `filepath.Dir`, `filepath.Base`, `filepath.Join` | Used to compute same-directory temp-file location |
| `errors` | `errors.Is(err, os.ErrNotExist)` for the missing-file fast path at load time | |
| `fmt` | `fmt.Errorf("context: %w", err)` per project error-wrapping convention (mirrors `internal/config/config.go`) | |
| `sort` | `sort.Strings` for sorted id list in List(); `sort.SliceStable` for Capabilities() 4-key comparator | |
| `strings` | `strings.TrimSpace`, `strings.ToLower`, `strings.SplitN`, `strings.HasPrefix`, `strings.Index` | Locale-independent for ASCII subject strings |
| `net/url` | `url.Parse` to validate manifest `url` field | Reject non-http(s) schemes, empty hosts |

### Test-only
| Package | Purpose |
|---------|---------|
| `testing` | Table-driven tests via `t.Run` subtests |
| `github.com/stretchr/testify/require` | Established in Phase 1; use `require.Error`, `require.NoError`, `require.Contains`, `require.Equal` |

### No new `go get`
`go mod tidy` should produce zero diff after this phase. If `go.sum` grows, something went wrong.

### Version verification (2026-04-09)
Not applicable — this phase adds no dependencies. The Go toolchain was verified in Phase 1 (`go 1.26.2`).

## Architecture Patterns

### File Layout Recommendation

```
internal/registry/
├── manifest.go       # Manifest, Capability, CapabilityProps types + Manifest.Validate()
├── manifest_test.go  # Validate table-driven tests + testdata fixtures
├── mime.go           # canonicalizeMIME + mimeMatch (pure functions, no types)
├── mime_test.go      # The 3×3 matrix table-driven test + canonicalization edge cases
├── store.go          # Store struct + NewStore + Upsert/Delete/Get/List/Capabilities
│                     # CapabilityView + CapabilityFilter live here (Store API types)
├── store_test.go     # Store mutation + concurrency tests + PERS-03 unwritable-dir test
├── persist.go        # persistLocked (atomic write) + loadFromFile (startup load)
├── persist_test.go   # atomic-write golden-file test + loadFromFile fail-fast cases
├── testdata/
│   ├── empty.json               # {"version":1,"manifests":[]}
│   ├── valid-two-apps.json      # Canonical valid file with two manifests
│   ├── malformed-json.json      # Broken JSON (test PERS-04)
│   ├── wrong-version.json       # {"version":2,...}
│   ├── invalid-manifest.json    # Structurally valid but one manifest fails Validate
│   └── unknown-field.json       # Has a top-level key that isn't version/manifests
└── doc.go            # Package doc — may be deleted if manifest.go carries the package comment
```

**Rationale for splits:**
- `manifest.go` carries the canonical package doc (replaces `doc.go`) since `Manifest` is the "face" of the domain. CONTEXT.md's "Existing Code Insights" section explicitly says to do this.
- `mime.go` is pure functions (no types) so test isolation is trivial. Putting it in its own file makes it grep-findable and signals "this is the correctness-critical code."
- `CapabilityView` + `CapabilityFilter` live in `store.go` (not `manifest.go`) because they are **Store API** types, not intrinsic to the Manifest domain. A manifest in isolation has `Capabilities`, not `CapabilityView`s — the view is constructed by the Store from multiple manifests.
- `persist.go` isolates the I/O path (`os.CreateTemp`, `os.Rename`, `loadFromFile`) away from the pure-Go state machine. Lets `store_test.go` stay in-memory for most tests and have persist-specific tests in `persist_test.go`.
- `doc.go` can be deleted (its content moved to the package comment atop `manifest.go`) OR kept as an anchor. Either works; the project convention from Phase 1 deletes redundant anchors once real code exists.

### Pattern 1: Defer Unlock (PITFALLS #12)

**What:** Every mutex acquisition is paired with a deferred unlock on the next line.

**Why:** A panic inside a mutation path (even `fmt.Errorf` on a broken format string) leaks the mutex forever without `defer`. The next request blocks, the process looks alive but is dead. `go vet` does not catch this — it's structural.

```go
// Right:
func (s *Store) Get(id string) (Manifest, bool) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    m, ok := s.manifests[id]
    return m, ok
}

// Wrong (never do this in this package):
func (s *Store) BadGet(id string) (Manifest, bool) {
    s.mu.RLock()
    m, ok := s.manifests[id]
    s.mu.RUnlock()  // <- a panic above this line leaks the lock
    return m, ok
}
```

### Pattern 2: Mutate Under Lock, Release Before Callback (PITFALLS #1, #3)

Not strictly needed in Phase 2 (there are no callbacks in Phase 2 — that's Phase 4's wiring job), but the Store's API is **designed** for this pattern: `Upsert` and `Delete` are single atomic operations that do not call into any other package while holding the mutex. The only thing they call into is `persistLocked` which only touches the filesystem. This preserves the ABBA-safe topology.

### Pattern 3: Snapshot-Before-Mutate Rollback (PITFALLS #5)

CONTEXT.md has the full pseudocode. Summary: snapshot `prev, existed := s.manifests[id]` before mutation; on persist failure, restore via either `s.manifests[id] = prev` (if `existed`) or `delete(s.manifests, id)` (if new). Error message MUST contain the phrase `"registry unchanged"` so the PERS-03 test can assert on it.

### Pattern 4: Value-Semantics Returns (no pointers escape the Store)

`Manifest`, `Capability`, `CapabilityView` are value types with no pointer fields. `Get` and `List` return copies via plain assignment + slice copy. Slices inside the returned values (`Capabilities`, `Properties.MimeTypes`) point into the Store's memory — for v1 this is acceptable because **the Store never mutates a stored manifest in place** (Upsert does full replacement). A caller that does `got, _ := store.Get("x"); got.Capabilities[0].Action = "HACK"` is mutating the stored state. If this matters, deep-copy the slices. Recommendation: **do not deep-copy in v1** — document the no-in-place-mutation contract in the package doc, and note in Phase 4 that the HTTP handler does not mutate returned values (it only serializes them).

### Pattern 5: Doc Comment Style Mirroring config.go

Mirror `internal/config/config.go`:
- Package comment at the top of `manifest.go`: `// Package registry …`
- Every exported type/function has a Godoc comment starting with its name
- Error messages: lowercase, no trailing punctuation, `%q` for user-supplied strings (`fmt.Errorf("manifest.id %q does not match pattern ...", id)`)
- Error wrap: `fmt.Errorf("parse %s: %w", path, err)`

### Anti-Patterns to Avoid

- **`sync.Map` for the manifests map.** CONTEXT explicitly prefers plain map under RWMutex. `sync.Map` does not support the atomic snapshot-mutate-rollback pattern cleanly.
- **Pointers to `Manifest` stored in the map.** Use values. Cloning on return is then just assignment.
- **Canonicalizing MIME types lazily during matching.** CONTEXT locks canonicalization at Validate-time (in place). This makes `mimeMatch` a pure bytewise/prefix comparison with no error path.
- **Calling `log.Printf` or `slog.Default()` anywhere in this package.** Phase 1 established that `internal/` code never reaches for a default logger. Store stays logger-free; errors are returned.
- **Holding `s.mu.Lock()` across any call into another package.** There's no call into another package in this phase — it's a pure domain core. Keep it that way.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| URL parsing for manifest.url | Regex or hand-rolled scheme detection | `net/url.Parse` | Handles edge cases: IPv6 hosts, userinfo, fragments. Rejecting `javascript:` is a `Scheme` check after `Parse`. |
| Atomic file write | `os.Create` + `Encode` + `Close` | `os.CreateTemp` in same dir + `Sync` + `Close` + `Rename` + dir fsync | PITFALLS #4 — the "obvious" path corrupts on crash. |
| JSON serialization determinism | Pre-sort struct fields by hand | Struct with JSON tags — `encoding/json` writes struct fields in declaration order and map keys sorted | Go 1.12+ guarantees sorted map keys in `Marshal` output (verified at [pkg.go.dev/encoding/json#Marshal](https://pkg.go.dev/encoding/json#Marshal)). Combined with sorted manifests slice → byte-stable file. |
| MIME parameter parsing | Regex or `mime.ParseMediaType` | A 10-line `strings.Index(s, ";")` split | `mime.ParseMediaType` is heavier than needed and *errors* on simple things like `text/*` with no parameters. Also brings RFC 2045 strictness that's inappropriate for a canonicalizer that should accept `type/subtype` wildcards. Hand-roll the trivial split — it's exactly the pseudocode in CONTEXT.md. |
| Path validation for manifest `path` field | Regex or custom state machine | `strings.HasPrefix(p, "/")` + length check | Spec says "starts with /, max 500 chars" — that's two lines. No need for anything fancier. |
| Deep-copy of Manifest for Get/List returns | reflect-based cloning or hand-rolled | Plain struct assignment + `append([]T(nil), src...)` for slices | Value types without pointer fields copy correctly via `=`. Slices need an explicit append-to-nil to detach. |

**Key insight:** This phase is small and stdlib-rich. Every "should I pull in X?" question should be answered with "no — write the 15 lines stdlib wants."

## Common Pitfalls

### Pitfall 1: Asymmetric MIME matching (PITFALLS #2)

**What goes wrong:** A capability declares `image/*` and a query asks for `image/png`. The naive matcher checks `cap == query` and returns false. Or the matcher handles `cap` wildcards but forgets that `query` can also be `*/*`.

**Why it happens:** Developers think of matching as "does the query find the capability" (one-directional). Symmetric matching requires thinking of it as "do these two sets intersect."

**How to avoid:** Write the exhaustive 3×3 matrix test **first**, then write `mimeMatch` to pass it. The test is the spec.

**Warning signs:** Any `mimeMatch` implementation that special-cases the capability side without mirroring the query side. Any test with fewer than 9 positive cases.

### Pitfall 2: Non-atomic persistence (PITFALLS #4)

Covered by the CONTEXT pseudocode. Warning signs: `os.Create` (truncates), any write that doesn't `Sync` before rename, any temp file in `os.TempDir()` instead of `filepath.Dir(s.path)`.

### Pitfall 3: In-memory / on-disk divergence on persist failure (PITFALLS #5)

Covered by CONTEXT. The PERS-03 test is the enforcement mechanism — see §"PERS-03 test recipe" below.

### Pitfall 4: ABBA deadlock across packages (PITFALLS #1, #3)

Not directly triggerable in Phase 2 (Store imports nothing), but the **architectural prevention** is enforced here: the Store must not import `wshub` or `httpapi`. Grep gate: `! go list -deps ./internal/registry | grep -E 'wshub|httpapi'` must return exit 0.

### Pitfall 5: Lock leak on panic (PITFALLS #12)

See Pattern 1 above. `defer s.mu.Unlock()` on every acquisition.

### Pitfall 6: Map iteration order and test flakes

Go randomizes map iteration order. Any test that iterates `s.manifests` directly will be flaky. Always sort before asserting. `Store.List()` and `Store.Capabilities()` do this; tests that reach deeper must do it too.

### Pitfall 7: `json.NewDecoder` + `DisallowUnknownFields` at the wrong scope

Apply `DisallowUnknownFields` to the **top-level file format struct** (`fileFormat{ Version int, Manifests []Manifest }`) at load time — this catches operator-introduced typos. Do NOT apply it to `Manifest` inside tests (some test fixtures may want forward-compatibility). The Phase 4 HTTP handler will apply it to Manifest decoding on POST requests.

### Pitfall 8: `strings.ToLower` on non-ASCII display names

`strings.ToLower` uses Unicode case-folding (`unicode.ToLower` per rune). It's locale-independent (does not depend on `LC_CTYPE`). For ASCII-only strings it's equivalent to a bytewise `s & 0x5F` kind of lowercase — deterministic across machines. For Unicode, it's still deterministic but uses Go's case-folding tables (which can change across Go versions — but only for obscure scripts; for CJK, Latin, Cyrillic etc. the tables are stable). **Recommendation:** use `strings.ToLower` directly, document in the code comment that sort is "locale-independent via `strings.ToLower` (Unicode case-fold)". For the expected manifest display names (ASCII + common Unicode), this is stable across rebuilds, restarts, and platforms — which is what CONTEXT's sort-order decision requires.

## Code Examples

### Example 1: Symmetric MIME 3×3 Matrix — Full Go Table Literal

This is the load-bearing test. Copy this into `internal/registry/mime_test.go` verbatim. It covers all 9 positive cells of the matrix plus negative cases and canonicalization edge cases.

```go
// Source: Synthesized from CONTEXT.md §"MIME canonicalization and matching"
// and PITFALLS.md §2 (symmetric MIME matching).
//
// The 3×3 matrix (both inputs canonicalized):
//
//     cap \ q  | exact (image/png) | type/*  (image/*) | */* (any)
//     ---------|-------------------|-------------------|----------
//     exact    | bytewise equal    | type matches      | always
//     type/*   | type matches      | type matches      | always
//     */*      | always            | always            | always

func TestMimeMatch(t *testing.T) {
    tests := []struct {
        name string
        cap  string
        q    string
        want bool
    }{
        // --- 9 positive cells of the 3×3 matrix ---
        {"exact vs exact (same)",          "image/png", "image/png", true},
        {"exact vs type/* (same type)",    "image/png", "image/*",   true},
        {"exact vs */*",                   "image/png", "*/*",       true},
        {"type/* vs exact (same type)",    "image/*",   "image/png", true},
        {"type/* vs type/* (same type)",   "image/*",   "image/*",   true},
        {"type/* vs */*",                  "image/*",   "*/*",       true},
        {"*/* vs exact",                   "*/*",       "image/png", true},
        {"*/* vs type/*",                  "*/*",       "image/*",   true},
        {"*/* vs */*",                     "*/*",       "*/*",       true},

        // --- negative: different exact types ---
        {"exact vs exact (different type)",    "image/png",  "image/jpeg", false},
        {"exact vs exact (different family)",  "image/png",  "text/plain", false},

        // --- negative: exact vs type/* with different type ---
        {"exact vs type/* (different type)",   "image/png",  "text/*",     false},
        {"type/* vs exact (different type)",   "image/*",    "text/plain", false},

        // --- negative: type/* vs type/* with different type ---
        {"type/* vs type/* (different type)",  "image/*",    "text/*",     false},

        // --- subtype boundary cases (avoid substring bugs) ---
        {"exact vs exact (subtype prefix)",    "image/pn",   "image/png",  false},
        {"exact vs exact (subtype superstring)","image/png", "image/pngx", false},
        {"exact vs exact (type prefix)",       "imag/png",   "image/png",  false},
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            require.Equal(t, tc.want, mimeMatch(tc.cap, tc.q),
                "mimeMatch(%q, %q)", tc.cap, tc.q)
            // Symmetry: matching must be commutative.
            require.Equal(t, tc.want, mimeMatch(tc.q, tc.cap),
                "symmetric mimeMatch(%q, %q)", tc.q, tc.cap)
        })
    }
}
```

**Notes for the planner:**
- The test asserts symmetry explicitly (`mimeMatch(a, b) == mimeMatch(b, a)`) — this catches the single most common bug class.
- All inputs are already canonicalized; `canonicalizeMIME` is tested separately (see next example).
- The test has 17 cases. The matrix has 9 positive cells + 8 negatives = 17. **This is the minimum. Do not ship fewer.**

### Example 2: `canonicalizeMIME` Edge-Case Table

```go
// Source: Synthesized from CONTEXT.md §"MIME canonicalization".
// Every input CanonicalizeMIME should accept or reject with a specific
// error shape. Paste into mime_test.go.

func TestCanonicalizeMIME(t *testing.T) {
    tests := []struct {
        name    string
        in      string
        want    string
        wantErr bool
    }{
        // --- normal cases ---
        {"simple exact",              "image/png",                    "image/png", false},
        {"type wildcard",             "image/*",                      "image/*",   false},
        {"full wildcard",             "*/*",                          "*/*",       false},
        {"uppercase exact",           "IMAGE/PNG",                    "image/png", false},
        {"mixed case",                "Image/Png",                    "image/png", false},
        {"structured suffix kept",    "application/vnd.api+json",     "application/vnd.api+json", false},

        // --- whitespace handling ---
        {"leading whitespace",        "  image/png",                  "image/png", false},
        {"trailing whitespace",       "image/png  ",                  "image/png", false},
        {"both sides whitespace",     "  image/png  ",                "image/png", false},

        // --- parameter stripping ---
        {"with charset param",        "text/plain; charset=utf-8",    "text/plain", false},
        {"with boundary param",       "multipart/form-data; boundary=xyz", "multipart/form-data", false},
        {"multiple params",           "text/plain; charset=utf-8; format=flowed", "text/plain", false},
        {"trailing semicolon",        "text/plain;",                  "text/plain", false},
        {"uppercase with param",      "TEXT/PLAIN; CHARSET=UTF-8",    "text/plain", false},
        {"param then whitespace",     "text/plain ; charset=utf-8",   "text/plain", false},

        // --- rejection cases ---
        {"empty string",              "",                             "", true},
        {"whitespace only",           "   ",                          "", true},
        {"no slash",                  "image",                        "", true},
        {"just slash",                "/",                            "", true},
        {"double slash",              "image//png",                   "", true}, // SplitN(..., 2) → ["image", "/png"] — subtype "/png" is accepted by split but rejected because subtype contains "/". See note below.
        {"empty type",                "/png",                         "", true},
        {"empty subtype",             "image/",                       "", true},
        {"wildcard subtype only",     "*/subtype",                    "", true}, // Explicitly rejected per CONTEXT: "*/subtype is NOT valid".
        {"three segments",            "image/png/extra",              "", true}, // SplitN(..., 2) would produce ["image", "png/extra"]; subtype containing "/" is rejected.
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            got, err := canonicalizeMIME(tc.in)
            if tc.wantErr {
                require.Error(t, err, "canonicalizeMIME(%q) should error", tc.in)
                return
            }
            require.NoError(t, err)
            require.Equal(t, tc.want, got)
        })
    }
}
```

**Note on "double slash" and "three segments":** The pseudocode in CONTEXT uses `strings.SplitN(s, "/", 2)`. That only splits into 2 parts — `"image//png"` becomes `["image", "/png"]` and `"image/png/extra"` becomes `["image", "png/extra"]`. These pass the 2-parts-nonempty check. **To correctly reject them, add a post-split validation:** `if strings.Contains(parts[1], "/") { return "", errors.New(...) }`. Similarly reject `"*/subtype"` with `if parts[0] == "*" && parts[1] != "*" { return "", errors.New(...) }`. The planner must include these rules in the implementation task; the canonicalize function in CONTEXT is a sketch, not a drop-in.

### Example 3: `Manifest.Validate()` Error Catalog

The planner needs a copy-paste catalog so tests can assert on exact substrings. All messages follow the `"field.path: problem"` convention from CONTEXT ("`validate: manifest.id is required`" / "`capability[2].action must be ...`"). Wrapped under `validate:` prefix for easy grepping in error messages.

| Field | Failure case | Error message (substring tests can assert on) |
|-------|--------------|-----------------------------------------------|
| `id` | empty | `manifest.id is required` |
| `id` | pattern mismatch | `manifest.id %q does not match pattern ^[a-zA-Z0-9][a-zA-Z0-9._-]*$` |
| `id` | too long (>128) | `manifest.id too long: 129 chars (max 128)` |
| `name` | empty after TrimSpace | `manifest.name is required` |
| `name` | too long (>200) | `manifest.name too long: N chars (max 200)` |
| `url` | empty | `manifest.url is required` |
| `url` | url.Parse failed | `manifest.url is invalid: parse %q: ...` |
| `url` | scheme not http/https | `manifest.url scheme must be http or https, got %q` |
| `url` | empty host | `manifest.url has empty host` |
| `version` | empty | `manifest.version is required` |
| `version` | too long (>64) | `manifest.version too long: N chars (max 64)` |
| `capabilities` | nil or empty slice | `manifest.capabilities must be non-empty` |
| `capabilities[i].action` | empty | `capability[%d].action is required` |
| `capabilities[i].action` | not PICK or SAVE | `capability[%d].action must be "PICK" or "SAVE", got %q` |
| `capabilities[i].path` | empty | `capability[%d].path is required` |
| `capabilities[i].path` | doesn't start with / | `capability[%d].path must start with "/"` |
| `capabilities[i].path` | too long (>500) | `capability[%d].path too long: N chars (max 500)` |
| `capabilities[i].properties.mimeTypes` | nil or empty | `capability[%d].properties.mimeTypes must be non-empty` |
| `capabilities[i].properties.mimeTypes[j]` | canonicalization failed | `capability[%d].properties.mimeTypes[%d]: <canonicalize error>` |

**Validation strategy:** Validate fails on the FIRST error found. Do not collect and return multi-errors in v1 — CONTEXT says "one failing field at a time." This keeps the test assertion pattern simple (`require.Contains(err.Error(), "capability[2].action")`).

**Validation side effect:** On success, `Validate` **mutates the receiver in place** by rewriting each `Capabilities[i].Properties.MimeTypes[j]` to its canonicalized form. This is important for the Store: after `Upsert(m); m = validate then stored`, the stored manifest has already-canonical MIME types, so `mimeMatch` on the query path is a pure comparison with no error path.

**Example `Validate` skeleton** (for the planner):

```go
// Source: synthesized from CONTEXT.md validation rules + internal/config/config.go style.
func (m *Manifest) Validate() error {
    if m.ID == "" {
        return errors.New("validate: manifest.id is required")
    }
    if !idPattern.MatchString(m.ID) {
        return fmt.Errorf("validate: manifest.id %q does not match pattern %s", m.ID, idPatternString)
    }
    if len(m.ID) > 128 {
        return fmt.Errorf("validate: manifest.id too long: %d chars (max 128)", len(m.ID))
    }
    name := strings.TrimSpace(m.Name)
    if name == "" {
        return errors.New("validate: manifest.name is required")
    }
    // ... etc
    for i := range m.Capabilities {
        cap := &m.Capabilities[i]
        if cap.Action != "PICK" && cap.Action != "SAVE" {
            return fmt.Errorf("validate: capability[%d].action must be \"PICK\" or \"SAVE\", got %q", i, cap.Action)
        }
        // ... etc
        for j, mt := range cap.Properties.MimeTypes {
            canon, err := canonicalizeMIME(mt)
            if err != nil {
                return fmt.Errorf("validate: capability[%d].properties.mimeTypes[%d]: %w", i, j, err)
            }
            cap.Properties.MimeTypes[j] = canon  // <- in-place canonicalization
        }
    }
    return nil
}

var (
    idPatternString = `^[a-zA-Z0-9][a-zA-Z0-9._-]*$`
    idPattern       = regexp.MustCompile(idPatternString)
)
```

### Example 4: PERS-03 Unwritable-Directory Test Recipe

```go
// Source: Synthesized from CONTEXT.md §"In-memory rollback on persist failure"
// and PITFALLS.md §5. This is the PERS-03 / TEST-04 canonical test.

func TestStore_Upsert_PersistFailureRollsBack(t *testing.T) {
    // Create a writable temp dir, seed a valid registry, open a Store.
    dir := t.TempDir()
    path := filepath.Join(dir, "registry.json")

    store, err := NewStore(path)
    require.NoError(t, err)

    // Seed with one manifest so we can distinguish "reverted to prior" from
    // "wiped to empty".
    seed := Manifest{
        ID:      "seed-app",
        Name:    "Seed",
        URL:     "https://example.com",
        Version: "1.0.0",
        Capabilities: []Capability{{
            Action: "PICK",
            Path:   "/pick",
            Properties: CapabilityProps{MimeTypes: []string{"*/*"}},
        }},
    }
    require.NoError(t, store.Upsert(seed))

    // Make the directory unwritable. On Linux CI runners we run as non-root,
    // so 0o500 (r-x) actually blocks CreateTemp. On macOS this also works.
    // On Windows this test is a no-op; gate with build tag if needed.
    require.NoError(t, os.Chmod(dir, 0o500))
    t.Cleanup(func() {
        // Restore writable bits so t.TempDir() cleanup can remove the dir.
        _ = os.Chmod(dir, 0o700)
    })

    // Attempt to upsert a new manifest. persistLocked should fail (CreateTemp
    // returns EACCES), Store should roll back, error should contain
    // "registry unchanged".
    newApp := Manifest{
        ID:      "new-app",
        Name:    "New",
        URL:     "https://example.com",
        Version: "1.0.0",
        Capabilities: []Capability{{
            Action: "SAVE",
            Path:   "/save",
            Properties: CapabilityProps{MimeTypes: []string{"text/plain"}},
        }},
    }
    err = store.Upsert(newApp)
    require.Error(t, err)
    require.Contains(t, err.Error(), "registry unchanged")

    // Assert: the new app is NOT in the in-memory state (it was rolled back).
    _, found := store.Get("new-app")
    require.False(t, found, "new-app should not be present after failed persist")

    // Assert: the seed app is STILL in the in-memory state (untouched).
    got, found := store.Get("seed-app")
    require.True(t, found, "seed-app should still be present")
    require.Equal(t, "Seed", got.Name)

    // Also test update-failure path: try to overwrite seed-app and assert
    // it rolls back to the original value.
    modified := seed
    modified.Name = "Modified"
    err = store.Upsert(modified)
    require.Error(t, err)
    require.Contains(t, err.Error(), "registry unchanged")
    got, _ = store.Get("seed-app")
    require.Equal(t, "Seed", got.Name, "seed-app should have been rolled back to original")
}
```

**Cross-platform caveat:** `os.Chmod(dir, 0o500)` blocks writes on Linux/macOS when running as non-root. CI runners (GitHub Actions ubuntu-latest) run as non-root by default, so this works. On Windows the semantics are different (`os.Chmod` only toggles the read-only bit on files, not directories). **Recommendation:** the test does not need a build tag because the project is Linux-first per PROJECT.md — but add a comment in the test file explaining the assumption. If the test is ever run on Windows CI it may spuriously pass (the write succeeds) — the PITFALLS #4 note about "Windows os.Rename caveats" applies to the production code path, not the test.

**`t.TempDir` cleanup gotcha:** If the test leaves the directory mode at `0o500`, `t.TempDir`'s automatic cleanup fails silently at the end of the test run (RemoveAll gets EACCES on the parent's delete). The `t.Cleanup` restoring `0o700` is mandatory. Without it, the CI runner accumulates phantom temp directories (not a test failure, but a warning in test output).

### Example 5: Concurrency Smoke Test (REG-04)

```go
// Source: Synthesized from PITFALLS.md §13 ("go test -race in CI") and
// CONTEXT.md §"Store API" — intended to exercise the RWMutex under -race.

func TestStore_ConcurrentAccess(t *testing.T) {
    dir := t.TempDir()
    store, err := NewStore(filepath.Join(dir, "registry.json"))
    require.NoError(t, err)

    // Seed 5 manifests.
    for i := 0; i < 5; i++ {
        m := Manifest{
            ID:      fmt.Sprintf("app-%d", i),
            Name:    fmt.Sprintf("App %d", i),
            URL:     "https://example.com",
            Version: "1.0.0",
            Capabilities: []Capability{{
                Action: "PICK", Path: "/pick",
                Properties: CapabilityProps{MimeTypes: []string{"*/*"}},
            }},
        }
        require.NoError(t, store.Upsert(m))
    }

    // 10 writers × 10 upserts each, concurrent with 10 readers × many reads.
    var wg sync.WaitGroup

    for w := 0; w < 10; w++ {
        wg.Add(1)
        go func(w int) {
            defer wg.Done()
            for i := 0; i < 10; i++ {
                m := Manifest{
                    ID:      fmt.Sprintf("writer-%d-%d", w, i),
                    Name:    "Writer",
                    URL:     "https://example.com",
                    Version: "1.0.0",
                    Capabilities: []Capability{{
                        Action: "SAVE", Path: "/save",
                        Properties: CapabilityProps{MimeTypes: []string{"text/plain"}},
                    }},
                }
                _ = store.Upsert(m)  // ignore error — goal is to exercise mutex
            }
        }(w)
    }

    for r := 0; r < 10; r++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for i := 0; i < 50; i++ {
                _ = store.List()
                _, _ = store.Get("app-0")
                _ = store.Capabilities(CapabilityFilter{Action: "PICK"})
            }
        }()
    }

    wg.Wait()

    // Sanity: final state should have at least the 5 seed + 100 writer manifests.
    require.GreaterOrEqual(t, len(store.List()), 105)
}
```

**Run with:** `go test -race ./internal/registry -run TestStore_ConcurrentAccess`. If the race detector reports anything, the test fails. If no races, it passes silently. No performance assertions — this is a correctness smoke test, not a benchmark.

### Example 6: `loadFromFile` Concrete Algorithm

```go
// Source: Synthesized from CONTEXT.md §"Load-at-startup behavior" and
// internal/config/config.go error wrapping style.

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
            return nil, nil  // fast path: greenfield start
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

    // Validate every manifest. Validate() also canonicalizes in place, so
    // after this loop the slice contains only canonical MIME strings.
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

**Key decisions baked in:**
- `DisallowUnknownFields` applied to `fileFormat` so a stray top-level key (e.g. operator added `"comment": "..."`) fails fast. This is a deliberate operator-strictness choice.
- The error message includes the full file path (for operator debugging) and — thanks to `%w` wrapping — the underlying `json.Decoder` error which includes byte offset and field context.
- Each manifest's Validate is called in the loop, so MIME canonicalization happens at load time. After `NewStore`, every stored manifest has already-canonical MIME types.
- Version mismatch error is distinct from JSON parse error so the PERS-04 test can assert on specific substrings.

### Example 7: Deterministic JSON Output for `persistLocked`

```go
// Source: CONTEXT.md §"File format" + Go 1.12+ json.Marshal map-key sort guarantee.

func (s *Store) snapshot() fileFormat {
    // Copy manifests into a slice sorted by ID so the output is byte-stable.
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

// Inside persistLocked:
//   enc := json.NewEncoder(tmp)
//   enc.SetIndent("", "  ")
//   if err := enc.Encode(s.snapshot()); err != nil { ... }
```

**Why this is deterministic:**
- `s.manifests` is a Go map (iteration order randomized). `sort.Strings(ids)` removes that nondeterminism.
- `encoding/json` writes struct fields **in declaration order** (not alphabetically). So `Manifest{ID, Name, URL, Version, Capabilities}` always serializes in that order, regardless of how the struct was constructed.
- Inside `Capabilities[i]`, same rule: struct fields in declaration order.
- Inside `Properties.MimeTypes`, the slice order is preserved — and since `Validate` canonicalizes in place without reordering, the order is whatever the operator POSTed. **Should we sort MimeTypes within a capability?** CONTEXT doesn't require it, but FEATURES.md §"Deterministic capability ordering" mentioned "sort mimeTypes[] within each capability" as a nice-to-have. **Recommendation: yes — sort MimeTypes alphabetically at the end of Validate().** It's 1 line (`sort.Strings(mimeTypes)`) and eliminates the last source of nondeterminism in the file.
- `encoding/json` does not encode any `map[K]V` in Manifest — every field is a concrete struct or slice, so the "map keys sorted since Go 1.12" behavior is not even exercised. Belt-and-suspenders: no nondeterminism surface.

**Source:** [pkg.go.dev/encoding/json#Marshal](https://pkg.go.dev/encoding/json#Marshal) — "The map keys are sorted and used as JSON object keys by applying the following rules..."

### Example 8: `Store.Capabilities` Filter+Sort Implementation

```go
// Source: Synthesized from CONTEXT.md §"Capability sort order" and
// §"Store API". CAP-01..04 all live here.

func (s *Store) Capabilities(filter CapabilityFilter) []CapabilityView {
    s.mu.RLock()
    defer s.mu.RUnlock()

    // Canonicalize the query MIME once, outside the loop. Empty = no filter.
    var wantMime string
    var wantMimeSet bool
    if filter.MimeType != "" {
        canon, err := canonicalizeMIME(filter.MimeType)
        if err != nil {
            // Malformed query MIME → return empty result set (not an error —
            // the filter is a query-param, not a mutation). Phase 4's handler
            // chose whether to 400 or empty-200; Store returns empty.
            return nil
        }
        wantMime = canon
        wantMimeSet = true
    }

    out := make([]CapabilityView, 0)
    for _, m := range s.manifests {
        for _, c := range m.Capabilities {
            // Action filter (exact match, case-sensitive).
            if filter.Action != "" && c.Action != filter.Action {
                continue
            }
            // MIME filter: OR across the capability's mimeTypes — a capability
            // matches if ANY of its declared types matches the query.
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
        if ai != aj { return ai < aj }
        if out[i].AppID != out[j].AppID { return out[i].AppID < out[j].AppID }
        if out[i].Action != out[j].Action { return out[i].Action < out[j].Action }
        return out[i].Path < out[j].Path
    })

    return out
}
```

**Subtlety:** `CapabilityView.Properties` is a value copy of `CapabilityProps{MimeTypes []string}`. The slice header is copied, but the underlying array is shared with the stored Manifest. A caller mutating `returned[0].Properties.MimeTypes[0]` would mutate the stored state. For v1 this is acceptable if the package-doc contract says "do not mutate returned values." If deep-copy safety matters, add `append([]string(nil), c.Properties.MimeTypes...)` in the loop. **Recommendation:** document the no-mutation contract; skip the deep-copy overhead in v1.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Non-atomic file writes (`Create` + `Encode` + `Close`) | `CreateTemp` in same dir + `Sync` + `Rename` + dir fsync | Well-established since ~2010; PITFALLS #4 cites it | Required for correctness |
| `sync.Map` for all concurrent maps | Plain map + RWMutex for non-hot-path reads | `sync.Map` was introduced Go 1.9, community learned it's only faster under very specific access patterns | RWMutex wins on simplicity and enables atomic rollback |
| `mime.ParseMediaType` for MIME canonicalization | Hand-rolled `strings.Index(";")` split | N/A — `mime.ParseMediaType` errors on trailing `;` and on wildcard subtypes, which are valid for this use case | Use stdlib `strings`, not `mime` package |
| Regex for URL validation | `net/url.Parse` + field check | Always — `net/url` was stdlib from day 1 | Correctness, no regex debt |
| Custom JSON deterministic output | `encoding/json` with sorted slices + struct declaration order | Go 1.12 added map key sorting in `Marshal` | No custom marshaler needed |

**Deprecated/outdated (do not do):**
- `ioutil.ReadFile` / `ioutil.WriteFile` — moved to `os.ReadFile` / `os.WriteFile` in Go 1.16; `ioutil` is soft-deprecated
- `filepath.Walk` for directory ops in tests — not needed here, but prefer `filepath.WalkDir` if you ever are
- `encoding/json.Unmarshal` without `DisallowUnknownFields` for operator-supplied files — drops typos silently

## Open Questions

1. **Should `Capabilities[].Properties.MimeTypes` be sorted alphabetically after canonicalization?**
   - What we know: CONTEXT doesn't require it. FEATURES.md §"Deterministic capability ordering" suggested it.
   - What's unclear: Whether operators care about byte-identical registry.json diffs across rewrites when the same manifest is re-upserted with mimeTypes in different order.
   - Recommendation: **Yes, sort.** One line, zero downside, eliminates the last source of file nondeterminism. Add it to the end of `Validate()` after canonicalization. Mention in the package doc.

2. **Should the canonicalizer reject empty params like `text/plain;`?**
   - What we know: CONTEXT's pseudocode strips the `;` and anything after, so `text/plain;` → `text/plain`. The canonicalize edge-case table above treats this as valid.
   - What's unclear: Whether a stricter parser (rejecting trailing `;`) would catch more operator typos.
   - Recommendation: **Accept `text/plain;`** — it's harmless, the lenient parser is simpler, and clients might legitimately strip params this way before POSTing. Lock this in the test table as an accept case.

3. **Should `Store.Capabilities` with a malformed `filter.MimeType` return an error instead of an empty list?**
   - What we know: CONTEXT doesn't specify. The pseudocode above returns empty.
   - What's unclear: Whether Phase 4's HTTP handler would want to 400 on malformed `?mimeType=` or silently return empty.
   - Recommendation: **Store returns empty** (not an error). Phase 4 can canonicalize the query MIME itself before calling Store and 400 on canonicalization failure — that's the handler's concern, not the Store's. This keeps Store's contract pure (no error path on reads).
   - Alternative: add a `CanonicalizeMIME` exported helper that Phase 4 uses to validate query params before calling `Store.Capabilities`. **This is the recommendation** — export `CanonicalizeMIME` so Phase 4 can use it without reimplementing.

4. **Should `NewStore` accept a missing directory (i.e. `mkdir -p` the parent)?**
   - What we know: CONTEXT says "file missing → empty store, no error." Silent on parent-directory-missing.
   - What's unclear: Whether "file missing" extends to "the directory the file lives in is missing."
   - Recommendation: **Do not mkdir.** Missing parent directory is an operator error — they configured `registry_file: /nonexistent/path/registry.json`. `os.Open` returns `os.ErrNotExist` for both cases; distinguish with `os.Stat(filepath.Dir(path))` if the clearer error matters. **Simpler recommendation:** only the missing-file case is a fast-path empty-store; any other error (permission denied, missing parent dir) surfaces as an error. This matches operator intent: if you set the config path wrong, you want to know.

5. **Should `Delete` on a non-existent id trigger a persist?**
   - What we know: CONTEXT signature is `(existed bool, err error)`. Not specified whether delete-of-nonexistent is a no-op or a disk write.
   - What's unclear: Whether an idempotent Delete that rewrites the file unnecessarily matters.
   - Recommendation: **Delete of non-existent is a no-op** — return `(false, nil)` without touching disk. Saves a disk write, makes repeated deletes cheap. Phase 4's HTTP handler uses the `existed` flag to choose 204 vs 404.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` + `github.com/stretchr/testify/require` v1.11.x (established in Phase 1) |
| Config file | None (`go test` discovers `*_test.go`) |
| Quick run command | `go test ./internal/registry -count=1` |
| Full suite command | `go test ./... -race -count=1` |
| Per-test filter | `go test ./internal/registry -run TestName -v` |

### Phase Requirements → Test Map

Every REQ ID has at least one automated verify command. Commands are `go test` invocations (runnable in < 30s each) or stdlib grep gates. The file paths are the recommended layout from §"File Layout Recommendation"; the planner may rename them but must keep the ID → test mapping.

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| REG-01 | Manifest validates required fields | unit | `go test ./internal/registry -run TestManifestValidate -v` | Wave 0 |
| REG-02 | action validated against PICK\|SAVE | unit (subtest of REG-01) | `go test ./internal/registry -run TestManifestValidate/action` | Wave 0 |
| REG-03 | mimeTypes non-empty, canonicalized | unit (subtest of REG-01 + TestCanonicalizeMIME) | `go test ./internal/registry -run 'TestManifestValidate/mime|TestCanonicalizeMIME'` | Wave 0 |
| REG-04 | Store RWMutex: mutations serialize, reads parallelize | unit (race) | `go test ./internal/registry -run TestStore_ConcurrentAccess -race` | Wave 0 |
| REG-05 | Upsert creates-or-replaces | unit | `go test ./internal/registry -run TestStore_Upsert` | Wave 0 |
| REG-06 | Delete reports existed bool | unit | `go test ./internal/registry -run TestStore_Delete` | Wave 0 |
| REG-07 | Get returns manifest or not-found | unit | `go test ./internal/registry -run TestStore_Get` | Wave 0 |
| REG-08 | List returns all in deterministic order | unit | `go test ./internal/registry -run TestStore_List` | Wave 0 |
| CAP-01 | Capabilities flattens with denormalized appId/appName | unit | `go test ./internal/registry -run TestStore_Capabilities/flatten` | Wave 0 |
| CAP-02 | Capabilities sorted by (lower(appName), appId, action, path) | unit | `go test ./internal/registry -run TestStore_Capabilities/sort` | Wave 0 |
| CAP-03 | Filter by action exact-match | unit | `go test ./internal/registry -run TestStore_Capabilities/filter_action` | Wave 0 |
| CAP-04 | Filter by mimeType, symmetric 3×3 | unit | `go test ./internal/registry -run 'TestStore_Capabilities/filter_mime|TestMimeMatch'` | Wave 0 |
| CAP-05 | Exhaustive 3×3 matrix + malformed rejection | unit | `go test ./internal/registry -run 'TestMimeMatch|TestCanonicalizeMIME' -v` | Wave 0 |
| PERS-01 | Missing file → empty store, no error | unit | `go test ./internal/registry -run TestNewStore_MissingFile` | Wave 0 |
| PERS-02 | Atomic write: temp→Sync→Rename→dir fsync | unit (golden-file) | `go test ./internal/registry -run TestStore_Upsert_WritesAtomically` | Wave 0 |
| PERS-03 | Persist failure rolls back in-memory | unit (unwritable dir) | `go test ./internal/registry -run TestStore_Upsert_PersistFailureRollsBack` | Wave 0 |
| PERS-04 | Corrupted registry.json fails fast | unit | `go test ./internal/registry -run TestNewStore_CorruptedFile` | Wave 0 |
| PERS-05 | registry.json written human-readable (indented) | unit (golden-file) | `go test ./internal/registry -run TestStore_Upsert_WritesIndentedJSON` | Wave 0 |
| TEST-01 | Table-driven coverage of Validate + Store + MIME matrix | meta | `go test ./internal/registry -v` (all the above pass) | Wave 0 |
| TEST-04 | Rollback test uses unwritable directory | == PERS-03 | (same as PERS-03) | Wave 0 |

**Additional architectural gates (not REQ IDs but phase acceptance criteria):**

| Gate | Command | Purpose |
|------|---------|---------|
| No transport imports | `! go list -deps ./internal/registry 2>&1 \| grep -E 'wshub\|httpapi'` | Enforces PITFALLS #1 — ABBA deadlock prevention |
| No logger imports | `! grep -rE 'log/slog\|slog.Default' internal/registry/*.go` (production files only, not _test.go) | Enforces "Store is logger-free" |
| Race detector clean | `go test ./internal/registry -race -count=1` | Enforces REG-04 + PITFALLS #13 |
| gofmt clean | `test -z "$(gofmt -l internal/registry/)"` | Project-wide convention |
| go vet clean | `go vet ./internal/registry` | Project-wide convention |

### Sampling Rate

- **Per task commit:** `go test ./internal/registry -count=1` (<5s on any modern machine)
- **Per wave merge:** `go test ./internal/registry -race -count=1` + the architectural gates above (<15s)
- **Phase gate:** `go test ./... -race -count=1` — all packages, all tests, full green before `/gsd:verify-work`

### Wave 0 Gaps

All test files need to be created — Phase 2 is a new package with only `doc.go` today.

- [ ] `internal/registry/manifest.go` — Manifest/Capability/CapabilityProps types + Validate()
- [ ] `internal/registry/manifest_test.go` — TestManifestValidate table-driven
- [ ] `internal/registry/mime.go` — canonicalizeMIME + mimeMatch (+ exported CanonicalizeMIME wrapper for Phase 4 to use)
- [ ] `internal/registry/mime_test.go` — TestMimeMatch (3×3 matrix + symmetry check) + TestCanonicalizeMIME (edge-case table)
- [ ] `internal/registry/store.go` — Store + NewStore + Upsert/Delete/Get/List/Capabilities + CapabilityView + CapabilityFilter
- [ ] `internal/registry/store_test.go` — TestStore_Upsert, TestStore_Delete, TestStore_Get, TestStore_List, TestStore_Capabilities (with /flatten, /sort, /filter_action, /filter_mime subtests), TestStore_ConcurrentAccess, TestStore_Upsert_PersistFailureRollsBack
- [ ] `internal/registry/persist.go` — persistLocked + loadFromFile + fileFormat type + currentFormatVersion const
- [ ] `internal/registry/persist_test.go` — TestNewStore_MissingFile, TestNewStore_CorruptedFile, TestStore_Upsert_WritesAtomically, TestStore_Upsert_WritesIndentedJSON
- [ ] `internal/registry/testdata/empty.json` — `{"version":1,"manifests":[]}`
- [ ] `internal/registry/testdata/valid-two-apps.json` — canonical valid file
- [ ] `internal/registry/testdata/malformed-json.json` — broken JSON for PERS-04
- [ ] `internal/registry/testdata/wrong-version.json` — `{"version":2,...}` for PERS-04
- [ ] `internal/registry/testdata/invalid-manifest.json` — structurally valid JSON, semantically invalid manifest
- [ ] `internal/registry/testdata/unknown-field.json` — has a top-level field that isn't version/manifests

**Framework install:** None — stdlib `testing` and testify/require v1.11 are already pinned in `go.mod` from Phase 1.

**REQUIREMENTS.md update:** The planner's first task should update REQUIREMENTS.md CAP-02 from "sorted by (appId, action, mimeType[0])" to "sorted by (appName, appId, action, path) lowercased". This is explicitly called out in CONTEXT.md's deferred-ideas section as a same-commit todo.

## Sources

### Primary (HIGH confidence)

- `.planning/phases/02-registry-core/02-CONTEXT.md` — locked decisions (all strategic choices sourced here)
- `.planning/REQUIREMENTS.md` — 20 REQ IDs Phase 2 must close
- `.planning/research/PITFALLS.md` §1-5, §12, §13 — atomic persistence, rollback, ABBA prevention, defer unlock, race detector
- `.planning/research/FEATURES.md` §"MIME Matching Semantics" — the canonical symmetric wildcard specification
- `.planning/research/ARCHITECTURE.md` §"internal/registry" — Store pattern, atomic write-through, package boundary
- `.planning/research/STACK.md` — stdlib-only confirmation for Phase 2
- `internal/config/config.go` — error wrapping style, package doc convention, validate-fails-fast pattern
- `internal/config/config_test.go` — table-driven test style with testdata fixtures
- [pkg.go.dev/encoding/json#Marshal](https://pkg.go.dev/encoding/json#Marshal) — confirmed map keys sorted in output (Go 1.12+)
- Go stdlib `os.CreateTemp` documentation — temp-file-in-same-directory pattern for rename atomicity

### Secondary (MEDIUM confidence)

- Developer community convention on `t.Chmod(dir, 0o500)` for unwritable-dir tests on Linux CI — widely used in Go test suites; requires non-root runner (GitHub Actions default)
- `strings.ToLower` locale-independence — verified by knowledge of Go's `unicode` package implementation (case-folding tables are compile-time constants, no `LC_CTYPE` lookup)

### Tertiary (LOW confidence)

- None. Everything in this research is either locked by CONTEXT or verified against stdlib documentation.

## Metadata

**Confidence breakdown:**
- Standard stack: **HIGH** — stdlib-only, already in use from Phase 1
- Architecture: **HIGH** — CONTEXT locks public API, this research fills implementation details
- MIME matrix test table: **HIGH** — derived directly from CONTEXT's 3×3 matrix specification
- Validate error catalog: **HIGH** — every rule sourced from CONTEXT's validation section
- canonicalizeMIME edge cases: **HIGH** — pseudocode in CONTEXT, edge cases enumerated by hand
- PERS-03 test recipe: **HIGH** — pattern is well-known, Linux-CI assumption documented
- File layout: **MEDIUM-HIGH** — recommendation is opinion-based but mirrors `internal/config` structure
- Open questions: **MEDIUM** — five items flagged for planner decision, each with a clear recommendation

**Research date:** 2026-04-09
**Valid until:** 2026-05-09 (30 days — this phase touches only stdlib, no fast-moving ecosystem)
