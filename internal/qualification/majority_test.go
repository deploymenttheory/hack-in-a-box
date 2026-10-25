package qualification

import (
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

// The results owner's shared qualification contract requires panel majority;
// a unique plurality must not masquerade as either majority or a casting tie.
func TestFiveMentorPluralityCannotResolveQualificationAsPanelMajority(t *testing.T) {
	f, s, cp, p := signedQualificationCohort(
		t,
		[][]int{
			{10, 10, 10, 10, 10},
			{9, 9, 9, 9, 9},
			{8, 8, 8, 8, 8},
			{8, 8, 8, 8, 8},
			{8, 8, 8, 8, 8},
			{8, 8, 8, 8, 8},
		},
	)
	panel := Panel{MentorIDs: []uuid.UUID{f.primary.ID, f.second.ID}, OrganiserID: f.owner.ID}
	for range 3 {
		mentor := testdb.Person(t, f.pool)
		f.exec(
			"INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'mentor')",
			f.event,
			mentor.ID,
		)
		panel.MentorIDs = append(panel.MentorIDs, mentor.ID)
	}
	change := func(actor uuid.UUID, c Command) (Page, error) {
		t.Helper()
		c.RequestID = uuid.New()
		c.Version = p.Round.Version
		c.Reason = "Explicit current panel evidence review of tied frozen finals"
		next, err := s.Change(t.Context(), actor, f.event, cp.Definition.RunID, f.revision(), c)
		if err == nil {
			p = next
		}
		return next, err
	}
	if _, err := change(f.owner.ID, Command{Action: "configure_panel", Panel: &panel}); err != nil {
		t.Fatal(err)
	}
	if _, err := change(f.owner.ID, Command{Action: "propose"}); err != nil {
		t.Fatal(err)
	}
	at := slices.IndexFunc(
		p.Round.Proposal.Ties,
		func(tie Tie) bool { return tie.Kind == "total" && tie.Seats > 0 },
	)
	if at < 0 {
		t.Fatal("missing affected native cutoff tie", p.Round.Proposal)
	}
	tie := p.Round.Proposal.Ties[at]
	if len(tie.TeamIDs) != 4 || len(panel.MentorIDs) != 5 {
		t.Fatal("fixture does not exercise four candidates and five mentors", tie, panel)
	}
	evidence := []scorecards.EvidenceReference{
		{Kind: "note", Note: "Explicit evidence reviewed for this exact frozen tied final"},
	}
	for index, mentor := range panel.MentorIDs {
		choice := 0
		if index > 1 {
			choice = index - 1
		}
		if _, err := change(
			mentor,
			Command{
				Action:   "vote_tie",
				TieID:    tie.ID,
				Seat:     1,
				TeamID:   tie.TeamIDs[choice],
				Evidence: evidence,
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	if len(p.Round.Votes) != 5 {
		t.Fatal("not all actual native ballots were retained", p.Round.Votes)
	}
	before := p.Round.Version
	accepted, err := change(
		f.owner.ID,
		Command{
			Action:   "resolve_tie",
			TieID:    tie.ID,
			Seat:     1,
			TeamID:   tie.TeamIDs[0],
			Evidence: evidence,
		},
	)
	if !errors.Is(err, access.ErrConflict) {
		t.Fatal(
			"2/1/1/1 plurality resolved as a majority without a casting tie",
			err,
			accepted.Round.Decisions,
		)
	}
	current, err := s.Read(t.Context(), f.owner.ID, f.event, cp.Definition.RunID)
	if err != nil || current.Round.Version != before || len(current.Round.Decisions) != 0 ||
		len(current.Round.Votes) != 5 ||
		current.Current ||
		len(current.Round.Proposal.FinalistIDs) != 0 {
		t.Fatal(
			"rejected plurality changed the private selection or lost recorded evidence",
			current,
			err,
		)
	}
	if len(current.TieActions) != 1 || !current.TieActions[0].VotesComplete ||
		current.TieActions[0].CanResolve ||
		current.TieActions[0].Casting ||
		len(current.TieActions[0].ResolveTeamIDs) != 0 {
		t.Fatal(
			"complete submajority ballots advertised a decision or missing participation",
			current.TieActions,
		)
	}
	// A reviewed new panel/proposal starts another ballot record; prior votes
	// remain historical. A true majority and existing casting rule still work.
	for _, scenario := range []struct {
		choices []int
		casting bool
	}{
		{[]int{0, 0, 0, 1, 2}, false},
		{[]int{0, 0, 1, 1, 2}, true},
	} {
		if _, err = change(
			f.owner.ID,
			Command{Action: "configure_panel", Panel: &panel},
		); err != nil {
			t.Fatal(err)
		}
		if _, err = change(f.owner.ID, Command{Action: "propose"}); err != nil {
			t.Fatal(err)
		}
		for index, mentor := range panel.MentorIDs {
			if _, err = change(
				mentor,
				Command{
					Action:   "vote_tie",
					TieID:    tie.ID,
					Seat:     1,
					TeamID:   tie.TeamIDs[scenario.choices[index]],
					Evidence: evidence,
				},
			); err != nil {
				t.Fatal(err)
			}
		}
		current, err = s.Read(t.Context(), f.owner.ID, f.event, cp.Definition.RunID)
		if err != nil || len(current.TieActions) != 1 || !current.TieActions[0].CanResolve ||
			!current.TieActions[0].VotesComplete ||
			current.TieActions[0].Casting != scenario.casting {
			t.Fatal("majority/casting readiness was lost", current.TieActions, err)
		}
		accepted, err = change(
			f.owner.ID,
			Command{
				Action:   "resolve_tie",
				TieID:    tie.ID,
				Seat:     1,
				TeamID:   tie.TeamIDs[0],
				Evidence: evidence,
			},
		)
		if err != nil || len(accepted.Round.Decisions) != 1 ||
			accepted.Round.Decisions[0].Casting != scenario.casting {
			t.Fatal(
				"true majority or existing tied-leader casting could not be recorded",
				accepted.Round.Decisions,
				err,
			)
		}
	}
}
