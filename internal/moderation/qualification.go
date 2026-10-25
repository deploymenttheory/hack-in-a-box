package moderation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
)

// QualificationSource contains immutable references, never independent scores,
// proposal prose, notes or drafts. The consuming owner must authorise its own
// actor/panel in the same transaction; this internal validation grants no read.
type QualificationSource struct {
	HuddleID           uuid.UUID
	HuddleVersion      int64
	ModeratorID        uuid.UUID
	SubmissionRevision int64
	Rubric             rubric.Reference
	Signoff            scorecards.Signoff
}

// QualificationSourceInTransaction revalidates the canonical signed final as
// its original intended moderator. It cannot substitute a checkpoint, another
// huddle, an incomplete review or a broader caller's evidence permission.
func (s Service) QualificationSourceInTransaction(ctx context.Context, tx pgx.Tx, eventID, teamID, runID uuid.UUID) (QualificationSource, error) {
	var out QualificationSource
	decision, err := finalDecision(ctx, tx, eventID, teamID, runID)
	if err != nil {
		return out, err
	}
	h, err := load(ctx, tx, eventID, teamID, decision.HuddleID)
	if err != nil {
		return out, err
	}
	if h.State != "signed" || h.Checkpoint.Purpose != "mentor_final" || h.Checkpoint.Definition.RunID != runID || h.Version != decision.HuddleVersion || h.SignoffID == nil || *h.SignoffID != decision.SignoffID || h.SignoffContext == nil || h.SignoffContext.Version != decision.ContextVersion {
		return out, access.ErrConflict
	}
	proof, err := s.ResolveScorecardContextInTransaction(ctx, tx, h.Definition.ModeratorID, eventID, teamID, h.Definition.ID)
	if err != nil {
		return out, err
	}
	if proof.UnresolvedGaps != 0 || proof.SubmissionRevision < 1 || binding(proof) != binding(*h.SignoffContext) {
		return out, access.ErrConflict
	}
	row, err := dbgen.New(tx).MentorScorecardSignoff(ctx, dbgen.MentorScorecardSignoffParams{EventID: eventID, ContextID: h.Definition.ID, ContextVersion: decision.ContextVersion})
	if err != nil {
		return out, fmt.Errorf("read canonical qualification proof: %w", err)
	}
	var signed scorecards.Signoff
	if err = json.Unmarshal(row.Proof, &signed); err != nil {
		return out, fmt.Errorf("decode canonical qualification proof: %w", err)
	}
	if signed.ID != decision.SignoffID || signed.TeamID != teamID || signed.RunID != runID || signed.ActorID != h.Definition.ModeratorID || binding(signed.Context) != binding(proof) {
		return out, access.ErrConflict
	}
	return QualificationSource{HuddleID: h.Definition.ID, HuddleVersion: h.Version, ModeratorID: h.Definition.ModeratorID, SubmissionRevision: proof.SubmissionRevision, Rubric: h.Checkpoint.Rubric, Signoff: signed}, nil
}
