package submissions

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// CutoffBinding is the original outcome recorded by Settle, independently of
// later organiser replacements. It contains no draft or submission prose.
type CutoffBinding struct {
	Outcome  string
	Revision *int64
	Cutoff   time.Time
}

// CutoffInTransaction is an internal evidence-owner seam. The caller holds the
// event lock and has invoked Settle; it does not infer outcomes independently.
func (s Service) CutoffInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, teamID uuid.UUID,
) (CutoffBinding, error) {
	q := dbgen.New(tx)
	state, err := q.SubmissionState(
		ctx,
		dbgen.SubmissionStateParams{EventID: eventID, TeamID: teamID},
	)
	if err != nil {
		return CutoffBinding{}, fmt.Errorf("read settled cutoff binding: %w", err)
	}
	if !state.FrozenAt.Valid {
		return CutoffBinding{}, fmt.Errorf("cutoff binding unavailable: %w", pgx.ErrNoRows)
	}
	change, err := q.SubmissionCutoffOutcome(
		ctx,
		dbgen.SubmissionCutoffOutcomeParams{
			EventID:    eventID,
			TeamID:     teamID,
			OccurredAt: state.FrozenAt.Time,
		},
	)
	if err != nil {
		return CutoffBinding{}, fmt.Errorf("read original cutoff outcome: %w", err)
	}
	out := CutoffBinding{Outcome: change.Action, Cutoff: state.FrozenAt.Time}
	if change.ActiveRevision.Valid {
		out.Revision = &change.ActiveRevision.Int64
	}
	return out, nil
}
