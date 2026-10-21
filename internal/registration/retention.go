package registration

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// Retain removes answer histories under the event's configured privacy deadline.
func (s Service) Retain(ctx context.Context) error {
	now := s.now()
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		ids, err := q.ExpiredFormResponseEvents(ctx, pgtype.Timestamptz{Time: now, Valid: true})
		if err != nil {
			return fmt.Errorf("find expired form responses: %w", err)
		}
		for _, id := range ids {
			row, err := q.LockEvent(ctx, id)
			if err != nil {
				return fmt.Errorf("lock form retention event: %w", err)
			}
			policy, err := q.PrivacyPolicy(ctx, id)
			if err != nil {
				return fmt.Errorf("read form retention policy: %w", err)
			}
			if !slices.Contains([]string{"completed", "cancelled", "archived"}, row.Stage) ||
				!row.CompletedAt.Valid ||
				row.CompletedAt.Time.AddDate(0, 0, int(policy.RetentionDays)).After(now) {
				continue
			}
			if err := q.ForgetRegistrationInput(ctx, id); err != nil {
				return fmt.Errorf("forget retained registration inputs: %w", err)
			}
			if _, err := q.DeleteFormResponses(ctx, id); err != nil {
				return fmt.Errorf("delete expired form responses: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("retain registration answers: %w", err)
	}
	return nil
}
