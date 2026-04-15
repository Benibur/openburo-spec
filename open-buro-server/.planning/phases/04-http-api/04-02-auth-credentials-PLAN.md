---
phase: 04-http-api
plan: 02
type: execute
wave: 2
depends_on: [04-01]
files_modified:
  - go.mod
  - go.sum
  - internal/httpapi/credentials.go
  - internal/httpapi/auth.go
  - internal/httpapi/server.go
  - internal/httpapi/credentials_test.go
  - internal/httpapi/auth_test.go
  - internal/httpapi/testdata/credentials-valid.yaml
  - internal/httpapi/testdata/credentials-low-cost.yaml
  - internal/httpapi/testdata/credentials-malformed.yaml
autonomous: true
requirements_addressed: [AUTH-01, AUTH-02, AUTH-03, AUTH-04, AUTH-05, TEST-06]
gap_closure: false
user_setup: []

must_haves:
  truths:
    - "LoadCredentials parses credentials.yaml, rejects missing files, rejects malformed YAML, and rejects any bcrypt hash with cost < 12"
    - "authBasic middleware protects write routes: POST and DELETE require valid Basic Auth, reads are public"
    - "Timing-safety: bcrypt.CompareHashAndPassword runs unconditionally (dummyHash fallback on unknown user), and the authorized decision uses subtle.ConstantTimeCompare on the (found, matches) byte tuple"
    - "On auth failure, the Warn log line contains path/method/remote ONLY — NEVER username, password, or Authorization header"
    - "Successful auth stashes the username in request context under an unexported ctxKeyUser sentinel type"
  artifacts:
    - path: "internal/httpapi/credentials.go"
      provides: "Credentials type (replaces Plan 04-01 stub), LoadCredentials, Lookup"
      contains: "func LoadCredentials"
    - path: "internal/httpapi/auth.go"
      provides: "authBasic middleware, dummyHash package init, ctxKeyUser sentinel, usernameFromContext helper"
      contains: "func (s *Server) authBasic"
    - path: "internal/httpapi/testdata/credentials-valid.yaml"
      provides: "Real cost-12 bcrypt hash fixture for admin/testpass"
    - path: "internal/httpapi/testdata/credentials-low-cost.yaml"
      provides: "Cost-10 bcrypt hash fixture that MUST be rejected by LoadCredentials"
  key_links:
    - from: "internal/httpapi/auth.go (authBasic)"
      to: "internal/httpapi/credentials.go (Credentials.Lookup)"
      via: "authBasic calls creds.Lookup(username); on !found, substitutes dummyHash"
      pattern: "creds.Lookup.*dummyHash"
    - from: "internal/httpapi/auth.go (authBasic decision)"
      to: "crypto/subtle.ConstantTimeCompare"
      via: "tuple byte{found, matches} compared against byte{1, 1}"
      pattern: "subtle\\.ConstantTimeCompare"
---

<objective>
Implement timing-safe HTTP Basic Auth. LoadCredentials reads credentials.yaml with a cost-≥12 bcrypt gate, the authBasic middleware enforces PITFALLS #8 (unconditional bcrypt + subtle.ConstantTimeCompare tuple), and a dedicated PII guard test proves no credential material ever reaches any log line.

Purpose: AUTH-01..05 and TEST-06 land here. This is the #1 most scrutinized correctness property in Phase 4 — reviewers look first at the timing-safety pattern and second at the PII guard.

Output: `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run 'TestLoadCredentials|TestAuth_'` all passing, with a real bcrypt cost-12 fixture on disk and the credentials stub from Plan 04-01 replaced by the real type.
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
@.planning/research/PITFALLS.md
@internal/httpapi/server.go
@internal/httpapi/errors.go
@internal/httpapi/middleware.go

<interfaces>
<!-- Plan 04-01 shipped this stub that this plan replaces: -->
```go
// FROM PLAN 04-01 server.go — REPLACE with real type in credentials.go
type Credentials struct {
    users map[string][]byte
}
```

<!-- This plan ships: -->
```go
// credentials.go
type Credentials struct {
    users map[string][]byte
}
func LoadCredentials(path string) (Credentials, error)
func (c Credentials) Lookup(username string) (hash []byte, ok bool)

// auth.go
var dummyHash []byte  // precomputed at init, cost 12

type ctxKey int
const ctxKeyUser ctxKey = iota

func (s *Server) authBasic(next http.Handler) http.Handler
func usernameFromContext(ctx context.Context) (string, bool)
```

Plan 04-03 will call `s.authBasic(...)` in `registerRoutes` to wrap the POST/DELETE handlers.
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 0: Add bcrypt dependency + generate testdata fixtures</name>
  <files>
    go.mod,
    go.sum,
    internal/httpapi/testdata/credentials-valid.yaml,
    internal/httpapi/testdata/credentials-low-cost.yaml,
    internal/httpapi/testdata/credentials-malformed.yaml
  </files>
  <read_first>
    .planning/phases/04-http-api/04-CONTEXT.md,
    .planning/phases/04-http-api/04-RESEARCH.md
  </read_first>
  <action>
    Install the bcrypt package:

    ```bash
    ~/sdk/go1.26.2/bin/go get golang.org/x/crypto/bcrypt
    ~/sdk/go1.26.2/bin/go mod tidy
    ```

    `golang.org/x/crypto` flips from indirect to direct. This is load-bearing because `go mod tidy` will remove it if no production file imports it yet — create a tiny `internal/httpapi/testdata_gen_test.go` anchor file (kept for this task only; deleted at the end of Task 2 when auth.go imports bcrypt directly):

    Actually simpler: run `~/sdk/go1.26.2/bin/go get` only AFTER Task 1 has the test file that imports bcrypt. So do dependency add here, THEN write the fixture files, THEN let Task 1's RED test file be the first importer.

    Alternative: generate fixtures via a standalone one-shot `go run` script in the shell (no anchor file). Do this:

    ```bash
    # Generate cost-12 hash for password "testpass"
    HASH12=$(~/sdk/go1.26.2/bin/go run -mod=mod - <<'EOF'
    package main
    import (
        "fmt"
        "golang.org/x/crypto/bcrypt"
    )
    func main() {
        h, err := bcrypt.GenerateFromPassword([]byte("testpass"), 12)
        if err != nil { panic(err) }
        fmt.Print(string(h))
    }
    EOF
    )

    # Generate cost-10 hash (below the 12-minimum)
    HASH10=$(~/sdk/go1.26.2/bin/go run -mod=mod - <<'EOF'
    package main
    import (
        "fmt"
        "golang.org/x/crypto/bcrypt"
    )
    func main() {
        h, err := bcrypt.GenerateFromPassword([]byte("testpass"), 10)
        if err != nil { panic(err) }
        fmt.Print(string(h))
    }
    EOF
    )
    ```

    Write `internal/httpapi/testdata/credentials-valid.yaml`:

    ```yaml
    users:
      admin: "${HASH12}"
    ```

    (Replace `${HASH12}` with the actual hash string captured from the command above — e.g. `"$2a$12$abcdefghijklmnopqrstuv.wxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123"`. Quote the value so YAML treats it as a plain string — the `$` characters otherwise cause no harm but the quotes are safer.)

    Write `internal/httpapi/testdata/credentials-low-cost.yaml`:

    ```yaml
    users:
      admin: "${HASH10}"
    ```

    Write `internal/httpapi/testdata/credentials-malformed.yaml`:

    ```yaml
    users:
      admin: "not a bcrypt hash"
      : "missing key"
    tabs	and	broken
    ```

    (The broken indentation with raw tabs and the empty-key line force `yaml.Unmarshal` to return a parse error.)

    Run `~/sdk/go1.26.2/bin/go mod tidy`. At this point `golang.org/x/crypto` is NOT yet in go.mod as direct because nothing imports it — that's fine. Task 1's RED test file will import it and Task 2 will make it stick.

    Verify the generated hashes by decoding them:

    ```bash
    ~/sdk/go1.26.2/bin/go run -mod=mod - <<'EOF'
    package main
    import (
        "fmt"
        "os"
        "golang.org/x/crypto/bcrypt"
    )
    func main() {
        h, _ := os.ReadFile("internal/httpapi/testdata/credentials-valid.yaml")
        // extract hash string manually
        fmt.Println(string(h))
        fmt.Println(bcrypt.Cost([]byte(/* the hash */)))
    }
    EOF
    ```

    (Or simpler — the Task 1 test `TestLoadCredentials_Valid` will verify the cost at test time. No need to double-check here.)

    Commit: `chore(04-02): add bcrypt + generate credential fixtures`
  </action>
  <verify>
    <automated>cd /home/ben/Dev-local/openburo-spec/open-buro-server &amp;&amp; test -f internal/httpapi/testdata/credentials-valid.yaml &amp;&amp; test -f internal/httpapi/testdata/credentials-low-cost.yaml &amp;&amp; test -f internal/httpapi/testdata/credentials-malformed.yaml &amp;&amp; grep -q '\$2a\$12\$' internal/httpapi/testdata/credentials-valid.yaml &amp;&amp; grep -q '\$2a\$10\$' internal/httpapi/testdata/credentials-low-cost.yaml</automated>
  </verify>
  <acceptance_criteria>
    - Three fixture files exist under `internal/httpapi/testdata/`
    - `credentials-valid.yaml` contains a `$2a$12$` hash prefix: `grep -c '\$2a\$12\$' internal/httpapi/testdata/credentials-valid.yaml → 1`
    - `credentials-low-cost.yaml` contains a `$2a$10$` hash prefix: `grep -c '\$2a\$10\$' internal/httpapi/testdata/credentials-low-cost.yaml → 1`
    - `credentials-malformed.yaml` exists and contains broken YAML indentation (manually verified — Task 1 will prove it by failing to parse)
    - `go.mod` and `go.sum` are clean after `~/sdk/go1.26.2/bin/go mod tidy`: `~/sdk/go1.26.2/bin/go mod tidy && git diff --exit-code go.mod go.sum || true` (tidy is idempotent)
  </acceptance_criteria>
  <done>Three bcrypt-fixture YAML files exist on disk with verified cost 12 / cost 10 / malformed content; dependency graph has bcrypt available even if not yet directly imported by production code.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 1: RED — write failing tests for LoadCredentials + authBasic timing-safety + PII guard</name>
  <files>
    internal/httpapi/credentials_test.go (create),
    internal/httpapi/auth_test.go (create)
  </files>
  <read_first>
    .planning/phases/04-http-api/04-CONTEXT.md,
    .planning/phases/04-http-api/04-RESEARCH.md,
    .planning/phases/04-http-api/04-VALIDATION.md,
    .planning/research/PITFALLS.md,
    internal/httpapi/server_test.go,
    internal/httpapi/server.go,
    internal/httpapi/errors.go,
    internal/httpapi/testdata/credentials-valid.yaml,
    internal/httpapi/testdata/credentials-low-cost.yaml,
    internal/httpapi/testdata/credentials-malformed.yaml
  </read_first>
  <behavior>
    - TestLoadCredentials_Valid: LoadCredentials("testdata/credentials-valid.yaml") returns a Credentials with exactly 1 user; creds.Lookup("admin") returns (hash, true); bcrypt.Cost(hash) == 12
    - TestLoadCredentials_Missing: LoadCredentials("testdata/does-not-exist.yaml") returns error whose message contains "credentials" (no panic, clean error)
    - TestLoadCredentials_Malformed: LoadCredentials("testdata/credentials-malformed.yaml") returns an error
    - TestLoadCredentials_LowCost: LoadCredentials("testdata/credentials-low-cost.yaml") returns error whose message contains "cost" AND "12" AND "admin"
    - TestAuth_TimingSafe: use two sub-benchmarks OR a simple wall-clock assertion — run the authBasic middleware 100x with (a) wrong-username, (b) wrong-password for a known-good user; both paths must call bcrypt.CompareHashAndPassword (observed by the test: both should take measurable time, both should be within an order of magnitude of each other). Implementation: wrap the authBasic check against a Credentials with 1 real user, hit with (nonexistent-user, any-password) 10x, measure total elapsed; hit with (real-user, wrong-password) 10x, measure total elapsed; assert both durations are > 50ms (proves bcrypt ran both times) and the ratio max/min < 3 (proves no early-exit shortcut).
    - TestAuth_NoCredentialsInLogs: build a Server with a `&syncBuffer{}`-backed slog.TextHandler, make one FAILED request (wrong password: Basic admin:WRONGPASSWORD) and one SUCCESSFUL request (admin:testpass) through a test handler wrapped by authBasic. After both, assert the captured buffer does NOT contain any of: the decoded password `WRONGPASSWORD`, `testpass`, the literal Authorization header value `Basic YWRtaW46V1JPTkdQQVNTV09SRA==`, the username `admin` (as in-body text — the log's `user` field is added by plan 04-03's audit log, NOT this middleware). **Grep assertions:** `require.NotContains(buf.String(), "WRONGPASSWORD")`, `require.NotContains(buf.String(), "testpass")`, `require.NotContains(buf.String(), "YWRtaW4=")` (base64 of "admin"), `require.NotContains(buf.String(), "Basic YWRt")`.
    - TestAuth_EmptyHeader: request with NO Authorization header → 401 response, WWW-Authenticate header set, next handler NOT invoked (assert via a sentinel counter)
    - TestAuth_WrongPassword: request with Basic admin:wrong → 401, next handler not invoked
    - TestAuth_UnknownUser: request with Basic alice:anything → 401, next handler not invoked
    - TestAuth_Success: request with Basic admin:testpass (real hash from fixture) → next handler invoked; the handler extracts the username from ctx via usernameFromContext(ctx) and asserts it == "admin"
    - TestAuth_DummyHashBcryptRuns: mock-free — use a Credentials{} empty table, hit authBasic with Basic bob:whatever; assert response is 401 AND wall-clock elapsed > 50ms (bcrypt cost 12 takes ~100ms on modern hardware; 50ms is a generous floor that still proves bcrypt ran)
  </behavior>
  <action>
    Create `internal/httpapi/credentials_test.go`:

    ```go
    package httpapi

    import (
        "testing"

        "github.com/stretchr/testify/require"
        "golang.org/x/crypto/bcrypt"
    )

    func TestLoadCredentials_Valid(t *testing.T) {
        creds, err := LoadCredentials("testdata/credentials-valid.yaml")
        require.NoError(t, err)
        hash, ok := creds.Lookup("admin")
        require.True(t, ok)
        cost, err := bcrypt.Cost(hash)
        require.NoError(t, err)
        require.Equal(t, 12, cost)
    }

    func TestLoadCredentials_Missing(t *testing.T) {
        _, err := LoadCredentials("testdata/does-not-exist.yaml")
        require.Error(t, err)
        require.Contains(t, err.Error(), "credentials")
    }

    func TestLoadCredentials_Malformed(t *testing.T) {
        _, err := LoadCredentials("testdata/credentials-malformed.yaml")
        require.Error(t, err)
    }

    func TestLoadCredentials_LowCost(t *testing.T) {
        _, err := LoadCredentials("testdata/credentials-low-cost.yaml")
        require.Error(t, err)
        require.Contains(t, err.Error(), "cost")
        require.Contains(t, err.Error(), "12")
        require.Contains(t, err.Error(), "admin")
    }
    ```

    Create `internal/httpapi/auth_test.go`:

    ```go
    package httpapi

    import (
        "log/slog"
        "net/http"
        "net/http/httptest"
        "testing"
        "time"

        "github.com/stretchr/testify/require"
    )

    // helper: build a Server wired to the valid fixture
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
            "unknown-user path too fast (%v) — bcrypt likely did NOT run, timing-safety violated", unknownElapsed)
        require.Greater(t, wrongElapsed, 50*time.Millisecond,
            "wrong-password path too fast (%v) — bcrypt did NOT run", wrongElapsed)

        // Ratio between the two paths must be modest. A ratio > 3 suggests
        // one path is short-circuiting.
        ratio := float64(unknownElapsed) / float64(wrongElapsed)
        if ratio < 1 {
            ratio = 1 / ratio
        }
        require.Less(t, ratio, 3.0,
            "timing ratio unknown/wrong = %v — one path is taking an order of magnitude longer than the other, indicating a short-circuit", ratio)
    }

    // TestAuth_DummyHashBcryptRuns additionally proves that even with an
    // entirely empty Credentials table (no users registered at all), the
    // authBasic middleware still runs bcrypt (against dummyHash), so an
    // attacker cannot probe "is there any user here?" via wall-clock time.
    func TestAuth_DummyHashBcryptRuns(t *testing.T) {
        srv := newTestServerWithLogger(t, slog.New(slog.NewTextHandler(&syncBuffer{}, nil)))
        srv.creds = Credentials{} // empty — zero users
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
            "empty-creds path too fast (%v) — dummyHash bcrypt did NOT run", elapsed)
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
        // PII assertions — these MUST all pass or AUTH-05 is violated.
        require.NotContains(t, out, "WRONGPASSWORD", "log contains plaintext password")
        require.NotContains(t, out, "testpass", "log contains plaintext valid password")
        require.NotContains(t, out, "Basic YWRtaW4", "log contains base64 Basic header")
        require.NotContains(t, out, "YWRtaW46", "log contains base64 username prefix")
        require.NotContains(t, out, "Authorization", "log leaks header name (sign of header dump)")
        // The username `admin` by itself is not forbidden here — the audit log
        // in plan 04-03 is ALLOWED to emit user=admin. This middleware just
        // must not leak the credential MATERIAL (password, header, base64).
    }
    ```

    Run the test file — MUST fail to compile because credentials.go and auth.go do not exist yet. Credentials, LoadCredentials, Lookup, authBasic, usernameFromContext are all undeclared. This is the RED state.

    Commit: `test(04-02): add failing tests for LoadCredentials + timing-safe auth + PII guard`
  </action>
  <verify>
    <automated>cd /home/ben/Dev-local/openburo-spec/open-buro-server &amp;&amp; ~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -timeout 30s 2>&amp;1 | head -60 ; echo "EXPECT: undefined symbols for LoadCredentials, authBasic, usernameFromContext"</automated>
  </verify>
  <acceptance_criteria>
    - Files exist: `test -f internal/httpapi/credentials_test.go && test -f internal/httpapi/auth_test.go`
    - Timing-safe test exists: `grep -c "func TestAuth_TimingSafe" internal/httpapi/auth_test.go → 1`
    - Dummy-hash bcrypt-runs test exists: `grep -c "func TestAuth_DummyHashBcryptRuns" internal/httpapi/auth_test.go → 1`
    - PII test exists and checks ≥5 forbidden substrings: `grep -c "require.NotContains" internal/httpapi/auth_test.go → ≥5`
    - PII test explicitly forbids "WRONGPASSWORD" and "testpass": `grep -c 'WRONGPASSWORD' internal/httpapi/auth_test.go → ≥2` AND `grep -c '"testpass"' internal/httpapi/auth_test.go → ≥2`
    - LoadCredentials_LowCost test asserts "cost" AND "12" AND "admin" in error: `grep -A5 "TestLoadCredentials_LowCost" internal/httpapi/credentials_test.go | grep -c 'Contains' → ≥3`
    - TestAuth_TimingSafe asserts `require.Greater(t, unknownElapsed, 50*time.Millisecond`: `grep -c "Greater.*50.*Millisecond" internal/httpapi/auth_test.go → ≥1`
    - RED state verified: `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -timeout 30s 2>&1 | grep -cE 'undefined.*(LoadCredentials|authBasic|usernameFromContext)' → ≥1`
    - gofmt clean on new files: `~/sdk/go1.26.2/bin/gofmt -l internal/httpapi/credentials_test.go internal/httpapi/auth_test.go` empty
  </acceptance_criteria>
  <done>RED committed: credentials_test.go and auth_test.go on disk with the full test matrix (valid/missing/malformed/low-cost for credentials; empty-header/wrong-password/unknown-user/success/timing-safe/dummy-hash/PII-guard for auth). Production code referenced by the tests does not exist yet, so compilation fails.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: GREEN — implement credentials.go + auth.go to make Task 1 tests pass</name>
  <files>
    internal/httpapi/credentials.go (create),
    internal/httpapi/auth.go (create),
    internal/httpapi/server.go (modify — remove Credentials stub, keep Credentials declared in credentials.go)
  </files>
  <read_first>
    .planning/phases/04-http-api/04-CONTEXT.md,
    .planning/phases/04-http-api/04-RESEARCH.md,
    .planning/research/PITFALLS.md,
    internal/httpapi/server.go,
    internal/httpapi/credentials_test.go,
    internal/httpapi/auth_test.go
  </read_first>
  <action>
    Create `internal/httpapi/credentials.go`:

    ```go
    package httpapi

    import (
        "fmt"
        "os"

        "go.yaml.in/yaml/v3"
        "golang.org/x/crypto/bcrypt"
    )

    // Credentials is the parsed bcrypt-hash table loaded from credentials.yaml.
    // Values are bcrypt hashes (cost >= 12 enforced at load time per AUTH-01).
    // The zero value is an empty table — no users, all write requests return
    // 401 (but authBasic still runs bcrypt against dummyHash to preserve
    // timing-safety per AUTH-04).
    type Credentials struct {
        users map[string][]byte
    }

    // credentialsFile is the on-disk YAML shape.
    type credentialsFile struct {
        Users map[string]string `yaml:"users"`
    }

    // LoadCredentials reads credentials.yaml and returns a Credentials table.
    // Returns an error if:
    //   - the file is missing (operator explicitly configured credentials_file
    //     in config.yaml — a missing file signals a misconfig, NOT an empty
    //     registry; fail fast so the operator sees the problem at startup)
    //   - the YAML is malformed
    //   - any bcrypt hash has cost strictly less than 12 (AUTH-01)
    func LoadCredentials(path string) (Credentials, error) {
        data, err := os.ReadFile(path)
        if err != nil {
            return Credentials{}, fmt.Errorf("credentials: read %q: %w", path, err)
        }
        var raw credentialsFile
        if err := yaml.Unmarshal(data, &raw); err != nil {
            return Credentials{}, fmt.Errorf("credentials: parse %q: %w", path, err)
        }
        users := make(map[string][]byte, len(raw.Users))
        for username, hashStr := range raw.Users {
            if username == "" {
                return Credentials{}, fmt.Errorf("credentials: %q: empty username", path)
            }
            hash := []byte(hashStr)
            cost, err := bcrypt.Cost(hash)
            if err != nil {
                return Credentials{}, fmt.Errorf("credentials: user %q: invalid bcrypt hash: %w", username, err)
            }
            if cost < 12 {
                return Credentials{}, fmt.Errorf("credentials: user %q: bcrypt cost %d is below minimum 12", username, cost)
            }
            users[username] = hash
        }
        return Credentials{users: users}, nil
    }

    // Lookup returns the bcrypt hash for a username. The second return is
    // false if the user does not exist. Callers in authBasic MUST still run
    // bcrypt.CompareHashAndPassword on a dummyHash fallback to preserve
    // timing-safety (AUTH-04). This method is ONLY the lookup — it does NOT
    // early-return, does NOT short-circuit, does NOT hash.
    func (c Credentials) Lookup(username string) ([]byte, bool) {
        h, ok := c.users[username]
        return h, ok
    }
    ```

    Create `internal/httpapi/auth.go`:

    ```go
    package httpapi

    import (
        "context"
        "crypto/subtle"
        "fmt"
        "log/slog"
        "net/http"

        "golang.org/x/crypto/bcrypt"
    )

    // ctxKey is the unexported context-key type used to stash the
    // authenticated username on the request context. Using an unexported
    // type prevents collisions with context keys from other packages
    // (Go convention).
    type ctxKey int

    const (
        ctxKeyUser ctxKey = iota
    )

    // usernameFromContext extracts the authenticated username previously
    // stashed by authBasic. Returns ("", false) if the context has no
    // user (i.e. the request was on a public route or auth was not run).
    func usernameFromContext(ctx context.Context) (string, bool) {
        u, ok := ctx.Value(ctxKeyUser).(string)
        return u, ok
    }

    // dummyHash is the precomputed bcrypt hash of a known-nonsense value.
    // Used in authBasic's "user not found" path so
    // bcrypt.CompareHashAndPassword always runs, making the unauthenticated
    // path and the wrong-password path indistinguishable by wall-clock time.
    //
    // Cost 12 matches the minimum enforced for real credentials (AUTH-01).
    // The password string ("openburo:dummy:do-not-match") is 27 bytes,
    // safely under bcrypt's 72-byte limit.
    var dummyHash []byte

    func init() {
        h, err := bcrypt.GenerateFromPassword([]byte("openburo:dummy:do-not-match"), 12)
        if err != nil {
            panic(fmt.Sprintf("httpapi: failed to generate dummy hash: %v", err))
        }
        dummyHash = h
    }

    // authBasic returns a middleware that enforces HTTP Basic Auth using the
    // Server's credential table. On failure, writes 401 + WWW-Authenticate
    // and returns without invoking next.
    //
    // TIMING-SAFETY CONTRACT (AUTH-04, PITFALLS #8):
    //   - bcrypt.CompareHashAndPassword runs UNCONDITIONALLY for every request
    //   - Unknown users use dummyHash so the bcrypt CPU cost is identical to
    //     the wrong-password path
    //   - The final authorized decision uses subtle.ConstantTimeCompare on
    //     the byte tuple {found, bcryptMatches} — NOT a short-circuit
    //     `if found && matches`
    //
    // PII-SAFETY CONTRACT (AUTH-05, TEST-06):
    //   - The Warn log line on failure logs ONLY path, method, remote
    //   - NEVER logs username, password, Authorization header, or bcrypt hash
    //   - The audit log (plan 04-03) runs AFTER this middleware on success
    //     and emits user=<username>, action=<upsert|delete>, appId=<id>
    //     with no password material
    func (s *Server) authBasic(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            username, password, ok := r.BasicAuth()
            if !ok {
                writeUnauthorized(w)
                return
            }

            // Look up the user; if not found, substitute dummyHash so bcrypt
            // ALWAYS runs. Do NOT early-return here.
            storedHash, found := s.creds.Lookup(username)
            if !found {
                storedHash = dummyHash
            }

            // bcrypt runs unconditionally. On the unknown-user path,
            // storedHash == dummyHash, so the CPU cost is identical to
            // the wrong-password path.
            bcryptErr := bcrypt.CompareHashAndPassword(storedHash, []byte(password))
            bcryptMatches := bcryptErr == nil

            // Constant-time combination of (found, matches). A short-circuit
            // `if found && bcryptMatches { ... } else { 401 }` would be
            // timing-equivalent here because bcrypt already ran, but using
            // subtle.ConstantTimeCompare makes the safety property explicit
            // to reviewers and future maintainers.
            var foundByte, matchByte byte
            if found {
                foundByte = 1
            }
            if bcryptMatches {
                matchByte = 1
            }
            if subtle.ConstantTimeCompare([]byte{foundByte, matchByte}, []byte{1, 1}) != 1 {
                // AUTH-05: log ONLY path/method/remote. Never username,
                // never password, never Authorization header.
                s.logger.Warn("httpapi: basic auth failed",
                    "path", r.URL.Path,
                    "method", r.Method,
                    "remote", clientIP(r))
                writeUnauthorized(w)
                return
            }

            // Success — stash the authenticated username in ctx so downstream
            // audit logging (plan 04-03) can emit the `user` field.
            ctx := context.WithValue(r.Context(), ctxKeyUser, username)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }

    // Compile-time check that slog is imported (silences unused import if
    // the middleware is ever edited to drop the Warn log line — keeps a
    // reviewer's attention on the fact that this file SHOULD log on failure).
    var _ = slog.LevelWarn
    ```

    Remove the `Credentials` stub from `internal/httpapi/server.go` (it was declared in Plan 04-01 as a placeholder; now the real type lives in credentials.go). Edit `server.go`:
    - Delete the `type Credentials struct { users map[string][]byte }` block
    - Keep everything else unchanged

    Run `~/sdk/go1.26.2/bin/go mod tidy` to flip `golang.org/x/crypto` from indirect to direct (auth.go and credentials.go both import `golang.org/x/crypto/bcrypt`).

    Run the full httpapi suite — all Task 1 tests plus all previous Plan 04-01 tests must pass under `-race`. Specifically target the new tests first:

    ```bash
    ~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run 'TestLoadCredentials|TestAuth_' -timeout 30s -v
    ```

    Then the whole suite:

    ```bash
    ~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -count=1 -timeout 60s
    ```

    Gate checks:
    - `! grep -rE 'slog\.Default' internal/httpapi/*.go | grep -v _test.go` — empty
    - `grep -c "subtle.ConstantTimeCompare" internal/httpapi/auth.go` → 1
    - `grep -c "bcrypt.CompareHashAndPassword" internal/httpapi/auth.go` → 1
    - `grep -c "dummyHash" internal/httpapi/auth.go` → ≥3 (var declaration, init assignment, usage in authBasic)

    Commit: `feat(04-02): implement timing-safe Basic Auth + bcrypt credential loading + PII guard`
  </action>
  <verify>
    <automated>cd /home/ben/Dev-local/openburo-spec/open-buro-server &amp;&amp; ~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -count=1 -timeout 60s &amp;&amp; ~/sdk/go1.26.2/bin/go vet ./internal/httpapi/... &amp;&amp; test -z "$(~/sdk/go1.26.2/bin/gofmt -l internal/httpapi/)"</automated>
  </verify>
  <acceptance_criteria>
    - Files exist: `test -f internal/httpapi/credentials.go && test -f internal/httpapi/auth.go`
    - Credentials stub removed from server.go: `grep -c "type Credentials struct" internal/httpapi/server.go → 0` AND `grep -c "type Credentials struct" internal/httpapi/credentials.go → 1`
    - LoadCredentials + Lookup exist: `grep -c "^func LoadCredentials" internal/httpapi/credentials.go → 1` AND `grep -c "^func (c Credentials) Lookup" internal/httpapi/credentials.go → 1`
    - Cost-12 check present: `grep -c "cost < 12" internal/httpapi/credentials.go → 1` AND `grep -c "minimum 12" internal/httpapi/credentials.go → 1`
    - authBasic function exists: `grep -c "^func (s \*Server) authBasic" internal/httpapi/auth.go → 1`
    - PITFALLS #8 critical anchors all present:
      - `grep -c "subtle.ConstantTimeCompare" internal/httpapi/auth.go → 1`
      - `grep -c "bcrypt.CompareHashAndPassword" internal/httpapi/auth.go → 1`
      - `grep -c "dummyHash" internal/httpapi/auth.go → 3` (var, init assign, middleware use; may be higher if comments mention it — ≥3 is the floor)
    - dummyHash initialized in init() via GenerateFromPassword: `grep -c "bcrypt.GenerateFromPassword" internal/httpapi/auth.go → 1`
    - usernameFromContext helper exists: `grep -c "^func usernameFromContext" internal/httpapi/auth.go → 1`
    - ctxKeyUser is an unexported sentinel type (not string): `grep -c "type ctxKey int" internal/httpapi/auth.go → 1` AND `grep -c "ctxKeyUser ctxKey" internal/httpapi/auth.go → 1`
    - Warn log line contains ONLY path/method/remote — no username/password/Authorization: `grep -A4 "httpapi: basic auth failed" internal/httpapi/auth.go | grep -cE "username|password|Authorization" → 0`
    - Named tests pass: `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestLoadCredentials' -timeout 10s` exits 0
    - Named tests pass: `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestAuth_TimingSafe$' -timeout 10s` exits 0
    - Named tests pass: `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestAuth_NoCredentialsInLogs$' -timeout 10s` exits 0
    - Named tests pass: `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestAuth_DummyHashBcryptRuns$' -timeout 10s` exits 0
    - Full suite green: `~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -count=1 -timeout 60s` exits 0
    - Architectural gate (registry isolation): `! ~/sdk/go1.26.2/bin/go list -deps ./internal/registry | grep -E 'wshub|httpapi'` exits 0
    - No slog.Default: `! grep -rE 'slog\.Default' internal/httpapi/*.go | grep -v _test.go` exits 0
    - go vet clean, gofmt clean
    - `golang.org/x/crypto` direct: `grep -c 'golang.org/x/crypto' go.mod → ≥1`
  </acceptance_criteria>
  <done>GREEN: LoadCredentials enforces cost ≥ 12, authBasic runs bcrypt unconditionally against dummyHash on unknown users, subtle.ConstantTimeCompare gates the final decision, the Warn log line is PII-free (no username/password/Authorization), and all Task 1 tests pass under -race including TestAuth_TimingSafe (proves both paths > 50ms and ratio < 3) and TestAuth_NoCredentialsInLogs (proves no credential material in the log buffer).</done>
</task>

</tasks>

<verification>
```bash
# 1. Full httpapi suite race-clean
~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -count=1 -timeout 90s

# 2. Plan 04-02 specific tests
~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestLoadCredentials' -timeout 10s
~/sdk/go1.26.2/bin/go test ./internal/httpapi -race -run '^TestAuth_' -timeout 30s

# 3. Architectural gates
! ~/sdk/go1.26.2/bin/go list -deps ./internal/registry | grep -E 'wshub|httpapi'
! grep -rE 'slog\.Default' internal/httpapi/*.go | grep -v _test.go
! grep -rn 'InsecureSkipVerify' internal/httpapi/*.go

# 4. PITFALLS #8 anchors
grep -c "subtle.ConstantTimeCompare" internal/httpapi/auth.go   # == 1
grep -c "bcrypt.CompareHashAndPassword" internal/httpapi/auth.go # == 1
grep -c "dummyHash" internal/httpapi/auth.go                     # >= 3

# 5. Format + vet
~/sdk/go1.26.2/bin/go vet ./internal/httpapi/...
test -z "$(~/sdk/go1.26.2/bin/gofmt -l internal/httpapi/)"
```
</verification>

<success_criteria>
- LoadCredentials validates the bcrypt cost-≥12 gate and rejects malformed/missing/low-cost fixtures
- authBasic runs bcrypt.CompareHashAndPassword unconditionally (proven by TestAuth_DummyHashBcryptRuns > 30ms on empty creds)
- The final authorization gate is `subtle.ConstantTimeCompare([]byte{foundByte, matchByte}, []byte{1, 1})` — not a short-circuit
- TestAuth_TimingSafe proves both the unknown-user and wrong-password paths take >50ms and have a ratio < 3
- TestAuth_NoCredentialsInLogs proves no credential material (password, base64 header, "Authorization") appears in captured slog output
- The authenticated username is stashed under an unexported `ctxKeyUser ctxKey` sentinel, not a string key
- Full httpapi suite green under -race; go vet and gofmt clean; architectural gates (registry isolation, no slog.Default) still pass
</success_criteria>

<output>
After completion, create `.planning/phases/04-http-api/04-02-SUMMARY.md` following Phase 3's SUMMARY conventions. Note: plan 04-03 will add the actual `s.authBasic(...)` wrap calls inside `registerRoutes()` around the POST and DELETE handlers.
</output>
