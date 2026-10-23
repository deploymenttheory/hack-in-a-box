package access_test

import (
	"errors"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestReviewCoverageAuthorityUsesCurrentRolesWithoutImplicitPlatformGrant(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	owner, mentor, other, admin := testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	if _, err := q.BootstrapAdmin(ctx, admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mentor'),($1,$3,'photographer')`, eventID, mentor.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		person             dbgen.Person
		allowed, organiser bool
	}{{owner, true, true}, {mentor, true, false}, {other, false, false}, {admin, false, false}} {
		organiser, err := access.ReviewCoverageAuthority(ctx, q, row.person.ID, eventID)
		if (err == nil) != row.allowed || organiser != row.organiser {
			t.Fatalf("current authority allowed=%t organiser=%t: %v", row.allowed, organiser, err)
		}
		if !row.allowed && !errors.Is(err, access.ErrForbidden) {
			t.Fatal(err)
		}
	}
	// An unrelated current role retains EventRead after the mentor grant ends.
	if _, err := pool.Exec(ctx, `INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'photographer')`, eventID, mentor.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mentor'`, eventID, mentor.ID); err != nil {
		t.Fatal(err)
	}
	if err := (access.EventPolicy{Queries: q}).Authorize(ctx, mentor.ID, eventID, eventID, access.EventRead); err != nil {
		t.Fatal("retained unrelated role lost event read", err)
	}
	if _, err := access.ReviewCoverageAuthority(ctx, q, mentor.ID, eventID); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("revoked mentor retained review coverage", err)
	}
}
