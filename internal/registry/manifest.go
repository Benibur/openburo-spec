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
