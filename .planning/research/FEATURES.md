# Feature Research

**Project:** OpenBuro Server (Go app registry + capability broker, reference implementation)
**Domain:** Manifest registry / intent-resolver / capability broker (Android-intent / Cozy-intent / XDG-portal family)
**Researched:** 2026-04-09
**Confidence:** HIGH (prior art corroborated by PROJECT.md, the OpenBuro design dossier, Cozy Stack docs, Android docs, XDG portal docs)

## Framing

This is **not** a generic REST CRUD server. The domain is **capability brokerage**: a small number of apps register manifests describing what they can do (`action` + `mimeType[]`), and client apps query the broker to discover who can fulfill an intent. Feature decisions must be judged against four existing systems, not against "what does a typical backend expose":

| System | Registry | Resolver | Notifications | Role it plays here |
|--------|----------|----------|---------------|--------------------|
| **Android `PackageManager` / `IntentResolver`** | PackageManager (installed APKs) | Action + category + data (URI + MIME) matching | `BroadcastReceiver` for `PACKAGE_ADDED` / `PACKAGE_REMOVED` / `PACKAGE_REPLACED` | MIME matching semantics, event taxonomy |
| **Cozy Stack intents** | App manifests stored in CouchDB | `action` + `type` (MIME or doctype) traversal | No push notifications — clients re-fetch | Closest architectural twin; resolver semantics, `availableApps` concept |
| **XDG `.desktop` + `xdg-mime`** | Desktop entry files on disk | MIME cache (`mimeinfo.cache`) | Inotify on desktop files | File-backed storage pattern |
| **XDG Desktop Portal** | DBus well-known name | Per-portal interface dispatch | DBus signals (`Response`) | Request/response pattern for capability invocation |

The reference implementation goal reshapes priorities: **clarity > completeness**. Every feature must earn its place by being either (a) essential to demonstrate the broker pattern, or (b) a clear illustration of a protocol decision that downstream implementers need to understand.

## Feature Landscape

### Table Stakes (the broker is useless without these)

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| **Manifest schema validation** | Garbage manifests poison the resolver; a broker that accepts invalid capabilities is worse than no broker | LOW | Required fields: `id` (non-empty, URL-safe), `name`, `url` (must parse as absolute URL with http/https scheme), `version`, `capabilities[]` (≥ 0, but each entry must validate). Per-capability: `action` (enum), `mimeTypes[]` (non-empty, each a valid MIME pattern). Reject unknown top-level fields in strict mode, accept in lenient mode. Return `400` with per-field error list, not the first error only. |
| **Capability action enum** | Without a closed enum, two apps will use `Pick` and `pick` and never resolve | LOW | Start with the two from PROJECT.md: `PICK`, `SAVE`. Case-sensitive, uppercase (matches Android/Cozy convention). Validate at manifest upsert time. Document "v1 is PICK + SAVE; extension is a v2 conversation." |
| **MIME type validation** | A MIME string like `image` (missing slash) or `image/*/thumb` will silently never match — validation at write time prevents ghost capabilities | LOW | Accept `type/subtype`, `type/*`, and `*/*`. Reject empty segments, whitespace, uppercase (RFC says case-insensitive but Android normalizes to lowercase and that's the safer convention). Reject `*/subtype` as malformed (it's nonsense and Android rejects it too). |
| **Upsert semantics** (PUT/POST with same `appId` replaces) | Every intent-resolver in the prior art treats manifest updates as replacement, not merge. Merge creates stale capabilities that never disappear | LOW | `POST /api/v1/registry` with existing `appId` returns `200`, new `appId` returns `201`. Body fully replaces prior manifest. Document this loudly — it's the single biggest "wait, really?" moment for readers. |
| **Delete by `appId`** | Apps get uninstalled; without delete, the broker accumulates zombie capabilities | LOW | `DELETE /api/v1/registry/{appId}` → `204` on success, `404` if unknown. Must fire a `REGISTRY_UPDATED` event. |
| **List all manifests** | Admin tooling, debugging, and "what's registered right now" are all the same question | LOW | `GET /api/v1/registry` returns `{ "apps": [...] }`, not a bare array — wrapped responses are easier to evolve without breaking clients. |
| **Fetch one manifest by `appId`** | Clients may need the full manifest (not just the capability view) to render "app info" | LOW | `GET /api/v1/registry/{appId}` → `200` with full manifest or `404`. |
| **Aggregate capabilities view** | This is the **whole point of the broker** — a flat, queryable capability list abstracted from manifests | LOW | `GET /api/v1/capabilities` returns `{ "capabilities": [{ action, mimeTypes, appId, appName, url }, ...] }`. Denormalize `appId`/`appName`/`url` into each capability entry so a client can act on one without re-fetching the manifest. |
| **Filter capabilities by `action`** | A client asking "who can PICK?" shouldn't have to get SAVE entries | LOW | `?action=PICK`. Exact match, case-sensitive. Unknown action returns empty list, not `400` — this lets clients probe for future actions safely. |
| **Filter capabilities by `mimeType` with wildcard matching** | This is the resolver — the heart of the broker | MEDIUM | `?mimeType=image/png` must match capabilities declaring `image/png`, `image/*`, or `*/*`. This is the single most important correctness concern in the project. See "MIME matching semantics" box below. |
| **WebSocket endpoint for change notifications** | Without push, clients poll, which defeats the "instantly aware of new capabilities" value prop from PROJECT.md Core Value | MEDIUM | `GET /api/v1/capabilities/ws`. On any registry mutation, broadcast a single message to all connected clients. Use `coder/websocket` (per STACK.md). |
| **WebSocket keepalive (ping)** | Browsers, proxies, and load balancers silently kill idle connections at 30-120s; without ping the "instant notification" promise breaks after the first minute | LOW | Periodic server-initiated ping frames, configurable (default 30s per PROJECT.md). `coder/websocket` has `Ping(ctx)` built in. Read deadline = 2× ping interval; close on timeout. |
| **HTTP Basic Auth on write routes** | Anyone can upsert would make the registry untrustworthy even for a demo | LOW | `POST` and `DELETE` only. `credentials.yaml` with bcrypt hashes (cost ≥ 12 per PROJECT.md). `WWW-Authenticate: Basic realm="openburo"` on 401. Read routes (GET, WS) are deliberately open per PROJECT.md. |
| **Constant-time credential comparison** | Timing-leaking auth on a "reference implementation" is a bad look and actively mis-educates readers | LOW | `bcrypt.CompareHashAndPassword` is already constant-time for the hash check. For the username, use `subtle.ConstantTimeCompare` or iterate the credentials map regardless of which user was presented. |
| **Persist registry to disk** | Without persistence, a restart erases the registry and every client loses its capability list silently | LOW | Write `registry.json` after every successful mutation. Atomic write via temp file + `os.Rename` (stdlib, POSIX-atomic). Load at startup; tolerate missing file (= empty registry) but fail fast on corrupted JSON. |
| **`GET /health` liveness** | Every Go service shipped to any container runtime needs this, and deploys break without it | LOW | `200 OK` with `{"status":"ok"}`. No auth. No external dependency checks (it's a liveness probe, not a readiness probe — see below). |
| **Structured request logging** | Debugging a broker without logs is impossible when a resolver returns the wrong capability | LOW | `log/slog` JSON handler in prod, text in dev. Log per-request: method, path, status, duration, remote_addr. Elide Authorization header and anything credential-shaped. |
| **CORS for browser clients** | PROJECT.md names browser clients as a target consumer, and browsers will refuse cross-origin fetch + WebSocket without CORS | LOW | `rs/cors` middleware (per STACK.md). Allow `GET` and `OPTIONS` publicly; allow `POST`/`DELETE` only from configured admin origins. See "CORS for WebSocket" below — it's a gotcha. |
| **WebSocket origin check** | Without it, a malicious site can open a WebSocket from a victim's browser and read the registry (CSRF-over-WebSocket) | LOW | `coder/websocket`'s `AcceptOptions.OriginPatterns`. Share the allow-list with the HTTP CORS config so there's one source of truth. |
| **Graceful shutdown** | SIGTERM without graceful shutdown drops in-flight WebSocket connections without a close frame, leaving clients in a confusing reconnect loop | LOW | `http.Server.Shutdown(ctx)` with a 10s budget. Close the WebSocket hub first (broadcast a close frame with code 1001 "going away"), then `Shutdown` the HTTP server. |
| **Startup banner log** | "What config is this process actually running with?" is the first question during every demo failure | LOW | One structured log line at startup with: version, config file path, listen addr, TLS on/off, ping interval, registry file path, credential-file checksum (not contents). |

### Differentiators (what makes this reference implementation stand out)

These are features that elevate OpenBuro Server from "yet another CRUD server with WebSocket" to "clear reference for the capability-broker pattern."

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **Correct MIME wildcard matching in both directions** | Cozy Stack's current intent resolver is a known source of frustration (explicitly called out in PROJECT.md dossier: "avec défaut connus :-)"). Getting this right on day one differentiates OpenBuro from its own prior art | MEDIUM | The resolver must handle the case where **the capability declares `image/*` and the client queries `image/png`** (hierarchical match in capability direction) **and** the case where **the capability declares `image/png` and the client queries `image/*`** (hierarchical match in query direction). Android does both. See the MIME matching box below for exact rules. Include a table-driven test that exhaustively covers the 3×3 matrix: {exact, type-wildcard, star-star} × {exact, type-wildcard, star-star}. |
| **Single event type, denormalized payload** | Prior art (Android) emits separate `PACKAGE_ADDED` / `REMOVED` / `REPLACED`. That forces clients to reconcile three event streams against their local state. Emitting one `REGISTRY_UPDATED` event with the full post-change capability list lets the client do a cheap replace-all and eliminates reconcile bugs entirely | LOW | Event payload: `{"type":"REGISTRY_UPDATED","timestamp":"...","capabilities":[...]}`. The client's update loop is `state.capabilities = event.capabilities`. Done. This is a deliberate choice *against* Android's event taxonomy; document the tradeoff in the README. |
| **No subscribe/unsubscribe WebSocket protocol** | Subscription semantics are a classic yak-shave. The broker has one thing to tell clients ("the registry changed") so every connected client wants every event. No topic routing, no subscribe/unsubscribe, just connect → receive | LOW | Document this explicitly. When a v2 broker needs multi-tenant, *that's* when you add channels. Not before. |
| **Hub pattern with deterministic fan-out** | The `coder/websocket` concurrent-write safety eliminates the #1 gorilla/websocket bug; combined with a channel-based hub it gives the cleanest possible fan-out code — exactly what a reference implementation should showcase | MEDIUM | Hub has: `register chan *client`, `unregister chan *client`, `broadcast chan []byte`, `clients map[*client]struct{}`, single goroutine that owns all mutations. Each client has a buffered outbound channel; if the buffer fills, disconnect the slow client (don't block the hub). This pattern is the canonical Go WebSocket hub; it deserves to be shown clearly. |
| **Event coalescing (short debounce)** | A batch upsert of 10 apps should emit 1 event, not 10 — and the client's UI shouldn't flicker 10 times | LOW-MEDIUM | Optional but high-value. After a mutation, instead of broadcasting immediately, start a 50-100ms timer; if another mutation arrives, reset the timer; when the timer fires, broadcast the current state. Worst-case latency: ~100ms. Benefit: one event for bulk operations. Document the debounce window in config. If deemed too complex for v1, defer to v1.1 and document as "known suboptimal." |
| **Full-state broadcast on connect** | A new client shouldn't have to make a separate `GET /capabilities` call to initialize state — it's a guaranteed source of race conditions (connect → fetch window where an event is missed) | LOW | On WS accept, immediately send a `REGISTRY_UPDATED` event containing the current capability list *before* subscribing the client to the broadcast channel. Atomic under the hub's single-goroutine ownership. Eliminates the connect-then-fetch race by design. |
| **Reconnect hint in close frame** | When the server closes (shutdown, restart, ping timeout), send a close frame with a reason code the client can reason about | LOW | Use standard codes: `1001` (going away) on shutdown, `1000` (normal) on idle close. Don't invent custom codes. Document in the README: "on close 1001, wait 1s and reconnect; on close 1000, don't auto-reconnect." |
| **`GET /readyz` readiness endpoint (separate from `/health`)** | Liveness and readiness are different questions. Liveness: "is the process alive?" Readiness: "is the registry loaded and ready to serve?" Conflating them in one endpoint causes deploy issues. PROJECT.md only mandates `/health` but splitting is trivial and educational | LOW | `/health` = liveness = always 200 once the HTTP server is up. `/readyz` = 200 once `registry.json` is loaded AND the WebSocket hub is running. This distinction is standard in Kubernetes and worth demonstrating. |
| **Example manifests in the repo** | A reference implementation that doesn't ship canonical examples is 30% less useful. Ship `examples/manifest-tdrive.json`, `examples/manifest-nextcloud.json` | LOW | Use the real-world drives from PROJECT.md Context section (TDrive, Fichier DINUM, Nextcloud). These double as integration-test fixtures and as the README's copy-paste quickstart. |
| **Human-readable `registry.json`** (indented) | When the file IS the database, someone will `cat` it. Make that someone happy | LOW | `json.MarshalIndent(registry, "", "  ")`. Negligible perf cost for the expected registry size (~kB, not MB). |
| **Deterministic capability ordering in responses** | Two consecutive `GET /capabilities` calls with no mutations in between should return the same JSON byte-for-byte. This is not required but it makes HTTP caching trivial and eliminates a class of "tests are flaky" complaints | LOW | Sort capabilities by `(appId, action, mimeType[0])` before serializing. Sort `mimeTypes[]` within each capability. Map iteration in Go is intentionally randomized, so this sort is load-bearing. |
| **Minimal audit log on write operations** | PROJECT.md says "Credentials never logged" — the flip side is that legitimate admin actions SHOULD be logged. Every upsert and delete writes a structured `slog.Info` line with `{user, action, appId, capability_count}` | LOW | Hackathon-appropriate observability; no separate audit sink, just structured logs grepped by `audit=true` attribute. |

### Anti-Features (seductive but wrong for v1)

These are features that sound reasonable, are often requested, and should be **deliberately not built** because they would compromise the reference-implementation goal.

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| **Pluggable storage backend (SQLite/Postgres)** | "Production-readiness" instinct | Out of scope per PROJECT.md. Adds ORM/driver dependencies, migration machinery, and conflict-resolution logic — none of which teach anything about the capability-broker pattern. The storage layer would become the biggest file in the repo and distract from the actual value proposition | JSON file + `sync.RWMutex`. Document the upgrade path in README ("when your registry exceeds X, switch to Y"). |
| **JSON Schema validation of the manifest** | Schemas feel more rigorous than hand-rolled validation | A hand-rolled Go struct with validation methods is ~50 lines, zero deps, and readable. A JSON Schema approach needs `github.com/santhosh-tekuri/jsonschema/v5` or similar, a schema file that duplicates the Go types, and a whole "which source of truth wins" conversation. For 6-8 fields, the complexity is backwards | Manual validation in `registry.Validate()`. Keep schema docs in the README as a table. |
| **Optimistic concurrency / `If-Match` / ETags / 409 conflict** | "What if two admins upsert simultaneously?" | Explicitly out of scope per PROJECT.md ("single-admin assumption for v1"). Adds ETag generation, header parsing, versioned state, and a whole error path. Zero value for a reference impl | Last-write-wins. Document the assumption. |
| **Capability-level IDs** (per-capability UUIDs) | "What if I want to reference a specific capability across manifests?" | Creates an identity-management problem on an ephemeral entity. A capability is fully described by `(appId, action, mimeType)` — that tuple IS its identity | Use the natural key. If a caller wants to refer to a capability, they use the tuple. |
| **Rate limiting on read routes** | "What if someone hammers GET /capabilities?" | PROJECT.md explicitly excludes rate limiting. In-memory reads with RWMutex handle millions of req/sec; the bottleneck is network, not the registry | If abuse becomes a problem, terminate at the reverse proxy (nginx, Caddy), not in application code |
| **WebSocket subscribe/unsubscribe protocol** | "What if a client only cares about certain actions?" | Topic routing adds: message parsing for subscribe/unsubscribe, per-client subscription state, per-event filtering, plus subtle bugs around re-subscription after reconnect. All for a filter the client can apply locally in 5 lines | Client filters the `REGISTRY_UPDATED` payload locally. The broker broadcasts to everyone. |
| **Multiple event types** (`APP_ADDED`, `APP_UPDATED`, `APP_REMOVED`) | Android and similar systems do this; feels like "the right way" | Forces the client into reconciliation logic (maintain local state, apply diffs in order, handle missed events). Our single `REGISTRY_UPDATED` with full post-change payload is strictly easier to consume and impossible to get wrong. Android's taxonomy exists because APKs are huge and sending the full package list on every change would be expensive — that constraint doesn't apply to a small capability registry | Single `REGISTRY_UPDATED` event with the full capability list. |
| **Per-event diffs** (`added: [...], removed: [...]`) | Smaller payloads | Premature optimization. A full capability list for 10-50 apps is a handful of kB. Diffs create consistency-across-reconnect problems. Not worth it | Full state every time. Compress with per-message deflate if it ever becomes a real problem. |
| **Rich OAuth/OIDC auth** | "Basic Auth is insecure" | PROJECT.md constraint: no OAuth in v1. Basic Auth over TLS + bcrypt is fine for a reference impl. Adding OIDC means: discovery, JWKs cache, token validation, refresh, client registration, a whole package. All to replace a 10-line `BasicAuth` middleware | Basic Auth over TLS only. Document the v2 upgrade path. |
| **Hot-reload of credentials / config** | "I want to add an admin without restarting" | Explicitly out of scope per PROJECT.md. File watchers + safe reload = non-trivial, and `docker compose restart openburo` takes 2 seconds | Restart to reload. |
| **Multi-tenancy / namespaced registries** | "What if multiple orgs share the server?" | Explicitly out of scope per PROJECT.md. Namespacing touches every route, every event payload, every auth decision. Not a 10% change — a complete reshape | One registry per server instance. Deploy N instances for N tenants. |
| **Capability invocation through the broker** | "Why can't the broker also fulfill the intent?" | That's what openDesk's Intercom Service does, and it concentrates all trust in the broker (PROJECT.md dossier: "pas de zero trust"). OpenBuro's whole architectural bet is that the client opens the capability URL directly, keeping the broker out of the data path. Building an invocation endpoint would betray the core design | Broker returns capability URLs. Client calls the capability. Broker never sees the actual intent payload. |
| **Capability request signing / intent payload validation** | "What if a client submits a malformed intent to a service?" | That's the service's problem, not the broker's. The broker only knows which services exist; it has no semantic knowledge of what a valid `PICK image/png` request looks like | Service validates its own incoming requests. Not the broker's concern. |
| **Prometheus metrics / `/metrics` endpoint** | "Production servers always expose metrics" | PROJECT.md explicitly says no metrics stack. slog request logs cover the observability needs of a reference impl | Structured logs. Aggregate in external tooling if needed. |
| **Distributed tracing / OpenTelemetry** | Same impulse as metrics | Same constraint. OTel SDK + exporter setup is 200 lines and 15 deps for a server with ~5 routes | slog `request_id` on every log line is 3 lines and covers correlation |
| **gRPC API alongside REST** | "gRPC is faster" | Irrelevant at this scale. Doubles the API surface, duplicates validation, confuses readers about which API is canonical | REST + WebSocket only. |
| **WebSocket compression (permessage-deflate)** | "Save bandwidth" | `coder/websocket` supports it but compression adds CPU and memory. For kB-sized capability payloads the compression ratio matters less than the latency cost | Off by default. Enable if capability list grows past ~50kB. |
| **TLS mutual authentication (mTLS) for write routes** | "Stronger than Basic Auth" | PROJECT.md constraint: Basic Auth only in v1. mTLS requires CA management, cert distribution, renewal — enormous operational burden for zero hackathon value | Basic Auth over TLS. |
| **GraphQL API** | "Clients pick the fields they want" | The response is a capability list. There's nothing to pick. GraphQL solves a problem this API doesn't have | REST. |
| **"PUT" semantics distinct from "POST"** | REST purity | PROJECT.md specifies `POST /api/v1/registry` for upsert. Adding `PUT /api/v1/registry/{appId}` as a second upsert path doubles the surface for no clarity gain | `POST` for upsert (both create and update). One path, 201 vs 200 distinguishes create vs update. |

---

### MIME Matching Semantics (the core correctness concern)

This is the most important spec in the document. Get this wrong and the broker silently ships the wrong capabilities. The rules below synthesize Android's IntentFilter behavior, Cozy Stack's resolver, and the XDG `shared-mime-info` conventions.

**Canonicalization (at validation time):**

1. MIME strings are lowercased.
2. Whitespace is stripped.
3. Parameters (e.g., `; charset=utf-8`) are stripped. The broker matches on `type/subtype` only.
4. Valid forms: `type/subtype`, `type/*`, `*/*`. Any other form (e.g., `*/subtype`, `type`, empty string) is rejected at upsert time.

**Matching rules for resolver query `q` against capability declaration `c`:**

| Capability declares (`c`) | Query (`q`) | Match? | Reasoning |
|---|---|---|---|
| `image/png` | `image/png` | ✅ | Exact |
| `image/png` | `image/jpeg` | ❌ | Subtype differs |
| `image/png` | `image/*` | ✅ | Query is broader; capability satisfies a subset of the query |
| `image/png` | `*/*` | ✅ | Query is "anything" |
| `image/*` | `image/png` | ✅ | Capability is broader; covers the specific query |
| `image/*` | `image/jpeg` | ✅ | Same |
| `image/*` | `text/plain` | ❌ | Type differs |
| `image/*` | `*/*` | ✅ | Query is "anything"; capability satisfies a subset |
| `*/*` | `image/png` | ✅ | Capability handles everything |
| `*/*` | `image/*` | ✅ | Same |
| `*/*` | `*/*` | ✅ | Same |

**Symmetry is intentional:** both the capability side and the query side can use wildcards. If either side is broader, the match succeeds. This matches Android's behavior and Cozy Stack's semantics and is what clients expect.

**No filter = no filter:** `GET /api/v1/capabilities` with no `mimeType` param returns all capabilities regardless of their MIME declarations. Absent filter ≠ `*/*` filter for clarity (though behaviorally they're equivalent).

**Action filter is orthogonal:** `?action=PICK&mimeType=image/png` AND-combines. `?action=PICK` alone returns all PICK capabilities. `?mimeType=image/png` alone returns capabilities for any action matching `image/png`.

---

## Feature Dependencies

```
Manifest validation
    └──enables──> Upsert / Delete
                       └──triggers──> Registry mutation
                                            ├──triggers──> Persist to registry.json
                                            └──triggers──> Hub broadcast
                                                               └──requires──> WebSocket endpoint
                                                                                  └──requires──> Origin check
                                                                                  └──requires──> Ping/keepalive
                                                                                  └──requires──> Hub pattern

Capability aggregation (GET /capabilities)
    └──depends on──> Registry state
    └──depends on──> MIME matching rules
    └──depends on──> Action enum

Basic Auth
    └──depends on──> credentials.yaml loader
    └──protects──> Upsert / Delete (ONLY)
    └──does NOT protect──> GET / WebSocket (per PROJECT.md)

CORS middleware
    └──must agree with──> WebSocket origin check (one allow-list, shared)

Graceful shutdown
    └──requires──> Hub close-broadcast (close code 1001)
    └──requires──> http.Server.Shutdown()

/readyz
    └──depends on──> Registry load complete
    └──depends on──> Hub running

Full-state broadcast on connect
    └──depends on──> Hub pattern (atomic snapshot under hub goroutine)

Event coalescing (debounce)
    └──enhances──> Hub broadcast
    └──should NOT delay──> First event after idle period (only coalesce bursts)
```

### Critical Dependencies (ordering implications for the roadmap)

- **Registry core → persistence → WebSocket hub.** You cannot ship notifications before you have something to notify about.
- **MIME matching logic → resolver endpoint.** The resolver is just a filter over manifests; it's downstream of matching rules.
- **Auth → writes.** You cannot protect writes before you have loaded credentials. And credential loading depends on config loading.
- **Config loading → everything.** The first phase must establish config + logging. All other phases depend on it.
- **CORS + origin check must ship together.** Shipping one without the other creates either a broken browser client or a CSRF window.
- **Graceful shutdown must ship with the hub.** A hub without graceful shutdown is a reconnect-storm generator.

---

## MVP Definition

### Launch With (v1 — Hackathon Reference)

The deliberate minimum to demonstrate the capability-broker pattern end to end. Everything here is Table Stakes.

**Core registry:**
- [ ] Manifest upsert (`POST /api/v1/registry`) with full validation — **essential; without this nothing else matters**
- [ ] Manifest delete (`DELETE /api/v1/registry/{appId}`) — **essential; zombie capabilities ruin demos**
- [ ] List manifests (`GET /api/v1/registry`) — **essential for admin/debug**
- [ ] Fetch one manifest (`GET /api/v1/registry/{appId}`) — **essential for "show me that app's full manifest"**
- [ ] Atomic JSON file persistence — **essential; restart must not erase state**

**Resolver:**
- [ ] `GET /api/v1/capabilities` with full list — **essential; this is the broker's raison d'être**
- [ ] `?action=PICK` filter — **essential; most common query**
- [ ] `?mimeType=...` filter with symmetric wildcard matching — **essential; the single most important correctness concern**

**Notifications:**
- [ ] `GET /api/v1/capabilities/ws` WebSocket endpoint — **essential; fulfills the Core Value "notified instantly" promise**
- [ ] Hub pattern with `coder/websocket` — **essential; the clean fan-out is a large part of the reference value**
- [ ] Full-state broadcast on connect — **essential; eliminates the connect-fetch race**
- [ ] Single `REGISTRY_UPDATED` event with full capability list — **essential; the simplicity is a deliberate teaching point**
- [ ] Periodic ping (30s default) + read deadline — **essential; without it connections die silently**
- [ ] Origin check on WebSocket accept — **essential; CSRF defense**

**Auth:**
- [ ] Basic Auth on POST/DELETE with bcrypt credential verification — **essential; PROJECT.md hard requirement**
- [ ] Credentials loaded from `credentials.yaml` — **essential; PROJECT.md hard requirement**
- [ ] Constant-time comparison — **essential; the reference impl should teach the right pattern**

**Ops:**
- [ ] `/health` liveness — **essential for containerized deployments**
- [ ] `log/slog` structured logging with request + audit lines — **essential; debugging the demo without logs is impossible**
- [ ] `rs/cors` middleware with configurable allowed origins — **essential; browser clients are a target**
- [ ] Graceful shutdown with WebSocket close-broadcast — **essential; reconnect storms are demo-killers**
- [ ] Config loading from `config.yaml` — **essential; PROJECT.md hard requirement**
- [ ] Optional TLS — **essential; PROJECT.md hard requirement**

**Quality:**
- [ ] Table-driven unit tests for MIME matching (exhaustive 3×3 matrix) — **essential; this is the core correctness concern**
- [ ] Integration tests for REST handlers (`httptest.NewServer`) — **essential**
- [ ] Integration test for WebSocket broadcast (full round-trip: upsert → event received) — **essential**

### Add After Validation (v1.1 — if hackathon time permits)

- [ ] **`/readyz` readiness endpoint** — one extra function; worth including if time allows. **Trigger:** first deployment that distinguishes liveness from readiness.
- [ ] **Event coalescing (50-100ms debounce)** — nice-to-have for bulk operations. **Trigger:** first bulk-upsert demo where the UI flickers noticeably.
- [ ] **Reconnect hint in close frame documentation + client guide** — "document the close-code contract." **Trigger:** the first client implementer asks "what do I do on close?"
- [ ] **Example manifests for TDrive / Nextcloud** — the docs layer. **Trigger:** the first "how do I use this" question.

### Future Consideration (v2 — explicitly out of scope per PROJECT.md)

- [ ] Pluggable storage backend (SQLite/Postgres) — only if registry grows past ~10MB
- [ ] Optimistic concurrency with `If-Match`/ETag — only if multi-admin becomes real
- [ ] Hot-reload of config/credentials — only if operational pain is demonstrated
- [ ] Read-route authentication — only if privacy becomes a requirement
- [ ] Multi-tenancy / namespaced registries — only if single-instance-per-tenant becomes impractical
- [ ] OAuth/OIDC authentication — only if Basic Auth is deemed inadequate for a specific deployment
- [ ] Rate limiting — only if abuse is observed (and prefer terminating at a reverse proxy first)
- [ ] Prometheus metrics — only if structured logs prove insufficient
- [ ] Richer event taxonomy (ADDED / REMOVED / UPDATED) — only if payload sizes force diff-based events

---

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Manifest upsert with validation | HIGH | LOW | **P1** |
| Manifest delete | HIGH | LOW | **P1** |
| List / fetch manifests | HIGH | LOW | **P1** |
| Atomic JSON persistence | HIGH | LOW | **P1** |
| Capability aggregation endpoint | HIGH | LOW | **P1** |
| Action filter | HIGH | LOW | **P1** |
| **MIME wildcard matching (symmetric)** | HIGH | **MEDIUM** | **P1** |
| WebSocket endpoint + hub | HIGH | MEDIUM | **P1** |
| Hub fan-out pattern | HIGH | MEDIUM | **P1** |
| Full-state broadcast on connect | HIGH | LOW | **P1** |
| Single `REGISTRY_UPDATED` event | HIGH | LOW | **P1** |
| WebSocket ping/keepalive | HIGH | LOW | **P1** |
| WebSocket origin check | HIGH | LOW | **P1** |
| Basic Auth on writes | HIGH | LOW | **P1** |
| bcrypt credential verification | HIGH | LOW | **P1** |
| Constant-time credential compare | MEDIUM | LOW | **P1** |
| `/health` liveness | HIGH | LOW | **P1** |
| Structured logging | HIGH | LOW | **P1** |
| CORS middleware | HIGH | LOW | **P1** |
| Graceful shutdown | MEDIUM | LOW | **P1** |
| Config loading | HIGH | LOW | **P1** |
| Optional TLS | MEDIUM | LOW | **P1** |
| MIME matching test matrix | HIGH | LOW | **P1** |
| WebSocket integration test | HIGH | LOW | **P1** |
| Event coalescing (debounce) | MEDIUM | LOW-MEDIUM | **P2** |
| `/readyz` readiness | MEDIUM | LOW | **P2** |
| Example manifests in repo | MEDIUM | LOW | **P2** |
| Startup banner log | LOW | LOW | **P2** |
| Deterministic ordering | LOW | LOW | **P2** |
| Audit log lines on writes | MEDIUM | LOW | **P2** |
| SQLite/Postgres backend | LOW (v1) | HIGH | **P3** |
| Optimistic concurrency | LOW (v1) | MEDIUM | **P3** |
| Multi-tenancy | LOW (v1) | HIGH | **P3** |
| OAuth/OIDC | LOW (v1) | HIGH | **P3** |
| Rate limiting | LOW (v1) | LOW | **P3** |
| Metrics / OTel | LOW (v1) | MEDIUM | **P3** |

**Priority key:**
- **P1:** Must ship for v1 (Hackathon reference impl)
- **P2:** Should ship if time permits; all are single-day additions
- **P3:** Explicitly deferred to v2; documented in PROJECT.md as out-of-scope

---

## Competitor / Prior-Art Feature Analysis

| Feature | Android `PackageManager` | Cozy Stack intents | XDG Desktop Portal | **OpenBuro Server** |
|---------|--------------------------|--------------------|--------------------|---------------------|
| **Manifest format** | Binary AndroidManifest.xml (compiled) | JSON app manifest stored in CouchDB | `.desktop` ini files on disk | JSON POSTed to REST, stored in `registry.json` |
| **Registry location** | System-level service | CouchDB backing store | Filesystem + `mimeinfo.cache` | Single JSON file + in-memory index |
| **Schema validation** | `aapt` at build time | Stack validates on app install | `desktop-file-validate` at install time | Broker validates at upsert time (strict) |
| **Action vocabulary** | Open set (any string); conventional: `ACTION_PICK`, `ACTION_VIEW`, ... | Open set, conventional: `PICK`, `EDIT`, `CREATE`, `OPEN`, `SHARE` | N/A (portals are per-interface) | **Closed enum (`PICK`, `SAVE`)** in v1; explicitly constrained |
| **Data typing** | MIME + URI scheme + category | MIME OR Cozy doctype (e.g. `io.cozy.files`) | MIME only | **MIME only** — intentionally simpler than Cozy's doctype hybrid |
| **Wildcard MIME matching** | Yes, symmetric (`*/*`, `image/*`) | Yes, asymmetric in some versions — known bug source | Partial (hierarchy via shared-mime-info) | **Yes, symmetric + documented + tested** |
| **Resolver endpoint** | `queryIntentActivities()` IPC call | `POST /intents` | `CreateRequest()` on the portal interface | **`GET /api/v1/capabilities?action=&mimeType=`** — simpler because it's just a filter |
| **`availableApps` (non-installed that could handle)** | No | **Yes** (Cozy signals "you could install this app") | No | **No** — single-instance broker, installable-discovery is out of scope |
| **Change notifications** | `BroadcastReceiver` with `PACKAGE_ADDED` / `REMOVED` / `REPLACED` / `CHANGED` | No push; clients re-fetch | DBus `Response` signal on request handles (not registry events) | **Single `REGISTRY_UPDATED` event over WebSocket** — pushed, not polled |
| **Event granularity** | Per-package, per-event-type | N/A | N/A | **Full capability list per event** (coarser but simpler for clients) |
| **Subscribe/unsubscribe** | Implicit via BroadcastReceiver intent filters | N/A | Per-request | **None** — every connected client receives every event |
| **Auth model** | Signature-based permissions | Cozy OAuth, scoped to doctypes | DBus policy | **Basic Auth on writes only, reads public** |
| **Persistence** | System database | CouchDB | Filesystem | `registry.json` on disk |
| **Out-of-process reach** | IPC (Binder) | HTTP | DBus | **HTTP + WebSocket** (only universal-browser combination) |

### Key Takeaways from the Comparison

1. **Cozy Stack is the closest twin architecturally** — HTTP-backed, JSON manifests, MIME-based resolution. OpenBuro's job is to be "Cozy Stack intents, but with push notifications and less surface area."
2. **Push notifications are the genuine innovation.** Neither Android (broadcast intents are local-only), Cozy (polling), XDG Portal (per-request), nor openDesk ICS (no registry at all) offers push-based capability change notifications. This is OpenBuro's differentiator.
3. **Constraining the action enum to `PICK` + `SAVE`** is more restrictive than any of the prior art, which is the right v1 choice — it forces discipline and prevents "vocabulary drift" from killing interoperability. v2 can expand.
4. **Rejecting Cozy doctypes in favor of pure MIME** is a deliberate simplification. Cozy's `io.cozy.files` is valuable in a Cozy-native ecosystem but would force OpenBuro to define or adopt a doctype vocabulary — out of scope for a reference impl.
5. **Android's event taxonomy is richer but harder to consume correctly.** Single-event-with-full-state wins on "client correctness per line of client code."

---

## Validation Against Quality Gate

- ✅ **Categories are clear:** Table Stakes (20 features), Differentiators (11 features), Anti-Features (20 features) — each section has a clear mission.
- ✅ **Complexity estimates provided:** Every feature is tagged LOW / LOW-MEDIUM / MEDIUM / HIGH.
- ✅ **Dependencies identified:** Dependency graph section spells out the critical ordering (registry → persistence → hub → notifications; config → auth → writes; CORS must agree with origin check).
- ✅ **Reference-impl alignment:** Anti-features section explicitly rejects anything that would move this toward "production hardening" over "pattern clarity" (no ORM, no OIDC, no mTLS, no metrics stack, no rate limiting, no GraphQL, no gRPC).

---

## Confidence Notes

| Area | Confidence | Basis |
|------|------------|-------|
| Table stakes list | HIGH | Every item either directly mapped from PROJECT.md or obvious from the prior-art comparison |
| MIME matching semantics | HIGH | Verified symmetric wildcard behavior in Android docs and `developer.android.com`; consistent with Cozy and XDG conventions |
| Event model (single type, full payload) | HIGH | Deliberate architectural choice; reasoning documented; alternatives considered in Anti-Features |
| No subscribe/unsubscribe | HIGH | Scope-driven; see Anti-Features |
| Debounce event coalescing | MEDIUM | A judgement call; optional feature. Could legitimately ship without it in v1 |
| Anti-features list | HIGH | Every item is either an explicit PROJECT.md out-of-scope or a clear scope-creep risk for a reference impl |
| Prioritization | HIGH | Directly tied to PROJECT.md "Active" section + Out-of-Scope section |

---

## Sources

**Primary project inputs:**
- `.planning/PROJECT.md` — feature list (Active section), out-of-scope decisions, architectural constraints, target consumers
- `.planning/research/STACK.md` — dependency choices (`coder/websocket`, `rs/cors`, `log/slog`), hub pattern selection rationale
- `../open-buro-dossier-technique-file-picker.md` — prior-art comparison (Android, Cozy, XDG, openDesk, Google Picker), protocol decisions, glossary

**Prior-art authoritative references:**
- [Android Intents and Intent Filters](https://developer.android.com/guide/components/intents-filters) — MIME matching semantics, action/data model, wildcard rules (HIGH confidence)
- [Android `<data>` element](https://developer.android.com/guide/topics/manifest/data-element) — MIME subtype wildcard syntax (HIGH confidence)
- [Cozy Stack `/intents` documentation](https://docs.cozy.io/en/cozy-stack/intents/) — resolver, `availableApps` concept, service lifecycle (HIGH confidence)
- [Cozy Stack intents source](https://github.com/cozy/cozy-stack/blob/master/docs/intents.md) — manifest format, action vocabulary (HIGH confidence)
- [XDG Desktop Portal FileChooser](https://flatpak.github.io/xdg-desktop-portal/docs/doc-org.freedesktop.portal.FileChooser.html) — request/response pattern over DBus signals (HIGH confidence)
- [XDG Desktop Portal architecture](https://flatpak.github.io/xdg-desktop-portal/) — backend pattern, interface dispatch (HIGH confidence)
- [`freedesktop.org` shared-mime-info specification](https://specifications.freedesktop.org/shared-mime-info-spec/latest/) — MIME hierarchy conventions (HIGH confidence)

**WebSocket and notification patterns:**
- [WebSocket.org Notifications Guide](https://websocket.org/guides/use-cases/notifications/) — subscribe/unsubscribe vs broadcast tradeoffs (MEDIUM confidence)
- [Solid Notifications WebSocket Subscription](https://solid.github.io/notifications/websocket-subscription-2021) — change-notification protocol pattern (MEDIUM confidence)

**Schema and validation:**
- [Semantic Versioning 2.0.0](https://semver.org/) — manifest `version` field validation reference (HIGH confidence)

---

*Feature research for: OpenBuro Server — Go capability broker reference implementation*
*Researched: 2026-04-09*
