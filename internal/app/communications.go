package app

import (
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/mailcontact"
	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
	"github.com/deploymenttheory/hack-in-a-box/internal/smtpintegration"
)

type communicationRuntime struct {
	Communications *communications.Service
	SMTP           *smtpintegration.Service
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
	provider := &smtpintegration.Service{Pool: pool, Transport: smtp, Origin: cfg.PublicOrigin}
	composer := &communications.Service{
		Pool:     pool,
		Channels: map[string]communications.ChannelProvider{"smtp": provider},
	}
	provider.Campaigns = composer
	outbox, err := jobs.New(
		pool,
		logger,
		map[string]jobs.Handler{
			"communication_test_smtp":     provider,
			"communication_campaign_smtp": smtpintegration.CampaignHandler{Service: provider},
		},
	)
	if err != nil {
		return communicationRuntime{}, fmt.Errorf("configure communication outbox: %w", err)
	}
	composer.Jobs = outbox
	return communicationRuntime{
		Communications: composer,
		SMTP:           provider,
		Jobs:           outbox,
		MailContacts:   mailcontact.Service{Pool: pool, Sender: smtp},
	}, nil
}
