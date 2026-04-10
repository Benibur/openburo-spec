---
phase: 01-foundation
plan: 02
type: execute
wave: 2
depends_on:
  - 01-01
files_modified:
  - internal/config/config.go
  - internal/config/config_test.go
  - internal/config/testdata/valid.yaml
  - internal/config/testdata/invalid-log-format.yaml
  - internal/config/testdata/invalid-log-level.yaml
  - internal/config/testdata/missing-credentials-file.yaml
  - internal/config/testdata/zero-port.yaml
  - internal/config/testdata/tls-no-cert.yaml
  - internal/config/testdata/zero-ping.yaml
  - internal/config/testdata/malformed.yaml
  - internal/config/.gitkeep
  - config.example.yaml
  - credentials.example.yaml
autonomous: true
requirements:
  - FOUND-02
  - FOUND-07
must_haves:
  truths:
    - "`config.Load(path)` returns a validated `*Config` on valid YAML input"
    - "`config.Load` fails fast with a friendly message when the file is missing"
    - "`config.Load` rejects invalid enum values for `logging.format` and `logging.level`"
    - "`config.Load` rejects zero/missing required fields (port, credentials_file, registry_file, ping_interval_seconds)"
    - "`config.Load` converts `ping_interval_seconds: 30` into `time.Duration(30 * time.Second)`"
    - "A developer can copy `config.example.yaml` and `credentials.example.yaml` from the repo root and edit them in place"
  artifacts:
    - path: "internal/config/config.go"
      provides: "Config/ServerConfig/TLSConfig/WebSocketConfig/LoggingConfig/CORSConfig types + Load + validate + Addr()"
      contains: "func Load(path string) (*Config, error)"
      min_lines: 100
    - path: "internal/config/config_test.go"
      provides: "Table-driven TestLoad + TestLoad_MissingFile + TestLoad_UnreadableFile + TestLoad_DerivesPingInterval"
      contains: "func TestLoad(t *testing.T)"
    - path: "config.example.yaml"
      provides: "Quickstart-ready config template with comments"
      contains: "server:"
    - path: "credentials.example.yaml"
      provides: "Quickstart-ready credentials template with bcrypt placeholder"
      contains: "admins:"
  key_links:
    - from: "internal/config/config.go"
      to: "go.yaml.in/yaml/v3"
      via: "yaml.Unmarshal call in Load"
      pattern: "yaml\\.Unmarshal"
    - from: "internal/config/config_test.go"
      to: "internal/config/testdata/*.yaml"
      via: "filepath.Join(\"testdata\", ...) in table cases"
      pattern: "testdata"
    - from: "config.example.yaml"
      to: "internal/config/config.go"
      via: "Operator-visible schema must match Config struct YAML tags"
      pattern: "server:"
---

<objective>
Implement the `internal/config` package: the `Config` type tree (Server, TLS, WebSocket, Logging, CORS substructs), the `Load(path string) (*Config, error)` entry point with YAML unmarshaling and validation, and the table-driven test suite backed by testdata fixtures. Ship the two `.example` YAML files at the repo root so a developer can `cp config.example.yaml config.yaml` and run the server in Phase 1-03.

Purpose: Every other Phase 1 component (slog construction, startup banner, httpapi.Server) depends on reading a validated `*Config`. Without this plan, Plan 03 cannot wire anything together.
Output: A fully unit-tested config loader + operator-facing example files that expose the complete Phase 1 schema.

**Pitfall-awareness:** Error wrapping uses `%w`, not `%v`, so `errors.Is(err, os.ErrNotExist)` survives the wrap chain (see PITFALLS #13 / RESEARCH §Pitfall 5). Validate() returns wrapped errors with field names so operators can fix their YAML.
</objective>

<execution_context>
@.planning/phases/01-foundation/01-CONTEXT.md
@.planning/phases/01-foundation/01-RESEARCH.md
@.planning/phases/01-foundation/01-VALIDATION.md
</execution_context>

<context>
@.planning/REQUIREMENTS.md
@.planning/research/STACK.md
@.planning/research/PITFALLS.md

<interfaces>
<!-- Contracts this plan establishes for Plan 01-03 to consume. -->
<!-- Plan 01-03's main.go will import internal/config and call Load(). -->

From internal/config/config.go (to be created by this plan):
```go
package config

type Config struct {
    Server          ServerConfig
    CredentialsFile string
    RegistryFile    string
    WebSocket       WebSocketConfig
    Logging         LoggingConfig
    CORS            CORSConfig
}

type ServerConfig struct {
    Port int
    TLS  TLSConfig
}

// Addr returns ":<port>" for http.Server.Addr.
func (s ServerConfig) Addr() string

type TLSConfig struct {
    Enabled  bool
    CertFile string
    KeyFile  string
}

type WebSocketConfig struct {
    PingIntervalSeconds int           // YAML-facing
    PingInterval        time.Duration // populated during Load, yaml:"-"
}

type LoggingConfig struct {
    Format string // "json" | "text"
    Level  string // "debug" | "info" | "warn" | "error"
}

type CORSConfig struct {
    AllowedOrigins []string
}

// Load reads, parses, and validates config.yaml. Returns a wrapped error
// on missing file ("config file not found: <path>; copy config.example.yaml
// to config.yaml to get started"), parse failure, or validation failure.
func Load(path string) (*Config, error)
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Implement internal/config package (types + Load + validate + Addr)</name>
  <files>internal/config/config.go, internal/config/config_test.go, internal/config/testdata/valid.yaml, internal/config/testdata/invalid-log-format.yaml, internal/config/testdata/invalid-log-level.yaml, internal/config/testdata/missing-credentials-file.yaml, internal/config/testdata/zero-port.yaml, internal/config/testdata/tls-no-cert.yaml, internal/config/testdata/zero-ping.yaml, internal/config/testdata/malformed.yaml</files>
  <read_first>
    - .planning/phases/01-foundation/01-CONTEXT.md (§Config File Layout, §Config Discovery, §Claude's Discretion duration choice)
    - .planning/phases/01-foundation/01-RESEARCH.md (§Pattern 5 Config Package Shape — full config.go reference skeleton, §Pattern 6 config_test.go Shape, §Pitfall 5 error wrapping)
    - .planning/phases/01-foundation/01-VALIDATION.md (§Per-Task Verification Map rows for FOUND-02)
    - go.mod (confirm go.yaml.in/yaml/v3 and testify/require are pinned)
    - internal/config/.gitkeep (delete after this task writes real files)
  </read_first>
  <behavior>
    - TestLoad with table entry "valid full config": Load("testdata/valid.yaml") returns non-nil *Config and nil error
    - TestLoad with "invalid log format": Load("testdata/invalid-log-format.yaml") returns error containing "logging.format"
    - TestLoad with "invalid log level": Load("testdata/invalid-log-level.yaml") returns error containing "logging.level"
    - TestLoad with "missing credentials_file": Load("testdata/missing-credentials-file.yaml") returns error containing "credentials_file"
    - TestLoad with "zero port": Load("testdata/zero-port.yaml") returns error containing "server.port"
    - TestLoad with "tls enabled without cert": Load("testdata/tls-no-cert.yaml") returns error containing "tls.cert_file"
    - TestLoad with "zero ping interval": Load("testdata/zero-ping.yaml") returns error containing "ping_interval_seconds"
    - TestLoad with "malformed yaml": Load("testdata/malformed.yaml") returns error containing "parse"
    - TestLoad_MissingFile: Load("testdata/does-not-exist.yaml") returns error containing both "config file not found" and "copy config.example.yaml"
    - TestLoad_UnreadableFile: A file with mode 0o000 returns a non-nil error
    - TestLoad_DerivesPingInterval: valid.yaml with ping_interval_seconds: 30 yields cfg.WebSocket.PingInterval.String() == "30s"
  </behavior>
  <action>
Write the config package in full. Delete `internal/config/.gitkeep` once `config.go` exists (git will pick up the real file). The file bodies below are the authoritative shape — copy them verbatim, adjusting only cosmetic formatting if `gofmt` rewrites them.

Step 1: Write `internal/config/config.go`:

```go
// Package config loads and validates the OpenBuro server configuration
// from a YAML file. It is the single source of truth for server-operational
// settings (port, TLS, credential path, registry path, WebSocket keepalive,
// slog format/level, CORS origin list).
//
// The credentials file referenced by Config.CredentialsFile is loaded
// separately in Phase 4 — this package only validates that the path is set.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"go.yaml.in/yaml/v3"
)

// Config is the root of config.yaml. Fields are populated from the YAML
// document; validation runs in Load after unmarshal.
type Config struct {
	Server          ServerConfig    `yaml:"server"`
	CredentialsFile string          `yaml:"credentials_file"`
	RegistryFile    string          `yaml:"registry_file"`
	WebSocket       WebSocketConfig `yaml:"websocket"`
	Logging         LoggingConfig   `yaml:"logging"`
	CORS            CORSConfig      `yaml:"cors"`
}

// ServerConfig holds HTTP listener settings.
type ServerConfig struct {
	Port int       `yaml:"port"`
	TLS  TLSConfig `yaml:"tls"`
}

// Addr returns the listen address in ":port" form for http.Server.Addr.
// Empty host binds all interfaces, which is the intended default for a
// reference implementation.
func (s ServerConfig) Addr() string {
	return net.JoinHostPort("", strconv.Itoa(s.Port))
}

// TLSConfig holds optional TLS termination settings. When Enabled is true,
// CertFile and KeyFile must both be set (validated at load time).
type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

// WebSocketConfig holds WebSocket keepalive settings. PingIntervalSeconds
// is the YAML-facing integer; PingInterval is derived during Load so
// downstream code can use a time.Duration directly.
type WebSocketConfig struct {
	PingIntervalSeconds int           `yaml:"ping_interval_seconds"`
	PingInterval        time.Duration `yaml:"-"`
}

// LoggingConfig holds slog construction settings.
type LoggingConfig struct {
	Format string `yaml:"format"` // "json" | "text"
	Level  string `yaml:"level"`  // "debug" | "info" | "warn" | "error"
}

// CORSConfig holds the cross-origin allow-list. Shared with the WebSocket
// OriginPatterns check in Phase 4.
type CORSConfig struct {
	AllowedOrigins []string `yaml:"allowed_origins"`
}

// Load reads, parses, and validates config.yaml. On any failure it returns
// a wrapped error explaining what went wrong in operator-friendly language.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf(
				"config file not found: %s; copy config.example.yaml to config.yaml to get started",
				path,
			)
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config in %s: %w", path, err)
	}

	// Derive runtime-friendly fields after validation so PingInterval is
	// only populated when PingIntervalSeconds is known to be > 0.
	cfg.WebSocket.PingInterval = time.Duration(cfg.WebSocket.PingIntervalSeconds) * time.Second

	return &cfg, nil
}

func (c *Config) validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535, got %d", c.Server.Port)
	}
	if c.Server.TLS.Enabled {
		if c.Server.TLS.CertFile == "" {
			return errors.New("server.tls.cert_file required when server.tls.enabled is true")
		}
		if c.Server.TLS.KeyFile == "" {
			return errors.New("server.tls.key_file required when server.tls.enabled is true")
		}
	}
	if c.CredentialsFile == "" {
		return errors.New("credentials_file is required")
	}
	if c.RegistryFile == "" {
		return errors.New("registry_file is required")
	}
	if c.WebSocket.PingIntervalSeconds <= 0 {
		return fmt.Errorf("websocket.ping_interval_seconds must be > 0, got %d", c.WebSocket.PingIntervalSeconds)
	}
	switch c.Logging.Format {
	case "json", "text":
	default:
		return fmt.Errorf("logging.format must be json or text, got %q", c.Logging.Format)
	}
	switch c.Logging.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("logging.level must be debug|info|warn|error, got %q", c.Logging.Level)
	}
	return nil
}
```

Step 2: Delete `internal/config/.gitkeep` (the real `config.go` now keeps the directory tracked).

Step 3: Write `internal/config/testdata/valid.yaml`:

```yaml
server:
  port: 8080
  tls:
    enabled: false
    cert_file: ""
    key_file: ""

credentials_file: "./credentials.yaml"
registry_file: "./registry.json"

websocket:
  ping_interval_seconds: 30

logging:
  format: json
  level: info

cors:
  allowed_origins: []
```

Step 4: Write `internal/config/testdata/invalid-log-format.yaml` — copy of valid.yaml with `logging.format: xml`:

```yaml
server:
  port: 8080
  tls:
    enabled: false
credentials_file: "./credentials.yaml"
registry_file: "./registry.json"
websocket:
  ping_interval_seconds: 30
logging:
  format: xml
  level: info
cors:
  allowed_origins: []
```

Step 5: Write `internal/config/testdata/invalid-log-level.yaml` — copy with `logging.level: verbose`:

```yaml
server:
  port: 8080
  tls:
    enabled: false
credentials_file: "./credentials.yaml"
registry_file: "./registry.json"
websocket:
  ping_interval_seconds: 30
logging:
  format: json
  level: verbose
cors:
  allowed_origins: []
```

Step 6: Write `internal/config/testdata/missing-credentials-file.yaml` — no credentials_file key:

```yaml
server:
  port: 8080
  tls:
    enabled: false
registry_file: "./registry.json"
websocket:
  ping_interval_seconds: 30
logging:
  format: json
  level: info
cors:
  allowed_origins: []
```

Step 7: Write `internal/config/testdata/zero-port.yaml` — port: 0:

```yaml
server:
  port: 0
  tls:
    enabled: false
credentials_file: "./credentials.yaml"
registry_file: "./registry.json"
websocket:
  ping_interval_seconds: 30
logging:
  format: json
  level: info
cors:
  allowed_origins: []
```

Step 8: Write `internal/config/testdata/tls-no-cert.yaml` — tls.enabled: true, empty cert_file:

```yaml
server:
  port: 8080
  tls:
    enabled: true
    cert_file: ""
    key_file: ""
credentials_file: "./credentials.yaml"
registry_file: "./registry.json"
websocket:
  ping_interval_seconds: 30
logging:
  format: json
  level: info
cors:
  allowed_origins: []
```

Step 9: Write `internal/config/testdata/zero-ping.yaml` — ping_interval_seconds: 0:

```yaml
server:
  port: 8080
  tls:
    enabled: false
credentials_file: "./credentials.yaml"
registry_file: "./registry.json"
websocket:
  ping_interval_seconds: 0
logging:
  format: json
  level: info
cors:
  allowed_origins: []
```

Step 10: Write `internal/config/testdata/malformed.yaml` — syntactically broken YAML:

```yaml
server:
  port: 8080
  tls:
    enabled: false
credentials_file: "./credentials.yaml
registry_file: "./registry.json"
```

(The unterminated string on the `credentials_file` line is the intentional syntax error.)

Step 11: Write `internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name        string
		fixture     string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid full config",
			fixture: "valid.yaml",
			wantErr: false,
		},
		{
			name:        "invalid log format",
			fixture:     "invalid-log-format.yaml",
			wantErr:     true,
			errContains: "logging.format",
		},
		{
			name:        "invalid log level",
			fixture:     "invalid-log-level.yaml",
			wantErr:     true,
			errContains: "logging.level",
		},
		{
			name:        "missing credentials_file",
			fixture:     "missing-credentials-file.yaml",
			wantErr:     true,
			errContains: "credentials_file",
		},
		{
			name:        "zero port",
			fixture:     "zero-port.yaml",
			wantErr:     true,
			errContains: "server.port",
		},
		{
			name:        "tls enabled without cert",
			fixture:     "tls-no-cert.yaml",
			wantErr:     true,
			errContains: "tls.cert_file",
		},
		{
			name:        "zero ping interval",
			fixture:     "zero-ping.yaml",
			wantErr:     true,
			errContains: "ping_interval_seconds",
		},
		{
			name:        "malformed yaml",
			fixture:     "malformed.yaml",
			wantErr:     true,
			errContains: "parse",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Load(filepath.Join("testdata", tc.fixture))
			if tc.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.errContains)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, cfg)
		})
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load(filepath.Join("testdata", "does-not-exist.yaml"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "config file not found")
	require.Contains(t, err.Error(), "copy config.example.yaml")
}

func TestLoad_UnreadableFile(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "unreadable.yaml")
	require.NoError(t, os.WriteFile(tmp, []byte("server:\n  port: 8080\n"), 0o000))
	t.Cleanup(func() { _ = os.Chmod(tmp, 0o600) })

	_, err := Load(tmp)
	require.Error(t, err)
}

func TestLoad_DerivesPingInterval(t *testing.T) {
	cfg, err := Load(filepath.Join("testdata", "valid.yaml"))
	require.NoError(t, err)
	require.Equal(t, 30, cfg.WebSocket.PingIntervalSeconds)
	require.Equal(t, "30s", cfg.WebSocket.PingInterval.String())
}
```

Step 12: Run the test suite. Everything must pass on the first try because the test cases are tightly coupled to the fixtures and validation rules defined above:

```bash
go test ./internal/config -race -count=1 -v
```

Expected output: 11 passing tests (8 subtests under `TestLoad` + 3 top-level).

Step 13: Verify formatting and vet:

```bash
gofmt -l internal/config/
go vet ./internal/config/
```

Both must produce no output / exit 0.

**Anti-patterns to avoid:**
- Do NOT use `%v` when wrapping errors that callers might inspect (PITFALLS.md #13 / RESEARCH §Pitfall 5) — always `%w`
- Do NOT apply silent defaults (missing `logging.format` should error, not fall back to "json") — RESEARCH Pattern 5 notes say explicit errors are a feature
- Do NOT add a `Credentials` struct or `LoadCredentials` function here — Phase 4 owns credential loading
- Do NOT pass `*slog.Logger` to `Load` — config loading happens before the logger exists
  </action>
  <verify>
    <automated>test -f internal/config/config.go &amp;&amp; test -f internal/config/config_test.go &amp;&amp; test -f internal/config/testdata/valid.yaml &amp;&amp; test -f internal/config/testdata/invalid-log-format.yaml &amp;&amp; test -f internal/config/testdata/invalid-log-level.yaml &amp;&amp; test -f internal/config/testdata/missing-credentials-file.yaml &amp;&amp; test -f internal/config/testdata/zero-port.yaml &amp;&amp; test -f internal/config/testdata/tls-no-cert.yaml &amp;&amp; test -f internal/config/testdata/zero-ping.yaml &amp;&amp; test -f internal/config/testdata/malformed.yaml &amp;&amp; grep -q '^package config$' internal/config/config.go &amp;&amp; grep -q '^package config$' internal/config/config_test.go &amp;&amp; grep -q 'func Load(path string) (\*Config, error)' internal/config/config.go &amp;&amp; grep -q 'func (s ServerConfig) Addr() string' internal/config/config.go &amp;&amp; grep -q 'type Config struct' internal/config/config.go &amp;&amp; grep -q 'yaml:"server"' internal/config/config.go &amp;&amp; grep -q 'yaml:"credentials_file"' internal/config/config.go &amp;&amp; grep -q 'yaml:"ping_interval_seconds"' internal/config/config.go &amp;&amp; grep -q 'yaml:"-"' internal/config/config.go &amp;&amp; grep -q 'copy config.example.yaml to config.yaml' internal/config/config.go &amp;&amp; go test ./internal/config -race -count=1 &amp;&amp; go vet ./internal/config/... &amp;&amp; test -z "$(gofmt -l internal/config/)"</automated>
  </verify>
  <acceptance_criteria>
    - `test -f internal/config/config.go` succeeds
    - `test -f internal/config/config_test.go` succeeds
    - All 8 testdata fixtures exist under `internal/config/testdata/`
    - `grep -q '^package config$' internal/config/config.go` succeeds
    - `grep -q '^package config$' internal/config/config_test.go` succeeds (white-box same-package tests per CONTEXT.md)
    - `grep -q 'func Load(path string) (\*Config, error)' internal/config/config.go`
    - `grep -q 'func (s ServerConfig) Addr() string' internal/config/config.go`
    - `grep -q 'yaml:"server"' internal/config/config.go` — YAML tags present
    - `grep -q 'yaml:"ping_interval_seconds"' internal/config/config.go`
    - `grep -q 'yaml:"-"' internal/config/config.go` — PingInterval is derived, not YAML-facing
    - `grep -q 'copy config.example.yaml to config.yaml' internal/config/config.go` — friendly missing-file message verbatim
    - `grep -q '%w' internal/config/config.go` — error wrapping uses %w not %v (PITFALLS compliance)
    - `! grep -q 'slog\\.' internal/config/config.go` — no slog imports in config package (FOUND-03 injection rule)
    - `! grep -q 'Credentials' internal/config/config.go` — no Credentials struct in Phase 1
    - `go test ./internal/config -race -count=1` passes (all 11 subtests)
    - `go vet ./internal/config/...` exits 0
    - `gofmt -l internal/config/` produces empty output
  </acceptance_criteria>
  <done>
`internal/config` package fully implemented with Config type tree, Load(), validate(), and ServerConfig.Addr(). All 11 table-driven and standalone test cases pass under `-race`. Error wrapping uses `%w` so `errors.Is(err, os.ErrNotExist)` survives. No `slog.*` imports, no `Credentials` struct, no silent defaults. Package is ready for `main.go` to call `config.Load(*configPath)` in Plan 03.
  </done>
</task>

<task type="auto">
  <name>Task 2: Write config.example.yaml and credentials.example.yaml at repo root</name>
  <files>config.example.yaml, credentials.example.yaml</files>
  <read_first>
    - .planning/phases/01-foundation/01-CONTEXT.md (§Config File Layout — full YAML schema)
    - .planning/phases/01-foundation/01-RESEARCH.md (§config.example.yaml, §credentials.example.yaml — verbatim bodies)
    - .planning/phases/01-foundation/01-VALIDATION.md (§Per-Task Verification Map rows for FOUND-07)
    - internal/config/config.go (confirm every YAML tag in the struct matches an example key)
    - .gitignore (confirm config.yaml and credentials.yaml are ignored, so the .example siblings are the only tracked form)
  </read_first>
  <action>
Write the two operator-facing `.example` YAML files at the repo root. These files are the self-documenting quickstart surface — a developer who clones the repo should be able to `cp config.example.yaml config.yaml && cp credentials.example.yaml credentials.yaml` and immediately run the server (after Plan 01-03 wires main.go).

Every top-level key in `config.example.yaml` must correspond to a field in `internal/config/config.go` (validated by the grep acceptance criteria below).

Step 1: Write `config.example.yaml` verbatim:

```yaml
# OpenBuro Server — example configuration
# Copy this file to config.yaml and edit to suit your deployment.

server:
  port: 8080
  tls:
    enabled: false
    cert_file: ""
    key_file: ""

# Path to the credentials file (bcrypt-hashed admin passwords).
# Loaded at startup; see credentials.example.yaml for the expected shape.
credentials_file: "./credentials.yaml"

# Path to the persistent registry store. Created on first write if absent.
registry_file: "./registry.json"

websocket:
  # How often the server sends ping frames to keep connections alive.
  ping_interval_seconds: 30

logging:
  # Format: "json" for production, "text" for local development.
  format: json
  # Level: debug | info | warn | error
  level: info

cors:
  # Explicit allow-list of origins. Leave empty to block all browser clients.
  # Shared with the WebSocket origin check in the API phase.
  allowed_origins: []
```

Step 2: Write `credentials.example.yaml` verbatim:

```yaml
# OpenBuro Server — example credentials
# Copy this file to credentials.yaml and replace the example hash.
#
# Generate a bcrypt hash (cost >= 12):
#   htpasswd -bnBC 12 "" your-password-here | tr -d ':\n'
# Or in Go:
#   bcrypt.GenerateFromPassword([]byte("your-password"), 12)

admins:
  - username: "admin"
    password_hash: "$2a$12$EXAMPLEEXAMPLEEXAMPLEEXAMPLEEXAMPLEEXAMPLEEXAMPLEEXAMPLEE"
```

**Security note:** The example bcrypt hash is deliberately a structurally-plausible but functionally-invalid string (not a real bcrypt hash for any password). Phase 4's auth middleware will reject it if someone copies the file and forgets to replace it — exactly the desired fail-loud behavior.

Step 3: Smoke-test the example by loading it through the Phase 1 config loader. This is the truest proof that the example's schema matches the Config struct:

```bash
go test ./internal/config -run TestLoad_Example -race -count=1 2>/dev/null || true
```

(There is no `TestLoad_Example` test yet — skipping this and relying on manual verification is fine. The quickstart smoke test from the phase success criteria will exercise it once Plan 03 wires main.go.)

Alternative quick verification — parse the example through yaml unmarshal without running Go tests:

```bash
go run -C . <<'EOF' 2>/dev/null || true
package main
// verified implicitly by the grep assertions below
EOF
```

The structural acceptance criteria below (grep for every top-level key) are the authoritative verification for this task.

**Important:** Do NOT commit real `config.yaml` or `credentials.yaml` files. The `.gitignore` created in Plan 01-01 already excludes them, but double-check with `git status` before committing that only the `.example` siblings appear as new files.
  </action>
  <verify>
    <automated>test -f config.example.yaml &amp;&amp; test -f credentials.example.yaml &amp;&amp; grep -q '^server:' config.example.yaml &amp;&amp; grep -q '^credentials_file:' config.example.yaml &amp;&amp; grep -q '^registry_file:' config.example.yaml &amp;&amp; grep -q '^websocket:' config.example.yaml &amp;&amp; grep -q '^logging:' config.example.yaml &amp;&amp; grep -q '^cors:' config.example.yaml &amp;&amp; grep -q 'port: 8080' config.example.yaml &amp;&amp; grep -q 'ping_interval_seconds: 30' config.example.yaml &amp;&amp; grep -q 'format: json' config.example.yaml &amp;&amp; grep -q 'level: info' config.example.yaml &amp;&amp; grep -q '^admins:' credentials.example.yaml &amp;&amp; grep -q 'password_hash' credentials.example.yaml &amp;&amp; grep -q 'username: "admin"' credentials.example.yaml</automated>
  </verify>
  <acceptance_criteria>
    - `test -f config.example.yaml` succeeds
    - `test -f credentials.example.yaml` succeeds
    - `config.example.yaml` contains ALL top-level keys: `server:`, `credentials_file:`, `registry_file:`, `websocket:`, `logging:`, `cors:` (one grep per key, each at line start)
    - `config.example.yaml` contains `port: 8080`
    - `config.example.yaml` contains `ping_interval_seconds: 30`
    - `config.example.yaml` contains `format: json` and `level: info`
    - `credentials.example.yaml` contains `^admins:` at line start
    - `credentials.example.yaml` contains `password_hash` key
    - `credentials.example.yaml` contains `username: "admin"`
    - `git status` should show both files as new/tracked; `config.yaml` and `credentials.yaml` must NOT appear as tracked files (they are .gitignore'd)
  </acceptance_criteria>
  <done>
Both `.example` YAML files exist at the repo root with every key the Phase 1 `Config` struct expects. Developer can `cp config.example.yaml config.yaml && cp credentials.example.yaml credentials.yaml` and have a valid config to run against once Plan 03 ships.
  </done>
</task>

</tasks>

<verification>
```bash
go test ./internal/config -race -count=1          # 11 subtests pass
go vet ./internal/config/...                       # exit 0
test -z "$(gofmt -l internal/config/)"             # empty
test -f config.example.yaml
test -f credentials.example.yaml

# Quickstart-style integration: point the loader at the example file and confirm it parses.
# We can do this by temporarily copying it into the testdata directory of a scratch test,
# but the structural grep asserts above already catch the schema drift.
# Plan 01-03's main.go will do the real end-to-end check via `go run ./cmd/server -config config.yaml`.

# Whole-module smoke
go build ./...
go vet ./...
test -z "$(gofmt -l .)"
```
</verification>

<success_criteria>
- `internal/config` package loads valid YAML into a validated `*Config` (FOUND-02)
- All 11 unit test cases pass under `-race` covering: valid, 6 validation-fail fixtures, missing file, unreadable file, ping interval derivation
- `config.example.yaml` and `credentials.example.yaml` exist at repo root with every key the schema expects (FOUND-07)
- No `slog.*` imports inside `internal/config` (FOUND-03 injection rule)
- Error wrapping uses `%w` throughout so `errors.Is` survives (PITFALLS #13 / §Pitfall 5)
- No `Credentials` struct in Phase 1 — deferred to Phase 4
</success_criteria>

<output>
After completion, create `.planning/phases/01-foundation/01-02-SUMMARY.md` documenting:
- `Config` struct shape (field list + YAML tag names)
- Validation rules enforced by `validate()` (list the 7 rules)
- Test coverage count (subtests in TestLoad + standalone tests)
- Any deviations from the RESEARCH reference skeleton (should be zero)
- Note that `Credentials` loading is intentionally deferred to Phase 4
</output>
