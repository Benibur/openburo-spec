# Phase 2: Registry Core - Context

**Gathered:** 2026-04-10
**Status:** Ready for planning

<domain>
## Phase Boundary

Implement `internal/registry`: the pure domain core that owns manifest validation, thread-safe in-memory state, atomic file persistence, symmetric 3×3 MIME wildcard matching, capability aggregation with sort by `appName`, and deterministic ordering. Zero dependencies on transport (no HTTP, no WebSocket, no auth, no broadcasting). The HTTP handler layer in Phase 4 will wrap this package; Phase 2 must be independently testable via `go test ./internal/registry -race`.

</domain>

<decisions>
## Implementation Decisions

### Capability response shape — USER DECISION (2026-04-10)

The user confirmed the `GET /api/v1/capabilities` response entry shape. Every capability entry is **denormalized** with the owning manifest's `appId` and `appName` copied in, plus the capability's own `action`, `path`, and `properties`:

```json
{
  "capabilities": [
    {
      "appId": "mail-app",
      "appName": "Mail",
      "action": "PICK",
      "path": "/pick",
      "properties": {
        "mimeTypes": ["*/*"]
      }
    },
    {
      "appId": "mail-app",
      "appName": "Mail",
      "action": "SAVE",
      "path": "/save",
      "properties": {
        "mimeTypes": ["*/*"]
      }
    }
  ],
  "count": 2
}
```

- Denormalization (copying `appName` into every entry) is deliberate: clients doing `state.capabilities = response.capabilities` don't need a second lookup to display the app name. This matches the denormalization philosophy from FEATURES.md.
- Entry shape is **exactly** `{ appId, appName, action, path, properties: { mimeTypes: [...] } }`. No extra fields in v1 (no `match_score`, no `manifest_version`, no `url`). Phase 4's HTTP handler produces this JSON from `registry.Capability` values.
- **The internal Go type** for the response element should be its own struct (e.g. `registry.CapabilityView`) distinct from the `Manifest.Capabilities[i]` struct, because the view carries denormalized fields. The planner picks the exact name.

### Capability sort order — USER DECISION (2026-04-10)

**Primary sort: `appName`** (user-specified).

**Tiebreakers (Claude's discretion, locked):**
1. `appName` — lowercased byte comparison (avoids locale-dependent collation; a reference impl must produce identical output across machines)
2. `appId` — lexical byte comparison (stable tiebreaker when two apps share a display name)
3. `action` — lexical (PICK comes before SAVE alphabetically; this is fine)
4. `path` — lexical (stable tiebreaker when an app declares multiple capabilities for the same action)

Implementation: `sort.SliceStable` with a comparator applying the four keys in order. Result is fully deterministic across rebuilds, restarts, and platforms.

**REQUIREMENTS.md update:** CAP-02 currently says "sorted by (appId, action, mimeType[0])". That is now stale — update to "sorted by (appName, appId, action, path) lowercased".

### Manifest validation (Claude's discretion)

The planner should implement `Manifest.Validate()` returning a wrapped error per first-failing field. Rules:

**Top-level fields:**
- `id`: required, non-empty, ASCII-only, pattern `^[a-zA-Z0-9][a-zA-Z0-9._-]*$` (32 chars typical, 128 max). Rejects whitespace, slashes, URL-unsafe characters. Rationale: `appId` appears in URL paths (`DELETE /api/v1/registry/{appId}`) so it must be URL-safe.
- `name`: required, non-empty after `strings.TrimSpace`, max 200 chars, any Unicode allowed (a reference impl should not gatekeep display names).
- `url`: required, parses via `url.Parse`, must have `Scheme` in {"http", "https"} and non-empty `Host`. Rejects `javascript:`, `data:`, relative URLs, `file:`.
- `version`: required, non-empty, max 64 chars, any printable ASCII. **NOT** enforcing semver strictness — apps use whatever scheme they want (semver, calver, git SHA).
- `capabilities`: required, non-empty slice.

**Capability entries (`Manifest.Capabilities[i]`):**
- `action`: required, **exactly** `"PICK"` or `"SAVE"` (case-sensitive; `pick` or `Pick` is rejected). Rationale: enum values are machine-readable constants.
- `path`: required, non-empty, max 500 chars. May be either (a) a relative path starting with `/`, resolved by the client against `Manifest.URL`, or (b) an absolute `http` / `https` URL with a non-empty host, for providers whose capability endpoints live on a different host than their manifest URL (e.g. a manifest hosted on `https://app.example` whose PICK capability sits on `https://storage.example/browse`). Any other value — bare words, non-http schemes, URLs with empty host — is rejected.
- `properties.mimeTypes`: required, non-empty slice. Each entry must match the loose pattern `type/subtype` with `*` allowed on either side (see MIME canonicalization below). Entries are **canonicalized** (lowercased, parameters stripped) during Validate — the stored form is already canonical so matching is a pure comparison.

**Unknown fields in JSON input:** use `json.Decoder.DisallowUnknownFields()` in the HTTP handler (Phase 4 concern) so clients learn the exact shape. Phase 2's `Manifest` struct has explicit `json:"..."` tags so the decoder rejects misspelled fields.

**Validation error shape:** `validate: manifest.id is required` / `validate: capability[2].action must be "PICK" or "SAVE" (got "pick")` — field path + problem, machine-friendly.

### MIME canonicalization and matching (Claude's discretion)

**Canonicalization** (applied during `Manifest.Validate` and when parsing the `?mimeType=` query filter):

```go
func canonicalizeMIME(s string) (string, error) {
    s = strings.TrimSpace(s)
    if s == "" {
        return "", errors.New("mime type is empty")
    }
    // Strip parameters — "text/plain; charset=utf-8" → "text/plain"
    if i := strings.Index(s, ";"); i >= 0 {
        s = strings.TrimSpace(s[:i])
    }
    s = strings.ToLower(s)
    // Must have exactly one "/"
    parts := strings.SplitN(s, "/", 2)
    if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
        return "", fmt.Errorf("mime type %q is not in type/subtype form", s)
    }
    return s, nil
}
```

- Parameters (`; charset=...`) are stripped and discarded. `text/plain; charset=utf-8` canonicalizes to `text/plain`.
- Case is normalized to lowercase (RFC 2045 says MIME types are case-insensitive).
- Structured suffixes (`application/vnd.api+json`) are kept as-is. Matching is literal against the full subtype string — no special handling for `+json`.
- Malformed forms (`*/subtype` — wildcard-only type, `/subtype` — empty type, `type/` — empty subtype, no slash) are **rejected** at canonicalization time.
- Valid wildcard forms: `*/*`, `type/*`. `*/subtype` is NOT valid (disallowed — semantically unclear and not supported by Android/Cozy).

**Symmetric wildcard matching** (the phase's biggest correctness concern):

```go
// mimeMatch reports whether a capability MIME type matches a query MIME type.
// Both inputs are assumed canonicalized (lowercased, no parameters).
// Matching is symmetric: matching(cap, q) == matching(q, cap).
// The 3×3 matrix of combinations:
//
//   cap \ q   | exact (image/png) | type/*  (image/*) | */* (any)
//   ----------|-------------------|-------------------|----------
//   exact     | bytewise equal    | type matches      | always
//   type/*    | type matches      | type matches      | always
//   */*       | always            | always            | always
//
func mimeMatch(cap, q string) bool { ... }
```

The phase MUST ship an exhaustive **table-driven** test covering all 9 cells of the 3×3 matrix plus negative cases (different types, different subtypes, malformed on either side) — this is called out as a critical requirement in VALIDATION.md Wave 0.

### Registry persistence (Claude's discretion)

**File format:** JSON, indented 2 spaces, with a stable top-level shape:

```json
{
  "version": 1,
  "manifests": [
    { /* Manifest 1 in deterministic order */ },
    { /* Manifest 2 */ }
  ]
}
```

- `version: 1` is a format version so v2 migration has a signal. No schema versioning logic in v1 — just read the int and fail fast if it's unexpected.
- `manifests` is an **array sorted by `id`** so diffs are stable across saves (operator-friendly even though it's not meant to be hand-edited).
- Indented output (`json.MarshalIndent(..., "", "  ")`) because operators will inspect the file during debugging.

**Atomic write pattern** (every mutation):

```go
func (s *Store) persistLocked() error {
    tmp, err := os.CreateTemp(filepath.Dir(s.path), filepath.Base(s.path)+".tmp-*")
    if err != nil {
        return fmt.Errorf("create temp file: %w", err)
    }
    tmpPath := tmp.Name()
    // Cleanup on any failure path
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
    // Directory fsync so the rename itself is durable
    if dir, err := os.Open(filepath.Dir(s.path)); err == nil {
        _ = dir.Sync()
        _ = dir.Close()
    }
    return nil
}
```

Four durability guarantees (all from PITFALLS #5):
1. Temp file is in the **same directory** as the target (so `os.Rename` is atomic on POSIX — not a cross-filesystem move)
2. `tmp.Sync()` flushes file contents before rename
3. `os.Rename` atomically replaces
4. Parent directory fsync so the rename is durable across crashes

**In-memory rollback on persist failure** (PITFALLS #5):

Every mutation method (`Upsert`, `Delete`) follows this pattern:

```go
func (s *Store) Upsert(m Manifest) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    if err := m.Validate(); err != nil {
        return err
    }

    // Snapshot
    prev, existed := s.manifests[m.ID]

    // Mutate
    s.manifests[m.ID] = m

    // Persist
    if err := s.persistLocked(); err != nil {
        // Rollback
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

- Mutex is held through the whole sequence (mutate → persist → rollback-on-failure) so no other goroutine can observe an inconsistent state.
- The explicit "registry unchanged" phrase in the error message tells operators the mutation was rolled back.
- **Test**: unit test uses `t.TempDir()`, then `os.Chmod(dir, 0o500)` to make it unwritable mid-test and asserts that `Store.Upsert` returns an error AND `Store.Get(id)` returns the **pre-mutation** manifest. This is the PERS-03 must-have.

### Load-at-startup behavior (Claude's discretion)

- `registry.Open(path)` or `registry.NewStore(path)` (planner picks the name) is the constructor. Behavior:
  - File missing → empty store, no error. Operators running the binary for the first time don't need to pre-create the file.
  - File present and parses → populated store, no error.
  - File present and malformed JSON → **fail fast** with a clear error pointing to the file path: `load registry from /path/to/registry.json: invalid JSON at offset N: ...`. No auto-quarantine, no auto-empty. Reference impl: data loss must not be silent.
  - File present but `version` field is unexpected (not `1`) → fail fast with `unsupported registry format version N at /path/...; expected 1`.
  - File present but a manifest fails `Validate` → fail fast with the offending appId and reason. This catches the case where operators hand-edit the file and break it.
- No automatic backup creation. Operators are expected to use filesystem snapshots / git / whatever. The reference impl does not take on backup-management responsibility.

### Manifest domain type (Claude's discretion — concrete shape)

```go
// Manifest is the full application description as registered by an admin.
// All fields are required; Validate() enforces this.
type Manifest struct {
    ID           string       `json:"id"`
    Name         string       `json:"name"`
    URL          string       `json:"url"`
    Version      string       `json:"version"`
    Capabilities []Capability `json:"capabilities"`
}

// Capability is a single (action, path, mimeTypes) tuple declared by a
// Manifest. Capability objects live inside a Manifest and carry no
// back-reference to it — the Store provides CapabilityView for flattened
// query results.
type Capability struct {
    Action     string             `json:"action"` // "PICK" | "SAVE"
    Path       string             `json:"path"`
    Properties CapabilityProps    `json:"properties"`
}

type CapabilityProps struct {
    MimeTypes []string `json:"mimeTypes"`
}

// CapabilityView is the denormalized flattened form returned by
// Store.Capabilities(filter). Includes the owning app's id + name so
// clients render results without a second lookup.
type CapabilityView struct {
    AppID      string          `json:"appId"`
    AppName    string          `json:"appName"`
    Action     string          `json:"action"`
    Path       string          `json:"path"`
    Properties CapabilityProps `json:"properties"`
}

// CapabilityFilter narrows Store.Capabilities() results. Empty values
// mean "no filter".
type CapabilityFilter struct {
    Action   string // "PICK" | "SAVE" | ""
    MimeType string // canonicalized MIME string or ""
}
```

- `CapabilityView` is distinct from `Capability` because the view carries denormalized `AppID`/`AppName`. The planner may name it differently but the distinction is load-bearing.
- All JSON tags are lowercase camelCase matching the wire format from PROJECT.md.
- No pointers in the field types — value semantics so cloning for snapshots is a simple struct copy.

### Store API (Claude's discretion — concrete shape)

```go
// Store is the thread-safe in-memory manifest registry with atomic
// JSON-file persistence. All mutation methods persist before returning;
// reads are served from memory under an RWMutex.
type Store struct {
    mu        sync.RWMutex
    manifests map[string]Manifest // keyed by Manifest.ID
    path      string              // registry.json location
}

// NewStore loads registry.json from path into memory. Missing file is
// not an error (yields an empty store). Malformed file is an error.
func NewStore(path string) (*Store, error)

// Upsert creates the manifest if absent, fully replaces if present.
// Persists to disk before returning. On persist failure, in-memory
// state is rolled back to pre-mutation.
func (s *Store) Upsert(m Manifest) error

// Delete removes a manifest by id. Returns (existed bool, error).
// existed==false + err==nil means the id wasn't in the registry;
// callers may choose to translate this into 404.
func (s *Store) Delete(id string) (existed bool, err error)

// Get returns a copy of the manifest with the given id. The second
// return is false if the id is not registered.
func (s *Store) Get(id string) (Manifest, bool)

// List returns all manifests, sorted by id. Caller gets a copy;
// mutating the returned slice does not affect the Store.
func (s *Store) List() []Manifest

// Capabilities returns all capabilities across all manifests as
// CapabilityView entries, filtered and sorted per Phase 2 rules.
// Filter is applied before sort. Empty filter returns everything.
func (s *Store) Capabilities(filter CapabilityFilter) []CapabilityView
```

- Every read method returns **copies**, not pointers into the map — so callers can't mutate `Store` state through the returned values. Go's value semantics on struct assignment + slice copy (`append([]Manifest(nil), s.manifests...)`) covers this cheaply.
- `Delete` returns `(bool, error)` rather than `error` so the handler can map `!existed && err == nil` to `404 Not Found` without re-querying.
- `Upsert` does NOT return a "created vs updated" bool. The handler at Phase 4 can distinguish by calling `Get(id)` first, or by checking if `id` was present before the call. Simpler Store API wins.

### Claude's Discretion (for the planner)

- Exact file names inside `internal/registry/`. Suggested: `manifest.go` (Manifest + Capability + Validate), `mime.go` (canonicalizeMIME + mimeMatch + tests), `store.go` (Store + NewStore + mutation + query methods), `persist.go` (persistLocked + atomic-write + loadFromFile), plus one _test.go per production file.
- Whether to use `map[string]Manifest` or `sync.Map` — recommend plain map under RWMutex (simpler, atomic rollback is easier).
- Pre-canonicalization of capability MIME types at load time vs on-the-fly at match time — recommend **at load** (Validate() canonicalizes in place). Cheaper per query.
- Test fixture filenames — pick descriptive kebab-case or table-driven inline; VALIDATION.md has a footnote declaring fixture naming illustrative.

</decisions>

<specifics>
## Specific Ideas

- **The symmetric 3×3 MIME matrix test is the single most important test in Phase 2.** The exhaustive table must cover all 9 combinations of `{exact, type/*, */*}` on both capability and query sides, plus negative cases. Cozy Stack's resolver is known to get this wrong — OpenBuro gets it right as a differentiator (FEATURES.md).
- **Atomic persistence test.** The "unwritable directory" test (PERS-03) is the one that catches the most bugs. Should use `t.TempDir()` + `os.Chmod(..., 0o500)` and assert the in-memory state is identical after a failed mutation.
- **Deterministic output is load-bearing.** `List()` sorted by `id`, `Capabilities()` sorted by `(appName, appId, action, path)`, `persistLocked()` writes manifests sorted by `id` — all three are different sort keys but each is deterministic in its own view, so diffs of registry.json and test golden files are stable.
- **No request logging in the registry package.** The Store does NOT take a `*slog.Logger` parameter in v1. If an error path matters for observability, it's returned up the call stack and the HTTP handler (Phase 4) logs it with request context. Keeping the Store logger-free preserves its status as a pure domain core.

</specifics>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Contracts and constraints
- `.planning/PROJECT.md` — Core value, validated reqs (Phase 1), active reqs (Phase 2+)
- `.planning/REQUIREMENTS.md` §Registry, §Capabilities, §Persistence, §Testing — 20 REQ-IDs Phase 2 must close
- `.planning/ROADMAP.md` §"Phase 2: Registry Core" — goal + 5 success criteria
- `.planning/phases/01-foundation/01-CONTEXT.md` — prior locked decisions (Go 1.26, testify/require, table-driven+testdata, no `slog.Default()` in internal/)

### Research (critical for this phase)
- `.planning/research/STACK.md` — Go 1.26, stdlib encoding/json + sync, no new deps in Phase 2
- `.planning/research/ARCHITECTURE.md` §"internal/registry" — Store pattern, atomic persistence, no transport awareness
- `.planning/research/PITFALLS.md` §1 (ABBA deadlock — registry NEVER imports wshub), §2 (symmetric MIME matching), §5 (atomic persistence + in-memory rollback), §12 (defer Unlock convention)
- `.planning/research/FEATURES.md` §"MIME matching is the single biggest correctness concern" + §"Symmetric wildcard matching" — the full 3×3 matrix is specified here
- `.planning/research/SUMMARY.md` §"Phase 2a: Registry Core" — deliverables list

### Go idioms
- No external spec — idioms are captured in STACK.md and ARCHITECTURE.md

### Prior phase artifacts to mirror
- `internal/config/config.go` — shows the doc.go package-header pattern, Validate-returns-wrapped-error style, how to structure errors with operator-friendly messages
- `internal/config/config_test.go` — shows the table-driven test style with `testdata/` fixtures
- `.planning/phases/01-foundation/01-02-config-examples-SUMMARY.md` — what Plan 01-02 built that Phase 2 should feel consistent with

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`internal/config/config.go`** — establishes the Validate-returns-wrapped-error style and the `%q`-quoting for user-facing error messages. Phase 2's `Manifest.Validate` should mirror this style for consistency.
- **`internal/config/config_test.go`** — the table-driven + `testdata/` fixture pattern. Phase 2's tests should follow the same shape so a reader opening either file sees a consistent codebase.
- **`internal/registry/doc.go`** — package doc is already written by Plan 01-01. Phase 2 replaces or extends this file; the existing text is a good summary of intent and can be kept.

### Established Patterns
- **Error wrapping:** use `fmt.Errorf("context: %w", err)` with lowercase context strings and no trailing punctuation (Go convention).
- **Validation errors:** describe the field path and problem (`"manifest.id is required"`, `"capability[2].action must be one of [PICK SAVE]"`). Don't include the full value in the message unless it's short (truncate long URLs/strings).
- **Package doc at top of first file:** `internal/config/config.go` has the package-level `// Package config ...` comment. Phase 2's first file (e.g. `manifest.go` or `store.go`) should carry the canonical package doc replacing `doc.go`.
- **Testdata convention:** `internal/config/testdata/*.yaml` fixtures. Phase 2 uses `internal/registry/testdata/*.json` for manifest fixtures and may use inline Go literals for the symmetric MIME matrix (table-driven, no fixture file needed).

### Integration Points
- **Phase 4 (HTTP API)** will import `internal/registry` and call `Store.Upsert/Delete/Get/List/Capabilities`. Phase 2 must NOT import `internal/httpapi` (reverse dep). The Store signature above (returning `(existed bool, err error)` from Delete, etc.) is designed for easy mapping to HTTP status codes in Phase 4.
- **Phase 3 (WebSocket Hub)** runs in parallel with Phase 2 and has no integration point with it (hub speaks `[]byte`, not registry types). `internal/registry` MUST NOT import `internal/wshub` (enforced by PITFALLS #1). `go list -deps ./internal/registry | grep -E 'wshub|httpapi'` should produce no output at end of Phase 2.
- **Phase 5 (wiring)** will call `registry.NewStore(cfg.RegistryFile)` from `cmd/server/main.go`. Phase 2 must ensure NewStore tolerates missing file (greenfield) and corrupted file (fail fast).

</code_context>

<deferred>
## Deferred Ideas

- **Pluggable storage backend (SQLite/Postgres):** v2, per PROJECT.md. `v1` tag in the JSON format gives us a migration hook.
- **Optimistic concurrency / ETag / 409:** v2. Single-admin assumption for v1.
- **Manifest versioning inside the registry** (keeping old versions on upsert): v2. v1 is full-replace on upsert.
- **Backup management** (registry.json.bak rotation): out of scope. Operators use filesystem snapshots.
- **MIME structured-suffix decomposition** (`application/vnd.api+json` → also matches `application/json`): out of scope. Neither Android nor Cozy do this; OpenBuro matches literal subtype strings.
- **Capability wildcards beyond MIME** (e.g. path wildcards, action wildcards): out of scope. Action is enum; path is literal.
- **Registry-level logging** (audit log of mutations): Phase 4 does this in the HTTP handler layer. Store itself stays logger-free.
- **Update REQUIREMENTS.md CAP-02 to say "sorted by appName"** (currently says "sorted by appId"): **TODO for this phase's commit** — the planner's first task should update REQUIREMENTS.md in the same commit that lands Store.Capabilities.

</deferred>

---

*Phase: 02-registry-core*
*Context gathered: 2026-04-10*
