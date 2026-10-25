package qualification

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
)

func (s Service) changeCard(ctx context.Context, tx pgx.Tx, actor, eventID uuid.UUID, v *scope, input Command) error {
	if !v.organiser {
		return access.ErrForbidden
	}
	teamID := input.TeamID
	if input.Card != nil {
		teamID = input.Card.TeamID
	}
	teams, err := s.cohort(ctx, tx, eventID, *v)
	if err != nil {
		return err
	}
	team, err := findTeam(teams, teamID)
	if err != nil {
		return err
	}
	if team.source == nil || team.source.ModeratorID != actor || !team.status.Eligible || len(team.status.Blockers) > 0 {
		return access.ErrConflict
	}
	if team.freeze.Status != "not_applicable" {
		if team.freeze.DecisionID != nil && (input.RepositoryDecisionID == nil || *input.RepositoryDecisionID != *team.freeze.DecisionID) {
			return access.ErrConflict
		}
		if team.freeze.DecisionID == nil && input.RepositoryDecisionID != nil {
			return access.ErrConflict
		}
	} else if input.RepositoryDecisionID != nil {
		return access.ErrInvalid
	}
	index := slices.IndexFunc(v.round.Cards, func(c FinalCard) bool { return c.TeamID == teamID })
	current := FinalCard{TeamID: teamID, State: "draft", Source: sourceReference(team), Scores: []scorecards.CategoryScore{}}
	if index >= 0 {
		current = v.round.Cards[index]
		if !s.cardCurrent(ctx, tx, eventID, current, team) {
			return access.ErrConflict
		}
	}
	if input.Action == "save_card" {
		if input.Card == nil || input.Card.Version != current.Version || current.State == "signed" {
			return access.ErrConflict
		}
		ids := []uuid.UUID{}
		for _, category := range v.criteria.Categories {
			ids = append(ids, category.ID)
		}
		if err = normalize(input.Card.Scores, ids); err != nil {
			return err
		}
		for _, score := range input.Card.Scores {
			category := v.criteria.Categories[slices.IndexFunc(v.criteria.Categories, func(c rubric.Category) bool { return c.ID == score.CategoryID })]
			if score.Value != nil && (*score.Value < category.MinScore || *score.Value > category.MaxScore) {
				return access.ErrInvalid
			}
			if len(score.Evidence) > 0 {
				if err = s.Cards.AuthorizeModerationEvidenceInTransaction(ctx, tx, actor, eventID, teamID, score.Evidence, &current.Source.SubmissionRevision); err != nil {
					return err
				}
			}
		}
		current.Scores = slices.Clone(input.Card.Scores)
	} else {
		if index < 0 || current.Version != input.CardVersion || current.State != "draft" || !complete(current, v.criteria.Categories) {
			return access.ErrConflict
		}
		current.State = "signed"
	}
	current.Version++
	current.ActorID = actor
	current.Reason = input.Reason
	current.ChangedAt = s.now()
	if index < 0 {
		v.round.Cards = append(v.round.Cards, current)
	} else {
		v.round.Cards[index] = current
	}
	return nil
}

func (s Service) decideEligibility(ctx context.Context, tx pgx.Tx, actor, eventID uuid.UUID, v *scope, input Command) error {
	if actor != v.round.Panel.OrganiserID || !slices.Contains([]string{"eligible", "ineligible"}, input.Decision) {
		return access.ErrForbidden
	}
	teams, err := s.cohort(ctx, tx, eventID, *v)
	if err != nil {
		return err
	}
	team, err := findTeam(teams, input.TeamID)
	if err != nil {
		return err
	}
	if team.status.Outcome != "frozen" || team.freeze.Status == "not_applicable" || team.freeze.Status == "completed" {
		return access.ErrConflict
	}
	revision := team.freeze.SubmissionRevision
	if err = s.Cards.AuthorizeModerationEvidenceInTransaction(ctx, tx, actor, eventID, input.TeamID, input.Evidence, &revision); err != nil {
		return err
	}
	decision := Eligibility{TeamID: input.TeamID, Binding: team.status.FreezeBinding, Decision: input.Decision, Reason: input.Reason, Evidence: slices.Clone(input.Evidence), ActorID: actor, At: s.now()}
	index := slices.IndexFunc(v.round.Eligibility, func(e Eligibility) bool { return e.TeamID == input.TeamID })
	if index < 0 {
		v.round.Eligibility = append(v.round.Eligibility, decision)
	} else {
		v.round.Eligibility[index] = decision
	}
	return nil
}

func complete(card FinalCard, categories []rubric.Category) bool {
	if len(card.Scores) != len(categories) {
		return false
	}
	for _, category := range categories {
		index := slices.IndexFunc(card.Scores, func(score scorecards.CategoryScore) bool { return score.CategoryID == category.ID })
		if index < 0 {
			return false
		}
		score := card.Scores[index]
		if score.Pending || score.Value == nil || *score.Value < category.MinScore || *score.Value > category.MaxScore || len(score.Evidence) == 0 {
			return false
		}
	}
	return true
}
