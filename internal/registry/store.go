package registry

import (
	"fmt"
	"sort"
	"strings"
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

// Capabilities returns all capabilities across all manifests as
// CapabilityView entries, filtered by filter and sorted per Phase 2
// rules: (lower(AppName), AppID, Action, Path). Filter is applied before
// sort. An empty filter returns every capability. A malformed
// filter.MimeType yields an empty slice (no error); callers wanting a
// 400-on-malformed-query should pre-validate via CanonicalizeMIME.
//
// OR semantics on MIME filter: a capability matches if ANY of its
// declared MIME types matches the query under symmetric 3x3 wildcard
// matching.
func (s *Store) Capabilities(filter CapabilityFilter) []CapabilityView {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Canonicalize the query MIME once, outside the loop.
	var wantMime string
	var wantMimeSet bool
	if filter.MimeType != "" {
		canon, err := canonicalizeMIME(filter.MimeType)
		if err != nil {
			// Open question #3 lock: malformed filter.MimeType → empty result,
			// not an error. Callers pre-validate with CanonicalizeMIME if they
			// want a distinct 400 response.
			return nil
		}
		wantMime = canon
		wantMimeSet = true
	}

	out := make([]CapabilityView, 0)
	for _, m := range s.manifests {
		for _, c := range m.Capabilities {
			// Action filter: exact match, case-sensitive.
			if filter.Action != "" && c.Action != filter.Action {
				continue
			}
			// MIME filter: OR over the capability's declared mimeTypes.
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
		if ai != aj {
			return ai < aj
		}
		if out[i].AppID != out[j].AppID {
			return out[i].AppID < out[j].AppID
		}
		if out[i].Action != out[j].Action {
			return out[i].Action < out[j].Action
		}
		return out[i].Path < out[j].Path
	})

	return out
}
