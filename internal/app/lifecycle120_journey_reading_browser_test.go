package app

import (
	"fmt"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

type lifecycleBrowserReadingInput struct {
	Actor            string            `json:"actor"`
	Resource         content.Page      `json:"resource"`
	Rubric           rubric.PublicPage `json:"public_rubric"`
	PrivateObjective string            `json:"private_objective_canary"`
}

func (r *lifecycleJourney) acknowledgePublicationInBrowser(t *testing.T, item *lifecycleJourneyEvent, eventIndex, readerIndex int, reader *lifecycleClient, released content.Page, public rubric.PublicPage) {
	t.Helper()
	if released.AcknowledgedAt != nil || !released.Document.RequireAcknowledgement {
		t.Fatal("normal UI reading requires a new exact-version acknowledgement")
	}
	alias := ""
	for i, actor := range item.cohort.clients {
		if actor.person == reader.person {
			alias = fmt.Sprintf("attendee-%02d", i+1)
		}
	}
	if alias == "" {
		t.Fatal("resource reader lacks a genuine normal-OIDC cohort identity")
	}
	ack := r.admissionBrowserPhase(t, item, lifecycleBrowserPhase{Sequence: 801 + eventIndex*20 + readerIndex, Action: "acknowledge-common-publication", Reading: &lifecycleBrowserReadingInput{Actor: alias, Resource: released, Rubric: public, PrivateObjective: item.objective.Definition.Description}})
	if !ack.Saved || ack.ContentID == nil || *ack.ContentID != released.ID {
		t.Fatal("reading lacks an exact UI acknowledgement of the released resource")
	}
}
