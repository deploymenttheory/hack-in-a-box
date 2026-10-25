package judging

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
)

// AuthorizeResultReceiptInTransaction protects immutable acknowledgement
// metadata under current role, assignment and conflict authority. It does not
// reopen a private source or require the old deliberation to remain writable.
func (s Service) AuthorizeResultReceiptInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
) error {
	a, err := s.guard(ctx, tx, actor, eventID, false)
	if err != nil {
		return err
	}
	if !a.judge {
		return access.ErrForbidden
	}
	ids, err := assigned(ctx, tx, eventID, teamID)
	if err != nil {
		return err
	}
	if !slices.Contains(ids, actor) {
		return access.ErrForbidden
	}
	ok, err := eligibleJudge(ctx, tx, eventID, teamID, actor)
	if err != nil {
		return err
	}
	if !ok {
		return access.ErrForbidden
	}
	return nil
}

// AuthorizeResultTargetInTransaction checks current judge/assignment/conflict,
// selected frozen final and open deliberation in the caller's transaction.
// It returns no source data and grants no additional assessment access.
func (s Service) AuthorizeResultTargetInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, runID, teamID uuid.UUID,
) error {
	_, err := s.resultTargetDocument(ctx, tx, actor, eventID, runID, teamID)
	return err
}

// AuthorizeResultEvidenceInTransaction validates a result ballot's own evidence
// against this actor's currently visible frozen final. The result owner must
// separately validate its complete current tally, tied candidates and command.
func (s Service) AuthorizeResultEvidenceInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, runID, teamID uuid.UUID,
	evidence []Evidence,
) error {
	document, err := s.resultTargetDocument(ctx, tx, actor, eventID, runID, teamID)
	if err != nil {
		return err
	}
	if len(evidence) < 1 || len(evidence) > 30 {
		return access.ErrInvalid
	}
	known := map[uuid.UUID]bool{}
	for _, item := range document.Evidence {
		known[item.DeliverableID] = true
	}
	// These are the same reference and byte-length rules as native judge cards.
	for _, item := range evidence {
		if len(item.Note) > 4000 || item.DeliverableID != nil && !known[*item.DeliverableID] ||
			item.DeliverableID == nil && strings.TrimSpace(item.Note) == "" {
			return access.ErrInvalid
		}
	}
	return nil
}

func (s Service) resultTargetDocument(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, runID, teamID uuid.UUID,
) (submissions.Document, error) {
	_, page, err := s.assessment(ctx, tx, actor, eventID, runID, teamID)
	if err != nil {
		return submissions.Document{}, failure("result ballot target", err)
	}
	_, open, err := round(ctx, tx, eventID, runID)
	if err != nil {
		return submissions.Document{}, failure("result deliberation", err)
	}
	if !open {
		return submissions.Document{}, access.ErrConflict
	}
	return page.Final.Document, nil
}
