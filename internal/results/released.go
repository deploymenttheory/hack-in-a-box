package results

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// Released reads only explicitly announced snapshots under current event and
// confirmed-attendee/staff audience authority. It cannot expose preparation.
func (s Service) Released(ctx context.Context, actor, eventID uuid.UUID) ([]Announcement, error) {
	out := []Announcement{}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(ctx, eventID); err != nil {
			return failure("lock released event", err)
		}
		if err := (access.EventPolicy{Queries: q, Clock: s.now}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.EventRead,
		); err != nil {
			return failure("released event access", err)
		}
		roles, confirmed, err := access.ContentReader(ctx, q, actor, eventID)
		if err != nil {
			return failure("released audience", err)
		}
		allowed, err := access.ContentAudienceAllows(
			ctx,
			q,
			actor,
			eventID,
			access.ContentAudience{
				Confirmed: true,
				Roles:     []string{"organiser", "mentor", "judge", "mc"},
			},
			roles,
			confirmed,
		)
		if err != nil {
			return failure("released audience policy", err)
		}
		if !allowed {
			return access.ErrForbidden
		}
		out, err = published(ctx, tx, eventID, s.now())
		return err
	})
	if err != nil {
		return nil, failure("read announced results", err)
	}
	return out, nil
}

// RoomAnnouncementsInTransaction is called only after presentation validates
// its scoped display pairing. It has no private round or controller projection.
func (s Service) RoomAnnouncementsInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
) ([]Announcement, error) {
	e, err := dbgen.New(tx).LockEvent(ctx, eventID)
	if err != nil {
		return nil, failure("room event", err)
	}
	if !e.PublishedAt.Valid || s.now().Before(e.PublishedAt.Time) {
		return []Announcement{}, nil
	}
	return published(ctx, tx, eventID, s.now())
}
