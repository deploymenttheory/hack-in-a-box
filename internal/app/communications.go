package app

import (
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/deploymenttheory/hack-in-a-box/internal/inbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/mailcontact"
	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
	"github.com/deploymenttheory/hack-in-a-box/internal/smtpintegration"
	"github.com/deploymenttheory/hack-in-a-box/internal/teamsworkflow"
)

type communicationRuntime struct {
	Help           help.Service
	Inbox          *inbox.Service
	Communications *communications.Service
	SMTP           *smtpintegration.Service
	Teams          *teamsworkflow.Service
	Jobs           *jobs.Service
	MailContacts   mailcontact.Service
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
	provider.Campaigns = composer
	inboxProvider := &inbox.Service{Pool: pool, Communications: composer}
	composer.Channels["in_app"] = inboxProvider
	helpProvider := help.Service{Pool: pool}
	inboxProvider.Owners = map[string]inbox.ResourceOwner{
		"help": helpInboxOwner{Help: helpProvider},
	}
	composer.FactOwners = map[string]communications.FactOwner{"help": helpProvider}
	helpProvider.Notifications = helpInboxSink{Inbox: inboxProvider}
	outbox, err := jobs.New(
		pool,
		logger,
		map[string]jobs.Handler{
			"communication_test_smtp":     provider,
			"communication_test_in_app":   inbox.TestHandler{Service: *inboxProvider},
			"communication_campaign_smtp": smtpintegration.CampaignHandler{Service: provider},
			teamsworkflow.HandlerKind():   teams,
		},
	)
	if err != nil {
		return communicationRuntime{}, fmt.Errorf("configure communication outbox: %w", err)
	}
	composer.Jobs = outbox
	teams.Jobs = outbox
	teams.Composer = *composer
	return communicationRuntime{
		Communications: composer,
		Inbox:          inboxProvider,
		Help:           helpProvider,
		SMTP:           provider,
		Teams:          teams,
		Jobs:           outbox,
		MailContacts:   mailcontact.Service{Pool: pool, Sender: smtp},
	}, nil
}
