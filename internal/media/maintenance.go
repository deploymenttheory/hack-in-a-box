package media

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// Maintain journals invalidated reviews without waiting for a gallery reader.
// Reads independently enforce current consent before this eventual bookkeeping.
// Event and item keysets have no permanent lifetime cap.
func (s Service) Maintain(ctx context.Context) error {
	cursor := uuid.Nil
	for {
		events, err := dbgen.New(s.Pool).
			MediaMaintenanceEvents(ctx, dbgen.MediaMaintenanceEventsParams{Cursor: cursor, PageSize: 100})
		if err != nil {
			return fmt.Errorf("page media consent reconciliation: %w", err)
		}
		for _, eventID := range events {
			if err = database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
				if _, e := q.LockEvent(ctx, eventID); e != nil {
					return fmt.Errorf("lock media reconciliation: %w", e)
				}
				after := uuid.Nil
				for {
					rows, e := q.MediaPage(
						ctx,
						dbgen.MediaPageParams{
							EventID:   eventID,
							Cursor:    after,
							Organiser: true,
							PageSize:  100,
						},
					)
					if e != nil {
						return fmt.Errorf("page media reconciliation: %w", e)
					}
					for _, row := range rows {
						after = row.ID
						if row.State != "reviewed" && row.State != "released" {
							continue
						}
						_, d, e := currentDocument(ctx, q, eventID, row.ID)
						if e != nil {
							return e
						}
						e = s.reviewValid(ctx, tx, eventID, d)
						if e == nil {
							continue
						}
						if !errors.Is(e, access.ErrConflict) && !errors.Is(e, access.ErrForbidden) {
							return e
						}
						// An owner-policy transition withdraws future access even on an archive.
						// This is privacy bookkeeping, not a general archived content write grant.
						d.Version++
						d.State = "withdrawn"
						d.Action = "consent_revoked"
						d.Reason = "Current publication or identified-person consent authority no longer permits sharing"
						d.ActorID = d.OwnerID
						d.ChangedAt = s.now()
						d.ReleasedAt = nil
						d.ReleasedBy = nil
						if e = s.persist(ctx, q, eventID, d, audit.MediaRevoked); e != nil {
							return e
						}
					}
					if len(rows) < 100 {
						break
					}
				}
				return nil
			}); err != nil {
				if err != nil {
					return fmt.Errorf("resolve current media policy: %w", err)
				}
				return nil
			}
		}
		if len(events) < 100 {
			return nil
		}
		cursor = events[len(events)-1]
	}
}
