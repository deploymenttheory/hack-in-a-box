package permissions_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/permissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestGrantHistoryEligibilityAndConcurrentRegistration(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	s := permissions.Service{Pool: pool}
	owner := testdb.Person(t, pool)
	event := testdb.Event(t, pool, owner.ID)
	other := testdb.Event(t, pool, owner.ID)
	registration := func(person, event uuid.UUID, state string) error {
		return database.Within(ctx, pool, func(_ pgx.Tx, q *dbgen.Queries) error {
			return permissions.SetRegistrationState(ctx, q, event, person, state)
		})
	}
	change := func(person uuid.UUID, role string, active bool) error {
		e, err := q.Event(ctx, event)
		if err != nil {
			return err
		}
		_, err = s.Change(
			ctx,
			permissions.Change{
				Actor:                             owner.ID,
				Event:                             event,
				Person:                            person,
				Role:                              role,
				Active:                            active,
				AcknowledgeCompetitionRestriction: true,
				Revision:                          e.Revision,
			},
		)
		return err
	}
	for _, role := range []string{"organiser", "early_brief_access"} {
		p := testdb.Person(t, pool)
		if err := registration(p.ID, event, "waitlisted"); err != nil {
			t.Fatal(err)
		}
		if err := change(p.ID, role, true); !errors.Is(err, permissions.ErrWithdrawal) {
			t.Fatalf("waitlisted grant: %v", err)
		}
		if err := registration(p.ID, event, "withdrawn"); err != nil {
			t.Fatal(err)
		}
		if err := change(p.ID, role, true); err != nil {
			t.Fatal(err)
		}
		if err := change(p.ID, role, false); err != nil {
			t.Fatal(err)
		}
		if err := registration(
			p.ID,
			event,
			"received",
		); !errors.Is(
			err,
			permissions.ErrIneligible,
		) {
			t.Fatalf("revocation restored eligibility: %v", err)
		}
		if err := registration(p.ID, other, "confirmed"); err != nil {
			t.Fatalf("exclusion crossed events: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, "DELETE FROM competition_exclusions"); err == nil {
		t.Fatal("lasting exclusion could be deleted")
	}
	if err := change(owner.ID, "organiser", false); !errors.Is(err, permissions.ErrLastOrganiser) {
		t.Fatalf("removed owner: %v", err)
	}
	for range 10 {
		person := testdb.Person(t, pool)
		start := make(chan struct{})
		results := make(chan error, 2)
		var wg sync.WaitGroup
		wg.Go(func() { <-start; results <- registration(person.ID, event, "waitlisted") })
		wg.Go(func() { <-start; results <- change(person.ID, "early_brief_access", true) })
		close(start)
		wg.Wait()
		close(results)
		success := 0
		for err := range results {
			if err == nil {
				success++
			} else if !errors.Is(err, permissions.ErrWithdrawal) && !errors.Is(err, permissions.ErrIneligible) {
				t.Fatal(err)
			}
		}
		if success != 1 {
			t.Fatalf("registration/grant race admitted %d changes", success)
		}
	}
	stale := permissions.Change{
		Actor:    owner.ID,
		Event:    event,
		Person:   owner.ID,
		Role:     "logistics",
		Active:   true,
		Revision: 1,
	}
	if _, err := s.Change(ctx, stale); !errors.Is(err, access.ErrConflict) {
		t.Fatalf("stale grant accepted: %v", err)
	}
}

func TestRoleResourceMatrixAndImmediateRevocation(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	policy := access.EventPolicy{Queries: q}
	owner := testdb.Person(t, pool)
	event := testdb.Event(t, pool, owner.ID)
	other := testdb.Event(t, pool, owner.ID)
	admin := testdb.Person(t, pool)
	q.BootstrapAdmin(ctx, admin.ID)
	cases := []struct {
		role, kind string
		action     access.Action
		assigned   bool
	}{
		{"mentor", "team", access.TeamRead, true},
		{"mentor", "assessment", access.AssessmentRead, true},
		{"judge", "assessment", access.AssessmentRead, true},
		{"mc", "presentation", access.PresentationRead, false},
		{"logistics", "logistics", access.LogisticsRead, false},
		{"photographer", "media", access.MediaRead, false},
		{"early_brief_access", "brief", access.BriefRead, false},
	}
	for _, tc := range cases {
		t.Run(tc.role+tc.kind, func(t *testing.T) {
			p := testdb.Person(t, pool)
			resource := uuid.New()
			if err := q.SetMembership(
				ctx,
				dbgen.SetMembershipParams{
					EventID:  event,
					PersonID: p.ID,
					Role:     tc.role,
					Active:   true,
				},
			); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(
				ctx,
				"INSERT INTO policy_resources(event_id,id,kind) VALUES ($1,$2,$3)",
				event,
				resource,
				tc.kind,
			); err != nil {
				t.Fatal(err)
			}
			if tc.assigned {
				if err := policy.Authorize(
					ctx,
					p.ID,
					event,
					resource,
					tc.action,
				); !errors.Is(
					err,
					access.ErrForbidden,
				) {
					t.Fatalf("unassigned access: %v", err)
				}
				if _, err := pool.Exec(
					ctx,
					"INSERT INTO resource_assignments VALUES ($1,$2,$3)",
					event,
					resource,
					p.ID,
				); err != nil {
					t.Fatal(err)
				}
			}
			if err := policy.Authorize(ctx, p.ID, event, resource, tc.action); err != nil {
				t.Fatal(err)
			}
			if err := policy.Authorize(ctx, p.ID, other, resource, tc.action); err == nil {
				t.Fatal("cross-event resource access")
			}
			if err := policy.Authorize(ctx, admin.ID, event, resource, tc.action); err == nil {
				t.Fatal("platform admin implicitly saw private resource")
			}
			q.SetMembership(
				ctx,
				dbgen.SetMembershipParams{
					EventID:  event,
					PersonID: p.ID,
					Role:     tc.role,
					Active:   false,
				},
			)
			if err := policy.Authorize(ctx, p.ID, event, resource, tc.action); err == nil {
				t.Fatal("revoked grant retained access")
			}
		})
	}
}
