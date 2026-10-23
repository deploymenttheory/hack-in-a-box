package objectives_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/objectives"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestObjectiveActualEventCloneOwnsIndependentConfigurationVersions(t *testing.T) {
	pool := testdb.New(t, true)

	owner := testdb.Person(t, pool)
	source := testdb.Event(t, pool, owner.ID)
	c := event.DefaultConfiguration()
	c.LiveStart = time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	c.SubmissionCutoff = c.LiveEnd
	c.TeamFreeze = c.LiveStart.Add(-8 * 24 * time.Hour)
	c.AdmissionCutoff = c.TeamFreeze
	config, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(
		t.Context(),
		"UPDATE events SET configuration=$2 WHERE id=$1",
		source,
		config,
	); err != nil {
		t.Fatal(err)
	}
	s := objectives.Service{Pool: pool}
	ts := templates.Service{Pool: pool}
	id, measure := uuid.New(), uuid.New()
	target := "10"
	d := objectives.Definition{
		Description:    "Private planning objective",
		ThemeSensitive: true,
		OwnerID:        &owner.ID,
		Measures: []objectives.Measure{
			{ID: measure, Description: "Example count", Target: &target},
		},
	}
	if _, err = s.Change(
		t.Context(),
		owner.ID,
		source,
		id,
		0,
		objectives.Command{Action: "save", Definition: &d, Reason: "Initial plan"},
	); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ts.Snapshot(t.Context(), owner.ID, source)
	if err != nil || len(snapshot.Definitions.Objectives) != 1 {
		t.Fatal("native configuration omitted from snapshot", snapshot, err)
	}
	cloneID := uuid.New()
	input := templates.CloneInput{
		ID:                                cloneID,
		Name:                              "Independent cloned event",
		Slug:                              uuid.NewString(),
		LiveStart:                         c.LiveStart.Add(365 * 24 * time.Hour),
		Timezone:                          c.Timezone,
		AcknowledgeCompetitionRestriction: true,
	}
	clone, err := ts.CloneEvent(t.Context(), owner.ID, source, input)
	if err != nil {
		t.Fatal(err)
	}
	listing, err := s.List(t.Context(), owner.ID, clone.ID, uuid.Nil, 50)
	if err != nil || len(listing.Items) != 1 {
		t.Fatal("cloned native definitions", listing, err)
	}
	copied := listing.Items[0]
	if copied.Reference.ID == id || copied.Definition.OwnerID != nil ||
		copied.Reference.Version != 1 ||
		copied.Retired ||
		copied.Definition.Measures[0].ID == measure {
		t.Fatal("copied source identity/person/history", copied)
	}
	d.Description = "Changed source event plan"
	if _, err = s.Change(
		t.Context(),
		owner.ID,
		source,
		id,
		1,
		objectives.Command{Action: "save", Definition: &d, Reason: "Source-only change"},
	); err != nil {
		t.Fatal(err)
	}
	retained, err := s.Read(t.Context(), owner.ID, clone.ID, copied.Reference.ID, 0)
	if err != nil || retained.Definition.Description != "Private planning objective" ||
		retained.CurrentVersion != 1 {
		t.Fatal("source change mutated clone", retained, err)
	}
	history, err := s.History(t.Context(), owner.ID, clone.ID, copied.Reference.ID, 0, 50)
	if err != nil || len(history.Items) != 1 {
		t.Fatal("source follow-up/version history copied", history, err)
	}
}
