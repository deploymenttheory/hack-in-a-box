package objectives

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	oq "github.com/deploymenttheory/hack-in-a-box/internal/objectives/objectivequery"
)

// SnapshotInTransaction copies active planning configuration by value. It cannot
// include achieved outcomes, evidence, approvals or history because none of those
// are represented by Configuration. The caller holds the event lock.
func SnapshotInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
) ([]Configuration, error) {
	out := []Configuration{}
	if _, err := authorize(ctx, tx, actor, eventID, false); err != nil {
		return out, err
	}
	q := oq.New(tx)
	cursor := uuid.Nil
	for {
		rows, err := q.ObjectiveHeads(
			ctx,
			oq.ObjectiveHeadsParams{EventID: eventID, Cursor: cursor, PageSize: 100},
		)
		if err != nil {
			return nil, fmt.Errorf("read objective configuration snapshot: %w", err)
		}
		for _, row := range rows {
			v, err := load(ctx, q, eventID, Reference{ID: row.ID, Version: row.CurrentVersion})
			if err != nil {
				return nil, err
			}
			if !v.Retired {
				out = append(out, Configuration{ID: row.ID, Definition: v.Definition})
			}
			cursor = row.ID
		}
		if len(rows) < 100 {
			return out, nil
		}
	}
}

// ImportInTransaction creates fresh version1 definitions in a newly cloned event.
// It accepts configuration only and requires remapped identities and no copied
// person owners. No disclosure or realised-result data can be imported here.
func (s Service) ImportInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	configs []Configuration,
) error {
	if _, err := authorize(ctx, tx, actor, eventID, true); err != nil {
		return err
	}
	q := oq.New(tx)
	for _, config := range configs {
		if config.ID == uuid.Nil || config.Definition.OwnerID != nil {
			return access.ErrInvalid
		}
		if err := config.Definition.Validate(); err != nil {
			return err
		}
		raw, err := json.Marshal(config.Definition)
		if err != nil {
			return fmt.Errorf("encode cloned objective: %w", err)
		}
		if err = q.CreateObjectiveHead(
			ctx,
			oq.CreateObjectiveHeadParams{EventID: eventID, ID: config.ID},
		); err != nil {
			return fmt.Errorf("create cloned objective: %w", err)
		}
		if err = q.AppendObjectiveVersion(
			ctx,
			oq.AppendObjectiveVersionParams{
				EventID:     eventID,
				ObjectiveID: config.ID,
				Version:     1,
				Definition:  raw,
				Retired:     false,
				Reason:      "Independent configuration copied from template",
				ActorID:     actor,
				CreatedAt:   s.now(),
			},
		); err != nil {
			return fmt.Errorf("retain cloned objective version: %w", err)
		}
	}
	return nil
}
