package archives

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/attendance"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestOperationalArchiveReadsRespectRetentionBeforeWorkerDeletion(t *testing.T) {
	pool := testdb.New(t, true)
	owner, logistic, person := testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	)
	for _, days := range []int32{90, 1} {
		t.Run(fmt.Sprintf("retention_%ddays", days), func(t *testing.T) {
			id := testdb.Event(t, pool, owner.ID)
			q := dbgen.New(pool)
			execute(
				t,
				pool,
				"INSERT INTO event_memberships(event_id,person_id,role,active)VALUES($1,$2,'logistics',true)",
				id,
				logistic.ID,
			)
			execute(
				t,
				pool,
				"INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')",
				id,
				person.ID,
			)
			execute(
				t,
				pool,
				"INSERT INTO operational_profiles(id,event_id,person_id,contact_email)VALUES($1,$2,$3,'EXPIRED-CANARY@example.org')",
				uuid.New(),
				id,
				person.ID,
			)
			policy := privacy.Service{Pool: pool}
			// The shorter setting is changed through the real policy command before
			// archival; no archive role/write exception is invented for this test.
			row, err := q.Event(t.Context(), id)
			if err != nil {
				t.Fatal(err)
			}
			if err = policy.Configure(
				t.Context(),
				owner.ID,
				id,
				row.Revision,
				privacy.Settings{RetentionDays: days, DraftDays: 7},
			); err != nil {
				t.Fatal(err)
			}
			if err = q.InitializeEventFacts(t.Context(), id); err != nil {
				t.Fatal(err)
			}
			row, err = q.Event(t.Context(), id)
			if err != nil {
				t.Fatal(err)
			}
			lifecycle := event.Service{Pool: pool}
			cancelled, err := lifecycle.Transition(
				t.Context(),
				owner.ID,
				id,
				row.Revision,
				"cancelled",
				"Synthetic never-held archive",
				false,
			)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = lifecycle.Transition(
				t.Context(),
				owner.ID,
				id,
				cancelled.Revision,
				"archived",
				"",
				false,
			); err != nil {
				t.Fatal(err)
			}
			row, err = q.Event(t.Context(), id)
			if err != nil {
				t.Fatal(err)
			}
			deadline := privacy.RetentionDeadline(row.CompletedAt.Time, days)
			now := deadline.Add(-time.Nanosecond)
			native := attendance.Service{Pool: pool, Now: func() time.Time { return now }}
			policy.Clock = func() time.Time { return now }
			archive := Service{Pool: pool, Clock: func() time.Time { return now }}
			if page, e := native.Logistics(
				t.Context(),
				logistic.ID,
				id,
				uuid.Nil,
				10,
				true,
			); e != nil ||
				len(page.Items) == 0 {
				t.Fatal("before deadline missing native fields", page, e)
			}
			if _, e := policy.Read(t.Context(), person.ID, id, person.ID); e != nil {
				t.Fatal("before deadline own profile", e)
			}
			if _, e := policy.Export(t.Context(), logistic.ID, id, uuid.Nil, 10); e != nil {
				t.Fatal("before deadline export", e)
			}
			now = deadline
			if _, e := native.Logistics(
				t.Context(),
				logistic.ID,
				id,
				uuid.Nil,
				10,
				true,
			); !errors.Is(
				e,
				access.ErrNotFound,
			) {
				t.Fatal("expired native logistics remained readable", e)
			}
			if _, e := policy.Export(
				t.Context(),
				logistic.ID,
				id,
				uuid.Nil,
				10,
			); !errors.Is(
				e,
				access.ErrNotFound,
			) {
				t.Fatal("expired operational export remained readable", e)
			}
			if _, e := policy.Read(
				t.Context(),
				person.ID,
				id,
				person.ID,
			); !errors.Is(
				e,
				access.ErrNotFound,
			) {
				t.Fatal("expired own operational profile remained readable", e)
			}
			exported, e := archive.Export(
				t.Context(),
				logistic.ID,
				id,
				ExportRequest{Source: "roster", Limit: 10},
			)
			if e != nil || exported.SourceState != "retention_expired" || exported.Roster != nil {
				t.Fatal("archive hid expiry or exposed expired fields", exported, e)
			}
			var count int
			if e = pool.QueryRow(t.Context(), "SELECT count(*) FROM operational_profiles WHERE event_id=$1", id).
				Scan(&count); e != nil ||
				count != 1 {
				t.Fatal("read proof silently depended on worker deletion", count, e)
			}
		})
	}
}
