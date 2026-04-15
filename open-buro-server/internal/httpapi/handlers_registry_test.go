package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// validManifestJSON is the canonical fixture body for upsert tests.
const validManifestJSON = `{
    "id": "mail-app",
    "name": "Mail",
    "url": "https://mail.example/",
    "version": "1.0",
    "capabilities": [
        {
            "action": "PICK",
            "path": "/pick",
            "properties": {"mimeTypes": ["*/*"]}
        }
    ]
}`

// newHandlersTestServer builds an authed test server wrapped in httptest.NewServer.
// Returns (srv, ts). The real admin:testpass creds from
// testdata/credentials-valid.yaml are loaded.
func newHandlersTestServer(t *testing.T, logger *slog.Logger) (*Server, *httptest.Server) {
	t.Helper()
	srv := newTestServerWithLogger(t, logger)
	creds, err := LoadCredentials("testdata/credentials-valid.yaml")
	require.NoError(t, err)
	srv.creds = creds
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return srv, ts
}

func postAuthed(t *testing.T, ts *httptest.Server, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/registry", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("admin", "testpass")
	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	return resp
}

func deleteAuthed(t *testing.T, ts *httptest.Server, appID string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/registry/"+appID, nil)
	require.NoError(t, err)
	req.SetBasicAuth("admin", "testpass")
	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	return resp
}

func TestHandleRegistryUpsert_Create(t *testing.T) {
	_, ts := newHandlersTestServer(t, slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))
	resp := postAuthed(t, ts, validManifestJSON)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
}

func TestHandleRegistryUpsert_Update(t *testing.T) {
	_, ts := newHandlersTestServer(t, slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))
	resp1 := postAuthed(t, ts, validManifestJSON)
	resp1.Body.Close()
	require.Equal(t, http.StatusCreated, resp1.StatusCode)

	resp2 := postAuthed(t, ts, validManifestJSON)
	resp2.Body.Close()
	require.Equal(t, http.StatusOK, resp2.StatusCode)
}

func TestHandleRegistryUpsert_InvalidBody(t *testing.T) {
	_, ts := newHandlersTestServer(t, slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))
	resp := postAuthed(t, ts, `{"id":"","name":"","url":"","version":"","capabilities":[]}`)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "invalid manifest")
}

func TestHandleRegistryUpsert_MalformedJSON(t *testing.T) {
	_, ts := newHandlersTestServer(t, slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))
	resp := postAuthed(t, ts, `{not json`)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "invalid JSON body")
}

func TestHandleRegistryUpsert_UnknownFields(t *testing.T) {
	_, ts := newHandlersTestServer(t, slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))
	body := `{"id":"a","name":"a","url":"https://a","version":"1","capabilities":[{"action":"PICK","path":"/p","properties":{"mimeTypes":["*/*"]}}],"foo":"bar"}`
	resp := postAuthed(t, ts, body)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestHandleRegistryUpsert_BodyTooLarge(t *testing.T) {
	_, ts := newHandlersTestServer(t, slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))
	// 2 MiB of junk — well past the 1 MiB cap
	big := strings.Repeat("x", 2<<20)
	resp := postAuthed(t, ts, `{"padding":"`+big+`"}`)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestHandleRegistryUpsert_NoAuth(t *testing.T) {
	_, ts := newHandlersTestServer(t, slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/registry", strings.NewReader(validManifestJSON))
	req.Header.Set("Content-Type", "application/json")
	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.Equal(t, `Basic realm="openburo"`, resp.Header.Get("WWW-Authenticate"))
}

func TestHandleRegistryUpsert_WrongAuth(t *testing.T) {
	_, ts := newHandlersTestServer(t, slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/registry", strings.NewReader(validManifestJSON))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("admin", "wrong")
	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestHandleRegistryDelete_Existing(t *testing.T) {
	_, ts := newHandlersTestServer(t, slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))
	postAuthed(t, ts, validManifestJSON).Body.Close()

	resp := deleteAuthed(t, ts, "mail-app")
	defer resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Subsequent GET → 404
	r, err := ts.Client().Get(ts.URL + "/api/v1/registry/mail-app")
	require.NoError(t, err)
	defer r.Body.Close()
	require.Equal(t, http.StatusNotFound, r.StatusCode)
}

func TestHandleRegistryDelete_NonExistent(t *testing.T) {
	_, ts := newHandlersTestServer(t, slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))
	resp := deleteAuthed(t, ts, "nope")
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestHandleRegistryDelete_NoAuth(t *testing.T) {
	_, ts := newHandlersTestServer(t, slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/registry/mail-app", nil)
	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestHandleRegistryList(t *testing.T) {
	_, ts := newHandlersTestServer(t, slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))
	// Empty
	r, _ := ts.Client().Get(ts.URL + "/api/v1/registry")
	require.Equal(t, http.StatusOK, r.StatusCode)
	require.Equal(t, "application/json", r.Header.Get("Content-Type"))
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()
	var listResp struct {
		Manifests []map[string]any `json:"manifests"`
		Count     int              `json:"count"`
	}
	require.NoError(t, json.Unmarshal(body, &listResp))
	require.Equal(t, 0, listResp.Count)
	require.Empty(t, listResp.Manifests)

	// After upsert
	postAuthed(t, ts, validManifestJSON).Body.Close()
	r2, _ := ts.Client().Get(ts.URL + "/api/v1/registry")
	body2, _ := io.ReadAll(r2.Body)
	r2.Body.Close()
	require.NoError(t, json.Unmarshal(body2, &listResp))
	require.Equal(t, 1, listResp.Count)
	require.Len(t, listResp.Manifests, 1)
}

func TestHandleRegistryGet(t *testing.T) {
	_, ts := newHandlersTestServer(t, slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))
	postAuthed(t, ts, validManifestJSON).Body.Close()

	r, _ := ts.Client().Get(ts.URL + "/api/v1/registry/mail-app")
	require.Equal(t, http.StatusOK, r.StatusCode)
	require.Equal(t, "application/json", r.Header.Get("Content-Type"))
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()
	var m map[string]any
	require.NoError(t, json.Unmarshal(body, &m))
	require.Equal(t, "mail-app", m["id"])

	r2, _ := ts.Client().Get(ts.URL + "/api/v1/registry/nope")
	require.Equal(t, http.StatusNotFound, r2.StatusCode)
	require.Equal(t, "application/json", r2.Header.Get("Content-Type"))
	r2.Body.Close()
}

func TestPublicRoutes(t *testing.T) {
	_, ts := newHandlersTestServer(t, slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))
	// All reads must NOT require auth.
	for _, path := range []string{"/health", "/api/v1/registry", "/api/v1/registry/x", "/api/v1/capabilities"} {
		r, err := ts.Client().Get(ts.URL + path)
		require.NoError(t, err, "path %s", path)
		r.Body.Close()
		require.NotEqual(t, http.StatusUnauthorized, r.StatusCode,
			"public route %s should not require auth; got %d", path, r.StatusCode)
	}
}

func TestHandlers_BodyClosed(t *testing.T) {
	// Two sequential POSTs on the same client force connection reuse.
	// If the first handler did not close the body, the second request
	// would error or block. Client.Do automatically closes the body on
	// response return, but the HANDLER is responsible for reading until
	// EOF and closing r.Body. Our assertion: three sequential POSTs succeed.
	_, ts := newHandlersTestServer(t, slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))
	c := ts.Client()
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/registry",
			bytes.NewReader([]byte(validManifestJSON)))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth("admin", "testpass")
		resp, err := c.Do(req)
		require.NoError(t, err, "request %d failed — body likely not closed", i)
		resp.Body.Close()
		require.True(t, resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK)
	}
}

func TestHandlers_ContentType(t *testing.T) {
	_, ts := newHandlersTestServer(t, slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))

	// List
	r, _ := ts.Client().Get(ts.URL + "/api/v1/registry")
	r.Body.Close()
	require.Equal(t, "application/json", r.Header.Get("Content-Type"))

	// 404
	r2, _ := ts.Client().Get(ts.URL + "/api/v1/registry/missing")
	r2.Body.Close()
	require.Equal(t, "application/json", r2.Header.Get("Content-Type"))

	// 400 invalid body
	r3 := postAuthed(t, ts, `not json`)
	r3.Body.Close()
	require.Equal(t, "application/json", r3.Header.Get("Content-Type"))
}

func TestServer_AuditLog(t *testing.T) {
	buf := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(buf, nil))
	_, ts := newHandlersTestServer(t, logger)

	postAuthed(t, ts, validManifestJSON).Body.Close()
	deleteAuthed(t, ts, "mail-app").Body.Close()

	out := buf.String()
	// Exactly one upsert audit line
	require.Equal(t, 1, strings.Count(out, `msg="httpapi: audit" user=admin action=upsert appId=mail-app`),
		"expected exactly one upsert audit line, got: %s", out)
	// Exactly one delete audit line
	require.Equal(t, 1, strings.Count(out, `msg="httpapi: audit" user=admin action=delete appId=mail-app`),
		"expected exactly one delete audit line, got: %s", out)

	// PII assertions: audit log must NOT leak credential material
	require.NotContains(t, out, "testpass")
	require.NotContains(t, out, "YWRtaW46") // base64 prefix of "admin:"
	require.NotContains(t, out, "$2a$")     // bcrypt hash prefix
	require.NotContains(t, out, "Basic YWRt")
}
