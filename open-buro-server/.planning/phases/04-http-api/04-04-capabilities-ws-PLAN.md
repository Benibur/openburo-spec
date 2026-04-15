---
phase: 04-http-api
plan: 04
type: execute
wave: 4
depends_on: [04-01, 04-02, 04-03]
files_modified:
  - internal/httpapi/events.go
  - internal/httpapi/handlers_caps.go
  - internal/httpapi/server.go
  - internal/httpapi/handlers_caps_test.go
autonomous: true
requirements_addressed: [API-05, WS-01, WS-06, API-10]
gap_closure: false
user_setup: []

must_haves:
  truths:
    - "GET /api/v1/capabilities returns {capabilities:[], count:N} with application/json"
    - "GET /api/v1/capabilities supports ?action=PICK|SAVE and ?mimeType=X/Y filtering"
    - "Malformed mimeType query parameter returns 400 with envelope (pre-validated via registry.CanonicalizeMIME)"
    - "GET /api/v1/capabilities/ws upgrades to WebSocket via websocket.Accept with OriginPatterns sourced from s.cfg.AllowedOrigins"
    - "The first WebSocket message on connect is a SNAPSHOT event carrying the full capability list"
    - "After the snapshot, the subscriber is handed off to hub.Subscribe which runs until disconnect"
    - "InsecureSkipVerify never appears in production code (grep gate)"
  artifacts:
    - path: "internal/httpapi/handlers_caps.go"
      provides: "handleCapabilities, handleCapabilitiesWS, buildFullStateSnapshot"
      contains: "func (s *Server) handleCapabilitiesWS"
    - path: "internal/httpapi/events.go"
      provides: "newSnapshotEvent helper"
      contains: "func newSnapshotEvent"
  key_links:
    - from: "internal/httpapi/handlers_caps.go (handleCapabilitiesWS)"
      to: "github.com/coder/websocket.Accept"
      via: "websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: s.cfg.AllowedOrigins})"
      pattern: "websocket\\.Accept.*OriginPatterns.*s\\.cfg\\.AllowedOrigins"
    - from: "internal/httpapi/handlers_caps.go (handleCapabilitiesWS)"
      to: "internal/wshub.Hub.Subscribe"
      via: "after snapshot write, hand off to s.hub.Subscribe(r.Context(), conn)"
      pattern: "s\\.hub\\.Subscribe\\(r\\.Context\\(\\), conn\\)"
    - from: "internal/httpapi/handlers_caps.go (handleCapabilities)"
      to: "internal/registry.CanonicalizeMIME"
      via: "pre-validate mimeType query param before store.Capabilities"
      pattern: "registry\\.CanonicalizeMIME"
---

<objective>
Implement the last two Phase 4 routes: `GET /api/v1/capabilities` (REST) and `GET /api/v1/capabilities/ws` (WebSocket upgrade with full-state snapshot on connect). This plan also adds `newSnapshotEvent` to `events.go` — the sibling of `newRegistryUpdatedEvent` for the SNAPSHOT change type pre-declared in Plan 04-03.

Purpose: API-05, WS-01, WS-06 land here. The load-bearing architectural invariant is snapshot-before-subscribe: the client MUST observe the full state snapshot as its first message, BEFORE any subsequent event, so `state = event.payload.capabilities` always works on connect.

Output: `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run 'TestHandleCapabilities'` all passing. CORS + integration round-trip tests are deferred to Plan 04-05.
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
@.planning/phases/04-http-api/04-03-SUMMARY.md
@.planning/phases/03-websocket-hub/03-CONTEXT.md
@.planning/research/PITFALLS.md
@internal/httpapi/server.go
@internal/httpapi/events.go
@internal/httpapi/handlers_registry.go
@internal/registry/store.go
@internal/registry/mime.go
@internal/wshub/hub.go
@internal/wshub/subscribe.go

<interfaces>
```go
// From internal/registry (stable Phase 2):
type CapabilityFilter struct {
    Action   string // "PICK" | "SAVE" | ""
    MimeType string // canonical form or ""
}
type CapabilityView struct {
    AppID      string
    AppName    string
    Action     string
    Path       string
    Properties CapabilityProps
}
func (s *Store) Capabilities(filter CapabilityFilter) []CapabilityView
func CanonicalizeMIME(s string) (string, error)

// From internal/wshub (stable Phase 3):
func (h *Hub) Subscribe(ctx context.Context, conn *websocket.Conn) error
// Subscribe blocks until the subscriber's writer goroutine exits
// (disconnect, error, context cancel, or hub Close). It handles
// CloseRead internally and guarantees defer removeSubscriber.

// From github.com/coder/websocket (v1.8.14):
type AcceptOptions struct {
    Subprotocols         []string
    InsecureSkipVerify   bool    // MUST NEVER BE TRUE
    OriginPatterns       []string
    CompressionMode      CompressionMode
    CompressionThreshold int
}
func Accept(w http.ResponseWriter, r *http.Request, opts *AcceptOptions) (*Conn, error)
// Accept writes the handshake response; on origin rejection it writes 403
// and returns a non-nil error — the handler should just return.
func (c *Conn) Write(ctx context.Context, typ MessageType, p []byte) error
func (c *Conn) Close(code StatusCode, reason string) error

// This plan adds to events.go:
func newSnapshotEvent(caps []registry.CapabilityView) []byte

// This plan creates handlers_caps.go:
func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request)
func (s *Server) handleCapabilitiesWS(w http.ResponseWriter, r *http.Request)
func (s *Server) buildFullStateSnapshot() []byte
```

The coder/websocket package is already a Phase 3 direct dependency — no `go get` needed here.
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: RED — write failing tests for handleCapabilities (REST) + handleCapabilitiesWS (upgrade, snapshot) + newSnapshotEvent</name>
  <files>
    internal/httpapi/handlers_caps_test.go (create),
    internal/httpapi/events_test.go (modify — add TestNewSnapshotEvent)
  </files>
  <read_first>
    .planning/phases/04-http-api/04-CONTEXT.md,
    .planning/phases/04-http-api/04-RESEARCH.md,
    .planning/phases/04-http-api/04-VALIDATION.md,
    .planning/phases/03-websocket-hub/03-CONTEXT.md,
    internal/httpapi/events.go,
    internal/httpapi/handlers_registry.go,
    internal/httpapi/server.go,
    internal/registry/store.go,
    internal/registry/manifest.go,
    internal/registry/mime.go,
    internal/wshub/subscribe.go
  </read_first>
  <behavior>
    events_test.go additions:
    - TestNewSnapshotEvent: newSnapshotEvent([]registry.CapabilityView{...}) returns JSON with event=REGISTRY_UPDATED, payload.change=SNAPSHOT, payload.capabilities=[...], NO payload.appId (omitempty). Timestamp parses as RFC3339 with ms.
    - TestNewSnapshotEvent_EmptyList: newSnapshotEvent([]registry.CapabilityView{}) returns JSON containing `"capabilities":[]` (not null, not missing — the consumer does `state = event.payload.capabilities` and expects an array).

    handlers_caps_test.go:
    - TestHandleCapabilities: populate store with one manifest (PICK /pick text/plain); GET /api/v1/capabilities; response is 200 + {capabilities:[{appId,appName,action,path,properties}], count:1}; Content-Type application/json.
    - TestHandleCapabilities_ActionFilter: same setup; GET ?action=PICK returns count=1; GET ?action=SAVE returns count=0.
    - TestHandleCapabilities_MimeTypeFilter: setup with text/plain; GET ?mimeType=text/plain returns count=1; GET ?mimeType=image/png returns count=0; GET ?mimeType=*/* returns count=1 (wildcard symmetry).
    - TestHandleCapabilities_MalformedMime: GET ?mimeType=notamime returns 400 with envelope containing "invalid mime type".
    - TestHandleCapabilitiesWS_Upgrade: start httptest.NewServer(srv.Handler()); websocket.Dial ts.URL + "/api/v1/capabilities/ws" (coder/websocket accepts http:// URLs directly); require.NoError; conn must be non-nil; close cleanly.
    - TestHandleCapabilitiesWS_Snapshot_Empty: dial; read first message; assert event=REGISTRY_UPDATED, payload.change=SNAPSHOT, payload.capabilities=[] (not null). Then close.
    - TestHandleCapabilitiesWS_Snapshot_WithData: upsert a manifest via direct store.Upsert (bypass REST — simpler for this unit); dial; read first message; assert payload.change=SNAPSHOT and payload.capabilities has 1 entry matching the upserted capability.
    - TestHandleCapabilitiesWS_SubscribesAfterSnapshot: dial; read snapshot; then direct store.Upsert of a new manifest + s.hub.Publish(newRegistryUpdatedEvent(id, changeAdded)); use require.Eventually (500ms max, 10ms tick) to read the next WS message; assert it's an ADDED event for the new manifest. **No time.Sleep** — PITFALLS #16. Actually simpler: set a read deadline via ctx with 2s timeout, then conn.Read; if the Publish succeeded, the message arrives within ms.
    - Use `import websocket "github.com/coder/websocket"` per Phase 3 convention.
  </behavior>
  <action>
    Append to `internal/httpapi/events_test.go`:

    ```go
    func TestNewSnapshotEvent(t *testing.T) {
        caps := []registry.CapabilityView{
            {
                AppID:   "mail-app",
                AppName: "Mail",
                Action:  "PICK",
                Path:    "/pick",
                Properties: registry.CapabilityProps{
                    MimeTypes: []string{"text/plain"},
                },
            },
        }
        raw := newSnapshotEvent(caps)
        var evt registryUpdatedEvent
        require.NoError(t, json.Unmarshal(raw, &evt))
        require.Equal(t, "REGISTRY_UPDATED", evt.Event)
        require.Equal(t, changeSnapshot, evt.Payload.Change)
        require.Len(t, evt.Payload.Capabilities, 1)
        require.Equal(t, "mail-app", evt.Payload.Capabilities[0].AppID)
        // appId omitted on snapshot events
        require.Empty(t, evt.Payload.AppID)
        // Timestamp parses
        _, err := time.Parse("2006-01-02T15:04:05.000Z07:00", evt.Timestamp)
        require.NoError(t, err)
    }

    func TestNewSnapshotEvent_EmptyList(t *testing.T) {
        raw := string(newSnapshotEvent([]registry.CapabilityView{}))
        // Must NOT be "null" for the capabilities field — clients do
        // `state = event.payload.capabilities` and expect an array.
        require.Contains(t, raw, `"capabilities":[]`)
        require.NotContains(t, raw, `"capabilities":null`)
    }
    ```

    Add the necessary import to `events_test.go`:
    ```go
    import (
        // ... existing ...
        "github.com/openburo/openburo-server/internal/registry"
    )
    ```

    Create `internal/httpapi/handlers_caps_test.go`:

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

        // ?action=PICK → 1
        r, _ := ts.Client().Get(ts.URL + "/api/v1/capabilities?action=PICK")
        body, _ := io.ReadAll(r.Body)
        r.Body.Close()
        var resp struct {
            Count int `json:"count"`
        }
        require.NoError(t, json.Unmarshal(body, &resp))
        require.Equal(t, 1, resp.Count)

        // ?action=SAVE → 0
        r2, _ := ts.Client().Get(ts.URL + "/api/v1/capabilities?action=SAVE")
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
        r, _ := ts.Client().Get(ts.URL + "/api/v1/capabilities?mimeType=text/plain")
        body, _ := io.ReadAll(r.Body)
        r.Body.Close()
        require.NoError(t, json.Unmarshal(body, &resp))
        require.Equal(t, 1, resp.Count)

        // Wildcard symmetric
        r2, _ := ts.Client().Get(ts.URL + "/api/v1/capabilities?mimeType=*/*")
        body2, _ := io.ReadAll(r2.Body)
        r2.Body.Close()
        require.NoError(t, json.Unmarshal(body2, &resp))
        require.Equal(t, 1, resp.Count)

        // Non-matching
        r3, _ := ts.Client().Get(ts.URL + "/api/v1/capabilities?mimeType=image/png")
        body3, _ := io.ReadAll(r3.Body)
        r3.Body.Close()
        require.NoError(t, json.Unmarshal(body3, &resp))
        require.Equal(t, 0, resp.Count)
    }

    func TestHandleCapabilities_MalformedMime(t *testing.T) {
        srv := newTestServer(t)
        ts := httptest.NewServer(srv.Handler())
        defer ts.Close()

        r, _ := ts.Client().Get(ts.URL + "/api/v1/capabilities?mimeType=notamime")
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
    ```

    Run — MUST fail to compile. handleCapabilities, handleCapabilitiesWS, newSnapshotEvent, buildFullStateSnapshot all undeclared. RED committed.

    Commit: `test(04-04): add failing tests for capabilities REST + WS upgrade + snapshot-on-connect`
  </action>
  <verify>
    <automated>cd /home/ben/Dev-local/openburo-spec/open-buro-server &amp;&amp; ~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -timeout 30s 2>&amp;1 | head -40 ; echo "EXPECT: undefined handleCapabilities, handleCapabilitiesWS, newSnapshotEvent"</automated>
  </verify>
  <acceptance_criteria>
    - Files exist: `test -f internal/httpapi/handlers_caps_test.go`
    - TestNewSnapshotEvent + TestNewSnapshotEvent_EmptyList added to events_test.go: `grep -c "^func TestNewSnapshotEvent" internal/httpapi/events_test.go → 2`
    - handlers_caps_test.go has ≥7 test functions: `grep -c "^func TestHandleCapabilities" internal/httpapi/handlers_caps_test.go → ≥7`
    - WS tests use coder/websocket import: `grep -c '"github.com/coder/websocket"' internal/httpapi/handlers_caps_test.go → 1`
    - No time.Sleep in the new test file: `! grep -n 'time\.Sleep' internal/httpapi/handlers_caps_test.go`
    - Snapshot test asserts capabilities not null and not missing: `grep -c '"capabilities":\[\]' internal/httpapi/events_test.go → ≥1`
    - Snapshot-then-publish test exists: `grep -c "TestHandleCapabilitiesWS_SubscribesAfterSnapshot" internal/httpapi/handlers_caps_test.go → 1`
    - RED state verified: `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -timeout 30s 2>&1 | grep -cE 'undefined.*(handleCapabilities|newSnapshotEvent|buildFullStateSnapshot)' → ≥1`
    - gofmt clean on new files
  </acceptance_criteria>
  <done>RED committed: 2 new events tests + 7+ capabilities tests on disk covering REST filtering, malformed mime rejection, WS upgrade, empty snapshot, populated snapshot, and subscribe-after-snapshot continuity. Production code does not exist; compilation fails with undefined symbols.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: GREEN — implement handlers_caps.go + newSnapshotEvent + wire handlers in registerRoutes</name>
  <files>
    internal/httpapi/events.go (modify — add newSnapshotEvent),
    internal/httpapi/handlers_caps.go (create),
    internal/httpapi/server.go (modify — replace two caps stub501 calls)
  </files>
  <read_first>
    .planning/phases/04-http-api/04-CONTEXT.md,
    .planning/phases/04-http-api/04-RESEARCH.md,
    .planning/research/PITFALLS.md,
    internal/httpapi/events.go,
    internal/httpapi/handlers_caps_test.go,
    internal/httpapi/handlers_registry.go,
    internal/httpapi/server.go,
    internal/registry/store.go,
    internal/registry/mime.go,
    internal/wshub/subscribe.go
  </read_first>
  <action>
    Append to `internal/httpapi/events.go`:

    ```go
    // newSnapshotEvent builds a REGISTRY_UPDATED event carrying the full
    // capability list. This is the initial message sent to every new
    // WebSocket subscriber (WS-06) — clients do
    //   state = event.payload.capabilities
    // and then refetch the REST endpoint on any subsequent event.
    //
    // The payload.change is SNAPSHOT, payload.appId is omitted (omitempty),
    // and payload.capabilities is ALWAYS a non-nil slice (possibly empty)
    // so the consumer can always do `state = event.payload.capabilities`
    // without a null check.
    func newSnapshotEvent(caps []registry.CapabilityView) []byte {
        if caps == nil {
            caps = []registry.CapabilityView{}
        }
        evt := registryUpdatedEvent{
            Event:     "REGISTRY_UPDATED",
            Timestamp: time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00"),
            Payload: eventPayload{
                Change:       changeSnapshot,
                Capabilities: caps,
            },
        }
        b, _ := json.Marshal(evt)
        return b
    }
    ```

    Create `internal/httpapi/handlers_caps.go`:

    ```go
    package httpapi

    import (
        "context"
        "encoding/json"
        "net/http"
        "time"

        "github.com/coder/websocket"
        "github.com/openburo/openburo-server/internal/registry"
    )

    // handleCapabilities returns the filtered capability list under
    // {capabilities:[], count:N}. Public route (no auth).
    //
    // Query params:
    //   ?action=PICK|SAVE — exact-match filter (empty = no filter)
    //   ?mimeType=X/Y    — symmetric wildcard MIME match via Store.Capabilities
    //
    // Malformed mimeType (one that CanonicalizeMIME rejects) returns 400.
    // This is the "callers wanting a 400 should pre-validate" path that
    // Plan 02-03 documented in the Store.Capabilities contract.
    func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
        defer r.Body.Close()

        filter := registry.CapabilityFilter{
            Action:   r.URL.Query().Get("action"),
            MimeType: r.URL.Query().Get("mimeType"),
        }

        // Pre-validate mimeType so callers see a 400 on malformed input
        // rather than a silent empty result (which store.Capabilities
        // would otherwise return per its documented lenient contract).
        if filter.MimeType != "" {
            canonical, err := registry.CanonicalizeMIME(filter.MimeType)
            if err != nil {
                writeBadRequest(w, "invalid mime type", map[string]any{
                    "mimeType": filter.MimeType,
                    "reason":   err.Error(),
                })
                return
            }
            filter.MimeType = canonical
        }

        caps := s.store.Capabilities(filter)
        if caps == nil {
            caps = []registry.CapabilityView{}
        }

        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        _ = json.NewEncoder(w).Encode(struct {
            Capabilities []registry.CapabilityView `json:"capabilities"`
            Count        int                       `json:"count"`
        }{
            Capabilities: caps,
            Count:        len(caps),
        })
    }

    // buildFullStateSnapshot marshals a snapshot event carrying the full
    // unfiltered capability list. Called by handleCapabilitiesWS once per
    // new subscriber, BEFORE handoff to hub.Subscribe, so WS-06 ordering
    // is guaranteed: snapshot first, then subsequent events.
    func (s *Server) buildFullStateSnapshot() []byte {
        // Empty filter = no filter = full list.
        caps := s.store.Capabilities(registry.CapabilityFilter{})
        return newSnapshotEvent(caps)
    }

    // handleCapabilitiesWS upgrades the request to a WebSocket and hands
    // off to the hub after sending the full-state snapshot as the first
    // message (WS-06).
    //
    // OriginPatterns are sourced from s.cfg.AllowedOrigins — the SAME
    // allow-list as CORS (WS-08 — shared allow-list). InsecureSkipVerify
    // is NEVER set. Reviewers should grep for it to prove it's absent.
    //
    // Accept writes the handshake response on success OR the 403 rejection
    // on origin mismatch — on error, the handler just returns.
    func (s *Server) handleCapabilitiesWS(w http.ResponseWriter, r *http.Request) {
        conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
            OriginPatterns: s.cfg.AllowedOrigins,
            // InsecureSkipVerify is NEVER set. PITFALLS #7 anchor.
        })
        if err != nil {
            // websocket.Accept wrote the rejection response already
            // (403 on origin mismatch). Just return.
            return
        }

        // WS-06: send the full-state snapshot BEFORE entering hub.Subscribe.
        // This eliminates the connect-then-fetch race: clients that receive
        // the snapshot then observe subsequent events in order.
        snapshot := s.buildFullStateSnapshot()
        writeCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
        err = conn.Write(writeCtx, websocket.MessageText, snapshot)
        cancel()
        if err != nil {
            s.logger.Warn("httpapi: snapshot write failed",
                "error", err.Error())
            _ = conn.Close(websocket.StatusInternalError, "snapshot write failed")
            return
        }

        // Hand off to the hub. Subscribe blocks until disconnect, context
        // cancel, hub.Close, or an error on the writer side.
        _ = s.hub.Subscribe(r.Context(), conn)
    }
    ```

    Modify `internal/httpapi/server.go` `registerRoutes` — replace the two caps stub501 lines:

    ```go
    func (s *Server) registerRoutes() {
        // Phase 1 route (unchanged)
        s.mux.HandleFunc("GET /health", s.handleHealth)

        // Phase 4 write routes (auth required)
        s.mux.Handle("POST /api/v1/registry",
            s.authBasic(http.HandlerFunc(s.handleRegistryUpsert)))
        s.mux.Handle("DELETE /api/v1/registry/{appId}",
            s.authBasic(http.HandlerFunc(s.handleRegistryDelete)))

        // Phase 4 read routes (public)
        s.mux.HandleFunc("GET /api/v1/registry", s.handleRegistryList)
        s.mux.HandleFunc("GET /api/v1/registry/{appId}", s.handleRegistryGet)
        s.mux.HandleFunc("GET /api/v1/capabilities", s.handleCapabilities)
        s.mux.HandleFunc("GET /api/v1/capabilities/ws", s.handleCapabilitiesWS)
    }
    ```

    Delete `stub501` from server.go — it has no callers now. (If the compiler complains about unused, remove the function.) Actually keep it for reviewer clarity with a doc-comment noting it is no longer wired — or delete it. **Decision: delete stub501 entirely** now that all 6 routes are wired; dead code confuses readers.

    Run the full suite:

    ```bash
    ~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -count=1 -timeout 90s
    ```

    Check architectural gates:
    ```bash
    ! grep -rn 'InsecureSkipVerify' internal/httpapi/*.go | grep -v _test.go
    ! ~/sdk/go1.26.2/bin/go list -deps ./internal/registry | grep -E 'wshub|httpapi'
    ```

    NOTE: The Plan 04-03 `TestPublicRoutes` test treated `/api/v1/capabilities` as a public route that must not return 401. Now that the capabilities handler is wired, that route returns 200 (not 501). The assertion `require.NotEqual(t, http.StatusUnauthorized, ...)` still passes. No test edits needed for that one.

    Commit: `feat(04-04): implement capabilities REST handler + WS upgrade with snapshot-on-connect`
  </action>
  <verify>
    <automated>cd /home/ben/Dev-local/openburo-spec/open-buro-server &amp;&amp; ~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -count=1 -timeout 90s &amp;&amp; ! grep -rn 'InsecureSkipVerify' internal/httpapi/*.go | grep -v _test.go &amp;&amp; ! ~/sdk/go1.26.2/bin/go list -deps ./internal/registry | grep -E 'wshub|httpapi' &amp;&amp; ~/sdk/go1.26.2/bin/go vet ./internal/httpapi/... &amp;&amp; test -z "$(~/sdk/go1.26.2/bin/gofmt -l internal/httpapi/)"</automated>
  </verify>
  <acceptance_criteria>
    - Files exist: `test -f internal/httpapi/handlers_caps.go`
    - newSnapshotEvent added to events.go: `grep -c "^func newSnapshotEvent" internal/httpapi/events.go → 1`
    - newSnapshotEvent guarantees non-nil slice: `grep -c 'caps = \[\]registry.CapabilityView{}' internal/httpapi/events.go → 1`
    - handleCapabilities implemented: `grep -c "^func (s \*Server) handleCapabilities(" internal/httpapi/handlers_caps.go → 1`
    - handleCapabilitiesWS implemented: `grep -c "^func (s \*Server) handleCapabilitiesWS(" internal/httpapi/handlers_caps.go → 1`
    - buildFullStateSnapshot implemented: `grep -c "^func (s \*Server) buildFullStateSnapshot()" internal/httpapi/handlers_caps.go → 1`
    - CanonicalizeMIME pre-validation: `grep -c "registry.CanonicalizeMIME" internal/httpapi/handlers_caps.go → 1`
    - WS upgrade uses OriginPatterns from s.cfg.AllowedOrigins: `grep -c "OriginPatterns: s.cfg.AllowedOrigins" internal/httpapi/handlers_caps.go → 1`
    - Snapshot sent BEFORE hub.Subscribe (ordering): the `conn.Write(...)` call must appear BEFORE the `s.hub.Subscribe(...)` call in the function body: `awk '/func .s .Server. handleCapabilitiesWS/,/^}/' internal/httpapi/handlers_caps.go | grep -n 'conn.Write\|s.hub.Subscribe' | head -5` (Write line number must be less than Subscribe line number)
    - InsecureSkipVerify absent from production: `! grep -rn 'InsecureSkipVerify' internal/httpapi/*.go | grep -v _test.go` exits 0
    - Architectural gate holds: `! ~/sdk/go1.26.2/bin/go list -deps ./internal/registry | grep -E 'wshub|httpapi'` exits 0
    - stub501 removed: `! grep -n 'stub501' internal/httpapi/server.go`
    - registerRoutes wires both caps handlers: `grep -c "s.handleCapabilities" internal/httpapi/server.go → 2`
    - Named tests pass:
      - `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestHandleCapabilities$' -timeout 15s` exits 0
      - `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestHandleCapabilities_' -timeout 15s` exits 0
      - `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestHandleCapabilitiesWS_Upgrade$' -timeout 15s` exits 0
      - `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestHandleCapabilitiesWS_Snapshot' -timeout 15s` exits 0
      - `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestHandleCapabilitiesWS_SubscribesAfterSnapshot$' -timeout 15s` exits 0
      - `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestNewSnapshotEvent' -timeout 10s` exits 0
    - Full suite green: `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -count=1 -timeout 90s` exits 0
    - go vet + gofmt clean
  </acceptance_criteria>
  <done>GREEN: handleCapabilities returns filtered capability list with 400 on malformed mime, handleCapabilitiesWS upgrades via websocket.Accept with OriginPatterns from s.cfg.AllowedOrigins, the snapshot event is written BEFORE hub.Subscribe runs (WS-06 ordering), newSnapshotEvent always emits a non-nil capabilities array, stub501 is deleted, InsecureSkipVerify gate still passes, and architectural registry-isolation gate still passes.</done>
</task>

</tasks>

<verification>
```bash
# 1. Full httpapi suite race-clean
~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -count=1 -timeout 120s

# 2. Named tests
~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestHandleCapabilities|^TestNewSnapshotEvent' -timeout 30s

# 3. Architectural gates
! ~/sdk/go1.26.2/bin/go list -deps ./internal/registry | grep -E 'wshub|httpapi'
! grep -rn 'InsecureSkipVerify' internal/httpapi/*.go | grep -v _test.go
! grep -rE 'slog\.Default' internal/httpapi/*.go | grep -v _test.go

# 4. Snapshot ordering (static)
awk '/func .s .Server. handleCapabilitiesWS/,/^}/' internal/httpapi/handlers_caps.go | grep -n 'conn.Write\|s.hub.Subscribe'

# 5. Format + vet
~/sdk/go1.26.2/bin/go vet ./internal/httpapi/...
test -z "$(~/sdk/go1.26.2/bin/gofmt -l internal/httpapi/)"
```
</verification>

<success_criteria>
- GET /api/v1/capabilities returns filtered `{capabilities:[], count:N}` with 400 on malformed mimeType
- GET /api/v1/capabilities/ws upgrades successfully via websocket.Accept with OriginPatterns = s.cfg.AllowedOrigins
- First WebSocket message on connect is a SNAPSHOT event with payload.capabilities populated (or empty array, never null)
- Snapshot is written BEFORE hub.Subscribe is called (WS-06 ordering)
- InsecureSkipVerify grep gate passes (never set in production)
- Registry architectural isolation gate still passes
- Full httpapi suite green under -race, go vet + gofmt clean
</success_criteria>

<output>
After completion, create `.planning/phases/04-http-api/04-04-SUMMARY.md`. Plan 04-05 ships CORS middleware via rs/cors, the big REST + WS integration round-trip tests, the WS origin-rejection test, and the final architectural gate sweep.
</output>
