package outcomes

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/automation"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/inbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type reviewInboxOwner struct{ service *Service }

func (o reviewInboxOwner) AuthorizeNotification(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	source inbox.Source,
) (inbox.Destination, error) {
	if source.Kind != "outcome_review" {
		return inbox.Destination{}, access.ErrNotFound
	}
	err := o.service.AuthorizeReviewNotification(ctx, tx, actor, eventID, source.ID, source.Version)
	return inbox.Destination{Kind: "outcome_review", ResourceID: source.ID}, err
}

func TestActualNativeArchivedPlanDeliversToNoEventRoleOwnerAndRetainsHistory(t *testing.T) {
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
	composer := &communications.Service{Pool: pool, Now: func() time.Time { return now }}
	auto := &automation.Service{
		Pool:     pool,
		Composer: composer,
		Now:      func() time.Time { return now },
	}
	local := &inbox.Service{
		Pool:           pool,
		Communications: composer,
		ScopePolicy:    auto,
		Now:            func() time.Time { return now },
	}
	s := &Service{
		Pool:            pool,
		Composer:        composer,
		Automation:      auto,
		WorkspaceOrigin: "https://workspace.example.test",
		Now:             func() time.Time { return now },
	}
	composer.Channels = map[string]communications.ChannelProvider{"in_app": local}
	composer.OutcomePlans = s
	auto.Sources = map[string]automation.SourcePolicy{
		"outcome.follow_up": ReviewSources{Service: s},
	}
	local.Owners = map[string]inbox.ResourceOwner{"outcome_review": reviewInboxOwner{service: s}}
	outbox, err := jobs.New(
		pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{automation.HandlerKind: auto},
	)
	if err != nil {
		t.Fatal(err)
	}
	auto.Jobs = outbox
	auto.Providers = map[string]automation.ProviderQueue{
		"in_app": func(ctx context.Context, tx pgx.Tx, eventID, campaignID uuid.UUID, version int64, scope communications.DeliveryScope) (automation.ChannelReport, error) {
			n, e := local.QueueApprovedOccurrenceInTransaction(
				ctx,
				tx,
				eventID,
				campaignID,
				version,
				scope,
			)
			return automation.ChannelReport{LocalPersisted: n}, e
		},
	}
	id, review := uuid.New(), uuid.New()
	if _, err = s.Change(t.Context(), owner.ID, id, 0, proposal()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Change(
		t.Context(),
		org.ID,
		id,
		1,
		Command{
			ID:         uuid.New(),
			Action:     "adopt",
			Provenance: &Provenance{EventID: &eventID},
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
		Command{ID: uuid.New(), Action: "assign", OwnerID: &owner.ID, Reason: "Explicit owner"},
	); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Change(
		t.Context(),
		org.ID,
		id,
		3,
		Command{
			ID:     uuid.New(),
			Action: "moderate",
			Status: "accepted",
			Reason: "Accept follow-through",
		},
	); err != nil {
		t.Fatal(err)
	}
	save := PlanCommand{
		ID:            uuid.New(),
		Action:        "save",
		SourceVersion: 4,
		Reason:        "Optional 30 day follow-through",
		Schedule: &ReviewSchedule{
			Days:     30,
			WallTime: due.Format("2006-01-02T15:04:05"),
			Timezone: "UTC",
			Channels: []string{"in_app"},
		},
		Message: &ReviewMessage{
			Subject: "Outcome review",
			Body:    "Hello {{recipient.name}}; review {{outcome.url}} at {{review.date}}.",
		},
	}
	if _, err = s.ChangePlan(t.Context(), org.ID, id, review, 0, save); err != nil {
		t.Fatal(err)
	}
	approve := PlanCommand{
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
	page, err = local.List(t.Context(), owner.ID, nil, nil, 25)
	if err != nil || len(page.Items) != 1 || page.Unread != 1 || page.Items[0].EventName != "" {
		t.Fatal("actual no-event-role inbox or private event leak", page, err)
	}
	detail, err := local.Open(t.Context(), owner.ID, page.Items[0].ID, nil)
	if err != nil || detail.Destination == nil || detail.Destination.Kind != "outcome_review" {
		t.Fatal("native owning deep link", detail, err)
	}
	encoded, _ := json.Marshal(detail)
	if bytesContainAny(encoded, "Synthetic event", "PRIVATE original", "proposed_value") {
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
		Command{
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

func bytesContainAny(value []byte, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(string(value), needle) {
			return true
		}
	}
	return false
}
