package results

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
)

func advisoryAuthor(ctx context.Context, tx pgx.Tx, actor, eventID, teamID uuid.UUID) error {
	// A role or generic resource grant alone is insufficient: actual published
	// mentor coverage, current conflicts and a competing entry are all required.
	assignments, err := dbgen.New(tx).MentorAssignments(ctx, eventID)
	if err != nil {
		return failure("mentor advisory", err)
	}
	if !slices.ContainsFunc(assignments, func(a dbgen.MentorAssignment) bool {
		return a.Snapshot == "published" && a.TeamID == teamID && a.PersonID == actor
	}) {
		return access.ErrForbidden
	}
	if err := mentors.AuthorizeModerationPanelInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		teamID,
	); err != nil {
		return failure("mentor advisory", err)
	}
	a, err := access.TeamPermissions(ctx, dbgen.New(tx), actor, eventID, teamID)
	if err != nil {
		return failure("mentor advisory", err)
	}
	if !a.Active || (!a.Mentor && !a.Reviewer) {
		return access.ErrForbidden
	}
	return nil
}

func currentAdvisories(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	source assessmentSource,
) ([]MentorAdvisory, error) {
	out := []MentorAdvisory{}
	rows, err := dbgen.New(tx).
		CurrentResultMentorAdvisories(ctx, dbgen.CurrentResultMentorAdvisoriesParams{EventID: eventID, SourceBinding: source.preview.Binding})
	if err != nil {
		return nil, failure("current mentor advisory", err)
	}
	for _, row := range rows {
		// Retain previous history while removing it from current reads when the
		// author no longer has current assignment/conflict authority.
		err := advisoryAuthor(ctx, tx, row.AuthorID, eventID, row.TeamID)
		if errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) ||
			errors.Is(err, access.ErrConflict) {
			continue
		}
		if err != nil {
			return nil, failure("mentor advisory", err)
		}
		if !slices.ContainsFunc(source.preview.Ties, func(t Tie) bool {
			return t.Kind == "category" && t.ID == row.TieID &&
				slices.Contains(t.TeamIDs, row.TeamID)
		}) {
			continue
		}
		var item MentorAdvisory
		if err = json.Unmarshal(row.Document, &item); err != nil {
			return nil, failure("decode mentor advisory", err)
		}
		out = append(out, item)
	}
	return out, nil
}

func (s Service) advisoryPage(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	current authority,
	source assessmentSource,
) (AdvisoryPage, error) {
	out := AdvisoryPage{
		Revision:      current.event.Revision,
		RunID:         source.preview.JudgeRunID,
		SourceBinding: source.preview.Binding,
		Targets:       []AdvisoryTarget{},
	}
	if !current.mentor {
		return out, access.ErrForbidden
	}
	advice, err := currentAdvisories(ctx, tx, eventID, source)
	if err != nil {
		return out, failure("mentor advisory", err)
	}
	q := dbgen.New(tx)
	for _, tie := range source.preview.Ties {
		if tie.Kind != "category" || tie.CategoryID == nil {
			continue
		}
		for _, team := range tie.TeamIDs {
			err := advisoryAuthor(ctx, tx, actor, eventID, team)
			if errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) ||
				errors.Is(err, access.ErrConflict) {
				continue
			}
			if err != nil {
				return out, failure("mentor advisory", err)
			}
			frozen, e := (submissions.Service{Pool: s.Pool, Clock: s.now}).ReadInTransaction(
				ctx,
				tx,
				actor,
				eventID,
				team,
			)
			if e != nil {
				return out, failure("read assigned frozen advisory target", e)
			}
			if frozen.Outcome != "frozen" || frozen.Final == nil {
				return out, access.ErrConflict
			}
			named, e := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: eventID, ID: team})
			if e != nil {
				return out, failure("read assigned frozen advisory target", e)
			}
			item := AdvisoryTarget{
				TieID:              tie.ID,
				CategoryID:         *tie.CategoryID,
				TeamID:             team,
				TeamName:           named.Name,
				SubmissionRevision: frozen.Final.Revision,
			}
			for _, category := range source.preview.Criteria.Categories {
				if category.ID == item.CategoryID {
					item.CategoryName = category.Label
				}
			}
			for _, a := range advice {
				if a.TieID == tie.ID && a.TeamID == team && a.AuthorID == actor {
					own := a
					item.Own = &own
					item.Version = a.Version
				}
			}
			out.Targets = append(out.Targets, item)
		}
	}
	return out, nil
}

func (s Service) MentorAdvisories(
	ctx context.Context,
	actor, eventID uuid.UUID,
) (AdvisoryPage, error) {
	var out AdvisoryPage
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		current, err := s.guardRoles(ctx, tx, actor, eventID, false, true)
		if err != nil {
			return failure("mentor advisory", err)
		}
		if !current.mentor {
			return access.ErrForbidden
		}
		round, err := load(ctx, tx, eventID)
		if err != nil {
			return failure("mentor advisory", err)
		}
		source, err := s.ballotSource(ctx, tx, eventID, round)
		if err != nil {
			return failure("mentor advisory", err)
		}
		out, err = s.advisoryPage(ctx, tx, actor, eventID, current, source)
		return failure("mentor advisory", err)
	})
	if err != nil {
		return AdvisoryPage{}, failure("read own mentor advisory", err)
	}
	return out, nil
}

func advisoryReceipt(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID, requestID uuid.UUID,
) (*AdvisoryReceipt, error) {
	row, err := q.ResultMentorAdvisoryReceipt(
		ctx,
		dbgen.ResultMentorAdvisoryReceiptParams{EventID: eventID, RequestID: requestID},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // Absence is not an acknowledgement.
	}
	if err != nil {
		return nil, failure("mentor advisory", err)
	}
	if row.AuthorID != actor {
		return nil, access.ErrForbidden
	}
	var out AdvisoryReceipt
	err = json.Unmarshal(row.Receipt, &out)
	return &out, failure("decode advisory acknowledgement", err)
}

func (s Service) MentorAdvisoryReceipt(
	ctx context.Context,
	actor, eventID, requestID uuid.UUID,
) (AdvisoryRecovery, error) {
	var out AdvisoryRecovery
	if requestID == uuid.Nil {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		current, err := s.guardRoles(ctx, tx, actor, eventID, false, true)
		if err != nil {
			return failure("mentor advisory", err)
		}
		if !current.mentor {
			return access.ErrForbidden
		}
		out.Receipt, err = advisoryReceipt(ctx, q, actor, eventID, requestID)
		if err != nil || out.Receipt == nil {
			return failure("mentor advisory", err)
		}
		return advisoryAuthor(ctx, tx, actor, eventID, out.Receipt.TeamID)
	})
	if err != nil {
		return AdvisoryRecovery{}, failure("recover mentor advisory metadata", err)
	}
	return out, nil
}

func (s Service) Advise(
	ctx context.Context,
	actor, eventID uuid.UUID,
	expected int64,
	input AdvisoryCommand,
) (AdvisoryPage, error) {
	var out AdvisoryPage
	if !validAdvisory(expected, input) {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		current, err := s.guardRoles(ctx, tx, actor, eventID, true, true)
		if err != nil {
			return failure("mentor advisory", err)
		}
		if !current.mentor {
			return access.ErrForbidden
		}
		if err = advisoryAuthor(ctx, tx, actor, eventID, input.TeamID); err != nil {
			return failure("mentor advisory", err)
		}
		round, err := load(ctx, tx, eventID)
		if err != nil {
			return failure("mentor advisory", err)
		}
		source, err := s.ballotSource(ctx, tx, eventID, round)
		if err != nil {
			return failure("mentor advisory", err)
		}
		page, err := s.advisoryPage(ctx, tx, actor, eventID, current, source)
		if err != nil {
			return failure("mentor advisory", err)
		}
		index := slices.IndexFunc(
			page.Targets,
			func(t AdvisoryTarget) bool { return t.TieID == input.TieID && t.TeamID == input.TeamID },
		)
		if index < 0 {
			return access.ErrForbidden
		}
		target := page.Targets[index]
		if input.SourceBinding != source.preview.Binding ||
			input.SubmissionRevision != target.SubmissionRevision {
			return access.ErrConflict
		}
		fingerprint := digest(struct {
			Command          AdvisoryCommand
			ExpectedRevision int64
		}{input, expected})
		original, err := advisoryReceipt(ctx, q, actor, eventID, input.RequestID)
		if err != nil {
			return failure("mentor advisory", err)
		}
		if original != nil {
			if original.Fingerprint != fingerprint {
				return access.ErrConflict
			}
			out = page
			out.Receipt = original
			return nil
		}
		if expected != current.event.Revision || input.Version != target.Version {
			return access.ErrConflict
		}
		author, err := q.Person(ctx, actor)
		if err != nil {
			return failure("mentor advisory", err)
		}
		item := MentorAdvisory{
			TieID:              input.TieID,
			TeamID:             input.TeamID,
			AuthorID:           actor,
			AuthorName:         author.DisplayName,
			Version:            target.Version + 1,
			PreviousVersion:    target.Version,
			RunID:              source.preview.JudgeRunID,
			SourceBinding:      source.preview.Binding,
			SubmissionRevision: target.SubmissionRevision,
			Views:              input.Views,
			Reason:             input.Reason,
			At:                 s.now(),
		}
		raw, err := json.Marshal(item)
		if err != nil {
			return failure("mentor advisory", err)
		}
		if err = q.InsertResultMentorAdvisory(
			ctx,
			dbgen.InsertResultMentorAdvisoryParams{
				EventID:       eventID,
				TieID:         item.TieID,
				TeamID:        item.TeamID,
				AuthorID:      actor,
				Version:       item.Version,
				JudgeRunID:    item.RunID,
				SourceBinding: item.SourceBinding,
				Document:      raw,
			},
		); err != nil {
			return failure("mentor advisory", err)
		}
		receipt := AdvisoryReceipt{
			RequestID:        input.RequestID,
			ActorID:          actor,
			Action:           input.Action,
			RunID:            item.RunID,
			TieID:            item.TieID,
			TeamID:           item.TeamID,
			ExpectedRevision: expected,
			ExpectedVersion:  target.Version,
			Version:          item.Version,
			Fingerprint:      fingerprint,
		}
		raw, err = json.Marshal(receipt)
		if err != nil {
			return failure("mentor advisory", err)
		}
		if err = q.InsertResultMentorAdvisoryReceipt(
			ctx,
			dbgen.InsertResultMentorAdvisoryReceiptParams{
				EventID:   eventID,
				RequestID: input.RequestID,
				TieID:     item.TieID,
				TeamID:    item.TeamID,
				AuthorID:  actor,
				Version:   item.Version,
				Receipt:   raw,
			},
		); err != nil {
			return failure("mentor advisory", err)
		}
		current.event, err = q.BumpEventRevision(ctx, eventID)
		if err != nil {
			return failure("mentor advisory", err)
		}
		if err = audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &eventID,
				TargetID: item.TeamID,
				Action:   audit.ModerationChanged,
				Reason:   audit.UserRequest,
				Metadata: audit.Metadata{
					PreviousRevision: &expected,
					Revision:         &current.event.Revision,
				},
			},
		); err != nil {
			return failure("mentor advisory", err)
		}
		out, err = s.advisoryPage(ctx, tx, actor, eventID, current, source)
		out.Receipt = &receipt
		return failure("mentor advisory", err)
	})
	if err != nil {
		return AdvisoryPage{}, failure("record mentor advisory", err)
	}
	return out, nil
}

func validAdvisory(expected int64, input AdvisoryCommand) bool {
	return input.Action == "advise" && input.RequestID != uuid.Nil && input.TeamID != uuid.Nil &&
		input.Version >= 0 &&
		expected >= 0 &&
		input.SubmissionRevision > 0 &&
		len(input.SourceBinding) == 64 &&
		len(input.TieID) == 64 &&
		strings.TrimSpace(input.Views) != "" &&
		len(input.Views) <= 4000 &&
		strings.TrimSpace(input.Reason) != "" &&
		len(input.Reason) <= 2000
}
