package app

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/objectives"
	"github.com/google/uuid"
)

type lifecycleBrowserObjectiveInput struct {
	Definition objectives.Definition `json:"definition"`
	Reason     string                `json:"reason"`
}

func (r *lifecycleJourney) createObjectiveInBrowser(t *testing.T, item *lifecycleJourneyEvent, index int, expected objectives.Definition) {
	t.Helper()
	base := "/api/v1/events/" + item.view.ID.String()
	eventBefore := r.readRoleEvent(t, item)
	var before objectives.List
	if err := r.owner.call(t.Context(), http.MethodGet, base+"/objectives?limit=25", -1, nil, &before, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if len(before.Items) != 0 || before.NextCursor != nil {
		t.Fatal("UI objective authoring requires an empty native planning directory")
	}
	const reason = "Planning estimate only; not achieved outcome evidence"
	ack := r.admissionBrowserPhase(t, item, lifecycleBrowserPhase{Sequence: 701 + index*20, Action: "create-objective", Objective: &lifecycleBrowserObjectiveInput{Definition: expected, Reason: reason}})
	if !ack.Saved || ack.ObjectiveID == nil || *ack.ObjectiveID == uuid.Nil {
		t.Fatal("objective lacks an actual UI-generated resource acknowledgement")
	}
	if err := r.owner.call(t.Context(), http.MethodGet, base+"/objectives/"+ack.ObjectiveID.String(), -1, nil, &item.objective, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	saved := item.objective
	if saved.Reference.ID != *ack.ObjectiveID || saved.Reference.Version != 1 || saved.CurrentVersion != 1 || saved.Retired || !saved.CanManage || saved.ActorID != r.owner.person || saved.Reason != reason || len(saved.Definition.Measures) != len(expected.Measures) {
		t.Fatal("UI objective has unexpected native resource, version, actor or measure count")
	}
	ids := map[uuid.UUID]bool{}
	for i, measure := range saved.Definition.Measures {
		if measure.ID == uuid.Nil || ids[measure.ID] {
			t.Fatal("UI objective measures lack unique native identities")
		}
		ids[measure.ID] = true
		expected.Measures[i].ID = measure.ID
	}
	if !reflect.DeepEqual(saved.Definition, expected) {
		t.Fatal("native objective differs from the complete UI-authored owner, estimated baseline, target and evidence definition")
	}
	var after objectives.List
	if err := r.owner.call(t.Context(), http.MethodGet, base+"/objectives?limit=25", -1, nil, &after, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if len(after.Items) != 1 || after.Items[0].Reference != saved.Reference || after.NextCursor != nil {
		t.Fatal("UI objective authoring duplicated or omitted a native planning definition")
	}
	eventAfter := r.readRoleEvent(t, item)
	if eventBefore.Stage != eventAfter.Stage || eventBefore.ConfirmedAttendees != eventAfter.ConfirmedAttendees || !reflect.DeepEqual(eventBefore.Configuration, eventAfter.Configuration) || !reflect.DeepEqual(eventBefore.PublishedAt, eventAfter.PublishedAt) {
		t.Fatal("objective planning altered event, census or publication gates")
	}
	item.browserObjectiveAuthored = true
}
