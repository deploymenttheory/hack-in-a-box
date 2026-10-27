package access_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestPublishedMembershipWithoutClaimDoesNotInventCompetitionOrLoseOrganiser(t *testing.T) {
	pool := testdb.New(t, true)
	owner := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	team := uuid.New()
	if _, err := pool.Exec(
		t.Context(),
		"INSERT INTO hack_teams(event_id,id,name,state)VALUES($1,$2,'Synthetic native scope','published')",
		eventID,
		team,
	); err != nil {
		t.Fatal(err)
	}
	q := dbgen.New(pool)
	if err := q.CreateCompetitionEntry(
		t.Context(),
		dbgen.CreateCompetitionEntryParams{EventID: eventID, TeamID: team},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(
		t.Context(),
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot)VALUES($1,$2,$3,'published')",
		eventID,
		team,
		owner.ID,
	); err != nil {
		t.Fatal(err)
	}
	current, err := access.TeamPermissions(t.Context(), q, owner.ID, eventID, team)
	if err != nil || !current.Active || !current.Organiser || current.Member {
		t.Fatal("membership without a claim changed current native authority", current, err)
	}
}

func TestContentReaderUsesCurrentActivePersonRolesAndClaim(t *testing.T) {
	pool := testdb.New(t, true)
	owner, mentor := testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	q := dbgen.New(pool)
	if _, err := pool.Exec(
		t.Context(),
		"INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'mentor')",
		eventID,
		mentor.ID,
	); err != nil {
		t.Fatal(err)
	}
	check := func(wantRole string, wantConfirmed bool) {
		t.Helper()
		roles, confirmed, err := access.ContentReader(t.Context(), q, mentor.ID, eventID)
		if err != nil || confirmed != wantConfirmed || wantRole == "" && len(roles) != 0 ||
			wantRole != "" && (len(roles) != 1 || roles[0] != wantRole) {
			t.Fatal("current content role/claim changed", roles, confirmed, err)
		}
	}
	check("mentor", false)
	if _, err := pool.Exec(
		t.Context(),
		"INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')",
		eventID,
		mentor.ID,
	); err != nil {
		t.Fatal(err)
	}
	check("mentor", true)
	if _, err := pool.Exec(
		t.Context(),
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		eventID,
		mentor.ID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(
		t.Context(),
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2",
		eventID,
		mentor.ID,
	); err != nil {
		t.Fatal(err)
	}
	check("", false)
	if _, err := pool.Exec(
		t.Context(),
		"UPDATE people SET active=false WHERE id=$1",
		mentor.ID,
	); err != nil {
		t.Fatal(err)
	}
	if _, _, err := access.ContentReader(
		t.Context(),
		q,
		mentor.ID,
		eventID,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("inactive person retained content authority", err)
	}
	if _, _, err := access.ContentReader(
		t.Context(),
		q,
		uuid.New(),
		eventID,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("missing person retained content authority", err)
	}
}
