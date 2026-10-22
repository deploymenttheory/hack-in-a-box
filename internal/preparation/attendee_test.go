package preparation

import (
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestAssignedPreparationRequiresCurrentRoleOrConfirmedPlace(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	owner, person := testdb.Person(t, pool), testdb.Person(t, pool)
	id := testdb.Event(t, pool, owner.ID)
	if err := q.SetMembership(
		ctx,
		dbgen.SetMembershipParams{
			EventID:  id,
			PersonID: person.ID,
			Role:     "photographer",
			Active:   true,
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := q.SetCompetitionState(
		ctx,
		dbgen.SetCompetitionStateParams{EventID: id, PersonID: person.ID, State: "confirmed"},
	); err != nil {
		t.Fatal(err)
	}
	s := Service{Pool: pool}
	page, err := s.List(ctx, owner.ID, id, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	u := taskInput("Check development tools", "attendee")
	u.Definition.OwnerID = &person.ID
	page, err = s.Save(ctx, owner.ID, id, page.Revision, u)
	if err != nil {
		t.Fatal(err)
	}
	mine, err := s.List(ctx, person.ID, id, Filter{})
	if err != nil || len(mine.Items) != 1 {
		t.Fatalf("assigned confirmed attendee: %+v %v", mine, err)
	}
	if err := q.SetCompetitionState(
		ctx,
		dbgen.SetCompetitionStateParams{EventID: id, PersonID: person.ID, State: "withdrawn"},
	); err != nil {
		t.Fatal(err)
	}
	mine, err = s.List(ctx, person.ID, id, Filter{})
	if err != nil || len(mine.Items) != 0 {
		t.Fatalf("unrelated staff role retained attendee task: %+v %v", mine, err)
	}
	u.Definition.OwnerRole = "photographer"
	page, err = s.Save(ctx, owner.ID, id, page.Revision, u)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.SetMembership(
		ctx,
		dbgen.SetMembershipParams{
			EventID:  id,
			PersonID: person.ID,
			Role:     "photographer",
			Active:   false,
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := q.SetMembership(
		ctx,
		dbgen.SetMembershipParams{
			EventID:  id,
			PersonID: person.ID,
			Role:     "logistics",
			Active:   true,
		},
	); err != nil {
		t.Fatal(err)
	}
	mine, err = s.List(ctx, person.ID, id, Filter{})
	if err != nil || len(mine.Items) != 0 {
		t.Fatalf("revoked owner role retained access: %+v %v", mine, err)
	}
}
