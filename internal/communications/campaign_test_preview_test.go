package communications

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
)

func previewFixture(t *testing.T) (*fixture, Campaign) {
	t.Helper()
	f := newFixture(t)
	f.person(t) // The campaign audience excludes its requesting organiser.
	f.exec(t, "UPDATE people SET display_name='Preview organiser' WHERE id=$1", f.owner.ID)
	f.exec(
		t,
		"INSERT INTO person_email_contacts(person_id,mailbox,provenance) VALUES($1,'reviewed-organiser@synthetic.invalid','code_confirmed')",
		f.owner.ID,
	)
	f.s.Channels = map[string]ChannelProvider{
		"smtp":   syntheticChannel{},
		"in_app": syntheticChannel{},
	}
	var err error
	f.s.Jobs, err = jobs.New(
		f.pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{},
	)
	if err != nil {
		t.Fatal(err)
	}
	detail := newCampaign(t, f)
	return f, detail.Campaign.Campaign
}

func previewCounts(t *testing.T, f *fixture) []int64 {
	t.Helper()
	var revision, operations, requests, audit int64
	err := f.pool.QueryRow(t.Context(), `SELECT revision,(SELECT count(*) FROM operations WHERE event_id=$1),(SELECT count(*) FROM communication_test_requests WHERE event_id=$1),(SELECT count(*) FROM audit_events WHERE event_id=$1) FROM events WHERE id=$1`, f.eventID).
		Scan(&revision, &operations, &requests, &audit)
	if err != nil {
		t.Fatal(err)
	}
	return []int64{revision, operations, requests, audit}
}

func TestSelfTestPreviewExactCurrentOrganiserWithoutWrites(t *testing.T) {
	f, c := previewFixture(t)
	before := previewCounts(t, f)
	first, err := f.s.PreviewSelfTest(t.Context(), f.owner.ID, f.eventID, c.Definition.ID, "smtp")
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.s.PreviewSelfTest(t.Context(), f.owner.ID, f.eventID, c.Definition.ID, "smtp")
	if err != nil {
		t.Fatal(err)
	}
	if first != second || !validReviewFingerprint(first.ReviewFingerprint) {
		t.Fatal("same current facts must produce the same review")
	}
	if first.ActorID != f.owner.ID || first.Subject != "[TEST] Hello Preview organiser" ||
		!strings.HasPrefix(
			first.PlainText,
			"This is a test for the requesting organiser only.\n\n",
		) ||
		!strings.Contains(first.PlainText, "Preview organiser") {
		t.Fatalf("wrong fixed organiser rendering: %#v", first)
	}
	if !sameCounts(before, previewCounts(t, f)) {
		t.Fatal("preview changed revision, operation, request or audit rows")
	}
	encoded, _ := json.Marshal(first)
	if strings.Contains(string(encoded), "synthetic.invalid") ||
		strings.Contains(string(encoded), "mailbox") {
		t.Fatal("private mailbox entered the preview")
	}
	outsider := f.person(t)
	if _, err = f.s.PreviewSelfTest(
		t.Context(),
		outsider.ID,
		f.eventID,
		c.Definition.ID,
		"smtp",
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatalf("competitor preview: %v", err)
	}
	if _, err = f.s.PreviewSelfTest(
		t.Context(),
		f.owner.ID,
		f.eventID,
		c.Definition.ID,
		"teams",
	); !errors.Is(
		err,
		access.ErrInvalid,
	) {
		t.Fatalf("broadcast preview: %v", err)
	}
	f.exec(t, "UPDATE people SET active=false WHERE id=$1", f.owner.ID)
	if _, err = f.s.PreviewSelfTest(
		t.Context(),
		f.owner.ID,
		f.eventID,
		c.Definition.ID,
		"smtp",
	); err == nil {
		t.Fatal("inactive account preview succeeded")
	}
}

func sameCounts(a, b []int64) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSelfTestReviewedMailboxAndSourceStayBoundThroughWorker(t *testing.T) {
	f, c := previewFixture(t)
	var err error
	f.s.Jobs, err = jobs.New(
		f.pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{},
	)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.s.PreviewSelfTest(t.Context(), f.owner.ID, f.eventID, c.Definition.ID, "smtp")
	if err != nil {
		t.Fatal(err)
	}
	change := func(binding string) (CampaignDetail, error) {
		return f.s.ChangeCampaign(
			t.Context(),
			f.owner.ID,
			f.eventID,
			c.Definition.ID,
			f.revision(t),
			CampaignCommand{
				Action:                  "test_self",
				Version:                 c.Version,
				Channel:                 "smtp",
				Reason:                  "Synthetic reviewed TEST",
				ReviewedTestFingerprint: binding,
			},
		)
	}
	// Mailbox changes do not need an event revision bump; the review hash must bind them.
	f.exec(
		t,
		"UPDATE person_email_contacts SET mailbox='changed-organiser@synthetic.invalid' WHERE person_id=$1",
		f.owner.ID,
	)
	if _, err = change(preview.ReviewFingerprint); !errors.Is(err, access.ErrConflict) {
		t.Fatalf("stale mailbox review: %v", err)
	}
	if counts := previewCounts(t, f); counts[1] != 0 || counts[2] != 0 {
		t.Fatal("rejected review enqueued work")
	}
	f.exec(
		t,
		"UPDATE person_email_contacts SET mailbox='reviewed-organiser@synthetic.invalid' WHERE person_id=$1",
		f.owner.ID,
	)
	// Exact rendering can change independently of the campaign version.
	f.exec(t, "UPDATE people SET display_name='Changed organiser' WHERE id=$1", f.owner.ID)
	if _, err = change(preview.ReviewFingerprint); !errors.Is(err, access.ErrConflict) {
		t.Fatalf("stale personalised output: %v", err)
	}
	f.exec(t, "UPDATE people SET display_name='Preview organiser' WHERE id=$1", f.owner.ID)
	queued, err := change(preview.ReviewFingerprint)
	if err != nil || queued.TestRequestID == nil {
		t.Fatalf("reviewed queue: %v", err)
	}
	resolve := func() (TestMessage, error) {
		var value TestMessage
		err := database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
			var e error
			value, e = f.s.ResolveTestSelfInTransaction(
				t.Context(),
				tx,
				f.eventID,
				*queued.TestRequestID,
			)
			return e
		})
		return value, err
	}
	message, err := resolve()
	if err != nil || message.Subject != preview.Subject || message.PlainText != preview.PlainText ||
		message.ReviewedTestBinding != preview.ReviewFingerprint {
		t.Fatalf("persisted output differs from reviewed output: %v", err)
	}
	if err = database.Within(t.Context(), f.pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, e := q.Event(t.Context(), f.eventID)
		if e != nil {
			return e
		}
		_, legacy, _, e := f.s.selfMessage(
			t.Context(),
			q,
			row,
			f.owner.ID,
			c,
			*queued.TestRequestID,
			"smtp",
		)
		if e != nil {
			return e
		}
		retained, e := q.CommunicationTestRequest(
			t.Context(),
			dbgen.CommunicationTestRequestParams{EventID: f.eventID, ID: *queued.TestRequestID},
		)
		if e != nil {
			return e
		}
		if retained.Fingerprint == legacy ||
			retained.Fingerprint != reviewedTestDurableFingerprint(
				legacy,
				preview.ReviewFingerprint,
			) {
			t.Fatal("old core-only worker can accept the newly reviewed request")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// The worker resolves this before preparing its first immutable SMTP attempt.
	f.exec(
		t,
		"UPDATE person_email_contacts SET mailbox='changed-before-effect@synthetic.invalid' WHERE person_id=$1",
		f.owner.ID,
	)
	if _, err = resolve(); !errors.Is(err, ErrUnresolved) {
		t.Fatalf("mailbox changed before first effect: %v", err)
	}
	f.exec(
		t,
		"UPDATE person_email_contacts SET mailbox='reviewed-organiser@synthetic.invalid' WHERE person_id=$1",
		f.owner.ID,
	)
	// Template changes invalidate a review even if its visible strings stay equal.
	f.change(t, Command{Action: "save_template", Template: &f.t})
	if _, err = change(preview.ReviewFingerprint); !errors.Is(err, access.ErrConflict) {
		t.Fatalf("stale source version review: %v", err)
	}
	if _, err = resolve(); !errors.Is(err, ErrUnresolved) {
		t.Fatalf("changed source worker guard: %v", err)
	}
	// The new optional field is never meaningful on an unrelated command.
	_, err = f.s.ChangeCampaign(
		context.Background(),
		f.owner.ID,
		f.eventID,
		c.Definition.ID,
		f.revision(t),
		CampaignCommand{
			Action:                  "preview",
			Version:                 c.Version,
			Reason:                  "Synthetic",
			ReviewedTestFingerprint: strings.Repeat("a", 64),
		},
	)
	if !errors.Is(err, access.ErrInvalid) {
		t.Fatalf("fingerprint on non-test command: %v", err)
	}
}
