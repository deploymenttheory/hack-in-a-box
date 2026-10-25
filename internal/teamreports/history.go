package teamreports

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// History is staff-only and always rechecks current native target authority.
// Immutable versions retain authors, original audiences and release times.
func (s Service) History(ctx context.Context, actor, eventID, teamID uuid.UUID) ([]Record, error) {
	out := []Record{}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		if err := s.guard(ctx, tx, actor, eventID, teamID, true, false); err != nil {
			return err
		}
		page, err := s.page(ctx, tx, actor, eventID, teamID)
		if err != nil {
			return err
		}
		rows, err := tx.Query(
			ctx,
			"SELECT document FROM team_feedback_versions WHERE event_id=$1 AND team_id=$2 ORDER BY version DESC LIMIT 100",
			eventID,
			teamID,
		)
		if err != nil {
			return failure("history rows", err)
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			var record Record
			if err = rows.Scan(&raw); err != nil {
				return failure("history row", err)
			}
			if err = json.Unmarshal(raw, &record); err != nil {
				return failure("history document", err)
			}
			if page.CurrentSource == nil || !currentScores(&record, *page.CurrentSource) {
				record.FinalScores = nil
			}
			out = append(out, record)
		}
		return failure("history iteration", rows.Err())
	})
	return out, failure("read report history", err)
}
