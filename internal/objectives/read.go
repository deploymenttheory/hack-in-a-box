package objectives

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	oq "github.com/deploymenttheory/hack-in-a-box/internal/objectives/objectivequery"
)

func (s Service) List(
	ctx context.Context,
	actor, eventID, cursor uuid.UUID,
	limit int32,
) (List, error) {
	out := List{Items: []View{}}
	if limit < 1 || limit > 100 {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		event, err := authorize(ctx, tx, actor, eventID, false)
		if err != nil {
			return err
		}
		q := oq.New(tx)
		rows, err := q.ObjectiveHeads(
			ctx,
			oq.ObjectiveHeadsParams{EventID: eventID, Cursor: cursor, PageSize: limit + 1},
		)
		if err != nil {
			return fmt.Errorf("list objective heads: %w", err)
		}
		more := len(rows) > int(limit)
		if more {
			rows = rows[:limit]
		}
		for _, row := range rows {
			v, err := load(ctx, q, eventID, Reference{ID: row.ID, Version: row.CurrentVersion})
			if err != nil {
				return err
			}
			out.Items = append(
				out.Items,
				View{
					Version:        v,
					CurrentVersion: row.CurrentVersion,
					CanManage:      event.WritesAllowed,
					PlanningGaps:   v.Definition.PlanningGaps(),
				},
			)
		}
		if more {
			id := rows[len(rows)-1].ID
			out.NextCursor = &id
		}
		return nil
	})
	if err != nil {
		return out, fmt.Errorf("list objectives configuration: %w", err)
	}
	return out, nil
}

func (s Service) History(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	before int64,
	limit int32,
) (History, error) {
	out := History{Items: []Version{}}
	if before < 0 || limit < 1 || limit > 100 {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		if _, err := authorize(ctx, tx, actor, eventID, false); err != nil {
			return err
		}
		q := oq.New(tx)
		if _, err := q.ObjectiveHead(
			ctx,
			oq.ObjectiveHeadParams{EventID: eventID, ID: id},
		); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return access.ErrNotFound
			}
			return fmt.Errorf("read objective history identity: %w", err)
		}
		rows, err := q.ObjectiveHistory(
			ctx,
			oq.ObjectiveHistoryParams{
				EventID:       eventID,
				ObjectiveID:   id,
				BeforeVersion: before,
				PageSize:      limit + 1,
			},
		)
		if err != nil {
			return fmt.Errorf("read objective history: %w", err)
		}
		more := len(rows) > int(limit)
		if more {
			rows = rows[:limit]
		}
		for _, row := range rows {
			v, err := load(ctx, q, eventID, Reference{ID: id, Version: row.Version})
			if err != nil {
				return err
			}
			out.Items = append(out.Items, v)
		}
		if more {
			v := rows[len(rows)-1].Version
			out.NextCursor = &v
		}
		return nil
	})
	if err != nil {
		return out, fmt.Errorf("read objectives history: %w", err)
	}
	return out, nil
}

// ResolveMeasureInTransaction is the canonical retained-version resolver for
// reporting owners. It does not grant report access or record an observation.
// Its caller must separately apply current report/resource authority; event
// organisers retain configuration access here, including archived versions.
func ResolveMeasureInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	ref MeasureReference,
) (Measure, error) {
	var out Measure
	if ref.Objective.ID == uuid.Nil || ref.Objective.Version < 1 || ref.MeasureID == uuid.Nil {
		return out, access.ErrInvalid
	}
	if _, err := authorize(ctx, tx, actor, eventID, false); err != nil {
		return out, err
	}
	v, err := load(ctx, oq.New(tx), eventID, ref.Objective)
	if err != nil {
		return out, err
	}
	for _, m := range v.Definition.Measures {
		if m.ID == ref.MeasureID {
			return m, nil
		}
	}
	return out, access.ErrNotFound
}
