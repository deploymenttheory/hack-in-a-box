package app

import (
	"context"
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
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

// This focused production-composition regression uses a completed-event
// component fixture. It is not the full HTTP lifecycle's setup or evidence.
// The review, approval, occurrence, real River execution and inbox effect all
// belong to their native owners; no handler/Execute call or queue clock is used.
func TestArchivePreservesPrequeuedNativeReviewUsingDomainClock(t *testing.T) {
	for _, tc := range []struct {
		name        string
		stage       string
		advanceDays int
		cancelled   bool
	}{
		{name: "current_approval", stage: "archived"},
		{name: "past_review_window", stage: "archived", advanceDays: 31, cancelled: true},
		{name: "exclusive_retention_boundary", stage: "archived", advanceDays: 90, cancelled: true},
		{name: "cancelled_event", stage: "cancelled", cancelled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			verifyPrequeuedReviewAtTransition(t, tc.stage, tc.advanceDays, tc.cancelled)
		})
	}
}

func verifyPrequeuedReviewAtTransition(
	t *testing.T,
	stage string,
	advanceDays int,
	wantCancelled bool,
) {
	t.Helper()
	pool := testdb.New(t, true)
	ctx := t.Context()
	organiser, owner := testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, organiser.ID)
	wall := time.Now().UTC().Truncate(time.Second)
	completed := wall.AddDate(0, 0, -120)
	due := completed.AddDate(0, 0, 30)
	var phase atomic.Int64
	phase.Store(completed.Add(time.Hour).UnixNano())
	now := func() time.Time { return time.Unix(0, phase.Load()).UTC() }
	if _, err := pool.Exec(
		ctx,
		`UPDATE events SET stage='completed',completed_at=$2,configuration=jsonb_build_object('live_start',$3::text,'live_end',$4::text,'timezone','UTC') WHERE id=$1`,
		eventID,
		completed,
		completed.Add(-8*time.Hour).Format(time.RFC3339),
		completed.Format(time.RFC3339),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO event_gate_facts(event_id) VALUES($1)",
		eventID,
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
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	outcomeID, reviewID := uuid.New(), uuid.New()
	for _, step := range []struct {
		actor   uuid.UUID
		version int64
		command outcomes.Command
	}{
		{owner.ID, 0, outcomes.Command{Action: "propose", Definition: &outcomes.Definition{Kind: "pitch", Problem: "Synthetic queue regression", Audience: "Organisation", ProposedValue: "Owned follow-through"}}},
		{organiser.ID, 1, outcomes.Command{Action: "adopt", Provenance: &outcomes.Provenance{EventID: &eventID}}},
		{organiser.ID, 2, outcomes.Command{Action: "assign", OwnerID: &owner.ID}},
		{organiser.ID, 3, outcomes.Command{Action: "moderate", Status: "accepted"}},
	} {
		step.command.ID, step.command.Reason = uuid.New(), "Synthetic native review ownership"
		if _, err := runtime.Outcomes.Change(
			ctx,
			step.actor,
			outcomeID,
			step.version,
			step.command,
		); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := runtime.Outcomes.ChangePlan(
		ctx,
		organiser.ID,
		outcomeID,
		reviewID,
		0,
		outcomes.PlanCommand{
			ID:            uuid.New(),
			Action:        "save",
			SourceVersion: 4,
			Reason:        "Exact thirty-calendar-day follow-through",
			Schedule: &outcomes.ReviewSchedule{
				Days:     30,
				WallTime: due.Format("2006-01-02T15:04:05"),
				Timezone: "UTC",
				Channels: []string{"in_app"},
			},
			Message: &outcomes.ReviewMessage{
				Subject: "Owned outcome review",
				Body:    "Hello {{recipient.name}}; review {{outcome.url}} at {{review.date}}.",
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Outcomes.ChangePlan(
		ctx,
		organiser.ID,
		outcomeID,
		reviewID,
		1,
		outcomes.PlanCommand{
			ID:            uuid.New(),
			Action:        "approve",
			SourceVersion: 4,
			Reason:        "Explicit current archived review approval",
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Automation.Maintain(ctx); err != nil {
		t.Fatalf(
			"native retained enqueue rejected a domain-current plan (domain=%s wall=%s): %v",
			now().Format(time.RFC3339),
			wall.Format(time.RFC3339),
			err,
		)
	}
	if advanceDays > 0 {
		phase.Store(completed.AddDate(0, 0, advanceDays).UnixNano())
	}
	row, err := dbgen.New(pool).Event(ctx, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (event.Service{Pool: pool, Clock: now}).Transition(
		ctx,
		organiser.ID,
		eventID,
		row.Revision,
		stage,
		"Synthetic transition with an existing queued review",
		false,
	); err != nil {
		t.Fatal(err)
	}
	var operationID uuid.UUID
	if err := pool.QueryRow(ctx, "SELECT operation_id FROM communication_occurrences WHERE event_id=$1 AND source_id=$2", eventID, reviewID).
		Scan(&operationID); err != nil {
		t.Fatal(err)
	}
	var cancelled, approved bool
	if err := pool.QueryRow(ctx, "SELECT cancelled,approved FROM operations WHERE id=$1", operationID).
		Scan(&cancelled, &approved); err != nil {
		t.Fatal(err)
	}
	if cancelled != wantCancelled || approved == wantCancelled {
		t.Fatalf(
			"prequeued review after %s: cancelled=%t approved=%t; want cancelled=%t",
			stage,
			cancelled,
			approved,
			wantCancelled,
		)
	}
	if wantCancelled {
		return
	}
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

	// A historical DueAt is runnable by wall-time River, but the native domain
	// phase is still before the real thirty-day review. Observe its queued snooze.
	deadline := time.Now().Add(8 * time.Second)
	var state string
	var attempted bool
	for time.Now().Before(deadline) {
		if err := pool.QueryRow(ctx, "SELECT state::text,attempted_at IS NOT NULL FROM river_job WHERE args->>'operation_id'=$1 ORDER BY id DESC LIMIT 1", operationID.String()).
			Scan(&state, &attempted); err != nil {
			t.Fatal(err)
		}
		if attempted && state == "scheduled" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !attempted || state != "scheduled" {
		t.Fatalf("early queued native review did not remain scheduled: %s", state)
	}
	page, err := runtime.Inbox.List(ctx, owner.ID, nil, nil, 10)
	if err != nil || len(page.Items) != 0 {
		t.Fatal("early domain phase produced a native inbox effect", err)
	}
	phase.Store(due.UnixNano())
	// Wait for River's actual bounded snooze; do not JobRetry, edit queue dates,
	// invoke handlers or propagate this domain clock into River scheduling.
	deadline = time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		page, err = runtime.Inbox.List(ctx, owner.ID, nil, nil, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) == 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(page.Items) != 1 {
		t.Fatal("real queued native review did not resume after the domain due phase")
	}
	detail, err := runtime.Inbox.Open(ctx, owner.ID, page.Items[0].ID, nil)
	if err != nil || detail.Subject != "Owned outcome review" {
		t.Fatal("queued native inbox effect did not preserve its approved message", err)
	}
}
