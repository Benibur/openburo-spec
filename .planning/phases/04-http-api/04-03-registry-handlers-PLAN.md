---
phase: 04-http-api
plan: 03
type: execute
wave: 3
depends_on: [04-01, 04-02]
files_modified:
  - internal/httpapi/events.go
  - internal/httpapi/handlers_registry.go
  - internal/httpapi/server.go
  - internal/httpapi/events_test.go
  - internal/httpapi/handlers_registry_test.go
autonomous: true
requirements_addressed: [API-01, API-02, API-03, API-04, API-11, WS-05, WS-09, OPS-06, AUTH-02, AUTH-03, API-10]
gap_closure: false
user_setup: []

must_haves:
  truths:
    - "POST /api/v1/registry returns 201 on create, 200 on update, 400 on invalid body, 401 without auth"
    - "DELETE /api/v1/registry/{appId} returns 204 on success, 404 on unknown id, 401 without auth"
    - "GET /api/v1/registry returns {manifests:[], count:N} with application/json"
    - "GET /api/v1/registry/{appId} returns one manifest or 404 with envelope"
    - "Every successful upsert and delete emits a REGISTRY_UPDATED event via hub.Publish AFTER the store mutation succeeds (mutation-then-broadcast, PITFALLS #1)"
    - "Every successful write emits a second 'httpapi: audit' log line with user, action, appId fields and no credential material"
    - "Request bodies are fully read and closed via defer r.Body.Close() + json.MaxBytesReader(1 MiB)"
    - "internal/registry does NOT import internal/wshub (architectural gate enforced by go list -deps)"
  artifacts:
    - path: "internal/httpapi/events.go"
      provides: "registryUpdatedEvent type, changeType constants, newRegistryUpdatedEvent helper"
      contains: "func newRegistryUpdatedEvent"
    - path: "internal/httpapi/handlers_registry.go"
      provides: "handleRegistryUpsert, handleRegistryDelete, handleRegistryList, handleRegistryGet"
      contains: "func (s *Server) handleRegistryUpsert"
    - path: "internal/httpapi/server.go"
      provides: "registerRoutes updated: stub501 replaced by real handlers + authBasic wrap on POST/DELETE"
  key_links:
    - from: "internal/httpapi/handlers_registry.go (upsert)"
      to: "internal/wshub.Hub.Publish"
      via: "s.hub.Publish(newRegistryUpdatedEvent(manifest.ID, changeAdded|changeUpdated)) AFTER store.Upsert succeeds"
      pattern: "store\\.Upsert.*s\\.hub\\.Publish|Upsert.*Publish"
    - from: "internal/httpapi/handlers_registry.go (delete)"
      to: "internal/wshub.Hub.Publish"
      via: "s.hub.Publish(newRegistryUpdatedEvent(appId, changeRemoved)) AFTER store.Delete returns existed=true"
      pattern: "store\\.Delete.*s\\.hub\\.Publish"
    - from: "internal/httpapi/server.go (registerRoutes)"
      to: "internal/httpapi/auth.go (authBasic)"
      via: "POST/DELETE handlers wrapped with s.authBasic(...)"
      pattern: "s\\.authBasic\\(http\\.HandlerFunc\\(s\\.handleRegistry(Upsert|Delete)\\)\\)"
---

<objective>
Replace the Plan 04-01 `stub501` handlers for the four REST registry routes with real implementations: JSON-body upsert/delete with validation, mutation-then-broadcast to the hub, structured audit log on writes, and the public list/get reads. This plan also wires `s.authBasic(...)` around the POST and DELETE routes in `registerRoutes` — the first time authentication is actually enforced end-to-end.

Purpose: API-01..04, WS-05, WS-09, OPS-06, AUTH-02, AUTH-03 all land here. The load-bearing architectural invariant is mutation-then-broadcast: `store.Upsert` MUST succeed before `hub.Publish` fires, or subscribers see a phantom event for state that doesn't exist.

Output: `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run 'TestHandleRegistry|TestNewRegistryUpdatedEvent|TestServer_AuditLog|TestPublicRoutes|TestHandlers_BodyClosed'` all passing.
</objective>

<execution_context>
@/home/ben/.claude/get-shit-done/workflows/execute-plan.md
@/home/ben/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/STATE.md
@.planning/REQUIREMENTS.md
@.planning/phases/04-http-api/04-CONTEXT.md
@.planning/phases/04-http-api/04-RESEARCH.md
@.planning/phases/04-http-api/04-VALIDATION.md
@.planning/phases/04-http-api/04-01-SUMMARY.md
@.planning/phases/04-http-api/04-02-SUMMARY.md
@.planning/research/PITFALLS.md
@internal/httpapi/server.go
@internal/httpapi/errors.go
@internal/httpapi/auth.go
@internal/httpapi/middleware.go
@internal/registry/store.go
@internal/registry/manifest.go
@internal/wshub/hub.go

<interfaces>
```go
// From internal/registry/store.go (stable Phase 2):
func (s *Store) Get(id string) (Manifest, bool)
func (s *Store) List() []Manifest
func (s *Store) Upsert(m Manifest) error     // wraps persist errors as "persist failed, registry unchanged: %w"
func (s *Store) Delete(id string) (bool, error)

// From internal/registry/manifest.go (stable Phase 2):
type Manifest struct {
    ID           string       `json:"id"`
    Name         string       `json:"name"`
    URL          string       `json:"url"`
    Version      string       `json:"version"`
    Capabilities []Capability `json:"capabilities"`
}
func (m *Manifest) Validate() error

// From internal/wshub/hub.go (stable Phase 3):
func (h *Hub) Publish(msg []byte)  // fire-and-forget, non-blocking, drop-slow-consumer

// From internal/httpapi/auth.go (Plan 04-02):
func (s *Server) authBasic(next http.Handler) http.Handler
func usernameFromContext(ctx context.Context) (string, bool)

// This plan creates:
// events.go
type changeType string
const (
    changeAdded    changeType = "ADDED"
    changeUpdated  changeType = "UPDATED"
    changeRemoved  changeType = "REMOVED"
    changeSnapshot changeType = "SNAPSHOT"
)
type registryUpdatedEvent struct {
    Event     string       `json:"event"`
    Timestamp string       `json:"timestamp"`
    Payload   eventPayload `json:"payload"`
}
type eventPayload struct {
    Change       changeType                `json:"change"`
    AppID        string                    `json:"appId,omitempty"`
    Capabilities []registry.CapabilityView `json:"capabilities,omitempty"`
}
func newRegistryUpdatedEvent(appID string, change changeType) []byte

// handlers_registry.go
func (s *Server) handleRegistryUpsert(w http.ResponseWriter, r *http.Request)
func (s *Server) handleRegistryDelete(w http.ResponseWriter, r *http.Request)
func (s *Server) handleRegistryList(w http.ResponseWriter, r *http.Request)
func (s *Server) handleRegistryGet(w http.ResponseWriter, r *http.Request)
```

Plan 04-04 will add `newSnapshotEvent(caps []registry.CapabilityView) []byte` using the same `eventPayload` struct — this plan pre-declares `changeSnapshot` and leaves the `Capabilities` field with `omitempty` so snapshot events can coexist with upsert events.
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: RED — write failing tests for events.go, registry handlers, audit log, public routes, body close</name>
  <files>
    internal/httpapi/events_test.go (create),
    internal/httpapi/handlers_registry_test.go (create)
  </files>
  <read_first>
    .planning/phases/04-http-api/04-CONTEXT.md,
    .planning/phases/04-http-api/04-RESEARCH.md,
    .planning/phases/04-http-api/04-VALIDATION.md,
    .planning/research/PITFALLS.md,
    internal/httpapi/server.go,
    internal/httpapi/server_test.go,
    internal/httpapi/auth.go,
    internal/httpapi/auth_test.go,
    internal/registry/store.go,
    internal/registry/manifest.go,
    internal/wshub/hub.go
  </read_first>
  <behavior>
    events_test.go:
    - TestNewRegistryUpdatedEvent_Added: newRegistryUpdatedEvent("mail-app", changeAdded) returns JSON with event=REGISTRY_UPDATED, payload.appId=mail-app, payload.change=ADDED, and a parseable RFC3339 timestamp with millisecond precision
    - TestNewRegistryUpdatedEvent_Updated: same with change=UPDATED
    - TestNewRegistryUpdatedEvent_Removed: same with change=REMOVED
    - TestEventPayload_OmitemptyCapabilities: upsert event must have NO `capabilities` key in the serialized JSON (omitempty)

    handlers_registry_test.go — using `newTestServer(t)` + `srv.creds = LoadCredentials("testdata/credentials-valid.yaml")` + `httptest.NewServer(srv.Handler())`:
    - TestHandleRegistryUpsert_Create: POST with valid manifest + admin:testpass → 201, body is the manifest JSON, subsequent GET returns it
    - TestHandleRegistryUpsert_Update: POST twice with same id → 201 then 200
    - TestHandleRegistryUpsert_InvalidBody: POST with `{"id":""}` → 400 with envelope containing "invalid manifest"
    - TestHandleRegistryUpsert_MalformedJSON: POST with `{not json` → 400 with envelope containing "invalid JSON body"
    - TestHandleRegistryUpsert_NoAuth: POST without Authorization → 401 with WWW-Authenticate header
    - TestHandleRegistryUpsert_WrongAuth: POST with admin:wrong → 401
    - TestHandleRegistryUpsert_UnknownFields: POST with extra field `"foo":"bar"` → 400 (DisallowUnknownFields)
    - TestHandleRegistryUpsert_BodyTooLarge: POST with 2 MiB body → 400 (MaxBytesReader cap is 1 MiB)
    - TestHandleRegistryDelete_Existing: POST then DELETE /api/v1/registry/mail-app → 204, subsequent GET → 404
    - TestHandleRegistryDelete_NonExistent: DELETE /api/v1/registry/nope → 404
    - TestHandleRegistryDelete_NoAuth: DELETE without auth → 401
    - TestHandleRegistryList: empty list → 200 + {manifests:[], count:0}; after upsert → count:1
    - TestHandleRegistryGet: GET /api/v1/registry/{id} returns 200 for existing, 404 for missing
    - TestPublicRoutes: GET /health, GET /api/v1/registry, GET /api/v1/registry/{id}, GET /api/v1/capabilities (still stub 501 at this phase — ACCEPT 501) all return WITHOUT 401, i.e. status code != 401. Health and list/get should return 200; capabilities will be 501 (plan 04-04 replaces).
    - TestHandlers_BodyClosed: POST a manifest, then POST another — connection reuse works (no "body closed" error on the second request; use ts.Client() to force keep-alive). Simpler alt: wrap r.Body in a custom reader that records Close(), register as a test-only handler, and assert Close() was called.
    - TestHandlers_ContentType: GET /api/v1/registry returns Content-Type: application/json; GET /api/v1/registry/{missing} returns application/json on 404; POST invalid body returns application/json on 400
    - TestServer_AuditLog: capture slog via &syncBuffer{}, POST /api/v1/registry + valid auth → assert buffer contains exactly ONE `"httpapi: audit"` line with `user=admin`, `action=upsert`, `appId=mail-app`; DELETE /api/v1/registry/mail-app + valid auth → assert second `"httpapi: audit"` line with `action=delete`, `appId=mail-app`. Also assert the audit log line does NOT contain `testpass`, `Basic`, `Authorization`, or any bcrypt hash prefix `$2a$`.
    - TestServer_RegistryBroadcast: subscribe a fake hub listener by intercepting hub.Publish? Hub has no observer hook. Alternative: construct the test server with a Hub whose subscribers receive events via `httptest.NewServer` + a ws client. That's heavy for this plan — DEFER the full round-trip test to Plan 04-05. For this plan, assert WS-05 indirectly: after POST, the audit log line confirms the handler reached the publish step (the handler implementation publishes before logging audit, so seeing audit log implies publish ran). Document this deliberate choice in the test comment.
    - **Better alternative** for WS-05 coverage in this plan: directly capture Publish calls by using a test-local Hub wrapper. Construct a `testHub` that records published messages, substitute it in newTestServer. But Hub is a concrete type, not an interface. Instead: subscribe a real websocket via httptest.NewServer(srv.Handler()) + websocket.Dial → receive the snapshot (stub — at this plan's state of the world, GET /api/v1/capabilities/ws is still stub501, so dial will FAIL). Defer WS round-trip to 04-05. In THIS plan, the audit log assertion is sufficient evidence that Publish was called — the audit log line runs AFTER hub.Publish in the handler, so if audit is seen, publish ran.
  </behavior>
  <action>
    Create `internal/httpapi/events_test.go`:

    ```go
    package httpapi

    import (
        "encoding/json"
        "strings"
        "testing"
        "time"

        "github.com/stretchr/testify/require"
    )

    func TestNewRegistryUpdatedEvent_Added(t *testing.T) {
        raw := newRegistryUpdatedEvent("mail-app", changeAdded)
        var evt registryUpdatedEvent
        require.NoError(t, json.Unmarshal(raw, &evt))
        require.Equal(t, "REGISTRY_UPDATED", evt.Event)
        require.Equal(t, "mail-app", evt.Payload.AppID)
        require.Equal(t, changeAdded, evt.Payload.Change)
        // Timestamp parses as RFC3339 with ms
        _, err := time.Parse("2006-01-02T15:04:05.000Z07:00", evt.Timestamp)
        require.NoError(t, err, "timestamp %q should parse as RFC3339 with ms", evt.Timestamp)
    }

    func TestNewRegistryUpdatedEvent_Updated(t *testing.T) {
        raw := newRegistryUpdatedEvent("mail-app", changeUpdated)
        var evt registryUpdatedEvent
        require.NoError(t, json.Unmarshal(raw, &evt))
        require.Equal(t, changeUpdated, evt.Payload.Change)
    }

    func TestNewRegistryUpdatedEvent_Removed(t *testing.T) {
        raw := newRegistryUpdatedEvent("mail-app", changeRemoved)
        var evt registryUpdatedEvent
        require.NoError(t, json.Unmarshal(raw, &evt))
        require.Equal(t, changeRemoved, evt.Payload.Change)
    }

    func TestEventPayload_OmitemptyCapabilities(t *testing.T) {
        // Upsert events must NOT include a `capabilities` field (omitempty).
        raw := string(newRegistryUpdatedEvent("mail-app", changeAdded))
        require.False(t, strings.Contains(raw, `"capabilities"`),
            "upsert event must not include capabilities field: %s", raw)
    }
    ```

    Create `internal/httpapi/handlers_registry_test.go`:

    ```go
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
    // Returns (ts, cleanup). The real admin:testpass creds from testdata/credentials-valid.yaml are loaded.
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
        r, _ := ts.Client().Get(ts.URL + "/api/v1/registry/mail-app")
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
        // EOF and closing r.Body. Our assertion: two sequential POSTs succeed.
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
        require.NotContains(t, out, "$2a$") // bcrypt hash prefix
        require.NotContains(t, out, "Basic YWRt")
    }
    ```

    Run — MUST fail to compile. Handlers, events.go symbols, changeAdded, etc. don't exist. RED committed.

    Commit: `test(04-03): add failing tests for registry handlers + events + audit log + body close`
  </action>
  <verify>
    <automated>cd /home/ben/Dev-local/openburo-spec/open-buro-server &amp;&amp; ~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -timeout 30s 2>&amp;1 | head -60 ; echo "EXPECT: undefined symbols for handleRegistry*, changeAdded, newRegistryUpdatedEvent"</automated>
  </verify>
  <acceptance_criteria>
    - Files exist: `test -f internal/httpapi/events_test.go && test -f internal/httpapi/handlers_registry_test.go`
    - All 4 new events tests declared: `grep -c "^func TestNewRegistryUpdatedEvent_\|^func TestEventPayload_" internal/httpapi/events_test.go → ≥4`
    - At least 15 handler tests declared: `grep -c "^func Test" internal/httpapi/handlers_registry_test.go → ≥15`
    - TestServer_AuditLog uses exact substring match: `grep -c 'user=admin action=upsert appId=mail-app' internal/httpapi/handlers_registry_test.go → ≥1`
    - TestServer_AuditLog forbids credential leakage: `grep -c 'require.NotContains' internal/httpapi/handlers_registry_test.go → ≥4`
    - TestPublicRoutes tests all 4 public paths: `grep -c '"/health"\|"/api/v1/registry"\|"/api/v1/registry/x"\|"/api/v1/capabilities"' internal/httpapi/handlers_registry_test.go → ≥4`
    - TestHandlers_BodyClosed loops ≥3 requests: `grep -B1 -A5 "func TestHandlers_BodyClosed" internal/httpapi/handlers_registry_test.go | grep -c 'i < 3' → 1`
    - RED state verified: `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -timeout 30s 2>&1 | grep -cE 'undefined.*(handleRegistry|newRegistryUpdatedEvent|changeAdded|changeRemoved|changeUpdated)' → ≥1`
    - gofmt clean on new files
  </acceptance_criteria>
  <done>RED committed: events_test.go and handlers_registry_test.go contain 20+ failing tests exercising every REST route (create/update/invalid/unauth/body too large/unknown fields/list/get/delete/non-existent/audit log/public routes/body close/content type). Production code does not exist; compilation fails with undefined symbols.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: GREEN — implement events.go + handlers_registry.go + wire authBasic into registerRoutes</name>
  <files>
    internal/httpapi/events.go (create),
    internal/httpapi/handlers_registry.go (create),
    internal/httpapi/server.go (modify — registerRoutes: replace 4 stub501 calls + wrap POST/DELETE with authBasic)
  </files>
  <read_first>
    .planning/phases/04-http-api/04-CONTEXT.md,
    .planning/phases/04-http-api/04-RESEARCH.md,
    .planning/research/PITFALLS.md,
    internal/httpapi/server.go,
    internal/httpapi/errors.go,
    internal/httpapi/auth.go,
    internal/httpapi/events_test.go,
    internal/httpapi/handlers_registry_test.go,
    internal/registry/store.go,
    internal/registry/manifest.go,
    internal/wshub/hub.go
  </read_first>
  <action>
    Create `internal/httpapi/events.go`:

    ```go
    package httpapi

    import (
        "encoding/json"
        "time"

        "github.com/openburo/openburo-server/internal/registry"
    )

    // changeType is the payload.change discriminant in a REGISTRY_UPDATED event.
    // The SNAPSHOT value is pre-declared here (plan 04-04 uses it for the
    // initial full-state snapshot sent on WS connect; this plan emits only
    // ADDED/UPDATED/REMOVED on the mutation-then-broadcast path).
    type changeType string

    const (
        changeAdded    changeType = "ADDED"
        changeUpdated  changeType = "UPDATED"
        changeRemoved  changeType = "REMOVED"
        changeSnapshot changeType = "SNAPSHOT"
    )

    // eventPayload is the nested payload of a REGISTRY_UPDATED event.
    // For upsert/delete events: Change is ADDED|UPDATED|REMOVED and AppID is set.
    // For snapshot events (plan 04-04): Change is SNAPSHOT, AppID is "", and
    // Capabilities carries the full list. omitempty on both variable fields
    // so the two shapes cleanly share one struct.
    type eventPayload struct {
        Change       changeType                `json:"change"`
        AppID        string                    `json:"appId,omitempty"`
        Capabilities []registry.CapabilityView `json:"capabilities,omitempty"`
    }

    // registryUpdatedEvent is the single WebSocket event type the broker
    // emits. Per FEATURES.md "the broker has one thing to say to everyone"
    // there is ONE event name, regardless of whether the change is an add,
    // update, delete, or initial snapshot.
    type registryUpdatedEvent struct {
        Event     string       `json:"event"`
        Timestamp string       `json:"timestamp"`
        Payload   eventPayload `json:"payload"`
    }

    // newRegistryUpdatedEvent builds a REGISTRY_UPDATED event for a single
    // manifest change (ADDED|UPDATED|REMOVED). The timestamp is UTC RFC 3339
    // with millisecond precision.
    //
    // The json.Marshal error is discarded because the struct is fixed and
    // cannot fail to marshal (no channels, no funcs, no unsupported types).
    func newRegistryUpdatedEvent(appID string, change changeType) []byte {
        evt := registryUpdatedEvent{
            Event:     "REGISTRY_UPDATED",
            Timestamp: time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00"),
            Payload: eventPayload{
                Change: change,
                AppID:  appID,
            },
        }
        b, _ := json.Marshal(evt)
        return b
    }
    ```

    Create `internal/httpapi/handlers_registry.go`:

    ```go
    package httpapi

    import (
        "encoding/json"
        "errors"
        "net/http"

        "github.com/openburo/openburo-server/internal/registry"
    )

    // maxRegistryBodyBytes caps the manifest POST body size. Larger bodies
    // return 400 via http.MaxBytesReader. 1 MiB is comfortably more than any
    // realistic manifest.
    const maxRegistryBodyBytes = 1 << 20

    // handleRegistryUpsert accepts a Manifest JSON body, validates it,
    // persists via store.Upsert, and broadcasts a REGISTRY_UPDATED event
    // AFTER the mutation succeeds (mutation-then-broadcast, PITFALLS #1).
    //
    // Status codes:
    //   201 Created — manifest did not previously exist
    //   200 OK      — manifest already existed; this is an update
    //   400 Bad Request — invalid JSON, unknown fields, body too large, or Validate error
    //   401 Unauthorized — handled by the authBasic middleware (not here)
    //   500 Internal Server Error — persistence failed; the store rolls back
    func (s *Server) handleRegistryUpsert(w http.ResponseWriter, r *http.Request) {
        // API-11: close body for connection reuse. The decoder reads until
        // EOF below; this defer is defensive for early-return paths.
        defer r.Body.Close()

        // Cap body size at 1 MiB. MaxBytesReader returns an error from
        // Read() when the limit is exceeded; the json.Decoder surfaces that
        // as an Unmarshal error.
        r.Body = http.MaxBytesReader(w, r.Body, maxRegistryBodyBytes)
        dec := json.NewDecoder(r.Body)
        dec.DisallowUnknownFields()

        var manifest registry.Manifest
        if err := dec.Decode(&manifest); err != nil {
            writeBadRequest(w, "invalid JSON body", map[string]any{"reason": err.Error()})
            return
        }
        if err := manifest.Validate(); err != nil {
            writeBadRequest(w, "invalid manifest", map[string]any{"reason": err.Error()})
            return
        }

        // Determine 201 vs 200 BEFORE the Upsert so we don't race a
        // concurrent Delete. The mutation is serialized by store.Upsert's
        // RWMutex; this lookup is read-only and may observe a stale result
        // in the face of concurrent mutations, which is acceptable — the
        // 201/200 distinction is advisory, not load-bearing.
        _, alreadyExisted := s.store.Get(manifest.ID)

        if err := s.store.Upsert(manifest); err != nil {
            s.logger.Error("httpapi: store.Upsert failed",
                "appId", manifest.ID,
                "error", err.Error())
            writeInternal(w, "failed to persist manifest")
            return
        }

        // MUTATION-THEN-BROADCAST (PITFALLS #1): publish AFTER the store
        // mutation succeeds. If publish happened first and the mutation
        // failed, subscribers would see a phantom event for state that
        // doesn't exist.
        change := changeAdded
        if alreadyExisted {
            change = changeUpdated
        }
        s.hub.Publish(newRegistryUpdatedEvent(manifest.ID, change))

        // OPS-06 audit log. Runs AFTER publish so that observing the audit
        // line implies publish fired. user comes from the ctx stashed by
        // authBasic; on this authenticated route it MUST be present.
        username, _ := usernameFromContext(r.Context())
        s.logger.Info("httpapi: audit",
            "user", username,
            "action", "upsert",
            "appId", manifest.ID)

        // Respond with 201 Created on new, 200 OK on update. Body is the
        // validated manifest (Validate may have canonicalized MIME strings).
        w.Header().Set("Content-Type", "application/json")
        if alreadyExisted {
            w.WriteHeader(http.StatusOK)
        } else {
            w.WriteHeader(http.StatusCreated)
        }
        _ = json.NewEncoder(w).Encode(manifest)
    }

    // handleRegistryDelete removes a manifest by id.
    // Status codes:
    //   204 No Content — deleted
    //   404 Not Found  — no manifest with that id
    //   401 Unauthorized — handled by authBasic middleware
    //   500 — persistence failed
    func (s *Server) handleRegistryDelete(w http.ResponseWriter, r *http.Request) {
        defer r.Body.Close()

        appID := r.PathValue("appId")
        if appID == "" {
            writeBadRequest(w, "missing appId path parameter", nil)
            return
        }

        existed, err := s.store.Delete(appID)
        if err != nil {
            s.logger.Error("httpapi: store.Delete failed",
                "appId", appID,
                "error", err.Error())
            writeInternal(w, "failed to delete manifest")
            return
        }
        if !existed {
            writeNotFound(w, "manifest not found")
            return
        }

        // MUTATION-THEN-BROADCAST: publish AFTER delete succeeds.
        s.hub.Publish(newRegistryUpdatedEvent(appID, changeRemoved))

        // Audit log
        username, _ := usernameFromContext(r.Context())
        s.logger.Info("httpapi: audit",
            "user", username,
            "action", "delete",
            "appId", appID)

        w.WriteHeader(http.StatusNoContent)
    }

    // handleRegistryList returns all manifests under {manifests:[], count:N}.
    // Public route (no auth).
    func (s *Server) handleRegistryList(w http.ResponseWriter, r *http.Request) {
        defer r.Body.Close()

        manifests := s.store.List()
        if manifests == nil {
            manifests = []registry.Manifest{} // ensure JSON "[]" not "null"
        }

        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        _ = json.NewEncoder(w).Encode(struct {
            Manifests []registry.Manifest `json:"manifests"`
            Count     int                 `json:"count"`
        }{
            Manifests: manifests,
            Count:     len(manifests),
        })
    }

    // handleRegistryGet returns one manifest by id or 404. Public route.
    func (s *Server) handleRegistryGet(w http.ResponseWriter, r *http.Request) {
        defer r.Body.Close()

        appID := r.PathValue("appId")
        if appID == "" {
            writeBadRequest(w, "missing appId path parameter", nil)
            return
        }

        manifest, ok := s.store.Get(appID)
        if !ok {
            writeNotFound(w, "manifest not found")
            return
        }

        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        _ = json.NewEncoder(w).Encode(manifest)
    }

    // Compile-time unused import guard: errors is imported only because
    // future edits may need errors.Is on the Upsert error to distinguish
    // persist-failed-but-rolled-back from other failure modes. Keep the
    // import to signal intent to reviewers.
    var _ = errors.New
    ```

    Modify `internal/httpapi/server.go` `registerRoutes`:

    ```go
    func (s *Server) registerRoutes() {
        // Phase 1 route (unchanged)
        s.mux.HandleFunc("GET /health", s.handleHealth)

        // Phase 4 write routes (auth required — authBasic wraps each handler)
        s.mux.Handle("POST /api/v1/registry",
            s.authBasic(http.HandlerFunc(s.handleRegistryUpsert)))
        s.mux.Handle("DELETE /api/v1/registry/{appId}",
            s.authBasic(http.HandlerFunc(s.handleRegistryDelete)))

        // Phase 4 read routes (public)
        s.mux.HandleFunc("GET /api/v1/registry", s.handleRegistryList)
        s.mux.HandleFunc("GET /api/v1/registry/{appId}", s.handleRegistryGet)

        // Phase 4 stubs replaced in later plans
        s.mux.HandleFunc("GET /api/v1/capabilities", s.stub501)      // plan 04-04
        s.mux.HandleFunc("GET /api/v1/capabilities/ws", s.stub501)   // plan 04-04
    }
    ```

    Run the full suite:

    ```bash
    ~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -count=1 -timeout 90s
    ```

    The existing Plan 04-01 TestLogMiddleware_LogsOtherRoute test expects `status=501` for `/api/v1/registry`. That test will now FAIL because the handler returns 200, not 501. FIX that test:

    ```go
    // In middleware_test.go TestLogMiddleware_LogsOtherRoute:
    //   OLD: require.Contains(t, out, "status=501")
    //   NEW: require.Contains(t, out, "status=200")
    ```

    (Edit middleware_test.go to change that single assertion. This is an expected consequence of real handlers replacing the stub — document in the commit message.)

    Run the architectural gate:
    ```bash
    ! ~/sdk/go1.26.2/bin/go list -deps ./internal/registry | grep -E 'wshub|httpapi'
    ```
    Must be empty — the registry package must NOT have gained any import of wshub or httpapi.

    Commit: `feat(04-03): implement registry REST handlers + mutation-then-broadcast + audit log`
  </action>
  <verify>
    <automated>cd /home/ben/Dev-local/openburo-spec/open-buro-server &amp;&amp; ~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -count=1 -timeout 90s &amp;&amp; ! ~/sdk/go1.26.2/bin/go list -deps ./internal/registry | grep -E 'wshub|httpapi' &amp;&amp; ~/sdk/go1.26.2/bin/go vet ./internal/httpapi/... &amp;&amp; test -z "$(~/sdk/go1.26.2/bin/gofmt -l internal/httpapi/)"</automated>
  </verify>
  <acceptance_criteria>
    - Files exist: `test -f internal/httpapi/events.go && test -f internal/httpapi/handlers_registry.go`
    - All 4 handlers defined: `grep -c "^func (s \*Server) handleRegistry" internal/httpapi/handlers_registry.go → 4`
    - Events.go has changeAdded/Updated/Removed/Snapshot: `grep -c "changeAdded\|changeUpdated\|changeRemoved\|changeSnapshot" internal/httpapi/events.go → ≥4`
    - newRegistryUpdatedEvent returns []byte: `grep -c "^func newRegistryUpdatedEvent(appID string, change changeType) \[\]byte" internal/httpapi/events.go → 1`
    - Upsert publishes AFTER store.Upsert: the order is verified by the test TestServer_AuditLog (audit line runs after publish by implementation order). Grep: the hub.Publish line appears AFTER the s.store.Upsert line in the function body: `awk '/func .s .Server. handleRegistryUpsert/,/^}/' internal/httpapi/handlers_registry.go | grep -n 'store.Upsert\|hub.Publish' | head -5` (Upsert line number must be LESS than Publish line number)
    - Delete publishes AFTER store.Delete returns existed=true: similar ordering check
    - DisallowUnknownFields used: `grep -c "DisallowUnknownFields" internal/httpapi/handlers_registry.go → 1`
    - MaxBytesReader used with 1 MiB cap: `grep -c "MaxBytesReader(w, r.Body, maxRegistryBodyBytes)" internal/httpapi/handlers_registry.go → 1` AND `grep -c "1 << 20" internal/httpapi/handlers_registry.go → 1`
    - defer r.Body.Close() in every handler: `grep -c "defer r.Body.Close()" internal/httpapi/handlers_registry.go → 4`
    - Audit log emits user/action/appId without PII: `grep -A4 '"httpapi: audit"' internal/httpapi/handlers_registry.go | grep -c '"user"' → ≥2` AND `grep -A4 '"httpapi: audit"' internal/httpapi/handlers_registry.go | grep -c '"appId"' → ≥2` AND `grep -A4 '"httpapi: audit"' internal/httpapi/handlers_registry.go | grep -cE 'password|Authorization|Basic' → 0`
    - registerRoutes wires authBasic around POST and DELETE: `grep -c "s.authBasic(http.HandlerFunc(s.handleRegistry" internal/httpapi/server.go → 2`
    - GET routes are NOT wrapped in authBasic: `grep "authBasic.*handleRegistryList\|authBasic.*handleRegistryGet" internal/httpapi/server.go | wc -l → 0`
    - Architectural gate: `! ~/sdk/go1.26.2/bin/go list -deps ./internal/registry | grep -E 'wshub|httpapi'` exits 0
    - Named tests pass:
      - `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestHandleRegistryUpsert' -timeout 15s` exits 0
      - `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestHandleRegistryDelete' -timeout 15s` exits 0
      - `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestHandleRegistryList$' -timeout 10s` exits 0
      - `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestHandleRegistryGet$' -timeout 10s` exits 0
      - `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestPublicRoutes$' -timeout 10s` exits 0
      - `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestHandlers_BodyClosed$' -timeout 10s` exits 0
      - `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestServer_AuditLog$' -timeout 10s` exits 0
      - `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestNewRegistryUpdatedEvent' -timeout 10s` exits 0
    - Full suite green: `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -count=1 -timeout 90s` exits 0
    - go vet + gofmt clean
  </acceptance_criteria>
  <done>GREEN: all 4 REST handlers implemented, mutation-then-broadcast wired for upsert+delete, audit log fires with user/action/appId and no PII, authBasic now wraps POST+DELETE in registerRoutes, the updated TestLogMiddleware_LogsOtherRoute test passes with status=200, and the architectural gate `go list -deps ./internal/registry | grep -E 'wshub|httpapi'` is still empty.</done>
</task>

</tasks>

<verification>
```bash
# 1. Full httpapi suite race-clean
~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -count=1 -timeout 120s

# 2. Architectural gate (PITFALLS #1)
! ~/sdk/go1.26.2/bin/go list -deps ./internal/registry | grep -E 'wshub|httpapi'

# 3. Plan 04-03 named tests
~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestHandleRegistry|^TestNewRegistryUpdatedEvent|^TestServer_AuditLog|^TestPublicRoutes|^TestHandlers_BodyClosed|^TestHandlers_ContentType' -timeout 30s

# 4. Mutation-then-broadcast ordering (static check)
awk '/func .s .Server. handleRegistryUpsert/,/^}/' internal/httpapi/handlers_registry.go | grep -n 'store.Upsert\|hub.Publish'
awk '/func .s .Server. handleRegistryDelete/,/^}/' internal/httpapi/handlers_registry.go | grep -n 'store.Delete\|hub.Publish'

# 5. Format + vet
~/sdk/go1.26.2/bin/go vet ./internal/httpapi/...
test -z "$(~/sdk/go1.26.2/bin/gofmt -l internal/httpapi/)"
```
</verification>

<success_criteria>
- POST /api/v1/registry returns 201/200/400/401 per API-01 contract
- DELETE /api/v1/registry/{appId} returns 204/404/401 per API-02 contract
- GET /api/v1/registry returns `{manifests:[], count:N}` per API-03
- GET /api/v1/registry/{appId} returns one manifest or 404 per API-04
- Every successful mutation calls hub.Publish AFTER the store succeeds (WS-05, PITFALLS #1)
- TestServer_AuditLog proves exactly one `httpapi: audit` line per write, with user/action/appId and no credential leakage
- registry package remains isolated: `go list -deps ./internal/registry | grep -E 'wshub|httpapi'` empty (WS-09)
- Request bodies are closed in every handler (API-11)
- Full suite green under -race, go vet + gofmt clean
</success_criteria>

<output>
After completion, create `.planning/phases/04-http-api/04-03-SUMMARY.md`. Note: plan 04-04 implements the /api/v1/capabilities REST handler + /api/v1/capabilities/ws WebSocket upgrade + snapshot-on-connect, using the eventPayload.Capabilities field and changeSnapshot constant pre-declared here.
</output>
