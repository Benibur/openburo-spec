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
