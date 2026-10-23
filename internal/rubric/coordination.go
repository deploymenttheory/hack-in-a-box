package rubric

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// ValidateRunPublicationInTransaction verifies the exact native current common
// publication without starting/freezing an assessment. It grants no reader
// permission and returns no criteria; consumers must authorize their own reader.
func ValidateRunPublicationInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, runID uuid.UUID,
) error {
	q := dbgen.New(tx)
	if _, err := q.LockEvent(ctx, eventID); err != nil {
		return fmt.Errorf("lock coordination source publication: %w", err)
	}
	run, err := q.RubricRun(ctx, dbgen.RubricRunParams{EventID: eventID, ID: runID})
	if err != nil {
		return fmt.Errorf("read current coordination rubric run: %w", err)
	}
	h, err := q.EventRubric(ctx, eventID)
	if err != nil {
		return fmt.Errorf("read current coordination rubric publication: %w", err)
	}
	if run.Status != "current" || run.RubricID != h.ID || !h.PublishedVersion.Valid ||
		run.RubricVersion != h.PublishedVersion.Int64 {
		return access.ErrConflict
	}
	return currentRunPublication(ctx, q, eventID, run)
}
