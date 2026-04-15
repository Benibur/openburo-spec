package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestGracefulShutdown is the Phase 5 correctness oracle for OPS-03 + OPS-04:
// it spawns run(ctx) in a goroutine, waits for the HTTP listener to bind,
// cancels ctx, and asserts run returns nil within 20s (two-phase shutdown
// has a 15s budget plus overhead). No time.Sleep — require.Eventually polls
// the listener.
func TestGracefulShutdown(t *testing.T) {
	dir := t.TempDir()

	// Reuse a cost-12 bcrypt hash so LoadCredentials accepts it.
	// The password is "admin" — not that it matters here, we never auth.
	credsPath := filepath.Join(dir, "credentials.yaml")
	credsContent := `users:
  admin: "$2a$12$11xOeoRoS9eFHu31.5.VGedG5fbjtsU3hqveeU22tMnsahELFfiX6"
`
	require.NoError(t, os.WriteFile(credsPath, []byte(credsContent), 0o600))

	regPath := filepath.Join(dir, "registry.json")
	cfgPath := filepath.Join(dir, "config.yaml")
	cfgContent := fmt.Sprintf(`server:
  port: 18089
  tls:
    enabled: false
credentials_file: %q
registry_file: %q
websocket:
  ping_interval_seconds: 30
logging:
  format: text
  level: error
cors:
  allowed_origins:
    - "http://localhost:3000"
`, credsPath, regPath)
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfgContent), 0o600))

	oldArgs := os.Args
	os.Args = []string{"openburo-server-test", "-config", cfgPath}
	defer func() { os.Args = oldArgs }()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- run(ctx) }()

	// Wait for the listener to be ready.
	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:18089", 50*time.Millisecond)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, 2*time.Second, 25*time.Millisecond, "server never bound 127.0.0.1:18089")

	// Trigger shutdown.
	cancel()

	select {
	case err := <-errCh:
		require.NoError(t, err, "run() should return nil on clean shutdown")
	case <-time.After(20 * time.Second):
		t.Fatal("run() did not return within 20s of ctx cancel")
	}
}
