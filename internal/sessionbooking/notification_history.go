package sessionbooking

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

type NotificationReport struct {
	Channel       string `json:"channel"`
	State         string `json:"state"`
	QueuedIntents int    `json:"queued_intents"`
	Reason        string `json:"reason"`
}

type NotificationRecord struct {
	Sequence             int64                `json:"sequence"`
	ID                   uuid.UUID            `json:"id"`
	RecipientID          uuid.UUID            `json:"recipient_id"`
	ConfigurationVersion int64                `json:"configuration_version"`
	SessionRevision      int64                `json:"session_revision"`
	BookingRevision      int64                `json:"booking_revision"`
	CalendarSequence     int64                `json:"calendar_sequence"`
	SourceState          string               `json:"source_state"`
	Reason               string               `json:"reason"`
	Reports              []NotificationReport `json:"reports"`
}

type NotificationHistory struct {
	Items      []NotificationRecord `json:"items"`
	NextCursor *int64               `json:"next_cursor,omitempty"`
}

// These are selected source/queue facts, not provider receipts, delivery success
// or email reading. Original provider attempts remain in their native owner.
func (s Service) NotificationHistory(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	after int64,
	limit int,
) (NotificationHistory, error) {
	result := NotificationHistory{Items: []NotificationRecord{}}
	if after < 0 || limit < 1 || limit > 100 {
		return result, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		st, err := load(ctx, tx, actor, eventID, id)
		if err != nil {
			return err
		}
		if !st.organiser {
			return access.ErrForbidden
		}
		rows, err := tx.Query(
			ctx,
			`SELECT sequence,id,recipient_id,configuration_version,session_revision,booking_revision,calendar_sequence,source_state,reason FROM session_notification_occurrences WHERE event_id=$1 AND session_id=$2 AND sequence>$3 ORDER BY sequence LIMIT $4`,
			eventID,
			id,
			after,
			limit+1,
		)
		if err != nil {
			return contextError("read notification history", err)
		}
		for rows.Next() {
			var record NotificationRecord
			record.Reports = []NotificationReport{}
			if err = rows.Scan(
				&record.Sequence,
				&record.ID,
				&record.RecipientID,
				&record.ConfigurationVersion,
				&record.SessionRevision,
				&record.BookingRevision,
				&record.CalendarSequence,
				&record.SourceState,
				&record.Reason,
			); err != nil {
				rows.Close()
				return contextError("scan notification history", err)
			}
			result.Items = append(result.Items, record)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return contextError("load notification history", err)
		}
		if len(result.Items) > limit {
			last := result.Items[limit-1].Sequence
			result.NextCursor = &last
			result.Items = result.Items[:limit]
		}
		for index := range result.Items {
			reports, err := tx.Query(
				ctx,
				`SELECT channel,state,queued_intents,reason FROM session_notification_reports WHERE occurrence_id=$1 ORDER BY channel`,
				result.Items[index].ID,
			)
			if err != nil {
				return contextError("read queue reports", err)
			}
			for reports.Next() {
				var report NotificationReport
				if err = reports.Scan(
					&report.Channel,
					&report.State,
					&report.QueuedIntents,
					&report.Reason,
				); err != nil {
					reports.Close()
					return contextError("scan queue report", err)
				}
				result.Items[index].Reports = append(result.Items[index].Reports, report)
			}
			err = reports.Err()
			reports.Close()
			if err != nil {
				return contextError("load queue reports", err)
			}
		}
		return nil
	})
	return result, contextError("notification history", err)
}
