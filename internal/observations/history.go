package observations

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func (s Service) History(
	ctx context.Context,
	actor, eventID, teamID, id uuid.UUID,
) (History, error) {
	result := History{Observations: []Observation{}, Statuses: []Status{}}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, _, err := guard(ctx, q, actor, eventID, teamID)
		if err != nil {
			return err
		}
		if id != uuid.Nil {
			open, err := s.reviewOpen(row)
			if err != nil {
				return err
			}
			if !open {
				return access.ErrForbidden
			}
			rows, err := q.MentorObservationHistory(
				ctx,
				dbgen.MentorObservationHistoryParams{EventID: eventID, TeamID: teamID, ID: id},
			)
			if err != nil {
				return fmt.Errorf("read private observation history: %w", err)
			}
			if len(rows) == 0 {
				return access.ErrNotFound
			}
			if len(rows) > 1000 {
				return access.ErrInvalid
			}
			for _, item := range rows {
				result.Observations = append(result.Observations, observation(item))
			}
		}
		statuses, err := q.MentorStatusHistory(
			ctx,
			dbgen.MentorStatusHistoryParams{EventID: eventID, TeamID: teamID},
		)
		if err != nil {
			return fmt.Errorf("read operational status history: %w", err)
		}
		if len(statuses) > 1000 {
			return access.ErrInvalid
		}
		for _, item := range statuses {
			result.Statuses = append(result.Statuses, status(item))
		}
		return nil
	})
	if err != nil {
		return History{}, fmt.Errorf("read mentor history: %w", err)
	}
	return result, nil
}
