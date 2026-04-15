package httpapi

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/openburo/openburo-server/internal/registry"
	"github.com/openburo/openburo-server/internal/wshub"
	"github.com/stretchr/testify/require"
)

// syncBuffer wraps bytes.Buffer with a mutex so the log middleware's
// write goroutine does not race the test's read goroutine. Mirrors
// the Phase 3 pattern from wshub/subscribe_test.go.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// newTestServer constructs a Server backed by a temp-dir registry store,
// a short-ping wshub.Hub, an empty credential table, and a discard logger.
// Callers that need captured logs should use newTestServerWithLogger.
// Uses t.Cleanup to shut the hub down automatically.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return newTestServerWithLogger(t, logger)
}

func newTestServerWithLogger(t *testing.T, logger *slog.Logger) *Server {
	t.Helper()
	storePath := filepath.Join(t.TempDir(), "registry.json")
	store, err := registry.NewStore(storePath)
	require.NoError(t, err)
	hub := wshub.New(logger, wshub.Options{
		PingInterval: 50 * time.Millisecond,
	})
	t.Cleanup(func() { hub.Close() })
	srv, err := New(logger, store, hub, Credentials{}, Config{
		AllowedOrigins: []string{"https://allowed.example"},
		WSPingInterval: 30 * time.Second,
	})
	require.NoError(t, err)
	return srv
}

func TestServer_New_RejectsEmptyAllowList(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	storePath := filepath.Join(t.TempDir(), "registry.json")
	store, err := registry.NewStore(storePath)
	require.NoError(t, err)
	hub := wshub.New(logger, wshub.Options{PingInterval: 50 * time.Millisecond})
	defer hub.Close()

	_, err = New(logger, store, hub, Credentials{}, Config{
		AllowedOrigins: nil,
		WSPingInterval: 30 * time.Second,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "AllowedOrigins is empty")
}

func TestServer_New_RejectsWildcardWithCredentials(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	storePath := filepath.Join(t.TempDir(), "registry.json")
	store, err := registry.NewStore(storePath)
	require.NoError(t, err)
	hub := wshub.New(logger, wshub.Options{PingInterval: 50 * time.Millisecond})
	defer hub.Close()

	_, err = New(logger, store, hub, Credentials{}, Config{
		AllowedOrigins: []string{"*"},
		WSPingInterval: 30 * time.Second,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), `"*"`)
	require.Contains(t, err.Error(), "AllowCredentials")
}

func TestServer_New_RejectsBadPattern(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	storePath := filepath.Join(t.TempDir(), "registry.json")
	store, err := registry.NewStore(storePath)
	require.NoError(t, err)
	hub := wshub.New(logger, wshub.Options{PingInterval: 50 * time.Millisecond})
	defer hub.Close()

	_, err = New(logger, store, hub, Credentials{}, Config{
		AllowedOrigins: []string{"[invalid"},
		WSPingInterval: 30 * time.Second,
	})
	require.Error(t, err)
	// path.Match returns ErrBadPattern wrapped by our error
	require.Contains(t, err.Error(), "[invalid")
}

func TestServer_New_PanicsOnNilDeps(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, err := registry.NewStore(filepath.Join(t.TempDir(), "r.json"))
	require.NoError(t, err)
	hub := wshub.New(logger, wshub.Options{PingInterval: 50 * time.Millisecond})
	defer hub.Close()
	cfg := Config{AllowedOrigins: []string{"https://allowed.example"}, WSPingInterval: 30 * time.Second}

	t.Run("nil logger", func(t *testing.T) {
		require.Panics(t, func() { _, _ = New(nil, store, hub, Credentials{}, cfg) })
	})
	t.Run("nil store", func(t *testing.T) {
		require.Panics(t, func() { _, _ = New(logger, nil, hub, Credentials{}, cfg) })
	})
	t.Run("nil hub", func(t *testing.T) {
		require.Panics(t, func() { _, _ = New(logger, store, nil, Credentials{}, cfg) })
	})
}

func TestServer_New_Valid(t *testing.T) {
	srv := newTestServer(t)
	require.NotNil(t, srv)
	require.NotNil(t, srv.Handler())
}

func TestServer_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	require.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}
