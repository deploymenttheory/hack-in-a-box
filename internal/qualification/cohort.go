package qualification

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/moderation"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissionfreeze"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
)

type nativeTeam struct {
	status TeamStatus
	source *moderation.QualificationSource
	freeze submissionfreeze.FinalEvidence
}

func (s Service) cohort(ctx context.Context, tx pgx.Tx, eventID uuid.UUID, v scope) ([]nativeTeam, error) {
	q := dbgen.New(tx)
	teams, err := q.HackTeams(ctx, eventID)
	if err != nil {
		return nil, err
	}
	out := []nativeTeam{}
	for _, team := range teams {
		item := nativeTeam{status: TeamStatus{TeamID: team.ID, Outcome: "inactive", Blockers: []string{}}}
		if team.State != "published" {
			item.status.Outcome = team.State
			out = append(out, item)
			continue
		}
		actor := v.round.Panel.OrganiserID
		if actor == uuid.Nil {
			return nil, access.ErrConflict
		}
		native, err := (submissions.Service{Pool: s.Pool, Clock: s.Now}).ReadInTransaction(ctx, tx, actor, eventID, team.ID)
		if err != nil {
			return nil, fmt.Errorf("read qualification output: %w", err)
		}
		item.status.EntryID = native.EntryID
		item.status.Outcome = native.Outcome
		if native.Outcome == "not_submitted" || native.Outcome == "withdrawn" {
			out = append(out, item)
			continue
		}
		if native.Outcome != "frozen" || native.Final == nil {
			item.status.Blockers = append(item.status.Blockers, "final_output_unresolved")
			out = append(out, item)
			continue
		}
		item.freeze, err = s.Freeze.EvidenceForFinalInTransaction(ctx, tx, actor, eventID, team.ID, native.EntryID, native.Final.Revision)
		if err != nil {
			return nil, fmt.Errorf("read qualification freeze identity: %w", err)
		}
		item.status.FreezeStatus = item.freeze.Status
		item.status.FreezeBinding = digest(item.freeze)
		item.status.Eligible = true
		if item.freeze.Status != "not_applicable" && (item.freeze.Status != "completed" || item.freeze.DecisionID == nil || item.freeze.SHA == "") {
			index := slices.IndexFunc(v.round.Eligibility, func(e Eligibility) bool { return e.TeamID == team.ID && e.Binding == item.status.FreezeBinding })
			if index < 0 {
				item.status.Blockers = append(item.status.Blockers, "freeze_eligibility_decision_required")
			} else {
				e := v.round.Eligibility[index]
				if err = s.Cards.AuthorizeModerationEvidenceInTransaction(ctx, tx, e.ActorID, eventID, team.ID, e.Evidence, &native.Final.Revision); err != nil {
					item.status.Blockers = append(item.status.Blockers, "freeze_eligibility_evidence_changed")
				} else if e.Decision == "ineligible" {
					item.status.Eligible = false
				}
			}
		}
		if item.status.Eligible {
			source, sourceErr := s.Moderation.QualificationSourceInTransaction(ctx, tx, eventID, team.ID, v.round.RunID)
			if sourceErr != nil {
				if errors.Is(sourceErr, access.ErrForbidden) || errors.Is(sourceErr, access.ErrConflict) || errors.Is(sourceErr, access.ErrNotFound) {
					item.status.Blockers = append(item.status.Blockers, "signed_independent_final_source_required")
				} else {
					return nil, sourceErr
				}
			} else {
				item.source = &source
			}
		}
		out = append(out, item)
	}
	slices.SortFunc(out, func(a, b nativeTeam) int { return compareID(a.status.TeamID, b.status.TeamID) })
	return out, nil
}

func sourceReference(team nativeTeam) FinalSource {
	h := team.source
	return FinalSource{HuddleID: h.HuddleID, HuddleVersion: h.HuddleVersion, SignoffID: h.Signoff.ID, SubmissionRevision: h.SubmissionRevision, Rubric: h.Rubric, Binding: digest(h), FreezeBinding: team.status.FreezeBinding, FreezeStatus: team.freeze.Status, RepositoryDecisionID: team.freeze.DecisionID}
}
func (s Service) cardCurrent(ctx context.Context, tx pgx.Tx, eventID uuid.UUID, card FinalCard, team nativeTeam) bool {
	if team.source == nil || !team.status.Eligible || len(team.status.Blockers) > 0 || digest(card.Source) != digest(sourceReference(team)) {
		return false
	}
	if card.ActorID != team.source.ModeratorID {
		return false
	}
	for _, score := range card.Scores {
		if score.Pending || len(score.Evidence) == 0 {
			continue
		}
		if err := s.Cards.AuthorizeModerationEvidenceInTransaction(ctx, tx, card.ActorID, eventID, card.TeamID, score.Evidence, &card.Source.SubmissionRevision); err != nil {
			return false
		}
	}
	return true
}
func findTeam(teams []nativeTeam, id uuid.UUID) (nativeTeam, error) {
	for _, team := range teams {
		if team.status.TeamID == id {
			return team, nil
		}
	}
	return nativeTeam{}, access.ErrNotFound
}
func normalize(scores []scorecards.CategoryScore, criteria []uuid.UUID) error {
	seen := map[uuid.UUID]bool{}
	for _, score := range scores {
		if seen[score.CategoryID] || !slices.Contains(criteria, score.CategoryID) || len(score.Evidence) > 50 {
			return access.ErrInvalid
		}
		seen[score.CategoryID] = true
		if score.Pending && score.Value != nil {
			return access.ErrInvalid
		}
	}
	return nil
}
