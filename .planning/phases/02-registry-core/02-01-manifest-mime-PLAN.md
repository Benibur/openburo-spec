---
phase: 02-registry-core
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/registry/doc.go
  - internal/registry/manifest.go
  - internal/registry/manifest_test.go
  - internal/registry/mime.go
  - internal/registry/mime_test.go
  - internal/registry/testdata/valid-two-apps.json
autonomous: true
requirements:
  - REG-01
  - REG-02
  - REG-03
  - CAP-05
  - TEST-01

must_haves:
  truths:
    - "Manifest.Validate rejects every one of the 19 documented invalid inputs with a field-path-prefixed error starting with 'validate:'"
    - "canonicalizeMIME lowercases, strips parameters, and rejects */subtype, double-slash, three-segment, and empty inputs"
    - "mimeMatch returns the correct truth value for every cell of the 9-cell 3x3 wildcard matrix AND is symmetric (mimeMatch(a,b) == mimeMatch(b,a))"
    - "Validate mutates the receiver in place so stored manifests carry already-canonical MIME strings (matcher is pure comparison)"
    - "go test ./internal/registry -race -count=1 passes for the Manifest + MIME layer after this plan"
  artifacts:
    - path: "internal/registry/manifest.go"
      provides: "Manifest, Capability, CapabilityProps types + Manifest.Validate() + package doc"
      contains: "type Manifest struct"
    - path: "internal/registry/manifest_test.go"
      provides: "TestManifestValidate table-driven with 19 error cases + happy path"
      contains: "func TestManifestValidate"
    - path: "internal/registry/mime.go"
      provides: "canonicalizeMIME + mimeMatch + exported CanonicalizeMIME wrapper for Phase 4"
      contains: "func mimeMatch"
    - path: "internal/registry/mime_test.go"
      provides: "TestMimeMatch (full 3x3 matrix + symmetry) + TestCanonicalizeMIME (edge cases)"
      contains: "func TestMimeMatch"
  key_links:
    - from: "internal/registry/manifest.go Validate()"
      to: "internal/registry/mime.go canonicalizeMIME()"
      via: "for each capability, for each mimeType, canonicalize in place"
      pattern: "canonicalizeMIME"
    - from: "internal/registry/mime_test.go"
      to: "internal/registry/mime.go mimeMatch"
      via: "17-case table-driven test asserts value AND symmetry"
      pattern: "mimeMatch\\(tc"
---

<objective>
Ship the pure, stateless layer of `internal/registry`: the `Manifest` domain type with a complete `Validate()` implementation, plus `canonicalizeMIME` and the correctness-critical symmetric `mimeMatch` — proven by exhaustive table-driven tests. After this plan, the hardest correctness problem in Phase 2 (symmetric 3x3 MIME matching, PITFALLS #2) is solved and frozen by tests before any Store state ever exists.

Purpose: Solve the phase's two biggest correctness risks (validation coverage + symmetric MIME) with no I/O, no mutation, no concurrency — so Plans 02-02 and 02-03 can build on a proven foundation.

Output: `manifest.go`, `mime.go`, and their test siblings. The package doc previously in `doc.go` is moved to the top of `manifest.go` (the new "face" of the domain) and `doc.go` is removed.
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
@internal/config/config.go
@internal/config/config_test.go
@internal/registry/doc.go

<interfaces>
<!-- Canonical type shapes from CONTEXT.md. Paste VERBATIM into manifest.go. -->

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

// Capability is a single (action, path, mimeTypes) tuple declared by a Manifest.
type Capability struct {
    Action     string          `json:"action"` // "PICK" | "SAVE"
    Path       string          `json:"path"`
    Properties CapabilityProps `json:"properties"`
}

type CapabilityProps struct {
    MimeTypes []string `json:"mimeTypes"`
}
```

<!-- Note: CapabilityView and CapabilityFilter live in store.go (Plan 02-02 / 02-03), not manifest.go.
     They are Store API types, not intrinsic to the Manifest domain. -->
</interfaces>

<locked_open_questions>
<!-- The 5 open questions from RESEARCH §"Open Questions" — all locked in this plan. -->
1. MimeTypes sort after canonicalization: **YES** — sort.Strings at end of Validate after canonicalizing in place. Eliminates last nondeterminism source. One line.
2. Reject trailing `;` in canonicalizer: **NO** — lenient. `text/plain;` canonicalizes to `text/plain`. Locked as accept case in TestCanonicalizeMIME.
3. Malformed filter MIME in Store.Capabilities: **empty result** + export `CanonicalizeMIME` wrapper for Phase 4. (Applied in Plan 02-03; the exported wrapper is created in THIS plan's mime.go so it exists early.)
4. NewStore mkdir missing parent: **NO** — surface operator error. (Applied in Plan 02-02.)
5. Delete of non-existent: **no-op**, return (false, nil) without disk write. (Applied in Plan 02-02.)
</locked_open_questions>

<canonicalize_bug_fixes>
<!-- Two bugs RESEARCH flagged in the CONTEXT canonicalizeMIME sketch. MUST be fixed in this plan. -->

**Bug 1: `image//png` and `image/png/extra` pass SplitN.**
`strings.SplitN("image//png", "/", 2)` returns `["image", "/png"]` — both parts non-empty, so the CONTEXT sketch would incorrectly accept this. Fix: after SplitN, additionally reject if `strings.Contains(parts[1], "/")`.

**Bug 2: `*/subtype` passes SplitN.**
`strings.SplitN("*/png", "/", 2)` returns `["*", "png"]` — both parts non-empty, would incorrectly accept. Fix: explicitly reject when `parts[0] == "*" && parts[1] != "*"` (wildcard type with concrete subtype is semantically unclear; per CONTEXT only `*/*` and `type/*` and `type/subtype` are valid).
</canonicalize_bug_fixes>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: MIME canonicalization + symmetric 3x3 matching with exhaustive test</name>
  <files>internal/registry/mime.go, internal/registry/mime_test.go</files>
  <read_first>
- internal/registry/doc.go (to know what the package doc currently says; it moves to manifest.go in Task 2 so Task 1 should NOT touch the package-level doc)
- .planning/phases/02-registry-core/02-CONTEXT.md §"MIME canonicalization and matching" (pseudocode + semantics)
- .planning/phases/02-registry-core/02-RESEARCH.md §"Example 1: Symmetric MIME 3x3 Matrix" AND §"Example 2: canonicalizeMIME Edge-Case Table" (copy-paste tables)
- .planning/phases/02-registry-core/02-RESEARCH.md §"Note on 'double slash' and 'three segments'" (the two canonicalizer bugs)
- .planning/phases/02-registry-core/02-VALIDATION.md (rows CAP-04, CAP-05, REG-03)
- internal/config/config.go (error wrapping + `%q` quoting style to mirror)
  </read_first>
  <behavior>
**TestMimeMatch** (17 cases minimum, all pass; symmetry asserted for every case):
- 9 positive cells of the 3x3 matrix: exact-exact-same, exact vs type/* (same type), exact vs */*, type/* vs exact (same type), type/* vs type/* (same type), type/* vs */*, */* vs exact, */* vs type/*, */* vs */*
- 8 negative cases: exact vs exact (different type), exact vs exact (different family), exact vs type/* (different type), type/* vs exact (different type), type/* vs type/* (different type), subtype prefix boundary, subtype superstring boundary, type prefix boundary
- Every test case asserts BOTH `mimeMatch(cap, q) == want` AND `mimeMatch(q, cap) == want` (symmetry)

**TestCanonicalizeMIME** (≥20 cases): normal cases (exact, type wildcard, full wildcard, uppercase, mixed case, structured suffix kept), whitespace handling (leading/trailing/both), parameter stripping (charset, boundary, multiple params, trailing semicolon, uppercase with param, param then whitespace), and rejection cases (empty, whitespace-only, no slash, just slash, double slash, empty type, empty subtype, `*/subtype`, three segments).
  </behavior>
  <action>
**RED phase: write mime_test.go first.**

Create `internal/registry/mime_test.go` with the two table-driven tests below. Paste VERBATIM; do not paraphrase — this is the spec.

```go
package registry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMimeMatch(t *testing.T) {
	tests := []struct {
		name string
		cap  string
		q    string
		want bool
	}{
		// --- 9 positive cells of the 3x3 matrix ---
		{"exact vs exact (same)", "image/png", "image/png", true},
		{"exact vs type/* (same type)", "image/png", "image/*", true},
		{"exact vs */*", "image/png", "*/*", true},
		{"type/* vs exact (same type)", "image/*", "image/png", true},
		{"type/* vs type/* (same type)", "image/*", "image/*", true},
		{"type/* vs */*", "image/*", "*/*", true},
		{"*/* vs exact", "*/*", "image/png", true},
		{"*/* vs type/*", "*/*", "image/*", true},
		{"*/* vs */*", "*/*", "*/*", true},

		// --- negative: different exact types ---
		{"exact vs exact (different type)", "image/png", "image/jpeg", false},
		{"exact vs exact (different family)", "image/png", "text/plain", false},

		// --- negative: exact vs type/* with different type ---
		{"exact vs type/* (different type)", "image/png", "text/*", false},
		{"type/* vs exact (different type)", "image/*", "text/plain", false},

		// --- negative: type/* vs type/* with different type ---
		{"type/* vs type/* (different type)", "image/*", "text/*", false},

		// --- subtype boundary cases (avoid substring bugs) ---
		{"exact vs exact (subtype prefix)", "image/pn", "image/png", false},
		{"exact vs exact (subtype superstring)", "image/png", "image/pngx", false},
		{"exact vs exact (type prefix)", "imag/png", "image/png", false},
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

func TestCanonicalizeMIME(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		// --- normal cases ---
		{"simple exact", "image/png", "image/png", false},
		{"type wildcard", "image/*", "image/*", false},
		{"full wildcard", "*/*", "*/*", false},
		{"uppercase exact", "IMAGE/PNG", "image/png", false},
		{"mixed case", "Image/Png", "image/png", false},
		{"structured suffix kept", "application/vnd.api+json", "application/vnd.api+json", false},

		// --- whitespace handling ---
		{"leading whitespace", "  image/png", "image/png", false},
		{"trailing whitespace", "image/png  ", "image/png", false},
		{"both sides whitespace", "  image/png  ", "image/png", false},

		// --- parameter stripping ---
		{"with charset param", "text/plain; charset=utf-8", "text/plain", false},
		{"with boundary param", "multipart/form-data; boundary=xyz", "multipart/form-data", false},
		{"multiple params", "text/plain; charset=utf-8; format=flowed", "text/plain", false},
		{"trailing semicolon (accepted, lenient)", "text/plain;", "text/plain", false},
		{"uppercase with param", "TEXT/PLAIN; CHARSET=UTF-8", "text/plain", false},
		{"param then whitespace", "text/plain ; charset=utf-8", "text/plain", false},

		// --- rejection cases ---
		{"empty string", "", "", true},
		{"whitespace only", "   ", "", true},
		{"no slash", "image", "", true},
		{"just slash", "/", "", true},
		{"double slash", "image//png", "", true},
		{"empty type", "/png", "", true},
		{"empty subtype", "image/", "", true},
		{"wildcard type with concrete subtype", "*/subtype", "", true},
		{"three segments", "image/png/extra", "", true},
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

// TestCanonicalizeMIME_Exported verifies the exported wrapper Phase 4 will use
// to validate ?mimeType= query parameters before calling Store.Capabilities.
func TestCanonicalizeMIME_Exported(t *testing.T) {
	got, err := CanonicalizeMIME("IMAGE/PNG; charset=utf-8")
	require.NoError(t, err)
	require.Equal(t, "image/png", got)

	_, err = CanonicalizeMIME("*/subtype")
	require.Error(t, err)
}
```

Run `go test ./internal/registry -run TestMimeMatch -count=1` — it MUST fail to compile (mimeMatch and canonicalizeMIME don't exist yet). This is the RED state.

**GREEN phase: implement mime.go to pass the tests.**

Create `internal/registry/mime.go`:

```go
package registry

import (
	"errors"
	"fmt"
	"strings"
)

// canonicalizeMIME normalizes a MIME type string to the canonical form used
// for storage and matching: lowercased, whitespace trimmed, parameters
// stripped, and validated to be one of the supported shapes:
//
//   - type/subtype (e.g. "image/png")
//   - type/*       (e.g. "image/*")
//   - */*          (any)
//
// Invalid shapes (empty, no slash, "*/subtype", double-slash, three-segment,
// empty type or subtype) are rejected with a descriptive error.
func canonicalizeMIME(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("mime type is empty")
	}
	// Strip parameters (anything after the first ";").
	if i := strings.Index(s, ";"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if s == "" {
		return "", errors.New("mime type is empty after stripping parameters")
	}
	s = strings.ToLower(s)
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("mime type %q is not in type/subtype form", s)
	}
	// Bug fix 1: reject "image//png", "image/png/extra" (subtype must not contain "/").
	if strings.Contains(parts[1], "/") {
		return "", fmt.Errorf("mime type %q has invalid subtype (contains \"/\")", s)
	}
	// Bug fix 2: reject "*/subtype" (wildcard type with concrete subtype is unsupported).
	if parts[0] == "*" && parts[1] != "*" {
		return "", fmt.Errorf("mime type %q: wildcard type with concrete subtype is not supported", s)
	}
	return s, nil
}

// CanonicalizeMIME is the exported wrapper that Phase 4's HTTP handler uses
// to validate ?mimeType= query parameters before calling Store.Capabilities.
// Returns the canonical form or a descriptive error for malformed input.
func CanonicalizeMIME(s string) (string, error) {
	return canonicalizeMIME(s)
}

// mimeMatch reports whether a capability MIME type matches a query MIME type.
// Both inputs MUST already be canonicalized (lowercased, no parameters, validated
// via canonicalizeMIME). Matching is symmetric: mimeMatch(a, b) == mimeMatch(b, a).
//
// The 3x3 matrix of wildcard combinations:
//
//	cap \ q   | exact (image/png) | type/*  (image/*) | */* (any)
//	----------|-------------------|-------------------|----------
//	exact     | bytewise equal    | type matches      | always
//	type/*    | type matches      | type matches      | always
//	*/*       | always            | always            | always
func mimeMatch(cap, q string) bool {
	// Rule 1: */* on either side matches anything.
	if cap == "*/*" || q == "*/*" {
		return true
	}
	// Split both sides; assume canonicalized (no error path).
	capType, capSub, capOK := strings.Cut(cap, "/")
	qType, qSub, qOK := strings.Cut(q, "/")
	if !capOK || !qOK {
		return false // defensive; canonicalized inputs always have "/"
	}
	// Rule 2: types must be equal (neither side can be "*" here, because
	// "*/*" is already handled and "*/subtype" is rejected at canonicalization).
	if capType != qType {
		return false
	}
	// Rule 3: at least one side must be subtype wildcard OR subtypes equal.
	if capSub == "*" || qSub == "*" {
		return true
	}
	return capSub == qSub
}
```

Run `go test ./internal/registry -run 'TestMimeMatch|TestCanonicalizeMIME' -count=1 -v`. All 17 mimeMatch cases and 25 canonicalizeMIME cases MUST pass.
  </action>
  <verify>
<automated>go test ./internal/registry -run 'TestMimeMatch|TestCanonicalizeMIME' -count=1 -v && go vet ./internal/registry && gofmt -l internal/registry/mime.go internal/registry/mime_test.go</automated>
  </verify>
  <done>
- `internal/registry/mime.go` exists, exports `CanonicalizeMIME`, has unexported `canonicalizeMIME` and `mimeMatch`
- `internal/registry/mime_test.go` exists with `TestMimeMatch` (17+ cases, symmetry asserted on every case), `TestCanonicalizeMIME` (25+ cases), and `TestCanonicalizeMIME_Exported`
- `go test ./internal/registry -run 'TestMimeMatch|TestCanonicalizeMIME' -count=1` exits 0
- `gofmt -l internal/registry/mime.go internal/registry/mime_test.go` produces no output
- `go vet ./internal/registry` exits 0
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Manifest domain types + Validate() with 19-case error catalog</name>
  <files>internal/registry/manifest.go, internal/registry/manifest_test.go, internal/registry/testdata/valid-two-apps.json, internal/registry/doc.go</files>
  <read_first>
- internal/registry/doc.go (to copy its package doc prose into manifest.go before deleting it)
- internal/registry/mime.go (from Task 1 — Validate calls canonicalizeMIME)
- .planning/phases/02-registry-core/02-CONTEXT.md §"Manifest validation" (full rules)
- .planning/phases/02-registry-core/02-CONTEXT.md §"Manifest domain type" (struct shape — MUST paste verbatim)
- .planning/phases/02-registry-core/02-RESEARCH.md §"Example 3: Manifest.Validate() Error Catalog" (19-row table + skeleton)
- .planning/phases/02-registry-core/02-RESEARCH.md §"Open Question 1" (sort MimeTypes at end of Validate)
- .planning/phases/02-registry-core/02-VALIDATION.md (rows REG-01, REG-02, REG-03, TEST-01)
- internal/config/config.go (Validate style — wrapped errors, lowercase messages, `%q` for user input)
- internal/config/config_test.go (table-driven style with testdata)
  </read_first>
  <behavior>
**TestManifestValidate** (table-driven, 19+ error cases + happy path):
- Happy path: a fully valid manifest returns nil AND has its MIME types canonicalized in place (e.g. `IMAGE/PNG; charset=utf-8` becomes `image/png` in the receiver) AND sorted alphabetically
- 19 error cases, each asserting `require.Contains(err.Error(), "<field path substring>")`:
  1. empty id → "manifest.id is required"
  2. id pattern mismatch (e.g. "has space") → "does not match pattern"
  3. id too long (>128) → "manifest.id too long"
  4. empty name (also whitespace-only) → "manifest.name is required"
  5. name too long (>200) → "manifest.name too long"
  6. empty url → "manifest.url is required"
  7. url parse error (e.g. "://bad") → "manifest.url is invalid"
  8. url scheme not http/https (e.g. "javascript:alert(1)") → "manifest.url scheme must be http or https"
  9. url empty host (e.g. "https://") → "manifest.url has empty host"
  10. empty version → "manifest.version is required"
  11. version too long (>64) → "manifest.version too long"
  12. empty capabilities slice → "manifest.capabilities must be non-empty"
  13. empty action → `capability[0].action`
  14. action "pick" (wrong case) → `capability[0].action must be \"PICK\" or \"SAVE\", got \"pick\"`
  15. empty path → `capability[0].path is required`
  16. path that is neither `/`-relative nor an absolute http(s) URL (e.g. bare word `pick`, `ftp://...`, `https://` with empty host) → `capability[0].path must start with \"/\" or be an absolute http(s) URL`
  16b. absolute http(s) URL paths accepted — covered by a dedicated positive test `TestManifestValidate_AbsolutePathAccepted`
  17. path too long (>500) → `capability[0].path too long`
  18. empty mimeTypes slice → `capability[0].properties.mimeTypes must be non-empty`
  19. mimeType canonicalize failure (e.g. "image") → `capability[0].properties.mimeTypes[0]`

Plus a separate test `TestManifestValidate_CanonicalizesAndSorts` asserting that after a successful Validate, MimeTypes are lowercased, param-stripped, and sorted alphabetically.
  </behavior>
  <action>
**Step 1: write manifest_test.go first (RED phase).**

Create `internal/registry/manifest_test.go` with table-driven tests. Error table entries use `errSubstring string` to assert via `require.Contains`. Make the happy-path test validate a `Manifest` whose MimeTypes are deliberately uppercase + param-bearing + out-of-order, and assert they come back canonical + sorted.

Key structure:

```go
package registry

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func validManifest() Manifest {
	return Manifest{
		ID:      "mail-app",
		Name:    "Mail",
		URL:     "https://example.com",
		Version: "1.0.0",
		Capabilities: []Capability{
			{
				Action: "PICK",
				Path:   "/pick",
				Properties: CapabilityProps{MimeTypes: []string{"text/plain", "image/png"}},
			},
		},
	}
}

func TestManifestValidate_Happy(t *testing.T) {
	m := validManifest()
	require.NoError(t, m.Validate())
	require.Equal(t, []string{"image/png", "text/plain"}, m.Capabilities[0].Properties.MimeTypes,
		"MimeTypes should be sorted alphabetically after Validate")
}

func TestManifestValidate_CanonicalizesInPlace(t *testing.T) {
	m := validManifest()
	m.Capabilities[0].Properties.MimeTypes = []string{"TEXT/PLAIN; charset=utf-8", "IMAGE/PNG"}
	require.NoError(t, m.Validate())
	// Canonical: lowercased, params stripped, sorted alphabetically.
	require.Equal(t, []string{"image/png", "text/plain"}, m.Capabilities[0].Properties.MimeTypes)
}

func TestManifestValidate_Errors(t *testing.T) {
	mutate := func(fn func(*Manifest)) Manifest {
		m := validManifest()
		fn(&m)
		return m
	}
	longID := strings.Repeat("a", 129)
	longName := strings.Repeat("n", 201)
	longVersion := strings.Repeat("v", 65)
	longPath := "/" + strings.Repeat("p", 500)

	tests := []struct {
		name         string
		m            Manifest
		errSubstring string
	}{
		{"empty id", mutate(func(m *Manifest) { m.ID = "" }), "manifest.id is required"},
		{"id has space", mutate(func(m *Manifest) { m.ID = "has space" }), "does not match pattern"},
		{"id too long", mutate(func(m *Manifest) { m.ID = longID }), "manifest.id too long"},
		{"empty name", mutate(func(m *Manifest) { m.Name = "" }), "manifest.name is required"},
		{"whitespace-only name", mutate(func(m *Manifest) { m.Name = "   " }), "manifest.name is required"},
		{"name too long", mutate(func(m *Manifest) { m.Name = longName }), "manifest.name too long"},
		{"empty url", mutate(func(m *Manifest) { m.URL = "" }), "manifest.url is required"},
		{"url parse fails", mutate(func(m *Manifest) { m.URL = "://bad" }), "manifest.url is invalid"},
		{"javascript scheme", mutate(func(m *Manifest) { m.URL = "javascript:alert(1)" }), "manifest.url scheme must be http or https"},
		{"file scheme", mutate(func(m *Manifest) { m.URL = "file:///etc/passwd" }), "manifest.url scheme must be http or https"},
		{"empty host", mutate(func(m *Manifest) { m.URL = "https://" }), "manifest.url has empty host"},
		{"empty version", mutate(func(m *Manifest) { m.Version = "" }), "manifest.version is required"},
		{"version too long", mutate(func(m *Manifest) { m.Version = longVersion }), "manifest.version too long"},
		{"empty capabilities", mutate(func(m *Manifest) { m.Capabilities = nil }), "manifest.capabilities must be non-empty"},
		{"zero-length capabilities slice", mutate(func(m *Manifest) { m.Capabilities = []Capability{} }), "manifest.capabilities must be non-empty"},
		{"empty action", mutate(func(m *Manifest) { m.Capabilities[0].Action = "" }), "capability[0].action"},
		{"lowercase action rejected", mutate(func(m *Manifest) { m.Capabilities[0].Action = "pick" }), `capability[0].action must be "PICK" or "SAVE", got "pick"`},
		{"empty path", mutate(func(m *Manifest) { m.Capabilities[0].Path = "" }), "capability[0].path is required"},
		{"path is bare word rejected", mutate(func(m *Manifest) { m.Capabilities[0].Path = "pick" }), `capability[0].path must start with "/" or be an absolute http(s) URL`},
		{"path with non-http scheme rejected", mutate(func(m *Manifest) { m.Capabilities[0].Path = "ftp://example.com/pick" }), `capability[0].path must start with "/" or be an absolute http(s) URL`},
		{"path absolute URL missing host rejected", mutate(func(m *Manifest) { m.Capabilities[0].Path = "https://" }), `capability[0].path must start with "/" or be an absolute http(s) URL`},
		{"path too long", mutate(func(m *Manifest) { m.Capabilities[0].Path = longPath }), "capability[0].path too long"},
		{"empty mimeTypes", mutate(func(m *Manifest) { m.Capabilities[0].Properties.MimeTypes = nil }), "capability[0].properties.mimeTypes must be non-empty"},
		{"empty mimeTypes slice", mutate(func(m *Manifest) { m.Capabilities[0].Properties.MimeTypes = []string{} }), "capability[0].properties.mimeTypes must be non-empty"},
		{"invalid mimeType", mutate(func(m *Manifest) { m.Capabilities[0].Properties.MimeTypes = []string{"image"} }), "capability[0].properties.mimeTypes[0]"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.m.Validate()
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.errSubstring, "error: %v", err)
			require.Contains(t, err.Error(), "validate:", "error must be prefixed with 'validate:'")
		})
	}
}
```

Run `go test ./internal/registry -run TestManifestValidate -count=1` — MUST fail to compile (Manifest type and Validate method do not exist yet). RED state confirmed.

**Step 2: implement manifest.go (GREEN phase).**

Create `internal/registry/manifest.go` with the canonical package doc moved from doc.go, the three domain types, and a Validate() method implementing all 19+ rules in field-declaration order, mutating MimeTypes in place to canonical form and sorting them at the end of each capability.

```go
// Package registry holds the in-memory manifest store, domain types
// (Manifest, Capability), symmetric MIME wildcard matching, and atomic
// JSON persistence. It is the pure domain core and depends on nothing
// from other internal/ packages — the HTTP handler in Phase 4 is the
// sole wiring point between this package and transport concerns.
//
// The Store returns copies of stored manifests. Callers MUST NOT mutate
// returned values; the package does not deep-copy slice contents.
package registry

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// Manifest is the full application description as registered by an admin.
// All fields are required; Validate() enforces this and canonicalizes the
// MIME type strings inside each capability in place.
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
	Action     string          `json:"action"` // "PICK" | "SAVE"
	Path       string          `json:"path"`
	Properties CapabilityProps `json:"properties"`
}

// CapabilityProps is the "properties" sub-object of a Capability. Kept as
// its own type so future phases can add fields (e.g. size limits) without
// breaking the wire format.
type CapabilityProps struct {
	MimeTypes []string `json:"mimeTypes"`
}

const (
	manifestIDPatternStr = `^[a-zA-Z0-9][a-zA-Z0-9._-]*$`
	maxManifestIDLen     = 128
	maxManifestNameLen   = 200
	maxManifestVerLen    = 64
	maxCapabilityPathLen = 500
)

var manifestIDPattern = regexp.MustCompile(manifestIDPatternStr)

// Validate checks every field of the manifest per Phase 2 validation rules
// and returns the first encountered error prefixed with "validate: ". On
// success it MUTATES the receiver in place to canonicalize every
// capability's MIME type strings (lowercased, parameters stripped) and
// sorts them alphabetically within each capability so the file
// representation is byte-stable across re-upserts.
//
// Validate fails fast on the first error; callers see one problem at a
// time. This is intentional — multi-error accumulation is out of scope
// for v1.
func (m *Manifest) Validate() error {
	// id
	if m.ID == "" {
		return errors.New("validate: manifest.id is required")
	}
	if !manifestIDPattern.MatchString(m.ID) {
		return fmt.Errorf("validate: manifest.id %q does not match pattern %s", m.ID, manifestIDPatternStr)
	}
	if len(m.ID) > maxManifestIDLen {
		return fmt.Errorf("validate: manifest.id too long: %d chars (max %d)", len(m.ID), maxManifestIDLen)
	}

	// name
	name := strings.TrimSpace(m.Name)
	if name == "" {
		return errors.New("validate: manifest.name is required")
	}
	if len(m.Name) > maxManifestNameLen {
		return fmt.Errorf("validate: manifest.name too long: %d chars (max %d)", len(m.Name), maxManifestNameLen)
	}

	// url
	if m.URL == "" {
		return errors.New("validate: manifest.url is required")
	}
	u, err := url.Parse(m.URL)
	if err != nil {
		return fmt.Errorf("validate: manifest.url is invalid: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("validate: manifest.url scheme must be http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("validate: manifest.url has empty host")
	}

	// version
	if m.Version == "" {
		return errors.New("validate: manifest.version is required")
	}
	if len(m.Version) > maxManifestVerLen {
		return fmt.Errorf("validate: manifest.version too long: %d chars (max %d)", len(m.Version), maxManifestVerLen)
	}

	// capabilities
	if len(m.Capabilities) == 0 {
		return errors.New("validate: manifest.capabilities must be non-empty")
	}
	for i := range m.Capabilities {
		c := &m.Capabilities[i]
		if c.Action == "" {
			return fmt.Errorf("validate: capability[%d].action is required", i)
		}
		if c.Action != "PICK" && c.Action != "SAVE" {
			return fmt.Errorf("validate: capability[%d].action must be \"PICK\" or \"SAVE\", got %q", i, c.Action)
		}
		if c.Path == "" {
			return fmt.Errorf("validate: capability[%d].path is required", i)
		}
		// Path may be either a relative path (starts with "/", resolved
		// against Manifest.URL by the client) or an absolute http/https
		// URL (for providers whose capability endpoints live on a
		// different host than their manifest URL).
		if !strings.HasPrefix(c.Path, "/") {
			cu, err := url.Parse(c.Path)
			if err != nil || (cu.Scheme != "http" && cu.Scheme != "https") || cu.Host == "" {
				return fmt.Errorf("validate: capability[%d].path must start with \"/\" or be an absolute http(s) URL", i)
			}
		}
		if len(c.Path) > maxCapabilityPathLen {
			return fmt.Errorf("validate: capability[%d].path too long: %d chars (max %d)", i, len(c.Path), maxCapabilityPathLen)
		}
		if len(c.Properties.MimeTypes) == 0 {
			return fmt.Errorf("validate: capability[%d].properties.mimeTypes must be non-empty", i)
		}
		for j, mt := range c.Properties.MimeTypes {
			canon, err := canonicalizeMIME(mt)
			if err != nil {
				return fmt.Errorf("validate: capability[%d].properties.mimeTypes[%d]: %w", i, j, err)
			}
			c.Properties.MimeTypes[j] = canon
		}
		// Sort MIME types alphabetically so the file representation is
		// byte-stable across re-upserts of the same manifest with
		// differently-ordered mimeTypes arrays.
		sort.Strings(c.Properties.MimeTypes)
	}
	return nil
}
```

**Step 3: delete the obsolete `internal/registry/doc.go`** — manifest.go now carries the canonical package doc.

**Step 4: create `internal/registry/testdata/valid-two-apps.json`** so later plans can read it from disk. Content (this becomes the canonical "good file" fixture):

```json
{
  "version": 1,
  "manifests": [
    {
      "id": "files-app",
      "name": "Files",
      "url": "https://files.example.com",
      "version": "2.1.0",
      "capabilities": [
        {
          "action": "PICK",
          "path": "/pick",
          "properties": {
            "mimeTypes": ["*/*"]
          }
        },
        {
          "action": "SAVE",
          "path": "/save",
          "properties": {
            "mimeTypes": ["*/*"]
          }
        }
      ]
    },
    {
      "id": "mail-app",
      "name": "Mail",
      "url": "https://mail.example.com",
      "version": "1.0.0",
      "capabilities": [
        {
          "action": "SAVE",
          "path": "/attach",
          "properties": {
            "mimeTypes": ["image/png", "text/plain"]
          }
        }
      ]
    }
  ]
}
```

Run `go test ./internal/registry -count=1`. All tests in manifest_test.go and mime_test.go MUST pass. Run `go vet ./internal/registry` and `gofmt -l internal/registry/` — both must be clean. Run `go build ./...` — must succeed.
  </action>
  <verify>
<automated>go test ./internal/registry -count=1 && go build ./... && go vet ./internal/registry && test -z "$(gofmt -l internal/registry/)" && test ! -f internal/registry/doc.go && test -f internal/registry/testdata/valid-two-apps.json</automated>
  </verify>
  <done>
- `internal/registry/manifest.go` exists with `Manifest`, `Capability`, `CapabilityProps` types and package doc at the top
- `internal/registry/doc.go` has been deleted (package doc moved to manifest.go)
- `Manifest.Validate()` implements all 19 error cases with the `validate:` prefix, mutates MimeTypes in place to canonical form, sorts MimeTypes alphabetically
- `internal/registry/manifest_test.go` exists with `TestManifestValidate_Happy`, `TestManifestValidate_CanonicalizesInPlace`, `TestManifestValidate_Errors` (22+ subtests)
- `internal/registry/testdata/valid-two-apps.json` exists and is valid JSON matching the locked fileFormat shape
- `go test ./internal/registry -count=1` exits 0
- `go build ./...` succeeds
- `go vet ./internal/registry` is clean
- `gofmt -l internal/registry/` produces no output
- Architectural gate: `go list -deps ./internal/registry | grep -E 'wshub|httpapi'` produces no output (this plan imports nothing from other internal packages)
- Architectural gate: no production file references `log/slog` or `slog.Default` (grep `-rE 'log/slog|slog\.Default' internal/registry/*.go` returns no match in non-test files)
  </done>
</task>

</tasks>

<verification>
Run the full Phase 2 gate after both tasks land:

```bash
go test ./internal/registry -race -count=1 -v
go build ./...
go vet ./internal/registry
test -z "$(gofmt -l internal/registry/)"
! go list -deps ./internal/registry 2>&1 | grep -E 'wshub|httpapi'
! grep -rE 'log/slog|slog\.Default' internal/registry/*.go | grep -v _test.go
```

All six commands must exit 0. Tests expected to pass after this plan: TestMimeMatch, TestCanonicalizeMIME, TestCanonicalizeMIME_Exported, TestManifestValidate_Happy, TestManifestValidate_CanonicalizesInPlace, TestManifestValidate_Errors.
</verification>

<success_criteria>
- Manifest domain type with 19-case Validate() coverage matches RESEARCH error catalog exactly
- Symmetric 3x3 MIME matching proven by 17+ table-driven cases including symmetry assertions
- Canonicalizer rejects `image//png` and `*/subtype` (both bugs from RESEARCH fixed)
- Exported `CanonicalizeMIME` wrapper available for Phase 4
- Package doc lives in manifest.go (face of the domain); doc.go deleted
- Zero transport imports, zero slog imports, zero new go.mod deps
- `go test ./internal/registry -race -count=1 -v` shows all tests passing
</success_criteria>

<output>
After completion, create `.planning/phases/02-registry-core/02-01-manifest-mime-SUMMARY.md` documenting:
- Files created/modified/deleted
- Test count breakdown (TestMimeMatch: N subtests, TestCanonicalizeMIME: M, TestManifestValidate_Errors: K)
- Locked decisions (5 open questions answered, 2 canonicalizer bugs fixed)
- Handoff notes for Plan 02-02 (Manifest + mime.go are stable; Store can be built on top)
</output>
</content>
</invoke>