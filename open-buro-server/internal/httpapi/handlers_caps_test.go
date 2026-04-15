package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/openburo/openburo-server/internal/registry"
	"github.com/stretchr/testify/require"
)

// seedManifest inserts a test manifest into the server's store bypassing
// the REST layer. Used by caps tests to focus on read/WS behavior without
// re-exercising the upsert path (which has its own test file).
func seedManifest(t *testing.T, srv *Server, id string) {
	t.Helper()
	m := registry.Manifest{
		ID:      id,
		Name:    "Mail",
		URL:     "https://mail.example/",
		Version: "1.0",
		Capabilities: []registry.Capability{
			{
				Action: "PICK",
				Path:   "/pick",
				Properties: registry.CapabilityProps{
					MimeTypes: []string{"text/plain"},
				},
			},
		},
	}
	require.NoError(t, srv.store.Upsert(m))
}

func TestHandleCapabilities(t *testing.T) {
	srv := newTestServer(t)
	seedManifest(t, srv, "mail-app")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	r, err := ts.Client().Get(ts.URL + "/api/v1/capabilities")
	require.NoError(t, err)
	defer r.Body.Close()
	require.Equal(t, http.StatusOK, r.StatusCode)
	require.Equal(t, "application/json", r.Header.Get("Content-Type"))
	body, _ := io.ReadAll(r.Body)
	var resp struct {
		Capabilities []registry.CapabilityView `json:"capabilities"`
		Count        int                       `json:"count"`
	}
	require.NoError(t, json.Unmarshal(body, &resp))
	require.Equal(t, 1, resp.Count)
	require.Len(t, resp.Capabilities, 1)
	require.Equal(t, "mail-app", resp.Capabilities[0].AppID)
	require.Equal(t, "PICK", resp.Capabilities[0].Action)
}

func TestHandleCapabilities_ActionFilter(t *testing.T) {
	srv := newTestServer(t)
	seedManifest(t, srv, "mail-app")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	var resp struct {
		Count int `json:"count"`
	}

	// ?action=PICK → 1
	r, err := ts.Client().Get(ts.URL + "/api/v1/capabilities?action=PICK")
	require.NoError(t, err)
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()
	require.NoError(t, json.Unmarshal(body, &resp))
	require.Equal(t, 1, resp.Count)

	// ?action=SAVE → 0
	r2, err := ts.Client().Get(ts.URL + "/api/v1/capabilities?action=SAVE")
	require.NoError(t, err)
	body2, _ := io.ReadAll(r2.Body)
	r2.Body.Close()
	require.NoError(t, json.Unmarshal(body2, &resp))
	require.Equal(t, 0, resp.Count)
}

func TestHandleCapabilities_MimeTypeFilter(t *testing.T) {
	srv := newTestServer(t)
	seedManifest(t, srv, "mail-app")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	var resp struct {
		Count int `json:"count"`
	}

	// Exact match
	r, err := ts.Client().Get(ts.URL + "/api/v1/capabilities?mimeType=text/plain")
	require.NoError(t, err)
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()
	require.NoError(t, json.Unmarshal(body, &resp))
	require.Equal(t, 1, resp.Count)

	// Wildcard symmetric
	r2, err := ts.Client().Get(ts.URL + "/api/v1/capabilities?mimeType=*/*")
	require.NoError(t, err)
	body2, _ := io.ReadAll(r2.Body)
	r2.Body.Close()
	require.NoError(t, json.Unmarshal(body2, &resp))
	require.Equal(t, 1, resp.Count)

	// Non-matching
	r3, err := ts.Client().Get(ts.URL + "/api/v1/capabilities?mimeType=image/png")
	require.NoError(t, err)
	body3, _ := io.ReadAll(r3.Body)
	r3.Body.Close()
	require.NoError(t, json.Unmarshal(body3, &resp))
	require.Equal(t, 0, resp.Count)
}

func TestHandleCapabilities_MalformedMime(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	r, err := ts.Client().Get(ts.URL + "/api/v1/capabilities?mimeType=notamime")
	require.NoError(t, err)
	defer r.Body.Close()
	require.Equal(t, http.StatusBadRequest, r.StatusCode)
	require.Equal(t, "application/json", r.Header.Get("Content-Type"))
	body, _ := io.ReadAll(r.Body)
	require.Contains(t, strings.ToLower(string(body)), "mime")
}

func TestHandleCapabilitiesWS_Upgrade(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, ts.URL+"/api/v1/capabilities/ws", nil)
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Successful upgrade — connection is live.
	require.NotNil(t, conn)
}

func TestHandleCapabilitiesWS_Snapshot_Empty(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, ts.URL+"/api/v1/capabilities/ws", nil)
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "")

	_, msg, err := conn.Read(ctx)
	require.NoError(t, err)

	var evt registryUpdatedEvent
	require.NoError(t, json.Unmarshal(msg, &evt))
	require.Equal(t, "REGISTRY_UPDATED", evt.Event)
	require.Equal(t, changeSnapshot, evt.Payload.Change)
	require.NotNil(t, evt.Payload.Capabilities)
	require.Len(t, evt.Payload.Capabilities, 0)
}

func TestHandleCapabilitiesWS_Snapshot_WithData(t *testing.T) {
	srv := newTestServer(t)
	seedManifest(t, srv, "mail-app")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, ts.URL+"/api/v1/capabilities/ws", nil)
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "")

	_, msg, err := conn.Read(ctx)
	require.NoError(t, err)

	var evt registryUpdatedEvent
	require.NoError(t, json.Unmarshal(msg, &evt))
	require.Equal(t, changeSnapshot, evt.Payload.Change)
	require.Len(t, evt.Payload.Capabilities, 1)
	require.Equal(t, "mail-app", evt.Payload.Capabilities[0].AppID)
}

func TestHandleCapabilitiesWS_SubscribesAfterSnapshot(t *testing.T) {
	srv := newTestServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, ts.URL+"/api/v1/capabilities/ws", nil)
	require.NoError(t, err)
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Read snapshot first
	_, msg, err := conn.Read(ctx)
	require.NoError(t, err)
	var snap registryUpdatedEvent
	require.NoError(t, json.Unmarshal(msg, &snap))
	require.Equal(t, changeSnapshot, snap.Payload.Change)

	// Trigger a publish by seeding the store and calling Publish directly.
	// This simulates what handleRegistryUpsert does internally.
	seedManifest(t, srv, "new-app")
	srv.hub.Publish(newRegistryUpdatedEvent("new-app", changeAdded))

	// Read the next message — must arrive within the ctx deadline
	_, msg2, err := conn.Read(ctx)
	require.NoError(t, err)
	var evt registryUpdatedEvent
	require.NoError(t, json.Unmarshal(msg2, &evt))
	require.Equal(t, changeAdded, evt.Payload.Change)
	require.Equal(t, "new-app", evt.Payload.AppID)
}
