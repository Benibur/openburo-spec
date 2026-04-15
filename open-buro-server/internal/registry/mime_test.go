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
