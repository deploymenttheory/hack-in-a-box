package app

import (
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestNativeReviewApprovalRejectsExclusiveDefaultRetentionBoundary(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	organiser, owner := testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, organiser.ID)
	// Independent worked calendar example: January 1 + 90 days is April 1 in
	// 2026. The existing privacy deadline is exclusive, including at equality.
	completed := time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC)
	expiry := time.Date(2026, time.April, 1, 12, 0, 0, 0, time.UTC)
	phase := completed.Add(time.Hour)
	if _, err := pool.Exec(ctx, `UPDATE events SET stage='completed',completed_at=$2,configuration=jsonb_build_object('live_start',$3::text,'live_end',$4::text,'timezone','UTC') WHERE id=$1`,
		eventID, completed, completed.Add(-8*time.Hour).Format(time.RFC3339), completed.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	policy, err := (privacy.Service{Pool: pool}).Policy(ctx, organiser.ID, eventID)
	if err != nil || policy.RetentionDays != 90 {
		t.Fatal("component fixture did not retain the production default ninety-day policy", err)
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
	runtime, err := newCommunicationRuntime(pool, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), func() time.Time { return phase })
	if err != nil {
		t.Fatal(err)
	}
	outcomeID, reviewID := uuid.New(), uuid.New()
	for _, step := range []struct {
		actor   uuid.UUID
		version int64
		command outcomes.Command
	}{
		{owner.ID, 0, outcomes.Command{Action: "propose", Definition: &outcomes.Definition{Kind: "pitch", Problem: "Synthetic retention boundary", Audience: "Organisation", ProposedValue: "Owned follow-through"}}},
		{organiser.ID, 1, outcomes.Command{Action: "adopt", Provenance: &outcomes.Provenance{EventID: &eventID}}},
		{organiser.ID, 2, outcomes.Command{Action: "assign", OwnerID: &owner.ID}},
		{organiser.ID, 3, outcomes.Command{Action: "moderate", Status: "accepted"}},
	} {
		step.command.ID, step.command.Reason = uuid.New(), "Synthetic native boundary review"
		if _, err := runtime.Outcomes.Change(ctx, step.actor, outcomeID, step.version, step.command); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := runtime.Outcomes.ChangePlan(ctx, organiser.ID, outcomeID, reviewID, 0, outcomes.PlanCommand{
		ID: uuid.New(), Action: "save", SourceVersion: 4, Reason: "Proposed exact ninety-day boundary",
		Schedule: &outcomes.ReviewSchedule{Days: 90, WallTime: "2026-04-01T12:00:00", Timezone: "UTC", Channels: []string{"in_app"}},
		Message:  &outcomes.ReviewMessage{Subject: "Owned outcome review", Body: "Review {{outcome.url}} at {{review.date}}."},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Outcomes.ChangePlan(ctx, organiser.ID, outcomeID, reviewID, 1, outcomes.PlanCommand{
		ID: uuid.New(), Action: "approve", SourceVersion: 4, Reason: "Must respect the existing exclusive expiry",
	}); !errors.Is(err, access.ErrInvalid) {
		t.Fatalf("native review approval accepted an unsendable plan at exclusive retention expiry: %v", err)
	}
	phase = expiry.Add(-time.Second)
	if _, err := runtime.Outcomes.Read(ctx, owner.ID, outcomeID, 0); err != nil {
		t.Fatal("native owner lost access before the existing retention deadline", err)
	}
	phase = expiry
	if _, err := runtime.Outcomes.Read(ctx, owner.ID, outcomeID, 0); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("native owner retained access at exclusive default expiry", err)
	}
}
