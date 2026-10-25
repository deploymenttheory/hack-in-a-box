package qualification

import (
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestDiscoveryAndActionProjectionUseActualCurrentRunAndRecordedPanel(t *testing.T) {
	f, s, run := receiptFixture(t)
	owner, err := s.Rounds(t.Context(), f.owner.ID, f.event)
	if err != nil || len(owner.Items) != 1 || owner.Items[0].RunID != run ||
		!owner.Items[0].CanConfigurePanel {
		t.Fatal("organiser cannot discover actual unconfigured native run", owner, err)
	}
	mentor, err := s.Rounds(t.Context(), f.primary.ID, f.event)
	if err != nil || len(mentor.Items) != 0 {
		t.Fatal("unrecorded mentor discovered a private panel", mentor, err)
	}
	initial, err := s.Read(t.Context(), f.owner.ID, f.event, run)
	if err != nil || !initial.Capabilities.CanConfigurePanel || initial.Capabilities.CanReadPanel ||
		initial.Capabilities.CanSignOff {
		t.Fatal("initial authority was inferred", initial.Capabilities, err)
	}
	page, err := s.Change(t.Context(), f.owner.ID, f.event, run, f.revision(), panelCommand(f))
	if err != nil {
		t.Fatal(err)
	}
	mentor, err = s.Rounds(t.Context(), f.primary.ID, f.event)
	if err != nil || len(mentor.Items) != 1 || mentor.Items[0].CanConfigurePanel {
		t.Fatal("actual named panel mentor discovery", mentor, err)
	}
	reader, err := s.Read(t.Context(), f.primary.ID, f.event, run)
	if err != nil || !reader.Capabilities.CanReadPanel || reader.Capabilities.CanConfigurePanel ||
		reader.Capabilities.CanPropose ||
		reader.Capabilities.CanSignOff {
		t.Fatal("mentor gained organiser actions", reader.Capabilities, err)
	}
	if !page.Capabilities.CanReadPanel || !page.Capabilities.CanPropose ||
		page.Capabilities.CanSignOff ||
		page.Teams[0].CanSaveCard ||
		page.Teams[0].CanSignCard ||
		page.Teams[0].Source != nil {
		t.Fatal("missing canonical source offered a moderated write", page.Capabilities, page.Teams)
	}
	outsider := testdb.Person(t, f.pool)
	f.exec(
		"INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'mentor')",
		f.event,
		outsider.ID,
	)
	unknown, err := s.Rounds(t.Context(), outsider.ID, f.event)
	if err != nil || len(unknown.Items) != 0 {
		t.Fatal("unassigned outsider acquired panel discovery", unknown, err)
	}
	f.exec("UPDATE events SET writes_allowed=false WHERE id=$1", f.event)
	readonly, err := s.Read(t.Context(), f.owner.ID, f.event, run)
	if err != nil || !readonly.Capabilities.ReadOnly || readonly.Capabilities.CanConfigurePanel ||
		readonly.Capabilities.CanPropose ||
		readonly.Capabilities.CanSignOff {
		t.Fatal("read-only native state offered writes", readonly.Capabilities, err)
	}
	for _, team := range readonly.Teams {
		if team.CanSaveCard || team.CanSignCard || team.CanDecideEligibility {
			t.Fatal("read-only team action", team)
		}
	}
}

func TestTieReadinessProjectsNativeRequiredVotesAndCastingChoices(t *testing.T) {
	f, s, cp, p := signedQualificationCohort(
		t,
		[][]int{
			{10, 9, 10, 9, 9},
			{9, 8, 9, 8, 8},
			{8, 7, 8, 7, 7},
			{8, 7, 8, 7, 7},
			{1, 10, 1, 1, 1},
		},
	)
	if len(p.Round.Proposal.Ties) != 1 || len(p.TieActions) != 1 {
		t.Fatal(
			"fixture must contain exactly the affected total tie",
			p.Round.Proposal,
			p.TieActions,
		)
	}
	tie := p.TieActions[0]
	if tie.CanVote || tie.CanResolve || tie.VotesComplete || tie.RequiredVotes != 2 ||
		len(tie.CandidateTeamIDs) != 2 ||
		len(tie.ResolveTeamIDs) != 0 {
		t.Fatal("organiser tie readiness fabricated authority or votes", tie)
	}
	for index, actor := range []uuid.UUID{f.primary.ID, f.second.ID} {
		before, err := s.Read(t.Context(), actor, f.event, cp.Definition.RunID)
		if err != nil || !before.TieActions[0].CanVote || before.TieActions[0].HasVoted {
			t.Fatal("native mentor ballot not offered", before.TieActions, err)
		}
		p, err = s.Change(
			t.Context(),
			actor,
			f.event,
			cp.Definition.RunID,
			f.revision(),
			Command{
				RequestID: uuid.New(),
				Version:   p.Round.Version,
				Action:    "vote_tie",
				TieID:     tie.TieID,
				Seat:      tie.Seat,
				TeamID:    tie.CandidateTeamIDs[index],
				Reason:    "Explicit native tie evidence review",
				Evidence: []scorecards.EvidenceReference{
					{Kind: "note", Note: "Reviewed exact tied frozen-final evidence"},
				},
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.TieActions) != 1 || p.TieActions[0].CanVote || !p.TieActions[0].HasVoted ||
			p.TieActions[0].CanResolve {
			t.Fatal("recorded mentor vote became a duplicate or organiser grant", p.TieActions)
		}
	}
	current, err := s.Read(t.Context(), f.owner.ID, f.event, cp.Definition.RunID)
	if err != nil || len(current.TieActions) != 1 || !current.TieActions[0].CanResolve ||
		!current.TieActions[0].Casting ||
		!current.TieActions[0].VotesComplete ||
		len(current.TieActions[0].ResolveTeamIDs) != 2 {
		t.Fatal("native casting choices were not projected", current.TieActions, err)
	}
}
