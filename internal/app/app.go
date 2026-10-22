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

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubidentity"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpserver"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/permissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/preparation"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
	"github.com/deploymenttheory/hack-in-a-box/internal/teams"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
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
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer pool.Close()
	if err := database.CheckSchema(ctx, pool); err != nil {
		return fmt.Errorf("check schema: %w", err)
	}
	storage, err := files.OpenLocal(cfg.StoragePath)
	if err != nil {
		return fmt.Errorf("open local: %w", err)
	}
	defer func() { _ = storage.Close() }()
	if err := storage.Ready(ctx); err != nil {
		return fmt.Errorf("ready: %w", err)
	}
	auth, err := identity.New(
		ctx,
		pool,
		identity.Settings{
			Issuer:           cfg.OIDCIssuer,
			ClientID:         cfg.OIDCClientID,
			ClientSecret:     cfg.OIDCClientSecret,
			Origin:           cfg.PublicOrigin,
			BootstrapSubject: cfg.BootstrapSubject,
			Secure:           cfg.Environment == "production",
		},
	)
	if err != nil {
		return fmt.Errorf("new: %w", err)
	}
	github, err := githubidentity.New(
		pool,
		githubidentity.Settings{
			ClientID:     cfg.GitHubClientID,
			ClientSecret: cfg.GitHubClientSecret,
			TokenKey:     cfg.GitHubTokenKey,
			Origin:       cfg.PublicOrigin,
		},
	)
	if err != nil {
		return fmt.Errorf("configure GitHub identity: %w", err)
	}
	queries := dbgen.New(pool)
	policy := access.EventPolicy{Queries: queries}
	deps := httpapi.Dependencies{
		Preparation:  preparation.Service{Pool: pool},
		Teams:        teams.Service{Pool: pool},
		GitHub:       github,
		Pool:         pool,
		Permissions:  permissions.Service{Pool: pool},
		Events:       event.Service{Pool: pool},
		Privacy:      privacy.Service{Pool: pool},
		Templates:    templates.Service{Pool: pool},
		Registration: registration.Service{Pool: pool},
		Auth:         auth,
		Audit:        audit.Service{Queries: queries, Policy: policy},
		Files: files.Service{
			Pool:       pool,
			Storage:    storage,
			Policy:     policy,
			MaxBytes:   cfg.FileMaxBytes,
			EventQuota: cfg.FileEventQuota,
		},
	}
	server := httpserver.New(cfg, httpapi.NewRouter(cfg, logger, deps))
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

func RunWorker(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer pool.Close()
	if err := database.CheckSchema(ctx, pool); err != nil {
		return fmt.Errorf("check schema: %w", err)
	}
	storage, err := files.OpenLocal(cfg.StoragePath)
	if err != nil {
		return fmt.Errorf("open local: %w", err)
	}
	defer func() { _ = storage.Close() }()
	if err := storage.Cleanup(ctx, dbgen.New(pool).ObjectExists); err != nil {
		return fmt.Errorf("cleanup: %w", err)
	}
	worker, err := jobs.New(pool, logger, map[string]jobs.Handler{})
	if err != nil {
		return fmt.Errorf("new: %w", err)
	}
	logger.Info("worker started", "jobs_enabled", true)
	worker.Maintenance = func(ctx context.Context) error {
		if err := (registration.Service{Pool: pool}).MaintainOffers(ctx); err != nil {
			return fmt.Errorf("maintain attendee offers: %w", err)
		}
		if err := dbgen.New(pool).ExpireGitHubLinkAttempts(ctx); err != nil {
			return fmt.Errorf("expire GitHub verification attempts: %w", err)
		}
		if err := (registration.Service{Pool: pool}).Retain(ctx); err != nil {
			return fmt.Errorf("retain registration responses: %w", err)
		}
		if err := (privacy.Service{Pool: pool}).Retain(ctx); err != nil {
			return fmt.Errorf("retain private records: %w", err)
		}
		return storage.Cleanup(ctx, dbgen.New(pool).ObjectExists)
	}
	if err := worker.Run(ctx, cfg.ShutdownTimeout); err != nil {
		return fmt.Errorf("run worker: %w", err)
	}
	return nil
}
