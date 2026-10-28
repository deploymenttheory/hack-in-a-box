package app

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/google/uuid"
)

type lifecycleBrowserAskInput struct {
	Actor  string    `json:"actor"`
	TeamID uuid.UUID `json:"team_id"`
	Topic  string    `json:"topic"`
	Body   string    `json:"body"`
}
type lifecycleBrowserAnswerInput struct {
	Question clarification.Question            `json:"question"`
	Before   clarification.AnswerConfiguration `json:"before"`
	Document clarification.AnswerDocument      `json:"document"`
	Version  int64                             `json:"version"`
}

func (r *lifecycleJourney) askClarificationInBrowser(t *testing.T, item *lifecycleJourneyEvent, index int, actor *lifecycleClient, input clarification.QuestionInput) clarification.Question {
	t.Helper()
	alias := ""
	for i, person := range item.cohort.clients {
		if person.person == actor.person {
			alias = fmt.Sprintf("attendee-%02d", i+1)
		}
	}
	if alias == "" {
		t.Fatal("private question lacks its actual normal-OIDC cohort actor")
	}
	ack := r.admissionBrowserPhase(t, item, lifecycleBrowserPhase{Sequence: 1001 + index*20, Action: "ask-common-question", Ask: &lifecycleBrowserAskInput{Actor: alias, TeamID: input.TeamID, Topic: input.Topic, Body: input.Body}})
	if !ack.Saved || ack.QuestionID == nil || *ack.QuestionID == uuid.Nil || ack.CommandID == nil || *ack.CommandID == uuid.Nil {
		t.Fatal("private question lacks actual UI-generated question/command identities")
	}
	var question clarification.Question
	if err := actor.call(t.Context(), http.MethodGet, "/api/v1/events/"+item.view.ID.String()+"/questions/"+ack.QuestionID.String(), -1, nil, &question, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if question.ID != *ack.QuestionID || question.Topic != input.Topic || len(question.Attachments) != 0 {
		t.Fatal("UI question changed intended topic, attachments or native resource")
	}
	item.clarification.QuestionCommandID = *ack.CommandID
	item.browserClarificationActions = append(item.browserClarificationActions, "ask")
	return question
}

func (r *lifecycleJourney) answerClarificationInBrowser(t *testing.T, item *lifecycleJourneyEvent, index int, version int64, action string, question clarification.Question, before clarification.AnswerConfiguration, document clarification.AnswerDocument) clarification.AnswerConfiguration {
	t.Helper()
	offset := 0
	switch action {
	case "save":
	case "approve":
		offset = 1
	case "publish":
		offset = 2
	default:
		t.Fatal("unsupported UI answer action")
	}
	ack := r.admissionBrowserPhase(t, item, lifecycleBrowserPhase{Sequence: 1002 + index*20 + int(version-1)*3 + offset, Action: "answer-" + action, Answer: &lifecycleBrowserAnswerInput{Question: question, Before: before, Document: document, Version: version}})
	if !ack.Saved || ack.QuestionID == nil || *ack.QuestionID != question.ID {
		t.Fatal("common answer lacks its exact real UI question acknowledgement")
	}
	var after clarification.AnswerConfiguration
	if err := r.owner.call(t.Context(), http.MethodGet, "/api/v1/events/"+item.view.ID.String()+"/questions/"+question.ID.String()+"/answer", -1, nil, &after, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision+1 || after.Version != version || after.ID == uuid.Nil || after.QuestionID != question.ID || after.Source != question.Source || after.Document != document || before.ID != uuid.Nil && after.ID != before.ID {
		t.Fatal("UI common answer changed its exact native source, version, document or resource")
	}
	if action == "save" && (after.Approved || after.Published) || action == "approve" && (!after.Approved || after.Published) || action == "publish" && (!after.Approved || !after.Published) {
		t.Fatal("UI common answer inferred an approval or publication")
	}
	item.browserClarificationActions = append(item.browserClarificationActions, fmt.Sprintf("%s-%d", action, version))
	return after
}

func lifecycleBrowserClarificationComplete(actions []string) bool {
	return reflect.DeepEqual(actions, []string{"ask", "save-1", "approve-1", "publish-1", "save-2", "approve-2", "publish-2"})
}
