package httpapi

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

// statusCapturingWriter is a tiny wrapper around http.ResponseWriter that
// captures the status code so logMiddleware can log it. Handlers that call
// WriteHeader multiple times are caught by the first call only (same as
// stdlib behavior).
type statusCapturingWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusCapturingWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
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

// corsMiddleware is a Plan 04-01 PLACEHOLDER pass-through. Plan 04-05
// replaces this with the real rs/cors wrap driven by s.cfg.AllowedOrigins.
// It is declared here so the middleware chain order is locked from day one.
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return next
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
