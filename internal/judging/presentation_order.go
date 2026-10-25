package judging

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// AuthorizePresentationOrderInTransaction grants only a shared published-order
// read. It does not grant a build, private card, final scoring or MC control.
func (s Service) AuthorizePresentationOrderInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
) error {
	a, err := s.guard(ctx, tx, actor, eventID, false)
	if err != nil {
		return err
	}
	if a.organiser {
		return nil
	}
	entries, err := dbgen.New(tx).SubmissionEntries(ctx, eventID)
	if err != nil {
		return failure("order assignment entries", err)
	}
	for _, entry := range entries {
		if !entry.Active || entry.TeamState != "published" {
			continue
		}
		ids, err := assigned(ctx, tx, eventID, entry.TeamID)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if id != actor {
				continue
			}
			eligible, err := eligibleJudge(ctx, tx, eventID, entry.TeamID, actor)
			if err != nil {
				return err
			}
			if eligible {
				return nil
			}
		}
	}
	return access.ErrForbidden
}
