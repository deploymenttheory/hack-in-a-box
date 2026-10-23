package observations

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// SupportStatus exposes only operational state/time under native observation
// team-read authority; private evidence, reasons and next actions stay separate.
type SupportStatus struct {
	State     string     `json:"state"`
	ChangedAt *time.Time `json:"changed_at"`
}

func (s Service) SupportInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
) (SupportStatus, error) {
	out := SupportStatus{State: "unknown"}
	q := dbgen.New(tx)
	if _, _, err := guard(ctx, q, actor, eventID, teamID); err != nil {
		return out, err
	}
	current, err := q.CurrentMentorStatus(
		ctx,
		dbgen.CurrentMentorStatusParams{EventID: eventID, TeamID: teamID},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("read current operational support status: %w", err)
	}
	out.State = current.State
	out.ChangedAt = &current.ChangedAt
	return out, nil
}
