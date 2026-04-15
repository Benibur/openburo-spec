package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/openburo/openburo-server/internal/registry"
)

// handleCapabilities returns the filtered capability list under
// {capabilities:[], count:N}. Public route (no auth).
//
// Query params:
//
//	?action=PICK|SAVE — exact-match filter (empty = no filter)
//	?mimeType=X/Y    — symmetric wildcard MIME match via Store.Capabilities
//
// Malformed mimeType (one that CanonicalizeMIME rejects) returns 400.
// This is the "callers wanting a 400 should pre-validate" path that
// Plan 02-03 documented in the Store.Capabilities contract.
func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	filter := registry.CapabilityFilter{
		Action:   r.URL.Query().Get("action"),
		MimeType: r.URL.Query().Get("mimeType"),
	}

	// Pre-validate mimeType so callers see a 400 on malformed input
	// rather than a silent empty result (which store.Capabilities
	// would otherwise return per its documented lenient contract).
	if filter.MimeType != "" {
		canonical, err := registry.CanonicalizeMIME(filter.MimeType)
		if err != nil {
			writeBadRequest(w, "invalid mime type", map[string]any{
				"mimeType": filter.MimeType,
				"reason":   err.Error(),
			})
			return
		}
		filter.MimeType = canonical
	}

	caps := s.store.Capabilities(filter)
	if caps == nil {
		caps = []registry.CapabilityView{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(struct {
		Capabilities []registry.CapabilityView `json:"capabilities"`
		Count        int                       `json:"count"`
	}{
		Capabilities: caps,
		Count:        len(caps),
	})
}

// buildFullStateSnapshot marshals a snapshot event carrying the full
// unfiltered capability list. Called by handleCapabilitiesWS once per
// new subscriber, BEFORE handoff to hub.Subscribe, so WS-06 ordering
// is guaranteed: snapshot first, then subsequent events.
func (s *Server) buildFullStateSnapshot() []byte {
	// Empty filter = no filter = full list.
	caps := s.store.Capabilities(registry.CapabilityFilter{})
	return newSnapshotEvent(caps)
}

// handleCapabilitiesWS upgrades the request to a WebSocket and hands
// off to the hub after sending the full-state snapshot as the first
// message (WS-06).
//
// OriginPatterns are sourced from s.cfg.AllowedOrigins — the SAME
// allow-list as CORS (WS-08 — shared allow-list). The origin-skip
// knob on AcceptOptions is deliberately omitted (PITFALLS #7 anchor);
// reviewers should grep the source tree for that knob to prove it's
// absent in production code.
//
// Accept writes the handshake response on success OR the 403 rejection
// on origin mismatch — on error, the handler just returns.
func (s *Server) handleCapabilitiesWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: s.cfg.AllowedOrigins,
		// Origin-skip knob deliberately omitted. PITFALLS #7 anchor.
	})
	if err != nil {
		// websocket.Accept wrote the rejection response already
		// (403 on origin mismatch). Just return.
		return
	}

	// WS-06: send the full-state snapshot BEFORE entering hub.Subscribe.
	// This eliminates the connect-then-fetch race: clients that receive
	// the snapshot then observe subsequent events in order.
	snapshot := s.buildFullStateSnapshot()
	writeCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	err = conn.Write(writeCtx, websocket.MessageText, snapshot)
	cancel()
	if err != nil {
		s.logger.Warn("httpapi: snapshot write failed",
			"error", err.Error())
		_ = conn.Close(websocket.StatusInternalError, "snapshot write failed")
		return
	}

	// Hand off to the hub. Subscribe blocks until disconnect, context
	// cancel, hub.Close, or an error on the writer side.
	_ = s.hub.Subscribe(r.Context(), conn)
}
