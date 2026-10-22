package smtpintegration

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func (f *fixture) approveProduction() {
	f.t.Helper()
	for _, action := range []string{"preview", "approve"} {
		result, err := f.comm.ChangeCampaign(
			f.t.Context(),
			f.owner.ID,
			f.event,
			f.campaign.Definition.ID,
			f.revision(),
			communications.CampaignCommand{
				Action:  action,
				Version: f.campaign.Version,
				Reason:  "Synthetic reviewed production campaign",
			},
		)
		if err != nil {
			f.t.Fatal(err)
		}
		f.campaign = result.Campaign.Campaign
	}
	f.now = f.campaign.Approval.DueAt
}

func (f *fixture) queueProduction() jobs.Delivery {
	f.t.Helper()
	err := database.Within(f.t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		count, err := f.service.QueueApprovedCampaignInTransaction(
			f.t.Context(),
			tx,
			f.event,
			f.campaign.Definition.ID,
			f.campaign.Version,
		)
		if err == nil && count != 1 {
			f.t.Fatalf("production intents=%d want1", count)
		}
		return err
	})
	if err != nil {
		f.t.Fatal(err)
	}
	var d jobs.Delivery
	if err = f.pool.QueryRow(f.t.Context(), "SELECT operation_id,event_id,intent_id FROM smtp_campaign_messages WHERE event_id=$1", f.event).
		Scan(&d.ID, &d.EventID, &d.TargetID); err != nil {
		f.t.Fatal(err)
	}
	return d
}

func TestApprovedSMTPCampaignQueueIsTransactionalDeduplicatedAndPrivate(t *testing.T) {
	f := setup(t)
	f.exec("UPDATE people SET display_name='Synthetic <&> Person' WHERE id=$1", f.owner.ID)
	f.approveProduction()
	rollback := errors.New("synthetic outer transaction rollback")
	err := database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		if _, err := f.service.QueueApprovedCampaignInTransaction(
			t.Context(),
			tx,
			f.event,
			f.campaign.Definition.ID,
			f.campaign.Version,
		); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	var count int
	if err = f.pool.QueryRow(t.Context(), "SELECT count(*) FROM smtp_campaign_messages").
		Scan(&count); err != nil || count != 0 ||
		f.transport.count() != 0 {
		t.Fatal("rolled-back queue retained intent or network effect")
	}
	d := f.queueProduction()
	again := f.queueProduction()
	if d.ID != again.ID || f.transport.count() != 0 {
		t.Fatal("queue changed identity or performed external effect")
	}
	handler := CampaignHandler{Service: f.service}
	if handler.Deliver(t.Context(), d) != jobs.Delivered {
		t.Fatal("approved production intent was not accepted")
	}
	message := f.transport.messages[0]
	if !strings.Contains(message.PlainText, "Synthetic <&> Person") ||
		strings.Contains(message.PlainText, "&lt;") {
		t.Fatal("SMTP literal text double-escaped the approved personal rendering")
	}
	if f.transport.count() != 1 || message.Calendar == nil || message.Calendar.Test ||
		message.Recipient != "organiser@example.test" {
		t.Fatal("production intent lost personal destination/live calendar")
	}
	if handler.Deliver(t.Context(), d) != jobs.Delivered || f.transport.count() != 1 {
		t.Fatal("accepted production intent replayed")
	}
	var genericRecords string
	if err = f.pool.QueryRow(t.Context(), "SELECT coalesce((SELECT string_agg(row_to_json(o)::text,' ') FROM operations o),'') || coalesce((SELECT string_agg(row_to_json(a)::text,' ') FROM audit_events a),'')").
		Scan(&genericRecords); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(genericRecords, message.Recipient) ||
		strings.Contains(genericRecords, message.PlainText) {
		t.Fatal("production personal payload leaked into generic records")
	}
	var snapshot campaignSnapshot
	row, err := dbgen.New(f.pool).SMTPCampaignMessage(t.Context(), d.ID)
	if err != nil || json.Unmarshal(row.Snapshot, &snapshot) != nil || row.State != "accepted" {
		t.Fatal("private immutable production snapshot missing")
	}
	if _, err = f.pool.Exec(
		t.Context(),
		"UPDATE smtp_campaign_messages SET mailbox='other@example.test' WHERE operation_id=$1",
		d.ID,
	); err == nil {
		t.Fatal("production destination was mutable")
	}
	err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		_, err := f.service.QueueApprovedCampaignInTransaction(
			t.Context(),
			tx,
			f.event,
			f.campaign.Definition.ID,
			f.campaign.Version+1,
		)
		return err
	})
	if !errors.Is(err, access.ErrConflict) {
		t.Fatal("stale/wrong selected version queued")
	}
}

func TestApprovedSMTPCampaignWorkerRechecksAuthorityDestinationAndSource(t *testing.T) {
	for _, change := range []string{"role", "mailbox", "config", "source", "cancel", "cross_event"} {
		t.Run(change, func(t *testing.T) {
			f := setup(t)
			f.approveProduction()
			d := f.queueProduction()
			switch change {
			case "role":
				f.exec(
					"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2",
					f.event,
					f.owner.ID,
				)
			case "mailbox":
				f.exec(
					"UPDATE person_email_contacts SET mailbox='changed@example.test' WHERE person_id=$1",
					f.owner.ID,
				)
			case "config":
				f.transport.revision = "changed-relay"
			case "source":
				f.exec("UPDATE events SET name='Changed source' WHERE id=$1", f.event)
			case "cancel":
				f.exec("UPDATE operations SET cancelled=true WHERE id=$1", d.ID)
			case "cross_event":
				d.EventID = uuid.New()
			}
			if outcome := (CampaignHandler{Service: f.service}).Deliver(
				t.Context(),
				d,
			); outcome != jobs.Permanent ||
				f.transport.count() != 0 {
				t.Fatalf("changed authority/source produced %s or an effect", outcome)
			}
		})
	}
}

func TestApprovedSMTPCampaignAmbiguousNeverResendsAndDefiniteRejectionRetainsSnapshot(
	t *testing.T,
) {
	f := setup(t)
	f.approveProduction()
	d := f.queueProduction()
	h := CampaignHandler{Service: f.service}
	f.transport.state = maildelivery.Retryable
	if h.Deliver(t.Context(), d) != jobs.Retryable ||
		h.Reconcile(t.Context(), d) != jobs.Retryable {
		t.Fatal("definite temporary rejection lost safe retry")
	}
	first := f.transport.messages[0]
	f.transport.state = maildelivery.Ambiguous
	if h.Deliver(t.Context(), d) != jobs.Ambiguous {
		t.Fatal("uncertain production acceptance mislabelled")
	}
	second := f.transport.messages[1]
	if first.ID != second.ID || !first.SentAt.Equal(second.SentAt) ||
		first.Calendar.Sequence != second.Calendar.Sequence {
		t.Fatal("production retry changed immutable message/calendar")
	}
	if h.Reconcile(t.Context(), d) != jobs.Ambiguous ||
		h.Deliver(t.Context(), d) != jobs.Ambiguous ||
		f.transport.count() != 2 {
		t.Fatal("ambiguous production attempt replayed")
	}
}

type syntheticScopePolicy struct {
	audience communications.ScopeAudience
	denied   bool
}

func (p *syntheticScopePolicy) ResolveInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	version int64,
	scope communications.DeliveryScope,
) (communications.ScopeAudience, error) {
	if _, err := dbgen.New(tx).LockEvent(ctx, eventID); err != nil {
		return communications.ScopeAudience{}, err
	}
	if p.denied {
		return communications.ScopeAudience{}, access.ErrForbidden
	}
	return p.audience, nil
}

func TestApprovedSMTPOccurrencesRetainIndependentIdentityAndCurrentScope(t *testing.T) {
	f := setup(t)
	f.approveProduction()
	scope := communications.DeliveryScope{OccurrenceID: uuid.New()}
	queue := func(current communications.DeliveryScope) (int, error) {
		count := 0
		err := database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
			var err error
			count, err = f.service.QueueApprovedOccurrenceInTransaction(
				t.Context(),
				tx,
				f.event,
				f.campaign.Definition.ID,
				f.campaign.Version,
				current,
			)
			return err
		})
		return count, err
	}
	if _, err := queue(scope); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("nonzero occurrence without policy did not fail closed")
	}
	policy := &syntheticScopePolicy{
		audience: communications.ScopeAudience{
			Binding:      "synthetic-source-version1",
			RecipientIDs: []uuid.UUID{f.owner.ID},
		},
	}
	f.service.ScopePolicy = policy
	for _, current := range []communications.DeliveryScope{scope, scope, {OccurrenceID: uuid.New()}} {
		if count, err := queue(current); err != nil || count != 1 {
			t.Fatalf("approved occurrence intents=%d error=%v", count, err)
		}
	}
	var count int
	if err := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM smtp_campaign_messages").
		Scan(&count); err != nil ||
		count != 2 {
		t.Fatal("distinct occurrence suppressed or repeated occurrence duplicated")
	}
	policy.audience.RecipientIDs = []uuid.UUID{uuid.New()}
	if count, err := queue(
		communications.DeliveryScope{OccurrenceID: uuid.New()},
	); err != nil ||
		count != 0 {
		t.Fatal("scope widened the approved campaign selector")
	}
	policy.audience.RecipientIDs = []uuid.UUID{f.owner.ID}
	var d jobs.Delivery
	if err := f.pool.QueryRow(t.Context(), "SELECT operation_id,event_id,intent_id FROM smtp_campaign_messages WHERE occurrence_id=$1", scope.OccurrenceID).
		Scan(&d.ID, &d.EventID, &d.TargetID); err != nil {
		t.Fatal(err)
	}
	policy.denied = true
	if outcome := (CampaignHandler{Service: f.service}).Deliver(
		t.Context(),
		d,
	); outcome != jobs.Permanent ||
		f.transport.count() != 0 {
		t.Fatal("paused/cancelled occurrence caused an SMTP effect")
	}
	policy.denied = false
	policy.audience.Binding = "changed-source-version"
	if outcome := (CampaignHandler{Service: f.service}).Deliver(
		t.Context(),
		d,
	); outcome != jobs.Permanent ||
		f.transport.count() != 0 {
		t.Fatal("changed occurrence source binding caused an SMTP effect")
	}
}

func TestApprovedSMTPCampaignMessagesAreIndividualAndUnverifiedContactsAreExcluded(t *testing.T) {
	f := setup(t)
	other, unverified := testdb.Person(t, f.pool), testdb.Person(t, f.pool)
	for _, id := range []uuid.UUID{other.ID, unverified.ID} {
		f.exec(
			"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'organiser')",
			f.event,
			id,
		)
	}
	if err := dbgen.New(f.pool).
		ConfirmPersonEmail(t.Context(), dbgen.ConfirmPersonEmailParams{PersonID: other.ID, Mailbox: "second-organiser@example.test"}); err != nil {
		t.Fatal(err)
	}
	f.approveProduction()
	err := database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		count, err := f.service.QueueApprovedCampaignInTransaction(
			t.Context(),
			tx,
			f.event,
			f.campaign.Definition.ID,
			f.campaign.Version,
		)
		if err == nil && count != 2 {
			t.Fatalf("personal intents=%d want2", count)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := f.pool.Query(
		t.Context(),
		"SELECT operation_id,event_id,intent_id FROM smtp_campaign_messages WHERE event_id=$1 ORDER BY operation_id",
		f.event,
	)
	if err != nil {
		t.Fatal(err)
	}
	var deliveries []jobs.Delivery
	for rows.Next() {
		var d jobs.Delivery
		if err = rows.Scan(&d.ID, &d.EventID, &d.TargetID); err != nil {
			t.Fatal(err)
		}
		deliveries = append(deliveries, d)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	handler := CampaignHandler{Service: f.service}
	for _, d := range deliveries {
		if handler.Deliver(t.Context(), d) != jobs.Delivered {
			t.Fatal("individual approved message was not accepted")
		}
	}
	if f.transport.count() != 2 {
		t.Fatal("unverified address became a send or personal messages were bundled")
	}
	seen := map[string]bool{}
	for _, message := range f.transport.messages {
		seen[message.Recipient] = true
		if strings.Contains(message.PlainText, "organiser@example.test") ||
			strings.Contains(message.PlainText, "second-organiser@example.test") {
			t.Fatal("personal body exposed audience mailbox addresses")
		}
	}
	if !seen["organiser@example.test"] || !seen["second-organiser@example.test"] {
		t.Fatal("individual SMTP recipients were redirected")
	}
}

func TestSMTPCampaignOmissionIsImmutableAndLaterVerificationRequiresFreshReviewedIntent(
	t *testing.T,
) {
	f := setup(t)
	person := testdb.Person(t, f.pool)
	f.exec(
		"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'organiser')",
		f.event,
		person.ID,
	)
	f.approveProduction()
	queue := func(want int) {
		t.Helper()
		err := database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
			count, err := f.service.QueueApprovedCampaignInTransaction(
				t.Context(),
				tx,
				f.event,
				f.campaign.Definition.ID,
				f.campaign.Version,
			)
			if err == nil && count != want {
				t.Fatalf("eligible intents=%d want%d", count, want)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	queue(1)
	queue(1)
	var id uuid.UUID
	var count int
	if err := f.pool.QueryRow(t.Context(), "SELECT count(*),min(intent_id::text)::uuid FROM smtp_campaign_omissions WHERE event_id=$1 AND person_id=$2 AND reason='address_unverified'", f.event, person.ID).
		Scan(&count, &id); err != nil ||
		count != 1 {
		t.Fatal("unverified reviewed recipient omission not deduplicated")
	}
	for _, statement := range []string{"UPDATE smtp_campaign_omissions SET reason=reason WHERE intent_id=$1", "DELETE FROM smtp_campaign_omissions WHERE intent_id=$1"} {
		if _, err := f.pool.Exec(t.Context(), statement, id); err == nil {
			t.Fatal("reviewed recipient omission was mutable")
		}
	}
	if err := dbgen.New(f.pool).
		ConfirmPersonEmail(t.Context(), dbgen.ConfirmPersonEmailParams{PersonID: person.ID, Mailbox: "later-confirmed@example.test"}); err != nil {
		t.Fatal(err)
	}
	queue(1)
	if err := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM smtp_campaign_messages WHERE person_id=$1", person.ID).
		Scan(&count); err != nil ||
		count != 0 ||
		f.transport.count() != 0 {
		t.Fatal("later verification silently created retrospective delivery")
	}
	// Saving and explicitly reviewing a fresh version establishes a new intent.
	f.now = time.Now().UTC()
	definition := f.campaign.Definition
	definition.Name = "Explicitly reviewed follow-up"
	result, err := f.comm.ChangeCampaign(
		t.Context(),
		f.owner.ID,
		f.event,
		definition.ID,
		f.revision(),
		communications.CampaignCommand{
			Action:     "save",
			Version:    f.campaign.Version,
			Definition: &definition,
			Reason:     "Explicitly review now-confirmed mailbox audience",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.campaign = result.Campaign.Campaign
	f.approveProduction()
	queue(2)
	if err = f.pool.QueryRow(t.Context(), "SELECT count(*) FROM smtp_campaign_messages WHERE person_id=$1", person.ID).
		Scan(&count); err != nil ||
		count != 1 {
		t.Fatal("fresh reviewed eligible intent did not include newly verified recipient")
	}
}

func TestSMTPQueueReportCommitsAllUnavailableEvidenceWithoutInventingIntents(t *testing.T) {
	f := setup(t)
	f.exec("DELETE FROM person_email_contacts WHERE person_id=$1", f.owner.ID)
	f.approveProduction()
	for range 2 {
		err := database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
			report, err := f.service.QueueApprovedOccurrenceReportInTransaction(
				t.Context(),
				tx,
				f.event,
				f.campaign.Definition.ID,
				f.campaign.Version,
				communications.DeliveryScope{},
			)
			if err == nil && (report.Queued != 0 || report.Unavailable != 1) {
				t.Fatal(
					"all-unverified report implied queued/provider delivery or omitted unavailability",
				)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	var omitted, messages, attempts int
	if err := f.pool.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM smtp_campaign_omissions),(SELECT count(*) FROM smtp_campaign_messages),(SELECT count(*) FROM smtp_submission_attempts)").
		Scan(&omitted, &messages, &attempts); err != nil ||
		omitted != 1 ||
		messages != 0 ||
		attempts != 0 ||
		f.transport.count() != 0 {
		t.Fatal("all-unavailable queue lost evidence or invented external effects")
	}
}

func TestSMTPOmissionReviewQueuesOnlyPreviouslyOmittedPeopleAndRetainsPriorStatuses(t *testing.T) {
	for _, outcome := range []maildelivery.State{maildelivery.Accepted, maildelivery.Permanent, maildelivery.Ambiguous} {
		t.Run(string(outcome), func(t *testing.T) {
			f := setup(t)
			person := testdb.Person(t, f.pool)
			f.exec(
				"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'organiser')",
				f.event,
				person.ID,
			)
			f.approveProduction()
			scope := communications.DeliveryScope{OccurrenceID: uuid.New()}
			policy := &syntheticScopePolicy{
				audience: communications.ScopeAudience{
					Binding:      "explicit-review-1",
					RecipientIDs: []uuid.UUID{f.owner.ID, person.ID},
				},
			}
			f.service.ScopePolicy = policy
			queue := func() (QueueReport, error) {
				var report QueueReport
				err := database.Within(
					t.Context(),
					f.pool,
					func(tx pgx.Tx, _ *dbgen.Queries) error {
						var err error
						report, err = f.service.QueueApprovedOccurrenceReportInTransaction(
							t.Context(),
							tx,
							f.event,
							f.campaign.Definition.ID,
							f.campaign.Version,
							scope,
						)
						return err
					},
				)
				return report, err
			}
			report, err := queue()
			if err != nil || report.Queued != 1 || report.Unavailable != 1 ||
				len(report.Preexisting) != 0 {
				t.Fatal("initial scoped queue did not distinguish omission from new intent")
			}
			var d jobs.Delivery
			if err = f.pool.QueryRow(t.Context(), "SELECT operation_id,event_id,intent_id FROM smtp_campaign_messages WHERE person_id=$1", f.owner.ID).
				Scan(&d.ID, &d.EventID, &d.TargetID); err != nil {
				t.Fatal(err)
			}
			f.transport.state = outcome
			handler := CampaignHandler{Service: f.service}
			if handler.Deliver(
				t.Context(),
				d,
			) != jobs.Outcome(
				map[maildelivery.State]string{maildelivery.Accepted: "delivered", maildelivery.Permanent: "permanent", maildelivery.Ambiguous: "ambiguous"}[outcome],
			) {
				t.Fatal("synthetic provider outcome not retained")
			}
			if err = dbgen.New(f.pool).
				ConfirmPersonEmail(t.Context(), dbgen.ConfirmPersonEmailParams{PersonID: person.ID, Mailbox: "reviewed-later@example.test"}); err != nil {
				t.Fatal(err)
			}
			report, err = queue()
			if err != nil || report.Queued != 0 || report.Unavailable != 1 ||
				report.Preexisting[string(outcome)] != 1 {
				t.Fatal(
					"mailbox verification silently changed reviewed omission or fabricated a new send",
				)
			}
			checkReview := func() error {
				return database.Within(
					t.Context(),
					f.pool,
					func(tx pgx.Tx, _ *dbgen.Queries) error {
						return f.service.CheckOccurrenceReviewInTransaction(
							t.Context(),
							tx,
							f.event,
							f.campaign.Definition.ID,
							f.campaign.Version,
							scope,
						)
					},
				)
			}
			if err = checkReview(); !errors.Is(err, access.ErrConflict) {
				t.Fatal(
					"review preflight failed to retain original generation while worker effects pending",
				)
			}
			policy.audience.Binding = "explicit-review-2"
			if _, err = queue(); !errors.Is(err, access.ErrConflict) {
				t.Fatal(
					"review proceeded while original operation still had pending worker effects",
				)
			}
			operationState := map[maildelivery.State]string{maildelivery.Accepted: "delivered", maildelivery.Permanent: "permanent", maildelivery.Ambiguous: "ambiguous"}[outcome]
			f.exec("UPDATE operations SET state=$2 WHERE id=$1", d.ID, operationState)
			if err = checkReview(); err != nil {
				t.Fatal("settled original effects prevented explicit omission review")
			}

			report, err = queue()
			if err != nil || report.Queued != 1 || report.Unavailable != 0 ||
				report.Preexisting[string(outcome)] != 1 {
				t.Fatal(
					"explicit omission review did not isolate new intent from retained prior status",
				)
			}
			if f.transport.count() != 1 {
				t.Fatal("review resent a prior accepted/permanent/uncertain intent")
			}
			var existing int
			if err = f.pool.QueryRow(t.Context(), "SELECT count(*) FROM smtp_campaign_messages WHERE person_id=$1", f.owner.ID).
				Scan(&existing); err != nil ||
				existing != 1 {
				t.Fatal("review generation duplicated original personal intent")
			}
			var other jobs.Delivery
			if err = f.pool.QueryRow(t.Context(), "SELECT operation_id,event_id,intent_id FROM smtp_campaign_messages WHERE person_id=$1", person.ID).
				Scan(&other.ID, &other.EventID, &other.TargetID); err != nil {
				t.Fatal(err)
			}
			f.transport.state = maildelivery.Accepted
			if handler.Deliver(t.Context(), other) != jobs.Delivered || f.transport.count() != 2 {
				t.Fatal(
					"reviewed newly eligible person did not receive their first personal intent",
				)
			}
		})
	}
}

func TestSMTPCampaignChangedSourceRequiresDistinctOccurrence(t *testing.T) {
	f := setup(t)
	f.approveProduction()
	f.service.ScopePolicy = &syntheticScopePolicy{
		audience: communications.ScopeAudience{
			Binding:      "source-review",
			RecipientIDs: []uuid.UUID{f.owner.ID},
		},
	}
	scope := communications.DeliveryScope{OccurrenceID: uuid.New()}
	queue := func() error {
		return database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
			_, err := f.service.QueueApprovedOccurrenceReportInTransaction(
				t.Context(),
				tx,
				f.event,
				f.campaign.Definition.ID,
				f.campaign.Version,
				scope,
			)
			return err
		})
	}
	if err := queue(); err != nil {
		t.Fatal(err)
	}
	f.now = time.Now().UTC()
	definition := f.campaign.Definition
	definition.Name = "New reviewed source version"
	result, err := f.comm.ChangeCampaign(
		t.Context(),
		f.owner.ID,
		f.event,
		definition.ID,
		f.revision(),
		communications.CampaignCommand{
			Action:     "save",
			Version:    f.campaign.Version,
			Definition: &definition,
			Reason:     "Explicit new source review",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.campaign = result.Campaign.Campaign
	f.approveProduction()
	if err = queue(); !errors.Is(err, access.ErrConflict) {
		t.Fatal("changed source approval rebound the original immutable occurrence")
	}
	scope.OccurrenceID = uuid.New()
	if err = queue(); err != nil {
		t.Fatal("distinct reviewed source occurrence could not queue")
	}
	if f.transport.count() != 0 {
		t.Fatal("source review queue performed network effects")
	}
}
