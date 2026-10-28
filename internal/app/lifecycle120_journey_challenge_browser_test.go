package app

import (
	"fmt"
	"net/http"
	"os"
	"reflect"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

type lifecycleBrowserChallengeInput struct {
	Input content.Input `json:"input"`
	Saved *content.Page `json:"saved,omitempty"`
}

// The real template creates its own content ID. This native observer never
// writes the challenge when browser authoring is required.
func (r *lifecycleJourney) browserChallengeAuthoring(t *testing.T, item *lifecycleJourneyEvent, index int) func(content.Input) (content.Page, error) {
	t.Helper()
	if os.Getenv("HACKBOX_LIFECYCLE_BROWSER_PHASES") != "ui-journey" {
		return nil
	}
	return func(input content.Input) (content.Page, error) {
		base := "/api/v1/events/" + item.view.ID.String()
		var before event.View
		var directory content.List
		if err := r.owner.call(t.Context(), http.MethodGet, base, -1, nil, &before, http.StatusOK); err != nil {
			return content.Page{}, err
		}
		if err := r.owner.call(t.Context(), http.MethodGet, base+"/content?preview=true&limit=50", -1, nil, &directory, http.StatusOK); err != nil {
			return content.Page{}, err
		}
		if len(directory.Items) != 0 || directory.NextCursor != nil || input.Document.Rubric == nil {
			return content.Page{}, fmt.Errorf("UI challenge must begin with an empty native directory and exact rubric")
		}
		var saved content.Page
		for step, action := range []string{"save-challenge", "review-challenge", "approve-challenge"} {
			phase := lifecycleBrowserPhase{Sequence: 503 + index*20 + step, Action: action, Challenge: &lifecycleBrowserChallengeInput{Input: input}}
			if step > 0 {
				captured := saved
				phase.Challenge.Saved = &captured
			}
			ack := r.admissionBrowserPhase(t, item, phase)
			if !ack.Saved || ack.ContentID == nil {
				return saved, fmt.Errorf("challenge phase lacks an actual UI resource acknowledgement")
			}
			if step > 0 && *ack.ContentID != saved.ID {
				return saved, fmt.Errorf("challenge review changed the original UI-created resource")
			}
			if err := r.owner.call(t.Context(), http.MethodGet, base+"/content/"+ack.ContentID.String()+"?preview=true", -1, nil, &saved, http.StatusOK); err != nil {
				return saved, err
			}
			d := saved.Document
			if saved.Revision != int64(step+1) || saved.Version != 1 || saved.State != []string{"draft", "review", "approved"}[step] ||
				saved.Kind != "challenge" || saved.ReleasedVersion != nil || saved.ReleasedAt != nil || !saved.CanManage ||
				d.Title != input.Document.Title || d.Markdown != input.Document.Markdown || d.Rubric == nil || *d.Rubric != *input.Document.Rubric ||
				!reflect.DeepEqual(d.Audience, input.Document.Audience) || !d.RequireAcknowledgement || len(d.Attachments) != 0 || len(d.Objectives) != 0 || len(d.Contacts) != 0 ||
				d.ScheduledAt == nil || !d.ScheduledAt.Equal(r.timeline.Publication) {
				return saved, fmt.Errorf("native challenge differs from exact UI-authored reference, content, audience, schedule or approval state")
			}
		}
		var after event.View
		if err := r.owner.call(t.Context(), http.MethodGet, base, -1, nil, &after, http.StatusOK); err != nil {
			return saved, err
		}
		if after.PublishedAt != nil || before.Stage != after.Stage || before.ConfirmedAttendees != after.ConfirmedAttendees || !reflect.DeepEqual(before.Configuration, after.Configuration) {
			return saved, fmt.Errorf("UI challenge authoring released content or changed event/census state")
		}
		if err := r.owner.call(t.Context(), http.MethodGet, base+"/content?preview=true&limit=50", -1, nil, &directory, http.StatusOK); err != nil {
			return saved, err
		}
		if len(directory.Items) != 1 || directory.Items[0].ID != saved.ID || directory.NextCursor != nil {
			return saved, fmt.Errorf("challenge authoring duplicated or removed native content")
		}
		item.browserChallengeApproved = true
		return saved, nil
	}
}
