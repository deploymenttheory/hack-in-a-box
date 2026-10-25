package results

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

// TeamFeedbackSource is a narrowly typed report source, never a result release.
// It cannot carry rank, awards, raw judge cards or private mentor assessment.
type TeamFeedbackSource struct {
	ScoresAvailable bool                   `json:"scores_available"`
	ConfirmationID  uuid.UUID              `json:"confirmation_id"`
	JudgeRunID      uuid.UUID              `json:"judge_run_id"`
	Binding         string                 `json:"binding"`
	TeamID          uuid.UUID              `json:"team_id"`
	Reference       rubric.Reference       `json:"reference"`
	RequiredJudges  int                    `json:"required_judges"`
	Categories      []TeamFeedbackCategory `json:"categories"`
	Total           *Exact                 `json:"total"`
}
type TeamFeedbackCategory struct {
	CategoryID uuid.UUID `json:"category_id"`
	Label      string    `json:"label"`
	Average    Exact     `json:"average"`
}

// ConfirmedTeamFeedbackInTransaction reads an exact own-team confirmed source
// under current organiser authority. Callers own their independent draft/release
// and recipient checks in this same event-locked transaction.
func (s Service) ConfirmedTeamFeedbackInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
) (TeamFeedbackSource, error) {
	var out TeamFeedbackSource
	current, err := s.guard(ctx, tx, actor, eventID, false)
	if err != nil {
		return out, err
	}
	if !current.organiser {
		return out, access.ErrForbidden
	}
	q := dbgen.New(tx)
	authority, err := access.TeamPermissions(ctx, q, actor, eventID, teamID)
	if err != nil {
		return out, failure("current report target", err)
	}
	team, err := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: eventID, ID: teamID})
	if err != nil {
		return out, failure("published report target", err)
	}
	if !authority.Active || !authority.Organiser || team.State != "published" {
		return out, access.ErrForbidden
	}
	round, err := load(ctx, tx, eventID)
	if err != nil {
		return out, err
	}
	source, err := s.sourceMode(ctx, tx, actor, eventID, true)
	if err != nil {
		return out, err
	}
	if source.completion == nil || len(source.preview.Blockers) != 0 || round.Confirmation == nil ||
		round.Source.Binding != source.preview.Binding ||
		round.Confirmation.SourceBinding != source.preview.Binding {
		return out, access.ErrConflict
	}
	if err = s.Judging.ResultApprovalCurrentInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		source.preview.JudgeRunID,
		round.Confirmation.JudgeApprovalID,
	); err != nil {
		return out, failure("current report confirmation", err)
	}
	for _, team := range source.preview.Tallies {
		if team.TeamID != teamID {
			continue
		}
		out = TeamFeedbackSource{
			ScoresAvailable: true,
			ConfirmationID:  round.Confirmation.ID,
			JudgeRunID:      source.preview.JudgeRunID,
			Binding:         source.preview.Binding,
			TeamID:          teamID,
			Reference:       source.preview.Reference,
			RequiredJudges:  team.RequiredJudges,
			Total:           &team.Total,
			Categories:      []TeamFeedbackCategory{},
		}
		for _, category := range team.Categories {
			label := ""
			for _, definition := range source.preview.Criteria.Categories {
				if definition.ID == category.CategoryID {
					label = definition.Label
				}
			}
			out.Categories = append(
				out.Categories,
				TeamFeedbackCategory{
					CategoryID: category.CategoryID,
					Label:      label,
					Average:    category.Average,
				},
			)
		}
		return out, nil
	}
	// A confirmed outcome can support prose for a published unscored team.
	// Absence of a final tally is explicit; it never manufactures zero scores.
	return TeamFeedbackSource{
		ConfirmationID: round.Confirmation.ID,
		JudgeRunID:     source.preview.JudgeRunID,
		Binding:        source.preview.Binding,
		TeamID:         teamID,
		Reference:      source.preview.Reference,
		Categories:     []TeamFeedbackCategory{},
	}, nil
}
