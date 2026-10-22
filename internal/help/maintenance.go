package help

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// Maintain reconciles native help sources independently of optional campaigns.
// The existing per-event transaction owns transitions, dedup and local intents;
// this method performs no external delivery.
func (s Service) Maintain(ctx context.Context) error {
	after := uuid.Nil
	for {
		events, err := dbgen.New(s.Pool).HelpMaintenanceEvents(ctx, after)
		if err != nil {
			return fmt.Errorf("page native help maintenance: %w", err)
		}
		if len(events) == 0 {
			return nil
		}
		for _, eventID := range events {
			err = database.Within(
				ctx,
				s.Pool,
				func(tx pgx.Tx, _ *dbgen.Queries) error { return s.MaintainInTransaction(ctx, tx, eventID) },
			)
			if err != nil {
				return fmt.Errorf("maintain native help event: %w", err)
			}
			after = eventID
		}
	}
}
