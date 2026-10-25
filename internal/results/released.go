package results

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/resultpublication"
)

// Released reads only explicitly announced snapshots under current event and
// confirmed-attendee/staff audience authority. It cannot expose preparation.
func (s Service) Released(ctx context.Context, actor, eventID uuid.UUID) ([]Announcement, error) {
	out := []Announcement{}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		if err := s.releasedAudience(ctx, tx, actor, eventID); err != nil {
			return err
		}
		announcements, readErr := published(ctx, tx, eventID, s.now())
		out = announcements
		return readErr
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
	return latestPublished(ctx, tx, eventID, s.now())
}

// releasedAudience is shared by explicit result readers and the reviewed
// showcase. Publishing a project cannot grant access to an event result.
func (s Service) releasedAudience(ctx context.Context, tx pgx.Tx, actor, eventID uuid.UUID) error {
	q := dbgen.New(tx)
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

	return nil
}

// ProjectAwardsInTransaction returns only this team's already announced awards,
// never a confirmation, tally, candidate, vote or other team's metadata.
func (s Service) ProjectAwardsInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
) ([]resultpublication.AnnouncedAward, error) {
	out := []resultpublication.AnnouncedAward{}
	if err := s.releasedAudience(ctx, tx, actor, eventID); err != nil {
		return nil, err
	}
	announcements, err := latestPublished(ctx, tx, eventID, s.now())
	if err != nil {
		return nil, err
	}
	for _, value := range announcements {
		if value.Kind == "awards" {
			for _, award := range value.Awards {
				if award.Team.ID == teamID {
					out = append(out, award)
				}
			}
		}
	}
	return out, nil
}
