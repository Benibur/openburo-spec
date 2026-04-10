// Package wshub implements the WebSocket broadcast hub using the
// coder/websocket library. It holds a map of subscribers under a mutex
// and fans out events non-blockingly with drop-slow-consumer semantics.
//
// wshub intentionally knows nothing about the registry package — events
// are opaque byte slices supplied by the handler layer. This inversion
// keeps the dependency graph acyclic.
//
// Phase 1 ships this file only; the real implementation lands in Phase 3.
package wshub
