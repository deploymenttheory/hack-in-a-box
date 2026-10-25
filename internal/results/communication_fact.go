package results

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// AuthorizeCommunicationFact verifies the real latest released source and its
// current attendee/staff audience inside the caller's locked transaction. A fact
// is routing metadata, never a role/resource grant or a private result projection.
func (s Service) AuthorizeCommunicationFact(
	ctx context.Context,
	q *dbgen.Queries,
	actor uuid.UUID,
	fact dbgen.CommunicationFact,
) (bool, error) {
	if actor == uuid.Nil || fact.EventID == uuid.Nil || fact.ID == uuid.Nil ||
		fact.Kind != "results" ||
		fact.Status != "published" ||
		fact.Revision < 1 ||
		fact.OwnerID != nil ||
		fact.ResourceID != nil {
		return false, nil
	}
	event, err := q.LockEvent(ctx, fact.EventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, failure("communication fact event", err)
	}
	if !event.PublishedAt.Valid || s.now().Before(event.PublishedAt.Time) {
		return false, nil
	}
	current, err := q.CurrentResultCommunicationPublication(
		ctx,
		dbgen.CurrentResultCommunicationPublicationParams{
			EventID:       fact.EventID,
			PublicationID: fact.ID,
			ResultVersion: fact.Revision,
			AtTime:        s.now(),
		},
	)
	if err != nil || !current {
		return false, failure("communication fact publication", err)
	}
	if err = (access.EventPolicy{Queries: q, Clock: s.Now}).Authorize(
		ctx,
		actor,
		fact.EventID,
		fact.EventID,
		access.EventRead,
	); err != nil {
		if errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) ||
			errors.Is(err, access.ErrUnauthenticated) {
			return false, nil
		}
		return false, failure("communication fact parent audience", err)
	}
	roles, confirmed, err := access.ContentReader(ctx, q, actor, fact.EventID)
	if errors.Is(err, access.ErrForbidden) {
		return false, nil
	}
	if err != nil {
		return false, failure("communication fact reader", err)
	}
	return confirmed ||
		slices.ContainsFunc(
			roles,
			func(role string) bool { return slices.Contains([]string{"organiser", "mentor", "judge", "mc"}, role) },
		), nil
}
