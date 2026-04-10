// Package config will load server-operational settings from YAML in
// Plan 01-02. Phase 1 only ships this stub so `go mod tidy` keeps the
// yaml dependency in go.mod; the real Config/Credentials types and
// loader land in Plan 01-02.
package config

// The blank import anchors go.yaml.in/yaml/v3 in go.mod so the pinned
// version is recorded at Phase 1 time. Plan 01-02 replaces this with
// a real typed import when the loader is written.
import (
	_ "go.yaml.in/yaml/v3"
)
