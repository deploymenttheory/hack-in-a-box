package submissionfreeze

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
)

// FinalEvidence is an exact current native-final projection. It contains no
// private original revision, historical SHA, decision prose or candidate payload.
type FinalEvidence struct {
	SourceID           uuid.UUID
	SubmissionRevision int64
	RepositoryID       int64
	RepositoryURL      string
	Branch             string
	Cutoff             time.Time
	Status             string
	DecisionID         *uuid.UUID
	DecidedAt          *time.Time
	SHA                string
}

// EvidenceForFinalInTransaction preserves actual parent/team/assignment/audience
// policy via the native owner and contains no public judge shortcut. The owning
// assessment module must additionally enforce its own scoring window/conflicts
// in this same event-locked transaction. No unscoped metadata can grant access.
func (s Service) EvidenceForFinalInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID, entryID uuid.UUID,
	revision int64,
) (FinalEvidence, error) {
	var out FinalEvidence
	v, err := s.scoped(ctx, tx, actor, eventID, teamID)
	if err != nil {
		return out, err
	}
	if !v.authority.Active || v.native.EntryID != entryID {
		return out, access.ErrForbidden
	}
	if v.native.Outcome != "frozen" || v.native.Final == nil ||
		v.native.Final.Revision != revision {
		return out, access.ErrConflict
	}
	p, err := s.page(ctx, tx, actor, v, eventID, teamID)
	if err != nil {
		return out, err
	}
	out = FinalEvidence{
		SubmissionRevision: revision,
		Status:             p.Status,
		Cutoff:             p.Cutoff,
		RepositoryID:       p.RepositoryID,
		RepositoryURL:      p.RepositoryURL,
		Branch:             p.Branch,
	}
	if p.Source != nil {
		out.SourceID = p.Source.ID
	}
	if p.BindingCurrent && p.Status == "completed" && p.Current != nil &&
		p.Current.SubmissionRevision == revision {
		out.SHA = p.Current.SHA
		out.DecisionID = &p.Current.ID
		out.DecidedAt = &p.Current.At
	}
	return out, nil
}
