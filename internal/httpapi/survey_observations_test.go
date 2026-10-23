package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/objectives"
	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestSurveyMeasurementDashboardConsumesNativeRetainedOutcomeEvidence(t *testing.T) {
	pool := testdb.New(t, true)
	f := newEmbargoFixture(t, pool, time.Now().UTC().AddDate(0, 0, 20))
	if err := database.Migrate(t.Context(), pool, 0); err != nil {
		t.Fatal(err)
	}
	native := &outcomes.Service{Pool: pool, Now: func() time.Time { return f.now }}
	f.router = NewRouter(
		f.cfg,
		slog.New(slog.NewJSONHandler(&f.logs, nil)),
		Dependencies{Pool: pool, Auth: f.auth, Outcomes: native},
	)
	id, objectiveID, measureID := uuid.New(), uuid.New(), uuid.New()
	command := outcomes.Command{
		ID:     uuid.New(),
		Action: "propose",
		Reason: "Submit a private measurable idea",
		Definition: &outcomes.Definition{
			Kind:          "pitch",
			Problem:       "PRIVATE-DASHBOARD-IDEA",
			Audience:      "People affected",
			ProposedValue: "Potential benefit, not evidenced achievement",
		},
	}
	if _, err := native.Change(t.Context(), f.stranger.ID, id, 0, command); err != nil {
		t.Fatal(err)
	}
	if _, err := native.Change(
		t.Context(),
		f.owner.ID,
		id,
		1,
		outcomes.Command{
			ID:         uuid.New(),
			Action:     "adopt",
			Reason:     "Adopt into the actual event",
			Provenance: &outcomes.Provenance{EventID: &f.id},
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := native.Change(
		t.Context(),
		f.owner.ID,
		id,
		2,
		outcomes.Command{
			ID:      uuid.New(),
			Action:  "assign",
			Reason:  "Assign actual follow-through ownership",
			OwnerID: &f.stranger.ID,
		},
	); err != nil {
		t.Fatal(err)
	}
	target := "10"
	definition := objectives.Definition{
		Description: "Retained planning intent",
		Measures: []objectives.Measure{
			{
				ID:             measureID,
				Description:    "Adopting teams",
				Unit:           "teams",
				Target:         &target,
				Method:         "Native owner review",
				EvidenceSource: "Dated observation",
				ReviewHorizon:  "One month",
			},
		},
	}
	planner := objectives.Service{Pool: pool}
	if _, err := planner.Change(
		t.Context(),
		f.owner.ID,
		f.id,
		objectiveID,
		0,
		objectives.Command{
			Action:     "save",
			Definition: &definition,
			Reason:     "Retain exact measurement definition",
		},
	); err != nil {
		t.Fatal(err)
	}
	reference := objectives.MeasureReference{
		Objective: objectives.Reference{ID: objectiveID, Version: 1},
		MeasureID: measureID,
	}
	if _, err := native.Change(
		t.Context(),
		f.owner.ID,
		id,
		3,
		outcomes.Command{
			ID:       uuid.New(),
			Action:   "moderate",
			Status:   "accepted",
			Measures: []objectives.MeasureReference{reference},
			Reason:   "Review exact measure",
		},
	); err != nil {
		t.Fatal(err)
	}
	definition.Measures[0].Unit = "people"
	if _, err := planner.Change(
		t.Context(),
		f.owner.ID,
		f.id,
		objectiveID,
		1,
		objectives.Command{
			Action:     "save",
			Definition: &definition,
			Reason:     "A separate newer definition",
		},
	); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		evidence := outcomes.Evidence{
			ID:       uuid.New(),
			Basis:    "self_report",
			Claim:    "PRIVATE-DASHBOARD-OBSERVATION",
			Source:   "PRIVATE-DASHBOARD-SOURCE",
			Measures: []objectives.MeasureReference{reference},
		}
		if _, err := native.Change(
			t.Context(),
			f.stranger.ID,
			id,
			int64(4+index),
			outcomes.Command{
				ID:       uuid.New(),
				Action:   "evidence",
				Evidence: &evidence,
				Reason:   "Record explicitly labelled self-report",
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	path := f.base() + "/outcomes/observations?limit=1"
	f.expect(f.request(dbgen.Person{}, "GET", path, nil, 0), 401)
	for _, actor := range []dbgen.Person{f.stranger, f.members[0][0], f.mentor, f.judge} {
		denied := f.request(actor, "GET", path, nil, 0)
		f.expect(denied, 403)
		if strings.Contains(denied.Body.String(), "PRIVATE-DASHBOARD-") {
			t.Fatal("native denial disclosed restricted observations")
		}
	}
	first := f.request(f.owner, "GET", path, nil, 0)
	f.expect(first, 200)
	validateResponse(t, httptest.NewRequest("GET", path, nil), first)
	if first.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("restricted observation cache policy")
	}
	var page outcomes.ObservationPage
	if err := json.Unmarshal(first.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.NextCursor == nil || len(page.Items[0].Measures) != 1 ||
		page.Items[0].Measures[0].Reference != reference ||
		page.Items[0].Measures[0].Definition.Unit != "teams" ||
		page.Items[0].Evidence.Basis != "self_report" {
		t.Fatal("native exact retained projection changed", first.Body.String())
	}
	for _, unknown := range []string{`"value":null`, `"denominator":null`, `"observed_at":null`, `"baseline":null`} {
		if !strings.Contains(first.Body.String(), unknown) {
			t.Fatal("missing observation became fabricated zero", unknown)
		}
	}
	second := f.request(f.owner, "GET", path+"&cursor="+*page.NextCursor, nil, 0)
	f.expect(second, 200)
	var next outcomes.ObservationPage
	if err := json.Unmarshal(second.Body.Bytes(), &next); err != nil {
		t.Fatal(err)
	}
	if len(next.Items) != 1 || next.NextCursor != nil ||
		next.Items[0].Evidence.ID == page.Items[0].Evidence.ID {
		t.Fatal("native cursor duplicated or omitted evidence")
	}
	staff := testdb.Person(t, pool)
	f.exec(
		"INSERT INTO event_memberships(event_id,person_id,role,active)VALUES($1,$2,'organiser',true)",
		f.id,
		staff.ID,
	)
	f.expect(f.request(staff, "GET", path, nil, 0), 200)
	f.exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2",
		f.id,
		staff.ID,
	)
	f.expect(f.request(staff, "GET", path, nil, 0), 403)
	if strings.Contains(f.logs.String(), "PRIVATE-DASHBOARD-") {
		t.Fatal("restricted observation leaked to request logs")
	}
}
