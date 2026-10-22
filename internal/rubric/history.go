package rubric

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

type (
	Change struct {
		Revision int64      `json:"revision"`
		Version  int64      `json:"version"`
		Action   string     `json:"action"`
		ActorID  *uuid.UUID `json:"actor_id"`
		Reason   string     `json:"reason"`
		At       time.Time  `json:"at"`
	}
	History struct {
		Items      []Change `json:"items"`
		NextCursor *int64   `json:"next_cursor"`
	}
)

func (s Service) History(
	ctx context.Context,
	actor, eventID uuid.UUID,
	before int64,
	limit int32,
) (History, error) {
	out := History{Items: []Change{}}
	if before <= 0 {
		before = math.MaxInt64
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, err := authorize(ctx, q, actor, eventID, false); err != nil {
			return err
		}
		header, err := q.EventRubric(ctx, eventID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read rubric history header: %w", err)
		}
		rows, err := q.RubricChanges(
			ctx,
			dbgen.RubricChangesParams{
				EventID:  eventID,
				RubricID: header.ID,
				Before:   before,
				PageSize: limit + 1,
			},
		)
		if err != nil {
			return fmt.Errorf("read rubric history: %w", err)
		}
		if len(rows) > int(limit) {
			rows = rows[:limit]
			cursor := rows[len(rows)-1].Revision
			out.NextCursor = &cursor
		}
		for _, row := range rows {
			out.Items = append(
				out.Items,
				Change{
					Revision: row.Revision,
					Version:  row.Version,
					Action:   row.Action,
					ActorID:  row.ActorID,
					Reason:   row.Reason,
					At:       row.OccurredAt,
				},
			)
		}
		return nil
	})
	if err != nil {
		return History{}, fmt.Errorf("organiser rubric history: %w", err)
	}
	return out, nil
}
