package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

// newIntegrationTestServer builds a full test server with a cost-12 fixture
// credential ("admin" / "testpass") and a non-empty AllowedOrigins allow-list
// (https://allowed.example, inherited from newTestServerWithLogger). Returns
// (srv, ts) — callers defer ts.Close() (handled via t.Cleanup).
func newIntegrationTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := newTestServerWithLogger(t, logger)
	creds, err := LoadCredentials("testdata/credentials-valid.yaml")
	require.NoError(t, err)
	srv.creds = creds
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return srv, ts
}

// TestServer_Integration_RESTRoundTrip exercises every REST route in a
// single connected flow via httptest.NewServer(srv.Handler()). This is the
// workhorse test — it proves middleware chain, handlers, store, and audit
// plumbing all wire together correctly.
func TestServer_Integration_RESTRoundTrip(t *testing.T) {
	_, ts := newIntegrationTestServer(t)
	c := ts.Client()

	const manifestJSON = `{
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

	doAuthedPOST := func(body string) *http.Response {
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/registry", strings.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth("admin", "testpass")
		resp, err := c.Do(req)
		require.NoError(t, err)
		return resp
	}

	doAuthedDELETE := func(appID string) *http.Response {
		req, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/registry/"+appID, nil)
		require.NoError(t, err)
		req.SetBasicAuth("admin", "testpass")
		resp, err := c.Do(req)
		require.NoError(t, err)
		return resp
	}

	t.Run("1_POST_Create_201", func(t *testing.T) {
		resp := doAuthedPOST(manifestJSON)
		defer resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	})

	t.Run("2_GET_List_200", func(t *testing.T) {
		r, err := c.Get(ts.URL + "/api/v1/registry")
		require.NoError(t, err)
		defer r.Body.Close()
		require.Equal(t, http.StatusOK, r.StatusCode)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		body, _ := io.ReadAll(r.Body)
		var list struct {
			Manifests []map[string]any `json:"manifests"`
			Count     int              `json:"count"`
		}
		require.NoError(t, json.Unmarshal(body, &list))
		require.Equal(t, 1, list.Count)
		require.Equal(t, "mail-app", list.Manifests[0]["id"])
	})

	t.Run("3_GET_Single_200", func(t *testing.T) {
		r, err := c.Get(ts.URL + "/api/v1/registry/mail-app")
		require.NoError(t, err)
		defer r.Body.Close()
		require.Equal(t, http.StatusOK, r.StatusCode)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
	})

	t.Run("4_POST_Update_200", func(t *testing.T) {
		resp := doAuthedPOST(manifestJSON)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("5_GET_Capabilities_ActionFilter", func(t *testing.T) {
		r, err := c.Get(ts.URL + "/api/v1/capabilities?action=PICK")
		require.NoError(t, err)
		defer r.Body.Close()
		require.Equal(t, http.StatusOK, r.StatusCode)
		body, _ := io.ReadAll(r.Body)
		var cr struct {
			Count int `json:"count"`
		}
		require.NoError(t, json.Unmarshal(body, &cr))
		require.Equal(t, 1, cr.Count)
	})

	t.Run("6_GET_Capabilities_MimeTypeWildcardSymmetric", func(t *testing.T) {
		// Manifest has */* registered. Query with image/png — the Store
		// symmetric 3x3 wildcard match (Phase 2 CAP-04) should match.
		r, err := c.Get(ts.URL + "/api/v1/capabilities?mimeType=image/png")
		require.NoError(t, err)
		defer r.Body.Close()
		require.Equal(t, http.StatusOK, r.StatusCode)
		body, _ := io.ReadAll(r.Body)
		var cr struct {
			Count int `json:"count"`
		}
		require.NoError(t, json.Unmarshal(body, &cr))
		require.Equal(t, 1, cr.Count)
	})

	t.Run("7_DELETE_204", func(t *testing.T) {
		resp := doAuthedDELETE("mail-app")
		defer resp.Body.Close()
		require.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("8_GET_AfterDelete_404", func(t *testing.T) {
		r, err := c.Get(ts.URL + "/api/v1/registry/mail-app")
		require.NoError(t, err)
		defer r.Body.Close()
		require.Equal(t, http.StatusNotFound, r.StatusCode)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
	})

	t.Run("9_POST_NoAuth_401", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/registry",
			strings.NewReader(manifestJSON))
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		require.Equal(t, `Basic realm="openburo"`, resp.Header.Get("WWW-Authenticate"))
	})

	t.Run("10_POST_InvalidJSON_400", func(t *testing.T) {
		resp := doAuthedPOST(`{not json`)
		defer resp.Body.Close()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("11_POST_MissingFields_400", func(t *testing.T) {
		resp := doAuthedPOST(`{"id":"","name":"","url":"","version":"","capabilities":[]}`)
		defer resp.Body.Close()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// TestServer_Integration_WebSocketRoundTrip drives a full WebSocket client
// through the snapshot + subsequent-event protocol.
func TestServer_Integration_WebSocketRoundTrip(t *testing.T) {
	_, ts := newIntegrationTestServer(t)
	c := ts.Client()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Connect to /api/v1/capabilities/ws — coder/websocket accepts
	// http:// URLs directly; no need to rewrite to ws://.
	conn, _, err := websocket.Dial(ctx, ts.URL+"/api/v1/capabilities/ws", nil)
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "")

	// 2. First message is a SNAPSHOT event with empty capabilities
	_, msg, err := conn.Read(ctx)
	require.NoError(t, err)
	var snap registryUpdatedEvent
	require.NoError(t, json.Unmarshal(msg, &snap))
	require.Equal(t, "REGISTRY_UPDATED", snap.Event)
	require.Equal(t, changeSnapshot, snap.Payload.Change)
	require.NotNil(t, snap.Payload.Capabilities)
	require.Len(t, snap.Payload.Capabilities, 0)

	// 3. Upsert a manifest via REST (separate client, authed)
	const manifestJSON = `{
		"id": "mail-app",
		"name": "Mail",
		"url": "https://mail.example/",
		"version": "1.0",
		"capabilities": [
			{
				"action": "PICK",
				"path": "/pick",
				"properties": {"mimeTypes": ["text/plain"]}
			}
		]
	}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/registry", strings.NewReader(manifestJSON))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("admin", "testpass")
	resp, err := c.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// 4. Next WS message is an ADDED event for mail-app
	_, msg2, err := conn.Read(ctx)
	require.NoError(t, err)
	var evt registryUpdatedEvent
	require.NoError(t, json.Unmarshal(msg2, &evt))
	require.Equal(t, changeAdded, evt.Payload.Change)
	require.Equal(t, "mail-app", evt.Payload.AppID)

	// 5. DELETE
	req2, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/registry/mail-app", nil)
	req2.SetBasicAuth("admin", "testpass")
	resp2, err := c.Do(req2)
	require.NoError(t, err)
	resp2.Body.Close()
	require.Equal(t, http.StatusNoContent, resp2.StatusCode)

	// 6. Next WS message is a REMOVED event
	_, msg3, err := conn.Read(ctx)
	require.NoError(t, err)
	var del registryUpdatedEvent
	require.NoError(t, json.Unmarshal(msg3, &del))
	require.Equal(t, changeRemoved, del.Payload.Change)
	require.Equal(t, "mail-app", del.Payload.AppID)
}

// TestServer_WebSocket_RejectsDisallowedOrigin proves WS-08.
//
// CRITICAL: the Origin header value MUST be a host DIFFERENT from ts.URL's
// host. coder/websocket has a same-host bypass
// (strings.EqualFold(r.Host, u.Host) in accept.go) that auto-passes the
// origin check if the request host matches the origin host. If we used
// ts.URL's host as the Origin, this test would be a FALSE POSITIVE — the
// request would succeed even without a correctly-configured OriginPatterns
// allow-list. Using "https://evil.example" guarantees a different host, so
// the only way this test passes is via the real allow-list check.
func TestServer_WebSocket_RejectsDisallowedOrigin(t *testing.T) {
	_, ts := newIntegrationTestServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, resp, err := websocket.Dial(ctx, ts.URL+"/api/v1/capabilities/ws", &websocket.DialOptions{
		HTTPHeader: http.Header{
			// Must be a host different from ts.URL to defeat the
			// same-host bypass. See function doc-comment.
			"Origin": []string{"https://evil.example"},
		},
	})
	require.Error(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// TestServer_CORS_Headers proves rs/cors is actually wired.
func TestServer_CORS_Headers(t *testing.T) {
	_, ts := newIntegrationTestServer(t)
	c := ts.Client()

	// Preflight OPTIONS with allowed Origin
	req, _ := http.NewRequest(http.MethodOptions, ts.URL+"/api/v1/registry", nil)
	req.Header.Set("Origin", "https://allowed.example")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Authorization,Content-Type")
	resp, err := c.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, "https://allowed.example", resp.Header.Get("Access-Control-Allow-Origin"))
	require.Equal(t, "true", resp.Header.Get("Access-Control-Allow-Credentials"))
	require.Contains(t, resp.Header.Get("Access-Control-Allow-Methods"), "POST")

	// Simple GET with disallowed Origin — rs/cors omits the ACAO header
	req2, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/registry", nil)
	req2.Header.Set("Origin", "https://evil.example")
	resp2, err := c.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()
	require.Empty(t, resp2.Header.Get("Access-Control-Allow-Origin"))
}
