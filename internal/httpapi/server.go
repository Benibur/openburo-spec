// Package httpapi owns the HTTP routing layer of the OpenBuro server.
// Phase 1 ships a minimal Server with only /health wired; subsequent
// phases extend it with the registry store, websocket hub, credentials,
// middleware chain, CORS, and the full /api/v1/* route set.
package httpapi

import (
	"log/slog"
	"net/http"
)

// Server owns the HTTP routing and handler implementations for the
// OpenBuro broker. Phase 1 ships a minimal version with /health only;
// subsequent phases will add store, hub, creds fields alongside the
// existing logger and mux.
type Server struct {
	logger *slog.Logger
	mux    *http.ServeMux
}

// New constructs a Server with the given dependencies and registers its routes.
// The *slog.Logger must be constructed by the caller (compose-root) and
// injected here. No internal/ package is permitted to grab a global logger;
// every component receives its *slog.Logger through its constructor.
func New(logger *slog.Logger) *Server {
	s := &Server{
		logger: logger,
		mux:    http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

// Handler returns the root http.Handler. Phase 1 returns the raw mux;
// Phase 4 will wrap this in the middleware chain (recover -> log -> CORS -> auth).
func (s *Server) Handler() http.Handler {
	return s.mux
}

// registerRoutes wires Phase 1's single route. Future phases add more
// routes here without touching main.go. Always use the Go 1.22+
// method-prefixed pattern ("GET /health", not "/health") so the mux
// rejects wrong methods with 405 instead of silently matching them.
func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /health", s.handleHealth)
}
