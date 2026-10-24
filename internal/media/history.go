package media

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// HistoryEntry deliberately excludes historical document contents and identities.
// It cannot confer approval, byte access or an original command acknowledgement.
type HistoryEntry struct {
	Version   int64     `json:"version"`
	Action    string    `json:"action"`
	ChangedAt time.Time `json:"changed_at"`
}
type HistoryPage struct {
	Items             []HistoryEntry `json:"items"`
	NextBeforeVersion *int64         `json:"next_before_version"`
}

// History authorizes the actual current owner before reading immutable metadata.
// Read-only archives use the same current ownership check, without a write grant.
func (s Service) History(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	before int64,
	limit int32,
) (HistoryPage, error) {
	out := HistoryPage{Items: []HistoryEntry{}}
	if before < 0 || limit < 1 || limit > 100 {
		return out, access.ErrInvalid
	}
	if before == 0 {
		before = math.MaxInt64
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		sc, err := s.guard(ctx, tx, actor, eventID, false)
		if err != nil {
			return err
		}
		_, current, err := currentDocument(ctx, q, eventID, id)
		if err != nil {
			return err
		}
		if !owned(sc, actor, current) {
			return access.ErrForbidden
		}
		rows, err := q.MediaHistoryMetadata(
			ctx,
			dbgen.MediaHistoryMetadataParams{
				EventID:       eventID,
				ID:            id,
				BeforeVersion: before,
				PageSize:      limit + 1,
			},
		)
		if err != nil {
			return fmt.Errorf("read media decision metadata: %w", err)
		}
		if len(rows) > int(limit) {
			rows = rows[:limit]
			last := rows[len(rows)-1].Version
			out.NextBeforeVersion = &last
		}
		for _, row := range rows {
			out.Items = append(
				out.Items,
				HistoryEntry{Version: row.Version, Action: row.Action, ChangedAt: row.CreatedAt},
			)
		}
		return nil
	})
	if err != nil {
		return out, fmt.Errorf("read current-authorised media history: %w", err)
	}
	return out, nil
}
