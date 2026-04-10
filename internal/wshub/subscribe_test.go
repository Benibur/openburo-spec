package wshub

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

// testLogger returns a discard-backed slog logger for tests that don't
// care about log output. Tests that DO capture logs (see hub_test.go in
// Plan 03-03) construct their own logger with a bytes.Buffer handler.
func testLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestHub_New_PanicsOnNilLogger(t *testing.T) {
	require.PanicsWithValue(
		t,
		"wshub.New: logger is required; use slog.New(slog.NewTextHandler(io.Discard, nil)) in tests",
		func() { New(nil, Options{}) },
	)
}

func TestHub_DefaultOptions(t *testing.T) {
	// Zero-value Options should be replaced with package defaults.
	h := New(testLogger(t), Options{})
	require.Equal(t, 16, h.opts.MessageBuffer, "default MessageBuffer")
	require.Equal(t, 30*time.Second, h.opts.PingInterval, "default PingInterval")
	require.Equal(t, 5*time.Second, h.opts.WriteTimeout, "default WriteTimeout")
	require.Equal(t, 10*time.Second, h.opts.PingTimeout, "default PingTimeout")

	// Non-zero overrides should be preserved exactly.
	h2 := New(testLogger(t), Options{
		MessageBuffer: 4,
		PingInterval:  5 * time.Millisecond,
		WriteTimeout:  100 * time.Millisecond,
		PingTimeout:   200 * time.Millisecond,
	})
	require.Equal(t, 4, h2.opts.MessageBuffer)
	require.Equal(t, 5*time.Millisecond, h2.opts.PingInterval)
	require.Equal(t, 100*time.Millisecond, h2.opts.WriteTimeout)
	require.Equal(t, 200*time.Millisecond, h2.opts.PingTimeout)
}

// TestSubscribe_NoGoroutineLeak is THE correctness test for Phase 3.
// 1000 connect-then-disconnect cycles against an httptest.NewServer-backed
// hub MUST end with runtime.NumGoroutine() <= baseline+5. The +5 epsilon
// accounts for in-flight teardown goroutines caught mid-exit by the poll.
//
// This test enforces PITFALLS #3: CloseRead + defer removeSubscriber.
// If a future refactor drops either one, this test fails within seconds.
func TestSubscribe_NoGoroutineLeak(t *testing.T) {
	hub := New(testLogger(t), Options{PingInterval: 50 * time.Millisecond})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		_ = hub.Subscribe(r.Context(), conn)
	}))
	defer srv.Close()

	// Allow the httptest accept-loop goroutine to settle into the baseline.
	runtime.GC()
	baseline := runtime.NumGoroutine()

	for i := 0; i < 1000; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		conn, _, err := websocket.Dial(ctx, srv.URL, nil)
		require.NoError(t, err, "dial cycle %d", i)
		_ = conn.Close(websocket.StatusNormalClosure, "")
		cancel()
	}

	// Poll until the writer loops observe ctx.Done() and exit.
	// runtime.GC() inside the closure reaps finalizer-held conns.
	require.Eventually(t, func() bool {
		runtime.GC()
		return runtime.NumGoroutine() <= baseline+5
	}, 2*time.Second, 20*time.Millisecond,
		"goroutines did not drain after 1000 disconnect cycles")
}
