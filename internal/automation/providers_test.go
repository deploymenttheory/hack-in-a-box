package automation

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/inbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
	"github.com/deploymenttheory/hack-in-a-box/internal/smtpintegration"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type recordingRelay struct{ calls atomic.Int32 }

func (*recordingRelay) Configured() bool { return true }
func (*recordingRelay) Revision() string { return "synthetic-recording-relay" }

func (*recordingRelay) MessageID(
	id uuid.UUID,
) string {
	return "<" + id.String() + "@example.test>"
}

func (r *recordingRelay) Send(context.Context, maildelivery.Message) maildelivery.Receipt {
	r.calls.Add(1)
	return maildelivery.Receipt{State: maildelivery.Accepted, Code: "relay_accepted"}
}

func installProviders(t *testing.T, f *fixture) (*smtpintegration.Service, *recordingRelay) {
	t.Helper()
	relay := &recordingRelay{}
	smtp := &smtpintegration.Service{
		Pool:        f.pool,
		Transport:   relay,
		Origin:      "https://hack.example.test",
		Campaigns:   f.s.Composer,
		ScopePolicy: f.s,
		Now:         func() time.Time { return f.now },
	}
	local := &inbox.Service{Pool: f.pool, Communications: f.s.Composer, ScopePolicy: f.s}
	f.s.Composer.Channels = map[string]communications.ChannelProvider{"smtp": smtp, "in_app": local}
	outbox, err := jobs.New(
		f.pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{
			HandlerKind:                   f.s,
			"communication_campaign_smtp": smtpintegration.CampaignHandler{Service: smtp},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.s.Jobs = outbox
	f.s.Composer.Jobs = outbox
	f.s.ReviewGuards = map[string]ReviewGuard{
		"smtp": smtp.CheckOccurrenceReviewInTransaction,
		"in_app": func(context.Context, pgx.Tx, uuid.UUID, uuid.UUID, int64, communications.DeliveryScope) error {
			return nil
		},
	}
	f.s.Providers = map[string]ProviderQueue{
		"smtp": func(ctx context.Context, tx pgx.Tx, eventID, campaignID uuid.UUID, version int64, scope communications.DeliveryScope) (ChannelReport, error) {
			report, queueErr := smtp.QueueApprovedOccurrenceReportInTransaction(
				ctx,
				tx,
				eventID,
				campaignID,
				version,
				scope,
			)
			return ChannelReport{
				Queued:      report.Queued,
				Unavailable: report.Unavailable,
				Preexisting: report.Preexisting,
			}, queueErr
		},
		"in_app": func(ctx context.Context, tx pgx.Tx, eventID, campaignID uuid.UUID, version int64, scope communications.DeliveryScope) (ChannelReport, error) {
			count, queueErr := local.QueueApprovedOccurrenceInTransaction(
				ctx,
				tx,
				eventID,
				campaignID,
				version,
				scope,
			)
			return ChannelReport{LocalPersisted: count}, queueErr
		},
	}
	return smtp, relay
}

func TestProductionSMTPAllUnavailableEvidenceSurvivesUntilExplicitReview(t *testing.T) {
	f := newFixture(t)
	_, relay := installProviders(t, f)
	f.campaign("reminder", "Safe reminder", true)
	f.bind()
	id := f.maintain().Occurrences[0].ID
	f.now = f.now.Add(time.Minute)
	f.execute(id)
	page, err := f.s.Read(t.Context(), f.owner.ID, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if page.Occurrences[0].State != "blocked" ||
		page.Occurrences[0].ChannelReports["smtp"].Unavailable != 1 {
		t.Fatal("all-unverified dispatch invented delivery or lost blocked evidence")
	}
	var omissions, intents int
	if err = f.pool.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM smtp_campaign_omissions),(SELECT count(*) FROM smtp_campaign_messages)").
		Scan(&omissions, &intents); err != nil {
		t.Fatal(err)
	}
	if omissions != 1 || intents != 0 || relay.calls.Load() != 0 {
		t.Fatal("unverified recipient omission rolled back or produced a send intent")
	}
	if err = dbgen.New(f.pool).
		ConfirmPersonEmail(t.Context(), dbgen.ConfirmPersonEmailParams{PersonID: f.person.ID, Mailbox: "competitor@example.test"}); err != nil {
		t.Fatal(err)
	}
	f.execute(id)
	if err = f.pool.QueryRow(t.Context(), "SELECT count(*) FROM smtp_campaign_messages").
		Scan(&intents); err != nil {
		t.Fatal(err)
	}
	if intents != 0 {
		t.Fatal("later mailbox proof silently replayed an omitted occurrence")
	}
	f.change(
		Command{
			Action:       "review_occurrence",
			OccurrenceID: id,
			Reason:       "Explicitly review newly proven current address",
		},
	)
	f.execute(id)
	if err = f.pool.QueryRow(t.Context(), "SELECT count(*) FROM smtp_campaign_messages").
		Scan(&intents); err != nil {
		t.Fatal(err)
	}
	if intents != 1 || relay.calls.Load() != 0 {
		t.Fatal("explicit review failed to create one first intent without sending")
	}
}

func TestProductionPartialSMTPAndInboxCommitTogetherWithoutInventingReceipts(t *testing.T) {
	f := newFixture(t)
	_, relay := installProviders(t, f)
	other := testdb.Person(t, f.pool)
	f.exec("INSERT INTO competition_claims VALUES($1,$2,'confirmed')", f.id, other.ID)
	if err := dbgen.New(f.pool).
		ConfirmPersonEmail(t.Context(), dbgen.ConfirmPersonEmailParams{PersonID: f.person.ID, Mailbox: "competitor@example.test"}); err != nil {
		t.Fatal(err)
	}
	f.campaign("reminder", "Safe shared reminder", true, "smtp", "in_app")
	f.bind()
	id := f.maintain().Occurrences[0].ID
	f.now = f.now.Add(time.Minute)
	f.execute(id)
	page, err := f.s.Read(t.Context(), f.owner.ID, f.id)
	if err != nil {
		t.Fatal(err)
	}
	report := page.Occurrences[0]
	if report.State != "partially_queued" || report.ChannelReports["smtp"].Queued != 1 ||
		report.ChannelReports["smtp"].Unavailable != 1 ||
		report.ChannelReports["in_app"].LocalPersisted != 2 {
		t.Fatalf("untruthful mixed-channel evidence: %+v", report)
	}
	var local, intents, omissions, receipts int
	if err = f.pool.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM inbox_notifications),(SELECT count(*) FROM smtp_campaign_messages),(SELECT count(*) FROM smtp_campaign_omissions),(SELECT count(*) FROM smtp_submission_receipts)").
		Scan(&local, &intents, &omissions, &receipts); err != nil {
		t.Fatal(err)
	}
	if local != 2 || intents != 1 || omissions != 1 || receipts != 0 || relay.calls.Load() != 0 {
		t.Fatal("mixed outer transaction lost evidence or implied a network receipt")
	}
	var op uuid.UUID
	if err = f.pool.QueryRow(t.Context(), "SELECT operation_id FROM smtp_campaign_messages").
		Scan(&op); err != nil {
		t.Fatal(err)
	}
	if err = f.s.Jobs.Execute(t.Context(), op); err != nil {
		t.Fatal(err)
	}
	if relay.calls.Load() != 1 {
		t.Fatal("partially queued scope stranded its valid SMTP intent")
	}
	var state string
	if err = f.pool.QueryRow(t.Context(), "SELECT state FROM smtp_campaign_messages").
		Scan(&state); err != nil ||
		state != "accepted" {
		t.Fatal("relay acceptance was not retained distinctly", err)
	}
	if err = dbgen.New(f.pool).
		ConfirmPersonEmail(t.Context(), dbgen.ConfirmPersonEmailParams{PersonID: other.ID, Mailbox: "newly-proven@example.test"}); err != nil {
		t.Fatal(err)
	}
	f.change(
		Command{
			Action:       "review_occurrence",
			OccurrenceID: id,
			Reason:       "Review settled effects and newly verified address",
		},
	)
	if err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		proof, resolveErr := f.s.ResolveInTransaction(
			t.Context(),
			tx,
			f.id,
			f.campaignID,
			f.campaignVersion,
			communications.DeliveryScope{OccurrenceID: id},
		)
		if resolveErr != nil {
			return resolveErr
		}
		if proof.Review == nil || proof.Review.OccurrenceID != id || proof.Review.Revision < 1 ||
			proof.Review.Binding != proof.Binding {
			t.Fatal("canonical persisted review proof missing")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	f.execute(id)
	reviewed, err := f.s.Read(t.Context(), f.owner.ID, f.id)
	if err != nil {
		t.Fatal(err)
	}
	currentReport := reviewed.Occurrences[0].ChannelReports["smtp"]
	if currentReport.Queued != 1 || currentReport.Preexisting["accepted"] != 1 ||
		currentReport.Unavailable != 0 ||
		relay.calls.Load() != 1 {
		t.Fatalf("review replayed accepted effect or lost original status: %+v", currentReport)
	}
	if err = f.pool.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM inbox_notifications),(SELECT count(*) FROM smtp_campaign_messages)").
		Scan(&local, &intents); err != nil {
		t.Fatal(err)
	}
	if local != 2 || intents != 2 {
		t.Fatal("review duplicated local inbox or an accepted SMTP identity")
	}
}

func TestProductionExpectedPublicationUsesTypedProofWithoutGenericApproval(t *testing.T) {
	f := newFixture(t)
	_, relay := installProviders(t, f)
	if err := dbgen.New(f.pool).
		ConfirmPersonEmail(t.Context(), dbgen.ConfirmPersonEmailParams{PersonID: f.person.ID, Mailbox: "competitor@example.test"}); err != nil {
		t.Fatal(err)
	}
	sourceService, source := f.approvedBrief()
	f.campaign("brief_release", "Read {{brief.title}}, {{person.name}}", false, "smtp", "in_app")
	binding := f.publicationBinding(source)
	f.now = f.now.Add(time.Minute)
	if len(f.maintain().Occurrences) != 0 || relay.calls.Load() != 0 {
		t.Fatal("planned time generated an unpublished notification")
	}
	if err := sourceService.Dispatch(t.Context(), f.id, source.ID, source.Revision); err != nil {
		t.Fatal(err)
	}
	occurrence := f.maintain().Occurrences[0]
	f.execute(occurrence.ID)
	var smtpKind, inboxKind, planID string
	var smtpVersion, inboxVersion int64
	if err := f.pool.QueryRow(t.Context(), "SELECT scope_approval->>'kind',scope_approval->>'id',(scope_approval->>'source_version')::bigint FROM smtp_campaign_messages").
		Scan(&smtpKind, &planID, &smtpVersion); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(t.Context(), "SELECT scope_approval->>'kind',(scope_approval->>'source_version')::bigint FROM inbox_notifications").
		Scan(&inboxKind, &inboxVersion); err != nil {
		t.Fatal(err)
	}
	if smtpKind != "expected_publication" || inboxKind != smtpKind ||
		binding.PublicationApprovalID == nil ||
		planID != binding.PublicationApprovalID.String() ||
		smtpVersion != source.Version ||
		inboxVersion != source.Version {
		t.Fatal("provider immutable evidence lost typed publication identity")
	}
	detail, err := f.s.Composer.Campaign(t.Context(), f.owner.ID, f.id, f.campaignID)
	if err != nil || detail.Campaign.Status != "draft" || detail.Campaign.Campaign.Approval != nil {
		t.Fatal("scoped dispatch forged generic approval", err)
	}
	var op uuid.UUID
	if err = f.pool.QueryRow(t.Context(), "SELECT operation_id FROM smtp_campaign_messages").
		Scan(&op); err != nil {
		t.Fatal(err)
	}
	if err = f.s.Jobs.Execute(t.Context(), op); err != nil {
		t.Fatal(err)
	}
	if relay.calls.Load() != 1 {
		t.Fatal("canonical typed publication did not survive current worker recheck")
	}
}

func TestProductionRetainedSMTPUsesNarrowReadOnlyWorkerProof(t *testing.T) {
	f := newFixture(t)
	_, relay := installProviders(t, f)
	if err := dbgen.New(f.pool).
		ConfirmPersonEmail(t.Context(), dbgen.ConfirmPersonEmailParams{PersonID: f.person.ID, Mailbox: "competitor@example.test"}); err != nil {
		t.Fatal(err)
	}
	f.retainedBinding()
	f.now = f.now.Add(70 * time.Second)
	id := f.maintain().Occurrences[0].ID
	if _, err := (event.Service{Pool: f.pool}).Transition(
		t.Context(),
		f.owner.ID,
		f.id,
		f.revision(),
		"archived",
		"Archive ordinary work",
		false,
	); err != nil {
		t.Fatal(err)
	}
	f.execute(id)
	var op, marker uuid.UUID
	if err := f.pool.QueryRow(t.Context(), "SELECT m.operation_id,o.retained_occurrence_id FROM smtp_campaign_messages m JOIN operations o ON o.id=m.operation_id").
		Scan(&op, &marker); err != nil {
		t.Fatal(err)
	}
	if marker != id {
		t.Fatal("retained adapter intent omitted durable occurrence capability")
	}
	if err := f.s.Jobs.Execute(t.Context(), op); err != nil {
		t.Fatal(err)
	}
	if relay.calls.Load() != 1 {
		t.Fatal("narrow retained proof did not authorise current owner follow-up")
	}
}
