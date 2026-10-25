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

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/automation"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/deliveryhistory"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/mailcontact"
	"github.com/deploymenttheory/hack-in-a-box/internal/smtpintegration"
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
	if err := owner.call(
		ctx,
		http.MethodGet,
		"/api/v1/account/email",
		-1,
		nil,
		&contact,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	if !contact.Configured || !contact.Verified {
		t.Fatal("normal OIDC contact was not verified by the native account owner")
	}
	var current event.View
	if err := owner.call(
		ctx,
		http.MethodPost,
		"/api/v1/events",
		-1,
		lifecycleEventInput("SMTP", timeline),
		&current,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/events/" + current.ID.String() + "/communications"
	template := communications.Template{
		ID:      uuid.New(),
		Name:    "Synthetic relay checkpoint",
		Purpose: "reminder",
		Format:  "html",
		Subject: "Synthetic SMTP {{event.name}}",
		Body:    "Reviewed organiser-only TLS transport checkpoint {{person.name}}",
	}
	audience := communications.Audience{
		ID:                 uuid.New(),
		Name:               "Current organisers",
		Roles:              []string{"organiser"},
		RegistrationStates: []string{},
		Teams:              []uuid.UUID{},
	}
	var page communications.Page
	if err := owner.call(ctx, http.MethodGet, base, -1, nil, &page, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	for _, command := range []map[string]any{{"action": "save_template", "template": template, "reason": "Synthetic transport source"}, {"action": "save_audience", "audience": audience, "reason": "Synthetic transport audience"}} {
		var next communications.Page
		if err := owner.call(
			ctx,
			http.MethodPost,
			base,
			page.Revision,
			command,
			&next,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		page = next
	}
	definition := communications.CampaignDefinition{
		ID:         uuid.New(),
		Name:       "Synthetic queued SMTP",
		TemplateID: template.ID,
		AudienceID: audience.ID,
		Channels:   []string{"smtp"},
		Timing: communications.CampaignTiming{
			Kind:   "relative",
			Anchor: "live_start",
			Days:   -15,
		},
	}
	campaignPath := base + "/campaigns/" + definition.ID.String()
	var detail communications.CampaignDetail
	if err := owner.call(
		ctx,
		http.MethodPost,
		campaignPath,
		page.Revision,
		communications.CampaignCommand{
			Action:     "save",
			Definition: &definition,
			Reason:     "Synthetic reviewed native transport",
		},
		&detail,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	var preview communications.SelfTestPreview
	if err := owner.call(
		ctx,
		http.MethodGet,
		campaignPath+"/self-test-preview?channel=smtp",
		-1,
		nil,
		&preview,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	if preview.ActorID != owner.person || preview.ReviewFingerprint == "" ||
		!strings.HasPrefix(preview.Subject, "[TEST] ") {
		t.Fatal("native self-test review not bound to requesting organiser")
	}
	if err := owner.call(
		ctx,
		http.MethodPost,
		campaignPath,
		preview.Revision,
		communications.CampaignCommand{
			Action:                  "test_self",
			Version:                 preview.CampaignVersion,
			Channel:                 "smtp",
			ReviewedTestFingerprint: preview.ReviewFingerprint,
			Reason:                  "Explicit exact organiser-only transport review",
		},
		&detail,
		http.StatusOK,
	); err != nil {
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
	if len(mail.Items) != 1 || mail.Items[0].Recipient != "owner@example.test" ||
		!mail.Items[0].Accepted ||
		len(mail.Items[0].Digest) != 64 ||
		!strings.Contains(mail.Items[0].Body, "Reviewed organiser-only TLS transport checkpoint") {
		t.Fatal("native accepted receipt did not match exactly one private synthetic relay message")
	}
	receipt, err := json.MarshalIndent(
		map[string]any{
			"checkpoint":         "queued-smtp-accepted-only",
			"complete_lifecycle": false,
			"event_id":           current.ID,
			"test_id":            detail.TestRequestID,
			"state":              status.State,
			"reason":             status.Reason,
			"mail_sha256":        mail.Items[0].Digest,
			"relay_count":        len(mail.Items),
			"assertions": []string{
				"native-oidc-verified-contact",
				"reviewed-requesting-organiser",
				"real-river-worker",
				"tls-auth-smtp",
				"native-bounded-accepted-receipt",
				"single-private-relay-message",
			},
		},
		"",
		"  ",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join("/artifacts", "smtp-accepted.json"),
		receipt,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	exerciseLifecycleUnknownCampaign(t, control, owner, current.ID, campaignPath, detail)
}

func waitLifecycleSMTP(
	ctx context.Context,
	actor *lifecycleClient,
	path, want string,
) (smtpintegration.DeliveryStatus, error) {
	deadline := time.Now().Add(45 * time.Second)
	var status smtpintegration.DeliveryStatus
	for time.Now().Before(deadline) {
		if err := actor.call(
			ctx,
			http.MethodGet,
			path,
			-1,
			nil,
			&status,
			http.StatusOK,
		); err != nil {
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

// This additional phase is actual scheduled campaign dispatch and deliberate
// no-resend reconciliation. The original uncertain evidence must stay intact.
func exerciseLifecycleUnknownCampaign(
	t *testing.T,
	control *lifecycleControl,
	owner *lifecycleClient,
	eventID uuid.UUID,
	campaignPath string,
	campaign communications.CampaignDetail,
) {
	t.Helper()
	ctx := t.Context()
	base := "/api/v1/events/" + eventID.String() + "/communications"
	for _, action := range []string{"preview", "approve"} {
		var next communications.CampaignDetail
		if err := owner.call(
			ctx,
			http.MethodPost,
			campaignPath,
			campaign.Revision,
			communications.CampaignCommand{
				Action:  action,
				Version: campaign.Campaign.Campaign.Version,
				Reason:  "Explicit synthetic campaign audience and schedule review",
			},
			&next,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		campaign = next
	}
	if campaign.Preview == nil || len(campaign.Preview.Recipients) != 1 ||
		campaign.Preview.Recipients[0].PersonID != owner.person ||
		campaign.Campaign.Campaign.Approval == nil ||
		campaign.Campaign.DueAt == nil {
		t.Fatal("campaign did not bind the exact current organiser audience")
	}
	definition := automation.Definition{
		ID:                     uuid.New(),
		CampaignID:             campaign.Campaign.Campaign.Definition.ID,
		CampaignVersion:        campaign.Campaign.Campaign.Version,
		Trigger:                "scheduled",
		MaximumLatenessSeconds: 60,
	}
	var changed struct {
		Page automation.Page `json:"page"`
	}
	if err := owner.call(
		ctx,
		http.MethodPost,
		base+"/automation",
		campaign.Revision,
		map[string]any{
			"action":     "save",
			"definition": definition,
			"version":    0,
			"reason":     "Explicit real queued synthetic schedule",
		},
		&changed,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	if len(changed.Page.Bindings) != 1 || changed.Page.Bindings[0].Definition.ID != definition.ID {
		t.Fatal("native scheduled binding was not retained")
	}
	if err := owner.call(
		ctx,
		http.MethodPost,
		base+"/automation",
		changed.Page.Revision,
		map[string]any{
			"action":     "approve",
			"binding_id": definition.ID,
			"version":    changed.Page.Bindings[0].Version,
			"reason":     "Approve exact native synthetic dispatch",
		},
		&changed,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	if err := control.call(
		ctx,
		http.MethodPost,
		"/mail/fault",
		map[string]string{"recipient": "owner@example.test", "mode": "ambiguous"},
		nil,
	); err != nil {
		t.Fatal(err)
	}
	if err := control.advance(ctx, *campaign.Campaign.DueAt); err != nil {
		t.Fatal(err)
	}
	var history deliveryhistory.Page
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if err := owner.call(
			ctx,
			http.MethodGet,
			base+"/deliveries?channel=smtp&campaign_id="+definition.CampaignID.String(),
			-1,
			nil,
			&history,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		campaignItems := make([]deliveryhistory.Item, 0, 1)
		selfTests := 0
		for _, item := range history.Items {
			switch item.Purpose {
			case "campaign":
				campaignItems = append(campaignItems, item)
			case "self_test":
				selfTests++
			default:
				t.Fatal("unexpected native campaign delivery history purpose")
			}
		}
		if selfTests != 1 {
			t.Fatal("native history lost the independent original self-test")
		}
		history.Items = campaignItems

		if len(history.Items) == 1 && history.Items[0].Purpose == "campaign" &&
			history.Items[0].State == "ambiguous" &&
			history.Items[0].OperationState == "ambiguous" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(history.Items) != 1 || history.Items[0].State != "ambiguous" ||
		history.Items[0].RecipientID == nil ||
		*history.Items[0].RecipientID != owner.person {
		t.Fatal("actual native scheduled campaign did not retain its uncertain recipient effect")
	}
	path := base + "/deliveries/" + history.Items[0].ID.String()
	var original deliveryhistory.Detail
	if err := owner.call(ctx, http.MethodGet, path, -1, nil, &original, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if len(original.Attempts) != 1 || original.Attempts[0].State != "ambiguous" {
		t.Fatal("native unknown effect did not preserve its first attempt")
	}
	command := deliveryhistory.Command{
		RequestID: uuid.New(),
		Action:    "resolve_no_resend",
		Reason:    "Synthetic relay consumed DATA but response was lost; deliberately do not duplicate",
	}
	var resolved deliveryhistory.Detail
	// Repeat the exact original command/revision to model a lost HTTP response.
	for range 2 {
		if err := owner.call(
			ctx,
			http.MethodPost,
			path,
			original.Item.Revision,
			command,
			&resolved,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
	}
	if resolved.Item.ID != original.Item.ID || !resolved.Item.Cancelled ||
		resolved.Item.State != "ambiguous" ||
		len(resolved.Attempts) != 1 ||
		resolved.Attempts[0].ID != original.Attempts[0].ID ||
		len(resolved.Reviews) != 1 ||
		resolved.Reviews[0].ID != command.RequestID {
		t.Fatal(
			"unknown-command retry changed original effect identity/evidence or duplicated the decision",
		)
	}
	collision := command
	collision.Reason = "Changed payload cannot reuse the lost-response identity"
	if err := owner.call(
		ctx,
		http.MethodPost,
		path,
		original.Item.Revision,
		collision,
		nil,
		http.StatusConflict,
	); err != nil {
		t.Fatal(err)
	}
	// Past the native initial retry/snooze interval; no manual queue/handler call.
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-time.After(32 * time.Second):
	}
	var mail struct {
		Items []lifecycleMail `json:"items"`
	}
	if err := control.call(ctx, http.MethodGet, "/mail/messages", nil, &mail); err != nil {
		t.Fatal(err)
	}
	if len(mail.Items) != 2 || mail.Items[1].Accepted || len(mail.Items[1].Digest) != 64 {
		t.Fatal("uncertain original was resent or lost its relay evidence")
	}
	var final deliveryhistory.Detail
	if err := owner.call(ctx, http.MethodGet, path, -1, nil, &final, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if final.Item.ID != original.Item.ID || len(final.Attempts) != 1 ||
		final.Attempts[0].ID != original.Attempts[0].ID ||
		len(final.Reviews) != 1 {
		t.Fatal("native reconciliation replayed the uncertain effect")
	}
	receipt, err := json.MarshalIndent(
		map[string]any{
			"checkpoint":          "queued-smtp-unknown-no-resend-only",
			"complete_lifecycle":  false,
			"event_id":            eventID,
			"campaign_id":         definition.CampaignID,
			"binding_id":          definition.ID,
			"delivery_id":         final.Item.ID,
			"attempt_id":          final.Attempts[0].ID,
			"decision_id":         command.RequestID,
			"state":               final.Item.State,
			"operation_state":     final.Item.OperationState,
			"cancelled":           final.Item.Cancelled,
			"attempt_count":       len(final.Attempts),
			"decision_count":      len(final.Reviews),
			"relay_count":         len(mail.Items),
			"unknown_mail_sha256": mail.Items[1].Digest,
			"assertions": []string{
				"native-exact-audience-approval",
				"native-schedule-binding-approval",
				"real-queued-dispatch-and-provider-effect",
				"lost-data-response-ambiguous",
				"explicit-no-resend",
				"same-original-command-idempotent",
				"changed-payload-conflict",
				"original-provider-evidence-retained",
				"no-blind-resend-after-natural-retry-interval",
			},
		},
		"",
		"  ",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join("/artifacts", "smtp-unknown.json"),
		receipt,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
}
