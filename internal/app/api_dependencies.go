package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/archives"
	"github.com/deploymenttheory/hack-in-a-box/internal/attendance"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/credentials"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubidentity"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi"
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

// newAPIDependencies is the single native API composition used by RunAPI.
// Infrastructure ownership, schema checks and shutdown stay with the caller.
func newAPIDependencies(
	ctx context.Context,
	pool *pgxpool.Pool,
	cfg config.Config,
	logger *slog.Logger,
	storage files.Storage,
	auth *identity.Auth,
	clocks ...func() time.Time,
) (httpapi.Dependencies, error) {
	var now func() time.Time
	if len(clocks) > 0 {
		now = clocks[0]
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
		return httpapi.Dependencies{}, fmt.Errorf("configure GitHub identity: %w", err)
	}
	communicationRuntime, err := newCommunicationRuntime(pool, cfg, logger, clocks...)
	if err != nil {
		return httpapi.Dependencies{}, err
	}
	communicationRuntime.Clarifications.Storage = storage
	queries := dbgen.New(pool)
	policy := access.EventPolicy{Queries: queries, Clock: now}
	deps := httpapi.Dependencies{
		Repositories:     *communicationRuntime.Repositories,
		RepositoryAccess: *communicationRuntime.RepositoryAccess,
		Media: media.Service{
			Clock:   now,
			Pool:    pool,
			Privacy: privacy.Service{Pool: pool, Clock: now},
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
		GitHubWebhooks:  communicationRuntime.GitHubWebhooks,
		Outcomes:        communicationRuntime.Outcomes,
		Surveys:         communicationRuntime.Surveys,
		Deliveries:      communicationRuntime.Deliveries,
		Help:            communicationRuntime.Help,
		Feedback:        *communicationRuntime.Feedback,
		Clarifications:  *communicationRuntime.Clarifications,
		Presentation:    presentation.Service{Pool: pool, Clock: now},
		Automation:      communicationRuntime.Automation,
		Inbox:           *communicationRuntime.Inbox,
		Communications:  *communicationRuntime.Communications,
		MailContacts:    communicationRuntime.MailContacts,
		SMTP:            communicationRuntime.SMTP,
		TeamsWorkflow:   communicationRuntime.Teams,
		Credentials:     credentials.Service{Pool: pool, Keys: cfg.CredentialKeys, Now: now},
		Mentors:         mentors.Service{Pool: pool},
		Rubrics:         rubric.Service{Pool: pool, Clock: now},
		Objectives:      objectives.Service{Pool: pool, Clock: now},
		Observations:    observations.Service{Pool: pool, Now: now},
		Scorecards:      *communicationRuntime.Scorecards,
		ReviewDashboard: communicationRuntime.ReviewDashboard,
		Moderation:      communicationRuntime.Moderation,
		Content:         content.Service{Pool: pool, Storage: storage, Clock: now},
		Submissions:     submissions.Service{Pool: pool, Clock: now},
		Archives:        archives.Service{Pool: pool},
		Attendance:      attendance.Service{Pool: pool, Now: now},
		SessionBookings: *communicationRuntime.SessionBookings,
		Preparation:     preparation.Service{Pool: pool, Now: now},
		Teams:           teams.Service{Pool: pool, Now: now},
		GitHub:          github,
		Pool:            pool,
		Permissions:     permissions.Service{Pool: pool},
		Events: event.Service{
			Clock:             now,
			Pool:              pool,
			SettleSubmissions: (submissions.Service{Pool: pool, Clock: now}).Settle,
			ReconcileSessions: communicationRuntime.SessionBookings.ReconcileEventInTransaction,
		},
		Privacy:   privacy.Service{Pool: pool, Clock: now},
		Templates: templates.Service{Pool: pool},
		Registration: registration.Service{
			Pool:              pool,
			Now:               now,
			ReconcileSessions: communicationRuntime.SessionBookings.ReconcileEventInTransaction,
		},
		Auth:  auth,
		Audit: audit.Service{Queries: queries, Policy: policy},
		Files: files.Service{
			Pool:       pool,
			Storage:    storage,
			Policy:     policy,
			MaxBytes:   cfg.FileMaxBytes,
			EventQuota: cfg.FileEventQuota,
		},
	}
	return deps, nil
}
