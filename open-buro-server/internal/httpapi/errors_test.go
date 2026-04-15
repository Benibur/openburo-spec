package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrors_Envelope(t *testing.T) {
	rr := httptest.NewRecorder()
	writeBadRequest(rr, "bad input", map[string]any{"field": "name"})
	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var body struct {
		Error   string         `json:"error"`
		Details map[string]any `json:"details"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	require.Equal(t, "bad input", body.Error)
	require.Equal(t, "name", body.Details["field"])
}

func TestErrors_Envelope_NoDetails(t *testing.T) {
	rr := httptest.NewRecorder()
	writeNotFound(rr, "not found")
	require.Equal(t, http.StatusNotFound, rr.Code)
	// omitempty: no "details" key in the body at all
	require.NotContains(t, rr.Body.String(), `"details"`)
	require.Contains(t, rr.Body.String(), `"error":"not found"`)
}

func TestWriteUnauthorized_Header(t *testing.T) {
	rr := httptest.NewRecorder()
	writeUnauthorized(rr)
	require.Equal(t, http.StatusUnauthorized, rr.Code)
	require.Equal(t, `Basic realm="openburo"`, rr.Header().Get("WWW-Authenticate"))
}
