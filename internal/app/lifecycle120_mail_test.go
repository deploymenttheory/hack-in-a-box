package app

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/mailcontact"
	"github.com/deploymenttheory/hack-in-a-box/internal/smtpintegration"
	"github.com/google/uuid"
)

// This checkpoint exercises the native queue and transport, not merely the
// campaign owner or a successful fake transport. It does not complete L11.
func TestLifecycleComposeSMTP(t *testing.T) {
	if os.Getenv("HACKBOX_LIFECYCLE_ISOLATED") != "synthetic-only" {
		t.Skip("isolated Compose checkpoint only")
	}
	ctx := t.Context()
	control, err := newLifecycleControl(os.Getenv("HACKBOX_LIFECYCLE_CONTROL"))
	if err != nil {
		t.Fatal(err)
	}
	defer control.close()
	timeline, err := control.initialTimeline(ctx)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := signInLifecycle(ctx, "http://admin:8080", "http://provider:8080", "owner")
	if err != nil {
		t.Fatal(err)
	}
	var contact mailcontact.Status
	if err := owner.call(ctx, http.MethodGet, "/api/v1/account/email", -1, nil, &contact, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if !contact.Configured || !contact.Verified {
		t.Fatal("normal OIDC contact was not verified by the native account owner")
	}
	var current event.View
	if err := owner.call(ctx, http.MethodPost, "/api/v1/events", -1, lifecycleEventInput("SMTP", timeline), &current, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/events/" + current.ID.String() + "/communications"
	template := communications.Template{ID: uuid.New(), Name: "Synthetic relay checkpoint", Purpose: "reminder", Format: "html", Subject: "Synthetic SMTP {{event.name}}", Body: "Reviewed organiser-only TLS transport checkpoint {{person.name}}"}
	audience := communications.Audience{ID: uuid.New(), Name: "Confirmed competitors", Roles: []string{"attendee"}, RegistrationStates: []string{}, Teams: []uuid.UUID{}}
	var page communications.Page
	if err := owner.call(ctx, http.MethodGet, base, -1, nil, &page, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	for _, command := range []communications.Command{{Action: "save_template", Template: &template, Reason: "Synthetic transport source"}, {Action: "save_audience", Audience: &audience, Reason: "Synthetic transport audience"}} {
		var next communications.Page
		if err := owner.call(ctx, http.MethodPost, base, page.Revision, command, &next, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		page = next
	}
	definition := communications.CampaignDefinition{ID: uuid.New(), Name: "Synthetic queued SMTP", TemplateID: template.ID, AudienceID: audience.ID, Channels: []string{"smtp"}, Timing: communications.CampaignTiming{Kind: "relative", Anchor: "live_start", Days: -1}}
	campaignPath := base + "/campaigns/" + definition.ID.String()
	var detail communications.CampaignDetail
	if err := owner.call(ctx, http.MethodPost, campaignPath, page.Revision, communications.CampaignCommand{Action: "save", Definition: &definition, Reason: "Synthetic reviewed native transport"}, &detail, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	var preview communications.SelfTestPreview
	if err := owner.call(ctx, http.MethodGet, campaignPath+"/self-test-preview?channel=smtp", -1, nil, &preview, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if preview.ActorID != owner.person || preview.ReviewFingerprint == "" || !strings.HasPrefix(preview.Subject, "[TEST] ") {
		t.Fatal("native self-test review not bound to requesting organiser")
	}
	if err := owner.call(ctx, http.MethodPost, campaignPath, preview.Revision, communications.CampaignCommand{Action: "test_self", Version: preview.CampaignVersion, Channel: "smtp", ReviewedTestFingerprint: preview.ReviewFingerprint, Reason: "Explicit exact organiser-only transport review"}, &detail, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if detail.TestRequestID == nil {
		t.Fatal("native test request omitted durable identity")
	}
	statusPath := base + "/tests/" + detail.TestRequestID.String() + "/delivery"
	status, err := waitLifecycleSMTP(ctx, owner, statusPath, "accepted")
	if err != nil {
		t.Fatal(err)
	}
	var mail struct {
		Items []lifecycleMail `json:"items"`
	}
	if err := control.call(ctx, http.MethodGet, "/mail/messages", nil, &mail); err != nil {
		t.Fatal(err)
	}
	if len(mail.Items) != 1 || mail.Items[0].Recipient != "owner@example.test" || !mail.Items[0].Accepted || len(mail.Items[0].Digest) != 64 || !strings.Contains(mail.Items[0].Body, "Reviewed organiser-only TLS transport checkpoint") {
		t.Fatal("native accepted receipt did not match exactly one private synthetic relay message")
	}
	receipt, err := json.MarshalIndent(map[string]any{"checkpoint": "queued-smtp-accepted-only", "complete_lifecycle": false, "event_id": current.ID, "test_id": detail.TestRequestID, "state": status.State, "reason": status.Reason, "mail_sha256": mail.Items[0].Digest, "relay_count": len(mail.Items), "assertions": []string{"native-oidc-verified-contact", "reviewed-requesting-organiser", "real-river-worker", "tls-auth-smtp", "native-bounded-accepted-receipt", "single-private-relay-message"}}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("/artifacts", "smtp-accepted.json"), receipt, 0600); err != nil {
		t.Fatal(err)
	}
}

func waitLifecycleSMTP(ctx context.Context, actor *lifecycleClient, path, want string) (smtpintegration.DeliveryStatus, error) {
	deadline := time.Now().Add(45 * time.Second)
	var status smtpintegration.DeliveryStatus
	for time.Now().Before(deadline) {
		if err := actor.call(ctx, http.MethodGet, path, -1, nil, &status, http.StatusOK); err != nil {
			return status, err
		}
		if status.State == want {
			return status, nil
		}
		select {
		case <-ctx.Done():
			return status, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	return status, &lifecycleSMTPStateError{state: status.State, want: want}
}

type lifecycleSMTPStateError struct{ state, want string }

func (e *lifecycleSMTPStateError) Error() string {
	return "native SMTP state " + e.state + " did not reach " + e.want
}
