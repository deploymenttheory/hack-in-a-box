package event

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
)

// Challenge approval belongs to its immutable content version and exact common
// publication instant. Event rescheduling invalidates it in the same transaction.
func invalidateChallengeApproval(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	actor uuid.UUID,
	c Configuration,
	now time.Time,
) error {
	if row.PublishedAt.Valid {
		return nil
	}
	due := pgtype.Timestamptz{}
	if !c.LiveStart.IsZero() {
		at, err := schedule.ChallengePublication(c.LiveStart, c.Timezone, c.DSTChoice)
		if err != nil {
			return fmt.Errorf("plan content invalidation: %w", err)
		}
		due = pgtype.Timestamptz{Time: at, Valid: true}
	}
	if err := q.InvalidateChallengeApproval(
		ctx,
		dbgen.InvalidateChallengeApprovalParams{
			EventID:   row.ID,
			DueAt:     due,
			ActorID:   actor,
			ChangedAt: now,
		},
	); err != nil {
		return fmt.Errorf("invalidate challenge approval: %w", err)
	}
	return nil
}
