package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHealth(t *testing.T) {
	// Use a discard logger so tests don't spew to stderr.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(logger)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	// Critical for FOUND-04: no Authorization header set. The test builds
	// the request without calling req.Header.Set("Authorization", ...).
	rr := httptest.NewRecorder()

	srv.Handler().ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	body, err := io.ReadAll(rr.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), `"status"`)
	require.Contains(t, string(body), `"ok"`)
}

func TestHealth_RejectsWrongMethod(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(logger)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/health", strings.NewReader(""))
			rr := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rr, req)
			require.Equal(t, http.StatusMethodNotAllowed, rr.Code)
		})
	}
}
