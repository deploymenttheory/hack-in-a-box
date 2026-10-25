package qualification

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
)

// A recorded qualification panel grants the explicitly authored result, never
// the referenced native private observation or an entire submission document.
// Drop a withheld reference together with its attached prose; allocate new
// slices so a recipient projection cannot alter authored/history state.
func (s Service) projectReferences(
	ctx context.Context,
	tx pgx.Tx,
	reader, author, eventID, teamID uuid.UUID,
	refs []scorecards.EvidenceReference,
	frozen *int64,
) ([]scorecards.EvidenceReference, error) {
	out := []scorecards.EvidenceReference{}
	for _, ref := range refs {
		err := s.Cards.AuthorizeModerationEvidenceInTransaction(
			ctx,
			tx,
			reader,
			eventID,
			teamID,
			[]scorecards.EvidenceReference{ref},
			frozen,
		)
		if err == nil && ref.Kind == "submission" {
			native := submissions.Service{Pool: s.Pool, Clock: s.Now}
			var source, visible map[int64]submissions.Document
			source, err = native.VisibleEvidenceInTransaction(
				ctx,
				tx,
				author,
				eventID,
				teamID,
				[]int64{*ref.Version},
			)
			if err == nil {
				visible, err = native.VisibleEvidenceInTransaction(
					ctx,
					tx,
					reader,
					eventID,
					teamID,
					[]int64{*ref.Version},
				)
			}
			if err == nil {
				original, hasSource := source[*ref.Version]
				current, hasVisible := visible[*ref.Version]
				if !hasSource || !hasVisible || digest(original) != digest(current) {
					continue
				}
			}
		}
		if errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) ||
			errors.Is(err, access.ErrConflict) {
			continue
		}
		if err != nil {
			return nil, failure("projection", err)
		}
		out = append(out, ref)
	}
	return out, nil
}

func (s Service) projectRoundEvidence(
	ctx context.Context,
	tx pgx.Tx,
	reader, eventID uuid.UUID,
	round *Round,
) error {
	if err := s.projectRoundTargets(ctx, tx, reader, eventID, round); err != nil {
		return err
	}
	round.Cards = slices.Clone(round.Cards)
	for i := range round.Cards {
		card := &round.Cards[i]
		card.Scores = slices.Clone(card.Scores)
		for j := range card.Scores {
			refs, err := s.projectReferences(
				ctx,
				tx,
				reader,
				card.ActorID,
				eventID,
				card.TeamID,
				card.Scores[j].Evidence,
				&card.Source.SubmissionRevision,
			)
			if err != nil {
				return failure("projection", err)
			}
			card.Scores[j].Evidence = refs
		}
	}
	round.Eligibility = slices.Clone(round.Eligibility)
	for i := range round.Eligibility {
		decision := &round.Eligibility[i]
		refs, err := s.projectReferences(
			ctx,
			tx,
			reader,
			decision.ActorID,
			eventID,
			decision.TeamID,
			decision.Evidence,
			nil,
		)
		if err != nil {
			return failure("projection", err)
		}
		decision.Evidence = refs
	}
	round.Votes = slices.Clone(round.Votes)
	for i := range round.Votes {
		vote := &round.Votes[i]
		refs, err := s.projectReferences(
			ctx,
			tx,
			reader,
			vote.ActorID,
			eventID,
			vote.TeamID,
			vote.Evidence,
			nil,
		)
		if err != nil {
			return failure("projection", err)
		}
		vote.Evidence = refs
	}
	round.Decisions = slices.Clone(round.Decisions)
	for i := range round.Decisions {
		decision := &round.Decisions[i]
		refs, err := s.projectReferences(
			ctx,
			tx,
			reader,
			decision.ActorID,
			eventID,
			decision.TeamID,
			decision.Evidence,
			nil,
		)
		if err != nil {
			return failure("projection", err)
		}
		decision.Evidence = refs
	}
	return nil
}
