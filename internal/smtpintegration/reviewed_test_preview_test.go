package smtpintegration

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
)

func (f *fixture) queueReviewed() jobs.Delivery {
	f.t.Helper()
	preview, err := f.comm.PreviewSelfTest(
		f.t.Context(),
		f.owner.ID,
		f.event,
		f.campaign.Definition.ID,
		"smtp",
	)
	if err != nil {
		f.t.Fatal(err)
	}
	result, err := f.comm.ChangeCampaign(
		f.t.Context(),
		f.owner.ID,
		f.event,
		f.campaign.Definition.ID,
		f.revision(),
		communications.CampaignCommand{
			Action:                  "test_self",
			Channel:                 "smtp",
			Version:                 f.campaign.Version,
			Reason:                  "Synthetic exact TEST review",
			ReviewedTestFingerprint: preview.ReviewFingerprint,
		},
	)
	if err != nil || result.TestRequestID == nil {
		f.t.Fatalf("queue reviewed TEST: %v", err)
	}
	operation, err := dbgen.New(f.pool).
		SMTPTestOperation(f.t.Context(), dbgen.SMTPTestOperationParams{EventID: f.event, TargetID: *result.TestRequestID})
	if err != nil {
		f.t.Fatal(err)
	}
	return jobs.Delivery{
		ID:       operation.ID,
		EventID:  operation.EventID,
		TargetID: operation.TargetID,
		Key:      operation.ID.String(),
	}
}

func TestSMTPReviewedTESTChangesBeforeFirstPreparationNeverContactProvider(t *testing.T) {
	for _, change := range []string{"verified_mailbox_replacement", "template", "provider", "organiser_role", "inactive_account", "event_source", "calendar_dates", "workspace_origin", "branding"} {
		t.Run(change, func(t *testing.T) {
			f := setup(t)
			delivery := f.queueReviewed()
			switch change {
			case "verified_mailbox_replacement":
				f.exec(
					"UPDATE person_email_contacts SET mailbox='replacement@synthetic.invalid' WHERE person_id=$1",
					f.owner.ID,
				)
			case "template":
				page, err := f.comm.Read(t.Context(), f.owner.ID, f.event)
				if err != nil {
					t.Fatal(err)
				}
				template := page.Templates[0]
				template.Subject = "Changed reviewed template {{person.name}}"
				if _, err = f.comm.Change(
					t.Context(),
					f.owner.ID,
					f.event,
					f.revision(),
					communications.Command{
						Action:   "save_template",
						Template: &template,
						Reason:   "Synthetic source version change",
					},
				); err != nil {
					t.Fatal(err)
				}
			case "provider":
				f.transport.revision = "synthetic-relay-v2"
			case "organiser_role":
				f.exec(
					"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='organiser'",
					f.event,
					f.owner.ID,
				)
			case "inactive_account":
				f.exec("UPDATE people SET active=false WHERE id=$1", f.owner.ID)
			case "event_source":
				f.exec("UPDATE events SET name='Changed event source' WHERE id=$1", f.event)
			case "calendar_dates":
				value, _ := json.Marshal(f.now.AddDate(0, 1, 0).Add(time.Hour))
				f.exec(
					"UPDATE events SET configuration=jsonb_set(configuration,'{live_start}', $2::jsonb) WHERE id=$1",
					f.event,
					value,
				)
			case "workspace_origin":
				f.service.Origin = "https://changed-workspace.synthetic.invalid"
			case "branding":
				f.exec(
					"INSERT INTO event_definitions(event_id,definitions) VALUES($1,$2)",
					f.event,
					[]byte(
						`{"branding":{"primary":"#123456","background":"#ffffff","foreground":"#111111"}}`,
					),
				)
			}
			if outcome := f.service.Deliver(t.Context(), delivery); outcome != jobs.Permanent {
				t.Fatalf("changed review outcome: %s", outcome)
			}
			if f.transport.count() != 0 {
				t.Fatal("changed review contacted the deterministic provider")
			}
			var attempts int
			if err := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM smtp_submission_attempts WHERE operation_id=$1", delivery.ID).
				Scan(&attempts); err != nil {
				t.Fatal(err)
			}
			if attempts != 0 {
				t.Fatal("changed review prepared an effect")
			}
		})
	}
}

func TestSMTPReviewedTESTAmbiguousSubmissionNeverReplays(t *testing.T) {
	f := setup(t)
	f.transport.state = maildelivery.Ambiguous
	delivery := f.queueReviewed()
	if got := f.service.Deliver(t.Context(), delivery); got != jobs.Ambiguous {
		t.Fatalf("first submission: %s", got)
	}
	if got := f.service.Deliver(t.Context(), delivery); got != jobs.Ambiguous {
		t.Fatalf("repeated submission: %s", got)
	}
	if got := f.service.Reconcile(t.Context(), delivery); got != jobs.Ambiguous {
		t.Fatalf("reconciliation: %s", got)
	}
	if f.transport.count() != 1 {
		t.Fatal("uncertain reviewed TEST was sent again")
	}
	if f.transport.messages[0].Recipient != "organiser@example.test" {
		t.Fatal("reviewed TEST targeted another recipient")
	}
}

func TestSMTPReviewedTESTPreviewIncludesExactCalendarBodyWithoutRetainingSequence(t *testing.T) {
	f := setup(t)
	var before, after int
	if err := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM email_calendar_sequences WHERE event_id=$1", f.event).
		Scan(&before); err != nil {
		t.Fatal(err)
	}
	revision := f.revision()
	preview, err := f.comm.PreviewSelfTest(
		t.Context(),
		f.owner.ID,
		f.event,
		f.campaign.Definition.ID,
		"smtp",
	)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.comm.PreviewSelfTest(
		t.Context(),
		f.owner.ID,
		f.event,
		f.campaign.Definition.ID,
		"smtp",
	)
	if err != nil {
		t.Fatal(err)
	}
	if preview.ReviewFingerprint != again.ReviewFingerprint || preview.Calendar == nil ||
		preview.Calendar.WorkspaceURL != f.service.Origin+"/events/"+f.event.String() {
		t.Fatal("calendar review is not deterministic or omits workspace facts")
	}
	if err := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM email_calendar_sequences WHERE event_id=$1", f.event).
		Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after || revision != f.revision() || f.transport.count() != 0 {
		t.Fatal("preview retained calendar sequence, changed revision or contacted provider")
	}
	delivery := f.queueReviewed()
	if got := f.service.Deliver(t.Context(), delivery); got != jobs.Delivered {
		t.Fatalf("reviewed submission: %s", got)
	}
	if f.transport.count() != 1 || f.transport.messages[0].PlainText != preview.DeliveryPlainText ||
		f.transport.messages[0].Subject != preview.Subject {
		t.Fatal("prepared plain-text envelope differs from reviewed exact content")
	}
	calendar := f.transport.messages[0].Calendar
	if calendar == nil || calendar.Summary != preview.Calendar.Summary ||
		calendar.WorkspaceURL != preview.Calendar.WorkspaceURL ||
		calendar.Timezone != preview.Calendar.Timezone ||
		!calendar.Start.Equal(preview.Calendar.Start) ||
		!calendar.End.Equal(preview.Calendar.End) {
		t.Fatal("prepared calendar facts differ from review")
	}
}
