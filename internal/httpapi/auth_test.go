package httpapi

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newAuthTestServer builds a Server wired to the valid fixture credentials.
func newAuthTestServer(t *testing.T, logger *slog.Logger) *Server {
	t.Helper()
	srv := newTestServerWithLogger(t, logger)
	creds, err := LoadCredentials("testdata/credentials-valid.yaml")
	require.NoError(t, err)
	srv.creds = creds
	return srv
}

func TestAuth_EmptyHeader(t *testing.T) {
	srv := newAuthTestServer(t, slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	h := srv.authBasic(next)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/registry", nil)
	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusUnauthorized, rr.Code)
	require.False(t, called)
	require.Equal(t, `Basic realm="openburo"`, rr.Header().Get("WWW-Authenticate"))
}

func TestAuth_WrongPassword(t *testing.T) {
	srv := newAuthTestServer(t, slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	h := srv.authBasic(next)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/registry", nil)
	req.SetBasicAuth("admin", "wrong")
	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusUnauthorized, rr.Code)
	require.False(t, called)
}

func TestAuth_UnknownUser(t *testing.T) {
	srv := newAuthTestServer(t, slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	h := srv.authBasic(next)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/registry", nil)
	req.SetBasicAuth("alice", "whatever")
	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusUnauthorized, rr.Code)
	require.False(t, called)
}

func TestAuth_Success(t *testing.T) {
	srv := newAuthTestServer(t, slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))
	var gotUser string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := usernameFromContext(r.Context())
		require.True(t, ok)
		gotUser = u
		w.WriteHeader(http.StatusOK)
	})
	h := srv.authBasic(next)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/registry", nil)
	req.SetBasicAuth("admin", "testpass")
	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "admin", gotUser)
}

// TestAuth_TimingSafe proves bcrypt runs on BOTH the "unknown user" and
// "wrong password" paths, and that neither path takes drastically longer
// than the other. A short-circuit early return on `if !found` would make
// the unknown-user path nearly instant, violating AUTH-04.
func TestAuth_TimingSafe(t *testing.T) {
	srv := newAuthTestServer(t, slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := srv.authBasic(next)

	const iterations = 5

	// Unknown user path
	startUnknown := time.Now()
	for i := 0; i < iterations; i++ {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/registry", nil)
		req.SetBasicAuth("nonexistent", "anything")
		h.ServeHTTP(rr, req)
		require.Equal(t, http.StatusUnauthorized, rr.Code)
	}
	unknownElapsed := time.Since(startUnknown)

	// Wrong password path (known user)
	startWrong := time.Now()
	for i := 0; i < iterations; i++ {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/registry", nil)
		req.SetBasicAuth("admin", "wrongpassword")
		h.ServeHTTP(rr, req)
		require.Equal(t, http.StatusUnauthorized, rr.Code)
	}
	wrongElapsed := time.Since(startWrong)

	// Both paths must take > 50ms total for 5 iterations (proves bcrypt cost 12 ran each time)
	require.Greater(t, unknownElapsed, 50*time.Millisecond,
		"unknown-user path too fast (%v) - bcrypt likely did NOT run, timing-safety violated", unknownElapsed)
	require.Greater(t, wrongElapsed, 50*time.Millisecond,
		"wrong-password path too fast (%v) - bcrypt did NOT run", wrongElapsed)

	// Ratio between the two paths must be modest. A ratio > 3 suggests
	// one path is short-circuiting.
	ratio := float64(unknownElapsed) / float64(wrongElapsed)
	if ratio < 1 {
		ratio = 1 / ratio
	}
	require.Less(t, ratio, 3.0,
		"timing ratio unknown/wrong = %v - one path is taking an order of magnitude longer than the other, indicating a short-circuit", ratio)
}

// TestAuth_DummyHashBcryptRuns proves that even with an entirely empty
// Credentials table (no users registered at all), the authBasic middleware
// still runs bcrypt (against dummyHash), so an attacker cannot probe
// "is there any user here?" via wall-clock time.
func TestAuth_DummyHashBcryptRuns(t *testing.T) {
	srv := newTestServerWithLogger(t, slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))
	srv.creds = Credentials{} // empty - zero users
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := srv.authBasic(next)

	start := time.Now()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/registry", nil)
	req.SetBasicAuth("bob", "whatever")
	h.ServeHTTP(rr, req)
	elapsed := time.Since(start)

	require.Equal(t, http.StatusUnauthorized, rr.Code)
	require.Greater(t, elapsed, 30*time.Millisecond,
		"empty-creds path too fast (%v) - dummyHash bcrypt did NOT run", elapsed)
}

// TestAuth_NoCredentialsInLogs captures slog output and proves no PII leaks.
// This is the TEST-06 anchor. Plan 04-05 will extend it to cover the full
// middleware chain; this plan covers just authBasic's Warn log line.
func TestAuth_NoCredentialsInLogs(t *testing.T) {
	buf := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(buf, nil))
	srv := newAuthTestServer(t, logger)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := srv.authBasic(next)

	// 1. Failed auth
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/registry", nil)
	req.SetBasicAuth("admin", "WRONGPASSWORD")
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusUnauthorized, rr.Code)

	// 2. Successful auth
	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/registry", nil)
	req2.SetBasicAuth("admin", "testpass")
	h.ServeHTTP(rr2, req2)
	require.Equal(t, http.StatusOK, rr2.Code)

	out := buf.String()
	// PII assertions - these MUST all pass or AUTH-05 is violated.
	require.NotContains(t, out, "WRONGPASSWORD", "log contains plaintext password")
	require.NotContains(t, out, "testpass", "log contains plaintext valid password")
	require.NotContains(t, out, "Basic YWRtaW4", "log contains base64 Basic header")
	require.NotContains(t, out, "YWRtaW46", "log contains base64 username prefix")
	require.NotContains(t, out, "Authorization", "log leaks header name (sign of header dump)")
	// The username `admin` by itself is not forbidden here - the audit log
	// in plan 04-03 is ALLOWED to emit user=admin. This middleware just
	// must not leak the credential MATERIAL (password, header, base64).
}
