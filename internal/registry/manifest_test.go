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
		{"path without leading slash", mutate(func(m *Manifest) { m.Capabilities[0].Path = "pick" }), `capability[0].path must start with "/"`},
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
