package mentors

import (
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestMentorMigrationPreservesPublishedTeamsAndScopesAssignments(t *testing.T) {
	pool := testdb.New(t, false)
	ctx := t.Context()
	if err := database.Migrate(ctx, pool, 20); err != nil {
		t.Fatal(err)
	}
	owner := testdb.Person(t, pool)
	event := testdb.Event(t, pool, owner.ID)
	other := testdb.Event(t, pool, owner.ID)
	team := uuid.New()
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO hack_teams(id,event_id,name,state,allocation_locked) VALUES($1,$2,'Synthetic published team','published',true)",
		team,
		event,
	); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, pool, 21); err != nil {
		t.Fatal(err)
	}
	var state string
	var locked bool
	if err := pool.QueryRow(ctx, "SELECT state,allocation_locked FROM hack_teams WHERE id=$1", team).
		Scan(&state, &locked); err != nil ||
		state != "published" ||
		!locked {
		t.Fatalf("mentor schema changed competing teams: %s %v %v", state, locked, err)
	}
	_, err := pool.Exec(
		ctx,
		"INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at) VALUES($1,$2,$3,'primary','draft','2027-06-20T09:00Z','2027-06-20T17:00Z')",
		other,
		team,
		owner.ID,
	)
	if err == nil {
		t.Fatal("cross-event assignment accepted")
	}
	_, err = pool.Exec(
		ctx,
		"INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at) VALUES($1,$2,$3,'primary','draft','2027-06-20T09:00Z','2027-06-20T17:00Z')",
		event,
		team,
		owner.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(
		ctx,
		"INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at) VALUES($1,$2,$3,'second','draft','2027-06-20T09:00Z','2027-06-20T17:00Z')",
		event,
		team,
		owner.ID,
	)
	if err == nil {
		t.Fatal("database allowed same primary and second reviewer")
	}
}
