package help

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// AuthorizeReferenceInTransaction validates a same-team source/link under the
// existing native read policy, returning no original request prose or location.
// A zero revision is a link to the currently authorised request, not an assertion
// that a particular historical version was reviewed.
func (s Service) AuthorizeReferenceInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID, id uuid.UUID,
	revision int64,
) error {
	q := dbgen.New(tx)
	row, err := lock(ctx, q, eventID)
	if err != nil {
		return err
	}
	r, err := load(ctx, q, eventID, id)
	if err != nil {
		return err
	}
	if r.TeamID != teamID || revision < 0 || (revision > 0 && r.Revision != revision) {
		return access.ErrNotFound
	}
	_, err = s.authorizeRead(ctx, q, row, actor, r)
	return err
}
