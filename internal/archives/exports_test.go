package archives

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/objectives"
	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
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

func TestArchiveEvidenceRetainsExactDefinitionsAndDoesNotInventAdoption(t *testing.T) {
	pool := testdb.New(t, true)
	owner, author := testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	now := time.Now().UTC().Truncate(time.Second)
	plans := objectives.Service{Pool: pool, Clock: func() time.Time { return now }}
	objectiveID, measureID := uuid.New(), uuid.New()
	target := "10"
	definition := objectives.Definition{
		Description: "Original objective",
		Measures: []objectives.Measure{
			{
				ID:             measureID,
				Description:    "Dated adoption",
				Baseline:       &objectives.Baseline{Value: "0", Basis: "estimate"},
				Target:         &target,
				Unit:           "teams",
				Method:         "Recorded demonstration",
				EvidenceSource: "Synthetic review",
				ReviewHorizon:  "30 days",
			},
		},
	}
	original, err := plans.Change(
		t.Context(),
		owner.ID,
		eventID,
		objectiveID,
		0,
		objectives.Command{Action: "save", Definition: &definition, Reason: "Record original plan"},
	)
	if err != nil {
		t.Fatal(err)
	}
	ref := objectives.MeasureReference{Objective: original.Reference, MeasureID: measureID}
	native := outcomes.Service{Pool: pool, Now: func() time.Time { return now }}
	id := uuid.New()
	proposal := outcomes.Command{
		ID:     uuid.New(),
		Action: "propose",
		Reason: "Synthetic idea",
		Definition: &outcomes.Definition{
			Kind:          "pitch",
			Problem:       "PRIVATE-PROBLEM-CANARY",
			Audience:      "Synthetic participants",
			ProposedValue: "A proposal, not success",
		},
	}
	v, err := native.Change(t.Context(), author.ID, id, 0, proposal)
	if err != nil {
		t.Fatal(err)
	}
	v, err = native.Change(
		t.Context(),
		owner.ID,
		id,
		v.CurrentVersion,
		outcomes.Command{
			ID:         uuid.New(),
			Action:     "adopt",
			Reason:     "Associate explicit source",
			Provenance: &outcomes.Provenance{EventID: &eventID},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	v, err = native.Change(
		t.Context(),
		owner.ID,
		id,
		v.CurrentVersion,
		outcomes.Command{
			ID:       uuid.New(),
			Action:   "moderate",
			Status:   "in_progress",
			Measures: []objectives.MeasureReference{ref},
			Reason:   "Record ongoing follow-up",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	definition.Description = "Replacement objective"
	definition.Measures[0].Unit = "different unit"
	if _, err = plans.Change(
		t.Context(),
		owner.ID,
		eventID,
		objectiveID,
		1,
		objectives.Command{
			Action:     "save",
			Definition: &definition,
			Reason:     "Later independent planning version",
		},
	); err != nil {
		t.Fatal(err)
	}
	q := dbgen.New(pool)
	if err = q.InitializeEventFacts(t.Context(), eventID); err != nil {
		t.Fatal(err)
	}
	row, err := q.Event(t.Context(), eventID)
	if err != nil {
		t.Fatal(err)
	}
	lifecycle := event.Service{Pool: pool, Clock: func() time.Time { return now }}
	cancelled, err := lifecycle.Transition(
		t.Context(),
		owner.ID,
		eventID,
		row.Revision,
		"cancelled",
		"Synthetic cancellation",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	archived, err := lifecycle.Transition(
		t.Context(),
		owner.ID,
		eventID,
		cancelled.Revision,
		"archived",
		"",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	// Dedicated native archived follow-up is deliberately separate from competition.
	zero := "0"
	for _, basis := range []string{"demonstrated_adoption", "self_report", "estimate"} {
		evidence := outcomes.Evidence{
			ID:         uuid.New(),
			Basis:      basis,
			Claim:      "Sourced observation, not an inferred award",
			Source:     "Synthetic review record",
			ObservedAt: &now,
			Measures:   []objectives.MeasureReference{ref},
		}
		if basis == "demonstrated_adoption" {
			evidence.Value = &zero
		}
		v, err = native.Change(
			t.Context(),
			owner.ID,
			id,
			v.CurrentVersion,
			outcomes.Command{
				ID:       uuid.New(),
				Action:   "evidence",
				Reason:   "Record explicit evidence",
				Evidence: &evidence,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	service := Service{Pool: pool, Clock: func() time.Time { return now }}
	report, err := service.Read(t.Context(), owner.ID, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if metric(t, *report.Summary, "continued_projects").Value != 1 ||
		metric(t, *report.Summary, "evidenced_adoption_observations").Value != 1 {
		t.Fatal("conflated follow-up/evidence bases", report.Summary)
	}
	result, err := service.Export(
		t.Context(),
		owner.ID,
		eventID,
		ExportRequest{Source: "objective-evidence", Limit: 2},
	)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for {
		if result.Observations == nil {
			t.Fatal("missing observations")
		}
		for _, o := range result.Observations.Items {
			seen++
			if o.CurrentFollowUpStatus != "in_progress" || len(o.Measures) != 1 {
				t.Fatal("missing native status/source", o)
			}
			m := o.Measures[0]
			if m.Reference != ref || m.ObjectiveDescription != "Original objective" ||
				m.Definition.Unit != "teams" ||
				m.Definition.Target == nil ||
				*m.Definition.Target != "10" ||
				m.Definition.Baseline == nil ||
				m.Definition.Baseline.Value != "0" {
				t.Fatal("retargeted exact retained definition", m)
			}
			if o.Evidence.Basis == "demonstrated_adoption" &&
				(o.Evidence.Value == nil || *o.Evidence.Value != "0") {
				t.Fatal("explicit zero lost", o)
			}
			if o.Evidence.Basis != "demonstrated_adoption" && o.Evidence.Value != nil {
				t.Fatal("unknown became numeric zero", o)
			}
		}
		if result.Observations.NextCursor == nil {
			break
		}
		result, err = service.Export(
			t.Context(),
			owner.ID,
			eventID,
			ExportRequest{
				Source: "objective-evidence",
				Cursor: *result.Observations.NextCursor,
				Limit:  2,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	if seen != 3 {
		t.Fatal("lost native observation cursor cohort", seen)
	}
	result, err = service.Export(
		t.Context(),
		owner.ID,
		eventID,
		ExportRequest{Source: "published-outcomes", Limit: 10},
	)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	if len(result.Projects.Items) != 0 || strings.Contains(string(raw), "PRIVATE-PROBLEM-CANARY") {
		t.Fatal("unpublished evidence leaked into public project export", string(raw))
	}
	current, err := q.Event(t.Context(), eventID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != archived.Revision || current.WritesAllowed {
		t.Fatal("dedicated follow-up/export reopened competition", current)
	}
	execute(
		t,
		pool,
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2",
		eventID,
		owner.ID,
	)
	if _, err = service.Export(
		t.Context(),
		owner.ID,
		eventID,
		ExportRequest{Source: "objective-evidence", Limit: 10},
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("lost organiser retained restricted evidence", err)
	}
}
