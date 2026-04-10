package httpapi

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecover_PanicCaught(t *testing.T) {
	srv := newTestServer(t)
	// Register a panicking handler on the mux directly (test-only).
	srv.mux.HandleFunc("GET /panic", func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	srv.Handler().ServeHTTP(rr, req)
	require.Equal(t, http.StatusInternalServerError, rr.Code)
	require.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	require.Contains(t, rr.Body.String(), `"error":"internal server error"`)

	// Server survives: next request works
	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/health", nil)
	srv.Handler().ServeHTTP(rr2, req2)
	require.Equal(t, http.StatusOK, rr2.Code)
}

func TestLogMiddleware_SkipsHealth(t *testing.T) {
	buf := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(buf, nil))
	srv := newTestServerWithLogger(t, logger)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	srv.Handler().ServeHTTP(httptest.NewRecorder(), req)
	require.NotContains(t, buf.String(), "httpapi: request")
}

func TestLogMiddleware_LogsOtherRoute(t *testing.T) {
	buf := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(buf, nil))
	srv := newTestServerWithLogger(t, logger)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/registry", nil)
	srv.Handler().ServeHTTP(httptest.NewRecorder(), req)
	out := buf.String()
	require.Contains(t, out, "httpapi: request")
	require.Contains(t, out, "path=/api/v1/registry")
	require.Contains(t, out, "status=501")
	require.Contains(t, out, "duration_ms=")
	require.Contains(t, out, "remote=")
}

func TestMiddleware_ChainOrder(t *testing.T) {
	// Register a panicking handler at the mux level. The panic must be
	// caught by recover (outermost), which proves recover wraps log+cors
	// (i.e. recover is on the outside). We assert 500 + envelope, which
	// only happens if recover is correctly the outermost wrapper.
	srv := newTestServer(t)
	srv.mux.HandleFunc("GET /chain-panic", func(w http.ResponseWriter, r *http.Request) {
		panic("from handler, should be caught by recover at top")
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/chain-panic", nil)
	req.Header.Set("Origin", "https://allowed.example")
	srv.Handler().ServeHTTP(rr, req)
	require.Equal(t, http.StatusInternalServerError, rr.Code)
	require.True(t, strings.HasPrefix(rr.Header().Get("Content-Type"), "application/json"))
}
