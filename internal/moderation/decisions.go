package moderation

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
)

func (s Service) changeGap(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	h *Huddle,
	input Command,
) error {
	if h.State != "open" || s.Actions == nil {
		return access.ErrForbidden
	}
	if input.Action == "add_gap" {
		if input.GapID == uuid.Nil || input.ActionID == uuid.Nil || input.ActionVersion < 1 ||
			slices.ContainsFunc(h.Gaps, func(g Gap) bool { return g.ID == input.GapID }) {
			return access.ErrInvalid
		}
		action, err := s.Actions.ResolveModerationActionInTransaction(
			ctx,
			tx,
			actor,
			eventID,
			h.TeamID,
			input.ActionID,
			input.ActionVersion,
		)
		if err != nil {
			return fmt.Errorf("resolve moderation dependency: %w", err)
		}
		if action.ID != input.ActionID || action.Version != input.ActionVersion ||
			action.Binding == "" ||
			action.FeedbackID == uuid.Nil ||
			action.FeedbackVersion < 1 {
			return access.ErrConflict
		}
		h.Gaps = append(
			h.Gaps,
			Gap{
				ID:            input.GapID,
				ActionID:      input.ActionID,
				ActionVersion: input.ActionVersion,
				Reason:        input.Reason,
				Evidence:      []scorecards.EvidenceReference{},
			},
		)
		return nil
	}
	if actor != h.Definition.ModeratorID {
		return access.ErrForbidden
	}
	index := slices.IndexFunc(h.Gaps, func(g Gap) bool { return g.ID == input.GapID })
	if index < 0 {
		return access.ErrNotFound
	}
	gap := &h.Gaps[index]
	if input.ActionVersion < gap.ActionVersion {
		return access.ErrConflict
	}
	// Reopening/reassignment cannot silently keep an old completion binding.
	action, err := s.Actions.ResolveModerationActionInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		h.TeamID,
		gap.ActionID,
		input.ActionVersion,
	)
	if err != nil {
		return fmt.Errorf("resolve moderation dependency: %w", err)
	}
	if action.ID != gap.ActionID || action.Version != input.ActionVersion ||
		action.Status != "done" ||
		!action.OwnerEligible ||
		action.Binding == "" {
		return access.ErrConflict
	}
	if err = s.Cards.AuthorizeModerationEvidenceInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		h.TeamID,
		input.Evidence,
		h.SubmissionRevision,
	); err != nil {
		return fmt.Errorf("resolve moderation dependency: %w", err)
	}
	now := s.now()
	gap.ActionVersion = input.ActionVersion
	gap.Action = &action
	gap.ResolvedBy = &actor
	gap.ResolvedAt = &now
	gap.Resolution = input.Reason
	gap.Evidence = slices.Clone(input.Evidence)
	return nil
}

func (s Service) changeProposal(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	h *Huddle,
	input Command,
) error {
	if h.State != "open" {
		return access.ErrForbidden
	}
	if input.Action != "propose_change" {
		if actor != h.Definition.ModeratorID {
			return access.ErrForbidden
		}
		index := slices.IndexFunc(
			h.Proposals,
			func(p Proposal) bool { return p.ID == input.ProposalID },
		)
		if index < 0 {
			return access.ErrNotFound
		}
		p := &h.Proposals[index]
		if p.State != "proposed" {
			return access.ErrConflict
		}
		now := s.now()
		p.DecidedAt = &now
		p.DecidedBy = &actor
		p.DecisionReason = input.Reason
		p.State = map[string]string{"accept_change": "accepted", "reject_change": "rejected"}[input.Action]
		return nil
	}
	if input.ProposalID == uuid.Nil || input.ProposedValue == nil ||
		slices.ContainsFunc(
			h.Proposals,
			func(p Proposal) bool { return p.ID == input.ProposalID },
		) {
		return access.ErrInvalid
	}
	cards, err := s.currentCards(ctx, tx, eventID, *h)
	if err != nil {
		return fmt.Errorf("resolve moderation dependency: %w", err)
	}
	cardIndex := slices.IndexFunc(
		cards,
		func(c scorecards.Card) bool { return c.Reference == input.Card },
	)
	if cardIndex < 0 {
		return access.ErrNotFound
	}
	card := cards[cardIndex]
	index := slices.IndexFunc(
		card.Scores,
		func(c scorecards.CategoryScore) bool { return c.CategoryID == input.CategoryID },
	)
	if index < 0 {
		return access.ErrNotFound
	}
	categoryIndex := slices.IndexFunc(
		card.Criteria.Categories,
		func(c rubric.Category) bool { return c.ID == input.CategoryID },
	)
	if categoryIndex < 0 {
		return access.ErrConflict
	}
	category := card.Criteria.Categories[categoryIndex]
	if *input.ProposedValue < category.MinScore || *input.ProposedValue > category.MaxScore {
		return access.ErrInvalid
	}
	if err = s.Cards.AuthorizeModerationEvidenceInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		h.TeamID,
		input.Evidence,
		h.SubmissionRevision,
	); err != nil {
		return fmt.Errorf("resolve moderation dependency: %w", err)
	}
	h.Proposals = append(
		h.Proposals,
		Proposal{
			ID:                 input.ProposalID,
			Card:               input.Card,
			OriginalReviewerID: card.AuthorID,
			Original:           card.Scores[index],
			ProposedValue:      *input.ProposedValue,
			Evidence:           slices.Clone(input.Evidence),
			ProposedBy:         actor,
			ProposedAt:         s.now(),
			Reason:             input.Reason,
			State:              "proposed",
		},
	)
	return nil
}
