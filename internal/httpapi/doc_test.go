// Phase 1 anchor for github.com/stretchr/testify/require so go mod tidy
// keeps it pinned in go.mod. Plan 01-03 replaces this with real tests
// for the /health handler.
package httpapi

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPhase1Anchor(t *testing.T) {
	require.True(t, true, "phase 1 anchor test")
}
