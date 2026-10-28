package app

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/google/uuid"
)

type lifecycleBrowserMentorCardInput struct {
	FrozenRevision *int64                 `json:"frozen_revision"`
	Actor          string                 `json:"actor"`
	TeamID         uuid.UUID              `json:"team_id"`
	AuthorID       uuid.UUID              `json:"author_id"`
	Revision       int64                  `json:"revision"`
	Checkpoint     scorecards.Checkpoint  `json:"checkpoint"`
	Command        scorecards.CardCommand `json:"command"`
}

func (r *lifecycleJourney) submitMentorCheckpointInBrowser(t *testing.T, item *lifecycleJourneyEvent, eventIndex, assignmentIndex int, actor string, client *lifecycleClient, teamID uuid.UUID, before scorecards.Page, command scorecards.CardCommand) scorecards.Card {
	t.Helper()
	card := r.submitIndependentMentorCardInBrowser(t, item, 1701+eventIndex*20+assignmentIndex, client, before, lifecycleBrowserMentorCardInput{Actor: actor, TeamID: teamID, AuthorID: client.person, Revision: before.Revision, Checkpoint: item.review.Checkpoint, Command: command})
	item.browserLiveMentorCards = append(item.browserLiveMentorCards, lifecycleMentorCardReceipt{TeamID: card.TeamID, AuthorID: card.AuthorID, Reference: card.Reference, RunID: card.RunID})
	return card
}

func (r *lifecycleJourney) submitIndependentMentorCardInBrowser(t *testing.T, item *lifecycleJourneyEvent, sequence int, client *lifecycleClient, before scorecards.Page, input lifecycleBrowserMentorCardInput) scorecards.Card {
	t.Helper()
	if item.cohort.staff[input.Actor] != client || input.AuthorID != client.person || input.Revision != before.Revision || input.Command.Action != "submit" || input.Command.Version != 0 {
		t.Fatal("mentor card lost its current actor/source or fresh independent intent")
	}
	action := "submit-live-mentor-card"
	if input.Checkpoint.Purpose == "mentor_final" {
		if input.FrozenRevision == nil || *input.FrozenRevision < 1 {
			t.Fatal("final mentor UI lacks its exact frozen formal source")
		}
		action = "submit-final-mentor-card"
	} else if input.Checkpoint.Purpose != "checkpoint" || input.FrozenRevision != nil {
		t.Fatal("live mentor UI acquired a final source")
	}
	ack := r.admissionBrowserPhase(t, item, lifecycleBrowserPhase{Sequence: sequence, Action: action, MentorCard: &input})
	if !ack.Saved || ack.CardID == nil || *ack.CardID == uuid.Nil || ack.CommandID != nil {
		t.Fatal("mentor card requires its actual native identity, not an invented command UUID")
	}
	path := fmt.Sprintf("/api/v1/events/%s/teams/%s/scorecards", item.view.ID, input.TeamID)
	var current scorecards.Page
	if err := client.call(t.Context(), http.MethodGet, path, -1, nil, &current, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if len(current.Items) != len(before.Items)+1 {
		t.Fatal("mentor UI leaked an author or changed its expected own-card census")
	}
	for _, prior := range before.Items {
		found := false
		for _, card := range current.Items {
			if reflect.DeepEqual(prior, card) {
				found = true
			}
		}
		if !found {
			t.Fatal("final mentor submission mutated or carried an earlier independent live card")
		}
	}
	var saved *scorecards.Card
	for _, card := range current.Items {
		if card.AuthorID != client.person {
			t.Fatal("mentor UI exposed another author's private card")
		}
		if card.Reference.ID == *ack.CardID {
			copy := card
			saved = &copy
		}
	}
	if saved == nil {
		t.Fatal("native owner did not retain the actual browser-created card")
	}
	card := *saved
	if card.Reference.Version != 1 || card.Revision != input.Revision+1 || card.AuthorID != client.person || card.TeamID != input.TeamID || card.CheckpointID != input.Command.CheckpointID || card.CheckpointVersion != input.Command.CheckpointVersion || card.Rubric != input.Checkpoint.Rubric || card.RunID != input.Checkpoint.Definition.RunID || card.Purpose != input.Checkpoint.Purpose || card.State != "submitted" || !reflect.DeepEqual(card.SubmissionRevision, input.FrozenRevision) || !reflect.DeepEqual(card.Scores, input.Command.Scores) {
		t.Fatal("browser mentor card differs from its full native author/source/version/content")
	}
	var history scorecards.History
	if err := client.call(t.Context(), http.MethodGet, fmt.Sprintf("%s/%s/history?cursor=2&limit=1", path, *ack.CardID), -1, nil, &history, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if len(history.Items) != 1 || !reflect.DeepEqual(history.Items[0], card) {
		t.Fatal("mentor card lacks its exact immutable native first version")
	}
	return card
}

func lifecycleBrowserMentorCheckpointsComplete(item lifecycleJourneyEvent) bool {
	return len(item.browserLiveMentorCards) == 12 && reflect.DeepEqual(item.browserLiveMentorCards, item.review.Cards)
}

func lifecycleBrowserFinalMentorCardsComplete(item lifecycleJourneyEvent) bool {
	if item.rubric.Definition.Mode != rubric.MentorQualification {
		return len(item.browserFinalMentorCards) == 0 && len(item.review.FinalCards) == 0
	}
	return len(item.browserFinalMentorCards) == 8 && reflect.DeepEqual(item.browserFinalMentorCards, item.review.FinalCards)
}
