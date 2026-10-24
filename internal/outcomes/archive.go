package outcomes

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// PublishedEventProjectsInTransaction selects only this event's deliberately
// released projects and rechecks the owning publication/media policy per item.
// It cannot project private outcome evidence or unpublished project versions.
func (s Service) PublishedEventProjectsInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, after uuid.UUID,
	limit int32,
) (ProjectPage, error) {
	out := ProjectPage{Items: []Project{}}
	if limit < 1 || limit > 100 {
		return out, access.ErrInvalid
	}
	q := dbgen.New(tx)
	if err := active(ctx, q, actor); err != nil {
		return out, err
	}
	rows, err := q.OutcomePublishedEventProjectsPage(
		ctx,
		dbgen.OutcomePublishedEventProjectsPageParams{
			EventID:  eventID,
			AfterID:  after,
			ActorID:  actor,
			AtTime:   s.now(),
			PageSize: limit + 1,
		},
	)
	if err != nil {
		return out, wrap("read released event projects", err)
	}
	more := len(rows) > int(limit)
	if more {
		rows = rows[:limit]
	}
	for _, row := range rows {
		p, e := s.publicProjection(ctx, tx, actor, row)
		if e != nil {
			return ProjectPage{}, e
		}
		out.Items = append(out.Items, p)
	}
	if more {
		id := rows[len(rows)-1].OutcomeID
		out.NextCursor = &id
	}
	return out, nil
}

// ArchiveReportingFacts counts retained readable records, not inferred business
// value, participant performance, or a declaration that an adopted system works.
type ArchiveReportingFacts struct {
	ContinuedProjects         int64
	DatedAdoptionObservations int64
}

func (s Service) ArchiveReportingFactsInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
) (ArchiveReportingFacts, error) {
	q := dbgen.New(tx)
	if err := (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		access.StaffRead,
	); err != nil {
		return ArchiveReportingFacts{}, wrap("authorize archive reporting facts", err)
	}
	v, err := q.OutcomeArchiveReportingFacts(
		ctx,
		dbgen.OutcomeArchiveReportingFactsParams{EventID: eventID, ActorID: actor, AtTime: s.now()},
	)
	if err != nil {
		return ArchiveReportingFacts{}, wrap("read retained outcome reporting facts", err)
	}
	return ArchiveReportingFacts{
		ContinuedProjects:         v.ContinuedProjects,
		DatedAdoptionObservations: v.DatedAdoptionObservations,
	}, nil
}
