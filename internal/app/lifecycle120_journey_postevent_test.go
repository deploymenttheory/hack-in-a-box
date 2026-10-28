package app

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/media"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/surveys"
	"github.com/google/uuid"
)

type lifecyclePostEventReceipt struct {
	MediaID                       uuid.UUID `json:"media_id"`
	MediaVersion                  int64     `json:"media_version"`
	ReviewedRelease               bool      `json:"reviewed_release"`
	RevokedConsentDenied          bool      `json:"revoked_consent_denied"`
	ReconsentRequiresReview       bool      `json:"reconsent_requires_review"`
	SurveyID                      uuid.UUID `json:"survey_id"`
	SurveyVersion                 int64     `json:"survey_version"`
	Responses                     int       `json:"responses"`
	DuplicateResponseAcknowledged bool      `json:"duplicate_response_acknowledged"`
	PeerResponsesDenied           bool      `json:"peer_responses_denied"`
	CrossEventDenied              bool      `json:"cross_event_denied"`
}

// The deadline settles submissions while the event is still live. Advancing
// stage is a distinct organiser decision, permitted only by the native complete
// current-team outcome census; final judging then uses that frozen source.
func (r *lifecycleJourney) advanceReconciledAssessment(t *testing.T) {
	t.Helper()
	for i := range r.events {
		item := &r.events[i]
		current, err := lifecycleTransition(t.Context(), r.owner, item.view.ID, "assessment", http.StatusOK)
		if err != nil {
			t.Fatal(err)
		}
		if current.Stage != "assessment" {
			t.Fatal("native assessment transition did not retain its stage")
		}
		item.view = current
	}
}

// Continue the same two event/actor lifetimes through real completion. Native
// HTTP owners create all media, consent and survey records; no SQL writes seed
// a publication, invitation or successful response.
func (r *lifecycleJourney) completeAndCollectFeedback(t *testing.T) {
	t.Helper()
	if err := r.control.advance(t.Context(), r.timeline.Completed); err != nil {
		t.Fatal(err)
	}
	for i := range r.events {
		item := &r.events[i]
		var err error
		item.view, err = lifecycleTransition(t.Context(), r.owner, item.view.ID, "completed", http.StatusOK)
		if err != nil {
			t.Fatal(err)
		}
		if item.view.Stage != "completed" || item.resultDecision == nil {
			t.Fatal("completion did not retain the reviewed native competition")
		}
		r.reviewConsentedRecording(t, item)
		r.collectNativeSurvey(t, item)
	}
}

func (r *lifecycleJourney) reviewConsentedRecording(t *testing.T, item *lifecycleJourneyEvent) {
	t.Helper()
	own, peer, withdrawn := r.feedbackReaders(t, item, item.judgeFinal.EligibleTeamIDs[0])
	base := "/api/v1/events/" + item.view.ID.String()
	var choice privacy.OwnPhotoConsent
	if err := own.call(t.Context(), http.MethodGet, base+"/photo-consent", -1, nil, &choice, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	setConsent := func(value string) {
		t.Helper()
		if err := own.call(t.Context(), http.MethodPut, base+"/photo-consent", choice.Revision, map[string]string{"choice": value}, &choice, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if choice.Choice != value {
			t.Fatal("own consent acknowledgement did not match choice")
		}
	}
	setConsent("yes")
	id := uuid.New()
	path := base + "/media/" + id.String()
	var current media.Item
	metadata := media.Metadata{Kind: "recording", Title: "Synthetic reviewed event recording", Caption: "Consented synthetic retrospective", RecordingURL: "https://video.example.test/recordings/" + id.String(), People: []uuid.UUID{own.person}}
	change := func(action string, review *media.Review) {
		t.Helper()
		if err := r.owner.call(t.Context(), http.MethodGet, base, -1, nil, &item.view, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		revision := item.view.Revision
		input := media.Command{Action: action, Version: current.Version, CommandID: uuid.New(), Reason: "Explicit synthetic media review", Review: review}
		if action == "save" {
			input.Metadata = &metadata
		}
		if err := r.owner.call(t.Context(), http.MethodPut, path, revision, input, &current, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if current.AcknowledgedVersion == nil || *current.AcknowledgedVersion != current.Version {
			t.Fatal("media write lacks exact original acknowledgement")
		}
	}
	change("save", nil)
	for _, client := range []*lifecycleClient{own, peer} {
		if err := client.call(t.Context(), http.MethodGet, path, -1, nil, nil, http.StatusNotFound); err != nil {
			t.Fatal(err)
		}
	}
	change("review", &media.Review{ContentReviewed: true, IdentificationReviewed: true})
	change("publish", nil)
	var visible media.Item
	if err := peer.call(t.Context(), http.MethodGet, path, -1, nil, &visible, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if visible.RecordingURL != metadata.RecordingURL || visible.State != "released" || visible.ConsentState != "reviewed" {
		t.Fatal("reviewed native media release is not currently readable")
	}
	if err := withdrawn.call(t.Context(), http.MethodGet, path, -1, nil, nil, http.StatusForbidden); err != nil {
		t.Fatal(err)
	}
	item.postEvent.MediaID = id
	item.postEvent.ReviewedRelease = true
	setConsent("no")
	if err := peer.call(t.Context(), http.MethodGet, path, -1, nil, nil, http.StatusNotFound); err != nil {
		t.Fatal(err)
	}
	item.postEvent.RevokedConsentDenied = true
	setConsent("yes")
	if err := peer.call(t.Context(), http.MethodGet, path, -1, nil, nil, http.StatusNotFound); err != nil {
		t.Fatal(err)
	}
	item.postEvent.ReconsentRequiresReview = true
	if err := r.owner.call(t.Context(), http.MethodGet, path, -1, nil, &current, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	change("save", nil) // Revocation retired the old publication; opt-in cannot revive it.
	change("review", &media.Review{ContentReviewed: true, IdentificationReviewed: true})
	change("publish", nil)
	if err := peer.call(t.Context(), http.MethodGet, path, -1, nil, &visible, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if visible.ConsentState != "reviewed" || visible.RecordingURL != metadata.RecordingURL {
		t.Fatal("renewed consent lacked a new explicit review")
	}
	item.postEvent.MediaVersion = current.Version
}

func (r *lifecycleJourney) collectNativeSurvey(t *testing.T, item *lifecycleJourneyEvent) {
	t.Helper()
	id, question := uuid.New(), uuid.New()
	base := "/api/v1/events/" + item.view.ID.String()
	path := base + "/surveys/" + id.String()
	definition := surveys.Definition{Form: forms.Definition{ID: id, Name: "Synthetic post-event learning", Audience: "attendee", Fields: []forms.Field{{ID: question, Label: "How useful was the event?", Type: "single", Required: true, Choices: []string{"Useful", "Needs work"}}}}, OpensAt: r.timeline.Completed, ClosesAt: r.timeline.Completed.Add(time.Hour), MinimumCohort: 5, Measures: []surveys.QuestionMeasure{}}
	var configuration surveys.Configuration
	if err := r.owner.call(t.Context(), http.MethodPut, path+"/configuration", 0, surveys.Command{Action: "save", Definition: &definition}, &configuration, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"approve", "publish"} {
		if err := r.owner.call(t.Context(), http.MethodPut, path+"/configuration", configuration.Version, surveys.Command{Action: action}, &configuration, http.StatusOK); err != nil {
			t.Fatal(err)
		}
	}
	if configuration.PublishedAt == nil || configuration.ApprovedAt == nil {
		t.Fatal("survey invitations lack explicit approval/publication")
	}
	seen := map[uuid.UUID]bool{}
	for _, team := range item.teams.Teams {
		if team.State != "published" {
			continue
		}
		for _, member := range team.Published {
			if seen[member.ID] || len(seen) >= 6 {
				continue
			}
			seen[member.ID] = true
			client := r.participant(t, member.ID)
			var own surveys.ParticipantSurvey
			if err := client.call(t.Context(), http.MethodGet, path, -1, nil, &own, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if !own.CanRespond || own.Response != nil || own.ResponseVisibility != surveys.ResponseVisibility {
				t.Fatal("new native survey response is not private and available")
			}
			input := surveys.ResponseCommand{CommandID: uuid.New(), FormVersion: configuration.Version, Answers: forms.Answers{question.String(): json.RawMessage(`"Useful"`)}}
			var response surveys.Response
			if err := client.call(t.Context(), http.MethodPut, path+"/response", 0, input, &response, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if response.Revision != 1 {
				t.Fatal("survey did not retain its first original response")
			}
			var repeated surveys.Response
			if err := client.call(t.Context(), http.MethodPut, path+"/response", 0, input, &repeated, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if repeated.Revision != response.Revision || !repeated.SubmittedAt.Equal(response.SubmittedAt) {
				t.Fatal("duplicate survey intent created another accepted response")
			}
			if err := client.call(t.Context(), http.MethodGet, path+"/responses", -1, nil, nil, http.StatusForbidden); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(seen) != 6 {
		t.Fatal("survey lacks the required actual current cohort")
	}
	foreign := r.events[0].view.ID
	if foreign == item.view.ID {
		foreign = r.events[1].view.ID
	}
	if err := r.owner.call(t.Context(), http.MethodGet, "/api/v1/events/"+foreign.String()+"/surveys/"+id.String()+"/configuration", -1, nil, nil, http.StatusNotFound); err != nil {
		t.Fatal(err)
	}
	item.postEvent.SurveyID = id
	item.postEvent.SurveyVersion = configuration.Version
	item.postEvent.Responses = len(seen)
	item.postEvent.DuplicateResponseAcknowledged = true
	item.postEvent.PeerResponsesDenied = true
	item.postEvent.CrossEventDenied = true
}
