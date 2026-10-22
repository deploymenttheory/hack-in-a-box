package content

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// Acknowledge records only the acting person's explicit confirmation of the
// currently released version. It never turns a private preview into an audience.
func (s Service) Acknowledge(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	version int64,
) (Page, error) {
	var result Page
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock acknowledgement event: %w", err)
		}
		item, err := load(ctx, q, eventID, id)
		if err != nil {
			return err
		}
		result, err = projection(ctx, q, actor, item, false, 0)
		if err != nil {
			return err
		}
		if result.Version != version {
			return access.ErrConflict
		}
		if !row.WritesAllowed {
			return access.ErrForbidden
		}
		if !result.Document.RequireAcknowledgement {
			return blocked(
				"acknowledgement_not_required",
				"This released version does not require an acknowledgement.",
			)
		}
		inserted, err := q.AcknowledgeContent(
			ctx,
			dbgen.AcknowledgeContentParams{
				EventID:        eventID,
				ContentID:      id,
				Version:        version,
				PersonID:       actor,
				AcknowledgedAt: s.now(),
			},
		)
		if err != nil {
			return fmt.Errorf("record acknowledgement: %w", err)
		}
		if inserted > 0 {
			if err = audit.Append(
				ctx,
				q,
				audit.Record{
					ActorID:  &actor,
					EventID:  &eventID,
					TargetID: id,
					Action:   audit.ContentAcknowledged,
					Reason:   audit.UserRequest,
					Metadata: audit.Metadata{Revision: &version},
				},
			); err != nil {
				return fmt.Errorf("audit acknowledgement: %w", err)
			}
		}
		result, err = projection(ctx, q, actor, item, false, 0)
		return err
	})
	if err != nil {
		return Page{}, fmt.Errorf("acknowledge released version: %w", err)
	}
	return result, nil
}
