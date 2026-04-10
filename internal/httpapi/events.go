package httpapi

import (
	"encoding/json"
	"time"

	"github.com/openburo/openburo-server/internal/registry"
)

// changeType is the payload.change discriminant in a REGISTRY_UPDATED event.
// The SNAPSHOT value is pre-declared here (plan 04-04 uses it for the
// initial full-state snapshot sent on WS connect; this plan emits only
// ADDED/UPDATED/REMOVED on the mutation-then-broadcast path).
type changeType string

const (
	changeAdded    changeType = "ADDED"
	changeUpdated  changeType = "UPDATED"
	changeRemoved  changeType = "REMOVED"
	changeSnapshot changeType = "SNAPSHOT"
)

// eventPayload is the nested payload of a REGISTRY_UPDATED event.
// For upsert/delete events: Change is ADDED|UPDATED|REMOVED and AppID is set.
// For snapshot events (plan 04-04): Change is SNAPSHOT, AppID is "", and
// Capabilities carries the full list. omitempty on both variable fields
// so the two shapes cleanly share one struct.
type eventPayload struct {
	Change       changeType                `json:"change"`
	AppID        string                    `json:"appId,omitempty"`
	Capabilities []registry.CapabilityView `json:"capabilities,omitempty"`
}

// registryUpdatedEvent is the single WebSocket event type the broker
// emits. Per FEATURES.md "the broker has one thing to say to everyone"
// there is ONE event name, regardless of whether the change is an add,
// update, delete, or initial snapshot.
type registryUpdatedEvent struct {
	Event     string       `json:"event"`
	Timestamp string       `json:"timestamp"`
	Payload   eventPayload `json:"payload"`
}

// newRegistryUpdatedEvent builds a REGISTRY_UPDATED event for a single
// manifest change (ADDED|UPDATED|REMOVED). The timestamp is UTC RFC 3339
// with millisecond precision.
//
// The json.Marshal error is discarded because the struct is fixed and
// cannot fail to marshal (no channels, no funcs, no unsupported types).
func newRegistryUpdatedEvent(appID string, change changeType) []byte {
	evt := registryUpdatedEvent{
		Event:     "REGISTRY_UPDATED",
		Timestamp: time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		Payload: eventPayload{
			Change: change,
			AppID:  appID,
		},
	}
	b, _ := json.Marshal(evt)
	return b
}

// snapshotPayload is the dedicated payload type for SNAPSHOT events.
// It exists separately from eventPayload because eventPayload tags
// Capabilities with `omitempty` (so upsert/delete events drop the
// field), but snapshot events MUST always emit `"capabilities":[]` —
// never null, never missing — so clients can do
//
//	state = event.payload.capabilities
//
// without a null check. Same wire shape as eventPayload for the
// populated case; different zero-value serialization.
type snapshotPayload struct {
	Change       changeType                `json:"change"`
	Capabilities []registry.CapabilityView `json:"capabilities"`
}

// snapshotEvent is the envelope for a SNAPSHOT REGISTRY_UPDATED event.
// Reuses the top-level Event/Timestamp fields of registryUpdatedEvent
// but carries snapshotPayload so the capabilities field always renders.
type snapshotEvent struct {
	Event     string          `json:"event"`
	Timestamp string          `json:"timestamp"`
	Payload   snapshotPayload `json:"payload"`
}

// newSnapshotEvent builds a REGISTRY_UPDATED event carrying the full
// capability list. This is the initial message sent to every new
// WebSocket subscriber (WS-06) — clients do
//
//	state = event.payload.capabilities
//
// and then refetch the REST endpoint on any subsequent event.
//
// The payload.change is SNAPSHOT, payload.appId is absent entirely
// (the snapshotPayload type has no AppID field), and
// payload.capabilities is ALWAYS a non-nil slice (possibly empty)
// so the consumer can always do `state = event.payload.capabilities`
// without a null check.
func newSnapshotEvent(caps []registry.CapabilityView) []byte {
	if caps == nil {
		caps = []registry.CapabilityView{}
	}
	evt := snapshotEvent{
		Event:     "REGISTRY_UPDATED",
		Timestamp: time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		Payload: snapshotPayload{
			Change:       changeSnapshot,
			Capabilities: caps,
		},
	}
	b, _ := json.Marshal(evt)
	return b
}
