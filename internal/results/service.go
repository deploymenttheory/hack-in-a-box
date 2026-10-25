package results

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
)

// CalculateInTransaction composes the native completion source in the caller's event-locked
// transaction. The source owner checks current frozen evidence, all assigned
// required judges and independent submitted cards before arithmetic is possible.
func CalculateInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	owner *judging.Service,
	actor, eventID, runID uuid.UUID,
) (judging.Completion, []TeamTally, error) {
	if owner == nil {
		return judging.Completion{}, nil, access.ErrForbidden
	}
	return calculateCompletion(ctx, tx, owner.CompletionInTransaction, actor, eventID, runID)
}

func calculateCompletion(
	ctx context.Context,
	tx pgx.Tx,
	reader func(context.Context, pgx.Tx, uuid.UUID, uuid.UUID, uuid.UUID) (judging.Completion, error),
	actor, eventID, runID uuid.UUID,
) (judging.Completion, []TeamTally, error) {
	completion, err := reader(ctx, tx, actor, eventID, runID)
	if err != nil {
		return judging.Completion{}, nil, fmt.Errorf("read current judge completion: %w", err)
	}
	tallies, err := Calculate(
		JudgeSource{
			RunID:       completion.RunID,
			Reference:   completion.Reference,
			Criteria:    completion.Criteria,
			Assignments: completion.Assignments,
			Cards:       completion.Cards,
		},
	)
	if err != nil {
		return judging.Completion{}, nil, err
	}
	return completion, tallies, nil
}
