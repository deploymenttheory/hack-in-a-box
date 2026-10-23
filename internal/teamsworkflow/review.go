package teamsworkflow

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
)

// ValidateRecoveryInTransaction uses the normal execution policy without
// preparing an attempt or contacting the workflow endpoint.
func (s Service) ValidateRecoveryInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	d jobs.Delivery,
) error {
	_, connection, _, err := s.validateDelivery(ctx, tx, d)
	if err != nil {
		return fmt.Errorf("validate current Teams recovery: %w", err)
	}
	if _, err = s.open(connection); err != nil {
		return fmt.Errorf("open current Teams recovery configuration: %w", err)
	}
	return nil
}
