package qualification

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
)

func TestTiedCutoffAndCategoriesRequireEveryMentorBallotAndRecordedCastingDecision(t *testing.T) {
	values := [][]int{
		{5, 5, 5, 5, 5},
		{5, 5, 5, 5, 5},
		{5, 5, 5, 5, 5},
		{5, 5, 5, 5, 5},
		{5, 5, 5, 5, 5},
	}
	f, s, cp, p := signedQualificationCohort(t, values)
	run := cp.Definition.RunID
	if p.Round.Proposal == nil || len(p.Round.Proposal.Ties) != 6 ||
		len(p.Round.Proposal.FinalistIDs) != 0 {
		t.Fatal("ties prematurely selected winners", p.Round.Proposal)
	}
	change := func(actor uuid.UUID, c Command) error {
		t.Helper()
		c.Version = p.Round.Version
		c.Reason = "Reviewed exact frozen final evidence for tied selection"
		next, err := s.Change(t.Context(), actor, f.event, run, f.revision(), c)
		if err == nil {
			p = next
		}
		return err
	}
	if err := change(f.owner.ID, Command{Action: "sign_off"}); !errors.Is(err, access.ErrConflict) {
		t.Fatal("unresolved tied cutoff signed off", err)
	}
	for _, tie := range p.Round.Proposal.Ties {
		if tie.Seats == 0 {
			continue
		}
		selected := []uuid.UUID{}
		for seat := 1; seat <= tie.Seats; seat++ {
			remaining := without(tie.TeamIDs, selected)
			first, second := remaining[0], remaining[0]
			if len(remaining) > 1 {
				second = remaining[1]
			}
			evidence := []scorecards.EvidenceReference{
				{Kind: "note", Note: "Explicit tied frozen-final evidence review"},
			}
			resolve := Command{
				Action:   "resolve_tie",
				TieID:    tie.ID,
				Seat:     seat,
				TeamID:   first,
				Evidence: evidence,
			}
			if err := change(f.owner.ID, resolve); !errors.Is(err, access.ErrConflict) {
				t.Fatal("tie resolved without complete native mentor ballots", err)
			}
			for index, mentor := range []uuid.UUID{f.primary.ID, f.second.ID} {
				choice := first
				if index == 1 {
					choice = second
				}
				vote := Command{
					Action:   "vote_tie",
					TieID:    tie.ID,
					Seat:     seat,
					TeamID:   choice,
					Evidence: evidence,
				}
				if err := change(mentor, vote); err != nil {
					t.Fatal("native mentor vote", err)
				}
				if err := change(mentor, vote); !errors.Is(err, access.ErrConflict) {
					t.Fatal("duplicate ballot changed tied selection", err)
				}
			}
			if err := change(f.primary.ID, resolve); !errors.Is(err, access.ErrForbidden) {
				t.Fatal("mentor cast organiser decision", err)
			}
			if err := change(f.owner.ID, resolve); err != nil {
				t.Fatal("reasoned casting resolution", err)
			}
			decision := p.Round.Decisions[len(p.Round.Decisions)-1]
			if decision.Casting != (len(remaining) > 1) || decision.TeamID != first ||
				len(decision.Evidence) != 1 {
				t.Fatal("casting evidence not retained", decision)
			}
			selected = append(selected, first)
		}
	}
	if len(p.Round.Proposal.Blockers) != 0 || len(p.Round.Proposal.FinalistIDs) != 3 ||
		len(p.Round.Proposal.ReserveIDs) != 1 {
		t.Fatal("recorded ties did not resolve configured selection", p.Round.Proposal)
	}
	if err := change(f.owner.ID, Command{Action: "sign_off"}); err != nil || !p.Current {
		t.Fatal("resolved exact panel selection not current", err, p)
	}
}
