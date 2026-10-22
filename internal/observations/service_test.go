package observations

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type fixture struct {
	t                                        *testing.T
	pool                                     *pgxpool.Pool
	s                                        Service
	owner, primary, second, member, outsider dbgen.Person
	event, team                              uuid.UUID
	c                                        event.Configuration
	now                                      time.Time
}

func setup(t *testing.T) *fixture {
	t.Helper()
	p := testdb.New(t, true)
	f := &fixture{t: t, pool: p, owner: testdb.Person(t, p), primary: testdb.Person(t, p), second: testdb.Person(t, p), member: testdb.Person(t, p), outsider: testdb.Person(t, p), team: uuid.New()}
	f.event = testdb.Event(t, p, f.owner.ID)
	f.c = event.DefaultConfiguration()
	f.c.LiveStart = time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	f.c.LiveEnd = f.c.LiveStart.Add(8 * time.Hour)
	f.c.SubmissionCutoff = f.c.LiveEnd.Add(-time.Hour)
	f.now = f.c.LiveStart
	f.s = Service{Pool: p, Now: func() time.Time { return f.now }}
	configuration, _ := json.Marshal(f.c)
	f.exec("UPDATE events SET configuration=$2,published_at=$3,stage='live' WHERE id=$1", f.event, configuration, f.c.LiveStart.Add(-7*24*time.Hour))
	f.exec("INSERT INTO hack_teams(event_id,id,name,state) VALUES($1,$2,'Synthetic observed team','published')", f.event, f.team)
	f.exec("INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')", f.event, f.member.ID)
	f.exec("INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')", f.event, f.team, f.member.ID)
	for _, person := range []dbgen.Person{f.primary, f.second} {
		f.exec("INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mentor')", f.event, person.ID)
	}
	f.exec("INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at) VALUES($1,$2,$3,'primary','published',$5,$6),($1,$2,$4,'second','published',$5,$6)", f.event, f.team, f.primary.ID, f.second.ID, f.c.LiveStart, f.c.LiveEnd)
	return f
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.t.Context(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) revision() int64 {
	f.t.Helper()
	row, err := dbgen.New(f.pool).Event(f.t.Context(), f.event)
	if err != nil {
		f.t.Fatal(err)
	}
	return row.Revision
}

func input() Command {
	return Command{Action: "observation", ID: uuid.New(), Evidence: Evidence{Category: "technical", Evidence: "Synthetic confidential implementation evidence", Outcome: "Synthetic confidential review outcome", FeedbackReference: "Synthetic private feedback reference"}}
}

func TestObservationsLiveBoundaryAuthorshipAndSeparateOperationalHistory(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	note := input()
	f.now = f.c.LiveStart.Add(-time.Nanosecond)
	if _, err := f.s.Change(ctx, f.primary.ID, f.event, f.team, f.revision(), note); !errors.Is(err, event.ErrBlocked) {
		t.Fatalf("pre-live review accepted: %v", err)
	}
	operational := Command{Action: "status", State: "at_risk", Reason: "Synthetic equipment delay", NextAction: "Request logistics support"}
	page, err := f.s.Change(ctx, f.second.ID, f.event, f.team, f.revision(), operational)
	if err != nil || page.Status == nil || page.Status.State != "at_risk" || page.CanRecord || !page.CanChangeStatus || len(page.Observations) != 0 {
		t.Fatalf("operational support coupled to formal scoring: %v", err)
	}
	f.now = f.c.LiveStart
	page, err = f.s.Change(ctx, f.primary.ID, f.event, f.team, page.Revision, note)
	if err != nil || len(page.Observations) != 1 {
		t.Fatalf("live review denied: %v", err)
	}
	first := page.Observations[0]
	note.Version = first.Version
	note.Evidence.Outcome = "Synthetic amended confidential outcome"
	f.now = f.now.Add(time.Minute)
	page, err = f.s.Change(ctx, f.primary.ID, f.event, f.team, page.Revision, note)
	if err != nil {
		t.Fatal(err)
	}
	updated := page.Observations[0]
	if updated.Version != 2 || updated.AuthorID != first.AuthorID || !updated.ObservedAt.Equal(first.ObservedAt) || !updated.ChangedAt.After(first.ChangedAt) {
		t.Fatal("revision reattributed or replaced original observation")
	}
	if _, err = f.s.Change(ctx, f.second.ID, f.event, f.team, page.Revision, note); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("second mentor rewrote another author: %v", err)
	}
	if _, err = f.s.Change(ctx, f.owner.ID, f.event, f.team, page.Revision, note); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("organiser rewrote private author record: %v", err)
	}
	history, err := f.s.History(ctx, f.second.ID, f.event, f.team, note.ID)
	if err != nil || len(history.Observations) != 2 || len(history.Statuses) != 1 || history.Observations[1].Evidence.Outcome != first.Evidence.Outcome {
		t.Fatalf("private history missing: %v", err)
	}
	if _, err = f.s.Change(ctx, f.primary.ID, f.event, f.team, page.Revision, note); !errors.Is(err, access.ErrConflict) {
		t.Fatalf("stale observation version accepted: %v", err)
	}
	for _, table := range []string{"mentor_observation_versions", "mentor_team_status_versions"} {
		if _, err = f.pool.Exec(ctx, "DELETE FROM "+table+" WHERE event_id=$1", f.event); err == nil {
			t.Fatal("private history could be deleted")
		}
		if _, err = f.pool.Exec(ctx, "TRUNCATE "+table); err == nil {
			t.Fatal("private history could be truncated")
		}
	}
	var auditJSON string
	if err = f.pool.QueryRow(ctx, "SELECT COALESCE(json_agg(a)::text,'') FROM audit_events a WHERE event_id=$1", f.event).Scan(&auditJSON); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(auditJSON, "confidential") || strings.Contains(auditJSON, note.Evidence.FeedbackReference) || strings.Contains(auditJSON, operational.Reason) || strings.Contains(auditJSON, operational.NextAction) {
		t.Fatal("private prose leaked through generic audit")
	}
}

func TestObservationWriteWaitingBehindRevocationRechecksCurrentAuthority(t *testing.T) {
	f := setup(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	revision := f.revision()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	q := dbgen.New(tx)
	if _, err = q.LockEvent(ctx, f.event); err != nil {
		t.Fatal(err)
	}
	var holder int32
	if err = tx.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&holder); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mentor'", f.event, f.primary.ID); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, writeErr := f.s.Change(ctx, f.primary.ID, f.event, f.team, revision, input())
		result <- writeErr
	}()
	// Observe the actual PostgreSQL wait, rather than guessing when the writer
	// has reached the shared event lock.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err = f.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))", holder).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case early := <-result:
			t.Fatalf("writer bypassed event lock: %v", early)
		case <-ctx.Done():
			t.Fatal("writer never reached the event lock")
		case <-ticker.C:
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-result:
		if !errors.Is(err, access.ErrForbidden) {
			t.Fatalf("waiting writer retained revoked authority: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("revoked writer did not finish")
	}
	var count int
	if err = f.pool.QueryRow(ctx, "SELECT count(*) FROM mentor_observation_versions WHERE event_id=$1", f.event).Scan(&count); err != nil || count != 0 {
		t.Fatalf("revoked write retained private history: count=%d err=%v", count, err)
	}
}

func TestObservationReadOnlyEventRetainsHistoryAndDeniesBothWrites(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	note := input()
	if _, err := f.s.Change(ctx, f.primary.ID, f.event, f.team, f.revision(), note); err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE events SET writes_allowed=false,stage='archived' WHERE id=$1", f.event)
	page, err := f.s.Read(ctx, f.primary.ID, f.event, f.team)
	if err != nil || len(page.Observations) != 1 || page.CanRecord || page.CanChangeStatus || page.Observations[0].Evidence.FeedbackReference != note.Evidence.FeedbackReference {
		t.Fatalf("read-only history lost or remains writable: %v", err)
	}
	note.Version = 1
	for _, command := range []Command{note, {Action: "status", State: "blocked", Reason: "Synthetic blocked status"}} {
		if _, err = f.s.Change(ctx, f.primary.ID, f.event, f.team, f.revision(), command); !errors.Is(err, access.ErrForbidden) {
			t.Fatalf("read-only event accepted %s: %v", command.Action, err)
		}
	}
}

func TestObservationsCurrentAssignmentConflictAndCrossEventIsolation(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	note := input()
	if _, err := f.s.Change(ctx, f.primary.ID, f.event, f.team, f.revision(), note); err != nil {
		t.Fatal(err)
	}
	for _, person := range []dbgen.Person{f.member, f.outsider} {
		if _, err := f.s.Read(ctx, person.ID, f.event, f.team); !errors.Is(err, access.ErrForbidden) {
			t.Fatalf("unassigned person read private notes: %v", err)
		}
	}
	f.exec("UPDATE people SET platform_admin=true WHERE id=$1", f.outsider.ID)
	f.exec("INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mc'),($1,$2,'judge'),($1,$2,'logistics')", f.event, f.outsider.ID)
	if _, err := f.s.Read(ctx, f.outsider.ID, f.event, f.team); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("unrelated privilege exposed mentor evidence")
	}
	otherEvent := testdb.Event(t, f.pool, f.owner.ID)
	if _, err := f.s.Read(ctx, f.primary.ID, otherEvent, f.team); err == nil {
		t.Fatal("cross-event team ID exposed evidence")
	}
	f.exec("INSERT INTO mentor_conflicts(event_id,mentor_id,competitor_id,kind) VALUES($1,$2,$3,'reporting')", f.event, f.primary.ID, f.member.ID)
	if _, err := f.s.Read(ctx, f.primary.ID, f.event, f.team); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("current conflict did not revoke historical access")
	}
	f.exec("DELETE FROM mentor_conflicts WHERE event_id=$1", f.event)
	f.exec("UPDATE mentor_assignments SET snapshot='draft' WHERE event_id=$1 AND person_id=$2", f.event, f.primary.ID)
	if _, err := f.s.History(ctx, f.primary.ID, f.event, f.team, note.ID); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("draft assignment retained author history access")
	}
	f.exec("UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mentor'", f.event, f.second.ID)
	if _, err := f.s.Read(ctx, f.second.ID, f.event, f.team); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("revoked mentor retained access")
	}
	f.exec("UPDATE hack_teams SET state='disbanded' WHERE event_id=$1 AND id=$2", f.event, f.team)
	if _, err := f.s.Read(ctx, f.owner.ID, f.event, f.team); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("disbanded team remained reviewable")
	}
}

func TestObservationsConcurrentEditsHaveOneWinner(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	note := input()
	page, err := f.s.Change(ctx, f.primary.ID, f.event, f.team, f.revision(), note)
	if err != nil {
		t.Fatal(err)
	}
	note.Version = 1
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for i := range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			change := note
			change.Evidence.Outcome = string(rune('A' + i))
			_, changeErr := f.s.Change(ctx, f.primary.ID, f.event, f.team, page.Revision, change)
			results <- changeErr
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	success, conflict := 0, 0
	for result := range results {
		if result == nil {
			success++
		} else if errors.Is(result, access.ErrConflict) {
			conflict++
		} else {
			t.Fatal(result)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("concurrent result: successes=%d conflicts=%d", success, conflict)
	}
	history, err := f.s.History(ctx, f.primary.ID, f.event, f.team, note.ID)
	if err != nil || len(history.Observations) != 2 {
		t.Fatalf("concurrent edit corrupted history: %v", err)
	}
}

func TestObservationRequiresActualCommonPublicationAndCurrentLiveSchedule(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	note := input()
	f.exec("UPDATE events SET published_at=NULL WHERE id=$1", f.event)
	if _, err := f.s.Change(ctx, f.primary.ID, f.event, f.team, f.revision(), note); !errors.Is(err, event.ErrBlocked) {
		t.Fatalf("scheduled live date substituted for actual common publication: %v", err)
	}
	f.exec("UPDATE events SET published_at=$2 WHERE id=$1", f.event, f.now.Add(time.Hour))
	if _, err := f.s.Change(ctx, f.primary.ID, f.event, f.team, f.revision(), note); !errors.Is(err, event.ErrBlocked) {
		t.Fatalf("future publication substituted for actual common publication: %v", err)
	}
	f.exec("UPDATE events SET published_at=$2 WHERE id=$1", f.event, f.now.AddDate(0, 0, -7))
	page, err := f.s.Change(ctx, f.primary.ID, f.event, f.team, f.revision(), note)
	if err != nil || len(page.Observations) != 1 {
		t.Fatalf("actual published live observation denied: %v", err)
	}
	// A later live-start schedule must be checked on subsequent reads as well
	// as writes; previously readable evidence is not an access cache.
	f.c.LiveStart = f.now.AddDate(0, 0, 1)
	data, _ := json.Marshal(f.c)
	f.exec("UPDATE events SET configuration=$2 WHERE id=$1", f.event, data)
	page, err = f.s.Read(ctx, f.primary.ID, f.event, f.team)
	if err != nil || page.CanRecord || len(page.Observations) != 0 || !page.CanChangeStatus {
		t.Fatalf("postponed live start retained formal review or blocked operational support: %v", err)
	}
	if _, err = f.s.History(ctx, f.primary.ID, f.event, f.team, note.ID); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("postponed live start exposed formal observation history: %v", err)
	}
}
