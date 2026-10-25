package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

// Component/composition proof only: the completed-event fixture is not the
// full HTTP lifecycle setup. Every outcome/plan/policy decision and effect uses
// its native owner, real PostgreSQL and real queued River workers.
func TestConfiguredOwnedThirtySixtyNinetyDayReviewsUseRealQueueAndCurrentSource(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	organiser, owner := testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, organiser.ID)
	completed := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	// Independent worked calendar examples, not production deadline helpers.
	dates := map[int]time.Time{
		30: time.Date(2026, time.January, 31, 12, 0, 0, 0, time.UTC),
		60: time.Date(2026, time.March, 2, 12, 0, 0, 0, time.UTC),
		90: time.Date(2026, time.April, 1, 12, 0, 0, 0, time.UTC),
	}
	var phase atomic.Int64
	phase.Store(completed.Add(time.Hour).UnixNano())
	now := func() time.Time { return time.Unix(0, phase.Load()).UTC() }
	if _, err := pool.Exec(ctx, `UPDATE events SET stage='completed',completed_at=$2,configuration=jsonb_build_object('live_start',$3::text,'live_end',$4::text,'timezone','UTC') WHERE id=$1`,
		eventID, completed, completed.Add(-8*time.Hour).Format(time.RFC3339), completed.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO event_gate_facts(event_id) VALUES($1)", eventID); err != nil {
		t.Fatal(err)
	}
	row, err := dbgen.New(pool).Event(ctx, eventID)
	if err != nil {
		t.Fatal(err)
	}
	// Explicit synthetic operator choice, through the existing native privacy
	// owner: day-90 follow-through plus a 30-day response window, expiring May 1.
	// This does not change the production default or rescue an expired plan.
	if err := (privacy.Service{Pool: pool, Clock: now}).Configure(ctx, organiser.ID, eventID, row.Revision, privacy.Settings{RetentionDays: 120, DraftDays: 7}); err != nil {
		t.Fatal(err)
	}
	policy, err := (privacy.Service{Pool: pool}).Policy(ctx, organiser.ID, eventID)
	if err != nil || policy.RetentionDays != 120 {
		t.Fatal("explicit native policy covering the owned ninety-day review was not retained", err)
	}
	row, err = dbgen.New(pool).Event(ctx, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (event.Service{Pool: pool, Clock: now}).Transition(ctx, organiser.ID, eventID, row.Revision, "archived", "Synthetic configured day-90 follow-through plus thirty-day response window", false); err != nil {
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
	runtime, err := newCommunicationRuntime(pool, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), now)
	if err != nil {
		t.Fatal(err)
	}
	create := func(days int, stale bool) (uuid.UUID, uuid.UUID) {
		t.Helper()
		outcomeID, reviewID := uuid.New(), uuid.New()
		for _, step := range []struct {
			actor   uuid.UUID
			version int64
			command outcomes.Command
		}{
			{owner.ID, 0, outcomes.Command{Action: "propose", Definition: &outcomes.Definition{Kind: "pitch", Problem: "Synthetic configured follow-through", Audience: "Organisation", ProposedValue: "Owned outcome work"}}},
			{organiser.ID, 1, outcomes.Command{Action: "adopt", Provenance: &outcomes.Provenance{EventID: &eventID}}},
			{organiser.ID, 2, outcomes.Command{Action: "assign", OwnerID: &owner.ID}},
			{organiser.ID, 3, outcomes.Command{Action: "moderate", Status: "accepted"}},
		} {
			step.command.ID, step.command.Reason = uuid.New(), "Synthetic native owned follow-through"
			if _, err := runtime.Outcomes.Change(ctx, step.actor, outcomeID, step.version, step.command); err != nil {
				t.Fatal(err)
			}
		}
		subject := fmt.Sprintf("Owned day-%d review", days)
		if stale {
			subject = "Obsolete closed source must not send"
		}
		if _, err := runtime.Outcomes.ChangePlan(ctx, organiser.ID, outcomeID, reviewID, 0, outcomes.PlanCommand{
			ID: uuid.New(), Action: "save", SourceVersion: 4, Reason: "Explicit review within the configured retained response window",
			Schedule: &outcomes.ReviewSchedule{Days: days, WallTime: dates[days].Format("2006-01-02T15:04:05"), Timezone: "UTC", Channels: []string{"in_app"}},
			Message:  &outcomes.ReviewMessage{Subject: subject, Body: "Hello {{recipient.name}}; review {{outcome.url}} at {{review.date}}."},
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.Outcomes.ChangePlan(ctx, organiser.ID, outcomeID, reviewID, 1, outcomes.PlanCommand{
			ID: uuid.New(), Action: "approve", SourceVersion: 4, Reason: "Native owner/source/channel approval under explicit 120-day policy",
		}); err != nil {
			t.Fatal(err)
		}
		return outcomeID, reviewID
	}
	expected := map[uuid.UUID]int{}
	for _, days := range []int{30, 60, 90} {
		_, id := create(days, false)
		expected[id] = days
	}
	obsoleteOutcome, obsoleteReview := create(60, true)
	if err := runtime.Jobs.Client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stop, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := runtime.Jobs.Client.StopAndCancel(stop); err != nil {
			t.Error(err)
		}
	})
	if err := runtime.Automation.Maintain(ctx); err != nil {
		t.Fatal(err)
	}
	// Observe all four real queued jobs attempt and snooze before domain due.
	deadline := time.Now().Add(8 * time.Second)
	var scheduled int
	for time.Now().Before(deadline) {
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM river_job r JOIN operations o ON o.id=(r.args->>'operation_id')::uuid WHERE o.event_id=$1 AND r.state='scheduled' AND r.attempted_at IS NOT NULL", eventID).Scan(&scheduled); err != nil {
			t.Fatal(err)
		}
		if scheduled == 4 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if scheduled != 4 {
		t.Fatal("configured future native reviews did not retain four actual queued snoozes")
	}
	for index, days := range []int{30, 60, 90} {
		phase.Store(dates[days].UnixNano())
		if err := runtime.Automation.Maintain(ctx); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(45 * time.Second)
		for time.Now().Before(deadline) {
			page, err := runtime.Inbox.List(ctx, owner.ID, nil, nil, 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Items) == index+1 {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		page, err := runtime.Inbox.List(ctx, owner.ID, nil, nil, 10)
		if err != nil || len(page.Items) != index+1 {
			t.Fatalf("day-%d actual queued native effect census: count=%d error=%v", days, len(page.Items), err)
		}
		for _, item := range page.Items {
			detail, err := runtime.Inbox.Open(ctx, owner.ID, item.ID, nil)
			if err != nil || detail.Destination == nil || detail.Destination.Kind != "outcome_review" {
				t.Fatal("native reminder lost its actual owned resource", err)
			}
			day, ok := expected[detail.Destination.ResourceID]
			if !ok || day > days || detail.Subject != fmt.Sprintf("Owned day-%d review", day) || item.EventName != "" {
				t.Fatal("native queued reminder used a stale source, early date or event-role-only disclosure")
			}
		}
		t.Logf("native retained effect: day=%d domain=%s inbox_count=%d explicit_retention_days=%d", days, now().Format(time.RFC3339), len(page.Items), policy.RetentionDays)
		if days == 30 {
			if _, err := runtime.Outcomes.Change(ctx, owner.ID, obsoleteOutcome, 4, outcomes.Command{
				ID: uuid.New(), Action: "progress", Status: "closed", Reason: "Owner completed this source before its already queued day-60 reminder",
			}); err != nil {
				t.Fatal(err)
			}
			if err := runtime.Automation.Maintain(ctx); err != nil {
				t.Fatal(err)
			}
			var state string
			var cancelled bool
			var attempts int
			if err := pool.QueryRow(ctx, "SELECT c.state,o.cancelled,o.attempts FROM communication_occurrences c JOIN operations o ON o.id=c.operation_id WHERE c.event_id=$1 AND c.source_id=$2", eventID, obsoleteReview).Scan(&state, &cancelled, &attempts); err != nil {
				t.Fatal(err)
			}
			if state != "blocked" || !cancelled || attempts != 0 {
				t.Fatal("native current-source decision did not retire the already queued obsolete reminder without attempting it")
			}
		}
	}
}
