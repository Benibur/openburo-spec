package wshub

import (
	"log/slog"
	"sync"
	"time"
)

// Default values used when Options fields are zero. Documented on the
// Options struct fields; duplicated here as unexported package-level
// constants so tests and the New constructor share one source of truth.
const (
	defaultMessageBuffer = 16
	defaultPingInterval  = 30 * time.Second
	defaultWriteTimeout  = 5 * time.Second
	defaultPingTimeout   = 10 * time.Second
)

// Options configures a Hub. Zero-value fields use documented defaults.
type Options struct {
	// MessageBuffer is the per-subscriber outbound channel capacity.
	// Zero means use the package default (16). Slow subscribers whose
	// buffer fills are kicked via closeSlow.
	MessageBuffer int

	// PingInterval is how often the Hub sends a WebSocket ping frame
	// to keep idle connections alive. Zero means use the package
	// default (30 * time.Second).
	PingInterval time.Duration

	// WriteTimeout bounds each conn.Write call so a wedged connection
	// cannot stall the writer goroutine. Zero means use the package
	// default (5 * time.Second).
	WriteTimeout time.Duration

	// PingTimeout bounds each conn.Ping call separately from
	// WriteTimeout so the two budgets don't compete. Zero means use
	// the package default (10 * time.Second).
	PingTimeout time.Duration
}

// Hub is the byte-oriented broadcast hub: a mutex-guarded map of
// subscribers that fans out []byte payloads non-blockingly. The hub
// knows nothing about the registry package — events are opaque byte
// slices supplied by the HTTP handler layer.
type Hub struct {
	logger *slog.Logger
	opts   Options

	mu          sync.Mutex
	subscribers map[*subscriber]struct{}
	closed      bool
}

// New constructs a Hub with the given logger and options. A nil logger
// panics at construction time — the hub must never fall back to a
// package-global default logger (enforced by the "no global default
// logger in internal/" gate from Phase 1).
//
// Zero-valued Options fields are replaced with package defaults.
func New(logger *slog.Logger, opts Options) *Hub {
	if logger == nil {
		panic("wshub.New: logger is required; use slog.New(slog.NewTextHandler(io.Discard, nil)) in tests")
	}
	if opts.MessageBuffer == 0 {
		opts.MessageBuffer = defaultMessageBuffer
	}
	if opts.PingInterval == 0 {
		opts.PingInterval = defaultPingInterval
	}
	if opts.WriteTimeout == 0 {
		opts.WriteTimeout = defaultWriteTimeout
	}
	if opts.PingTimeout == 0 {
		opts.PingTimeout = defaultPingTimeout
	}
	return &Hub{
		logger:      logger,
		opts:        opts,
		subscribers: make(map[*subscriber]struct{}),
	}
}

// addSubscriber registers s in the hub's subscriber set. Called at the
// top of Subscribe; paired with removeSubscriber via defer.
func (h *Hub) addSubscriber(s *subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subscribers[s] = struct{}{}
}

// removeSubscriber removes s from the hub's subscriber set. Idempotent:
// deleting an already-absent key is a no-op. Called via defer at the
// top of Subscribe so silent disconnects cannot leak.
func (h *Hub) removeSubscriber(s *subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subscribers, s)
}

// Publish delivers msg to every active subscriber without blocking.
// Slow subscribers whose outbound buffer is full are kicked via
// closeSlow rather than stalling the publisher.
//
// If the hub is closed, Publish is a silent no-op — it does not log,
// fan out, or panic. This makes it safe for Phase 5's two-phase
// shutdown to race with in-flight HTTP handlers.
//
// The `go` keyword on closeSlow is load-bearing: conn.Close has a
// 5s+5s handshake budget, and we hold h.mu during the loop. Calling
// closeSlow inline would block every other subscriber's enqueue.
func (h *Hub) Publish(msg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	for s := range h.subscribers {
		select {
		case s.msgs <- msg:
			// queued
		default:
			// Slow consumer — log Warn, kick off-mutex via `go`.
			h.logger.Warn("wshub: subscriber dropped (slow consumer)",
				"buffer_size", h.opts.MessageBuffer)
			go s.closeSlow()
		}
	}
}

// Close shuts down the hub, sending a StatusGoingAway close frame to
// every active subscriber. Idempotent: a second call is a no-op.
// Returns no error — graceful shutdown must not report failure.
//
// Close does NOT clear h.subscribers. Each writer loop observes its
// conn close as a write error (or ctx.Done() cancellation from the
// client-side TCP close) and calls `defer h.removeSubscriber(s)`.
// Clearing the map here would race with those defers.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return // idempotent: second call is a no-op
	}
	h.closed = true
	h.logger.Info("wshub: closing hub", "subscribers", len(h.subscribers))
	for s := range h.subscribers {
		go s.closeGoingAway()
	}
}
