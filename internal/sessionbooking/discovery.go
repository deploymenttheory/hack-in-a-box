package sessionbooking

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

type Collection struct {
	Items      []Page     `json:"items"`
	NextCursor *uuid.UUID `json:"next_cursor,omitempty"`
}

// List supports both discovery and the personal agenda without projecting the
// staff-only raw event schedule, private rosters or unrelated booking identities.
func (s Service) List(
	ctx context.Context,
	actor, eventID, cursor uuid.UUID,
	limit int,
	own bool,
) (Collection, error) {
	result := Collection{Items: []Page{}}
	if limit < 1 || limit > 100 {
		return result, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(ctx, eventID); err != nil {
			return contextError("lock discovery event", err)
		}
		if err := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.EventRead,
		); err != nil {
			return contextError("authorize discovery", err)
		}
		rows, err := tx.Query(
			ctx,
			`SELECT s.id FROM bookable_sessions s WHERE s.event_id=$1 AND s.id>$2 AND (NOT $3 OR EXISTS(SELECT 1 FROM session_bookings b WHERE b.event_id=s.event_id AND b.session_id=s.id AND b.person_id=$4)) ORDER BY s.id LIMIT $5`,
			eventID,
			cursor,
			own,
			actor,
			limit+1,
		)
		if err != nil {
			return contextError("discover sessions", err)
		}
		ids := []uuid.UUID{}
		for rows.Next() {
			var id uuid.UUID
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return contextError("scan discovery", err)
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return contextError("read discovery", err)
		}
		if len(ids) > limit {
			next := ids[limit-1]
			result.NextCursor = &next
			ids = ids[:limit]
		}
		for _, id := range ids {
			st, err := load(ctx, tx, actor, eventID, id)
			if errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			page, err := s.page(ctx, tx, actor, st)
			if err != nil {
				return err
			}
			result.Items = append(result.Items, page)
		}
		return nil
	})
	return result, contextError("list sessions", err)
}

func (s Service) conflicts(
	ctx context.Context,
	tx pgx.Tx,
	actor uuid.UUID,
	st scope,
) ([]uuid.UUID, error) {
	result := []uuid.UUID{}
	rows, err := tx.Query(
		ctx,
		`SELECT s.id FROM bookable_sessions s JOIN session_bookings b ON b.event_id=s.event_id AND b.session_id=s.id WHERE s.event_id=$1 AND s.id<>$2 AND b.person_id=$3 AND b.state='booked' AND NOT s.cancelled AND NOT s.needs_review ORDER BY s.id`,
		st.row.ID,
		st.item.ID,
		actor,
	)
	if err != nil {
		return result, contextError("read conflict candidates", err)
	}
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return result, contextError("scan conflict", err)
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, contextError("read conflicts", err)
	}
	for _, id := range ids {
		other, err := load(ctx, tx, actor, st.row.ID, id)
		if errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) {
			continue
		}
		if err != nil {
			return result, err
		}
		if st.start.Before(other.end) && other.start.Before(st.end) {
			result = append(result, id)
		}
	}
	return result, nil
}
