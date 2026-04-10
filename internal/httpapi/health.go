package httpapi

import (
	"net/http"
)

// handleHealth answers GET /health with 200 and a minimal JSON body.
// No authentication (public per AUTH-03 / FOUND-04).
// Returns application/json to establish the content-type convention
// that future handlers will follow.
//
// Deliberately does NOT log the request: health endpoints are the
// noisiest routes in any HTTP service, and logging them pollutes logs.
// Phase 4's log middleware will skip /health explicitly for the same reason.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
