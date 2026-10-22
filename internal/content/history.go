package content

import (
	"context"
	"fmt"
	"math"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func (s Service) History(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	before int64,
	limit int32,
) (History, error) {
	result := History{Items: []Change{}}
	if before < 0 || limit < 1 || limit > 100 {
		return result, access.ErrInvalid
	}
	if before == 0 {
		before = math.MaxInt64
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(ctx, eventID); err != nil {
			return fmt.Errorf("lock content history: %w", err)
		}
		if err := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.StaffRead,
		); err != nil {
			return fmt.Errorf("authorize content history: %w", err)
		}
		if _, err := load(ctx, q, eventID, id); err != nil {
			return err
		}
		rows, err := q.ContentChanges(
			ctx,
			dbgen.ContentChangesParams{
				EventID:   eventID,
				ContentID: id,
				Before:    before,
				PageSize:  limit + 1,
			},
		)
		if err != nil {
			return fmt.Errorf("read content changes: %w", err)
		}
		if len(rows) > int(limit) {
			rows = rows[:limit]
			v := rows[len(rows)-1].Revision
			result.NextCursor = &v
		}
		for _, r := range rows {
			result.Items = append(
				result.Items,
				Change{
					Revision:    r.Revision,
					Version:     r.Version,
					Action:      r.Action,
					ActorID:     r.ActorID,
					Reason:      r.Reason,
					ScheduledAt: timePointer(r.ScheduledAt),
					At:          r.OccurredAt,
				},
			)
		}
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("read content history: %w", err)
	}
	return result, nil
}

// Notices exposes the persisted revised-date announcement to every current
// attendee and staff member. Provider notification delivery remains owned by #82.
func (s Service) Notices(
	ctx context.Context,
	actor, eventID uuid.UUID,
	after int64,
) ([]Notice, error) {
	result := []Notice{}
	if after < 0 {
		return result, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(ctx, eventID); err != nil {
			return fmt.Errorf("lock revised-date notices: %w", err)
		}
		roles, confirmed, err := access.ContentReader(ctx, q, actor, eventID)
		if err != nil {
			return fmt.Errorf("authorize content reader: %w", err)
		}
		if !confirmed && len(roles) == 0 {
			return access.ErrForbidden
		}
		rows, err := q.ContentNotices(
			ctx,
			dbgen.ContentNoticesParams{EventID: eventID, After: after},
		)
		if err != nil {
			return fmt.Errorf("read revised dates: %w", err)
		}
		for _, r := range rows {
			result = append(
				result,
				Notice{
					ID:               r.ID,
					Revision:         r.Revision,
					LiveStart:        r.LiveStart,
					LiveEnd:          r.LiveEnd,
					SubmissionCutoff: r.SubmissionCutoff,
				},
			)
		}
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("read event announcements: %w", err)
	}
	return result, nil
}
