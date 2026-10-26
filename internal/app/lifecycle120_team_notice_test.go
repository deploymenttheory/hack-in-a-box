package app

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/automation"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/inbox"
)

type lifecycleTeamNoticeReceipt struct {
	CampaignID          uuid.UUID `json:"campaign_id"`
	CampaignVersion     int64     `json:"campaign_version"`
	BindingID           uuid.UUID `json:"binding_id"`
	BindingVersion      int64     `json:"binding_version"`
	ApprovedAt          time.Time `json:"approved_at"`
	DueAt               time.Time `json:"due_at"`
	CurrentSourceID     uuid.UUID `json:"current_source_id"`
	ObsoleteSourceID    uuid.UUID `json:"obsolete_source_id"`
	OccurrenceID        uuid.UUID `json:"occurrence_id"`
	Recipients          int       `json:"current_recipients"`
	ContentSHA256       string    `json:"content_sha256"`
	ObservedWallSeconds float64   `json:"observed_wall_seconds"`
	subject, body       string
}

// Review after depletion/reassignment but before its real publication. Native
// approval must not retrospectively authorise an earlier lifecycle occurrence.
func (r *lifecycleJourney) configureTeamNotice(t *testing.T, item *lifecycleJourneyEvent) {
	t.Helper()
	ctx := t.Context()
	var phase struct {
		Now time.Time `json:"now"`
	}
	if err := r.control.call(ctx, http.MethodGet, "/clock", nil, &phase); err != nil {
		t.Fatal(err)
	}
	zone, err := time.LoadLocation(item.view.Configuration.Timezone)
	if err != nil {
		t.Fatal(err)
	}
	due := phase.Now.Add(5 * time.Second)
	base := "/api/v1/events/" + item.view.ID.String() + "/communications"
	template := communications.Template{ID: uuid.New(), Name: "Current reviewed team allocation", Purpose: "preparation", Format: "text", Subject: "Current teams for {{event.name}}", Body: "Review the current allocation at {{event.workspace}}."}
	audience := communications.Audience{ID: uuid.New(), Name: "Current confirmed competitors", Roles: []string{"attendee"}, RegistrationStates: []string{}, Teams: []uuid.UUID{}}
	var page communications.Page
	if err := r.owner.call(ctx, http.MethodGet, base, -1, nil, &page, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	for _, command := range []map[string]any{{"action": "save_template", "template": template, "reason": "Reviewed generic roster notice, no private source prose"}, {"action": "save_audience", "audience": audience, "reason": "Only current confirmed competitors"}} {
		var next communications.Page
		if err := r.owner.call(ctx, http.MethodPost, base, page.Revision, command, &next, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		page = next
	}
	definition := communications.CampaignDefinition{ID: uuid.New(), Name: "Corrected team publication", TemplateID: template.ID, AudienceID: audience.ID, Channels: []string{"in_app"}, Timing: communications.CampaignTiming{Kind: "fixed", WallTime: due.In(zone).Format("2006-01-02T15:04:05"), DSTChoice: ""}}
	path := base + "/campaigns/" + definition.ID.String()
	var campaign communications.CampaignDetail
	if err := r.owner.call(ctx, http.MethodPost, path, page.Revision, communications.CampaignCommand{Action: "save", Definition: &definition, Reason: "Prepare notice before actual corrected publication"}, &campaign, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"preview", "approve"} {
		var next communications.CampaignDetail
		if err := r.owner.call(ctx, http.MethodPost, path, campaign.Revision, communications.CampaignCommand{Action: action, Version: campaign.Campaign.Campaign.Version, Reason: "Review exact current thirty-two-person audience before source publication"}, &next, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		campaign = next
	}
	if campaign.Preview == nil || len(campaign.Preview.Recipients) != 32 || campaign.Campaign.Campaign.Approval == nil || campaign.Campaign.DueAt == nil || !campaign.Campaign.DueAt.Equal(due) {
		t.Fatal("native team notice did not bind current confirmed audience and future due time")
	}
	approvedPeople := map[uuid.UUID]bool{}
	for _, p := range campaign.Preview.Recipients {
		approvedPeople[p.PersonID] = true
	}
	confirmed := 0
	for i, registration := range item.cohort.registrations {
		if registration.State == "confirmed" {
			confirmed++
			if !approvedPeople[item.cohort.clients[i].person] {
				t.Fatal("team notice omitted a current confirmed person")
			}
			delete(approvedPeople, item.cohort.clients[i].person)
		}
	}
	if confirmed != 32 || len(approvedPeople) != 0 {
		t.Fatal("team notice admitted former competitors or staff")
	}
	bindingDefinition := automation.Definition{ID: uuid.New(), CampaignID: definition.ID, CampaignVersion: campaign.Campaign.Campaign.Version, Trigger: "teams.published", MaximumLatenessSeconds: 60}
	var changed struct {
		Page automation.Page `json:"page"`
	}
	if err := r.owner.call(ctx, http.MethodPost, base+"/automation", campaign.Revision, map[string]any{"action": "save", "definition": bindingDefinition, "version": 0, "reason": "Bind real team publication facts before the corrected publication"}, &changed, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	var binding *automation.Binding
	for i := range changed.Page.Bindings {
		if changed.Page.Bindings[i].Definition.ID == bindingDefinition.ID {
			binding = &changed.Page.Bindings[i]
		}
	}
	if binding == nil {
		t.Fatal("native teams source binding not retained")
	}
	if err := r.owner.call(ctx, http.MethodPost, base+"/automation", changed.Page.Revision, map[string]any{"action": "approve", "binding_id": bindingDefinition.ID, "version": binding.Version, "reason": "Approve exact current message and audience for the later native source"}, &changed, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	for i := range changed.Page.Bindings {
		if changed.Page.Bindings[i].Definition.ID == bindingDefinition.ID {
			binding = &changed.Page.Bindings[i]
		}
	}
	if binding.ApprovedAt == nil || !binding.ApprovedAt.Equal(phase.Now) {
		t.Fatal("native lifecycle binding did not retain its actual approval instant")
	}
	item.teamNotice = lifecycleTeamNoticeReceipt{CampaignID: definition.ID, CampaignVersion: bindingDefinition.CampaignVersion, BindingID: bindingDefinition.ID, BindingVersion: binding.Version, ApprovedAt: *binding.ApprovedAt, DueAt: due, subject: campaign.Preview.Recipients[0].Subject, body: campaign.Preview.Recipients[0].Body}
}

func (r *lifecycleJourney) waitTeamNotice(t *testing.T, item *lifecycleJourneyEvent) {
	t.Helper()
	ctx := t.Context()
	if err := r.control.advance(ctx, item.teamNotice.DueAt); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	deadline := started.Add(20 * time.Second)
	path := "/api/v1/events/" + item.view.ID.String() + "/communications/automation"
	for time.Now().Before(deadline) {
		var page automation.Page
		if err := r.owner.call(ctx, http.MethodGet, path, -1, nil, &page, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		var current, old *automation.Occurrence
		count := 0
		for i := range page.Occurrences {
			o := &page.Occurrences[i]
			if o.BindingID != item.teamNotice.BindingID {
				continue
			}
			count++
			switch o.State {
			case "queued":
				current = o
			case "missed":
				if o.Reason == "source_precedes_approval" {
					old = o
				}
			}
		}
		if count > 2 {
			t.Fatal("team source generated duplicate native occurrences")
		}
		if count == 2 && current != nil && old != nil {
			if current.SourceID == old.SourceID || current.SourceRevision != 1 || current.ChannelReports["in_app"].LocalPersisted != 32 || old.ChannelReports["in_app"].LocalPersisted != 0 {
				t.Fatal("current/obsolete team source effects were not distinct and exact")
			}
			item.teamNotice.CurrentSourceID = current.SourceID
			item.teamNotice.ObsoleteSourceID = old.SourceID
			item.teamNotice.OccurrenceID = current.ID
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
	if item.teamNotice.CurrentSourceID == uuid.Nil {
		t.Fatal("actual queued latest-source roster notice did not deliver within its normal worker budget")
	}
	count := 0
	for i, client := range item.cohort.clients {
		var page inbox.Page
		if err := client.call(ctx, http.MethodGet, "/api/v1/inbox?event_id="+item.view.ID.String(), -1, nil, &page, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if item.cohort.registrations[i].State != "confirmed" {
			if len(page.Items) != 0 {
				t.Fatal("withdrawn competitor received current roster notice")
			}
			continue
		}
		if len(page.Items) != 1 {
			t.Fatal("current recipient did not receive exactly one native roster notification", len(page.Items))
		}
		var detail inbox.Detail
		if err := client.call(ctx, http.MethodGet, "/api/v1/inbox/"+page.Items[0].ID.String(), -1, nil, &detail, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if detail.Subject != item.teamNotice.subject || detail.Body != item.teamNotice.body || detail.Item.EventID != item.view.ID {
			t.Fatal("native roster inbox changed the reviewed content or event scope")
		}
		count++
	}
	if count != 32 {
		t.Fatal("native roster notification census was incomplete")
	}
	sum := sha256.Sum256([]byte(item.teamNotice.subject + "\n" + item.teamNotice.body))
	item.teamNotice.ContentSHA256 = hex.EncodeToString(sum[:])
	item.teamNotice.Recipients = count
	item.teamNotice.ObservedWallSeconds = time.Since(started).Seconds()
	t.Logf("native team notice: current=%s obsolete=%s recipients=%d", item.teamNotice.CurrentSourceID, item.teamNotice.ObsoleteSourceID, count)
}
