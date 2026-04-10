package httpapi

import (
	"encoding/json"
	"net/http"
)

// writeJSONError writes a JSON error response with the given status code
// and message. details is optional (pass nil for none). Content-Type is
// set to application/json. This is the single source of error-envelope
// truth — all 4xx/5xx handlers MUST funnel through here or the shortcuts
// below for consistency (API-09).
func writeJSONError(w http.ResponseWriter, status int, message string, details map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := struct {
		Error   string         `json:"error"`
		Details map[string]any `json:"details,omitempty"`
	}{
		Error:   message,
		Details: details,
	}
	_ = json.NewEncoder(w).Encode(body)
}

// writeBadRequest writes a 400 envelope with optional details.
func writeBadRequest(w http.ResponseWriter, msg string, details map[string]any) {
	writeJSONError(w, http.StatusBadRequest, msg, details)
}

// writeUnauthorized writes a 401 envelope and sets WWW-Authenticate so
// browsers and CLI tools know how to prompt for credentials.
func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="openburo"`)
	writeJSONError(w, http.StatusUnauthorized, "unauthorized", nil)
}

// writeForbidden writes a 403 envelope.
func writeForbidden(w http.ResponseWriter, msg string) {
	writeJSONError(w, http.StatusForbidden, msg, nil)
}

// writeNotFound writes a 404 envelope.
func writeNotFound(w http.ResponseWriter, msg string) {
	writeJSONError(w, http.StatusNotFound, msg, nil)
}

// writeInternal writes a 500 envelope. Used by recoverMiddleware for
// caught panics and by any handler that encounters an unrecoverable
// internal error it doesn't want to map to a more specific code.
func writeInternal(w http.ResponseWriter, msg string) {
	writeJSONError(w, http.StatusInternalServerError, msg, nil)
}
