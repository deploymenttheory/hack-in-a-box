package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/media"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/registration"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
)

// newWorkerRuntime is the native worker composition shared by the production
// entrypoint and isolated lifecycle fixture. With no clock, every domain owner
// retains its production wall-time default. Auth, provider and queue/lease
// timing are never replaced here.
func newWorkerRuntime(
	pool *pgxpool.Pool,
	cfg config.Config,
	logger *slog.Logger,
	storage *files.Local,
	clocks ...func() time.Time,
) (communicationRuntime, error) {
	var now func() time.Time
	if len(clocks) > 0 {
		now = clocks[0]
	}
	runtime, err := newCommunicationRuntime(pool, cfg, logger, clocks...)
	if err != nil {
		return communicationRuntime{}, err
	}
	worker := runtime.Jobs
	worker.FastMaintenance = func(ctx context.Context) error {
		// Guarded common publication has a bounded release window. Run it
		// before support sweeps so their work or errors cannot delay release.
		if err := (content.Service{Pool: pool, Storage: storage, Clock: now}).Maintain(
			ctx,
		); err != nil {
			return fmt.Errorf("maintain actual content release: %w", err)
		}
		if err := runtime.maintainSupport(ctx); err != nil {
			return fmt.Errorf("maintain native support sources: %w", err)
		}
		if err := runtime.Automation.Maintain(ctx); err != nil {
			return fmt.Errorf("maintain reviewed communication occurrences: %w", err)
		}
		return nil
	}
	worker.Maintenance = func(ctx context.Context) error {
		if err := runtime.RepositoryAccess.Maintain(ctx); err != nil {
			return fmt.Errorf("maintain reviewed repository access: %w", err)
		}
		if err := runtime.Repositories.Maintain(ctx); err != nil {
			return fmt.Errorf("maintain repository intents: %w", err)
		}
		if err := runtime.SubmissionFreeze.Maintain(ctx); err != nil {
			return fmt.Errorf("maintain submission capture: %w", err)
		}
		if err := (submissions.Service{Pool: pool, Clock: now}).Maintain(ctx); err != nil {
			return fmt.Errorf("maintain submission outcomes: %w", err)
		}
		if err := (registration.Service{Pool: pool, Now: now}).MaintainOffers(ctx); err != nil {
			return fmt.Errorf("maintain attendee offers: %w", err)
		}
		if err := dbgen.New(pool).ExpireGitHubAppAttempts(ctx); err != nil {
			return fmt.Errorf("expire GitHub App installation attempts: %w", err)
		}
		if err := dbgen.New(pool).ExpireGitHubLinkAttempts(ctx); err != nil {
			return fmt.Errorf("expire GitHub verification attempts: %w", err)
		}
		if err := (registration.Service{Pool: pool, Now: now}).Retain(ctx); err != nil {
			return fmt.Errorf("retain registration responses: %w", err)
		}
		if err := runtime.SMTP.Maintain(ctx); err != nil {
			return fmt.Errorf("maintain SMTP tests: %w", err)
		}
		if err := runtime.MailContacts.Maintain(ctx); err != nil {
			return fmt.Errorf("retain email confirmations: %w", err)
		}
		if err := (privacy.Service{Pool: pool, Clock: now}).Retain(ctx); err != nil {
			return fmt.Errorf("retain private records: %w", err)
		}
		if err := runtime.Surveys.Retain(ctx); err != nil {
			return fmt.Errorf("retain private survey records: %w", err)
		}
		if err := (media.Service{Pool: pool, Clock: now, Privacy: privacy.Service{Pool: pool, Clock: now}}).Maintain(
			ctx,
		); err != nil {
			return fmt.Errorf("reconcile current media consent: %w", err)
		}
		return storage.Cleanup(ctx, dbgen.New(pool).ObjectExists)
	}
	return runtime, nil
}
