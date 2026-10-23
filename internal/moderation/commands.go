package moderation

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
)

func (s Service) Change(
	ctx context.Context,
	actor, eventID, teamID, id uuid.UUID,
	revision int64,
	input Command,
) (Huddle, error) {
	if s.Cards == nil || id == uuid.Nil || input.Version < 0 ||
		strings.TrimSpace(input.Reason) == "" ||
		len(input.Reason) > 2000 {
		return Huddle{}, access.ErrInvalid
	}
	var out Huddle
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, _, err := s.guard(ctx, tx, actor, eventID, teamID, true)
		if err != nil {
			return fmt.Errorf("resolve moderation dependency: %w", err)
		}
		if row.Revision != revision {
			return access.ErrConflict
		}
		h, err := load(ctx, tx, eventID, teamID, id)
		missing := errors.Is(err, access.ErrNotFound)
		if err != nil && !missing {
			return fmt.Errorf("resolve moderation dependency: %w", err)
		}
		if input.Action == "configure" {
			h, err = s.configure(ctx, tx, actor, eventID, teamID, id, h, missing, input)
			if err != nil {
				return err
			}
		} else {
			if missing {
				return access.ErrNotFound
			}
			if _, err = s.current(ctx, tx, actor, eventID, h, true); err != nil {
				return fmt.Errorf("resolve moderation dependency: %w", err)
			}
			if h.Version != input.Version ||
				slices.Contains([]string{"signed", "cancelled"}, h.State) {
				return access.ErrConflict
			}
			if h.State == "open" && input.Action != "cancel" {
				if _, err = s.currentCards(ctx, tx, eventID, h); err != nil {
					return err
				}
			}
			err = s.decide(ctx, tx, actor, eventID, &h, input)
			if err != nil {
				return fmt.Errorf("resolve moderation dependency: %w", err)
			}
		}
		if err = s.retain(
			ctx,
			tx,
			eventID,
			actor,
			revision,
			&h,
			input.Action,
			input.Reason,
		); err != nil {
			return fmt.Errorf("resolve moderation dependency: %w", err)
		}
		out = h
		return nil
	})
	if err != nil {
		return Huddle{}, fmt.Errorf("change moderation huddle: %w", err)
	}
	return out, nil
}

func (s Service) completeSecond(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	h *Huddle,
) error {
	if actor != h.Review.ReviewerID || h.State != "planned" {
		return access.ErrForbidden
	}
	card, err := s.Cards.SubmittedReviewInTransaction(
		ctx,
		tx,
		eventID,
		h.TeamID,
		actor,
		h.Checkpoint,
	)
	if err != nil {
		return fmt.Errorf("resolve moderation dependency: %w", err)
	}
	if _, err = (rubric.Service{Pool: s.Pool, Clock: s.Clock}).StartAssessmentInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		h.TeamID,
		h.Checkpoint.Definition.RunID,
	); err != nil {
		return fmt.Errorf("resolve moderation dependency: %w", err)
	}
	now := s.now()
	h.Review.Card = &card.Reference
	h.Review.CompletedAt = &now
	return nil
}

func (s Service) currentCards(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	h Huddle,
) ([]scorecards.Card, error) {
	result := []scorecards.Card{}
	for _, author := range []uuid.UUID{h.PrimaryID, h.Review.ReviewerID} {
		card, err := s.Cards.SubmittedReviewInTransaction(
			ctx,
			tx,
			eventID,
			h.TeamID,
			author,
			h.Checkpoint,
		)
		if err != nil {
			return nil, fmt.Errorf("read current independent review: %w", err)
		}
		if len(h.Cards) > 0 && !slices.Contains(h.Cards, card.Reference) {
			return nil, access.ErrConflict
		}
		result = append(result, card)
	}
	if h.Review.CompletedAt == nil || h.Review.Card == nil ||
		*h.Review.Card != result[1].Reference {
		return nil, access.ErrConflict
	}
	return result, nil
}

func (s Service) open(ctx context.Context, tx pgx.Tx, actor, eventID uuid.UUID, h *Huddle) error {
	if actor != h.Definition.ModeratorID || h.State != "planned" {
		return access.ErrForbidden
	}
	if s.Actions == nil {
		return access.ErrForbidden
	}
	cards, err := s.currentCards(ctx, tx, eventID, *h)
	if err != nil {
		return fmt.Errorf("resolve moderation dependency: %w", err)
	}
	for _, card := range cards {
		snapshot, e := (rubric.Service{Pool: s.Pool, Clock: s.Clock}).StartAssessmentInTransaction(
			ctx,
			tx,
			card.AuthorID,
			eventID,
			h.TeamID,
			h.Checkpoint.Definition.RunID,
		)
		if e != nil {
			return fmt.Errorf("read moderation dependency: %w", e)
		}
		if snapshot.Reference != h.Checkpoint.Rubric {
			return access.ErrConflict
		}
		if h.Checkpoint.Purpose == "mentor_final" {
			if snapshot.SubmissionRevision == nil || card.SubmissionRevision == nil ||
				*snapshot.SubmissionRevision != *card.SubmissionRevision {
				return access.ErrConflict
			}
			if h.SubmissionRevision != nil &&
				*h.SubmissionRevision != *snapshot.SubmissionRevision {
				return access.ErrConflict
			}
			h.SubmissionRevision = snapshot.SubmissionRevision
		}
		h.Cards = append(h.Cards, card.Reference)
	}
	h.State = "open"
	return nil
}

func (s Service) decide(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	h *Huddle,
	input Command,
) error {
	var err error
	switch input.Action {
	case "complete_second_review":
		err = s.completeSecond(ctx, tx, actor, eventID, h)
	case "open":
		err = s.open(ctx, tx, actor, eventID, h)
	case "cancel":
		if actor != h.Definition.ModeratorID {
			return access.ErrForbidden
		}
		h.State = "cancelled"
	case "add_gap", "resolve_gap":
		err = s.changeGap(ctx, tx, actor, eventID, h, input)
	case "propose_change", "accept_change", "reject_change":
		err = s.changeProposal(ctx, tx, actor, eventID, h, input)
	case "sign_off":
		if actor != h.Definition.ModeratorID || h.State != "open" ||
			h.Checkpoint.Purpose != "mentor_final" {
			return access.ErrForbidden
		}
		// The real native owner calls its sole finalisation seam in THIS decision
		// transaction. A failed gap/action/card check retains neither side.
		native := *s.Cards
		native.Moderation = &s
		signed, e := native.SignOffInTransaction(
			ctx,
			tx,
			actor,
			eventID,
			h.TeamID,
			h.Definition.ID,
			h.Cards,
		)
		if e != nil {
			return fmt.Errorf("read moderation dependency: %w", e)
		}
		h.State = "signed"
		h.SignoffID = &signed.ID
		h.SignoffContext = &signed.Context
	default:
		return access.ErrInvalid
	}
	if err != nil {
		return fmt.Errorf("apply moderation decision: %w", err)
	}
	return nil
}

func (s Service) configure(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID, id uuid.UUID,
	h Huddle,
	missing bool,
	input Command,
) (Huddle, error) {
	var err error
	if input.Definition == nil || input.Definition.ID != id {
		return Huddle{}, access.ErrInvalid
	}
	if missing {
		if input.Version != 0 {
			return Huddle{}, access.ErrConflict
		}
	} else {
		if _, err = s.current(ctx, tx, actor, eventID, h, true); err != nil {
			return Huddle{}, fmt.Errorf("resolve moderation dependency: %w", err)
		}
		if actor != h.Definition.ModeratorID || input.Version != h.Version ||
			h.State != "planned" {
			return Huddle{}, access.ErrConflict
		}
	}
	next, e := s.newHuddle(ctx, tx, actor, eventID, teamID, *input.Definition)
	if e != nil {
		return Huddle{}, fmt.Errorf("read moderation dependency: %w", e)
	}
	if !missing {
		next.Version = h.Version
	}
	return next, nil
}
