package attendance_test

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/attendance"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type fixture struct {
	pool                        *pgxpool.Pool
	q                           *dbgen.Queries
	service                     attendance.Service
	owner, logistic, competitor dbgen.Person
	event                       uuid.UUID
}

func setup(t *testing.T) fixture {
	t.Helper()
	pool := testdb.New(t, true)
	q := dbgen.New(pool)
	owner, logistic, competitor := testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	)
	id := testdb.Event(t, pool, owner.ID)
	c := event.DefaultConfiguration()
	c.LiveStart = time.Date(2027, 10, 29, 8, 0, 0, 0, time.UTC)
	c.LiveEnd = time.Date(2027, 11, 1, 0, 0, 0, 0, time.UTC)
	raw, _ := json.Marshal(c)
	if _, err := q.SaveEventConfiguration(
		t.Context(),
		dbgen.SaveEventConfigurationParams{
			ID:            id,
			Name:          "Synthetic event",
			Slug:          uuid.NewString(),
			Configuration: raw,
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := q.InitializeEventFacts(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if err := q.SetMembership(
		t.Context(),
		dbgen.SetMembershipParams{
			EventID:  id,
			PersonID: logistic.ID,
			Role:     "logistics",
			Active:   true,
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := q.SetCompetitionState(
		t.Context(),
		dbgen.SetCompetitionStateParams{EventID: id, PersonID: competitor.ID, State: "confirmed"},
	); err != nil {
		t.Fatal(err)
	}
	return fixture{
		pool:       pool,
		q:          q,
		owner:      owner,
		logistic:   logistic,
		competitor: competitor,
		event:      id,
		service: attendance.Service{
			Pool: pool,
			Now:  func() time.Time { return time.Date(2027, 10, 30, 11, 0, 0, 0, time.UTC) },
		},
	}
}

func (f fixture) mark(t *testing.T, expected int64, c attendance.Command) attendance.Entry {
	t.Helper()
	result, err := f.service.Mark(
		t.Context(),
		f.logistic.ID,
		f.event,
		f.competitor.ID,
		"2027-10-30",
		expected,
		c,
	)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestConcurrentScansCorrectionsAndLateRetries(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	command := attendance.Command{RequestID: uuid.New(), Present: true}
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			r, err := f.service.Mark(
				ctx,
				f.logistic.ID,
				f.event,
				f.competitor.ID,
				"2027-10-30",
				0,
				command,
			)
			if err != nil || r.Revision != 1 || !r.Present {
				t.Errorf("duplicate: %+v %v", r, err)
			}
		})
	}
	wg.Wait()
	for range 3 {
		r := f.mark(t, 0, attendance.Command{RequestID: uuid.New(), Present: true})
		if r.Revision != 1 {
			t.Fatal("fresh duplicate scan changed revision")
		}
	}
	history, err := f.service.History(ctx, f.owner.ID, f.event, f.competitor.ID, "2027-10-30")
	if err != nil || len(history) != 1 {
		t.Fatalf("duplicate history: %+v %v", history, err)
	}
	if _, err := f.service.Mark(
		ctx,
		f.logistic.ID,
		f.event,
		f.competitor.ID,
		"2027-10-30",
		1,
		attendance.Command{RequestID: uuid.New(), Present: false},
	); err == nil {
		t.Fatal("reasonless correction accepted")
	}
	corrected := f.mark(
		t,
		1,
		attendance.Command{RequestID: uuid.New(), Present: false, Reason: "Wrong person selected"},
	)
	if corrected.Revision != 2 || corrected.Present {
		t.Fatal(corrected)
	}
	replay := f.mark(t, 0, command)
	if replay.Present || replay.Revision != 2 {
		t.Fatal("old retry undid correction")
	}
	if _, err := f.service.Mark(
		ctx,
		f.logistic.ID,
		f.event,
		f.competitor.ID,
		"2027-10-30",
		0,
		attendance.Command{RequestID: uuid.New(), Present: true},
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("stale new scan undid correction", err)
	}
	command.Reason = "Changed replay content"
	if _, err := f.service.Mark(
		ctx,
		f.logistic.ID,
		f.event,
		f.competitor.ID,
		"2027-10-30",
		0,
		command,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("request UUID reused for different content", err)
	}
	restored := f.mark(
		t,
		2,
		attendance.Command{RequestID: uuid.New(), Present: true, Reason: "Arrival verified"},
	)
	if restored.Revision != 3 {
		t.Fatal(restored)
	}
	next, err := f.service.Mark(
		ctx,
		f.logistic.ID,
		f.event,
		f.competitor.ID,
		"2027-10-31",
		0,
		attendance.Command{RequestID: uuid.New(), Present: true},
	)
	if err != nil || next.Revision != 1 {
		t.Fatalf("independent day: %+v %v", next, err)
	}
	history, err = f.service.History(ctx, f.owner.ID, f.event, f.competitor.ID, "2027-10-30")
	if err != nil || len(history) != 3 || history[0].Reason != "Arrival verified" {
		t.Fatalf("correction history: %+v %v", history, err)
	}
	if _, err := f.pool.Exec(
		ctx,
		"UPDATE attendance_changes SET reason='changed' WHERE event_id=$1",
		f.event,
	); err == nil {
		t.Fatal("history was mutable")
	}
	var count int
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE event_id=$1 AND action='attendance.changed'", f.event).
		Scan(&count); err != nil ||
		count != 4 {
		t.Fatalf("audit %d %v", count, err)
	}
	var leaked bool
	if err := f.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM audit_events WHERE event_id=$1 AND metadata::text LIKE '%Arrival verified%')", f.event).
		Scan(&leaked); err != nil ||
		leaked {
		t.Fatal("correction prose entered generic audit", err)
	}
	confirmed, err := f.q.ConfirmedAttendees(ctx, f.event)
	if err != nil || confirmed != 1 {
		t.Fatal("physical attendance changed admission", confirmed, err)
	}
}

func TestRosterHeadcountsDaysAndCateringAreIndependentOfAdmission(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	undated := testdb.Event(t, f.pool, f.owner.ID)
	draft, err := f.service.Read(ctx, f.owner.ID, undated, "")
	if err != nil || draft.CanCheckIn || len(draft.Days) != 0 || draft.Day != "" {
		t.Fatalf("undated roster: %+v %v", draft, err)
	}

	for _, state := range []string{"received", "waitlisted", "withdrawn"} {
		p := testdb.Person(t, f.pool)
		if err := f.q.SetCompetitionState(
			ctx,
			dbgen.SetCompetitionStateParams{EventID: f.event, PersonID: p.ID, State: state},
		); err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.Mark(
			ctx,
			f.logistic.ID,
			f.event,
			p.ID,
			"2027-10-30",
			0,
			attendance.Command{RequestID: uuid.New(), Present: true},
		); !errors.Is(
			err,
			access.ErrForbidden,
		) {
			t.Fatalf("%s checked in: %v", state, err)
		}
	}
	// A competitor holding two operational roles counts once in each category and once in catering.
	for _, role := range []string{"logistics", "photographer"} {
		if err := f.q.SetMembership(
			ctx,
			dbgen.SetMembershipParams{
				EventID:  f.event,
				PersonID: f.competitor.ID,
				Role:     role,
				Active:   true,
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	page, err := f.service.Read(ctx, f.owner.ID, f.event, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Days) != 3 || page.Days[2] != "2027-10-31" || page.Day != "2027-10-30" ||
		!page.CanCheckIn ||
		page.CanReadLogistics {
		t.Fatalf("local days/permissions: %+v", page)
	}
	if page.Counts.ConfirmedCompetitors != 1 || page.Counts.Staff != 3 ||
		page.Counts.StaffAlsoCompeting != 1 ||
		page.Counts.CateringTotal != 3 ||
		page.Counts.CheckedInCompetitors != 0 ||
		page.Counts.MinimumCompetitors != 30 {
		t.Fatal(page.Counts)
	}
	configured, err := f.service.Configure(ctx, f.owner.ID, f.event, page.Revision, 7)
	if err != nil || configured.Counts.CateringTotal != 10 ||
		configured.Counts.ConfirmedCompetitors != 1 {
		t.Fatalf("buffer: %+v %v", configured, err)
	}
	if _, err := f.service.Configure(
		ctx,
		f.owner.ID,
		f.event,
		page.Revision,
		20,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("stale buffer accepted", err)
	}
	if _, err := f.service.Configure(
		ctx,
		f.logistic.ID,
		f.event,
		configured.Revision,
		20,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("logistics changed organiser buffer", err)
	}
	f.mark(t, 0, attendance.Command{RequestID: uuid.New(), Present: true})
	page, err = f.service.Read(ctx, f.logistic.ID, f.event, "2027-10-30")
	if err != nil || page.Counts.CheckedInCompetitors != 1 || !page.CanReadLogistics ||
		page.CanConfigure {
		t.Fatalf("arrival counts: %+v %v", page, err)
	}
	other, err := f.service.Read(ctx, f.logistic.ID, f.event, "2027-10-31")
	if err != nil || other.Counts.CheckedInCompetitors != 0 {
		t.Fatal("arrival crossed local day", other, err)
	}
	for _, day := range []string{"2027-10-28", "2027-11-01", "2027-02-30", "2027-1-01"} {
		if _, err := f.service.Read(
			ctx,
			f.owner.ID,
			f.event,
			day,
		); !errors.Is(
			err,
			access.ErrInvalid,
		) {
			t.Fatal("invalid day", day, err)
		}
	}
	view, err := (event.Service{Pool: f.pool}).Read(ctx, f.owner.ID, f.event)
	if err != nil {
		t.Fatal(err)
	}
	if view.ConfirmedAttendees != 1 || !view.CanReadAttendance || view.CanReadLogistics {
		t.Fatal("event capability/gate changed", view)
	}
	raw, _ := json.Marshal(page)
	for _, key := range []string{"dietary", "accessibility", "contact_email", "travel_details", "photo_consent", "social_attendance"} {
		if strings.Contains(string(raw), key) {
			t.Fatal("shared roster contains sensitive projection", key)
		}
	}
}

func TestWithdrawalRoleRevocationAndReadOnlyEvents(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	command := attendance.Command{RequestID: uuid.New(), Present: true}
	f.mark(t, 0, command)
	if err := f.q.SetCompetitionState(
		ctx,
		dbgen.SetCompetitionStateParams{
			EventID:  f.event,
			PersonID: f.competitor.ID,
			State:    "withdrawn",
		},
	); err != nil {
		t.Fatal(err)
	}
	page, err := f.service.Read(ctx, f.logistic.ID, f.event, "")
	if err != nil || len(page.Items) != 0 || page.Counts.CheckedInCompetitors != 0 {
		t.Fatal("withdrawn person remained in current roster", page, err)
	}
	if _, err := f.service.Mark(
		ctx,
		f.logistic.ID,
		f.event,
		f.competitor.ID,
		"2027-10-30",
		0,
		command,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("withdrawal did not invalidate check-in", err)
	}
	f.mark(
		t,
		1,
		attendance.Command{
			RequestID: uuid.New(),
			Present:   false,
			Reason:    "Withdrawn attendee correction",
		},
	)
	if err := f.q.SetMembership(
		ctx,
		dbgen.SetMembershipParams{
			EventID:  f.event,
			PersonID: f.logistic.ID,
			Role:     "logistics",
			Active:   false,
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Read(
		ctx,
		f.logistic.ID,
		f.event,
		"",
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("revoked roster access", err)
	}
	if _, err := f.service.History(
		ctx,
		f.logistic.ID,
		f.event,
		f.competitor.ID,
		"2027-10-30",
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("revoked history access", err)
	}
	if _, err := f.service.Mark(
		ctx,
		f.logistic.ID,
		f.event,
		f.competitor.ID,
		"2027-10-30",
		2,
		attendance.Command{RequestID: uuid.New(), Present: false},
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("revoked attendance write", err)
	}
	other := testdb.Event(t, f.pool, testdb.Person(t, f.pool).ID)
	if _, err := f.service.Read(ctx, f.owner.ID, other, ""); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("cross-event roster access", err)
	}
	if _, err := f.pool.Exec(
		ctx,
		"UPDATE events SET writes_allowed=false WHERE id=$1",
		f.event,
	); err != nil {
		t.Fatal(err)
	}
	page, err = f.service.Read(ctx, f.owner.ID, f.event, "")
	if err != nil || page.CanCheckIn || page.CanConfigure {
		t.Fatal("read-only capabilities", page, err)
	}
	if _, err := f.service.Mark(
		ctx,
		f.owner.ID,
		f.event,
		f.competitor.ID,
		"2027-10-30",
		2,
		attendance.Command{RequestID: uuid.New(), Present: false},
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("read-only write", err)
	}
}

func TestSensitiveLogisticsProjectionPaginationAndDeletion(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	travel, social := "private-travel", "no"
	fields := privacy.Fields{
		ContactEmail:     "private@example.test",
		ContactPhone:     "private-phone",
		Dietary:          "private-dietary",
		Accessibility:    "private-accessibility",
		PhotoConsent:     "no",
		TravelDetails:    &travel,
		SocialAttendance: &social,
	}
	privacyService := privacy.Service{Pool: f.pool}
	if _, err := privacyService.Correct(ctx, f.competitor.ID, f.event, 0, fields); err != nil {
		t.Fatal(err)
	}
	for _, actor := range []dbgen.Person{f.owner, f.competitor} {
		if _, err := f.service.Logistics(
			ctx,
			actor.ID,
			f.event,
			uuid.Nil,
			50,
			false,
		); !errors.Is(
			err,
			access.ErrForbidden,
		) {
			t.Fatal("non-logistics sensitive access", err)
		}
	}
	admin := testdb.Person(t, f.pool)
	if _, err := f.q.BootstrapAdmin(ctx, admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Read(ctx, admin.ID, f.event, ""); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("admin implicit roster access", err)
	}
	if _, err := f.service.Logistics(
		ctx,
		admin.ID,
		f.event,
		uuid.Nil,
		50,
		true,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("admin implicit logistics export", err)
	}
	seen := map[uuid.UUID]bool{}
	cursor := uuid.Nil
	found := false
	for {
		page, err := f.service.Logistics(ctx, f.logistic.ID, f.event, cursor, 1, true)
		if err != nil || len(page.Items) != 1 {
			t.Fatalf("page: %+v %v", page, err)
		}
		p := page.Items[0]
		if seen[p.PersonID] {
			t.Fatal("duplicate cursor item")
		}
		seen[p.PersonID] = true
		if p.PersonID == f.competitor.ID {
			found = true
			if p.TravelDetails == nil || *p.TravelDetails != travel || p.SocialAttendance == nil ||
				*p.SocialAttendance != "no" ||
				p.Dietary == nil {
				t.Fatal("logistics fields missing", p)
			}
		}
		raw, _ := json.Marshal(page)
		if strings.Contains(string(raw), "photo_consent") {
			t.Fatal("photo consent leaked into operational export")
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	if !found || len(seen) != 3 {
		t.Fatal("deduplicated operational roster incorrect", seen)
	}
	if err := privacyService.Delete(ctx, f.competitor.ID, f.event, f.competitor.ID); err != nil {
		t.Fatal(err)
	}
	page, err := f.service.Logistics(ctx, f.logistic.ID, f.event, uuid.Nil, 50, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range page.Items {
		if p.PersonID == f.competitor.ID &&
			(p.Dietary != nil || p.TravelDetails != nil || p.SocialAttendance != nil) {
			t.Fatal("deleted profile retained operational data")
		}
	}
	if _, err := privacyService.Correct(ctx, f.competitor.ID, f.event, 0, fields); err != nil {
		t.Fatal(err)
	}
	if err := f.q.SetCompetitionState(
		ctx,
		dbgen.SetCompetitionStateParams{
			EventID:  f.event,
			PersonID: f.competitor.ID,
			State:    "withdrawn",
		},
	); err != nil {
		t.Fatal(err)
	}
	page, err = f.service.Logistics(ctx, f.logistic.ID, f.event, uuid.Nil, 50, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range page.Items {
		if p.PersonID == f.competitor.ID {
			t.Fatal("withdrawn person leaked into logistics")
		}
	}
	if err := f.q.SetMembership(
		ctx,
		dbgen.SetMembershipParams{
			EventID:  f.event,
			PersonID: f.logistic.ID,
			Role:     "logistics",
			Active:   false,
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Logistics(
		ctx,
		f.logistic.ID,
		f.event,
		uuid.Nil,
		50,
		true,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("revoked export", err)
	}
	var exports int
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE event_id=$1 AND actor_id=$2 AND action='profile.exported'", f.event, f.logistic.ID).
		Scan(&exports); err != nil ||
		exports != 3 {
		t.Fatal("export audit missing", exports, err)
	}
}
