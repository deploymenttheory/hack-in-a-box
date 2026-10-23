package moderation

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
)

// ResolveScorecardContextInTransaction is the real #104 owning policy. Neither
// organiser status nor an opaque ID alone grants cross-author score visibility.
func (s Service) ResolveScorecardContextInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID, id uuid.UUID,
) (scorecards.ModerationContext, error) {
	if s.Cards == nil || s.Actions == nil {
		return scorecards.ModerationContext{}, access.ErrForbidden
	}
	h, err := load(ctx, tx, eventID, teamID, id)
	if err != nil {
		return scorecards.ModerationContext{}, err
	}
	if _, err = s.current(ctx, tx, actor, eventID, h, false); err != nil {
		return scorecards.ModerationContext{}, err
	}
	if !slices.Contains([]string{"open", "signed"}, h.State) {
		return scorecards.ModerationContext{}, access.ErrForbidden
	}
	if _, err = s.currentCards(ctx, tx, eventID, h); err != nil {
		return scorecards.ModerationContext{}, err
	}
	if err = s.authorizeDecisionReader(ctx, tx, actor, eventID, h); err != nil {
		return scorecards.ModerationContext{}, err
	}
	if h.Checkpoint.Purpose == "mentor_final" {
		decision, e := finalDecision(ctx, tx, eventID, teamID, h.Checkpoint.Definition.RunID)
		if e == nil && decision.HuddleID != id {
			return scorecards.ModerationContext{}, access.ErrConflict
		}
		if e != nil && !errors.Is(e, access.ErrNotFound) {
			return scorecards.ModerationContext{}, e
		}
	}
	unresolved, err := s.unresolved(ctx, tx, actor, eventID, h)
	if err != nil {
		return scorecards.ModerationContext{}, err
	}
	frozen := int64(0)
	if h.SubmissionRevision != nil {
		frozen = *h.SubmissionRevision
	}
	out := scorecards.ModerationContext{
		ID:                 id,
		Version:            h.Version,
		Binding:            binding(h),
		TeamID:             teamID,
		RunID:              h.Checkpoint.Definition.RunID,
		SubmissionRevision: frozen,
		RequiredAuthorIDs:  []uuid.UUID{h.PrimaryID, h.Review.ReviewerID},
		Cards:              slices.Clone(h.Cards),
		UnresolvedGaps:     unresolved,
	}
	if h.State == "signed" {
		if h.SignoffContext == nil || h.SignoffID == nil {
			return scorecards.ModerationContext{}, access.ErrConflict
		}
		out = *h.SignoffContext
		out.UnresolvedGaps = unresolved
	}
	return out, nil
}

func (s Service) unresolved(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	h Huddle,
) (int, error) {
	if s.Actions == nil {
		return 0, access.ErrForbidden
	}
	count := 0
	for _, gap := range h.Gaps {
		if gap.ResolvedAt == nil || gap.ResolvedBy == nil || gap.Action == nil {
			count++
			continue
		}
		// Resolve as the recorded decision author, never borrow the caller's broader
		// action/evidence authority to preserve another author's stale decision.
		current, err := s.Actions.ResolveModerationActionInTransaction(
			ctx,
			tx,
			*gap.ResolvedBy,
			eventID,
			h.TeamID,
			gap.ActionID,
			gap.ActionVersion,
		)
		if err != nil || current.Status != "done" || !current.OwnerEligible ||
			binding(current) != binding(*gap.Action) {
			count++
			continue
		}
		if err = s.Cards.AuthorizeModerationEvidenceInTransaction(
			ctx,
			tx,
			*gap.ResolvedBy,
			eventID,
			h.TeamID,
			gap.Evidence,
			h.SubmissionRevision,
		); err != nil {
			count++
		}
	}
	for _, p := range h.Proposals {
		if p.State == "proposed" {
			count++
			continue
		}
		if p.DecidedBy == nil || p.DecidedAt == nil || p.DecisionReason == "" {
			return 0, access.ErrConflict
		}
		if p.State == "accepted" {
			if err := s.Cards.AuthorizeModerationEvidenceInTransaction(
				ctx,
				tx,
				p.ProposedBy,
				eventID,
				h.TeamID,
				p.Evidence,
				h.SubmissionRevision,
			); err != nil {
				count++
			}
		}
	}
	return count, nil
}

// Panel delegates disclosure to the native #101 exact submitted-card boundary.
func (s Service) Panel(
	ctx context.Context,
	actor, eventID, teamID, id uuid.UUID,
) ([]scorecards.Card, error) {
	if s.Cards == nil {
		return nil, access.ErrForbidden
	}
	native := *s.Cards
	native.Moderation = &s
	cards, err := native.Panel(ctx, actor, eventID, teamID, id)
	if err != nil {
		return nil, fmt.Errorf("read actual huddle panel: %w", err)
	}
	return cards, nil
}

// Intended-panel context does not confer the action owner's separate category
// or resource permission. Submitted/proposed evidence also retains author ACLs.
func (s Service) authorizeDecisionReader(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	h Huddle,
) error {
	for _, gap := range h.Gaps {
		if s.Actions == nil {
			return access.ErrForbidden
		}
		if _, err := s.Actions.ResolveModerationActionInTransaction(
			ctx,
			tx,
			actor,
			eventID,
			h.TeamID,
			gap.ActionID,
			gap.ActionVersion,
		); err != nil {
			return fmt.Errorf("authorize current native gap action: %w", err)
		}
		if gap.ResolvedBy != nil {
			if err := s.Cards.AuthorizeModerationEvidenceInTransaction(
				ctx,
				tx,
				*gap.ResolvedBy,
				eventID,
				h.TeamID,
				gap.Evidence,
				h.SubmissionRevision,
			); err != nil {
				return fmt.Errorf("authorize recorded gap evidence: %w", err)
			}
		}
	}
	for _, proposal := range h.Proposals {
		if err := s.Cards.AuthorizeModerationEvidenceInTransaction(
			ctx,
			tx,
			proposal.ProposedBy,
			eventID,
			h.TeamID,
			proposal.Evidence,
			h.SubmissionRevision,
		); err != nil {
			return fmt.Errorf("authorize proposed change evidence: %w", err)
		}
	}
	return nil
}
