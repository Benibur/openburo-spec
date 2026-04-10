// Command openburo-server runs the OpenBuro capability broker.
//
// The compose-root wires config -> logger -> registry store -> websocket
// hub -> httpapi server -> http.Server, then blocks on a signal-aware
// context and performs a two-phase graceful shutdown on SIGINT/SIGTERM:
// first httpSrv.Shutdown drains in-flight HTTP requests, then hub.Close
// sends StatusGoingAway frames to every WebSocket subscriber
// (PITFALLS #6: http.Server.Shutdown does NOT close hijacked conns).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/openburo/openburo-server/internal/config"
	"github.com/openburo/openburo-server/internal/httpapi"
	"github.com/openburo/openburo-server/internal/registry"
	"github.com/openburo/openburo-server/internal/version"
	"github.com/openburo/openburo-server/internal/wshub"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
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
	logger.Info("openburo server starting", "version", version.Version, "go", runtime.Version(), "listen", cfg.Server.Addr(), "tls", cfg.Server.TLS.Enabled, "registry", cfg.RegistryFile)
	store, err := registry.NewStore(cfg.RegistryFile)
	if err != nil {
		return fmt.Errorf("load registry: %w", err)
	}
	creds, err := httpapi.LoadCredentials(cfg.CredentialsFile)
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}
	hub := wshub.New(logger, wshub.Options{PingInterval: cfg.WebSocket.PingInterval})
	srv, err := httpapi.New(logger, store, hub, creds, httpapi.Config{AllowedOrigins: cfg.CORS.AllowedOrigins, WSPingInterval: cfg.WebSocket.PingInterval})
	if err != nil {
		return fmt.Errorf("build httpapi server: %w", err)
	}
	httpSrv := &http.Server{Addr: cfg.Server.Addr(), Handler: srv.Handler()}
	serveErr := make(chan error, 1)
	go func() {
		if cfg.Server.TLS.Enabled {
			serveErr <- httpSrv.ListenAndServeTLS(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile)
			return
		}
		serveErr <- httpSrv.ListenAndServe()
	}()
	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http serve: %w", err)
		}
		return nil
	case <-ctx.Done():
		logger.Info("server shutting down")
	}
	// Phase A: drain in-flight HTTP requests.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	shutdownErr := httpSrv.Shutdown(shutdownCtx)
	// Phase B: close WebSocket subscribers with StatusGoingAway (PITFALLS #6).
	hub.Close()
	if shutdownErr != nil {
		return fmt.Errorf("http shutdown: %w", shutdownErr)
	}
	logger.Info("server stopped cleanly")
	return nil
}

var logLevels = map[string]slog.Level{
	"debug": slog.LevelDebug, "info": slog.LevelInfo,
	"warn": slog.LevelWarn, "error": slog.LevelError,
}

// newLogger builds a *slog.Logger from config (no global-logger fallback).
func newLogger(format, level string) (*slog.Logger, error) {
	lvl, ok := logLevels[level]
	if !ok {
		return nil, fmt.Errorf("invalid log level %q (want debug|info|warn|error)", level)
	}
	opts := &slog.HandlerOptions{Level: lvl}
	switch format {
	case "json":
		return slog.New(slog.NewJSONHandler(os.Stderr, opts)), nil
	case "text":
		return slog.New(slog.NewTextHandler(os.Stderr, opts)), nil
	default:
		return nil, fmt.Errorf("invalid log format %q (want json|text)", format)
	}
}
