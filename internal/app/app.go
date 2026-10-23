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
	"github.com/deploymenttheory/hack-in-a-box/internal/attendance"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/credentials"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubidentity"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpserver"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/media"
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
	"github.com/deploymenttheory/hack-in-a-box/internal/objectives"
	"github.com/deploymenttheory/hack-in-a-box/internal/observations"
	"github.com/deploymenttheory/hack-in-a-box/internal/permissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/preparation"
	"github.com/deploymenttheory/hack-in-a-box/internal/presentation"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
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
			CookiePrefix:     "hackbox_" + cfg.Surface,
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
	communicationRuntime, err := newCommunicationRuntime(pool, cfg, logger)
	if err != nil {
		return err
	}
	communicationRuntime.Clarifications.Storage = storage
	queries := dbgen.New(pool)
	policy := access.EventPolicy{Queries: queries}
	deps := httpapi.Dependencies{
		Repositories: *communicationRuntime.Repositories,
		Media: media.Service{
			Pool:    pool,
			Privacy: privacy.Service{Pool: pool},
			Files: files.Service{
				Pool:       pool,
				Storage:    storage,
				MaxBytes:   cfg.FileMaxBytes,
				EventQuota: cfg.FileEventQuota,
			},
		},
		GitHubApp: githubapp.Service{
			Pool:     pool,
			Keys:     cfg.CredentialKeys,
			Provider: githubapp.NewProvider(),
		},
		Outcomes:        communicationRuntime.Outcomes,
		Surveys:         communicationRuntime.Surveys,
		Deliveries:      communicationRuntime.Deliveries,
		Help:            communicationRuntime.Help,
		Feedback:        *communicationRuntime.Feedback,
		Clarifications:  *communicationRuntime.Clarifications,
		Presentation:    presentation.Service{Pool: pool},
		Automation:      communicationRuntime.Automation,
		Inbox:           *communicationRuntime.Inbox,
		Communications:  *communicationRuntime.Communications,
		MailContacts:    communicationRuntime.MailContacts,
		SMTP:            communicationRuntime.SMTP,
		TeamsWorkflow:   communicationRuntime.Teams,
		Credentials:     credentials.Service{Pool: pool, Keys: cfg.CredentialKeys},
		Mentors:         mentors.Service{Pool: pool},
		Rubrics:         rubric.Service{Pool: pool},
		Objectives:      objectives.Service{Pool: pool},
		Observations:    observations.Service{Pool: pool},
		Scorecards:      *communicationRuntime.Scorecards,
		ReviewDashboard: communicationRuntime.ReviewDashboard,
		Moderation:      communicationRuntime.Moderation,
		Content:         content.Service{Pool: pool, Storage: storage},
		Submissions:     submissions.Service{Pool: pool},
		Attendance:      attendance.Service{Pool: pool},
		Preparation:     preparation.Service{Pool: pool},
		Teams:           teams.Service{Pool: pool},
		GitHub:          github,
		Pool:            pool,
		Permissions:     permissions.Service{Pool: pool},
		Events: event.Service{
			Pool:              pool,
			SettleSubmissions: (submissions.Service{Pool: pool}).Settle,
		},
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
	communicationRuntime, err := newCommunicationRuntime(pool, cfg, logger)
	if err != nil {
		return err
	}
	worker := communicationRuntime.Jobs
	logger.Info("worker started", "jobs_enabled", true)
	worker.FastMaintenance = func(ctx context.Context) error {
		// Guarded common publication has a bounded release window. Run it
		// before support sweeps so their work or errors cannot delay release.
		if err := (content.Service{Pool: pool, Storage: storage}).Maintain(ctx); err != nil {
			return fmt.Errorf("maintain actual content release: %w", err)
		}
		if err := communicationRuntime.maintainSupport(ctx); err != nil {
			return fmt.Errorf("maintain native support sources: %w", err)
		}
		if err := communicationRuntime.Automation.Maintain(ctx); err != nil {
			return fmt.Errorf("maintain reviewed communication occurrences: %w", err)
		}
		return nil
	}
	worker.Maintenance = func(ctx context.Context) error {
		if err := communicationRuntime.Repositories.Maintain(ctx); err != nil {
			return fmt.Errorf("maintain repository intents: %w", err)
		}
		if err := (submissions.Service{Pool: pool}).Maintain(ctx); err != nil {
			return fmt.Errorf("maintain submission outcomes: %w", err)
		}
		if err := (registration.Service{Pool: pool}).MaintainOffers(ctx); err != nil {
			return fmt.Errorf("maintain attendee offers: %w", err)
		}
		if err := dbgen.New(pool).ExpireGitHubAppAttempts(ctx); err != nil {
			return fmt.Errorf("expire GitHub App installation attempts: %w", err)
		}
		if err := dbgen.New(pool).ExpireGitHubLinkAttempts(ctx); err != nil {
			return fmt.Errorf("expire GitHub verification attempts: %w", err)
		}
		if err := (registration.Service{Pool: pool}).Retain(ctx); err != nil {
			return fmt.Errorf("retain registration responses: %w", err)
		}
		if err := communicationRuntime.SMTP.Maintain(ctx); err != nil {
			return fmt.Errorf("maintain SMTP tests: %w", err)
		}
		if err := communicationRuntime.MailContacts.Maintain(ctx); err != nil {
			return fmt.Errorf("retain email confirmations: %w", err)
		}
		if err := (privacy.Service{Pool: pool}).Retain(ctx); err != nil {
			return fmt.Errorf("retain private records: %w", err)
		}
		if err := communicationRuntime.Surveys.Retain(ctx); err != nil {
			return fmt.Errorf("retain private survey records: %w", err)
		}
		if err := (media.Service{Pool: pool, Privacy: privacy.Service{Pool: pool}}).Maintain(
			ctx,
		); err != nil {
			return fmt.Errorf("reconcile current media consent: %w", err)
		}
		return storage.Cleanup(ctx, dbgen.New(pool).ObjectExists)
	}
	if err := worker.Run(ctx, cfg.ShutdownTimeout); err != nil {
		return fmt.Errorf("run worker: %w", err)
	}
	return nil
}
