---
phase: 04-http-api
plan: 05
type: execute
wave: 5
depends_on: [04-01, 04-02, 04-03, 04-04]
files_modified:
  - go.mod
  - go.sum
  - internal/httpapi/middleware.go
  - internal/httpapi/integration_test.go
  - internal/httpapi/auth_test.go
autonomous: true
requirements_addressed: [OPS-01, WS-08, TEST-02, TEST-05, TEST-06]
gap_closure: false
user_setup: []

must_haves:
  truths:
    - "rs/cors is wired into corsMiddleware with the same AllowedOrigins allow-list that feeds OriginPatterns (WS-08 shared allow-list)"
    - "A full REST round-trip via httptest.NewServer exercises POST → GET list → GET single → second POST (update) → capabilities filter → DELETE → GET 404 → POST without auth → POST invalid body"
    - "A full WebSocket round-trip receives the initial SNAPSHOT, then receives an ADDED event after a subsequent POST within the ctx deadline"
    - "A disallowed Origin header on the WS handshake returns 403; the test sets DialOptions.HTTPHeader['Origin'] to a host different from ts.URL to defeat the same-host bypass"
    - "TestAuth_NoCredentialsInLogs is extended to run the request through the FULL middleware chain (recover→log→cors→authBasic→handler) and asserts PII-free logs end-to-end"
    - "The final architectural gate sweep passes: registry isolation, wshub isolation, no slog.Default, no time.Sleep in tests, no InsecureSkipVerify, no internal/config import, go vet clean, gofmt clean"
  artifacts:
    - path: "internal/httpapi/middleware.go"
      provides: "Real corsMiddleware using rs/cors (replaces the Plan 04-01 pass-through placeholder)"
      contains: "cors.New(cors.Options"
    - path: "internal/httpapi/integration_test.go"
      provides: "TestServer_Integration_RESTRoundTrip, TestServer_Integration_WebSocketRoundTrip, TestServer_WebSocket_RejectsDisallowedOrigin, TestServer_CORS_Headers"
      contains: "func TestServer_Integration_RESTRoundTrip"
    - path: "go.mod"
      provides: "github.com/rs/cors as a direct dependency"
      contains: "github.com/rs/cors"
  key_links:
    - from: "internal/httpapi/middleware.go (corsMiddleware)"
      to: "github.com/rs/cors.New"
      via: "cors.New(cors.Options{AllowedOrigins: s.cfg.AllowedOrigins, AllowCredentials: true, ...}).Handler(next)"
      pattern: "cors\\.New\\(cors\\.Options\\{"
    - from: "integration_test.go (WS origin rejection)"
      to: "websocket.Dial with Origin header"
      via: "DialOptions.HTTPHeader['Origin'] = 'https://evil.example' (different host than ts.URL to bypass same-host exception)"
      pattern: 'HTTPHeader.*Origin.*evil\\.example'
---

<objective>
Wire the real CORS middleware (rs/cors) into the Plan 04-01 placeholder, write the two workhorse integration tests (REST round-trip + WebSocket round-trip via `httptest.NewServer`), write the WebSocket origin-rejection test with the same-host-bypass-defeating Origin header, extend the Plan 04-02 PII test to cover the full middleware chain, and run the final Phase 4 gate sweep.

Purpose: OPS-01, WS-08, TEST-02, TEST-05, TEST-06 all land here. This plan is the "big green bar" moment — everything Phase 4 promised must work end-to-end through `httptest.NewServer(srv.Handler())`.

Output: Full httpapi suite green under -race including the two round-trip tests; all 8 architectural gates pass; `golang.org/x/crypto`, `github.com/rs/cors`, and `github.com/coder/websocket` are all direct dependencies in go.mod.
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
@.planning/phases/04-http-api/04-03-SUMMARY.md
@.planning/phases/04-http-api/04-04-SUMMARY.md
@.planning/research/PITFALLS.md
@internal/httpapi/server.go
@internal/httpapi/middleware.go
@internal/httpapi/auth.go
@internal/httpapi/auth_test.go
@internal/httpapi/handlers_registry.go
@internal/httpapi/handlers_caps.go
@internal/httpapi/events.go

<interfaces>
```go
// github.com/rs/cors v1.11.1 — the library Phase 4 research locked in
type Options struct {
    AllowedOrigins       []string
    AllowOriginFunc      func(origin string) bool
    AllowedMethods       []string
    AllowedHeaders       []string
    ExposedHeaders       []string
    MaxAge               int
    AllowCredentials     bool
    AllowPrivateNetwork  bool
    OptionsPassthrough   bool
    Debug                bool
}
func New(options Options) *Cors
func (c *Cors) Handler(next http.Handler) http.Handler
```

Known quirk: rs/cors v1.11.1 does NOT reject `AllowedOrigins: ["*"] + AllowCredentials: true` at construction time — it silently emits both headers. Plan 04-01's Server.New already guards against this at Server construction time, so the combination cannot reach corsMiddleware.

From Plan 04-04 (already shipped):
```go
func (s *Server) handleCapabilitiesWS(w http.ResponseWriter, r *http.Request)
// Uses s.cfg.AllowedOrigins as OriginPatterns in websocket.Accept
```
Both the CORS allow-list and the WS OriginPatterns share the same `s.cfg.AllowedOrigins` slice — the shared allow-list is WS-08's contract.
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 0: Add github.com/rs/cors dependency</name>
  <files>go.mod, go.sum</files>
  <read_first>
    .planning/phases/04-http-api/04-CONTEXT.md,
    .planning/phases/04-http-api/04-RESEARCH.md
  </read_first>
  <action>
    Add rs/cors:

    ```bash
    ~/sdk/go1.26.2/bin/go get github.com/rs/cors
    ~/sdk/go1.26.2/bin/go mod tidy
    ```

    At this point rs/cors is listed as indirect (nothing imports it yet). Task 1 writes the test file that forces a build of middleware.go, and Task 2 will flip it to direct by importing it from middleware.go. `go mod tidy` after Task 2 will leave it as direct.

    Verify `go build ./...` still succeeds (it should — nothing imports the package yet but the module graph is clean):

    ```bash
    ~/sdk/go1.26.2/bin/go build ./...
    ```

    Commit: `chore(04-05): add github.com/rs/cors dependency`
  </action>
  <verify>
    <automated>cd /home/ben/Dev-local/openburo-spec/open-buro-server &amp;&amp; ~/sdk/go1.26.2/bin/go build ./... &amp;&amp; grep -c "github.com/rs/cors" go.mod</automated>
  </verify>
  <acceptance_criteria>
    - `grep -c "github.com/rs/cors" go.mod → ≥1`
    - `~/sdk/go1.26.2/bin/go build ./...` exits 0
    - `~/sdk/go1.26.2/bin/go mod tidy` is a no-op after the commit (idempotent)
  </acceptance_criteria>
  <done>rs/cors added to go.mod + go.sum; full module builds.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 1: RED — write failing tests for integration REST round-trip, WS round-trip, WS origin rejection, CORS headers, and extend TestAuth_NoCredentialsInLogs</name>
  <files>
    internal/httpapi/integration_test.go (create),
    internal/httpapi/auth_test.go (modify — extend TestAuth_NoCredentialsInLogs)
  </files>
  <read_first>
    .planning/phases/04-http-api/04-CONTEXT.md,
    .planning/phases/04-http-api/04-RESEARCH.md,
    .planning/phases/04-http-api/04-VALIDATION.md,
    .planning/research/PITFALLS.md,
    internal/httpapi/server.go,
    internal/httpapi/middleware.go,
    internal/httpapi/handlers_registry.go,
    internal/httpapi/handlers_caps.go,
    internal/httpapi/events.go,
    internal/httpapi/auth_test.go,
    internal/httpapi/handlers_registry_test.go
  </read_first>
  <behavior>
    integration_test.go:

    - TestServer_Integration_RESTRoundTrip: Full cycle via httptest.NewServer(srv.Handler()):
      1. POST /api/v1/registry (mail-app) + admin:testpass → 201 + application/json
      2. GET /api/v1/registry → 200 + {manifests:[mail-app], count:1} + application/json
      3. GET /api/v1/registry/mail-app → 200 + manifest body + application/json
      4. POST /api/v1/registry (same mail-app payload) + auth → 200 (update)
      5. GET /api/v1/capabilities?action=PICK → 200 + count:1
      6. GET /api/v1/capabilities?mimeType=image/png → 200 + count:1 (wildcard symmetric: */*  matches image/png)
      7. DELETE /api/v1/registry/mail-app + auth → 204
      8. GET /api/v1/registry/mail-app → 404 + envelope
      9. POST /api/v1/registry without auth → 401 + WWW-Authenticate
      10. POST /api/v1/registry with `{not json}` → 400 + envelope
      11. POST with missing required field `{"id":""}` → 400
      Each subtest uses `t.Run` for failure localization.

    - TestServer_Integration_WebSocketRoundTrip: Full WS cycle:
      1. Spin up httptest.NewServer(srv.Handler())
      2. websocket.Dial ts.URL + "/api/v1/capabilities/ws"
      3. Read first message → assert snapshot (SNAPSHOT, empty capabilities)
      4. POST /api/v1/registry (mail-app) + auth in a separate goroutine OR synchronously — the upsert publishes to the hub
      5. Read next WS message within ctx deadline → assert ADDED event for mail-app
      6. DELETE /api/v1/registry/mail-app + auth
      7. Read next WS message → assert REMOVED event for mail-app
      8. Close the connection cleanly

    - TestServer_WebSocket_RejectsDisallowedOrigin: Set up newTestServer (AllowedOrigins = ["https://allowed.example"]); ts := httptest.NewServer(srv.Handler()); `websocket.Dial(ctx, ts.URL + "/api/v1/capabilities/ws", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{"https://evil.example"}}})`; assert err != nil AND resp != nil AND resp.StatusCode == 403. **CRITICAL:** the Origin value MUST be a different host from ts.URL's host — if we used ts.URL itself as the Origin, the `strings.EqualFold(r.Host, u.Host)` same-host bypass in coder/websocket would auto-pass the check, and the test would be a false positive. Document this in a comment.

    - TestServer_CORS_Headers: ts := httptest.NewServer(srv.Handler()); send an OPTIONS preflight with Origin: https://allowed.example, Access-Control-Request-Method: POST; assert 200 + Access-Control-Allow-Origin: https://allowed.example + Access-Control-Allow-Credentials: true + Access-Control-Allow-Methods contains POST. Also send a simple GET with Origin: https://evil.example — assert no Access-Control-Allow-Origin header is set (rs/cors filters disallowed origins out of the response).

    auth_test.go extension:
    - Replace the existing TestAuth_NoCredentialsInLogs (which tested just authBasic in isolation) with one that runs the request through the FULL middleware chain via httptest.NewServer(srv.Handler()). Make 3 requests: (1) failed auth (wrong password), (2) successful auth (admin:testpass) that reaches handleRegistryUpsert and produces an audit log, (3) another failed auth (wrong username). Capture buf.String() and assert all the same PII forbidden substrings PLUS: `Authorization` header name absent, base64 "admin:testpass" encoding absent, bcrypt hash prefix `$2a$` absent.
  </behavior>
  <action>
    Create `internal/httpapi/integration_test.go`:

    ```go
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
        "github.com/openburo/openburo-server/internal/registry"
        "github.com/stretchr/testify/require"
    )

    // newIntegrationTestServer builds a full test server with a cost-12 fixture
    // credential ("admin" / "testpass") and a non-empty AllowedOrigins allow-list.
    // Returns (srv, ts) — callers defer ts.Close().
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

    // TestServer_Integration_RESTRoundTrip exercises every REST route in
    // a single connected flow via httptest.NewServer. This is the workhorse
    // test — it proves middleware chain, handlers, store, and audit plumbing
    // all wire together correctly.
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

    // TestServer_Integration_WebSocketRoundTrip drives a full WebSocket
    // client through the snapshot + subsequent-event protocol.
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
    // CRITICAL: the Origin header value MUST be a host DIFFERENT from
    // ts.URL's host. coder/websocket has a same-host bypass
    // (strings.EqualFold(r.Host, u.Host) in accept.go) that auto-passes
    // the origin check if the request host matches the origin host.
    // If we used ts.URL's host as the Origin, this test would be a
    // FALSE POSITIVE — the request would succeed even without a
    // correctly-configured OriginPatterns allow-list. Using
    // "https://evil.example" guarantees a different host, so the only
    // way this test passes is via the real allow-list check.
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
    ```

    Modify `internal/httpapi/auth_test.go`:

    Replace the body of `TestAuth_NoCredentialsInLogs` with a version that drives the request through the full middleware chain:

    ```go
    // TestAuth_NoCredentialsInLogs — extended in Plan 04-05 to cover the
    // FULL middleware chain (recover → log → cors → authBasic → handler)
    // via httptest.NewServer. Asserts PII-free logs end-to-end across a
    // failed auth, a successful auth (which emits an audit log line), and
    // a second failed auth with a different (nonexistent) username.
    // This is the TEST-06 final assertion.
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
            "$2a$",                   // bcrypt hash prefix — the stored hash MUST NOT leak
            "Basic YWRt",             // base64 "Basic admin..." prefix
            "YWRtaW46",               // base64 "admin:" prefix
            "Authorization",          // header name would indicate header dump
            "nonexistent_user",       // the supplied-but-unknown username — authBasic must NOT log usernames
        }
        for _, needle := range forbidden {
            require.NotContains(t, out, needle, "log contains forbidden substring %q — full log:\n%s", needle, out)
        }
    }
    ```

    This test requires importing `strings` and `net/http/httptest` in auth_test.go (probably already imported). Verify and add if missing.

    Run — MUST fail because corsMiddleware is still the Plan 04-01 pass-through placeholder: `TestServer_CORS_Headers` will fail (no CORS headers), and `TestServer_WebSocket_RejectsDisallowedOrigin` already works because OriginPatterns was wired in Plan 04-04. The REST and WS round-trip tests may actually pass at this point since they don't depend on CORS — unless the test extends over something that requires the real cors wiring. `TestAuth_NoCredentialsInLogs` should still pass (the log format didn't change).

    Document in the RED commit: "TestServer_CORS_Headers will fail until Task 2 wires the real rs/cors middleware; other tests may pass immediately."

    Commit: `test(04-05): add REST + WS round-trip integration tests + WS origin rejection + CORS headers + PII guard over full chain`
  </action>
  <verify>
    <automated>cd /home/ben/Dev-local/openburo-spec/open-buro-server &amp;&amp; ~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestServer_CORS_Headers$' -timeout 10s 2>&amp;1 | head -30 ; echo "EXPECT: TestServer_CORS_Headers FAILS (placeholder corsMiddleware does not set ACAO)"</automated>
  </verify>
  <acceptance_criteria>
    - File exists: `test -f internal/httpapi/integration_test.go`
    - All 4 integration tests declared: `grep -c "^func TestServer_Integration_RESTRoundTrip\|^func TestServer_Integration_WebSocketRoundTrip\|^func TestServer_WebSocket_RejectsDisallowedOrigin\|^func TestServer_CORS_Headers" internal/httpapi/integration_test.go → 4`
    - REST round-trip has ≥11 t.Run sub-steps: `grep -c "t.Run(" internal/httpapi/integration_test.go → ≥11`
    - WS origin rejection uses "https://evil.example" as Origin (different host than ts.URL): `grep -c "https://evil.example" internal/httpapi/integration_test.go → ≥2`
    - Same-host bypass comment present: `grep -c "same-host bypass\|strings.EqualFold" internal/httpapi/integration_test.go → ≥1`
    - WS round-trip reads snapshot then upsert ADDED then delete REMOVED: `grep -c "changeSnapshot\|changeAdded\|changeRemoved" internal/httpapi/integration_test.go → ≥3`
    - TestAuth_NoCredentialsInLogs extended to use httptest.NewServer: `grep -c "httptest.NewServer" internal/httpapi/auth_test.go → ≥1`
    - TestAuth_NoCredentialsInLogs forbids ≥8 substrings: look at the forbidden slice — `grep -c '"\$2a\$"\|"Authorization"\|"Basic YWRt"\|"YWRtaW46"\|"WRONGPASSWORD\|"testpass"\|"nonexistent_user"' internal/httpapi/auth_test.go → ≥6`
    - No time.Sleep in integration test: `! grep -n 'time\.Sleep' internal/httpapi/integration_test.go`
    - RED state for CORS test: `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestServer_CORS_Headers$' -timeout 10s 2>&1 | grep -cE 'FAIL|Access-Control-Allow-Origin' → ≥1`
    - gofmt clean on new files
  </acceptance_criteria>
  <done>RED committed: integration_test.go has the 4 big integration tests including the full REST round-trip (11 sub-steps) and the WS round-trip (snapshot→added→removed), TestServer_WebSocket_RejectsDisallowedOrigin uses a non-ts.URL host to defeat same-host bypass, TestAuth_NoCredentialsInLogs now drives through the full middleware chain with ≥8 forbidden substrings. TestServer_CORS_Headers fails because corsMiddleware is still the Plan 04-01 pass-through placeholder.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: GREEN — wire rs/cors into corsMiddleware and run the final Phase 4 gate sweep</name>
  <files>internal/httpapi/middleware.go (modify — replace placeholder corsMiddleware)</files>
  <read_first>
    .planning/phases/04-http-api/04-CONTEXT.md,
    .planning/phases/04-http-api/04-RESEARCH.md,
    .planning/phases/04-http-api/04-VALIDATION.md,
    internal/httpapi/middleware.go,
    internal/httpapi/server.go,
    internal/httpapi/integration_test.go
  </read_first>
  <action>
    Modify `internal/httpapi/middleware.go`:

    1. Add `"github.com/rs/cors"` to the imports
    2. Replace the placeholder corsMiddleware body with the real implementation:

    ```go
    // corsMiddleware wraps next with the rs/cors handler, driven by
    // s.cfg.AllowedOrigins. The allow-list is SHARED with coder/websocket's
    // AcceptOptions.OriginPatterns in handleCapabilitiesWS, so a browser
    // client that can call the REST API can also connect to the WebSocket
    // endpoint (WS-08).
    //
    // Construction note: rs/cors v1.11.1 does NOT reject the
    // `AllowedOrigins: ["*"] + AllowCredentials: true` combination here —
    // Server.New has already rejected that combination at construction time,
    // so this path only ever sees a safe allow-list.
    func (s *Server) corsMiddleware(next http.Handler) http.Handler {
        c := cors.New(cors.Options{
            AllowedOrigins:   s.cfg.AllowedOrigins,
            AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodOptions},
            AllowedHeaders:   []string{"Authorization", "Content-Type"},
            AllowCredentials: true,
            MaxAge:           300,
        })
        return c.Handler(next)
    }
    ```

    Run `~/sdk/go1.26.2/bin/go mod tidy` — rs/cors flips from indirect to direct.

    Run the full suite under `-race`:
    ```bash
    ~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -count=1 -timeout 120s
    ```

    All tests — including `TestServer_CORS_Headers`, `TestServer_Integration_RESTRoundTrip`, `TestServer_Integration_WebSocketRoundTrip`, `TestServer_WebSocket_RejectsDisallowedOrigin`, `TestAuth_NoCredentialsInLogs` — MUST pass.

    Then run the **full Phase 4 gate sweep**. Save output to a tracking file:

    ```bash
    # 1. Full httpapi suite race-clean
    ~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -count=1 -timeout 120s

    # 2. Architectural isolation — registry never imports wshub or httpapi
    ! ~/sdk/go1.26.2/bin/go list -deps ./internal/registry | grep -E 'wshub|httpapi'

    # 3. Architectural isolation — wshub never imports registry or httpapi (Phase 3 lock continues)
    ! ~/sdk/go1.26.2/bin/go list -deps ./internal/wshub | grep -E 'registry|httpapi'

    # 4. No slog.Default in production
    ! grep -rE 'slog\.Default' internal/httpapi/*.go | grep -v _test.go

    # 5. No time.Sleep in tests (PITFALLS #16)
    ! grep -n 'time\.Sleep' internal/httpapi/*_test.go

    # 6. No InsecureSkipVerify in production (PITFALLS #7)
    ! grep -rn 'InsecureSkipVerify' internal/httpapi/*.go | grep -v _test.go

    # 7. No internal/config import
    ! grep -rn '"github.com/openburo/openburo-server/internal/config"' internal/httpapi/*.go

    # 8. go vet + gofmt
    ~/sdk/go1.26.2/bin/go vet ./internal/httpapi/...
    test -z "$(~/sdk/go1.26.2/bin/gofmt -l internal/httpapi/)"

    # 9. Whole-module build sanity (no phase 5 shutdown work yet, so we skip TEST-03 whole-module race)
    ~/sdk/go1.26.2/bin/go build ./...
    ~/sdk/go1.26.2/bin/go vet ./...
    ```

    All gates MUST pass (empty output where expected, exit 0 where expected).

    Commit: `feat(04-05): wire rs/cors middleware + phase 4 final gate sweep`
  </action>
  <verify>
    <automated>cd /home/ben/Dev-local/openburo-spec/open-buro-server &amp;&amp; ~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -count=1 -timeout 120s &amp;&amp; ! ~/sdk/go1.26.2/bin/go list -deps ./internal/registry | grep -E 'wshub|httpapi' &amp;&amp; ! ~/sdk/go1.26.2/bin/go list -deps ./internal/wshub | grep -E 'registry|httpapi' &amp;&amp; ! grep -rE 'slog\.Default' internal/httpapi/*.go | grep -v _test.go &amp;&amp; ! grep -n 'time\.Sleep' internal/httpapi/*_test.go &amp;&amp; ! grep -rn 'InsecureSkipVerify' internal/httpapi/*.go | grep -v _test.go &amp;&amp; ! grep -rn '"github.com/openburo/openburo-server/internal/config"' internal/httpapi/*.go &amp;&amp; ~/sdk/go1.26.2/bin/go vet ./internal/httpapi/... &amp;&amp; test -z "$(~/sdk/go1.26.2/bin/gofmt -l internal/httpapi/)" &amp;&amp; ~/sdk/go1.26.2/bin/go build ./...</automated>
  </verify>
  <acceptance_criteria>
    - rs/cors imported in middleware.go: `grep -c '"github.com/rs/cors"' internal/httpapi/middleware.go → 1`
    - cors.New with shared allow-list: `grep -c "cors.New(cors.Options" internal/httpapi/middleware.go → 1`
    - AllowedOrigins sourced from s.cfg: `grep -c "AllowedOrigins:   s.cfg.AllowedOrigins" internal/httpapi/middleware.go → 1`
    - AllowCredentials explicitly true: `grep -c "AllowCredentials: true" internal/httpapi/middleware.go → 1`
    - Placeholder pass-through removed: `grep -c "return next" internal/httpapi/middleware.go → 0` (no more `return next` as a standalone cors body)
    - rs/cors is a DIRECT dependency in go.mod: `grep -c '^\s*github.com/rs/cors' go.mod → ≥1` and NOT marked `// indirect`: `! grep -E 'rs/cors.*// indirect' go.mod`
    - Full suite green: `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -count=1 -timeout 120s` exits 0
    - Named tests pass:
      - `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestServer_Integration_RESTRoundTrip$' -timeout 30s` exits 0
      - `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestServer_Integration_WebSocketRoundTrip$' -timeout 30s` exits 0
      - `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestServer_WebSocket_RejectsDisallowedOrigin$' -timeout 10s` exits 0
      - `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestServer_CORS_Headers$' -timeout 10s` exits 0
      - `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestAuth_NoCredentialsInLogs$' -timeout 10s` exits 0
    - Gate 1 (registry isolation): `! ~/sdk/go1.26.2/bin/go list -deps ./internal/registry | grep -E 'wshub|httpapi'` exits 0
    - Gate 2 (wshub isolation): `! ~/sdk/go1.26.2/bin/go list -deps ./internal/wshub | grep -E 'registry|httpapi'` exits 0
    - Gate 3 (no slog.Default): `! grep -rE 'slog\.Default' internal/httpapi/*.go | grep -v _test.go` exits 0
    - Gate 4 (no time.Sleep in tests): `! grep -n 'time\.Sleep' internal/httpapi/*_test.go` exits 0
    - Gate 5 (no InsecureSkipVerify): `! grep -rn 'InsecureSkipVerify' internal/httpapi/*.go | grep -v _test.go` exits 0
    - Gate 6 (no internal/config import): `! grep -rn '"github.com/openburo/openburo-server/internal/config"' internal/httpapi/*.go` exits 0
    - Gate 7 (go vet clean): `~/sdk/go1.26.2/bin/go vet ./internal/httpapi/...` exits 0
    - Gate 8 (gofmt clean): `test -z "$(~/sdk/go1.26.2/bin/gofmt -l internal/httpapi/)"` exits 0
    - Whole-module build: `~/sdk/go1.26.2/bin/go build ./...` exits 0
    - Whole-module vet: `~/sdk/go1.26.2/bin/go vet ./...` exits 0
  </acceptance_criteria>
  <done>GREEN: rs/cors wired into corsMiddleware with the shared allow-list, all 4 integration tests pass (REST round-trip with 11 sub-steps, WS round-trip with snapshot+added+removed, WS origin rejection with different-host Origin, CORS preflight headers), TestAuth_NoCredentialsInLogs passes through the full middleware chain with ≥8 forbidden substrings, and all 8 Phase 4 architectural gates pass. Phase 4 is complete.</done>
</task>

</tasks>

<verification>
```bash
# 1. Full httpapi suite
~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -count=1 -timeout 120s

# 2. Four integration tests
~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestServer_Integration|^TestServer_WebSocket_RejectsDisallowedOrigin$|^TestServer_CORS_Headers$' -timeout 60s

# 3. TEST-06 PII guard over full chain
~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestAuth_NoCredentialsInLogs$' -timeout 10s

# 4. Phase 4 full gate sweep — the canonical home for every architectural check
! ~/sdk/go1.26.2/bin/go list -deps ./internal/registry | grep -E 'wshub|httpapi'
! ~/sdk/go1.26.2/bin/go list -deps ./internal/wshub | grep -E 'registry|httpapi'
! grep -rE 'slog\.Default' internal/httpapi/*.go | grep -v _test.go
! grep -n 'time\.Sleep' internal/httpapi/*_test.go
! grep -rn 'InsecureSkipVerify' internal/httpapi/*.go | grep -v _test.go
! grep -rn '"github.com/openburo/openburo-server/internal/config"' internal/httpapi/*.go
~/sdk/go1.26.2/bin/go vet ./internal/httpapi/...
test -z "$(~/sdk/go1.26.2/bin/gofmt -l internal/httpapi/)"

# 5. Whole-module sanity
~/sdk/go1.26.2/bin/go build ./...
~/sdk/go1.26.2/bin/go vet ./...
```
</verification>

<success_criteria>
- rs/cors is a direct dependency and corsMiddleware sets the right Access-Control headers on preflight
- REST round-trip test covers 11 status-code + Content-Type assertions through the full middleware chain
- WebSocket round-trip test proves snapshot-first, then ADDED on upsert, then REMOVED on delete, all via a single persistent connection
- TestServer_WebSocket_RejectsDisallowedOrigin uses a non-ts.URL host for the Origin header to defeat the same-host bypass and returns 403
- TestAuth_NoCredentialsInLogs captures logs from the full chain (cors + log + audit) and asserts ≥8 forbidden substrings never appear
- All 8 Phase 4 architectural gates pass: registry isolation, wshub isolation, no slog.Default, no time.Sleep, no InsecureSkipVerify, no internal/config import, go vet clean, gofmt clean
- Full httpapi suite green under -race
- Whole module builds and passes vet
</success_criteria>

<output>
After completion, create `.planning/phases/04-http-api/04-05-SUMMARY.md` AND `.planning/phases/04-http-api/04-GATES.md` (the Phase 4 final gate sweep results, mirroring the `.planning/phases/03-websocket-hub/03-GATES.md` document shipped by Plan 03-03). Phase 4 is then ready for `/gsd:verify-work`.
</output>
