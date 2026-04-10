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
