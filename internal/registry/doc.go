// Package registry holds the in-memory manifest store, domain types
// (Manifest, Capability), MIME wildcard matching, and atomic JSON
// persistence. It is the pure domain core and depends on nothing
// from other internal/ packages.
//
// Phase 1 ships this file only; the real implementation lands in Phase 2.
package registry
