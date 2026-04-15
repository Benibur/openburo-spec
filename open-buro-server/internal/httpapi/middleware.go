package httpapi

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/rs/cors"
)

// statusCapturingWriter is a tiny wrapper around http.ResponseWriter that
// captures the status code so logMiddleware can log it. Handlers that call
// WriteHeader multiple times are caught by the first call only (same as
// stdlib behavior).
//
// Hijack() is implemented so the WebSocket upgrade path in
// handleCapabilitiesWS can succeed: coder/websocket.Accept calls
// http.Hijacker on the response writer, and middleware wrappers that
// don't forward the interface break the upgrade with a 501 response.
type statusCapturingWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusCapturingWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Hijack forwards to the underlying ResponseWriter if it implements
// http.Hijacker so the WebSocket upgrade can take over the TCP conn.
// Returning a non-nil error when the inner writer doesn't support
// hijacking matches the stdlib convention that coder/websocket checks.
func (w *statusCapturingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("httpapi: underlying ResponseWriter does not support Hijacker")
	}
	// Mark status as 101 so the request log reflects the successful
	// protocol switch instead of the default 200.
	w.status = http.StatusSwitchingProtocols
	return h.Hijack()
}

// recoverMiddleware is the OUTERMOST middleware. It catches panics from
// any inner middleware or handler, logs them (with stack), emits a 500
// envelope, and returns — the server stays alive for the next request.
// API-08 anchor.
func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.logger.Error("httpapi: handler panic",
					"path", r.URL.Path,
					"method", r.Method,
					"panic", fmt.Sprintf("%v", rec),
					"stack", string(debug.Stack()))
				writeInternal(w, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// logMiddleware logs every non-/health request with structured fields.
// /health is skipped explicitly (it's the noisiest route and clutters
// logs — inherited from Phase 1's "never log health" convention).
//
// Deliberately NO `user` field here — that's the audit log's job
// (Plan 04-03 OPS-06) and the request log must not imply user identity
// for public read routes.
func (s *Server) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rw := &statusCapturingWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		s.logger.Info("httpapi: request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote", clientIP(r))
	})
}

// corsMiddleware wraps next with the rs/cors handler, driven by
// s.cfg.AllowedOrigins. The allow-list is SHARED with coder/websocket's
// AcceptOptions.OriginPatterns in handleCapabilitiesWS, so a browser client
// that can call the REST API can also connect to the WebSocket endpoint
// (WS-08 shared allow-list contract).
//
// Construction note: rs/cors v1.11.1 does NOT reject the
// `AllowedOrigins: ["*"] + AllowCredentials: true` combination here —
// Server.New has already rejected that combination at construction time,
// so this path only ever sees a safe allow-list.
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	c := cors.New(cors.Options{
		AllowedOrigins:   s.cfg.AllowedOrigins,
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	})
	return c.Handler(next)
}

// clientIP returns the client's IP for logging. Respects X-Forwarded-For
// (first entry) when present, falls back to r.RemoteAddr. Reference-impl
// only — a hardened prod service would need a trusted-proxy allow-list.
func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if i := strings.Index(fwd, ","); i >= 0 {
			return strings.TrimSpace(fwd[:i])
		}
		return strings.TrimSpace(fwd)
	}
	return r.RemoteAddr
}
