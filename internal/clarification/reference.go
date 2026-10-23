package clarification

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

// AuthorizeQuestionReferenceInTransaction is an organiser-only metadata seam.
// Mentor, help, feedback or assessment authority cannot grant question access;
// actual released-source/current private question policy is rechecked here.
func (s Service) AuthorizeQuestionReferenceInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID, id uuid.UUID,
	revision int64,
) error {
	if revision < 1 {
		return access.ErrInvalid
	}
	q := dbgen.New(tx)
	if _, err := rubric.ReleasedSourceForReaderInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		s.now(),
	); err != nil {
		return fmt.Errorf("authorize question provenance source: %w", err)
	}
	if err := staff(ctx, q, actor, eventID); err != nil {
		return err
	}
	row, err := loadQuestion(ctx, q, eventID, id)
	if err != nil {
		return err
	}
	if row.TeamID != teamID || row.Revision != revision {
		return access.ErrNotFound
	}
	return teamAuthority(ctx, q, actor, eventID, teamID, true)
}
