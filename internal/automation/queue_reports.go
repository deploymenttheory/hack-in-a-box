package automation

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
)

func validateReport(report ChannelReport) (ChannelReport, int, error) {
	if report.Preexisting == nil {
		report.Preexisting = map[string]int{}
	}
	if report.Queued < 0 || report.Unavailable < 0 || report.LocalPersisted < 0 {
		return report, 0, communications.ErrUnresolved
	}
	intents := report.Queued + report.LocalPersisted
	for state, count := range report.Preexisting {
		if count < 0 ||
			!slices.Contains(
				[]string{
					"queued",
					"pending",
					"running",
					"prepared",
					"retryable",
					"accepted",
					"delivered",
					"permanent",
					"ambiguous",
					"cancelled",
				},
				state,
			) {
			return report, 0, communications.ErrUnresolved
		}
		intents += count
	}
	if intents+report.Unavailable == 0 || intents+report.Unavailable > 10000 {
		return report, 0, communications.ErrUnresolved
	}
	return report, intents, nil
}

func (s Service) queueChannels(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	version int64,
	scope communications.DeliveryScope,
	channels []string,
) (map[string]ChannelReport, string, string, error) {
	// Validate every selected adapter before any local queue writes. Each provider
	// also resolves current approval/source in this same locked outer transaction.
	for _, channel := range channels {
		if s.Providers[channel] == nil {
			return nil, "", "", communications.ErrUnresolved
		}
	}
	reports := map[string]ChannelReport{}
	totalIntents, totalUnavailable := 0, 0
	for _, channel := range channels {
		report, err := s.Providers[channel](ctx, tx, eventID, campaignID, version, scope)
		if err != nil {
			return nil, "", "", fmt.Errorf("record selected channel intent: %w", err)
		}
		report, count, err := validateReport(report)
		if err != nil {
			return nil, "", "", err
		}
		reports[channel] = report
		totalIntents += count
		totalUnavailable += report.Unavailable
	}
	state, reason := "queued", "provider_intents_recorded"
	if totalUnavailable > 0 {
		if totalIntents > 0 {
			state = "partially_queued"
			reason = "intents_and_unavailable_recipients_recorded"
		} else {
			state = "blocked"
			reason = "unavailable_recipient_evidence_recorded"
		}
	}
	return reports, state, reason, nil
}
