package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/qualification"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/google/uuid"
)

type lifecycleBrowserQualificationInput struct {
	Actor   string                `json:"actor"`
	ActorID uuid.UUID             `json:"actor_id"`
	Page    qualification.Page    `json:"page"`
	Command qualification.Command `json:"command"`
}

func (r *lifecycleJourney) changeQualificationInBrowser(t *testing.T, item *lifecycleJourneyEvent, client *lifecycleClient, before qualification.Page, body map[string]any) (qualification.Page, uuid.UUID) {
	t.Helper()
	actor := "owner"
	if client != r.owner {
		actor = ""
		for _, name := range []string{"mentor-a", "mentor-b"} {
			if client == item.cohort.staff[name] {
				actor = name
			}
		}
		if actor == "" {
			t.Fatal("qualification browser command has no named normal OIDC actor")
		}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	var command qualification.Command
	if err = json.Unmarshal(raw, &command); err != nil {
		t.Fatal(err)
	}
	command.RequestID = uuid.Nil // Production owns the actual request identity.
	input := lifecycleBrowserQualificationInput{Actor: actor, ActorID: client.person, Page: before, Command: command}
	ack := r.admissionBrowserPhase(t, item, lifecycleBrowserPhase{Sequence: 2101 + len(item.browserQualification), Action: "record-qualification", Qualification: &input})
	if !ack.Saved || ack.CommandID == nil || *ack.CommandID == uuid.Nil {
		t.Fatal("qualification UI did not identify its original request")
	}
	path := fmt.Sprintf("/api/v1/events/%s/mentor-qualification/%s", item.view.ID, before.Round.RunID)
	var next qualification.Page
	if err = client.call(t.Context(), http.MethodGet, path, -1, nil, &next, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	var original qualification.Recovery
	if err = client.call(t.Context(), http.MethodGet, path+"/commands/"+ack.CommandID.String(), -1, nil, &original, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	next.Receipt = original.Receipt
	if next.Round.RunID != before.Round.RunID || next.Round.Version != before.Round.Version+1 || next.Round.ActorID != client.person || next.Round.Action != command.Action || next.Round.Reason != command.Reason || next.Revision != before.Revision+1 || next.Receipt == nil {
		t.Fatal("qualification UI changed its captured native actor/action/source/version")
	}
	var history qualification.History
	if err = client.call(t.Context(), http.MethodGet, fmt.Sprintf("%s/history?cursor=%d&limit=1", path, next.Round.Version+1), -1, nil, &history, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if len(history.Items) != 1 || !reflect.DeepEqual(history.Items[0].Snapshot, next.Round) {
		t.Fatal("qualification command lacks exact original immutable round history")
	}
	switch command.Action {
	case "configure_panel":
		if command.Panel == nil || !reflect.DeepEqual(next.Round.Panel, *command.Panel) {
			t.Fatal("qualification UI substituted intended panel")
		}
	case "save_card":
		if command.Card == nil {
			t.Fatal("qualification browser card missing")
		}
		found := false
		for _, card := range next.Round.Cards {
			if card.TeamID == command.Card.TeamID {
				found = card.Version == command.Card.Version+1 && reflect.DeepEqual(card.Scores, command.Card.Scores) && card.State == "draft"
			}
		}
		if !found {
			t.Fatal("qualification UI did not save its exact moderated content")
		}
	case "sign_card":
		found := false
		for _, card := range next.Round.Cards {
			if card.TeamID == command.TeamID {
				for _, prior := range before.Round.Cards {
					if prior.TeamID == card.TeamID {
						found = card.Version == command.CardVersion+1 && card.State == "signed" && reflect.DeepEqual(card.Source, prior.Source) && reflect.DeepEqual(card.Scores, prior.Scores)
					}
				}
			}
		}
		if !found {
			t.Fatal("qualification UI did not confirm its exact saved final card")
		}
	case "vote_tie":
		if len(next.Round.Votes) != len(before.Round.Votes)+1 {
			t.Fatal("qualification ballot census changed")
		}
		vote := next.Round.Votes[len(next.Round.Votes)-1]
		if vote.TieID != command.TieID || vote.Seat != command.Seat || vote.TeamID != command.TeamID || vote.ActorID != client.person || vote.Reason != command.Reason || !reflect.DeepEqual(vote.Evidence, command.Evidence) {
			t.Fatal("qualification ballot lost original explicit choice or evidence")
		}
	case "resolve_tie":
		if len(next.Round.Decisions) != len(before.Round.Decisions)+1 {
			t.Fatal("qualification decision census changed")
		}
		decision := next.Round.Decisions[len(next.Round.Decisions)-1]
		if decision.TieID != command.TieID || decision.Seat != command.Seat || decision.TeamID != command.TeamID || decision.ActorID != client.person || decision.Reason != command.Reason || decision.Casting || !reflect.DeepEqual(decision.Evidence, command.Evidence) {
			t.Fatal("qualification resolution lost its original complete panel choice")
		}
	case "sign_off":
		if next.Round.Signoff == nil || next.Round.Signoff.ActorID != client.person || next.Round.Signoff.Reason != command.Reason || next.Round.Signoff.Binding == "" {
			t.Fatal("qualification signoff lost its native original proof")
		}
	case "propose":
		if next.Round.Proposal == nil || len(next.Round.Votes) != 0 || len(next.Round.Decisions) != 0 {
			t.Fatal("qualification proposal did not retain a fresh explicit ballot context")
		}
	default:
		t.Fatal("unexpected qualification browser action")
	}
	receipt := next.Receipt
	item.browserQualification = append(item.browserQualification, lifecycleQualificationCommandReceipt{RequestID: *ack.CommandID, ActorID: client.person, Action: command.Action, Fingerprint: receipt.Fingerprint, ExpectedRevision: before.Revision, ExpectedVersion: before.Round.Version, Version: next.Round.Version})
	return next, *ack.CommandID
}

func lifecycleBrowserQualificationComplete(item lifecycleJourneyEvent) bool {
	if item.rubric.Definition.Mode != rubric.MentorQualification {
		return len(item.browserQualification) == 0 && item.review.Qualification == nil
	}
	receipt := item.review.Qualification
	return receipt != nil && len(item.browserQualification) == 35 && reflect.DeepEqual(item.browserQualification, receipt.Commands)
}
