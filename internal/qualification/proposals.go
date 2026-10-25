package qualification

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
)

func emptyProposal() Proposal {
	return Proposal{
		Rankings:    []Ranking{},
		FinalistIDs: []uuid.UUID{},
		ReserveIDs:  []uuid.UUID{},
		Categories:  []CategoryProposal{},
		Ties:        []Tie{},
		Blockers:    []string{},
	}
}

func (s Service) proposal(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	v scope,
) (Proposal, error) {
	p := emptyProposal()
	teams, err := s.cohort(ctx, tx, eventID, v)
	if err != nil {
		return p, failure("proposals", err)
	}
	signatures := []any{v.reference, v.round.Panel, v.round.Eligibility}
	cards := map[uuid.UUID]FinalCard{}
	if err = validatePanel(ctx, dbgen.New(tx), eventID, v.round.Panel); err != nil {
		p.Blockers = append(p.Blockers, "current_panel_required")
	}
	for _, team := range teams {
		signatures = append(signatures, team.status)
		if team.source != nil {
			signatures = append(signatures, sourceReference(team))
		}
		if !team.status.Eligible {
			if len(team.status.Blockers) > 0 {
				p.Blockers = append(p.Blockers, team.status.Blockers...)
			}
			continue
		}
		p.Blockers = append(p.Blockers, team.status.Blockers...)
		for _, member := range append(slices.Clone(v.round.Panel.MentorIDs), v.round.Panel.OrganiserID) {
			if err := mentors.AuthorizeQualificationPanelInTransaction(
				ctx,
				tx,
				member,
				eventID,
				team.status.TeamID,
			); err != nil {
				p.Blockers = append(p.Blockers, "panel_conflict_or_scope_changed")
			}
		}
		index := slices.IndexFunc(
			v.round.Cards,
			func(c FinalCard) bool { return c.TeamID == team.status.TeamID },
		)
		if index < 0 {
			p.Blockers = append(
				p.Blockers,
				"signed_final_card_required:"+team.status.TeamID.String(),
			)
			continue
		}
		card := v.round.Cards[index]
		signatures = append(signatures, card)
		if card.State != "signed" || !complete(card, v.criteria.Categories) ||
			!s.cardCurrent(ctx, tx, eventID, card, team) {
			p.Blockers = append(
				p.Blockers,
				"current_complete_final_card_required:"+team.status.TeamID.String(),
			)
			continue
		}
		cards[card.TeamID] = card
	}
	p.Binding = digest(signatures)
	if len(p.Blockers) > 0 {
		return p, nil
	}
	s.resolveTotals(ctx, tx, eventID, v, &p, cards)
	s.resolveCategories(ctx, tx, eventID, v, &p, cards)
	if len(p.Blockers) == 0 {
		for index, ranking := range p.Rankings {
			if index < v.criteria.FinalistCount {
				p.FinalistIDs = append(p.FinalistIDs, ranking.TeamID)
			} else if index < v.criteria.FinalistCount+v.criteria.ReserveCount {
				p.ReserveIDs = append(p.ReserveIDs, ranking.TeamID)
			}
		}
		if len(p.FinalistIDs) != v.criteria.FinalistCount ||
			len(p.ReserveIDs) != v.criteria.ReserveCount {
			p.Blockers = append(p.Blockers, "insufficient_eligible_teams")
			p.FinalistIDs = []uuid.UUID{}
			p.ReserveIDs = []uuid.UUID{}
		}
	}
	return p, nil
}

func compareID(a, b uuid.UUID) int {
	if a.String() < b.String() {
		return -1
	}
	if a == b {
		return 0
	}
	return 1
}

func tieID(binding string, tie Tie) string {
	return digest(struct {
		Binding string
		Tie     Tie
	}{binding, tie})
}

func without(ids, exclude []uuid.UUID) []uuid.UUID {
	out := []uuid.UUID{}
	for _, id := range ids {
		if !slices.Contains(exclude, id) {
			out = append(out, id)
		}
	}
	return out
}

func unresolvedOnly(p Proposal) bool {
	for _, block := range p.Blockers {
		if len(block) < len("unresolved_tie:") ||
			block[:len("unresolved_tie:")] != "unresolved_tie:" {
			return false
		}
	}
	return true
}

func signedBinding(p Proposal, r Round) string {
	return digest(struct {
		Proposal  Proposal
		Votes     []Vote
		Decisions []TieDecision
	}{p, r.Votes, r.Decisions})
}

func nextSeat(round Round, tie Tie) int {
	seat := 1
	for _, decision := range round.Decisions {
		if decision.TieID == tie.ID && decision.Seat == seat {
			seat++
		}
	}
	return seat
}

func (s Service) resolveTotals(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	v scope,
	p *Proposal,
	cards map[uuid.UUID]FinalCard,
) {
	for id, card := range cards {
		var total int64
		for _, category := range v.criteria.Categories {
			score := card.Scores[slices.IndexFunc(card.Scores, func(score scorecards.CategoryScore) bool { return score.CategoryID == category.ID })]
			total += int64(*score.Value) * int64(category.Weight)
		}
		p.Rankings = append(p.Rankings, Ranking{TeamID: id, Total: total})
	}
	slices.SortFunc(p.Rankings, func(a, b Ranking) int {
		if a.Total > b.Total {
			return -1
		}
		if a.Total < b.Total {
			return 1
		}
		return compareID(a.TeamID, b.TeamID)
	})
	for start := 0; start < len(p.Rankings); {
		end := start + 1
		for end < len(p.Rankings) && p.Rankings[end].Total == p.Rankings[start].Total {
			end++
		}
		for index := start; index < end; index++ {
			p.Rankings[index].Rank = start + 1
		}
		if end-start > 1 {
			ids := make([]uuid.UUID, 0, end-start)
			for _, r := range p.Rankings[start:end] {
				ids = append(ids, r.TeamID)
			}
			seats := 0
			final := v.criteria.FinalistCount
			selected := final + v.criteria.ReserveCount
			if start < final && end > final {
				seats = final - start
				if selected < end && selected > final {
					seats = selected - start
				}
			} else if start < selected && end > selected {
				seats = selected - start
			}
			tie := Tie{Kind: "total", TeamIDs: ids, Seats: seats}
			tie.ID = tieID(p.Binding, tie)
			p.Ties = append(p.Ties, tie)
			if seats > 0 {
				resolved, valid := s.resolved(ctx, tx, eventID, v, tie)
				if !valid {
					p.Blockers = append(p.Blockers, "unresolved_tie:"+tie.ID)
				} else {
					ordered := append(slices.Clone(resolved), without(ids, resolved)...)
					for index, id := range ordered {
						p.Rankings[start+index].TeamID = id
					}
				}
			}
		}
		start = end
	}
}

func (s Service) resolveCategories(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	v scope,
	p *Proposal,
	cards map[uuid.UUID]FinalCard,
) {
	for _, category := range v.criteria.Categories {
		var max *int
		winners := []uuid.UUID{}
		for id, card := range cards {
			for _, score := range card.Scores {
				if score.CategoryID != category.ID {
					continue
				}
				if max == nil || *score.Value > *max {
					value := *score.Value
					max = &value
					winners = []uuid.UUID{id}
				} else if *score.Value == *max {
					winners = append(winners, id)
				}
			}
		}
		slices.SortFunc(winners, compareID)
		if len(winners) > 1 {
			id := category.ID
			tie := Tie{Kind: "category", CategoryID: &id, TeamIDs: winners, Seats: 1}
			tie.ID = tieID(p.Binding, tie)
			p.Ties = append(p.Ties, tie)
			if resolved, valid := s.resolved(ctx, tx, eventID, v, tie); valid {
				winners = resolved
			} else {
				p.Blockers = append(p.Blockers, "unresolved_tie:"+tie.ID)
			}
		}
		p.Categories = append(
			p.Categories,
			CategoryProposal{CategoryID: category.ID, TeamIDs: winners},
		)
	}
}
