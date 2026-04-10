package wshub

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

// syncBuffer is a mutex-guarded bytes.Buffer wrapper for log capture in
// tests. Required because slog handlers write from the Subscribe writer
// goroutine concurrently with the test goroutine reading via String()
// inside require.Eventually, which races a plain bytes.Buffer.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// subscribeHandler is a minimal httptest handler that accepts a WS
// conn and hands it to hub.Subscribe. Used by every integration test
// in this file.
func subscribeHandler(hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		_ = hub.Subscribe(r.Context(), conn)
	}
}

// waitForSubscribers polls hub.subscribers length via the internal mu.
// This is the justification for internal tests — the public API
// deliberately does not expose Stats() or NumSubscribers().
func waitForSubscribers(t *testing.T, hub *Hub, want int, within time.Duration) {
	t.Helper()
	require.Eventually(t, func() bool {
		hub.mu.Lock()
		defer hub.mu.Unlock()
		return len(hub.subscribers) == want
	}, within, 10*time.Millisecond, "expected %d subscribers", want)
}

func TestHub_Publish_FanOut(t *testing.T) {
	hub := New(testLogger(t), Options{PingInterval: 50 * time.Millisecond})
	srv := httptest.NewServer(subscribeHandler(hub))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Two subscribers.
	conn1, _, err := websocket.Dial(ctx, srv.URL, nil)
	require.NoError(t, err)
	defer conn1.CloseNow()

	conn2, _, err := websocket.Dial(ctx, srv.URL, nil)
	require.NoError(t, err)
	defer conn2.CloseNow()

	waitForSubscribers(t, hub, 2, time.Second)

	// Publish one message; both subscribers must receive it.
	hub.Publish([]byte("hello"))

	readOne := func(c *websocket.Conn) []byte {
		rctx, rcancel := context.WithTimeout(ctx, 2*time.Second)
		defer rcancel()
		_, data, err := c.Read(rctx)
		require.NoError(t, err)
		return data
	}
	require.Equal(t, []byte("hello"), readOne(conn1))
	require.Equal(t, []byte("hello"), readOne(conn2))
}

func TestHub_SlowConsumerDropped(t *testing.T) {
	hub := New(testLogger(t), Options{
		MessageBuffer: 1,
		PingInterval:  10 * time.Millisecond,
	})
	srv := httptest.NewServer(subscribeHandler(hub))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, srv.URL, nil)
	require.NoError(t, err)
	defer conn.CloseNow()

	// Do NOT read from conn — this is the slow-consumer simulation.
	waitForSubscribers(t, hub, 1, time.Second)

	// Publish enough messages to overflow the 1-slot buffer.
	for i := 0; i < 5; i++ {
		hub.Publish([]byte("msg"))
	}

	// The slow subscriber must be kicked. closeSlow fires conn.Close
	// (StatusPolicyViolation) which has a 5s+5s handshake budget when
	// the peer never reads — we allow up to 7 seconds to let the
	// handshake time out and the writer loop observe c.closed.
	waitForSubscribers(t, hub, 0, 7*time.Second)
}

func TestHub_Close_GoingAway(t *testing.T) {
	hub := New(testLogger(t), Options{PingInterval: 50 * time.Millisecond})
	srv := httptest.NewServer(subscribeHandler(hub))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, srv.URL, nil)
	require.NoError(t, err)
	defer conn.CloseNow()

	waitForSubscribers(t, hub, 1, time.Second)

	// First Close: kicks all subscribers, sets closed=true.
	hub.Close()

	hub.mu.Lock()
	require.True(t, hub.closed, "Close must set h.closed = true")
	hub.mu.Unlock()

	// closeGoingAway fires conn.Close(StatusGoingAway) which has the
	// same 5s+5s handshake budget as closeSlow when the peer never
	// reads — allow up to 7 seconds for the writer loop to observe
	// c.closed and defer removeSubscriber.
	waitForSubscribers(t, hub, 0, 7*time.Second)

	// Second Close: idempotent no-op (must not panic).
	require.NotPanics(t, func() { hub.Close() })
}

func TestHub_Publish_AfterCloseIsNoOp(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	hub := New(logger, Options{})

	// Close an empty hub, then publish. No panic, no Warn drop log,
	// and the only log line should be the Info "closing hub" from
	// Close itself (subscribers=0).
	hub.Close()
	hub.Publish([]byte("noise"))

	out := buf.String()
	require.Contains(t, out, "wshub: closing hub", "Close must log Info once")
	require.Contains(t, out, "subscribers=0", "Close log carries subscribers count")
	require.NotContains(t, out, "wshub: subscriber dropped", "Publish after Close must NOT fan out")
}

// piiSubstrings enumerates strings that MUST NOT appear in any wshub
// log line. The hub is byte-oriented and has no notion of client
// identity; logging any of these would violate the no-PII contract
// from 03-CONTEXT.md §"Drop-subscriber logging".
var piiSubstrings = []string{
	"peer_addr",
	"remote_addr",
	"RemoteAddr",
	"user_agent",
	"User-Agent",
	"authorization",
	"Authorization",
	"Basic ",
	"Bearer ",
	"username",
	"password",
}

func requireNoPII(t *testing.T, out string) {
	t.Helper()
	for _, pii := range piiSubstrings {
		require.NotContainsf(t, out, pii,
			"no-PII contract violated: %q appeared in log output", pii)
	}
}

func TestHub_Logging_DropIsWarn(t *testing.T) {
	var buf syncBuffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	hub := New(logger, Options{
		MessageBuffer: 1,
		PingInterval:  10 * time.Millisecond,
	})
	srv := httptest.NewServer(subscribeHandler(hub))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, srv.URL, nil)
	require.NoError(t, err)
	defer conn.CloseNow()

	// Slow-consumer simulation: never read from conn.
	waitForSubscribers(t, hub, 1, time.Second)

	for i := 0; i < 5; i++ {
		hub.Publish([]byte("msg"))
	}

	// Wait for the Warn line to appear.
	require.Eventually(t, func() bool {
		return strings.Contains(buf.String(), "wshub: subscriber dropped (slow consumer)")
	}, time.Second, 10*time.Millisecond, "drop Warn log never appeared")

	out := buf.String()
	require.Contains(t, out, "level=WARN", "drop log must be at Warn level")
	require.Contains(t, out, "buffer_size=1", "drop log must carry buffer_size field")
	requireNoPII(t, out)
}

func TestHub_Logging_CloseIsInfo(t *testing.T) {
	var buf syncBuffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	hub := New(logger, Options{})

	hub.Close()

	out := buf.String()
	require.Contains(t, out, "wshub: closing hub", "Close must log Info")
	require.Contains(t, out, "level=INFO", "close log must be at Info level")
	require.Contains(t, out, "subscribers=0", "close log must carry subscribers count")
	require.Equal(t, 1, strings.Count(out, "wshub: closing hub"),
		"Close must log exactly one line")
	require.NotContains(t, out, "level=WARN", "Close must not log at Warn")
	require.NotContains(t, out, "level=ERROR", "Close must not log at Error")
	requireNoPII(t, out)
}

func TestHub_Logging_NoPII(t *testing.T) {
	var buf syncBuffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	hub := New(logger, Options{
		MessageBuffer: 1,
		PingInterval:  10 * time.Millisecond,
	})
	srv := httptest.NewServer(subscribeHandler(hub))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Full lifecycle: connect, publish (drops the slow subscriber),
	// close the hub. The captured log buffer must contain NONE of
	// the PII substrings across this entire flow.
	conn, _, err := websocket.Dial(ctx, srv.URL, nil)
	require.NoError(t, err)
	defer conn.CloseNow()

	waitForSubscribers(t, hub, 1, time.Second)

	for i := 0; i < 5; i++ {
		hub.Publish([]byte("msg"))
	}

	waitForSubscribers(t, hub, 0, 7*time.Second)

	hub.Close()

	// Give any deferred log writes a chance to flush via require.Eventually
	// (polling, not blocking). The condition is immediately true on entry
	// in the common case; the Eventually loop is just a safety margin.
	require.Eventually(t, func() bool {
		return strings.Contains(buf.String(), "wshub: closing hub")
	}, time.Second, 10*time.Millisecond)

	requireNoPII(t, buf.String())
}
