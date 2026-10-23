package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
	"github.com/deploymenttheory/hack-in-a-box/internal/media"
	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestOutcomeRuntimeResolvesCurrentNativeMediaWithoutPromotingReaderAuthority(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	cfg, err := config.Load(func(key string) string {
		if key == "HACKBOX_ENV" {
			return "test"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := newCommunicationRuntime(
		pool,
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	org, author, subject, viewer, outsider := testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	)
	eventID := testdb.Event(t, pool, org.ID)
	if _, err = pool.Exec(
		ctx,
		`INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed'),($1,$3,'confirmed')`,
		eventID,
		subject.ID,
		viewer.ID,
	); err != nil {
		t.Fatal(err)
	}
	p := privacy.Service{Pool: pool}
	if _, err = p.SetOwnPhotoConsent(ctx, subject.ID, eventID, 0, "yes"); err != nil {
		t.Fatal(err)
	}
	revision := func() int64 {
		t.Helper()
		row, e := dbgen.New(pool).Event(ctx, eventID)
		if e != nil {
			t.Fatal(e)
		}
		return row.Revision
	}
	m := media.Service{Pool: pool, Privacy: p}
	item, err := m.Change(
		ctx,
		org.ID,
		eventID,
		uuid.New(),
		revision(),
		media.Command{
			Action:    "save",
			Version:   0,
			CommandID: uuid.New(),
			Reason:    "Review a native operational recording",
			Metadata: &media.Metadata{
				Kind:         "recording",
				Title:        "Reviewed venue introduction",
				RecordingURL: "https://recordings.example.test/PRIVATE-ORIGINAL",
				People:       []uuid.UUID{subject.ID},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	item, err = m.Change(
		ctx,
		org.ID,
		eventID,
		item.ID,
		revision(),
		media.Command{
			Action:    "review",
			Version:   item.Version,
			CommandID: uuid.New(),
			Reason:    "Review content and identified consent",
			Review:    &media.Review{ContentReviewed: true, IdentificationReviewed: true},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	item, err = m.Change(
		ctx,
		org.ID,
		eventID,
		item.ID,
		revision(),
		media.Command{
			Action:    "publish",
			Version:   item.Version,
			CommandID: uuid.New(),
			Reason:    "Publish separately reviewed recording",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	s := runtime.Outcomes
	id := uuid.New()
	if _, err = s.Change(
		ctx,
		author.ID,
		id,
		0,
		outcomes.Command{
			ID:     uuid.New(),
			Action: "propose",
			Reason: "Ordinary member idea",
			Definition: &outcomes.Definition{
				Kind:          "pitch",
				Problem:       "PRIVATE pitch",
				Audience:      "Organisation",
				ProposedValue: "Proposed benefit",
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Change(
		ctx,
		org.ID,
		id,
		1,
		outcomes.Command{
			ID:         uuid.New(),
			Action:     "adopt",
			Reason:     "Deliberately adopt",
			Provenance: &outcomes.Provenance{EventID: &eventID},
		},
	); err != nil {
		t.Fatal(err)
	}
	fields := outcomes.PublicationFields{
		Problem:  "Reviewed public problem",
		Solution: "Reviewed public solution",
		Audience: "organisation",
		Media:    []outcomes.MediaReference{{EventID: eventID, ID: item.ID, Version: item.Version}},
	}
	if _, err = s.ChangePublication(
		ctx,
		org.ID,
		id,
		0,
		outcomes.PublicationCommand{
			ID:            uuid.New(),
			Action:        "save",
			SourceVersion: 2,
			Fields:        &fields,
			Reason:        "Review positive fields and exact native media",
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ChangePublication(
		ctx,
		org.ID,
		id,
		1,
		outcomes.PublicationCommand{
			ID:            uuid.New(),
			Action:        "publish",
			SourceVersion: 2,
			Reason:        "Explicit public project publication",
		},
	); err != nil {
		t.Fatal(err)
	}
	project, err := s.Public(ctx, viewer.ID, id)
	if err != nil || len(project.Links) != 1 || !project.Links[0].Media ||
		project.Links[0].URL != "/events/"+eventID.String()+"/media/"+item.ID.String() {
		t.Fatal("native current reader", project, err)
	}
	raw, _ := json.Marshal(project)
	for _, secret := range []string{"PRIVATE", subject.ID.String(), "recordings.example.test", "people", "recording_url"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("private media provenance copied", secret, string(raw))
		}
	}
	project, err = s.Public(ctx, outsider.ID, id)
	if err != nil || len(project.Links) != 0 {
		t.Fatal("organisation card promoted native media authority", project, err)
	}
	// Archived self-service consent withdrawal remains authoritative for every
	// linked project read; a later yes cannot resurrect the old reviewed release.
	if _, err = pool.Exec(
		ctx,
		`UPDATE events SET stage='archived',completed_at=$2 WHERE id=$1`,
		eventID,
		time.Now().UTC(),
	); err != nil {
		t.Fatal(err)
	}
	consent, err := p.OwnPhotoConsent(ctx, subject.ID, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.SetOwnPhotoConsent(ctx, subject.ID, eventID, consent.Revision, "no"); err != nil {
		t.Fatal(err)
	}
	project, err = s.Public(ctx, viewer.ID, id)
	if err != nil || len(project.Links) != 0 {
		t.Fatal("revoked native media retained in project", project, err)
	}
	consent, err = p.OwnPhotoConsent(ctx, subject.ID, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.SetOwnPhotoConsent(
		ctx,
		subject.ID,
		eventID,
		consent.Revision,
		"yes",
	); err != nil {
		t.Fatal(err)
	}
	project, err = s.Public(ctx, viewer.ID, id)
	if err != nil || len(project.Links) != 0 {
		t.Fatal("consent restore resurrected old release", project, err)
	}
}

func TestOutcomeActualRuntimeArchivedOwnerReminderAndHistoricalRead(t *testing.T) {
	testOutcomeRuntimeReminder(t, "in_app", maildelivery.Accepted)
}

func TestOutcomeActualRuntimeSMTPPendingEffectBlocksReplacementApproval(t *testing.T) {
	testOutcomeRuntimeReminder(t, "smtp", maildelivery.Retryable)
}

func TestOutcomeActualRuntimeSMTPAmbiguousAttemptNeverAutomaticallyResends(t *testing.T) {
	testOutcomeRuntimeReminder(t, "smtp", maildelivery.Ambiguous)
}

type outcomeSyntheticRelay struct {
	calls atomic.Int32
	state maildelivery.State
	last  atomic.Pointer[maildelivery.Message]
}

func (*outcomeSyntheticRelay) Configured() bool { return true }
func (*outcomeSyntheticRelay) Revision() string { return "synthetic-outcome-relay-v1" }
func (*outcomeSyntheticRelay) MessageID(id uuid.UUID) string {
	return "<" + id.String() + "@example.test>"
}

func (r *outcomeSyntheticRelay) Send(
	_ context.Context,
	message maildelivery.Message,
) maildelivery.Receipt {
	r.calls.Add(1)
	r.last.Store(&message)
	return maildelivery.Receipt{State: r.state, Code: "synthetic_outcome"}
}

func testOutcomeRuntimeReminder(t *testing.T, channel string, receipt maildelivery.State) {
	pool := testdb.New(t, true)
	org, owner := testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, org.ID)
	actual := time.Now().UTC().Truncate(time.Second)
	now := actual.Add(-2 * time.Minute)
	due := actual.Add(-time.Minute)
	completed := due.AddDate(0, 0, -30)
	if _, err := pool.Exec(
		t.Context(),
		`UPDATE events SET stage='completed',completed_at=$2,configuration=jsonb_build_object('live_start',$3::text,'live_end',$4::text,'timezone','UTC') WHERE id=$1`,
		eventID,
		completed,
		completed.Add(-2*time.Hour).Format(time.RFC3339),
		completed.Add(-time.Hour).Format(time.RFC3339),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(
		t.Context(),
		`INSERT INTO event_gate_facts(event_id)VALUES($1)`,
		eventID,
	); err != nil {
		t.Fatal(err)
	}
	row, err := dbgen.New(pool).Event(t.Context(), eventID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = (event.Service{Pool: pool}).Transition(
		t.Context(),
		org.ID,
		eventID,
		row.Revision,
		"archived",
		"Archive competition records before explicitly approving outcome follow-through",
		false,
	); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(func(key string) string {
		if key == "HACKBOX_ENV" {
			return "test"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := newCommunicationRuntime(
		pool,
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	s, auto, local, outbox := runtime.Outcomes, runtime.Automation, runtime.Inbox, runtime.Jobs
	s.Now = func() time.Time { return now }
	auto.Now = func() time.Time { return now }
	local.Now = func() time.Time { return now }
	runtime.Communications.Now = func() time.Time { return now }
	runtime.SMTP.Now = func() time.Time { return now }
	relay := &outcomeSyntheticRelay{state: receipt}
	if channel == "smtp" {
		runtime.SMTP.Transport = relay
		if err = dbgen.New(pool).
			ConfirmPersonEmail(t.Context(), dbgen.ConfirmPersonEmailParams{PersonID: owner.ID, Mailbox: "synthetic-owner@example.test"}); err != nil {
			t.Fatal(err)
		}
	}
	id, review := uuid.New(), uuid.New()
	if _, err = s.Change(
		t.Context(),
		owner.ID,
		id,
		0,
		outcomes.Command{
			ID:     uuid.New(),
			Action: "propose",
			Reason: "Ordinary idea",
			Definition: &outcomes.Definition{
				Kind:          "pitch",
				Problem:       "PRIVATE original idea",
				Audience:      "Organisation",
				ProposedValue: "Proposed value",
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Change(
		t.Context(),
		org.ID,
		id,
		1,
		outcomes.Command{
			ID:         uuid.New(),
			Action:     "adopt",
			Provenance: &outcomes.Provenance{EventID: &eventID},
			Reason:     "Adopt an ordinary member idea after archive",
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Change(
		t.Context(),
		org.ID,
		id,
		2,
		outcomes.Command{
			ID:      uuid.New(),
			Action:  "assign",
			OwnerID: &owner.ID,
			Reason:  "Explicit owner",
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Change(
		t.Context(),
		org.ID,
		id,
		3,
		outcomes.Command{
			ID:     uuid.New(),
			Action: "moderate",
			Status: "accepted",
			Reason: "Accept follow-through",
		},
	); err != nil {
		t.Fatal(err)
	}
	save := outcomes.PlanCommand{
		ID:            uuid.New(),
		Action:        "save",
		SourceVersion: 4,
		Reason:        "Optional 30 day follow-through",
		Schedule: &outcomes.ReviewSchedule{
			Days:     30,
			WallTime: due.Format("2006-01-02T15:04:05"),
			Timezone: "UTC",
			Channels: []string{channel},
		},
		Message: &outcomes.ReviewMessage{
			Subject: "Outcome review",
			Body:    "Hello {{recipient.name}}; review {{outcome.url}} at {{review.date}}.",
		},
	}
	if _, err = s.ChangePlan(t.Context(), org.ID, id, review, 0, save); err != nil {
		t.Fatal(err)
	}
	approve := outcomes.PlanCommand{
		ID:            uuid.New(),
		Action:        "approve",
		SourceVersion: 4,
		Reason:        "Explicit exact archived review approval",
	}
	if _, err = s.ChangePlan(t.Context(), org.ID, id, review, 1, approve); err != nil {
		t.Fatal(err)
	}

	if err = auto.Maintain(t.Context()); err != nil {
		t.Fatal(err)
	}
	page, err := local.List(t.Context(), owner.ID, nil, nil, 25)
	if err != nil || len(page.Items) != 0 {
		t.Fatal("early reminder", page, err)
	}
	now = actual
	if err = auto.Maintain(t.Context()); err != nil {
		t.Fatal(err)
	}
	var operation uuid.UUID
	if err = pool.QueryRow(t.Context(), `SELECT operation_id FROM communication_occurrences WHERE source_id=$1`, review).
		Scan(&operation); err != nil {
		t.Fatal(err)
	}
	if err = outbox.Execute(t.Context(), operation); err != nil {
		t.Fatal(err)
	}
	if channel == "smtp" {
		var providerOperation uuid.UUID
		if err = pool.QueryRow(t.Context(), `SELECT operation_id FROM smtp_campaign_messages WHERE occurrence_id=(SELECT id FROM communication_occurrences WHERE source_id=$1)`, review).
			Scan(&providerOperation); err != nil {
			t.Fatal(err)
		}
		if receipt == maildelivery.Retryable {
			// No SMTP submission is attempted in this pending-operation scenario.
			reschedule := save
			reschedule.ID = uuid.New()
			reschedule.Schedule = &outcomes.ReviewSchedule{
				Days:     30,
				WallTime: actual.Add(time.Hour).Format("2006-01-02T15:04:05"),
				Timezone: "UTC",
				Channels: []string{"in_app"},
			}
			if _, err = s.ChangePlan(t.Context(), org.ID, id, review, 2, reschedule); err != nil {
				t.Fatal(err)
			}
			approve.ID = uuid.New()
			if _, err = s.ChangePlan(
				t.Context(),
				org.ID,
				id,
				review,
				3,
				approve,
			); !errors.Is(
				err,
				access.ErrConflict,
			) {
				t.Fatal("pending original operation bypassed by replacement", err)
			}
			if relay.calls.Load() != 0 {
				t.Fatal("approval sent the pending message")
			}
			return
		}
		if err = outbox.Execute(t.Context(), providerOperation); err != nil {
			t.Fatal(err)
		}
		if relay.calls.Load() != 1 {
			t.Fatal("native approved SMTP did not submit once", relay.calls.Load())
		}
		sent := relay.last.Load()
		if sent == nil {
			t.Fatal("no immutable SMTP message captured")
		}
		encoded, _ := json.Marshal(sent)
		if strings.Contains(string(encoded), "Synthetic event") ||
			strings.Contains(string(encoded), "PRIVATE original") ||
			sent.Calendar != nil {
			t.Fatal("native owner reminder leaked event configuration", string(encoded))
		}
		var state string
		if err = pool.QueryRow(t.Context(), `SELECT state FROM operations WHERE id=$1`, providerOperation).
			Scan(&state); err != nil ||
			state != "ambiguous" {
			t.Fatal("unknown relay outcome lost durable barrier", state, err)
		}
		// Cancellation cannot erase an unknown attempt or turn replay into a send.
		if _, err = s.ChangePlan(
			t.Context(),
			org.ID,
			id,
			review,
			2,
			outcomes.PlanCommand{
				ID:            uuid.New(),
				Action:        "cancel",
				SourceVersion: 4,
				Reason:        "Cancel future work while retaining the unknown attempt",
			},
		); err != nil {
			t.Fatal(err)
		}
		if err = outbox.Execute(t.Context(), operation); err != nil {
			t.Fatal(err)
		}
		if err = outbox.Execute(t.Context(), providerOperation); err != nil {
			t.Fatal(err)
		}
		if relay.calls.Load() != 1 {
			t.Fatal("unknown native owner reminder resent", relay.calls.Load())
		}
		var attempts int
		if err = pool.QueryRow(t.Context(), `SELECT count(*) FROM smtp_submission_attempts WHERE operation_id=$1`, providerOperation).
			Scan(&attempts); err != nil ||
			attempts != 1 {
			t.Fatal("unknown attempt history changed", attempts, err)
		}
		// Changing the schedule or selected channel is not an explicit disposition
		// of a previous unknown SMTP effect for this same checkpoint.
		reschedule := save
		reschedule.ID = uuid.New()
		reschedule.Schedule = &outcomes.ReviewSchedule{
			Days:     30,
			WallTime: actual.Add(time.Hour).Format("2006-01-02T15:04:05"),
			Timezone: "UTC",
			Channels: []string{"in_app"},
		}
		if _, err = s.ChangePlan(t.Context(), org.ID, id, review, 3, reschedule); err != nil {
			t.Fatal(err)
		}
		approve.ID = uuid.New()
		if _, err = s.ChangePlan(
			t.Context(),
			org.ID,
			id,
			review,
			4,
			approve,
		); !errors.Is(
			err,
			access.ErrConflict,
		) {
			t.Fatal("new channel/date bypassed unknown original effect", err)
		}
		current, err := s.Review(t.Context(), org.ID, review, 0)
		if err != nil || current.Version != 4 || current.State != "draft" {
			t.Fatal("blocked approval changed exact plan", current, err)
		}
		if relay.calls.Load() != 1 {
			t.Fatal("replacement approved or resent unknown effect")
		}
		return
	}
	page, err = local.List(t.Context(), owner.ID, nil, nil, 25)
	if err != nil || len(page.Items) != 1 || page.Unread != 1 || page.Items[0].EventName != "" {
		t.Fatal("actual no-event-role inbox or private event leak", page, err)
	}
	detail, err := local.Open(t.Context(), owner.ID, page.Items[0].ID, nil)
	if err != nil || detail.Destination == nil || detail.Destination.Kind != "outcome_review" {
		t.Fatal("native owning deep link", detail, err)
	}
	encoded, _ := json.Marshal(detail)
	if strings.Contains(string(encoded), "Synthetic event") ||
		strings.Contains(string(encoded), "PRIVATE original") ||
		strings.Contains(string(encoded), "proposed_value") {
		t.Fatal("unapproved event/private evidence disclosed", string(encoded))
	}
	if err = (access.EventPolicy{Queries: dbgen.New(pool)}).Authorize(
		t.Context(),
		owner.ID,
		eventID,
		eventID,
		access.EventRead,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("native reminder broadened event permission", err)
	}
	if _, err = s.Change(
		t.Context(),
		owner.ID,
		id,
		4,
		outcomes.Command{
			ID:     uuid.New(),
			Action: "progress",
			Status: "closed",
			Reason: "Complete owner follow-through",
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err = local.Open(t.Context(), owner.ID, page.Items[0].ID, nil); err != nil {
		t.Fatal("completion erased historical delivery", err)
	}
	if err = auto.Maintain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = outbox.Execute(t.Context(), operation); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = pool.QueryRow(t.Context(), `SELECT count(*) FROM inbox_notifications`).
		Scan(&count); err != nil ||
		count != 1 {
		t.Fatal("reminder replay duplicated delivery", count, err)
	}
	if _, err = pool.Exec(
		t.Context(),
		`UPDATE people SET active=false WHERE id=$1`,
		owner.ID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = local.Open(
		t.Context(),
		owner.ID,
		page.Items[0].ID,
		nil,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("revoked owner retains body", err)
	}
}
