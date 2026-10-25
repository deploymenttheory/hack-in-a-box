package automation

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type readyProvider struct{}

func (readyProvider) Readiness(
	context.Context,
	uuid.UUID,
	uuid.UUID,
) (communications.ChannelReadiness, error) {
	return communications.ChannelReadiness{
		Channel:        "smtp",
		Available:      true,
		ConfigRevision: "synthetic-v1",
	}, nil
}

type fixture struct {
	factID          *uuid.UUID
	t               *testing.T
	pool            *pgxpool.Pool
	owner, person   dbgen.Person
	id              uuid.UUID
	now             time.Time
	s               *Service
	campaignID      uuid.UUID
	campaignVersion int64
}

func newFixture(t *testing.T) *fixture {
	return newFixtureWithPool(t, testdb.New(t, true))
}

func newFixtureWithPool(t *testing.T, pool *pgxpool.Pool) *fixture {
	f := &fixture{
		t:      t,
		pool:   pool,
		owner:  testdb.Person(t, pool),
		person: testdb.Person(t, pool),
		now:    time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second),
	}
	f.id = testdb.Event(t, pool, f.owner.ID)
	raw, _ := json.Marshal(event.DefaultConfiguration())
	f.exec("UPDATE events SET configuration=$2 WHERE id=$1", f.id, raw)
	f.exec(
		"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
		f.id,
		f.person.ID,
	)
	f.exec(
		"CREATE TABLE synthetic_provider_intents(event_id uuid,occurrence_id uuid,person_id uuid,PRIMARY KEY(event_id,occurrence_id,person_id))",
	)
	composer := &communications.Service{
		Pool:     pool,
		Now:      func() time.Time { return f.now },
		Channels: map[string]communications.ChannelProvider{"smtp": readyProvider{}},
	}
	f.s = &Service{Pool: pool, Composer: composer, Now: func() time.Time { return f.now }}
	f.s.ReviewGuards = map[string]ReviewGuard{
		"smtp": func(context.Context, pgx.Tx, uuid.UUID, uuid.UUID, int64, communications.DeliveryScope) error {
			return nil
		},
	}
	f.s.Providers = map[string]ProviderQueue{
		"smtp": func(ctx context.Context, tx pgx.Tx, eventID, campaignID uuid.UUID, version int64, scope communications.DeliveryScope) (ChannelReport, error) {
			approved, err := composer.ResolveApprovedScopedCampaignInTransaction(
				ctx,
				tx,
				eventID,
				campaignID,
				version,
				scope,
				f.s,
			)
			if err != nil {
				return ChannelReport{}, err
			}
			for _, person := range approved.Preview.Recipients {
				if _, err = tx.Exec(
					ctx,
					"INSERT INTO synthetic_provider_intents VALUES($1,$2,$3) ON CONFLICT DO NOTHING",
					eventID,
					scope.OccurrenceID,
					person.PersonID,
				); err != nil {
					return ChannelReport{}, err
				}
			}
			return ChannelReport{LocalPersisted: len(approved.Preview.Recipients)}, nil
		},
	}
	outbox, err := jobs.New(
		pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{HandlerKind: f.s},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.s.Jobs = outbox
	composer.Jobs = outbox
	return f
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.t.Context(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) revision() int64 {
	f.t.Helper()
	row, err := dbgen.New(f.pool).Event(f.t.Context(), f.id)
	if err != nil {
		f.t.Fatal(err)
	}
	return row.Revision
}

func (f *fixture) change(c Command) (Page, *communications.PublicationPlanPreview) {
	f.t.Helper()
	p, preview, err := f.s.Change(f.t.Context(), f.owner.ID, f.id, f.revision(), c)
	if err != nil {
		f.t.Fatal(err)
	}
	return p, preview
}

func (f *fixture) campaign(purpose, body string, approve bool, channels ...string) {
	if len(channels) == 0 {
		channels = []string{"smtp"}
	}
	f.t.Helper()
	ctx := f.t.Context()
	template := communications.Template{
		ID:      uuid.New(),
		Name:    "Synthetic message",
		Purpose: purpose,
		FactID:  f.factID,
		Format:  "text",
		Subject: "Synthetic reminder",
		Body:    body,
	}
	audience := communications.Audience{
		ID:                 uuid.New(),
		Name:               "Current competitors",
		Roles:              []string{"attendee"},
		RegistrationStates: []string{},
		Teams:              []uuid.UUID{},
	}
	for _, c := range []communications.Command{{Action: "save_template", Template: &template, Reason: "Synthetic template"}, {Action: "save_audience", Audience: &audience, Reason: "Synthetic audience"}} {
		if _, err := f.s.Composer.Change(ctx, f.owner.ID, f.id, f.revision(), c); err != nil {
			f.t.Fatal(err)
		}
	}
	wall, choice := testdb.WallTimeForInstant(f.t, f.now.Add(time.Minute), "Europe/London")
	f.campaignID = uuid.New()
	definition := communications.CampaignDefinition{
		ID:         f.campaignID,
		Name:       "Synthetic campaign",
		TemplateID: template.ID,
		AudienceID: audience.ID,
		Channels:   channels,
		Timing: communications.CampaignTiming{
			Kind:      "fixed",
			WallTime:  wall,
			DSTChoice: choice,
		},
	}
	actions := []string{"save"}
	if approve {
		actions = append(actions, "preview", "approve")
	}
	for _, action := range actions {
		command := communications.CampaignCommand{
			Action:  action,
			Version: f.campaignVersion,
			Reason:  "Synthetic explicit review",
		}
		if action == "save" {
			command.Definition = &definition
		}
		detail, err := f.s.Composer.ChangeCampaign(
			ctx,
			f.owner.ID,
			f.id,
			f.campaignID,
			f.revision(),
			command,
		)
		if err != nil {
			var blocker *event.Blocked
			if errors.As(err, &blocker) {
				f.t.Fatalf("campaign %s: %v; blockers=%+v", action, err, blocker.Issues)
			}
			f.t.Fatal(err)
		}
		f.campaignVersion = detail.Campaign.Campaign.Version
	}
}

func (f *fixture) bind() Binding {
	f.t.Helper()
	id := uuid.New()
	d := Definition{
		ID:                     id,
		CampaignID:             f.campaignID,
		CampaignVersion:        f.campaignVersion,
		Trigger:                "scheduled",
		MaximumLatenessSeconds: 60,
	}
	p, _ := f.change(Command{Action: "save", Definition: &d, Reason: "Create synthetic schedule"})
	p, _ = f.change(
		Command{
			Action:    "approve",
			BindingID: id,
			Version:   p.Bindings[0].Version,
			Reason:    "Approve synthetic schedule",
		},
	)
	return p.Bindings[0]
}

func (f *fixture) maintain() Page {
	f.t.Helper()
	if err := f.s.Maintain(f.t.Context()); err != nil {
		f.t.Fatal(err)
	}
	p, err := f.s.Read(f.t.Context(), f.owner.ID, f.id)
	if err != nil {
		f.t.Fatal(err)
	}
	return p
}

func (f *fixture) execute(id uuid.UUID) {
	f.t.Helper()
	row, err := dbgen.New(f.pool).
		AutomationOccurrence(f.t.Context(), dbgen.AutomationOccurrenceParams{EventID: f.id, ID: id})
	if err != nil {
		f.t.Fatal(err)
	}
	if row.OperationID == nil {
		f.t.Fatal("no durable dispatch operation")
	}
	if err = f.s.Jobs.Execute(
		f.t.Context(),
		*row.OperationID,
	); err != nil &&
		!errors.Is(err, jobs.ErrRetry) {
		f.t.Fatal(err)
	}
}

func (f *fixture) count() int {
	f.t.Helper()
	var count int
	if err := f.pool.QueryRow(f.t.Context(), "SELECT count(*) FROM synthetic_provider_intents WHERE event_id=$1", f.id).
		Scan(&count); err != nil {
		f.t.Fatal(err)
	}
	return count
}

func TestSchedulePauseResumeMissedReviewAndCurrentAudience(t *testing.T) {
	f := newFixture(t)
	f.campaign("reminder", "Hello {{person.name}}", true)
	f.bind()
	p := f.maintain()
	if len(p.Occurrences) != 1 {
		t.Fatal("schedule not durable")
	}
	id := p.Occurrences[0].ID
	f.change(Command{Action: "pause", Reason: "Pause synthetic sends"})
	f.now = f.now.Add(3 * time.Minute)
	f.execute(id)
	if f.count() != 0 {
		t.Fatal("paused schedule sent")
	}
	f.change(Command{Action: "resume", Reason: "Resume requires missed review"})
	p = f.maintain()
	if p.Occurrences[0].State != "missed" {
		t.Fatalf("late work burst after resume: %+v", p.Occurrences[0])
	}
	f.execute(id)
	if f.count() != 0 {
		t.Fatal("missed action sent without review")
	}
	f.change(
		Command{
			Action:       "review_occurrence",
			OccurrenceID: id,
			Reason:       "Explicitly review one late action",
		},
	)
	other := testdb.Person(t, f.pool)
	f.exec("INSERT INTO competition_claims VALUES($1,$2,'confirmed')", f.id, other.ID)
	f.exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		f.id,
		f.person.ID,
	)
	f.execute(id)
	f.execute(id)
	if f.count() != 1 {
		t.Fatal("current audience/dedup failed")
	}
	var recipient uuid.UUID
	if err := f.pool.QueryRow(t.Context(), "SELECT person_id FROM synthetic_provider_intents WHERE event_id=$1", f.id).
		Scan(&recipient); err != nil ||
		recipient != other.ID {
		t.Fatal("stale recipient selected", err)
	}
}

func TestReschedulingAndCancellationInvalidateQueuedAutomation(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(
			map[bool]string{false: "changed_approval", true: "cancelled"}[cancel],
			func(t *testing.T) {
				f := newFixture(t)
				f.campaign("reminder", "Public reminder", true)
				f.bind()
				p := f.maintain()
				id := p.Occurrences[0].ID
				c, err := f.s.Composer.Campaign(t.Context(), f.owner.ID, f.id, f.campaignID)
				if err != nil {
					t.Fatal(err)
				}
				command := communications.CampaignCommand{
					Action:  "cancel",
					Version: f.campaignVersion,
					Reason:  "Invalidate obsolete automation",
				}
				if !cancel {
					definition := c.Campaign.Campaign.Definition
					definition.Timing.WallTime, definition.Timing.DSTChoice = testdb.WallTimeForInstant(t, f.now.Add(10*time.Minute), c.Timezone)
					command.Action = "save"
					command.Definition = &definition
				}
				changed, err := f.s.Composer.ChangeCampaign(
					t.Context(),
					f.owner.ID,
					f.id,
					f.campaignID,
					f.revision(),
					command,
				)
				if err != nil {
					t.Fatal(err)
				}
				if !cancel && (changed.Campaign.DueAt == nil || !changed.Campaign.DueAt.Equal(f.now.Add(10*time.Minute))) {
					t.Fatalf("reschedule changed exact target %s: %+v", f.now.Add(10*time.Minute), changed.Campaign.DueAt)
				}
				f.now = f.now.Add(time.Minute)
				invalidated := f.maintain()
				expectedState := "blocked"
				if cancel {
					expectedState = "cancelled"
				}
				if invalidated.Occurrences[0].State != expectedState {
					t.Fatalf(
						"obsolete queued work lacks durable review state: %+v",
						invalidated.Occurrences[0],
					)
				}
				f.execute(id)
				if f.count() != 0 {
					t.Fatal("obsolete approved campaign sent")
				}
			},
		)
	}
}
