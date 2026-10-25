package submissions

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// VisibleEvidenceInTransaction projects exact immutable revisions under current
// native team/resource/audience authority. Historical judge access remains
// restricted to the selected final, as in the existing native history policy.
// Only evidence documents are returned; private reasons and drafts never cross
// this internal seam. The caller shares the native event transaction.
func (s Service) VisibleEvidenceInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
	revisions []int64,
) (map[int64]Document, error) {
	out := map[int64]Document{}
	v, err := s.scoped(ctx, tx, actor, eventID, teamID)
	if err != nil {
		return out, err
	}
	if !live(v, s.now()) {
		return out, nil
	}
	history := v.authority.Member || v.authority.Organiser || v.authority.Mentor ||
		v.authority.Reviewer
	q := dbgen.New(tx)
	for _, n := range revisions {
		if _, seen := out[n]; seen {
			continue
		}
		if !history && (!v.state.ActiveRevision.Valid || v.state.ActiveRevision.Int64 != n) {
			continue
		}
		row, err := q.SubmissionRevision(
			ctx,
			dbgen.SubmissionRevisionParams{EventID: eventID, TeamID: teamID, Revision: n},
		)
		if err != nil {
			return out, fmt.Errorf("read exact visible submission evidence: %w", err)
		}
		var document Document
		if err = json.Unmarshal(row.Document, &document); err != nil {
			return out, fmt.Errorf("decode exact visible submission evidence: %w", err)
		}
		out[n] = filtered(document, v.authority)
	}
	return out, nil
}
