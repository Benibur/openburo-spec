// Package wshub implements the WebSocket broadcast hub using the
// coder/websocket library. It holds a map of subscribers under a mutex
// and fans out events non-blockingly with drop-slow-consumer semantics.
//
// wshub intentionally knows nothing about the registry package — events
// are opaque byte slices supplied by the handler layer. This inversion
// keeps the dependency graph acyclic and makes the registry-hub ABBA
// deadlock structurally impossible from this side (see .planning/research/
// PITFALLS.md §1).
//
// The hub is the canonical coder/websocket chat-hub pattern, minus the
// rate limiter and plus four deltas: (1) injected *slog.Logger, (2) an
// Options struct with zero-value defaults, (3) a per-subscriber ping
// ticker inside the writer loop, (4) two close callbacks per subscriber
// (closeSlow for slow-consumer kicks, closeGoingAway for hub shutdown).
package wshub
