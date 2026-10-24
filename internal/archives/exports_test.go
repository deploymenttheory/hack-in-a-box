package archives

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestArchiveExportsPreserveNativeRoleAndUnavailableSources(t *testing.T) {
	pool := testdb.New(t, true)
	owner, logistic, member, outsider := testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	)
	eventID := testdb.Event(t, pool, owner.ID)
	execute(
		t,
		pool,
		"INSERT INTO event_memberships(event_id,person_id,role,active)VALUES($1,$2,'logistics',true)",
		eventID,
		logistic.ID,
	)
	execute(
		t,
		pool,
		"INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')",
		eventID,
		member.ID,
	)
	execute(
		t,
		pool,
		"INSERT INTO operational_profiles(id,event_id,person_id,contact_email,photo_consent)VALUES($1,$2,$3,'synthetic@example.org','yes')",
		uuid.New(),
		eventID,
		member.ID,
	)
	execute(
		t,
		pool,
		"UPDATE events SET stage='archived',writes_allowed=false,completed_at=now()WHERE id=$1",
		eventID,
	)
	service := Service{Pool: pool, Clock: func() time.Time { return time.Now().UTC() }}
	roster := ExportRequest{Source: "roster", Limit: 1}
	if _, err := service.Export(
		t.Context(),
		owner.ID,
		eventID,
		roster,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("archive organiser gained logistics profile export", err)
	}
	if _, err := service.Export(
		t.Context(),
		outsider.ID,
		eventID,
		roster,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("outsider exported private roster", err)
	}
	result, err := service.Export(t.Context(), logistic.ID, eventID, roster)
	if err != nil {
		t.Fatal(err)
	}
	if result.Roster == nil || len(result.Roster.Items) != 1 {
		t.Fatal("missing native paged roster", result)
	}
	// Follow native cursor pages; archived access must not bypass profile fields.
	all := []byte{}
	for {
		b, e := json.Marshal(result)
		if e != nil {
			t.Fatal(e)
		}
		all = append(all, b...)
		if result.Roster.NextCursor == nil {
			break
		}
		roster.Cursor = result.Roster.NextCursor.String()
		result, err = service.Export(t.Context(), logistic.ID, eventID, roster)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(string(all), "synthetic@example.org") ||
		strings.Contains(string(all), "photo_consent") {
		t.Fatal("native logistics field projection changed", string(all))
	}
	execute(
		t,
		pool,
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2",
		eventID,
		logistic.ID,
	)
	if _, err = service.Export(
		t.Context(),
		logistic.ID,
		eventID,
		roster,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("lost role retained export", err)
	}
	for _, source := range []string{"assessments", "feedback", "deliverables"} {
		_, err = service.Export(
			t.Context(),
			owner.ID,
			eventID,
			ExportRequest{Source: source, Limit: 10},
		)
		if !errors.Is(err, access.ErrInvalid) {
			t.Fatalf("%s missing scope silently exported: %v", source, err)
		}
	}
	current, err := service.Read(t.Context(), owner.ID, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if metric(t, *current.Summary, "recorded_attendance_people").Value != 0 {
		t.Fatal("invented attendance")
	}
	for _, source := range []string{"published-outcomes", "objective-evidence"} {
		result, err = service.Export(
			t.Context(),
			owner.ID,
			eventID,
			ExportRequest{Source: source, Limit: 10},
		)
		if err != nil {
			t.Fatal(source, err)
		}
		if source == "published-outcomes" &&
			(result.Projects == nil || len(result.Projects.Items) != 0) {
			t.Fatal("private/unpublished project invented", result)
		}
		if source == "objective-evidence" &&
			(result.Observations == nil || len(result.Observations.Items) != 0) {
			t.Fatal("empty evidence became result", result)
		}
	}
	var auditCount int
	if err = pool.QueryRow(t.Context(), "SELECT count(*) FROM audit_events WHERE event_id=$1 AND action='archive.exported'", eventID).
		Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount < 3 {
		t.Fatal("successful exports lack safe audit", auditCount)
	}
}
