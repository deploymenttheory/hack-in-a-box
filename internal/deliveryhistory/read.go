package deliveryhistory

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
)

func authorize(ctx context.Context, q *dbgen.Queries, actor, eventID uuid.UUID) error {
	if err := (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		access.StaffRead,
	); err != nil {
		return fmt.Errorf("authorize delivery history: %w", err)
	}
	return nil
}

func optionalID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

func reason(channel, value string) string {
	if value == "address_unverified" || value == "recorded_in_app" ||
		value == "awaiting_submission" {
		return value
	}
	if channel == "smtp" {
		return maildelivery.SafeReceipt(maildelivery.Receipt{Code: value}).Code
	}
	if slices.Contains(
		[]string{
			"unsafe_configuration",
			"payload_too_large",
			"dns_unavailable",
			"unsafe_destination",
			"uncertain_response",
			"connection_unavailable",
			"provider_accepted",
			"rate_limit_requires_review",
			"rate_limited",
			"uncertain_acceptance",
			"redirect_rejected",
			"provider_rejected",
			"authority_or_source_changed",
			"source_changed_before_send",
			"encryption_unavailable",
			"retry_limit_reached",
			"process_interrupted",
			"automation_paused",
		},
		value,
	) {
		return value
	}
	return "unknown"
}

func item(r dbgen.CommunicationDeliveryHistoryRow) Item {
	state := r.ProviderState
	if r.Channel == "smtp" && state == "accepted" || state == "provider_accepted" {
		state = "provider_accepted"
	}
	if r.AttemptCount == 0 && state == "queued" {
		state = "never_attempted"
	}
	if r.AttemptCount > 0 && (state == "prepared" || state == "queued") {
		state = "ambiguous"
		if r.OperationState == "running" {
			state = "in_progress"
		}
	}
	return Item{
		ID:              r.ID,
		CampaignID:      optionalID(r.CampaignID),
		CampaignVersion: r.CampaignVersion,
		RecipientID:     optionalID(r.PersonID),
		RecipientName:   r.RecipientName,
		Channel:         r.Channel,
		Purpose:         r.Purpose,
		SelectedAt:      r.SelectedAt,
		State:           state,
		Reason:          reason(r.Channel, r.Reason),
		Attempted:       r.AttemptCount > 0,
		Attempts:        r.AttemptCount,
		Revision:        r.Revision,
		OperationState:  r.OperationState,
		Cancelled:       r.Cancelled,
	}
}

func historyParams(eventID uuid.UUID, f Filter) dbgen.CommunicationDeliveryHistoryParams {
	p := dbgen.CommunicationDeliveryHistoryParams{
		EventID:        eventID,
		CampaignFilter: f.CampaignID,
		PageSize:       f.Limit + 1,
	}
	if f.Channel != "" {
		p.ChannelFilter = pgtype.Text{String: f.Channel, Valid: true}
	}
	if f.Before != nil {
		p.BeforeTime = pgtype.Timestamptz{Time: f.Before.At, Valid: true}
		p.BeforeID = &f.Before.ID
	}
	return p
}

func (s Service) Read(ctx context.Context, actor, eventID uuid.UUID, f Filter) (Page, error) {
	if f.Limit < 1 || f.Limit > 100 ||
		(f.Channel != "" && !slices.Contains([]string{"smtp", "teams", "in_app"}, f.Channel)) ||
		(f.Before != nil && (f.Before.ID == uuid.Nil || f.Before.At.IsZero())) {
		return Page{}, access.ErrInvalid
	}
	p := Page{Items: []Item{}}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		event, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock delivery history: %w", err)
		}
		if err = authorize(ctx, q, actor, eventID); err != nil {
			return err
		}
		p.Archived = !event.WritesAllowed
		p.Paused, err = q.AutomationPaused(ctx, eventID)
		if err != nil {
			return fmt.Errorf("read delivery pause: %w", err)
		}
		rows, err := q.CommunicationDeliveryHistory(ctx, historyParams(eventID, f))
		if err != nil {
			return fmt.Errorf("read delivery history: %w", err)
		}
		if len(rows) > int(f.Limit) {
			last := rows[f.Limit-1]
			p.NextCursor = &Cursor{At: last.SelectedAt, ID: last.ID}
			rows = rows[:f.Limit]
		}
		for _, row := range rows {
			p.Items = append(p.Items, item(row))
		}
		return nil
	})
	if err != nil {
		return Page{}, fmt.Errorf("delivery history: %w", err)
	}
	return p, nil
}

func lookup(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, deliveryID uuid.UUID,
) (dbgen.CommunicationDeliveryHistoryRow, error) {
	rows, err := q.CommunicationDeliveryHistory(
		ctx,
		dbgen.CommunicationDeliveryHistoryParams{
			EventID:        eventID,
			DeliveryFilter: &deliveryID,
			PageSize:       2,
		},
	)
	if err != nil {
		return dbgen.CommunicationDeliveryHistoryRow{}, fmt.Errorf(
			"read delivery identity: %w",
			err,
		)
	}
	if len(rows) != 1 {
		return dbgen.CommunicationDeliveryHistoryRow{}, access.ErrNotFound
	}
	return rows[0], nil
}
