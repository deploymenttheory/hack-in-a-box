package submissionfreeze

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
)

// FinalEvidence is an exact native-final projection for trusted assessment
// owners. No reconciliation prose or observation payload is exposed.
type FinalEvidence struct {
	SourceID           uuid.UUID
	SubmissionRevision int64
	CutoffSource       Source
	Status             string
	DecisionID         *uuid.UUID
	SHA                string
}

// EvidenceForFinalInTransaction does not grant actor permissions. The owning
// assessor MUST first enforce its current assignment/window/conflict policy in
// this same event-locked transaction. No public judge shortcut calls this seam.
// A different native final, inactive entry or unresolved evidence never returns
// a verified code revision. File/URL-only native finals return not_applicable.
func (s Service) EvidenceForFinalInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, teamID, entryID uuid.UUID,
	revision int64,
) (FinalEvidence, error) {
	var out FinalEvidence
	q := dbgen.New(tx)
	entry, err := q.CompetitionEntry(
		ctx,
		dbgen.CompetitionEntryParams{EventID: eventID, TeamID: teamID},
	)
	if err != nil {
		return out, failure("assessment entry", err)
	}
	if entry.ID != entryID || !entry.Active {
		return out, access.ErrForbidden
	}
	team, err := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: eventID, ID: teamID})
	if err != nil {
		return out, failure("assessment team", err)
	}
	if team.State == "disbanded" {
		return out, access.ErrForbidden
	}
	state, err := q.SubmissionState(
		ctx,
		dbgen.SubmissionStateParams{EventID: eventID, TeamID: teamID},
	)
	if err != nil {
		return out, failure("assessment final", err)
	}
	if state.Outcome != "frozen" || !state.ActiveRevision.Valid ||
		state.ActiveRevision.Int64 != revision {
		return out, access.ErrConflict
	}
	final, err := q.SubmissionRevision(
		ctx,
		dbgen.SubmissionRevisionParams{EventID: eventID, TeamID: teamID, Revision: revision},
	)
	if err != nil {
		return out, failure("assessment native document", err)
	}
	var document submissions.Document
	if err = json.Unmarshal(final.Document, &document); err != nil {
		return out, failure("assessment applicability", err)
	}
	managed, uncaptured, _ := referenceFacts(document, "")
	if len(managed) == 0 && len(uncaptured) == 0 {
		return FinalEvidence{SubmissionRevision: revision, Status: "not_applicable"}, nil
	}

	row, err := q.SubmissionFreezeSource(
		ctx,
		dbgen.SubmissionFreezeSourceParams{EventID: eventID, TeamID: teamID},
	)
	if err != nil {
		return out, failure("assessment source", err)
	}
	var src Source
	if err = json.Unmarshal(row.Definition, &src); err != nil {
		return out, fmt.Errorf("decode assessment source: %w", err)
	}
	out = FinalEvidence{
		SourceID:           src.ID,
		SubmissionRevision: revision,
		CutoffSource:       src,
		Status:             src.Status,
	}
	if src.EntryID != entryID {
		return out, access.ErrConflict
	}
	target := src.SubmissionRevision

	decisions, err := q.SubmissionFreezeDecisions(ctx, src.ID)
	if err != nil {
		return out, failure("assessment decision", err)
	}
	if len(decisions) > 0 {
		var d Decision
		if err = json.Unmarshal(decisions[len(decisions)-1].Definition, &d); err != nil {
			return out, fmt.Errorf("decode assessment decision: %w", err)
		}
		target = &d.SubmissionRevision
		out.Status = "completed"
		out.DecisionID = &d.ID
		out.SHA = d.SHA
	}
	if target == nil || *target != revision {
		out.Status = "native_revision_mismatch"
		out.SHA = ""
		out.DecisionID = nil
	}
	return out, nil
}
