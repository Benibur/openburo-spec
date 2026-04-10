// Command openburo-server runs the OpenBuro capability broker.
//
// Phase 1: loads config.yaml, constructs an injected slog logger, emits
// a structured startup banner, and serves GET /health. Phase 5 will add
// signal-aware graceful shutdown and two-phase WebSocket close.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/openburo/openburo-server/internal/config"
	"github.com/openburo/openburo-server/internal/httpapi"
	"github.com/openburo/openburo-server/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "./config.yaml", "path to config.yaml")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger, err := newLogger(cfg.Logging.Format, cfg.Logging.Level)
	if err != nil {
		return fmt.Errorf("build logger: %w", err)
	}

	logger.Info("openburo server starting",
		"version", version.Version,
		"go_version", runtime.Version(),
		"listen_addr", cfg.Server.Addr(),
		"tls_enabled", cfg.Server.TLS.Enabled,
		"config_file", *configPath,
		"credentials_file", cfg.CredentialsFile,
		"registry_file", cfg.RegistryFile,
		"ping_interval", cfg.WebSocket.PingInterval.String(),
		"log_format", cfg.Logging.Format,
		"log_level", cfg.Logging.Level,
	)

	srv := httpapi.New(logger)
	httpSrv := &http.Server{
		Addr:    cfg.Server.Addr(),
		Handler: srv.Handler(),
	}
	return httpSrv.ListenAndServe()
}

// newLogger builds a *slog.Logger from config.
//
// Lives inline in main.go (not in an internal/logging package) because
// it's compose-root wiring, and because keeping it here guarantees no
// internal/ package ever grabs a global logger behind the compose
// root's back. See .planning/phases/01-foundation/01-RESEARCH.md
// §Pattern 2.
func newLogger(format, level string) (*slog.Logger, error) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		return nil, fmt.Errorf("invalid log level %q (want debug|info|warn|error)", level)
	}
	opts := &slog.HandlerOptions{Level: lvl}
	var h slog.Handler
	switch strings.ToLower(format) {
	case "json":
		h = slog.NewJSONHandler(os.Stderr, opts)
	case "text":
		h = slog.NewTextHandler(os.Stderr, opts)
	default:
		return nil, fmt.Errorf("invalid log format %q (want json|text)", format)
	}
	return slog.New(h), nil
}
