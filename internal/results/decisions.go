package results

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
)

func (s Service) applyDecision(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	input Command,
	source assessmentSource,
	round *Round,
) (*Announcement, string, error) {
	var err error
	var announcement *Announcement
	publicationBinding := input.SourceBinding
	switch input.Action {
	case "prepare":
		full := source.preview.Binding != "" && len(source.preview.Blockers) == 0 &&
			input.SourceBinding == source.preview.Binding
		qualificationOnly := source.preview.FinalistsBinding != "" &&
			input.SourceBinding == source.preview.FinalistsBinding
		if !full && !qualificationOnly {
			return nil, publicationBinding, access.ErrConflict
		}
		round.Source = source.preview
		round.PreparedBy = actor
		round.Votes = []Vote{}
		round.Confirmation = nil
	case "publish_finalists":
		if source.preview.Selection == nil || source.preview.FinalistsBinding == "" ||
			input.SourceBinding != source.preview.FinalistsBinding {
			return nil, publicationBinding, access.ErrConflict
		}
		announcement, err = s.finalistAnnouncement(ctx, tx, eventID, source.preview)
		if err != nil {
			return nil, publicationBinding, err
		}
		// Preparation and release remain distinct; this snapshot records only the
		// exact reviewed source accompanying this deliberate release.
		if round.Source.Binding != source.preview.Binding ||
			round.Source.FinalistsBinding != source.preview.FinalistsBinding {
			round.Votes = []Vote{}
			round.Confirmation = nil
		}
		round.Source = source.preview
	case "confirm":
		err = s.confirmAwards(ctx, tx, actor, eventID, input, source, round)
	case "publish_awards":
		announcement, err = s.publishAwards(ctx, tx, actor, eventID, input, source, round)
		if err == nil {
			publicationBinding = round.Confirmation.DecisionBinding
		}
	}
	return announcement, publicationBinding, err
}

func (s Service) confirmAwards(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	input Command,
	source assessmentSource,
	round *Round,
) error {
	if !readyPreparedSource(*round, source, input.SourceBinding) {
		return access.ErrConflict
	}
	states, err := resolveJudgeTies(
		source.preview.Ties,
		source.completion.Assignments,
		round.Votes,
	)
	if err != nil {
		return failure("confirm awards", err)
	}
	if err := ValidateAwards(source.preview, states, input.Awards); err != nil {
		if errors.Is(err, ErrIncomplete) {
			return access.ErrConflict
		}
		return failure("confirm awards", err)
	}
	id := uuid.New()
	if err := s.Judging.RecordResultApprovalInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		source.preview.JudgeRunID,
		id,
		source.completion.Binding,
		input.Reason,
	); err != nil {
		return failure("confirm awards", err)
	}
	round.Confirmation = &Confirmation{
		ID:              id,
		JudgeApprovalID: id,
		SourceBinding:   source.preview.Binding,
		DecisionBinding: decisionBinding(source.preview.Binding, round.Votes, input.Awards),
		Awards:          input.Awards,
		ActorID:         actor,
		Reason:          input.Reason,
		At:              s.now(),
	}

	return nil
}

func (s Service) publishAwards(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	input Command,
	source assessmentSource,
	round *Round,
) (*Announcement, error) {
	if !readyPreparedSource(*round, source, input.SourceBinding) ||
		round.Confirmation == nil {
		return nil, access.ErrConflict
	}
	confirmation := round.Confirmation
	if confirmation.SourceBinding != source.preview.Binding ||
		confirmation.DecisionBinding != decisionBinding(
			source.preview.Binding,
			round.Votes,
			confirmation.Awards,
		) {
		return nil, access.ErrConflict
	}
	states, err := resolveJudgeTies(
		source.preview.Ties,
		source.completion.Assignments,
		round.Votes,
	)
	if err != nil {
		return nil, failure("publish awards", err)
	}
	if err := ValidateAwards(source.preview, states, confirmation.Awards); err != nil {
		if errors.Is(err, ErrIncomplete) {
			return nil, access.ErrConflict
		}
		return nil, failure("publish awards", err)
	}
	if err := s.Judging.ResultApprovalCurrentInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		source.preview.JudgeRunID,
		confirmation.JudgeApprovalID,
	); err != nil {
		return nil, failure("publish awards", err)
	}
	announcement, err := s.awardAnnouncement(ctx, tx, eventID, confirmation.Awards)
	if err != nil {
		return nil, failure("publish awards", err)
	}
	return announcement, nil
}
