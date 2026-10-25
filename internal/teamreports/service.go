package teamreports

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/results"
)

func (s Service) source(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
) (results.TeamFeedbackSource, error) {
	if s.Results == nil {
		return results.TeamFeedbackSource{}, access.ErrForbidden
	}
	source, err := s.Results.ConfirmedTeamFeedbackInTransaction(ctx, tx, actor, eventID, teamID)
	return source, failure("confirmed result source", err)
}

func (s Service) page(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
) (Page, error) {
	var out Page
	record, err := latest(ctx, tx, eventID, teamID, false)
	if err != nil {
		return out, err
	}
	out.Latest = record
	if record != nil {
		out.Version = record.Version
	}
	source, err := s.source(ctx, tx, actor, eventID, teamID)
	if err != nil {
		if errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrConflict) ||
			errors.Is(err, access.ErrNotFound) {
			if record != nil {
				record.FinalScores = nil
			}
			return out, nil
		}
		return out, failure("current confirmation", err)
	}
	out.CurrentSource = &source
	if record != nil &&
		(record.Source.Binding != source.Binding || record.Source.ConfirmationID != source.ConfirmationID) {
		record.FinalScores = nil
	}
	return out, nil
}

func (s Service) Read(ctx context.Context, actor, eventID, teamID uuid.UUID) (Page, error) {
	var out Page
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		if err := s.guard(ctx, tx, actor, eventID, teamID, true, false); err != nil {
			return err
		}
		var err error
		out, err = s.page(ctx, tx, actor, eventID, teamID)
		return err
	})
	return out, failure("read private report preparation", err)
}

func (s Service) Change(
	ctx context.Context,
	actor, eventID, teamID uuid.UUID,
	expected int64,
	input Command,
) (Page, error) {
	var out Page
	if err := validate(expected, input); err != nil {
		return out, err
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		if err := s.guard(ctx, tx, actor, eventID, teamID, true, true); err != nil {
			return err
		}
		hash := fingerprint(struct {
			TeamID          uuid.UUID
			ExpectedVersion int64
			Command         Command
		}{teamID, expected, input})
		receipt, err := original(ctx, tx, actor, eventID, teamID, input.RequestID)
		if err != nil {
			return err
		}
		if receipt != nil {
			if receipt.Fingerprint != hash {
				return access.ErrConflict
			}
			out, err = s.page(ctx, tx, actor, eventID, teamID)
			out.Receipt = receipt
			return err
		}
		current, err := latest(ctx, tx, eventID, teamID, false)
		if err != nil {
			return err
		}
		version := int64(0)
		if current != nil {
			version = current.Version
		}
		if version != expected {
			return access.ErrConflict
		}
		source, err := s.source(ctx, tx, actor, eventID, teamID)
		if err != nil {
			return err
		}
		if source.Binding != input.SourceBinding {
			return access.ErrConflict
		}
		next := Record{
			Version:            version + 1,
			TeamID:             teamID,
			Body:               input.Body,
			State:              "draft",
			IncludeFinalScores: input.IncludeFinalScores,
			Source: Source{
				ConfirmationID: source.ConfirmationID,
				JudgeRunID:     source.JudgeRunID,
				Binding:        source.Binding,
				TeamID:         teamID,
			},
			AuthorID:  actor,
			ActorID:   actor,
			Audience:  []uuid.UUID{},
			ChangedAt: s.now(),
			Reason:    input.Reason,
		}
		if input.IncludeFinalScores {
			next.FinalScores = &source
		}
		if input.Action == "release" {
			if current == nil || current.State != "draft" ||
				current.Source.Binding != source.Binding ||
				current.Source.ConfirmationID != source.ConfirmationID {
				return access.ErrConflict
			}
			next = *current
			next.Version = version + 1
			next.State = "released"
			next.ActorID = actor
			next.ChangedAt = s.now()
			next.Reason = input.Reason
			releasedAt := s.now()
			next.ReleasedAt = &releasedAt
			next.Audience, err = audience(ctx, tx, eventID, teamID)
			if err != nil {
				return err
			}
		}
		accepted := Receipt{
			RequestID:       input.RequestID,
			ActorID:         actor,
			TeamID:          teamID,
			Action:          input.Action,
			ExpectedVersion: expected,
			Version:         next.Version,
			Fingerprint:     hash,
		}
		if err = retain(ctx, tx, eventID, next, accepted); err != nil {
			return err
		}
		out, err = s.page(ctx, tx, actor, eventID, teamID)
		out.Receipt = &accepted
		return err
	})
	return out, failure("change report", err)
}

func (s Service) OriginalReceipt(
	ctx context.Context,
	actor, eventID, teamID, requestID uuid.UUID,
) (*Receipt, error) {
	var out *Receipt
	if requestID == uuid.Nil {
		return nil, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		if err := s.guard(ctx, tx, actor, eventID, teamID, true, false); err != nil {
			return err
		}
		var err error
		out, err = original(ctx, tx, actor, eventID, teamID, requestID)
		return err
	})
	return out, failure("recover original report acknowledgement", err)
}

func (s Service) ReadReleased(
	ctx context.Context,
	actor, eventID, teamID uuid.UUID,
) (*Released, error) {
	var out *Released
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		if err := s.guard(ctx, tx, actor, eventID, teamID, false, false); err != nil {
			return err
		}
		record, err := latest(ctx, tx, eventID, teamID, true)
		if err != nil || record == nil {
			return err
		}
		if !slices.Contains(record.Audience, actor) {
			return access.ErrForbidden
		}
		// Current recipient scope is independent of the publisher's source check.
		// Only this released own-team projection crosses the boundary, never Results.Read.
		source, err := s.source(ctx, tx, record.ActorID, eventID, teamID)
		if err != nil {
			return err
		}
		if source.Binding != record.Source.Binding ||
			source.ConfirmationID != record.Source.ConfirmationID {
			return access.ErrConflict
		}
		out = &Released{
			Version:    record.Version,
			TeamID:     teamID,
			Body:       record.Body,
			AuthorID:   record.AuthorID,
			ReleasedBy: record.ActorID,
			ReleasedAt: *record.ReleasedAt,
		}
		if record.IncludeFinalScores {
			out.FinalScores = record.FinalScores
		}
		return nil
	})
	return out, failure("read released own-team report", err)
}
