package sessionbooking

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/mailcontact"
)

type notificationRecipient struct {
	PersonID         uuid.UUID `json:"person_id"`
	State            string    `json:"state"`
	Revision         int64     `json:"revision"`
	CalendarSequence int64     `json:"calendar_sequence"`
}

func notificationRecipients(
	ctx context.Context,
	tx pgx.Tx,
	eventID, id uuid.UUID,
) ([]notificationRecipient, error) {
	result := []notificationRecipient{}
	rows, err := tx.Query(
		ctx,
		`SELECT person_id,state,revision,calendar_sequence FROM session_bookings WHERE event_id=$1 AND session_id=$2 ORDER BY person_id LIMIT 10001`,
		eventID,
		id,
	)
	if err != nil {
		return result, contextError("read notification recipients", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item notificationRecipient
		if err = rows.Scan(
			&item.PersonID,
			&item.State,
			&item.Revision,
			&item.CalendarSequence,
		); err != nil {
			return result, contextError("scan notification recipient", err)
		}
		result = append(result, item)
	}
	if err = rows.Err(); err != nil {
		return result, contextError("load notification recipients", err)
	}
	if len(result) > 10000 {
		return nil, access.ErrInvalid
	}
	return result, nil
}

type notificationOccurrence struct {
	ID, EventID, SessionID, PersonID                                         uuid.UUID
	ConfigurationVersion, SessionRevision, BookingRevision, CalendarSequence int64
	State, Binding, SourceState, Reason                                      string
}

// queueUpdates persists source identities before asking existing adapters to
// retain their actual intents. It performs no network send. Immutable channel
// reports say queued intent or unavailable, never delivery or receipt.
func (s Service) queueUpdates(
	ctx context.Context,
	tx pgx.Tx,
	actor uuid.UUID,
	st scope,
	changes map[uuid.UUID]bool,
) error {
	if _, err := tx.Exec(
		ctx,
		`UPDATE session_bookings SET calendar_sequence=$3+revision WHERE event_id=$1 AND session_id=$2 AND state IN ('booked','waitlisted') AND calendar_sequence<>$3+revision`,
		st.row.ID,
		st.item.ID,
		st.sequence,
	); err != nil {
		return contextError("update active personal calendar sequence", err)
	}
	config, err := notificationConfig(ctx, tx, st.row.ID, st.item.ID)
	if err != nil {
		return err
	}
	if config.ID == uuid.Nil && st.row.WritesAllowed {
		return nil
	}
	people, err := notificationRecipients(ctx, tx, st.row.ID, st.item.ID)
	if err != nil {
		return err
	}
	state, reason := "approved", "approved"
	switch {
	case !st.row.WritesAllowed:
		state, reason = "suppressed", "parent_event_readonly"
	case !config.Approved:
		state, reason = "unavailable", "notifications_not_approved"
	default:
		if err = (access.EventPolicy{Queries: dbgen.New(tx)}).Authorize(
			ctx,
			config.Actor,
			st.row.ID,
			st.row.ID,
			access.EventEdit,
		); err != nil {
			if !errors.Is(err, access.ErrForbidden) && !errors.Is(err, access.ErrNotFound) {
				return contextError("check notification approver", err)
			}
			state, reason = "unavailable", "approval_unavailable"
		}
	}
	for _, person := range people {
		if (person.State == "cancelled" || person.State == "ineligible") &&
			!changes[person.PersonID] {
			continue
		}
		occ := notificationOccurrence{
			ID:                   uuid.New(),
			EventID:              st.row.ID,
			SessionID:            st.item.ID,
			PersonID:             person.PersonID,
			ConfigurationVersion: config.Version,
			SessionRevision:      st.revision,
			BookingRevision:      person.Revision,
			CalendarSequence:     person.CalendarSequence,
			State:                person.State,
			SourceState:          state,
			Reason:               reason,
		}
		occ.Binding = hash(
			[]any{
				"session-notification-v1",
				occ.EventID,
				occ.SessionID,
				occ.PersonID,
				config.ID,
				occ.ConfigurationVersion,
				occ.SessionRevision,
				occ.BookingRevision,
				occ.CalendarSequence,
				occ.State,
				occ.SourceState,
				occ.Reason,
			},
		)
		tag, err := tx.Exec(
			ctx,
			`INSERT INTO session_notification_occurrences(id,event_id,session_id,recipient_id,configuration_version,session_revision,booking_revision,booking_state,calendar_sequence,binding,source_state,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT DO NOTHING`,
			occ.ID,
			occ.EventID,
			occ.SessionID,
			occ.PersonID,
			occ.ConfigurationVersion,
			occ.SessionRevision,
			occ.BookingRevision,
			occ.State,
			occ.CalendarSequence,
			occ.Binding,
			occ.SourceState,
			occ.Reason,
		)
		if err != nil {
			return contextError("retain notification occurrence", err)
		}
		if tag.RowsAffected() == 0 || state != "approved" {
			continue
		}
		if err = s.queueChannels(ctx, tx, config, occ); err != nil {
			return err
		}
	}
	return nil
}

func (s Service) queueChannels(
	ctx context.Context,
	tx pgx.Tx,
	config notificationConfiguration,
	occ notificationOccurrence,
) error {
	for _, channel := range config.Channels {
		state, reason, count := "unavailable", "provider_unavailable", 0
		if provider := s.DeliveryChannels[channel]; provider != nil {
			ready, err := provider.Readiness(ctx, occ.EventID, config.Actor)
			if err != nil {
				return contextError("check approved notification provider", err)
			}
			if ready.Available && ready.ConfigRevision == config.Revisions[channel] {
				count, err = provider.QueueApprovedOccurrenceInTransaction(
					ctx,
					tx,
					occ.EventID,
					config.ID,
					config.Version,
					communications.DeliveryScope{OccurrenceID: occ.ID},
				)
				switch {
				case err != nil:
					if !errors.Is(err, access.ErrForbidden) &&
						!errors.Is(err, access.ErrConflict) &&
						!errors.Is(err, access.ErrNotFound) &&
						!errors.Is(err, communications.ErrUnresolved) &&
						!errors.Is(err, mailcontact.ErrUnavailable) {
						return contextError("queue approved session notification", err)
					}
					count = 0
					reason = "source_or_recipient_unavailable"
				case count == 1:
					state, reason = "queued", "intent_queued"
				default:
					reason = "recipient_unavailable"
				}
			}
		}
		if count < 0 || count > 1 {
			return access.ErrConflict
		}
		if _, err := tx.Exec(
			ctx,
			`INSERT INTO session_notification_reports(occurrence_id,channel,state,queued_intents,reason) VALUES($1,$2,$3,$4,$5)`,
			occ.ID,
			channel,
			state,
			count,
			reason,
		); err != nil {
			return contextError("retain actual notification queue report", err)
		}
	}
	return nil
}

func loadOccurrence(
	ctx context.Context,
	tx pgx.Tx,
	eventID, id uuid.UUID,
) (notificationOccurrence, error) {
	var result notificationOccurrence
	err := tx.QueryRow(ctx, `SELECT id,event_id,session_id,recipient_id,configuration_version,session_revision,booking_revision,booking_state,calendar_sequence,binding,source_state,reason FROM session_notification_occurrences WHERE event_id=$1 AND id=$2`, eventID, id).
		Scan(&result.ID, &result.EventID, &result.SessionID, &result.PersonID, &result.ConfigurationVersion, &result.SessionRevision, &result.BookingRevision, &result.State, &result.CalendarSequence, &result.Binding, &result.SourceState, &result.Reason)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, access.ErrNotFound
	}
	return result, contextError("read notification occurrence", err)
}
