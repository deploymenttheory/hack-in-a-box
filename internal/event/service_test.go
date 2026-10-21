package event_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/permissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func input() event.Input {
	c := event.DefaultConfiguration()
	c.Timezone = "UTC"
	c.Capacity = 10
	c.MinimumAttendance = 2
	c.LiveStart = time.Date(2027, 6, 20, 10, 0, 0, 0, time.UTC)
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	c.SubmissionCutoff = c.LiveEnd.Add(-time.Hour)
	c.TeamFreeze = c.LiveStart.AddDate(0, 0, -8)
	c.AdmissionCutoff = c.TeamFreeze
	return event.Input{
		ID:                                uuid.New(),
		Name:                              "Synthetic hack",
		Slug:                              "hack-" + uuid.NewString(),
		Configuration:                     c,
		AcknowledgeCompetitionRestriction: true,
	}
}

func TestLifecycleConfirmedGatePublicationWithdrawalAndCutoff(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	owner := testdb.Person(t, pool)
	q.BootstrapAdmin(ctx, owner.ID)
	body := input()
	now := body.Configuration.LiveStart.AddDate(0, 0, -30)
	s := event.Service{Pool: pool, Clock: func() time.Time { return now }}
	row, err := s.Create(ctx, owner.ID, body)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Create(ctx, owner.ID, body)
	if err != nil || again.ID != row.ID {
		t.Fatalf("creation retry: %v", err)
	}
	transition := func(to, reason string, exception bool) error {
		next, err := s.Transition(ctx, owner.ID, row.ID, row.Revision, to, reason, exception)
		if err == nil {
			row = next
		}
		return err
	}
	if err := transition("registration", "", false); err != nil {
		t.Fatal(err)
	}
	if err := transition("preparation", "", false); !errors.Is(err, event.ErrBlocked) {
		t.Fatal("missing domain facts did not block preparation")
	}
	first, second, waiting := testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool)
	setClaim := func(person uuid.UUID, state string) {
		t.Helper()
		if err := database.Within(ctx, pool, func(tx pgx.Tx, q *dbgen.Queries) error {
			if err := event.RecordAttendanceReady(ctx, tx, row.ID); err != nil {
				return err
			}
			return permissions.SetRegistrationState(ctx, q, row.ID, person, state)
		}); err != nil {
			t.Fatal(err)
		}
	}
	setClaim(first.ID, "confirmed")
	setClaim(waiting.ID, "waitlisted")
	if err := transition("preparation", "", false); !errors.Is(err, event.ErrBlocked) {
		t.Fatal("waitlist counted toward attendance")
	}
	setClaim(second.ID, "confirmed")
	if current, _ := q.Event(ctx, row.ID); current.Stage != "registration" {
		t.Fatal("minimum automatically advanced stage")
	}
	if err := transition("preparation", "", false); err != nil {
		t.Fatal(err)
	}
	setClaim(second.ID, "withdrawn")
	if current, _ := q.Event(ctx, row.ID); current.Stage != "preparation" {
		t.Fatal("withdrawal regressed event")
	}
	if err := database.Within(ctx, pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		issues, err := event.PublicationReadiness(ctx, tx, row.ID)
		if err != nil {
			return err
		}
		if len(issues) == 0 {
			t.Fatal("publication ignored attendance/team gates")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Model facts supplied atomically by the later team/content owners.
	published := body.Configuration.LiveStart.AddDate(0, 0, -7)
	if _, err := pool.Exec(
		ctx,
		"UPDATE events SET published_at=$2,team_frozen_at=$3 WHERE id=$1",
		row.ID,
		published,
		body.Configuration.TeamFreeze,
	); err != nil {
		t.Fatal(err)
	}
	if err := database.Within(ctx, pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		return event.RecordTeamFacts(ctx, tx, row.ID, 1, 1, 1, &body.Configuration.TeamFreeze)
	}); err != nil {
		t.Fatal(err)
	}
	now = body.Configuration.LiveStart
	if err := transition("live", "", false); !errors.Is(err, event.ErrBlocked) {
		t.Fatal("attendance shortfall lacked exception")
	}
	if err := transition(
		"live",
		"Withdrawal after publication; continue with the remaining team.",
		true,
	); err != nil {
		t.Fatal(err)
	}
	if err := transition("assessment", "", false); !errors.Is(err, event.ErrBlocked) {
		t.Fatal("assessment opened before cutoff")
	}
	now = body.Configuration.SubmissionCutoff
	if err := transition("assessment", "", false); !errors.Is(err, event.ErrBlocked) {
		t.Fatal("missing submission facts accepted")
	}
	if err := database.Within(
		ctx,
		pool,
		func(tx pgx.Tx, _ *dbgen.Queries) error { return event.RecordSubmissionFacts(ctx, tx, row.ID, 1) },
	); err != nil {
		t.Fatal(err)
	}
	if err := transition("assessment", "", false); err != nil {
		t.Fatal(err)
	}
	if err := transition("completed", "", false); err != nil {
		t.Fatal(err)
	}
	if err := transition("assessment", "", false); err == nil {
		t.Fatal("reopened completed stage without reason")
	}
	if err := transition("assessment", "Correct the reviewed assessment.", false); err != nil {
		t.Fatal(err)
	}
	if current, _ := q.Event(ctx, row.ID); current.CompletedAt.Valid {
		t.Fatal("reopened event retained retention date")
	}
	if err := transition("completed", "", false); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO operations(event_id,operation_key,kind,target_id,due_at,approved) VALUES($1,'archive','synthetic',$1,now(),true)",
		row.ID,
	); err != nil {
		t.Fatal(err)
	}
	if err := transition("archived", "Retain the event record.", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(
		ctx,
		owner.ID,
		row.ID,
		row.Revision,
		body,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("archived event remained writable")
	}
	var cancelled bool
	if err := pool.QueryRow(ctx, "SELECT cancelled FROM operations WHERE event_id=$1 AND operation_key='archive'", row.ID).
		Scan(&cancelled); err != nil ||
		!cancelled {
		t.Fatal("archive retained outbound work")
	}
	outsider := testdb.Person(t, pool)
	if _, err := s.Read(ctx, outsider.ID, row.ID); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("cross-event read")
	}
	listed, err := s.List(ctx, outsider.ID, uuid.Nil, 50)
	if err != nil || len(listed) != 0 {
		t.Fatal("directory leaked event")
	}
}

func TestScheduleEditsCancelApprovalWithoutReplayingExecutedActions(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	owner := testdb.Person(t, pool)
	q.BootstrapAdmin(ctx, owner.ID)
	body := input()
	now := body.Configuration.LiveStart.AddDate(0, 0, -30)
	s := event.Service{Pool: pool, Clock: func() time.Time { return now }}
	item := event.AgendaItem{
		ID:       uuid.New(),
		Title:    "Briefing",
		Audience: []string{"all"},
		Anchor:   "live_start",
		Days:     -1,
	}
	body.Configuration.Schedule = []event.AgendaItem{item}
	row, err := s.Create(ctx, owner.ID, body)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApproveSchedule(ctx, owner.ID, row.ID, item.ID, row.Revision); err != nil {
		t.Fatal(err)
	}
	operation := uuid.New()
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO operations(id,event_id,operation_key,kind,target_id,due_at,approved) VALUES($1,$2,'scheduled','synthetic',$3,$4,true)",
		operation,
		row.ID,
		item.ID,
		body.Configuration.LiveStart,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(
		ctx,
		"UPDATE schedule_items SET operation_id=$3 WHERE event_id=$1 AND id=$2",
		row.ID,
		item.ID,
		operation,
	); err != nil {
		t.Fatal(err)
	}
	body.Configuration.Schedule[0].Title = "Revised briefing"
	row, err = s.Update(ctx, owner.ID, row.ID, row.Revision, body)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.Schedule(ctx, owner.ID, row.ID)
	if err != nil || rows[0].Status != "draft" {
		t.Fatalf("approval survived edit: %+v %v", rows, err)
	}
	var cancelled bool
	pool.QueryRow(ctx, "SELECT cancelled FROM operations WHERE id=$1", operation).Scan(&cancelled)
	if !cancelled {
		t.Fatal("obsolete queued operation survived")
	}
	if _, err := pool.Exec(
		ctx,
		"UPDATE schedule_items SET executed_at=now() WHERE event_id=$1",
		row.ID,
	); err != nil {
		t.Fatal(err)
	}
	body.Configuration.Schedule[0].Minutes = 30
	row, err = s.Update(ctx, owner.ID, row.ID, row.Revision, body)
	if err != nil {
		t.Fatal(err)
	}
	rows, err = s.Schedule(ctx, owner.ID, row.ID)
	if err != nil || rows[0].Status != "executed" {
		t.Fatal("executed action became replayable")
	}
	if err := s.ApproveSchedule(
		ctx,
		owner.ID,
		row.ID,
		item.ID,
		row.Revision,
	); !errors.Is(
		err,
		event.ErrBlocked,
	) {
		t.Fatal("executed action approved again")
	}
	if _, err := s.Update(ctx, owner.ID, row.ID, 1, body); !errors.Is(err, access.ErrConflict) {
		t.Fatal("stale edit accepted")
	}
}

func TestPublishedPostponementNeedsApprovalAndKeepsFrozenTeams(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	owner := testdb.Person(t, pool)
	q.BootstrapAdmin(ctx, owner.ID)
	body := input()
	now := body.Configuration.LiveStart.AddDate(0, 0, -5)
	s := event.Service{Pool: pool, Clock: func() time.Time { return now }}
	row, err := s.Create(ctx, owner.ID, body)
	if err != nil {
		t.Fatal(err)
	}
	published := body.Configuration.LiveStart.AddDate(0, 0, -7)
	pool.Exec(
		ctx,
		"UPDATE events SET published_at=$2,team_frozen_at=$3,stage='preparation' WHERE id=$1",
		row.ID,
		published,
		body.Configuration.TeamFreeze,
	)
	body.Configuration.LiveStart = body.Configuration.LiveStart.AddDate(0, 0, 7)
	body.Configuration.LiveEnd = body.Configuration.LiveEnd.AddDate(0, 0, 7)
	body.Configuration.SubmissionCutoff = body.Configuration.SubmissionCutoff.AddDate(0, 0, 7)
	if _, err := s.Update(
		ctx,
		owner.ID,
		row.ID,
		row.Revision,
		body,
	); !errors.Is(
		err,
		event.ErrBlocked,
	) {
		t.Fatal("post-publication postponement lacked approval")
	}
	body.ApprovePostponement = true
	body.Reason = "Venue disruption; continue work during the additional week."
	row, err = s.Update(ctx, owner.ID, row.ID, row.Revision, body)
	if err != nil {
		t.Fatal(err)
	}
	if row.PublishedAt == nil || !row.PublishedAt.Equal(published) {
		t.Fatal("published challenge was replayed")
	}
	var notices int
	pool.QueryRow(ctx, "SELECT count(*) FROM event_notices WHERE event_id=$1", row.ID).
		Scan(&notices)
	if notices != 1 {
		t.Fatal("revised-date communication intent missing")
	}
	body.Configuration.TeamFreeze = body.Configuration.TeamFreeze.AddDate(0, 0, 1)
	if _, err := s.Update(
		ctx,
		owner.ID,
		row.ID,
		row.Revision,
		body,
	); !errors.Is(
		err,
		event.ErrBlocked,
	) {
		t.Fatal("date edit reopened team freeze")
	}
}
