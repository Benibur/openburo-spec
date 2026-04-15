package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/openburo/openburo-server/internal/registry"
	"github.com/stretchr/testify/require"
)

func TestNewRegistryUpdatedEvent_Added(t *testing.T) {
	raw := newRegistryUpdatedEvent("mail-app", changeAdded)
	var evt registryUpdatedEvent
	require.NoError(t, json.Unmarshal(raw, &evt))
	require.Equal(t, "REGISTRY_UPDATED", evt.Event)
	require.Equal(t, "mail-app", evt.Payload.AppID)
	require.Equal(t, changeAdded, evt.Payload.Change)
	// Timestamp parses as RFC3339 with ms
	_, err := time.Parse("2006-01-02T15:04:05.000Z07:00", evt.Timestamp)
	require.NoError(t, err, "timestamp %q should parse as RFC3339 with ms", evt.Timestamp)
}

func TestNewRegistryUpdatedEvent_Updated(t *testing.T) {
	raw := newRegistryUpdatedEvent("mail-app", changeUpdated)
	var evt registryUpdatedEvent
	require.NoError(t, json.Unmarshal(raw, &evt))
	require.Equal(t, changeUpdated, evt.Payload.Change)
}

func TestNewRegistryUpdatedEvent_Removed(t *testing.T) {
	raw := newRegistryUpdatedEvent("mail-app", changeRemoved)
	var evt registryUpdatedEvent
	require.NoError(t, json.Unmarshal(raw, &evt))
	require.Equal(t, changeRemoved, evt.Payload.Change)
}

func TestEventPayload_OmitemptyCapabilities(t *testing.T) {
	// Upsert events must NOT include a `capabilities` field (omitempty).
	raw := string(newRegistryUpdatedEvent("mail-app", changeAdded))
	require.False(t, strings.Contains(raw, `"capabilities"`),
		"upsert event must not include capabilities field: %s", raw)
}

func TestNewSnapshotEvent(t *testing.T) {
	caps := []registry.CapabilityView{
		{
			AppID:   "mail-app",
			AppName: "Mail",
			Action:  "PICK",
			Path:    "/pick",
			Properties: registry.CapabilityProps{
				MimeTypes: []string{"text/plain"},
			},
		},
	}
	raw := newSnapshotEvent(caps)
	var evt registryUpdatedEvent
	require.NoError(t, json.Unmarshal(raw, &evt))
	require.Equal(t, "REGISTRY_UPDATED", evt.Event)
	require.Equal(t, changeSnapshot, evt.Payload.Change)
	require.Len(t, evt.Payload.Capabilities, 1)
	require.Equal(t, "mail-app", evt.Payload.Capabilities[0].AppID)
	// appId omitted on snapshot events
	require.Empty(t, evt.Payload.AppID)
	// Timestamp parses
	_, err := time.Parse("2006-01-02T15:04:05.000Z07:00", evt.Timestamp)
	require.NoError(t, err)
}

func TestNewSnapshotEvent_EmptyList(t *testing.T) {
	raw := string(newSnapshotEvent([]registry.CapabilityView{}))
	// Must NOT be "null" for the capabilities field — clients do
	// `state = event.payload.capabilities` and expect an array.
	require.Contains(t, raw, `"capabilities":[]`)
	require.NotContains(t, raw, `"capabilities":null`)
}
