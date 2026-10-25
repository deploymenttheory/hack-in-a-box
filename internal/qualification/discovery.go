package qualification

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

type Run struct {
	RunID             uuid.UUID  `json:"run_id"`
	PreviousRunID     *uuid.UUID `json:"previous_run_id"`
	Name              string     `json:"name"`
	Version           int64      `json:"version"`
	CanConfigurePanel bool       `json:"can_configure_panel"`
	RecordLocked      bool       `json:"record_locked"`
}
type Rounds struct {
	Items []Run `json:"items"`
}

// Rounds discovers only actual current published mentor-final contexts under
// this actor's current native organiser/intended-panel authority. A row contains
// no team, card or selection payload and never grants its exact reader access.
func (s Service) Rounds(ctx context.Context, actor, eventID uuid.UUID) (Rounds, error) {
	out := Rounds{Items: []Run{}}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if err := (access.EventPolicy{Queries: q, Clock: s.Now}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.EventRead,
		); err != nil {
			return failure("discovery event", err)
		}
		runs, err := q.CurrentRubricRuns(ctx, eventID)
		if err != nil {
			return failure("discover current run", err)
		}
		for _, run := range runs {
			if run.Purpose != "mentor_final" {
				continue
			}
			v, err := s.guard(ctx, tx, actor, eventID, run.ID, currentRead)
			if privateDenial(err) {
				continue
			}
			if err != nil {
				return err
			}
			capabilities, err := s.baseCapabilities(ctx, tx, actor, eventID, v)
			if err != nil {
				return err
			}
			out.Items = append(
				out.Items,
				Run{
					RunID:             run.ID,
					PreviousRunID:     run.PreviousID,
					Name:              v.criteria.Name,
					Version:           v.round.Version,
					CanConfigurePanel: capabilities.CanConfigurePanel,
					RecordLocked:      capabilities.RecordLocked,
				},
			)
		}
		return nil
	})
	return out, failure("discover qualification", err)
}
