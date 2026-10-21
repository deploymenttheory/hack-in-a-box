package testdb

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// Authorization is a shared two-event fixture. The overlapping person has
// deliberately different grants in each event; callers extend the matrix as
// new APIs, file purposes, exports and subscriptions are introduced.
type Authorization struct {
	Pool          *pgxpool.Pool
	Queries       *dbgen.Queries
	Owner, Person dbgen.Person
	First, Second uuid.UUID
	Policy        access.EventPolicy
}

func NewAuthorization(t *testing.T) Authorization {
	pool := New(t, true)
	q := dbgen.New(pool)
	owner, person := Person(t, pool), Person(t, pool)
	return Authorization{
		Pool:    pool,
		Queries: q,
		Owner:   owner,
		Person:  person,
		First:   Event(t, pool, owner.ID),
		Second:  Event(t, pool, owner.ID),
		Policy:  access.EventPolicy{Queries: q},
	}
}

func (f Authorization) Grant(t *testing.T, event, person uuid.UUID, role string, active bool) {
	t.Helper()
	if err := f.Queries.SetMembership(
		t.Context(),
		dbgen.SetMembershipParams{EventID: event, PersonID: person, Role: role, Active: active},
	); err != nil {
		t.Fatal(err)
	}
}

func (f Authorization) Resource(
	t *testing.T,
	event uuid.UUID,
	kind string,
	assignee uuid.UUID,
) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := f.Pool.Exec(
		t.Context(),
		"INSERT INTO policy_resources(event_id,id,kind) VALUES($1,$2,$3)",
		event,
		id,
		kind,
	); err != nil {
		t.Fatal(err)
	}
	if assignee != uuid.Nil {
		if _, err := f.Pool.Exec(
			t.Context(),
			"INSERT INTO resource_assignments(event_id,resource_id,person_id) VALUES($1,$2,$3)",
			event,
			id,
			assignee,
		); err != nil {
			t.Fatal(err)
		}
	}
	return id
}
