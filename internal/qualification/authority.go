package qualification

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

type scope struct {
	row       dbgen.Event
	round     Round
	criteria  rubric.PublicDefinition
	reference rubric.Reference
	organiser bool
}

func (s Service) guard(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, runID uuid.UUID,
	write bool,
) (scope, error) {
	var out scope
	if s.Moderation == nil || s.Cards == nil || s.Freeze == nil {
		return out, access.ErrForbidden
	}
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return out, failure("authority", err)
	}
	out.row = row
	if err = (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		access.EventRead,
	); err != nil {
		return out, failure("authority", err)
	}
	roles, _, err := access.ContentReader(ctx, q, actor, eventID)
	if err != nil {
		return out, failure("authority", err)
	}
	out.organiser = slices.Contains(roles, "organiser")
	if !out.organiser && !slices.Contains(roles, "mentor") {
		return out, access.ErrForbidden
	}
	var dates event.Configuration
	if err = json.Unmarshal(row.Configuration, &dates); err != nil {
		return out, fmt.Errorf("read qualification clock: %w", err)
	}
	if !row.PublishedAt.Valid || s.now().Before(row.PublishedAt.Time) ||
		dates.SubmissionCutoff.IsZero() ||
		s.now().Before(dates.SubmissionCutoff) {
		return out, access.ErrForbidden
	}
	if write &&
		(!row.WritesAllowed || !slices.Contains([]string{"live", "assessment"}, row.Stage)) {
		return out, access.ErrForbidden
	}
	if out.reference, out.criteria, err = rubric.QualificationCriteriaInTransaction(
		ctx,
		tx,
		eventID,
		runID,
	); err != nil {
		return out, failure("authority", err)
	}
	if out.round, err = load(ctx, tx, eventID, runID); err != nil {
		return out, failure("authority", err)
	}
	if out.round.Version == 0 {
		if !out.organiser {
			return out, access.ErrForbidden
		}
		return out, nil
	}
	if !out.organiser && actor != out.round.Panel.OrganiserID &&
		!slices.Contains(out.round.Panel.MentorIDs, actor) {
		return out, access.ErrForbidden
	}
	return out, nil
}

func validatePanel(ctx context.Context, q *dbgen.Queries, eventID uuid.UUID, p Panel) error {
	if p.OrganiserID == uuid.Nil || len(p.MentorIDs) < 1 || len(p.MentorIDs) > 100 ||
		slices.Contains(p.MentorIDs, p.OrganiserID) {
		return access.ErrInvalid
	}
	roles, _, err := access.ContentReader(ctx, q, p.OrganiserID, eventID)
	if err != nil {
		return failure("authority", err)
	}
	if !slices.Contains(roles, "organiser") {
		return access.ErrForbidden
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range p.MentorIDs {
		if id == uuid.Nil || seen[id] {
			return access.ErrInvalid
		}
		seen[id] = true
		roles, _, err = access.ContentReader(ctx, q, id, eventID)
		if err != nil {
			return failure("authority", err)
		}
		if !slices.Contains(roles, "mentor") {
			return access.ErrForbidden
		}
	}
	return nil
}
