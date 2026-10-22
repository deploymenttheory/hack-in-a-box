package submissions

import (
	"context"
	"fmt"
	"math"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func (s Service) History(
	ctx context.Context,
	actor, eventID, teamID uuid.UUID,
	before int64,
	limit int32,
) (History, error) {
	result := History{Items: []Change{}}
	if before < 0 || limit < 1 || limit > 100 {
		return result, access.ErrInvalid
	}
	if before == 0 {
		before = math.MaxInt64
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		v, err := s.scoped(ctx, tx, actor, eventID, teamID)
		if err != nil {
			return err
		}
		if !v.authority.Member && !v.authority.Organiser && !v.authority.Mentor &&
			!v.authority.Reviewer {
			return access.ErrForbidden
		}
		if !live(v, s.now()) {
			return blocked(
				"live_start",
				"review_not_open",
				"Formal submission history opens at live start.",
			)
		}
		rows, err := q.SubmissionChanges(
			ctx,
			dbgen.SubmissionChangesParams{
				EventID:  eventID,
				TeamID:   teamID,
				Revision: before,
				Limit:    limit + 1,
			},
		)
		if err != nil {
			return fmt.Errorf("read submission changes: %w", err)
		}
		if len(rows) > int(limit) {
			rows = rows[:limit]
			n := rows[len(rows)-1].Revision
			result.NextCursor = &n
		}
		for _, row := range rows {
			item := Change{
				Revision:     row.Revision,
				ActorID:      row.ActorID,
				Action:       row.Action,
				ReviewStatus: row.ReviewStatus,
				Reason:       row.Reason,
				At:           row.OccurredAt,
			}
			if row.ActiveRevision.Valid {
				n := row.ActiveRevision.Int64
				item.ActiveRevision = &n
			}
			if row.Action == "submitted" {
				item.Submission, err = revision(ctx, q, v, row.ActiveRevision.Int64)
				if err != nil {
					return err
				}
			}
			result.Items = append(result.Items, item)
		}
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("read submission history: %w", err)
	}
	return result, nil
}
