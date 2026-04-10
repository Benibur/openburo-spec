package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/openburo/openburo-server/internal/registry"
)

// maxRegistryBodyBytes caps the manifest POST body size. Larger bodies
// return 400 via http.MaxBytesReader. 1 MiB is comfortably more than any
// realistic manifest.
const maxRegistryBodyBytes = 1 << 20

// handleRegistryUpsert accepts a Manifest JSON body, validates it,
// persists via store.Upsert, and broadcasts a REGISTRY_UPDATED event
// AFTER the mutation succeeds (mutation-then-broadcast, PITFALLS #1).
//
// Status codes:
//
//	201 Created — manifest did not previously exist
//	200 OK      — manifest already existed; this is an update
//	400 Bad Request — invalid JSON, unknown fields, body too large, or Validate error
//	401 Unauthorized — handled by the authBasic middleware (not here)
//	500 Internal Server Error — persistence failed; the store rolls back
func (s *Server) handleRegistryUpsert(w http.ResponseWriter, r *http.Request) {
	// API-11: close body for connection reuse. The decoder reads until
	// EOF below; this defer is defensive for early-return paths.
	defer r.Body.Close()

	// Cap body size at 1 MiB. MaxBytesReader returns an error from
	// Read() when the limit is exceeded; the json.Decoder surfaces that
	// as an Unmarshal error.
	r.Body = http.MaxBytesReader(w, r.Body, maxRegistryBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var manifest registry.Manifest
	if err := dec.Decode(&manifest); err != nil {
		writeBadRequest(w, "invalid JSON body", map[string]any{"reason": err.Error()})
		return
	}
	if err := manifest.Validate(); err != nil {
		writeBadRequest(w, "invalid manifest", map[string]any{"reason": err.Error()})
		return
	}

	// Determine 201 vs 200 BEFORE the Upsert so we don't race a
	// concurrent Delete. The mutation is serialized by store.Upsert's
	// RWMutex; this lookup is read-only and may observe a stale result
	// in the face of concurrent mutations, which is acceptable — the
	// 201/200 distinction is advisory, not load-bearing.
	_, alreadyExisted := s.store.Get(manifest.ID)

	if err := s.store.Upsert(manifest); err != nil {
		s.logger.Error("httpapi: store.Upsert failed",
			"appId", manifest.ID,
			"error", err.Error())
		writeInternal(w, "failed to persist manifest")
		return
	}

	// MUTATION-THEN-BROADCAST (PITFALLS #1): publish AFTER the store
	// mutation succeeds. If publish happened first and the mutation
	// failed, subscribers would see a phantom event for state that
	// doesn't exist.
	change := changeAdded
	if alreadyExisted {
		change = changeUpdated
	}
	s.hub.Publish(newRegistryUpdatedEvent(manifest.ID, change))

	// OPS-06 audit log. Runs AFTER publish so that observing the audit
	// line implies publish fired. user comes from the ctx stashed by
	// authBasic; on this authenticated route it MUST be present.
	username, _ := usernameFromContext(r.Context())
	s.logger.Info("httpapi: audit",
		"user", username,
		"action", "upsert",
		"appId", manifest.ID)

	// Respond with 201 Created on new, 200 OK on update. Body is the
	// validated manifest (Validate may have canonicalized MIME strings).
	w.Header().Set("Content-Type", "application/json")
	if alreadyExisted {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusCreated)
	}
	_ = json.NewEncoder(w).Encode(manifest)
}

// handleRegistryDelete removes a manifest by id.
// Status codes:
//
//	204 No Content — deleted
//	404 Not Found  — no manifest with that id
//	401 Unauthorized — handled by authBasic middleware
//	500 — persistence failed
func (s *Server) handleRegistryDelete(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	appID := r.PathValue("appId")
	if appID == "" {
		writeBadRequest(w, "missing appId path parameter", nil)
		return
	}

	existed, err := s.store.Delete(appID)
	if err != nil {
		s.logger.Error("httpapi: store.Delete failed",
			"appId", appID,
			"error", err.Error())
		writeInternal(w, "failed to delete manifest")
		return
	}
	if !existed {
		writeNotFound(w, "manifest not found")
		return
	}

	// MUTATION-THEN-BROADCAST: publish AFTER delete succeeds.
	s.hub.Publish(newRegistryUpdatedEvent(appID, changeRemoved))

	// Audit log
	username, _ := usernameFromContext(r.Context())
	s.logger.Info("httpapi: audit",
		"user", username,
		"action", "delete",
		"appId", appID)

	w.WriteHeader(http.StatusNoContent)
}

// handleRegistryList returns all manifests under {manifests:[], count:N}.
// Public route (no auth).
func (s *Server) handleRegistryList(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	manifests := s.store.List()
	if manifests == nil {
		manifests = []registry.Manifest{} // ensure JSON "[]" not "null"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(struct {
		Manifests []registry.Manifest `json:"manifests"`
		Count     int                 `json:"count"`
	}{
		Manifests: manifests,
		Count:     len(manifests),
	})
}

// handleRegistryGet returns one manifest by id or 404. Public route.
func (s *Server) handleRegistryGet(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	appID := r.PathValue("appId")
	if appID == "" {
		writeBadRequest(w, "missing appId path parameter", nil)
		return
	}

	manifest, ok := s.store.Get(appID)
	if !ok {
		writeNotFound(w, "manifest not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(manifest)
}

// Compile-time unused import guard: errors is imported only because
// future edits may need errors.Is on the Upsert error to distinguish
// persist-failed-but-rolled-back from other failure modes. Keep the
// import to signal intent to reviewers.
var _ = errors.New
