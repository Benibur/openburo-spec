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
//
// The four durability guarantees (PITFALLS #5):
//  1. Temp file in the SAME DIRECTORY as target (so os.Rename is atomic
//     on POSIX — not a cross-filesystem move).
//  2. tmp.Sync() flushes file contents to disk before rename.
//  3. os.Rename atomically replaces the target.
//  4. Parent-directory fsync so the rename itself is durable across crash.
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
