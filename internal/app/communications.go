package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/automation"
	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/deliveryhistory"
	"github.com/deploymenttheory/hack-in-a-box/internal/feedback"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubwebhooks"
	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/deploymenttheory/hack-in-a-box/internal/inbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/mailcontact"
	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
	"github.com/deploymenttheory/hack-in-a-box/internal/media"
	"github.com/deploymenttheory/hack-in-a-box/internal/moderation"
	"github.com/deploymenttheory/hack-in-a-box/internal/observations"
	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/repositories"
	"github.com/deploymenttheory/hack-in-a-box/internal/reviewdashboard"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/smtpintegration"
	"github.com/deploymenttheory/hack-in-a-box/internal/surveys"
	"github.com/deploymenttheory/hack-in-a-box/internal/teamsworkflow"
)

type communicationRuntime struct {
	GitHubWebhooks  *githubwebhooks.Service
	Repositories    *repositories.Service
	Outcomes        *outcomes.Service
	Surveys         *surveys.Service
	ReviewDashboard *reviewdashboard.Service
	Moderation      *moderation.Service
	Scorecards      *scorecards.Service
	Feedback        *feedback.Service
	Deliveries      *deliveryhistory.Service
	Help            help.Service
	Clarifications  *clarification.Service
	Automation      *automation.Service
	Inbox           *inbox.Service
	Communications  *communications.Service
	SMTP            *smtpintegration.Service
	Teams           *teamsworkflow.Service
	Jobs            *jobs.Service
	MailContacts    mailcontact.Service
}

// newCommunicationRuntime supplies the same provider/readiness/handler graph to
// the API producer and worker consumer. Construction never sends a message.
func newCommunicationRuntime(
	pool *pgxpool.Pool,
	cfg config.Config,
	logger *slog.Logger,
) (communicationRuntime, error) {
	smtp, err := maildelivery.NewSMTP(cfg.SMTP)
	if err != nil {
		return communicationRuntime{}, fmt.Errorf("configure SMTP: %w", err)
	}
	provider := &smtpintegration.Service{Pool: pool, Transport: smtp, Origin: cfg.WorkspaceOrigin}
	teams := &teamsworkflow.Service{Pool: pool, Keys: cfg.CredentialKeys}
	composer := &communications.Service{
		Pool:     pool,
		Channels: map[string]communications.ChannelProvider{"smtp": provider, "teams": teams},
	}
	auto := &automation.Service{Pool: pool, Composer: composer}
	local := &inbox.Service{Pool: pool, Communications: composer, ScopePolicy: auto}
	composer.Channels["in_app"] = local
	provider.ScopePolicy = auto
	teams.ScopePolicy = auto
	provider.Campaigns = composer
	answers := &clarification.Service{Pool: pool}
	answers.Notifications = clarificationInboxSink{Inbox: local}
	helpProvider := help.Service{Pool: pool}
	shared := &feedback.Service{Pool: pool, Sources: map[string]feedback.SourcePolicy{
		"observation": observationFeedbackSource{Service: observations.Service{Pool: pool}},
		"question":    questionFeedbackSource{Service: answers},
		"help":        helpFeedbackSource{Service: helpProvider},
	}}
	shared.Notifications = feedbackInboxSink{Inbox: local}
	cards := &scorecards.Service{Pool: pool}
	huddles := &moderation.Service{
		Pool:    pool,
		Cards:   cards,
		Actions: moderation.FeedbackActions{Service: shared},
	}
	cards.Moderation = huddles
	questionnaires := &surveys.Service{Pool: pool}
	followThrough := &outcomes.Service{
		Media: outcomeMediaReferences{
			Media: media.Service{Pool: pool, Privacy: privacy.Service{Pool: pool}},
		},
		Pool:            pool,
		Composer:        composer,
		Automation:      auto,
		WorkspaceOrigin: cfg.WorkspaceOrigin,
	}
	composer.OutcomePlans = followThrough
	local.Owners = map[string]inbox.ResourceOwner{
		"outcome_review":    outcomeInboxOwner{Service: followThrough},
		"survey":            surveyInboxOwner{Surveys: questionnaires},
		"feedback":          feedbackInboxOwner{Service: shared},
		"feedback_followup": feedbackInboxOwner{Service: shared, Followup: true},
		"help":              helpInboxOwner{Help: helpProvider},
		"clarification":     clarificationInboxOwner{Answers: answers},
	}
	composer.FactOwners = map[string]communications.FactOwner{
		"help":           helpProvider,
		"outcome_review": followThrough,
	}
	helpProvider.Notifications = helpInboxSink{Inbox: local}
	auto.Sources = map[string]automation.SourcePolicy{
		"outcome.follow_up":       outcomes.ReviewSources{Service: followThrough},
		"survey.open":             automation.SurveySource{Surveys: questionnaires},
		"help.updated":            automation.HelpSource{Help: helpProvider},
		"clarification.published": automation.ClarificationSource{Answers: answers},
		"rubric.published":        automation.RubricSource{Rubric: rubric.Service{Pool: pool}},
	}
	auto.SourceMaintenance = helpProvider.MaintainInTransaction
	webhooks := &githubwebhooks.Service{Pool: pool, Keys: cfg.CredentialKeys}
	repos := &repositories.Service{
		Pool: pool,
		App: githubapp.Service{
			Pool:     pool,
			Keys:     cfg.CredentialKeys,
			Provider: githubapp.NewProvider(),
		},
	}
	outbox, err := jobs.New(
		pool,
		logger,
		map[string]jobs.Handler{
			githubwebhooks.HandlerKind:    webhooks,
			repositories.HandlerKind:      repos,
			automation.HandlerKind:        auto,
			"communication_test_in_app":   inbox.TestHandler{Service: *local},
			"communication_test_smtp":     provider,
			"communication_campaign_smtp": smtpintegration.CampaignHandler{Service: provider},
			teamsworkflow.HandlerKind():   teams,
		},
	)
	if err != nil {
		return communicationRuntime{}, fmt.Errorf("configure communication outbox: %w", err)
	}
	webhooks.Jobs = outbox
	composer.Jobs = outbox
	auto.Jobs = outbox
	auto.Providers = map[string]automation.ProviderQueue{
		"smtp": func(ctx context.Context, tx pgx.Tx, eventID, campaignID uuid.UUID, version int64, scope communications.DeliveryScope) (automation.ChannelReport, error) {
			report, queueErr := provider.QueueApprovedOccurrenceReportInTransaction(
				ctx,
				tx,
				eventID,
				campaignID,
				version,
				scope,
			)
			if queueErr != nil {
				return automation.ChannelReport{}, fmt.Errorf(
					"record automation provider intent: %w",
					queueErr,
				)
			}
			return automation.ChannelReport{
				Queued:      report.Queued,
				Unavailable: report.Unavailable,
				Preexisting: report.Preexisting,
			}, nil
		},
		"in_app": func(ctx context.Context, tx pgx.Tx, eventID, campaignID uuid.UUID, version int64, scope communications.DeliveryScope) (automation.ChannelReport, error) {
			count, queueErr := local.QueueApprovedOccurrenceInTransaction(
				ctx,
				tx,
				eventID,
				campaignID,
				version,
				scope,
			)
			if queueErr != nil {
				return automation.ChannelReport{}, fmt.Errorf(
					"record automation provider intent: %w",
					queueErr,
				)
			}
			return automation.ChannelReport{LocalPersisted: count}, nil
		},
		"teams": func(ctx context.Context, tx pgx.Tx, eventID, campaignID uuid.UUID, version int64, scope communications.DeliveryScope) (automation.ChannelReport, error) {
			report, queueErr := teams.QueueApprovedOccurrenceReportInTransaction(
				ctx,
				tx,
				eventID,
				campaignID,
				version,
				scope,
			)
			if queueErr != nil {
				return automation.ChannelReport{}, fmt.Errorf(
					"record automation provider intent: %w",
					queueErr,
				)
			}
			return automation.ChannelReport{
				Queued:      report.Queued,
				Preexisting: report.Preexisting,
			}, nil
		},
	}
	auto.ReviewGuards = map[string]automation.ReviewGuard{
		"smtp":  provider.CheckOccurrenceReviewInTransaction,
		"teams": teams.CheckOccurrenceReviewInTransaction,
		// Local inbox persistence completes inside the original outer transaction.
		"in_app": func(context.Context, pgx.Tx, uuid.UUID, uuid.UUID, int64, communications.DeliveryScope) error {
			return nil
		},
	}
	teams.Jobs = outbox
	teams.Composer = *composer
	return communicationRuntime{
		GitHubWebhooks: webhooks,
		Outcomes:       followThrough,
		Surveys:        questionnaires,
		Repositories:   repos,
		ReviewDashboard: &reviewdashboard.Service{
			Pool:       pool,
			Help:       helpProvider,
			Feedback:   shared,
			Cards:      cards,
			Moderation: huddles,
		},
		Moderation: huddles,
		Scorecards: cards,
		Feedback:   shared,
		Deliveries: &deliveryhistory.Service{
			Pool: pool,
			Jobs: outbox,
			Providers: map[string]deliveryhistory.RecoveryProvider{
				"smtp":  provider,
				"teams": teams,
			},
		},
		Clarifications: answers,
		Automation:     auto,
		Inbox:          local,
		Communications: composer,
		Help:           helpProvider,
		SMTP:           provider,
		Teams:          teams,
		Jobs:           outbox,
		MailContacts:   mailcontact.Service{Pool: pool, Sender: smtp},
	}, nil
}
