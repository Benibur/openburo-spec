// Package httpapi owns the HTTP routing layer of the OpenBuro server.
// It is the SOLE package where internal/registry and internal/wshub meet:
// registry state lives here behind HTTP handlers, and every mutation
// broadcasts via the hub from the handler layer (not from inside registry).
// This enforces the unidirectional dependency graph that prevents the
// registry<->hub ABBA deadlock (see .planning/research/PITFALLS.md #1).
package httpapi

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"time"

	"github.com/openburo/openburo-server/internal/registry"
	"github.com/openburo/openburo-server/internal/wshub"
)

// Config carries the subset of config.yaml that the handler layer needs.
// The compose-root (Phase 5 cmd/server/main.go) translates config.Config
// into this struct so internal/httpapi does NOT import internal/config.
type Config struct {
	// AllowedOrigins is the CORS + WebSocket OriginPatterns allow-list.
	// Must be non-empty; must not contain "*" because AllowCredentials=true.
	AllowedOrigins []string

	// WSPingInterval is exposed for the compose-root to pass through to
	// wshub.New. The Server itself does not use this field; it is kept
	// on Config so the hub and the cors allow-list come from one struct.
	WSPingInterval time.Duration
}

// Server owns the HTTP routing, middleware chain, and handler implementations.
// Dependencies (store, hub, credentials, logger, config) are injected by the
// compose-root — internal packages NEVER grab global state.
type Server struct {
	logger *slog.Logger
	store  *registry.Store
	hub    *wshub.Hub
	creds  Credentials
	cfg    Config
	mux    *http.ServeMux
}

// New constructs a Server with all domain and transport dependencies.
//
// Returns (*Server, error) — NOT *Server alone — because the Phase 4
// research revealed that rs/cors v1.11.1 does NOT reject the
// `AllowedOrigins: ["*"] + AllowCredentials: true` combination at
// construction time. This constructor performs that validation itself
// and returns a clear error if the operator misconfigures the allow-list.
//
// Nil logger/store/hub panic at construction (programmer error, not
// operator error — there is no recovery path).
func New(logger *slog.Logger, store *registry.Store, hub *wshub.Hub, creds Credentials, cfg Config) (*Server, error) {
	if logger == nil {
		panic("httpapi.New: logger is nil")
	}
	if store == nil {
		panic("httpapi.New: store is nil")
	}
	if hub == nil {
		panic("httpapi.New: hub is nil")
	}
	if len(cfg.AllowedOrigins) == 0 {
		return nil, errors.New("httpapi: cfg.AllowedOrigins is empty (no CORS allow-list; WebSocket handshakes would be rejected)")
	}
	for _, pattern := range cfg.AllowedOrigins {
		if pattern == "*" {
			return nil, errors.New(`httpapi: cfg.AllowedOrigins contains "*" which is incompatible with AllowCredentials=true (PITFALLS #9)`)
		}
		// Probe that the pattern is valid path.Match syntax. coder/websocket
		// uses path.Match for OriginPatterns; rs/cors has no equivalent check
		// but an invalid glob here would silently fail-open on the WS side.
		if _, err := path.Match(pattern, "probe"); err != nil {
			return nil, fmt.Errorf("httpapi: cfg.AllowedOrigins pattern %q: %w", pattern, err)
		}
	}

	s := &Server{
		logger: logger,
		store:  store,
		hub:    hub,
		creds:  creds,
		cfg:    cfg,
		mux:    http.NewServeMux(),
	}
	s.registerRoutes()
	return s, nil
}

// Handler returns the root http.Handler wrapped in the middleware chain.
// Order (outermost first — the request hits the outermost wrapper first):
//
//	recover -> log -> cors -> mux -> (per-route auth) -> handler
//
// Recover is outermost so it catches panics from log, cors, or any inner
// middleware/handler. Per-route auth is attached inside registerRoutes in
// later plans (04-02).
func (s *Server) Handler() http.Handler {
	var h http.Handler = s.mux
	h = s.corsMiddleware(h)    // innermost wrap (closest to mux)
	h = s.logMiddleware(h)     // wraps cors
	h = s.recoverMiddleware(h) // OUTERMOST — catches panics from all inner layers
	return h
}

// stub501 is the Plan 04-01 placeholder handler. Every Phase 4 route that
// is not wired yet (auth, registry handlers, caps, ws) is registered with
// this stub, returning 501 Not Implemented with the envelope shape so
// integration tests exercise the full middleware chain from day one.
// Later plans (04-02..04-05) replace each stub with the real handler.
func (s *Server) stub501(w http.ResponseWriter, r *http.Request) {
	writeJSONError(w, http.StatusNotImplemented, "not implemented", nil)
}

// registerRoutes wires every route Phase 4 is responsible for. Plans
// 04-02..04-04 replace the stub501 calls with real handlers in-place.
// Always use the Go 1.22+ method-prefixed pattern ("GET /health", not
// "/health") so the mux rejects wrong methods with 405 automatically.
func (s *Server) registerRoutes() {
	// Phase 1 route (unchanged)
	s.mux.HandleFunc("GET /health", s.handleHealth)

	// Phase 4 write routes (auth added in plan 04-02; 501 stubs for now)
	s.mux.HandleFunc("POST /api/v1/registry", s.stub501)
	s.mux.HandleFunc("DELETE /api/v1/registry/{appId}", s.stub501)

	// Phase 4 read routes (public)
	s.mux.HandleFunc("GET /api/v1/registry", s.stub501)
	s.mux.HandleFunc("GET /api/v1/registry/{appId}", s.stub501)
	s.mux.HandleFunc("GET /api/v1/capabilities", s.stub501)
	s.mux.HandleFunc("GET /api/v1/capabilities/ws", s.stub501)
}
