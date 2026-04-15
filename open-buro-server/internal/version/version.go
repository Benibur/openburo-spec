// Package version exposes the build-time version string for the OpenBuro
// server. The default value "dev" applies when running via `go run`.
// Release builds inject a real version via ldflags:
//
//	go build -ldflags "-X github.com/openburo/openburo-server/internal/version.Version=$(git describe --tags --always --dirty)" ./cmd/server
package version

// Version is the build-time version string. Overridden via ldflags;
// defaults to "dev" for local `go run` invocations.
var Version = "dev"
