package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type fixture struct {
	pool          *pgxpool.Pool
	service       Service
	owner, person dbgen.Person
	eventID       uuid.UUID
	now           time.Time
	campaign      communications.CampaignDetail
	worker        *jobs.Service
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := testdb.New(t, true)
	f := &fixture{
		pool:   pool,
		owner:  testdb.Person(t, pool),
		person: testdb.Person(t, pool),
		now:    time.Date(2029, 6, 1, 9, 0, 0, 0, time.UTC),
	}
	f.eventID = testdb.Event(t, pool, f.owner.ID)
	c := event.DefaultConfiguration()
	c.LiveStart = f.now.AddDate(0, 0, 10)
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	c.SubmissionCutoff = c.LiveEnd.Add(-time.Hour)
	raw, _ := json.Marshal(c)
	f.exec(t, "UPDATE events SET configuration=$2,stage='registration' WHERE id=$1", f.eventID, raw)
	f.exec(t, "INSERT INTO event_gate_facts(event_id) VALUES($1)", f.eventID)
	f.exec(
		t,
		"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
		f.eventID,
		f.person.ID,
	)
	handlers := map[string]jobs.Handler{}
	var err error
	f.worker, err = jobs.New(pool, slog.New(slog.NewTextHandler(io.Discard, nil)), handlers)
	if err != nil {
		t.Fatal(err)
	}
	comm := communications.Service{
		Pool:     pool,
		Now:      func() time.Time { return f.now },
		Jobs:     f.worker,
		Channels: map[string]communications.ChannelProvider{},
	}
	f.service = Service{Pool: pool, Communications: &comm, Now: func() time.Time { return f.now }}
	comm.Channels["in_app"] = f.service
	handlers["communication_test_in_app"] = TestHandler{Service: f.service}
	template := communications.Template{
		ID:      uuid.New(),
		Name:    "Synthetic personal announcement",
		Purpose: "reminder",
		Format:  "text",
		Subject: "Private {{person.name}}",
		Body:    "Only {{person.name}} sees this harmless body",
	}
	audience := communications.Audience{
		ID:                 uuid.New(),
		Name:               "Confirmed only",
		Roles:              []string{"attendee"},
		RegistrationStates: []string{},
		Teams:              []uuid.UUID{},
	}
	for _, cmd := range []communications.Command{{Action: "save_template", Template: &template}, {Action: "save_audience", Audience: &audience}} {
		cmd.Reason = "Synthetic setup"
		if _, err = comm.Change(
			t.Context(),
			f.owner.ID,
			f.eventID,
			f.revision(t),
			cmd,
		); err != nil {
			t.Fatal(err)
		}
	}
	definition := communications.CampaignDefinition{
		ID:         uuid.New(),
		Name:       "Synthetic inbox campaign",
		TemplateID: template.ID,
		AudienceID: audience.ID,
		Channels:   []string{"in_app"},
		Timing:     communications.CampaignTiming{Kind: "relative", Anchor: "live_start", Days: -1},
	}
	f.campaign = f.change(
		t,
		communications.CampaignCommand{Action: "save", Definition: &definition},
	)
	return f
}

func (f *fixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) revision(t *testing.T) int64 {
	t.Helper()
	row, err := dbgen.New(f.pool).Event(t.Context(), f.eventID)
	if err != nil {
		t.Fatal(err)
	}
	return row.Revision
}

func (f *fixture) change(
	t *testing.T,
	c communications.CampaignCommand,
) communications.CampaignDetail {
	t.Helper()
	id := f.campaign.Campaign.Campaign.Definition.ID
	if c.Definition != nil {
		id = c.Definition.ID
	}
	c.Version = f.campaign.Campaign.Campaign.Version
	c.Reason = "Synthetic reviewed intent"
	result, err := f.service.Communications.ChangeCampaign(
		t.Context(),
		f.owner.ID,
		f.eventID,
		id,
		f.revision(t),
		c,
	)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func (f *fixture) deliver(t *testing.T) {
	t.Helper()
	f.campaign = f.change(t, communications.CampaignCommand{Action: "preview"})
	f.campaign = f.change(t, communications.CampaignCommand{Action: "approve"})
	f.now = f.now.AddDate(0, 0, 10)
	err := database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		_, err := f.service.QueueApprovedInTransaction(
			t.Context(),
			tx,
			f.eventID,
			f.campaign.Campaign.Campaign.Definition.ID,
			f.campaign.Campaign.Campaign.Version,
		)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestInboxCurrentRecipientDedupReadAndRevocation(t *testing.T) {
	f := newFixture(t)
	f.deliver(t)
	ctx := t.Context()
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			err := database.Within(ctx, f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
				_, err := f.service.QueueApprovedInTransaction(
					ctx,
					tx,
					f.eventID,
					f.campaign.Campaign.Campaign.Definition.ID,
					f.campaign.Campaign.Campaign.Version,
				)
				return err
			})
			if err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	page, err := f.service.List(ctx, f.person.ID, nil, nil, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Unread != 1 || page.Items[0].Label != "Event announcement" {
		t.Fatalf("dedup/current count: %+v", page)
	}
	raw, _ := json.Marshal(page)
	if strings.Contains(string(raw), "Only") || strings.Contains(string(raw), "Private") {
		t.Fatal("list exposed personal body or subject")
	}
	id := page.Items[0].ID
	for _, statement := range []string{"UPDATE inbox_notifications SET body='Changed immutable body' WHERE id=$1", "UPDATE inbox_notifications SET provenance='{}'::jsonb WHERE id=$1"} {
		if _, err = f.pool.Exec(ctx, statement, id); err == nil {
			t.Fatal("delivered message identity/provenance was mutable")
		}
	}
	if _, err = f.service.Open(ctx, f.owner.ID, id, nil); !errors.Is(err, access.ErrNotFound) {
		t.Fatalf("other recipient read: %v", err)
	}
	read := true
	detail, err := f.service.Open(ctx, f.person.ID, id, &read)
	if err != nil || !strings.Contains(detail.Body, f.person.DisplayName) ||
		detail.Item.ReadAt == nil {
		t.Fatalf("own read: %+v %v", detail, err)
	}
	unread := false
	if _, err = f.service.Open(ctx, f.person.ID, id, &unread); err != nil {
		t.Fatal(err)
	}
	f.exec(
		t,
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		f.eventID,
		f.person.ID,
	)
	// An overlapping unrelated staff grant must not preserve the confirmed audience.
	f.exec(
		t,
		"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'photographer')",
		f.eventID,
		f.person.ID,
	)
	page, err = f.service.List(ctx, f.person.ID, nil, nil, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 0 || page.Unread != 0 || page.NextCursor != nil {
		t.Fatal("revoked source leaked count or preview")
	}
	if _, err = f.service.Open(ctx, f.person.ID, id, &read); !errors.Is(err, access.ErrNotFound) {
		t.Fatalf("revoked source read: %v", err)
	}
}

func TestInboxSelfTestIsActualFixedOrganiserDeliveryAndReconciles(t *testing.T) {
	f := newFixture(t)
	f.campaign = f.change(t, communications.CampaignCommand{Action: "test_self", Channel: "in_app"})
	if f.campaign.TestRequestID == nil {
		t.Fatal("no test")
	}
	var operation uuid.UUID
	if err := f.pool.QueryRow(t.Context(), "SELECT id FROM operations WHERE event_id=$1 AND target_id=$2", f.eventID, *f.campaign.TestRequestID).
		Scan(&operation); err != nil {
		t.Fatal(err)
	}
	if err := f.worker.Execute(t.Context(), operation); err != nil {
		t.Fatal(err)
	}
	handler := TestHandler{Service: f.service}
	if outcome := handler.Reconcile(
		t.Context(),
		jobs.Delivery{ID: operation, EventID: f.eventID, TargetID: *f.campaign.TestRequestID},
	); outcome != jobs.Delivered {
		t.Fatal(outcome)
	}
	page, err := f.service.List(t.Context(), f.owner.ID, nil, nil, 10)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("own test: %+v %v", page, err)
	}
	detail, err := f.service.Open(t.Context(), f.owner.ID, page.Items[0].ID, nil)
	if err != nil || !strings.HasPrefix(detail.Subject, "[TEST]") {
		t.Fatalf("test label: %+v %v", detail, err)
	}
	other, err := f.service.List(t.Context(), f.person.ID, nil, nil, 10)
	if err != nil || len(other.Items) != 0 {
		t.Fatal("test reached selected competitor")
	}
}

func TestInboxDeliveredHistorySurvivesCampaignCancelSourceEditAndArchive(t *testing.T) {
	f := newFixture(t)
	f.deliver(t)
	page, err := f.service.List(t.Context(), f.person.ID, nil, nil, 10)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("delivered history: %+v %v", page, err)
	}
	id := page.Items[0].ID
	original, err := f.service.Open(t.Context(), f.person.ID, id, nil)
	if err != nil {
		t.Fatal(err)
	}
	f.campaign = f.change(t, communications.CampaignCommand{Action: "cancel"})
	var template communications.Template
	source, err := f.service.Communications.Read(t.Context(), f.owner.ID, f.eventID)
	if err != nil {
		t.Fatal(err)
	}
	template = source.Templates[0]
	template.Body = "Later changed body"
	_, err = f.service.Communications.Change(
		t.Context(),
		f.owner.ID,
		f.eventID,
		f.revision(t),
		communications.Command{
			Action:   "save_template",
			Template: &template,
			Reason:   "Later content edit",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, "UPDATE events SET stage='archived',writes_allowed=false WHERE id=$1", f.eventID)
	read := true
	current, err := f.service.Open(t.Context(), f.person.ID, id, &read)
	if err != nil || current.Body != original.Body || current.Item.ReadAt == nil {
		t.Fatalf("readable archive lost immutable delivery: %+v %v", current, err)
	}
	page, err = f.service.List(t.Context(), f.person.ID, nil, nil, 10)
	if err != nil || len(page.Items) != 1 || page.Unread != 0 {
		t.Fatalf("readable historical list: %+v %v", page, err)
	}
	f.exec(t, "UPDATE people SET active=false WHERE id=$1", f.person.ID)
	if _, err = f.service.Open(
		t.Context(),
		f.person.ID,
		id,
		&read,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatalf("inactive recipient retained history: %v", err)
	}
}

func TestInboxReleasedResourceWithdrawnAndFutureOwnersUnavailable(t *testing.T) {
	f := newFixture(t)
	resource := uuid.New()
	f.exec(
		t,
		"INSERT INTO content_items(event_id,id,kind,revision,version,state,released_at) VALUES($1,$2,'page',1,1,'draft',$3)",
		f.eventID,
		resource,
		f.now.Add(-time.Hour),
	)
	f.exec(
		t,
		"INSERT INTO content_versions(event_id,content_id,version,document,actor_id,created_at) VALUES($1,$2,1,$3,$4,now())",
		f.eventID,
		resource,
		[]byte(
			`{"title":"PRIVATE-LONG-TITLE","markdown":"Harmless released body","audience":{"confirmed":true,"roles":[],"teams":[]}}`,
		),
		f.owner.ID,
	)
	f.exec(
		t,
		"UPDATE content_items SET state='released',released_version=1 WHERE event_id=$1 AND id=$2",
		f.eventID,
		resource,
	)
	record := func(kind string) error {
		return database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
			return f.service.RecordResourceInTransaction(
				t.Context(),
				tx,
				f.eventID,
				f.person.ID,
				Source{Kind: kind, ID: resource, Version: 1},
			)
		})
	}
	if err := record("content"); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"help", "clarification", "outcome_review"} {
		if err := record(kind); !errors.Is(err, access.ErrNotFound) {
			t.Fatalf("unimplemented %s journey fabricated: %v", kind, err)
		}
	}
	page, err := f.service.List(t.Context(), f.person.ID, &f.eventID, nil, 10)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("resource notification: %+v %v", page, err)
	}
	raw, _ := json.Marshal(page)
	if strings.Contains(string(raw), "PRIVATE") {
		t.Fatal("resource title leaked in preview")
	}
	detail, err := f.service.Open(t.Context(), f.person.ID, page.Items[0].ID, nil)
	if err != nil || detail.Destination == nil || detail.Destination.Kind != "content" ||
		detail.Destination.ResourceID != resource {
		t.Fatalf("resource deep link: %+v %v", detail, err)
	}
	f.exec(
		t,
		"UPDATE content_items SET state='withdrawn' WHERE event_id=$1 AND id=$2",
		f.eventID,
		resource,
	)
	page, err = f.service.List(t.Context(), f.person.ID, nil, nil, 10)
	if err != nil || len(page.Items) != 0 || page.Unread != 0 {
		t.Fatal("withdrawn resource leaked preview or counts")
	}
	if _, err = f.service.Open(
		t.Context(),
		f.person.ID,
		detail.Item.ID,
		nil,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatalf("withdrawn resource opened: %v", err)
	}
}

type syntheticScope struct {
	recipient uuid.UUID
	binding   string
}

func (p syntheticScope) ResolveInTransaction(
	context.Context,
	pgx.Tx,
	uuid.UUID,
	uuid.UUID,
	int64,
	communications.DeliveryScope,
) (communications.ScopeAudience, error) {
	return communications.ScopeAudience{
		Binding:      p.binding,
		RecipientIDs: []uuid.UUID{p.recipient},
	}, nil
}

func TestInboxScopedOccurrencesDeduplicateIndependentlyAndIntersectCurrentAudience(t *testing.T) {
	f := newFixture(t)
	f.deliver(t)
	ctx := t.Context()
	scope := communications.DeliveryScope{OccurrenceID: uuid.New()}
	deliver := func() (int, error) {
		count := 0
		err := database.Within(ctx, f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
			var err error
			count, err = f.service.QueueApprovedOccurrenceInTransaction(
				ctx,
				tx,
				f.eventID,
				f.campaign.Campaign.Campaign.Definition.ID,
				f.campaign.Campaign.Campaign.Version,
				scope,
			)
			return err
		})
		return count, err
	}
	if _, err := deliver(); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("missing scope policy allowed: %v", err)
	}
	f.service.ScopePolicy = syntheticScope{recipient: f.person.ID, binding: "occurrence-v1"}
	for range 2 {
		if count, err := deliver(); err != nil || count != 1 {
			t.Fatalf("scoped queue: %d %v", count, err)
		}
	}
	scope.OccurrenceID = uuid.New()
	if _, err := deliver(); err != nil {
		t.Fatal(err)
	}
	page, err := f.service.List(ctx, f.person.ID, nil, nil, 10)
	if err != nil || len(page.Items) != 3 {
		t.Fatalf("independent occurrence dedup: %+v %v", page, err)
	}
	first, err := f.service.List(ctx, f.person.ID, nil, nil, 1)
	if err != nil || len(first.Items) != 1 || first.NextCursor == nil || first.Unread != 3 {
		t.Fatalf("first authorised page: %+v %v", first, err)
	}
	second, err := f.service.List(ctx, f.person.ID, nil, first.NextCursor, 1)
	if err != nil || len(second.Items) != 1 || second.Items[0].ID == first.Items[0].ID ||
		second.Unread != 3 {
		t.Fatalf("next authorised page: %+v %v", second, err)
	}
	f.service.ScopePolicy = syntheticScope{recipient: f.person.ID, binding: "changed-intent"}
	if _, err = deliver(); !errors.Is(err, access.ErrConflict) {
		t.Fatalf("changed scope binding silently reused: %v", err)
	}
	scope.OccurrenceID = uuid.New()
	f.service.ScopePolicy = syntheticScope{
		recipient: f.owner.ID,
		binding:   "different-source-person",
	}
	if count, err := deliver(); err != nil || count != 0 {
		t.Fatalf("scope expanded approved audience: %d %v", count, err)
	}
}

func TestInboxContentTeamReassignmentAndCrossEventSourceIsolation(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	team, otherTeam, resource := uuid.New(), uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{team, otherTeam} {
		f.exec(
			t,
			"INSERT INTO hack_teams(event_id,id,name,state) VALUES($1,$2,'Synthetic team','published')",
			f.eventID,
			id,
		)
	}
	f.exec(
		t,
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
		f.eventID,
		team,
		f.person.ID,
	)
	f.exec(
		t,
		"INSERT INTO content_items(event_id,id,kind,revision,version,state) VALUES($1,$2,'page',1,1,'draft')",
		f.eventID,
		resource,
	)
	document, _ := json.Marshal(
		map[string]any{
			"title":    "PRIVATE-TEAM-TITLE",
			"markdown": "Synthetic team resource",
			"audience": map[string]any{
				"confirmed": false,
				"roles":     []string{},
				"teams":     []uuid.UUID{team},
			},
		},
	)
	f.exec(
		t,
		"INSERT INTO content_versions(event_id,content_id,version,document,actor_id,created_at) VALUES($1,$2,1,$3,$4,now())",
		f.eventID,
		resource,
		document,
		f.owner.ID,
	)
	f.exec(
		t,
		"UPDATE content_items SET state='released',released_version=1,released_at=$3 WHERE event_id=$1 AND id=$2",
		f.eventID,
		resource,
		f.now.Add(-time.Hour),
	)
	record := func(eventID uuid.UUID) error {
		return database.Within(ctx, f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
			return f.service.RecordResourceInTransaction(
				ctx,
				tx,
				eventID,
				f.person.ID,
				Source{Kind: "content", ID: resource, Version: 1},
			)
		})
	}
	if err := record(f.eventID); err != nil {
		t.Fatal(err)
	}
	otherEvent := testdb.Event(t, f.pool, f.owner.ID)
	if err := record(otherEvent); !errors.Is(err, access.ErrNotFound) {
		t.Fatalf("cross-event source accepted: %v", err)
	}
	page, err := f.service.List(ctx, f.person.ID, &f.eventID, nil, 1)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("team inbox: %+v %v", page, err)
	}
	f.exec(
		t,
		"UPDATE hack_team_members SET team_id=$3 WHERE event_id=$1 AND person_id=$2 AND snapshot='published'",
		f.eventID,
		f.person.ID,
		otherTeam,
	)
	read := true
	if _, err = f.service.Open(
		ctx,
		f.person.ID,
		page.Items[0].ID,
		&read,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatalf("reassigned team retained source read: %v", err)
	}
	page, err = f.service.List(ctx, f.person.ID, nil, nil, 1)
	if err != nil || len(page.Items) != 0 || page.Unread != 0 || page.NextCursor != nil {
		t.Fatalf("reassigned team leaked preview/count: %+v %v", page, err)
	}
}
