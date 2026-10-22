package scorecards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

func sameReferences(a, b []CardReference) bool {
	if len(a) != len(b) {
		return false
	}
	left, right := slices.Clone(a), slices.Clone(b)
	less := func(a, b CardReference) int { return slices.Compare(a.ID[:], b.ID[:]) }
	slices.SortFunc(left, less)
	slices.SortFunc(right, less)
	return slices.Equal(left, right)
}

// SignOffInTransaction is the only finalisation entrypoint. The future huddle
// owner calls it in its SAME persisted decision transaction. No HTTP caller or
// scorecard submission can manufacture a moderation context or signed score.
// The independent cards remain immutable; #110 consumes this exact proof rather
// than making its moderated totals a prerequisite of #101.
func (s Service) SignOffInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID, contextID uuid.UUID,
	references []CardReference,
) (Signoff, error) {
	proof, err := s.context(ctx, tx, actor, eventID, teamID, contextID)
	if err != nil {
		return Signoff{}, err
	}
	if proof.UnresolvedGaps != 0 || proof.SubmissionRevision < 1 ||
		!sameReferences(references, proof.Cards) {
		return Signoff{}, access.ErrConflict
	}
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return Signoff{}, fmt.Errorf("lock final card signoff: %w", err)
	}
	if !row.WritesAllowed {
		return Signoff{}, access.ErrForbidden
	}
	if err = s.completeFinalCards(ctx, tx, eventID, teamID, proof, references); err != nil {
		return Signoff{}, err
	}

	out := Signoff{
		ID: uuid.NewSHA1(
			contextID,
			[]byte(fmt.Sprintf("%d:%s", proof.Version, proof.Binding)),
		),
		TeamID:    teamID,
		RunID:     proof.RunID,
		Context:   proof,
		ActorID:   actor,
		CreatedAt: s.now(),
	}
	prior, err := q.MentorScorecardSignoff(
		ctx,
		dbgen.MentorScorecardSignoffParams{
			EventID:        eventID,
			ContextID:      contextID,
			ContextVersion: proof.Version,
		},
	)
	if err == nil {
		var stored Signoff
		if err = json.Unmarshal(prior.Proof, &stored); err != nil {
			return Signoff{}, fmt.Errorf("read retained signoff: %w", err)
		}
		if digest(stored.Context) != digest(proof) ||
			!sameReferences(stored.Context.Cards, references) {
			return Signoff{}, access.ErrConflict
		}
		return stored, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Signoff{}, fmt.Errorf("read current signoff identity: %w", err)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return Signoff{}, fmt.Errorf("encode exact final signoff: %w", err)
	}
	if err = q.InsertMentorScorecardSignoff(
		ctx,
		dbgen.InsertMentorScorecardSignoffParams{
			EventID:        eventID,
			ID:             out.ID,
			TeamID:         teamID,
			RunID:          out.RunID,
			ContextID:      contextID,
			ContextVersion: proof.Version,
			ActorID:        actor,
			Proof:          raw,
			CreatedAt:      out.CreatedAt,
		},
	); err != nil {
		return Signoff{}, fmt.Errorf("retain final card signoff: %w", err)
	}
	for _, ref := range references {
		if err = q.InsertMentorSignoffCard(
			ctx,
			dbgen.InsertMentorSignoffCardParams{
				EventID:     eventID,
				SignoffID:   out.ID,
				CardID:      ref.ID,
				CardVersion: ref.Version,
			},
		); err != nil {
			return Signoff{}, fmt.Errorf("retain signed exact source card: %w", err)
		}
	}
	if err = recordChange(
		ctx,
		q,
		eventID,
		actor,
		out.ID,
		row.Revision,
		audit.ScorecardSignedOff,
	); err != nil {
		return Signoff{}, err
	}
	return out, nil
}

func (s Service) completeFinalCards(
	ctx context.Context,
	tx pgx.Tx,
	eventID, teamID uuid.UUID,
	proof ModerationContext,
	references []CardReference,
) error {
	q := dbgen.New(tx)
	authors := map[uuid.UUID]bool{}
	for _, ref := range references {
		card, err := currentSubmitted(ctx, q, eventID, teamID, ref)
		if err != nil {
			return err
		}
		if card.Purpose != "mentor_final" || card.RunID != proof.RunID ||
			card.SubmissionRevision == nil ||
			*card.SubmissionRevision != proof.SubmissionRevision ||
			authors[card.AuthorID] {
			return access.ErrConflict
		}
		snapshot, err := (rubric.Service{Pool: s.Pool, Clock: s.Now}).StartAssessmentInTransaction(
			ctx,
			tx,
			card.AuthorID,
			eventID,
			teamID,
			card.RunID,
		)
		if err != nil {
			return fmt.Errorf("recheck final signoff assessment: %w", err)
		}
		if snapshot.Purpose != "mentor_final" || snapshot.Reference != card.Rubric ||
			snapshot.SubmissionRevision == nil ||
			*snapshot.SubmissionRevision != proof.SubmissionRevision {
			return access.ErrConflict
		}
		if _, err = normalizeScores(card.Scores, snapshot.Definition, true); err != nil {
			return err
		}
		if err = s.evidence(
			ctx,
			q,
			eventID,
			teamID,
			card.Scores,
			true,
			true,
			snapshot.SubmissionRevision,
		); err != nil {
			return err
		}
		authors[card.AuthorID] = true
	}
	for _, required := range proof.RequiredAuthorIDs {
		if !authors[required] {
			return access.ErrConflict
		}
	}
	return nil
}
