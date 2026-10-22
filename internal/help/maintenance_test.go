package help

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestMaintenanceIncludesClaimedRequestsAfterHelperDeparture(t *testing.T) {
	f := newFixture(t)
	claimed := f.change(f.helper.ID, f.create("competition_output"), "claim")
	f.exec(
		`UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mentor'`,
		f.eventID,
		f.helper.ID,
	)
	if err := f.s.Maintain(t.Context()); err != nil {
		t.Fatal(err)
	}
	current, err := f.s.Read(t.Context(), f.member.ID, f.eventID, claimed.Request.ID)
	if err != nil || current.Request.Status != "waiting" || current.Request.AssigneeID != nil ||
		current.Request.Revision != claimed.Request.Revision+1 {
		t.Fatalf("native maintenance missed departed claimed helper: %+v %v", current, err)
	}
	if len(current.History) != 3 || current.History[2].Action != "assignee_unavailable" {
		t.Fatalf("missing actual native transition: %+v", current.History)
	}
}

func TestMaintenanceTraversesEveryEventBeyondFirstKeysetPage(t *testing.T) {
	f := newFixture(t)
	for range 53 {
		eventID := testdb.Event(t, f.pool, f.owner.ID)
		team := uuid.New()
		f.exec(
			`INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')`,
			eventID,
			f.member.ID,
		)
		f.exec(
			`INSERT INTO hack_teams(event_id,id,name,state)VALUES($1,$2,'Synthetic team','published')`,
			eventID,
			team,
		)
		f.exec(
			`INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot)VALUES($1,$2,$3,'published')`,
			eventID,
			team,
			f.member.ID,
		)
		if _, err := f.s.Create(
			t.Context(),
			f.member.ID,
			eventID,
			uuid.New(),
			Input{
				TeamID:      team,
				Category:    "setup",
				Description: "Synthetic setup issue",
				Location:    "Synthetic room",
				CommandID:   uuid.New(),
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	f.now = f.now.Add(16 * time.Minute)
	for range 2 {
		if err := f.s.Maintain(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM help_transitions WHERE action='overdue'`).
		Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 53 {
		t.Fatalf("event keyset skipped or duplicated native due sources: %d", count)
	}
}
