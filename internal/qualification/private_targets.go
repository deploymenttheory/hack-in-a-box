package qualification

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
)

// Exclusion is a selection fact, never permission to read private prose about
// that team. Project every retained target under current native reader scope,
// copying slices so history and another recipient keep their original record.
func (s Service) projectRoundTargets(
	ctx context.Context,
	tx pgx.Tx,
	reader, eventID uuid.UUID,
	round *Round,
) error {
	denied := map[uuid.UUID]bool{}
	for _, card := range round.Cards {
		denied[card.TeamID] = false
	}
	for _, decision := range round.Eligibility {
		denied[decision.TeamID] = false
	}
	for _, vote := range round.Votes {
		denied[vote.TeamID] = false
	}
	for _, decision := range round.Decisions {
		denied[decision.TeamID] = false
	}
	withheld := false
	for teamID := range denied {
		err := mentors.AuthorizeQualificationPanelInTransaction(ctx, tx, reader, eventID, teamID)
		if privateDenial(err) {
			denied[teamID] = true
			withheld = true
		} else if err != nil {
			return failure("private target reader", err)
		}
	}
	if !withheld {
		return nil
	}
	aggregateItems := len(round.Cards) + len(round.Votes) + len(round.Decisions)
	round.Cards = slices.DeleteFunc(
		slices.Clone(round.Cards),
		func(c FinalCard) bool { return denied[c.TeamID] },
	)
	round.Eligibility = slices.DeleteFunc(
		slices.Clone(round.Eligibility),
		func(e Eligibility) bool { return denied[e.TeamID] },
	)
	round.Votes = slices.DeleteFunc(
		slices.Clone(round.Votes),
		func(v Vote) bool { return denied[v.TeamID] },
	)
	round.Decisions = slices.DeleteFunc(
		slices.Clone(round.Decisions),
		func(d TieDecision) bool { return denied[d.TeamID] },
	)
	// Round/signoff prose may refer to a withheld target without a structured ID.
	round.Reason = ""
	if aggregateItems != len(round.Cards)+len(round.Votes)+len(round.Decisions) {
		round.Proposal = nil
		round.Votes = []Vote{}
		round.Decisions = []TieDecision{}
		round.Signoff = nil
	} else if round.Signoff != nil {
		signoff := *round.Signoff
		signoff.Reason = ""
		round.Signoff = &signoff
	}
	return nil
}
