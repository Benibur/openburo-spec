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
