package results

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// Change keeps preparation, confirmation and each announcement independent.
// Final judges record their own ballots through Vote, never organiser authority.
func (s Service) Change(
	ctx context.Context,
	actor, eventID uuid.UUID,
	expected int64,
	input Command,
) (Page, error) {
	var out Page
	if err := validateCommand(expected, input); err != nil {
		return out, err
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		current, err := s.guard(ctx, tx, actor, eventID, true)
		if err != nil {
			return err
		}
		if !current.organiser {
			return access.ErrForbidden
		}
		fingerprint := digest(struct {
			Command          Command
			ExpectedRevision int64
		}{input, expected})
		original, err := originalReceipt(ctx, tx, actor, eventID, input.RequestID)
		if err != nil {
			return err
		}
		if original != nil {
			if original.Fingerprint != fingerprint {
				return access.ErrConflict
			}
			out, err = s.page(ctx, tx, actor, eventID, current)
			out.Receipt = original
			return err
		}
		round, err := load(ctx, tx, eventID)
		if err != nil {
			return err
		}
		if expected != current.event.Revision || input.Version != round.Version {
			return access.ErrConflict
		}
		source, err := s.source(ctx, tx, actor, eventID)
		if err != nil {
			return err
		}
		announcement, publicationBinding, err := s.applyDecision(
			ctx,
			tx,
			actor,
			eventID,
			input,
			source,
			&round,
		)
		if err != nil {
			return err
		}
		if err = s.retain(
			ctx,
			tx,
			eventID,
			actor,
			current.event.Revision,
			&round,
			input,
		); err != nil {
			return err
		}
		receipt := Receipt{
			RequestID:        input.RequestID,
			ActorID:          actor,
			Action:           input.Action,
			ExpectedRevision: expected,
			ExpectedVersion:  input.Version,
			Version:          round.Version,
			Fingerprint:      fingerprint,
		}
		if announcement != nil {
			announcement.Version = round.Version
			if err = retainPublication(
				ctx,
				tx,
				eventID,
				actor,
				publicationBinding,
				*announcement,
			); err != nil {
				return err
			}
			receipt.PublicationID = &announcement.ID
		}
		if err = retainReceipt(ctx, tx, eventID, receipt); err != nil {
			return err
		}
		current.event, err = dbgen.New(tx).Event(ctx, eventID)
		if err != nil {
			return failure("read changed event", err)
		}
		out, err = s.page(ctx, tx, actor, eventID, current)
		out.Receipt = &receipt
		return err
	})
	if err != nil {
		return Page{}, failure("change", err)
	}
	return out, nil
}

func validateCommand(expected int64, input Command) error {
	if input.RequestID == uuid.Nil || input.Version < 0 || expected < 0 ||
		strings.TrimSpace(input.Reason) == "" ||
		len(input.Reason) > 2000 ||
		len(input.Awards) > 50 ||
		len(input.Evidence) > 0 ||
		input.TieID != "" ||
		input.TeamID != uuid.Nil {
		return access.ErrInvalid
	}
	if input.Action != "prepare" && input.Action != "publish_finalists" &&
		input.Action != "confirm" &&
		input.Action != "publish_awards" {
		return access.ErrInvalid
	}
	if input.Action != "confirm" && len(input.Awards) != 0 {
		return access.ErrInvalid
	}
	return nil
}

func readyPreparedSource(round Round, source assessmentSource, binding string) bool {
	return round.PreparedBy != uuid.Nil && source.completion != nil &&
		len(source.preview.Blockers) == 0 &&
		binding != "" &&
		binding == source.preview.Binding &&
		round.Source.Binding == binding
}

func decisionBinding(binding string, votes []Vote, awards []Award) string {
	return digest(struct {
		SourceBinding string
		Votes         []Vote
		Awards        []Award
	}{binding, votes, awards})
}
