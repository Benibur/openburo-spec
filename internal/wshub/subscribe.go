package wshub

import (
	"context"
	"errors"
	"time"

	"github.com/coder/websocket"
)

// subscriber represents a single WebSocket subscriber. Messages are
// sent on msgs; if the client cannot keep up, closeSlow is called by
// Publish. closeGoingAway is called by Hub.Close at shutdown.
//
// Both close callbacks are pre-bound to the *websocket.Conn at
// Subscribe time so the close-code decision lives at the boundary
// where the conn is known, keeping Publish and Close branch-free.
type subscriber struct {
	msgs           chan []byte
	closeSlow      func() // Publish path: StatusPolicyViolation
	closeGoingAway func() // Hub.Close path: StatusGoingAway
}

// Subscribe registers a new WebSocket subscriber on the hub and blocks
// until the client disconnects or ctx is canceled. It is the caller's
// responsibility to call websocket.Accept before handing the conn in,
// and to close the conn if Subscribe returns a non-context error.
//
// The method installs conn.CloseRead(ctx) at the top so control frames
// (ping, pong, close) are handled and ctx cancels on peer disconnect.
// It also installs `defer h.removeSubscriber(s)` so silent disconnects
// cannot leak the writer goroutine (PITFALLS #3), and `defer
// conn.CloseNow()` as a safety-net that reaps the TCP conn on any
// unexpected return path.
//
// Return values:
//   - nil — normal disconnect (ctx canceled, peer closed cleanly)
//   - wrapped error from conn.Write / conn.Ping — write or ping
//     failure (including the post-kick error after closeSlow or
//     closeGoingAway fires on the conn)
//
// Callers (Phase 4 HTTP handlers) should treat context.Canceled and
// context.DeadlineExceeded as non-errors for logging purposes.
func (h *Hub) Subscribe(ctx context.Context, conn *websocket.Conn) error {
	// CloseRead spawns an internal reader goroutine that handles
	// ping/pong/close control frames and cancels ctx when the peer
	// closes. Assigning the returned ctx back into the local ctx is
	// load-bearing — the writer loop's <-ctx.Done() branch observes
	// this cancellation.
	ctx = conn.CloseRead(ctx)

	s := &subscriber{
		msgs: make(chan []byte, h.opts.MessageBuffer),
		closeSlow: func() {
			conn.Close(websocket.StatusPolicyViolation, "subscriber too slow")
		},
		closeGoingAway: func() {
			conn.Close(websocket.StatusGoingAway, "server shutting down")
		},
	}
	h.addSubscriber(s)
	defer h.removeSubscriber(s)
	defer conn.CloseNow()

	// Per-subscriber ping ticker. The ping loop case lands fully in
	// Plan 03-02; the ticker is declared here so the select shape is
	// final and 03-02 only needs to fill the case body with real ping
	// logic. Using time.NewTicker (not time.After) per ARCHITECTURE.md
	// Pattern 2 and 03-CONTEXT.md "Claude's Discretion".
	tick := time.NewTicker(h.opts.PingInterval)
	defer tick.Stop()

	for {
		select {
		case msg := <-s.msgs:
			writeCtx, cancel := context.WithTimeout(ctx, h.opts.WriteTimeout)
			err := conn.Write(writeCtx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return nil
				}
				h.logger.Debug("wshub: subscriber writer loop exited", "error", err.Error())
				return err
			}
		case <-tick.C:
			// TODO(03-02): real ping via conn.Ping(pingCtx) with
			// h.opts.PingTimeout. Plan 03-01 keeps this a no-op so the
			// ticker is wired but silent; the 1000-cycle goroutine-leak
			// test does not depend on ping traffic.
		case <-ctx.Done():
			return nil
		}
	}
}
