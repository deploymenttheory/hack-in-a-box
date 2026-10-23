package feedback

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

// SupportSummary is operational metadata, never feedback prose or assessment.
// Examples are bounded; totals include every currently authorised action.
type SupportSummary struct {
	Unresolved        int             `json:"unresolved"`
	Overdue           int             `json:"overdue"`
	UnavailableOwners int             `json:"unavailable_owners"`
	Examples          []SupportAction `json:"examples"`
}
type SupportAction struct {
	ID             uuid.UUID `json:"id"`
	FeedbackID     uuid.UUID `json:"feedback_id"`
	Version        int64     `json:"version"`
	Status         string    `json:"status"`
	DueAt          time.Time `json:"due_at"`
	OwnerAvailable bool      `json:"owner_available"`
}

func (s Service) SupportInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
) (SupportSummary, error) {
	out := SupportSummary{Examples: []SupportAction{}}
	q := dbgen.New(tx)
	row, err := lock(ctx, q, eventID)
	if err != nil {
		return out, err
	}
	if _, err = s.authority(ctx, q, actor, eventID, teamID, "setup"); err != nil {
		return out, err
	}
	cursor := uuid.Nil
	for {
		rows, readErr := tx.Query(
			ctx,
			`SELECT a.id,a.feedback_id,a.current_version FROM feedback_action_heads a JOIN feedback_heads h ON h.event_id=a.event_id AND h.id=a.feedback_id WHERE a.event_id=$1 AND h.team_id=$2 AND h.published_version IS NOT NULL AND a.id>$3 ORDER BY a.id LIMIT 100`,
			eventID,
			teamID,
			cursor,
		)
		if readErr != nil {
			return out, fmt.Errorf("page native support metadata: %w", readErr)
		}
		type reference struct {
			id, feedback uuid.UUID
			version      int64
		}
		batch := []reference{}
		for rows.Next() {
			var ref reference
			if readErr = rows.Scan(&ref.id, &ref.feedback, &ref.version); readErr != nil {
				rows.Close()
				return out, fmt.Errorf("read native support metadata: %w", readErr)
			}
			batch = append(batch, ref)
		}
		readErr = rows.Err()
		rows.Close()
		if readErr != nil {
			return out, fmt.Errorf("page support action metadata: %w", readErr)
		}
		for _, ref := range batch {
			cursor = ref.id
			h, headErr := head(ctx, q, eventID, ref.feedback)
			if headErr != nil {
				return out, headErr
			}
			a, actionErr := actionVersion(ctx, q, eventID, ref.id, ref.version)
			if actionErr != nil {
				return out, actionErr
			}
			view, authErr := s.actionView(ctx, q, row, actor, h, a)
			if authErr != nil {
				if errors.Is(authErr, access.ErrNotFound) ||
					errors.Is(authErr, access.ErrForbidden) ||
					errors.Is(authErr, event.ErrBlocked) {
					continue
				}
				return out, authErr
			}
			if a.Status != "open" && a.Status != "in_progress" {
				continue
			}
			out.Unresolved++
			if !s.now().Before(a.DueAt) {
				out.Overdue++
			}
			if !view.OwnerAvailable {
				out.UnavailableOwners++
			}
			if len(out.Examples) < 10 {
				out.Examples = append(
					out.Examples,
					SupportAction{
						ID:             a.ID,
						FeedbackID:     h.ID,
						Version:        a.Version,
						Status:         a.Status,
						DueAt:          a.DueAt,
						OwnerAvailable: view.OwnerAvailable,
					},
				)
			}
		}
		if len(batch) < 100 {
			break
		}
	}
	return out, nil
}
