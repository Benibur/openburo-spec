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
