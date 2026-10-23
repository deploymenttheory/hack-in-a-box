package outcomes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

// This reports configuration only. No synthetic transport result is used as
// evidence of an actual provider delivery.
type reviewProvider struct{ revision string }

func (p *reviewProvider) Readiness(
	context.Context,
	uuid.UUID,
	uuid.UUID,
) (communications.ChannelReadiness, error) {
	return communications.ChannelReadiness{Available: true, ConfigRevision: p.revision}, nil
}

func TestNativeReviewApprovalBindsOwnerPolicyProviderAndExactCommand(t *testing.T) {
	pool := testdb.New(t, true)
	org, owner := testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, org.ID)
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := pool.Exec(
		t.Context(),
		`UPDATE events SET configuration=jsonb_build_object('live_start',$2::text,'live_end',$3::text,'timezone','UTC') WHERE id=$1`,
		eventID,
		now.Add(-2*time.Hour).Format(time.RFC3339),
		now.Add(-time.Hour).Format(time.RFC3339),
	); err != nil {
		t.Fatal(err)
	}
	provider := &reviewProvider{revision: "transport-config-1"}
	composer := &communications.Service{
		Pool:     pool,
		Channels: map[string]communications.ChannelProvider{"in_app": provider},
	}
	s := Service{Pool: pool, Composer: composer, Now: func() time.Time { return now }}
	id, review := uuid.New(), uuid.New()
	if _, err := s.Change(t.Context(), owner.ID, id, 0, proposal()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Change(
		t.Context(),
		org.ID,
		id,
		1,
		Command{
			ID:         uuid.New(),
			Action:     "adopt",
			Provenance: &Provenance{EventID: &eventID},
			Reason:     "Adopt independently of results",
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Change(
		t.Context(),
		org.ID,
		id,
		2,
		Command{
			ID:      uuid.New(),
			Action:  "assign",
			OwnerID: &owner.ID,
			Reason:  "Current ordinary owner",
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Change(
		t.Context(),
		org.ID,
		id,
		3,
		Command{
			ID:     uuid.New(),
			Action: "moderate",
			Status: "accepted",
			Reason: "Accept future follow-through",
		},
	); err != nil {
		t.Fatal(err)
	}
	message := ReviewMessage{
		Subject: "Follow-through review",
		Body:    "Hello {{recipient.name}}. Review {{outcome.url}} at {{review.date}}.",
	}
	schedule := ReviewSchedule{
		Days:     30,
		WallTime: now.AddDate(0, 0, 30).Format("2006-01-02T15:04:05"),
		Timezone: "UTC",
		Channels: []string{"in_app"},
	}
	save := PlanCommand{
		ID:            uuid.New(),
		Action:        "save",
		SourceVersion: 4,
		Schedule:      &schedule,
		Message:       &message,
		Reason:        "Optional explicit 30 day review",
	}
	if _, err := s.ChangePlan(
		t.Context(),
		owner.ID,
		id,
		review,
		0,
		save,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("ordinary owner approved their own reminders", err)
	}
	draft, err := s.ChangePlan(t.Context(), org.ID, id, review, 0, save)
	if err != nil || draft.State != "draft" {
		t.Fatal(draft, err)
	}
	approve := PlanCommand{
		ID:            uuid.New(),
		Action:        "approve",
		SourceVersion: 4,
		Reason:        "Approve exact composition, owner, channel and timing",
	}
	approved, err := s.ChangePlan(t.Context(), org.ID, id, review, 1, approve)
	if err != nil || approved.EffectiveState != "approved" {
		t.Fatal(approved, err)
	}
	own, err := s.Plans(t.Context(), owner.ID, id)
	if err != nil || len(own.Items) != 1 || own.Items[0].EffectiveState != "approved" {
		t.Fatal("no event role owner plan", own, err)
	}
	if own.Items[0].Message.Subject != "" || own.Items[0].Message.Body != "" ||
		own.Items[0].Reason != "" ||
		own.Items[0].ActorID != uuid.Nil {
		t.Fatal("organiser reminder configuration leaked through owner metadata", own)
	}
	policy := privacy.Service{Pool: pool}
	configure := func(days int) {
		t.Helper()
		current, e := dbgen.New(pool).Event(t.Context(), eventID)
		if e != nil {
			t.Fatal(e)
		}
		if e = policy.Configure(
			t.Context(),
			org.ID,
			eventID,
			current.Revision,
			privacy.Settings{RetentionDays: int32(days), DraftDays: 7},
		); e != nil {
			t.Fatal(e)
		}
	}

	configure(90)
	own, err = s.Plans(t.Context(), owner.ID, id)
	if err != nil || own.Items[0].EffectiveState != "approved" {
		t.Fatal("no-op policy invalidated approval", own, err)
	}
	configure(91)
	configure(90)
	own, err = s.Plans(t.Context(), owner.ID, id)
	if err != nil || own.Items[0].EffectiveState != "review_required" {
		t.Fatal("changed then restored policy revived plan", own, err)
	}
	replay, err := s.ChangePlan(t.Context(), org.ID, id, review, 1, approve)
	if err != nil || replay.Version != 2 || replay.EffectiveState != "review_required" {
		t.Fatal("response-lost replay reapproved stale policy", replay, err)
	}
	save.ID = uuid.New()
	if _, err = s.ChangePlan(t.Context(), org.ID, id, review, 2, save); err != nil {
		t.Fatal(err)
	}
	approve.ID = uuid.New()
	if _, err = s.ChangePlan(t.Context(), org.ID, id, review, 3, approve); err != nil {
		t.Fatal(err)
	}
	provider.revision = "transport-config-2"
	own, err = s.Plans(t.Context(), owner.ID, id)
	if err != nil || own.Items[0].EffectiveState != "review_required" {
		t.Fatal("provider change revived plan", own, err)
	}
	provider.revision = "transport-config-1"
	if _, err = s.Change(
		t.Context(),
		owner.ID,
		id,
		4,
		Command{
			ID:     uuid.New(),
			Action: "progress",
			Status: "closed",
			Reason: "Complete owner follow-through",
		},
	); err != nil {
		t.Fatal(err)
	}
	own, err = s.Plans(t.Context(), owner.ID, id)
	if err != nil || own.Items[0].EffectiveState != "review_required" {
		t.Fatal("completed outcome retains send approval", own, err)
	}
}
