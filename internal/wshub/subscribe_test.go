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

// TestSubscribe_PingKeepsAlive asserts that a connected subscriber stays
// registered across many ping intervals with no inbound or outbound data
// traffic. If the ping loop were broken (PITFALLS #9: Ping requires a
// concurrent reader — CloseRead provides it), the subscriber would be
// dropped and h.subscribers would shrink to 0.
//
// We use require.Never (not require.Eventually) to assert the condition
// "len(h.subscribers) == 0" NEVER becomes true across 300ms — at a
// PingInterval of 10ms, that's 30+ ping cycles of aliveness.
func TestSubscribe_PingKeepsAlive(t *testing.T) {
	hub := New(testLogger(t), Options{PingInterval: 10 * time.Millisecond})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		_ = hub.Subscribe(r.Context(), conn)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, srv.URL, nil)
	require.NoError(t, err)
	defer conn.CloseNow()

	// Wait until the subscriber is registered.
	require.Eventually(t, func() bool {
		hub.mu.Lock()
		defer hub.mu.Unlock()
		return len(hub.subscribers) == 1
	}, time.Second, 10*time.Millisecond)

	// Across 300ms (30+ ping cycles), the subscriber must stay
	// registered. If Ping silently blocks because CloseRead is missing
	// or PingTimeout is ignored, this assertion fails.
	require.Never(t, func() bool {
		hub.mu.Lock()
		defer hub.mu.Unlock()
		return len(hub.subscribers) == 0
	}, 300*time.Millisecond, 20*time.Millisecond,
		"ping keepalive failed — subscriber was dropped during idle period")
}
