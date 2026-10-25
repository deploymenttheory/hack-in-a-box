package qualification

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
)

// These prepare/readiness projections mirror existing command checks. They are
// neither a grant nor a substitute for the command's current native validation.
type Capabilities struct {
	CanReadPanel      bool `json:"can_read_panel"`
	CanConfigurePanel bool `json:"can_configure_panel"`
	CanPropose        bool `json:"can_propose"`
	CanSignOff        bool `json:"can_sign_off"`
	ReadOnly          bool `json:"read_only"`
	RecordLocked      bool `json:"record_locked"`
}
type TieAction struct {
	TieID            string      `json:"tie_id"`
	Seat             int         `json:"seat"`
	CandidateTeamIDs []uuid.UUID `json:"candidate_team_ids"`
	CanVote          bool        `json:"can_vote"`
	HasVoted         bool        `json:"has_voted"`
	CanResolve       bool        `json:"can_resolve"`
	ResolveTeamIDs   []uuid.UUID `json:"resolve_team_ids"`
	Casting          bool        `json:"casting"`
	RequiredVotes    int         `json:"required_votes"`
	VotesComplete    bool        `json:"votes_complete"`
}

func writable(v scope) bool {
	return v.currentRun && v.row.WritesAllowed &&
		slices.Contains([]string{"live", "assessment"}, v.row.Stage) &&
		v.round.Signoff == nil
}

func (s Service) baseCapabilities(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	v scope,
) (Capabilities, error) {
	out := Capabilities{
		ReadOnly: !v.currentRun || !v.row.WritesAllowed ||
			!slices.Contains([]string{"live", "assessment"}, v.row.Stage),
		RecordLocked: v.round.Signoff != nil,
	}
	if writable(v) && v.organiser {
		err := (access.EventPolicy{Queries: dbgen.New(tx), Clock: s.Now}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.EventEdit,
		)
		if err != nil && !privateDenial(err) {
			return out, failure("configuration capability", err)
		}
		out.CanConfigurePanel = err == nil
	}
	return out, nil
}

func (s Service) teamActions(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	v scope,
	team nativeTeam,
) TeamStatus {
	out := team.status
	if team.source != nil && (panelReader(actor, v.round) || team.source.ModeratorID == actor) {
		source := sourceReference(team)
		out.Source = &source
	}
	index := slices.IndexFunc(
		v.round.Cards,
		func(card FinalCard) bool { return card.TeamID == out.TeamID },
	)
	if index >= 0 {
		card := v.round.Cards[index]
		out.CardVersion = card.Version
		out.CardState = card.State
		out.CardSourceCurrent = s.cardCurrent(ctx, tx, eventID, card, team)
	}
	if !writable(v) {
		return out
	}
	out.CanDecideEligibility = actor == v.round.Panel.OrganiserID && out.Outcome == "frozen" &&
		team.freeze.Status != "not_applicable" &&
		team.freeze.Status != "completed"
	if !v.organiser || team.source == nil || team.source.ModeratorID != actor || !out.Eligible ||
		len(out.Blockers) > 0 {
		return out
	}
	if index < 0 {
		out.CanSaveCard = true
		return out
	}
	card := v.round.Cards[index]
	if !out.CardSourceCurrent || card.State != "draft" {
		return out
	}
	out.CanSaveCard = true
	out.CanSignCard = complete(card, v.criteria.Categories)
	return out
}

func (s Service) tieActions(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	v scope,
	fresh Proposal,
	teams []nativeTeam,
) []TieAction {
	out := []TieAction{}
	if !writable(v) || v.round.Proposal == nil || v.round.Proposal.Binding != fresh.Binding ||
		!unresolvedOnly(fresh) {
		return out
	}
	for _, tie := range fresh.Ties {
		seat := nextSeat(v.round, tie)
		if seat > tie.Seats {
			continue
		}
		selected := []uuid.UUID{}
		for _, decision := range v.round.Decisions {
			if decision.TieID == tie.ID {
				selected = append(selected, decision.TeamID)
			}
		}
		remaining := without(tie.TeamIDs, selected)
		hasVoted := slices.ContainsFunc(
			v.round.Votes,
			func(vote Vote) bool { return vote.TieID == tie.ID && vote.Seat == seat && vote.ActorID == actor },
		)
		choices, valid := s.ballots(ctx, tx, eventID, v, tie, seat, remaining, teams)
		if choices == nil {
			choices = []uuid.UUID{}
		}
		out = append(
			out,
			TieAction{
				TieID:            tie.ID,
				Seat:             seat,
				CandidateTeamIDs: remaining,
				CanVote:          slices.Contains(v.round.Panel.MentorIDs, actor) && !hasVoted,
				HasVoted:         hasVoted,
				CanResolve:       actor == v.round.Panel.OrganiserID && valid && len(choices) > 0,
				ResolveTeamIDs:   choices,
				Casting:          valid && len(choices) > 1,
				RequiredVotes:    len(v.round.Panel.MentorIDs),
				VotesComplete:    valid,
			},
		)
	}
	return out
}

func (s Service) cohortReadable(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	v scope,
	teams []nativeTeam,
) (bool, error) {
	if !panelReader(actor, v.round) {
		return true, nil
	}
	for _, team := range teams {
		if !team.status.Eligible {
			continue
		}
		err := mentors.AuthorizeQualificationPanelInTransaction(
			ctx,
			tx,
			actor,
			eventID,
			team.status.TeamID,
		)
		if privateDenial(err) {
			return false, nil
		}
		if err != nil {
			return false, failure("current cohort reader", err)
		}
	}
	return true, nil
}
