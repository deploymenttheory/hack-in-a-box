// Package app is the composition root. It wires infrastructure and entrypoints;
// domain packages must not import it.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpserver"
)

func Main(component string, run func(context.Context, config.Config, *slog.Logger) error) int {
	cfg, err := config.Load(os.Getenv)
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel})).
		With("component", component)
	if err != nil {
		logger.Error("startup failed", "error", err.Error())
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg, logger); err != nil {
		logger.Error("process failed", "error", err.Error())
		return 1
	}
	return 0
}

func RunAPI(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	server := httpserver.New(cfg, httpapi.NewRouter(cfg, logger))
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen for HTTP: %w", err)
	}
	logger.Info(
		"HTTP server started",
		"address",
		listener.Addr().String(),
		"environment",
		cfg.Environment,
	)
	if err := httpserver.Serve(ctx, server, listener, cfg.ShutdownTimeout, logger); err != nil {
		return fmt.Errorf("run API: %w", err)
	}
	return nil
}

func RunWorker(ctx context.Context, _ config.Config, logger *slog.Logger) error {
	// River execution and durable shutdown are added in #27. This entrypoint
	// deliberately reports that it does not yet consume background jobs.
	logger.Info("worker entrypoint started", "jobs_enabled", false)
	<-ctx.Done()
	logger.Info("worker shutdown complete")
	return nil
}
