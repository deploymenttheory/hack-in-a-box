package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/deploymenttheory/hack-in-a-box/internal/inbox"
	"github.com/google/uuid"
)

type lifecycleClarificationVersion struct {
	Version         int64       `json:"version"`
	Revision        int64       `json:"revision"`
	DocumentSHA256  string      `json:"document_sha256"`
	NotificationIDs []uuid.UUID `json:"notification_ids"`
	RecipientIDs    []uuid.UUID `json:"recipient_ids"`
}
type lifecycleClarificationReceipt struct {
	TeamID                        uuid.UUID                       `json:"team_id"`
	QuestionID                    uuid.UUID                       `json:"question_id"`
	QuestionCommandID             uuid.UUID                       `json:"question_command_id"`
	AnswerID                      uuid.UUID                       `json:"answer_id"`
	Source                        clarification.Source            `json:"source"`
	OtherTeamQuestionDenied       bool                            `json:"other_team_question_denied"`
	PrivateDraftDenied            bool                            `json:"private_draft_denied"`
	UnapprovedPublicationDenied   bool                            `json:"unapproved_publication_denied"`
	CorrectionDraftKeptOldRelease bool                            `json:"correction_draft_kept_old_release"`
	Versions                      []lifecycleClarificationVersion `json:"versions"`
}

func lifecyclePrivateQuestion(teamID, commandID uuid.UUID) clarification.QuestionInput {
	return clarification.QuestionInput{TeamID: teamID, CommandID: commandID, Topic: "challenge", Body: "PRIVATE synthetic clarification question; never include this marker in the common answer.", Attachments: []uuid.UUID{}}
}
func lifecycleCommonAnswer(corrected bool) clarification.AnswerDocument {
	body := "Use the published common challenge and rubric; retain the team's own evidence."
	if corrected {
		body = "Correction: retain the team's own evidence and explain its limitations against the unchanged published rubric."
	}
	return clarification.AnswerDocument{Title: "Common evidence clarification", Body: body, Kind: "clarification"}
}

func lifecycleInboxSnapshot(t *testing.T, client *lifecycleClient, eventID uuid.UUID) map[uuid.UUID]bool {
	t.Helper()
	var page inbox.Page
	if err := client.call(t.Context(), http.MethodGet, "/api/v1/inbox?event_id="+eventID.String()+"&limit=100", -1, nil, &page, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if page.NextCursor != nil {
		t.Fatal("clarification inbox snapshot cannot silently omit paginated references")
	}
	ids := map[uuid.UUID]bool{}
	for _, item := range page.Items {
		if item.EventID != eventID || ids[item.ID] {
			t.Fatal("inbox snapshot mixed event or duplicated a reference")
		}
		ids[item.ID] = true
	}
	return ids
}

func (r *lifecycleJourney) publishCommonClarifications(t *testing.T) {
	t.Helper()
	for index := range r.events {
		item := &r.events[index]
		proof := &item.clarification
		proof.TeamID = item.liveHelp.TeamID
		own, other, _ := r.feedbackReaders(t, item, proof.TeamID)
		base := "/api/v1/events/" + item.view.ID.String()
		proof.QuestionCommandID = uuid.New()
		questionInput := lifecyclePrivateQuestion(proof.TeamID, proof.QuestionCommandID)
		var question clarification.Question
		if err := own.call(t.Context(), http.MethodPost, base+"/questions", -1, questionInput, &question, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if question.ID == uuid.Nil || question.TeamID != proof.TeamID || question.AuthorID != own.person || question.Revision != 1 || question.Body != questionInput.Body || question.Source.ChallengeID != item.challenge.ID || question.Source.ChallengeVersion != item.challenge.Version || question.Source.Rubric != item.rubric.Reference {
			t.Fatal("private question lost the actual common source or current team author")
		}
		proof.QuestionID, proof.Source = question.ID, question.Source
		questionPath := base + "/questions/" + question.ID.String()
		if err := other.call(t.Context(), http.MethodGet, questionPath, -1, nil, nil, http.StatusNotFound); err != nil {
			t.Fatal(err)
		}
		proof.OtherTeamQuestionDenied = true
		var draft clarification.AnswerConfiguration
		for version := int64(1); version <= 2; version++ {
			before := make([]map[uuid.UUID]bool, len(item.cohort.clients))
			for i, client := range item.cohort.clients {
				before[i] = lifecycleInboxSnapshot(t, client, item.view.ID)
			}
			var old clarification.PublicPage
			if version == 2 {
				if err := own.call(t.Context(), http.MethodGet, base+"/clarifications/"+draft.ID.String(), -1, nil, &old, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				if err := r.control.nextAction(t.Context()); err != nil {
					t.Fatal(err)
				}
			}
			document := lifecycleCommonAnswer(version == 2)
			if err := r.owner.call(t.Context(), http.MethodPut, questionPath+"/answer", draft.Revision, document, &draft, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if draft.ID == uuid.Nil || draft.QuestionID != question.ID || draft.Version != version || draft.Approved || draft.Published || draft.Source != proof.Source || draft.Document != document {
				t.Fatal("authored common answer inferred approval/publication or changed source")
			}
			proof.AnswerID = draft.ID
			publicPath := base + "/clarifications/" + draft.ID.String()
			if err := own.call(t.Context(), http.MethodGet, questionPath+"/answer", -1, nil, nil, http.StatusNotFound); err != nil {
				t.Fatal(err)
			}
			proof.PrivateDraftDenied = true
			if version == 1 {
				if err := own.call(t.Context(), http.MethodGet, publicPath, -1, nil, nil, http.StatusNotFound); err != nil {
					t.Fatal(err)
				}
			} else {
				var retained clarification.PublicPage
				if err := own.call(t.Context(), http.MethodGet, publicPath, -1, nil, &retained, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(old, retained) {
					t.Fatal("unapproved correction replaced an already published answer")
				}
				proof.CorrectionDraftKeptOldRelease = true
			}
			if err := r.owner.call(t.Context(), http.MethodPost, publicPath+"/actions", draft.Revision, clarification.AnswerCommand{Action: "publish", Version: version}, nil, http.StatusConflict); err != nil {
				t.Fatal(err)
			}
			var unchanged clarification.AnswerConfiguration
			if err := r.owner.call(t.Context(), http.MethodGet, questionPath+"/answer", -1, nil, &unchanged, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(draft, unchanged) {
				t.Fatal("denied publication mutated the retained draft")
			}
			proof.UnapprovedPublicationDenied = true
			for i, client := range item.cohort.clients {
				if !reflect.DeepEqual(before[i], lifecycleInboxSnapshot(t, client, item.view.ID)) {
					t.Fatal("unapproved answer emitted a recipient notification")
				}
			}
			for _, action := range []string{"approve", "publish"} {
				if err := r.owner.call(t.Context(), http.MethodPost, publicPath+"/actions", draft.Revision, clarification.AnswerCommand{Action: action, Version: version}, &draft, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				if draft.Version != version || !draft.Approved || (action == "publish" && !draft.Published) {
					t.Fatal("explicit exact-version answer action was not retained")
				}
			}
			var common clarification.PublicPage
			if err := other.call(t.Context(), http.MethodGet, publicPath, -1, nil, &common, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if common.Answer.ID != draft.ID || common.Answer.Version != version || common.Answer.Document != document || !common.Answer.Current || !common.Answer.SourceCurrent || common.Answer.Source != proof.Source || len(common.History) != int(version) || common.HistoryHasMore {
				t.Fatal("common answer did not expose the exact reviewed current version/history")
			}
			raw, err := json.Marshal(common)
			if err != nil {
				t.Fatal(err)
			}
			for _, private := range []string{questionInput.Body, question.ID.String(), proof.TeamID.String()} {
				if strings.Contains(string(raw), private) {
					t.Fatal("common answer disclosed private question or team context")
				}
			}
			if version == 2 {
				found := false
				for _, prior := range common.History {
					if prior.Version == 1 {
						copy := prior
						copy.Current = old.Answer.Current
						if copy != old.Answer {
							t.Fatal("correction changed immutable earlier public answer")
						}
						found = true
					}
				}
				if !found {
					t.Fatal("correction omitted earlier common answer")
				}
			}
			digest := sha256.Sum256([]byte(document.Title + "\n" + document.Body + "\n" + document.Kind))
			receipt := lifecycleClarificationVersion{Version: version, Revision: draft.Revision, DocumentSHA256: hex.EncodeToString(digest[:]), NotificationIDs: []uuid.UUID{}, RecipientIDs: []uuid.UUID{}}
			for i, client := range item.cohort.clients {
				after := lifecycleInboxSnapshot(t, client, item.view.ID)
				added := []uuid.UUID{}
				for id := range after {
					if !before[i][id] {
						added = append(added, id)
					}
				}
				expected := 0
				if item.cohort.registrations[i].State == "confirmed" {
					expected = 1
				}
				if len(added) != expected {
					t.Fatal("common answer inbox delta did not match the actual current confirmed audience", len(added), expected)
				}
				if expected == 0 {
					continue
				}
				var detail inbox.Detail
				if err := client.call(t.Context(), http.MethodGet, "/api/v1/inbox/"+added[0].String(), -1, nil, &detail, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				if detail.Item.EventID != item.view.ID || detail.Destination == nil || detail.Destination.Kind != "clarification" || detail.Destination.ResourceID != draft.ID {
					t.Fatal("common notification did not route through its actual owning answer")
				}
				encoded, err := json.Marshal(detail)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(encoded), questionInput.Body) || strings.Contains(string(encoded), question.ID.String()) {
					t.Fatal("common notification disclosed the private question")
				}
				receipt.NotificationIDs = append(receipt.NotificationIDs, added[0])
				receipt.RecipientIDs = append(receipt.RecipientIDs, client.person)
			}
			if len(receipt.RecipientIDs) != 29 {
				t.Fatal("common answer did not retain exactly29 current confirmed recipients after post-freeze withdrawals")
			}
			proof.Versions = append(proof.Versions, receipt)
			r.persistWorkProgress(t)
		}
	}
}
