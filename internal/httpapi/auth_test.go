package httpapi

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestAuth_NoCredentialsInLogs — extended in Plan 04-05 to cover the FULL
// middleware chain (recover -> log -> cors -> authBasic -> handler) via
// httptest.NewServer. Asserts PII-free logs end-to-end across a failed auth,
// a successful auth (which emits an audit log line), and a second failed
// auth with a different (nonexistent) username. This is the TEST-06 final
// assertion.
func TestAuth_NoCredentialsInLogs(t *testing.T) {
	buf := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(buf, nil))
	srv := newTestServerWithLogger(t, logger)
	creds, err := LoadCredentials("testdata/credentials-valid.yaml")
	require.NoError(t, err)
	srv.creds = creds
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	const manifestJSON = `{"id":"mail-app","name":"Mail","url":"https://mail.example/","version":"1.0","capabilities":[{"action":"PICK","path":"/pick","properties":{"mimeTypes":["*/*"]}}]}`

	doPOST := func(user, pass string) {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/registry", strings.NewReader(manifestJSON))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth(user, pass)
		resp, _ := ts.Client().Do(req)
		if resp != nil {
			resp.Body.Close()
		}
	}

	// 1. Failed auth (wrong password)
	doPOST("admin", "WRONGPASSWORD_secret_value")
	// 2. Successful auth (emits audit log)
	doPOST("admin", "testpass")
	// 3. Failed auth (unknown user)
	doPOST("nonexistent_user", "anotherSecret")

	out := buf.String()
	forbidden := []string{
		"WRONGPASSWORD_secret_value",
		"anotherSecret",
		"testpass",
		"$2a$",             // bcrypt hash prefix — the stored hash MUST NOT leak
		"Basic YWRt",       // base64 "Basic admin..." prefix
		"YWRtaW46",         // base64 "admin:" prefix
		"Authorization",    // header name would indicate header dump
		"nonexistent_user", // the supplied-but-unknown username — authBasic must NOT log usernames
	}
	for _, needle := range forbidden {
		require.NotContains(t, out, needle, "log contains forbidden substring %q — full log:\n%s", needle, out)
	}
}
