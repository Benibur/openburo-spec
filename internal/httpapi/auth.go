package httpapi

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

// ctxKey is the unexported context-key type used to stash the
// authenticated username on the request context. Using an unexported
// type prevents collisions with context keys from other packages
// (Go convention).
type ctxKey int

const (
	ctxKeyUser ctxKey = iota
)

// usernameFromContext extracts the authenticated username previously
// stashed by authBasic. Returns ("", false) if the context has no
// user (i.e. the request was on a public route or auth was not run).
func usernameFromContext(ctx context.Context) (string, bool) {
	u, ok := ctx.Value(ctxKeyUser).(string)
	return u, ok
}

// dummyHash is the precomputed bcrypt hash of a known-nonsense value.
// Used in authBasic's "user not found" path so
// bcrypt.CompareHashAndPassword always runs, making the unauthenticated
// path and the wrong-password path indistinguishable by wall-clock time.
//
// Cost 12 matches the minimum enforced for real credentials (AUTH-01).
// The password string ("openburo:dummy:do-not-match") is 27 bytes,
// safely under bcrypt's 72-byte limit.
var dummyHash []byte

func init() {
	h, err := bcrypt.GenerateFromPassword([]byte("openburo:dummy:do-not-match"), 12)
	if err != nil {
		panic(fmt.Sprintf("httpapi: failed to generate dummy hash: %v", err))
	}
	dummyHash = h
}

// authBasic returns a middleware that enforces HTTP Basic Auth using the
// Server's credential table. On failure, writes 401 + WWW-Authenticate
// and returns without invoking next.
//
// TIMING-SAFETY CONTRACT (AUTH-04, PITFALLS #8):
//   - bcrypt.CompareHashAndPassword runs UNCONDITIONALLY for every request
//   - Unknown users use dummyHash so the bcrypt CPU cost is identical to
//     the wrong-password path
//   - The final authorized decision uses subtle.ConstantTimeCompare on
//     the byte tuple {found, bcryptMatches} — NOT a short-circuit
//     `if found && matches`
//
// PII-SAFETY CONTRACT (AUTH-05, TEST-06):
//   - The Warn log line on failure logs ONLY path, method, remote
//   - NEVER logs username, password, Authorization header, or bcrypt hash
//   - The audit log (plan 04-03) runs AFTER this middleware on success
//     and emits user=<username>, action=<upsert|delete>, appId=<id>
//     with no password material
func (s *Server) authBasic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok {
			writeUnauthorized(w)
			return
		}

		// Look up the user; if not found, substitute dummyHash so bcrypt
		// ALWAYS runs. Do NOT early-return here.
		storedHash, found := s.creds.Lookup(username)
		if !found {
			storedHash = dummyHash
		}

		// bcrypt runs unconditionally. On the unknown-user path,
		// storedHash == dummyHash, so the CPU cost is identical to
		// the wrong-password path.
		bcryptErr := bcrypt.CompareHashAndPassword(storedHash, []byte(password))
		bcryptMatches := bcryptErr == nil

		// Constant-time combination of (found, matches). A short-circuit
		// `if found && bcryptMatches { ... } else { 401 }` would be
		// timing-equivalent here because bcrypt already ran, but using
		// subtle.ConstantTimeCompare makes the safety property explicit
		// to reviewers and future maintainers.
		var foundByte, matchByte byte
		if found {
			foundByte = 1
		}
		if bcryptMatches {
			matchByte = 1
		}
		if subtle.ConstantTimeCompare([]byte{foundByte, matchByte}, []byte{1, 1}) != 1 {
			// AUTH-05: log ONLY path/method/remote. Never username,
			// never password, never Authorization header.
			s.logger.Warn("httpapi: basic auth failed",
				"path", r.URL.Path,
				"method", r.Method,
				"remote", clientIP(r))
			writeUnauthorized(w)
			return
		}

		// Success — stash the authenticated username in ctx so downstream
		// audit logging (plan 04-03) can emit the `user` field.
		ctx := context.WithValue(r.Context(), ctxKeyUser, username)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
