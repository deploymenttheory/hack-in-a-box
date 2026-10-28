package app

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/deliveryhistory"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

type lifecycleFailedMessageReceipt struct {
	EventID        uuid.UUID `json:"event_id"`
	CampaignID     uuid.UUID `json:"campaign_id"`
	DeliveryID     uuid.UUID `json:"delivery_id"`
	AttemptID      uuid.UUID `json:"attempt_id"`
	RecipientID    uuid.UUID `json:"recipient_id"`
	State          string    `json:"state"`
	RetryDenied    bool      `json:"retry_denied"`
	HistoryIntact  bool      `json:"history_intact"`
	NoAcceptedData bool      `json:"no_accepted_data"`
}

// This is part of the same two-event workflow, not the independent SMTP
// checkpoint. Every send uses the approved campaign, native worker and actual
// TLS/auth SMTP relay; only the synthetic provider's RCPT response is faulted.
func (r *lifecycleJourney) rehearseFailedMessages(t *testing.T) {
	t.Helper()
	for i := range r.events {
		r.rehearseFailedMessage(t, &r.events[i])
	}
}

func (r *lifecycleJourney) rehearseFailedMessage(t *testing.T, item *lifecycleJourneyEvent) {
	t.Helper()
	ctx := t.Context()
	base := "/api/v1/events/" + item.view.ID.String() + "/communications"
	var clock struct {
		Now time.Time `json:"now"`
	}
	if err := r.control.call(ctx, http.MethodGet, "/clock", nil, &clock); err != nil {
		t.Fatal(err)
	}
	zone, err := time.LoadLocation(item.view.Configuration.Timezone)
	if err != nil {
		t.Fatal(err)
	}
	template := communications.Template{
		ID: uuid.New(), Name: "Synthetic failed planning reminder", Purpose: "reminder", Format: "text",
		Subject: "Planning reminder for {{event.name}}", Body: "Reviewed organiser-only reminder {{person.name}}",
	}
	audience := communications.Audience{
		ID: uuid.New(), Name: "Current organiser only", Roles: []string{"organiser"},
		RegistrationStates: []string{}, Teams: []uuid.UUID{},
	}
	var page communications.Page
	if err := r.owner.call(ctx, http.MethodGet, base, -1, nil, &page, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	for _, command := range []map[string]any{
		{"action": "save_template", "template": template, "reason": "Synthetic failed-message evidence"},
		{"action": "save_audience", "audience": audience, "reason": "Only the current synthetic organiser"},
	} {
		var next communications.Page
		if err := r.owner.call(ctx, http.MethodPost, base, page.Revision, command, &next, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		page = next
	}
	definition := communications.CampaignDefinition{
		ID: uuid.New(), Name: "Same-event failed SMTP reminder", TemplateID: template.ID, AudienceID: audience.ID,
		Channels: []string{"smtp"}, Timing: communications.CampaignTiming{
			Kind: "fixed", WallTime: clock.Now.Add(5 * time.Second).In(zone).Format("2006-01-02T15:04:05"),
		},
	}
	path := base + "/campaigns/" + definition.ID.String()
	var campaign communications.CampaignDetail
	if err := r.owner.call(ctx, http.MethodPost, path, page.Revision, communications.CampaignCommand{
		Action: "save", Definition: &definition, Reason: "Explicit synthetic failed-message rehearsal",
	}, &campaign, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	campaign, _ = approveLifecycleCampaignSchedule(t, r.owner, item.view.ID, path, campaign)
	if err := r.control.call(ctx, http.MethodPost, "/mail/fault",
		map[string]string{"recipient": "owner@example.test", "mode": "permanent"}, nil); err != nil {
		t.Fatal(err)
	}
	var before struct {
		Items []lifecycleMail `json:"items"`
	}
	if err := r.control.call(ctx, http.MethodGet, "/mail/messages", nil, &before); err != nil {
		t.Fatal(err)
	}
	if err := r.control.advance(ctx, *campaign.Campaign.DueAt); err != nil {
		t.Fatal(err)
	}
	failed := waitLifecycleCampaign(t, r.owner, base, definition.ID, "permanent")
	path = base + "/deliveries/" + failed.ID.String()
	var original deliveryhistory.Detail
	if err := r.owner.call(ctx, http.MethodGet, path, -1, nil, &original, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if len(original.Attempts) != 1 || original.Attempts[0].State != "permanent" ||
		original.Item.OperationState != "permanent" || original.Item.RecipientID == nil ||
		*original.Item.RecipientID != r.owner.person || len(original.Reviews) != 0 {
		t.Fatal("same-event failed message lost its exact native recipient/attempt/terminal state")
	}
	if err := r.owner.call(ctx, http.MethodPost, path, original.Item.Revision,
		deliveryhistory.Command{RequestID: uuid.New(), Action: "retry", Reason: "Permanent SMTP rejection must prohibit retry"},
		nil, http.StatusConflict); err != nil {
		t.Fatal(err)
	}
	var final deliveryhistory.Detail
	if err := r.owner.call(ctx, http.MethodGet, path, -1, nil, &final, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original, final) {
		t.Fatal("rejected permanent-message retry changed original delivery evidence")
	}
	var after struct {
		Items []lifecycleMail `json:"items"`
	}
	if err := r.control.call(ctx, http.MethodGet, "/mail/messages", nil, &after); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.Items, after.Items) {
		t.Fatal("definite failed-message rejection unexpectedly produced SMTP DATA")
	}
	if err := r.control.call(ctx, http.MethodPost, "/mail/fault",
		map[string]string{"recipient": "owner@example.test", "mode": ""}, nil); err != nil {
		t.Fatal(err)
	}
	var current event.View
	if err := r.owner.call(ctx, http.MethodGet, "/api/v1/events/"+item.view.ID.String(),
		-1, nil, &current, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if current.ID != item.view.ID || current.Stage != item.view.Stage || current.PublishedAt != nil {
		t.Fatal("failed-message rehearsal changed the native event stage/publication")
	}
	item.view = current
	item.failedMessage = lifecycleFailedMessageReceipt{
		EventID: item.view.ID, CampaignID: definition.ID, DeliveryID: final.Item.ID,
		AttemptID: original.Attempts[0].ID, RecipientID: r.owner.person, State: final.Item.State,
		RetryDenied: true, HistoryIntact: true, NoAcceptedData: true,
	}
}
