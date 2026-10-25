package presentation

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// JudgeOrderAuthority is implemented by the actual current judge-assignment
// owner. A role, cached directory or controller origin alone is not a grant.
type (
	JudgeOrderAuthority interface {
		AuthorizePresentationOrderInTransaction(context.Context, pgx.Tx, uuid.UUID, uuid.UUID) error
	}
	JudgeOrderItem struct {
		Ordinal int    `json:"ordinal"`
		Title   string `json:"title"`
		Current bool   `json:"current"`
	}
	JudgeOrder struct {
		Revision       int64            `json:"revision"`
		OverrideActive bool             `json:"override_active"`
		Items          []JudgeOrderItem `json:"items"`
	}
)

// ReadJudgeOrder contains only public-to-room titles in published order. Future
// unreleased/private content is filtered by the same native projection as the
// room display. Public ordinals are compact, so hidden positions are not leaked.
func (s Service) ReadJudgeOrder(ctx context.Context, actor, eventID uuid.UUID) (JudgeOrder, error) {
	out := JudgeOrder{Items: []JudgeOrderItem{}}
	if s.OrderAuthority == nil {
		return out, access.ErrForbidden
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if err := s.OrderAuthority.AuthorizePresentationOrderInTransaction(
			ctx,
			tx,
			actor,
			eventID,
		); err != nil {
			return fmt.Errorf("authorize current judge order reader: %w", err)
		}
		e, err := q.Event(ctx, eventID)
		if err != nil {
			return fmt.Errorf("read order event: %w", err)
		}
		if !e.WritesAllowed {
			return nil
		}
		row, err := q.PresentationState(ctx, eventID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read published order: %w", err)
		}
		_, published, err := decode(row)
		if err != nil {
			return err
		}
		_, override, err := decodeOverride(row)
		if err != nil {
			return err
		}
		out.Revision = row.Revision
		out.OverrideActive = override
		for index, cue := range published {
			projected, visible, err := s.project(ctx, q, e, cue)
			if err != nil {
				return err
			}
			if !visible {
				continue
			}
			out.Items = append(
				out.Items,
				JudgeOrderItem{
					Ordinal: len(out.Items) + 1,
					Title:   projected.Title,
					Current: !override && index == int(row.CurrentIndex),
				},
			)
		}
		return nil
	})
	if err != nil {
		return JudgeOrder{}, fmt.Errorf("read current published judging order: %w", err)
	}
	return out, nil
}
