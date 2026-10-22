package privacy_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestProfileFieldPoliciesCorrectionsAndExport(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	s := privacy.Service{Pool: pool}
	owner := testdb.Person(t, pool)
	event := testdb.Event(t, pool, owner.ID)
	other := testdb.Event(t, pool, owner.ID)
	person := testdb.Person(t, pool)
	q.SetCompetitionState(
		ctx,
		dbgen.SetCompetitionStateParams{EventID: event, PersonID: person.ID, State: "confirmed"},
	)
	travel, social := "private-travel", "yes"
	fields := privacy.Fields{
		ContactEmail:     "sensitive@example.test",
		ContactPhone:     "private-phone",
		Dietary:          "private-dietary",
		Accessibility:    "private-accessibility",
		PhotoConsent:     "no",
		TravelDetails:    &travel,
		SocialAttendance: &social,
	}
	profile, err := s.Correct(ctx, person.ID, event, 0, fields)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Dietary == nil || *profile.Dietary != fields.Dietary || profile.PhotoConsent == nil {
		t.Fatal("own profile omitted correction fields")
	}
	if _, err := s.Correct(ctx, person.ID, event, 0, fields); !errors.Is(err, access.ErrConflict) {
		t.Fatal("stale profile correction accepted")
	}
	legacy := fields
	legacy.TravelDetails, legacy.SocialAttendance = nil, nil
	current, err := s.Correct(ctx, person.ID, event, profile.Revision, legacy)
	if err != nil || current.TravelDetails == nil || *current.TravelDetails != travel ||
		current.SocialAttendance == nil || *current.SocialAttendance != social {
		t.Fatalf("legacy profile correction lost logistics fields: %+v %v", current, err)
	}
	for _, role := range []string{"organiser", "mentor", "judge", "mc", "logistics", "photographer"} {
		viewer := testdb.Person(t, pool)
		q.SetMembership(
			ctx,
			dbgen.SetMembershipParams{
				EventID:  event,
				PersonID: viewer.ID,
				Role:     role,
				Active:   true,
			},
		)
		view, err := s.Read(ctx, viewer.ID, event, person.ID)
		if role != "logistics" && role != "photographer" {
			if !errors.Is(err, access.ErrForbidden) {
				t.Fatalf("%s received private profile: %v", role, err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		wire, _ := json.Marshal(view)
		if role == "photographer" {
			if view.PhotoConsent == nil || view.SocialAttendance != nil ||
				strings.Contains(string(wire), "private-") ||
				strings.Contains(string(wire), "contact_email") {
				t.Fatalf("photographer received operational fields: %s", wire)
			}
		} else {
			if view.Dietary == nil || view.Accessibility == nil || view.TravelDetails == nil ||
				view.SocialAttendance == nil ||
				view.PhotoConsent != nil {
				t.Fatalf("logistics field policy: %s", wire)
			}
			exported, err := s.Export(ctx, viewer.ID, event, uuid.Nil, 50)
			if err != nil || len(exported) != 1 || exported[0].PhotoConsent != nil {
				t.Fatalf("logistics export: %+v %v", exported, err)
			}
			var count int
			pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE actor_id=$1 AND event_id=$2 AND action='profile.exported'", viewer.ID, event).
				Scan(&count)
			if count != 1 {
				t.Fatal("export audit missing")
			}
		}
		if _, err := s.Read(
			ctx,
			viewer.ID,
			other,
			person.ID,
		); !errors.Is(
			err,
			access.ErrForbidden,
		) {
			t.Fatalf("cross-event profile access: %v", err)
		}
		q.SetMembership(
			ctx,
			dbgen.SetMembershipParams{
				EventID:  event,
				PersonID: viewer.ID,
				Role:     role,
				Active:   false,
			},
		)
		if _, err := s.Read(
			ctx,
			viewer.ID,
			event,
			person.ID,
		); !errors.Is(
			err,
			access.ErrForbidden,
		) {
			t.Fatal("revoked profile permission survived")
		}
	}
	attendee := testdb.Person(t, pool)
	q.SetCompetitionState(
		ctx,
		dbgen.SetCompetitionStateParams{EventID: event, PersonID: attendee.ID, State: "confirmed"},
	)
	if _, err := s.Read(ctx, attendee.ID, event, person.ID); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("attendee read another person's fields")
	}
	admin := testdb.Person(t, pool)
	q.BootstrapAdmin(ctx, admin.ID)
	if _, err := s.Read(ctx, admin.ID, event, person.ID); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("platform admin implicitly read sensitive fields")
	}
	if err := s.Delete(ctx, admin.ID, event, person.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read(ctx, person.ID, event, person.ID); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("administrative deletion retained fields")
	}
}

func TestRetentionRemovesFieldsAndDraftsButPreservesSafeHistory(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	now := time.Date(2027, 7, 1, 12, 0, 0, 0, time.UTC)
	s := privacy.Service{Pool: pool, Clock: func() time.Time { return now }}
	owner := testdb.Person(t, pool)
	event := testdb.Event(t, pool, owner.ID)
	other := testdb.Event(t, pool, owner.ID)
	travel, social := "never-keep-this-travel", "yes"
	fields := privacy.Fields{
		Dietary:          "never-keep-this-diet",
		Accessibility:    "never-keep-this-accessibility",
		PhotoConsent:     "unknown",
		TravelDetails:    &travel,
		SocialAttendance: &social,
	}
	if _, err := s.Correct(ctx, owner.ID, event, 0, fields); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Correct(ctx, owner.ID, other, 0, fields); err != nil {
		t.Fatal(err)
	}
	if err := s.Configure(
		ctx,
		owner.ID,
		event,
		1,
		privacy.Settings{RetentionDays: 1, DraftDays: 1},
	); err != nil {
		t.Fatal(err)
	}
	expired, current := uuid.New(), uuid.New()
	if err := privacy.StoreDraft(
		ctx,
		q,
		event,
		owner.ID,
		expired,
		[]byte("private-expired-draft"),
		now.Add(-48*time.Hour),
	); err != nil {
		t.Fatal(err)
	}
	if err := privacy.StoreDraft(
		ctx,
		q,
		other,
		owner.ID,
		current,
		[]byte("current-draft"),
		now,
	); err != nil {
		t.Fatal(err)
	}
	if err := privacy.StoreDraft(
		ctx,
		q,
		event,
		owner.ID,
		current,
		[]byte("attempted-cross-event-overwrite"),
		now,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("draft ID moved across events")
	}
	if _, err := pool.Exec(
		ctx,
		"UPDATE events SET stage='completed',completed_at=$2 WHERE id=$1",
		event,
		now.Add(-48*time.Hour),
	); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := s.Retain(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Read(ctx, owner.ID, event, owner.ID); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("expired personal fields survived")
	}
	if _, err := s.Read(ctx, owner.ID, other, owner.ID); err != nil {
		t.Fatal("active event lost profile")
	}
	var drafts, count int
	pool.QueryRow(ctx, "SELECT count(*) FROM private_drafts").Scan(&drafts)
	pool.QueryRow(ctx, "SELECT removed_profiles FROM privacy_aggregates WHERE event_id=$1", event).
		Scan(&count)
	if drafts != 1 || count != 1 {
		t.Fatalf("retention idempotency: drafts=%d removed=%d", drafts, count)
	}
	excluded, err := q.CompetitionExcluded(
		ctx,
		dbgen.CompetitionExcludedParams{EventID: event, PersonID: owner.ID},
	)
	if err != nil || !excluded {
		t.Fatal("retention erased competition restriction")
	}
	auditRows, err := q.ListAudit(ctx, dbgen.ListAuditParams{EventID: &event, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(auditRows)
	if len(auditRows) < 3 || strings.Contains(string(encoded), "never-keep") ||
		strings.Contains(string(encoded), "private-expired") {
		t.Fatal("safe decision history missing or contains personal prose")
	}
}
