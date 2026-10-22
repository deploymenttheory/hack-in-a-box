package observations

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// AuthorizeEvidenceInTransaction reuses the private observation read policy for
// an exact retained version. A generic assessment assignment does not grant
// native observation access. This metadata-only check returns no private prose
// and shares the caller's event-locked transaction rather than nesting one.
func (s Service) AuthorizeEvidenceInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID, id uuid.UUID,
	version int64,
) error {
	if id == uuid.Nil || version < 1 {
		return access.ErrInvalid
	}
	q := dbgen.New(tx)
	row, _, err := guard(ctx, q, actor, eventID, teamID)
	if err != nil {
		return err
	}
	open, err := s.reviewOpen(row)
	if err != nil {
		return err
	}
	if !open {
		return access.ErrForbidden
	}
	exists, err := q.MentorObservationVersionExists(
		ctx,
		dbgen.MentorObservationVersionExistsParams{
			EventID: eventID,
			TeamID:  teamID,
			ID:      id,
			Version: version,
		},
	)
	if err != nil {
		return fmt.Errorf("check exact private observation evidence version: %w", err)
	}
	if !exists {
		return access.ErrNotFound
	}
	return nil
}
