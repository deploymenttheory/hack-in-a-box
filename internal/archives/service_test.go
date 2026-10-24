package archives

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func execute(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), sql, args...); err != nil {
		t.Fatal(err)
	}
}
func metric(t *testing.T, s Summary, key string) Metric {
	t.Helper()
	for _, m := range s.Metrics {
		if m.Key == key {
			return m
		}
	}
	t.Fatalf("missing metric %s", key)
	return Metric{}
}
func TestArchivedMetadataAndExportRecheckActualRoleWithoutCompetitionWrites(t *testing.T) {
	pool := testdb.New(t, true)
	owner, member, outsider, admin := testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	other := testdb.Event(t, pool, outsider.ID)
	execute(t, pool, "UPDATE events SET stage='archived',writes_allowed=false,completed_at=now() WHERE id=ANY($1)", []uuid.UUID{eventID, other})
	execute(t, pool, "INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')", eventID, member.ID)
	execute(t, pool, "UPDATE people SET platform_admin=true WHERE id=$1", admin.ID)
	service := Service{Pool: pool}
	participant, err := service.Read(t.Context(), member.ID, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if !participant.ReadOnly || participant.Summary != nil || participant.SummaryState != "restricted" {
		t.Fatalf("participant projection: %+v", participant)
	}
	if _, err = service.ExportSummary(t.Context(), member.ID, eventID); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("participant acquired organiser metrics export", err)
	}
	if _, err = service.Read(t.Context(), outsider.ID, eventID); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("outsider acquired archive", err)
	}
	platform, err := service.Read(t.Context(), admin.ID, eventID)
	if err != nil || platform.Summary != nil {
		t.Fatal("platform status granted private report", platform, err)
	}
	if _, err = service.ExportSummary(t.Context(), admin.ID, eventID); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("platform status granted export", err)
	}
	report, err := service.ExportSummary(t.Context(), owner.ID, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if metric(t, report, "confirmed_attendees").Value != 1 {
		t.Fatal("confirmed count is not its native cohort")
	}
	var revision int64
	var writable bool
	var outcomes, operations, audits int
	if err = pool.QueryRow(t.Context(), "SELECT revision,writes_allowed FROM events WHERE id=$1", eventID).Scan(&revision, &writable); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(t.Context(), "SELECT count(*) FROM submission_states WHERE event_id=$1", eventID).Scan(&outcomes); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(t.Context(), "SELECT count(*) FROM operations WHERE event_id=$1", eventID).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(t.Context(), "SELECT count(*) FROM audit_events WHERE event_id=$1 AND action='archive.exported'", eventID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if revision != 1 || writable || outcomes != 0 || operations != 0 || audits != 1 {
		t.Fatalf("export changed competition or lacks audit: rev%d writable%v outcomes%d operations%d audits%d", revision, writable, outcomes, operations, audits)
	}
	execute(t, pool, "UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2", eventID, owner.ID)
	if _, err = service.ExportSummary(t.Context(), owner.ID, eventID); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("revoked organiser retained export", err)
	}
	page, err := service.Reports(t.Context(), owner.ID, uuid.Nil, 10)
	if err != nil || len(page.Items) != 0 {
		t.Fatal("revoked cross-event data retained", page, err)
	}
}
func TestArchiveCountsDistinguishApplicationsPeopleAttendanceAndUnusedEntries(t *testing.T) {
	pool := testdb.New(t, true)
	owner, first, second := testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	form := uuid.New()
	execute(t, pool, "INSERT INTO registration_forms(event_id,id) VALUES($1,$2)", eventID, form)
	execute(t, pool, "INSERT INTO registration_form_versions(event_id,form_id,version,definition) VALUES($1,$2,1,'{}')", eventID, form)
	for i, person := range []uuid.UUID{first.ID, first.ID, second.ID} {
		state := "withdrawn"
		if i == 1 {
			state = "confirmed"
		}
		execute(t, pool, "INSERT INTO registrations(id,event_id,person_id,form_id,form_version,audience,kind,state,request_hash,team_preference)VALUES($1,$2,$3,$4,1,'attendee','attendee',$5,'synthetic','solo')", uuid.New(), eventID, person, form, state)
	}
	execute(t, pool, "INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed'),($1,$3,'withdrawn')", eventID, first.ID, second.ID)
	for _, day := range []string{"2027-06-20", "2027-06-21"} {
		execute(t, pool, "INSERT INTO attendance_records(event_id,person_id,attendance_day,present)VALUES($1,$2,$3,true)", eventID, first.ID, day)
	}
	execute(t, pool, "INSERT INTO attendance_records(event_id,person_id,attendance_day,present)VALUES($1,$2,'2027-06-20',true)", eventID, second.ID)
	for i := range 3 {
		team := uuid.New()
		state := "published"
		active := true
		if i == 1 {
			state = "disbanded"
			active = false
		}
		execute(t, pool, "INSERT INTO hack_teams(id,event_id,name,state)VALUES($1,$2,'Synthetic team',$3)", team, eventID, state)
		execute(t, pool, "INSERT INTO competition_entries(event_id,team_id,active)VALUES($1,$2,$3)", eventID, team, active)
		execute(t, pool, "INSERT INTO submission_states(event_id,team_id)VALUES($1,$2)", eventID, team)
		if i < 2 {
			execute(t, pool, "INSERT INTO submission_revisions(event_id,team_id,revision,actor_id,submitted_at,late,document)VALUES($1,$2,1,$3,now(),false,'{\"private\":\"PRIVATE-CANARY\"}')", eventID, team, owner.ID)
			execute(t, pool, "UPDATE submission_states SET outcome='frozen',revision=1,active_revision=1,frozen_at=now()WHERE event_id=$1 AND team_id=$2", eventID, team)
		} else {
			execute(t, pool, "UPDATE submission_states SET outcome='not_submitted'WHERE event_id=$1 AND team_id=$2", eventID, team)
		}
	}
	execute(t, pool, "UPDATE events SET name=$2,stage='archived',writes_allowed=false,completed_at=now()WHERE id=$1", eventID, "\t=HYPERLINK(\"https://example.org\")")
	service := Service{Pool: pool, Clock: func() time.Time { return time.Date(2027, 7, 1, 0, 0, 0, 0, time.UTC) }}
	report, err := service.ExportSummary(t.Context(), owner.ID, eventID)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]int64{"registrations": 3, "registered_people": 2, "confirmed_attendees": 1, "recorded_attendance_people": 2, "competition_entries": 3, "completed_entries": 1, "unused_entries": 1, "not_submitted_entries": 1} {
		if got := metric(t, report, key); got.Value != want {
			t.Fatalf("%s=%d want%d", key, got.Value, want)
		}
	}
	if m := metric(t, report, "recorded_attendance_people"); m.Denominator != nil || m.DenominatorSource != "" {
		t.Fatal("recorded attendance implies an unrelated registration rate", m)
	}

	// Explicit absences and no observations both mean zero recorded presence;
	// neither supplies a complete physical-attendance census.
	for _, explicitAbsence := range []bool{true, false} {
		if explicitAbsence {
			execute(t, pool, "UPDATE attendance_records SET present=false WHERE event_id=$1", eventID)
		} else {
			execute(t, pool, "DELETE FROM attendance_records WHERE event_id=$1", eventID)
		}
		current, readErr := service.Read(t.Context(), owner.ID, eventID)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if got := metric(t, *current.Summary, "recorded_attendance_people"); got.Value != 0 || got.Denominator != nil {
			t.Fatal("invented physical attendance/rate", got)
		}
		unavailable := false
		for _, source := range current.Summary.Sources {
			if source.Source == "actual_attendance" && source.State == "unavailable" {
				unavailable = true
			}
		}
		if !unavailable {
			t.Fatal("missing physical-attendance coverage was presented as known")
		}
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "PRIVATE-CANARY") {
		t.Fatal("private submission prose leaked")
	}
	csv, err := report.CSV()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(csv), "'\t=HYPERLINK") {
		t.Fatal("event name formula not neutralised", string(csv))
	}
	for _, source := range report.Sources {
		if source.State != "unavailable" {
			t.Fatal("invented judging/result source", source)
		}
	}
}
func TestArchivedReportCursorIsPerAuthorisedEventNotGlobalPopulation(t *testing.T) {
	pool := testdb.New(t, true)
	owner, outsider := testdb.Person(t, pool), testdb.Person(t, pool)
	first, second, private := testdb.Event(t, pool, owner.ID), testdb.Event(t, pool, owner.ID), testdb.Event(t, pool, outsider.ID)
	execute(t, pool, "UPDATE events SET stage='archived',writes_allowed=false WHERE id=ANY($1)", []uuid.UUID{first, second, private})
	service := Service{Pool: pool}
	page, err := service.Reports(t.Context(), owner.ID, uuid.Nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.NextCursor == nil || page.Items[0].EventID == private {
		t.Fatal("report paging/auth scope", page)
	}
	next, err := service.Reports(t.Context(), owner.ID, *page.NextCursor, 1)
	if err != nil || len(next.Items) != 1 || next.NextCursor != nil || next.Items[0].EventID == private || next.Items[0].EventID == page.Items[0].EventID {
		t.Fatal("second report page", next, err)
	}
}
