package help

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
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
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

// The schema has no migration number yet. These fixtures exercise the isolated
// domain draft; final ordered Goose-upgrade evidence is required before its PR.
//
//go:embed schema.provisional.sql
var provisionalSchema string

type delivery struct {
	eventID, requestID uuid.UUID
	revision           int64
	people             []uuid.UUID
}
type sink struct {
	mu         sync.Mutex
	deliveries []delivery
}

func (s *sink) RecordHelpInTransaction(
	_ context.Context,
	_ pgx.Tx,
	eventID, requestID uuid.UUID,
	revision int64,
	people []uuid.UUID,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deliveries = append(s.deliveries, delivery{eventID, requestID, revision, people})
	return nil
}

type fixture struct {
	t                                               *testing.T
	pool                                            *pgxpool.Pool
	s                                               Service
	notifications                                   *sink
	now, live                                       time.Time
	eventID, team, otherTeam                        uuid.UUID
	owner, member, other, helper, second, logistics dbgen.Person
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := testdb.New(t, true)
	if _, err := pool.Exec(t.Context(), provisionalSchema); err != nil {
		t.Fatal(err)
	}
	f := &fixture{
		t:             t,
		pool:          pool,
		now:           time.Date(2027, 6, 20, 10, 0, 0, 0, time.UTC),
		team:          uuid.New(),
		otherTeam:     uuid.New(),
		notifications: &sink{},
	}
	f.live = f.now.Add(-time.Hour)
	f.owner = testdb.Person(t, pool)
	f.member = testdb.Person(t, pool)
	f.other = testdb.Person(t, pool)
	f.helper = testdb.Person(t, pool)
	f.second = testdb.Person(t, pool)
	f.logistics = testdb.Person(t, pool)
	f.eventID = testdb.Event(t, pool, f.owner.ID)
	cfg := event.DefaultConfiguration()
	cfg.LiveStart = f.live
	cfg.LiveEnd = f.live.Add(8 * time.Hour)
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(
		"UPDATE events SET stage='live',configuration=$2,published_at=$3 WHERE id=$1",
		f.eventID,
		data,
		f.live.Add(-7*24*time.Hour),
	)
	for _, p := range []dbgen.Person{f.member, f.other} {
		f.exec(
			"INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')",
			f.eventID,
			p.ID,
		)
	}
	for _, id := range []uuid.UUID{f.team, f.otherTeam} {
		f.exec(
			"INSERT INTO hack_teams(event_id,id,name,state) VALUES($1,$2,'Synthetic help team','published')",
			f.eventID,
			id,
		)
	}
	f.exec(
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot)VALUES($1,$2,$3,'published'),($1,$4,$5,'published')",
		f.eventID,
		f.team,
		f.member.ID,
		f.otherTeam,
		f.other.ID,
	)
	for _, p := range []dbgen.Person{f.helper, f.second} {
		f.exec(
			"INSERT INTO event_memberships(event_id,person_id,role,active)VALUES($1,$2,'mentor',true)",
			f.eventID,
			p.ID,
		)
	}
	f.exec(
		"INSERT INTO event_memberships(event_id,person_id,role,active)VALUES($1,$2,'logistics',true)",
		f.eventID,
		f.logistics.ID,
	)
	f.exec(
		"INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at)VALUES($1,$2,$3,'primary','published',$5,$6),($1,$2,$4,'second','published',$5,$6)",
		f.eventID,
		f.team,
		f.helper.ID,
		f.second.ID,
		f.live,
		f.live.Add(8*time.Hour),
	)
	f.s = Service{
		Pool:          pool,
		Clock:         func() time.Time { return f.now },
		Notifications: f.notifications,
	}
	return f
}

func (f *fixture) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.t.Context(), query, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) create(category string) Page {
	f.t.Helper()
	p, err := f.s.Create(
		f.t.Context(),
		f.member.ID,
		f.eventID,
		uuid.New(),
		Input{
			TeamID:      f.team,
			Category:    category,
			Description: "SYNTHETIC-PRIVATE-PROBLEM",
			Location:    "SYNTHETIC-PRIVATE-LOCATION",
			CommandID:   uuid.New(),
		},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	return p
}

func (f *fixture) change(actor uuid.UUID, p Page, action string) Page {
	f.t.Helper()
	next, err := f.s.Change(
		f.t.Context(),
		actor,
		f.eventID,
		p.Request.ID,
		p.Request.Revision,
		Command{Action: action, CommandID: uuid.New()},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	return next
}

func TestHelpConcurrentClaimIdempotencyAndCurrentAuthority(t *testing.T) {
	f := newFixture(t)
	p := f.create("competition_output")
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, actor := range []uuid.UUID{f.helper.ID, f.second.ID} {
		wg.Go(func() {
			_, err := f.s.Change(
				t.Context(),
				actor,
				f.eventID,
				p.Request.ID,
				p.Request.Revision,
				Command{Action: "claim", CommandID: uuid.New()},
			)
			results <- err
		})
	}
	wg.Wait()
	close(results)
	won, lost := 0, 0
	for err := range results {
		if err == nil {
			won++
		} else if errors.Is(err, access.ErrConflict) {
			lost++
		} else {
			t.Fatal(err)
		}
	}
	if won != 1 || lost != 1 {
		t.Fatalf("claims won=%d conflicted=%d", won, lost)
	}
	current, err := f.s.Read(t.Context(), f.owner.ID, f.eventID, p.Request.ID)
	if err != nil {
		t.Fatal(err)
	}
	owner := *current.Request.AssigneeID
	cmd := Command{Action: "resolve", CommandID: uuid.New()}
	resolved, err := f.s.Change(
		t.Context(),
		owner,
		f.eventID,
		p.Request.ID,
		current.Request.Revision,
		cmd,
	)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := f.s.Change(
		t.Context(),
		owner,
		f.eventID,
		p.Request.ID,
		current.Request.Revision,
		cmd,
	)
	if err != nil || retry.Request.Revision != resolved.Request.Revision ||
		len(retry.History) != 3 {
		t.Fatalf("retry duplicated transition: %+v %v", retry, err)
	}
	if _, err = f.s.Read(
		t.Context(),
		f.other.ID,
		f.eventID,
		p.Request.ID,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatalf("other team read private request: %v", err)
	}
	if _, err = f.s.Read(
		t.Context(),
		f.member.ID,
		testdb.Event(t, f.pool, f.owner.ID),
		p.Request.ID,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("cross-event read")
	}
	f.exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		f.eventID,
		f.member.ID,
	)
	if _, err = f.s.Read(
		t.Context(),
		f.member.ID,
		f.eventID,
		p.Request.ID,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("withdrawn requester retained private access")
	}
	for _, d := range f.notifications.deliveries {
		if d.eventID != f.eventID || d.requestID != p.Request.ID || d.revision < 1 {
			t.Fatal("unsafe notification source")
		}
		for _, id := range d.people {
			if id == f.other.ID || id == f.logistics.ID {
				t.Fatal("wrong recipient for competition help")
			}
		}
	}
}

func TestHelpPreLiveLogisticsAndActualOutputReleaseGate(t *testing.T) {
	f := newFixture(t)
	f.now = f.live.Add(-time.Hour)
	f.exec("UPDATE events SET stage='preparation',published_at=NULL WHERE id=$1", f.eventID)
	p := f.create("logistics")
	p = f.change(f.logistics.ID, p, "claim")
	p = f.change(f.logistics.ID, p, "resolve")
	if p.Request.Status != "resolved" {
		t.Fatal("pre-live logistics unavailable")
	}
	_, err := f.s.Create(
		t.Context(),
		f.member.ID,
		f.eventID,
		uuid.New(),
		Input{
			TeamID:      f.team,
			Category:    "competition_output",
			Description: "Output",
			Location:    "Desk",
			CommandID:   uuid.New(),
		},
	)
	if !errors.Is(err, event.ErrBlocked) {
		t.Fatalf("pre-live output accepted: %v", err)
	}
	f.now = f.live
	_, err = f.s.Create(
		t.Context(),
		f.member.ID,
		f.eventID,
		uuid.New(),
		Input{
			TeamID:      f.team,
			Category:    "competition_output",
			Description: "Output",
			Location:    "Desk",
			CommandID:   uuid.New(),
		},
	)
	if !errors.Is(err, event.ErrBlocked) {
		t.Fatal("scheduled instant invented actual publication")
	}
	f.exec("UPDATE events SET published_at=$2 WHERE id=$1", f.eventID, f.live.Add(-7*24*time.Hour))
	output := f.create("competition_output")
	f.now = f.live.Add(-time.Minute)
	if _, err = f.s.Read(
		t.Context(),
		f.owner.ID,
		f.eventID,
		output.Request.ID,
	); !errors.Is(
		err,
		event.ErrBlocked,
	) {
		t.Fatal("pre-live output body exposed")
	}
	if _, err = f.s.Change(
		t.Context(),
		f.owner.ID,
		f.eventID,
		output.Request.ID,
		output.Request.Revision,
		Command{Action: "claim", CommandID: uuid.New()},
	); !errors.Is(
		err,
		event.ErrBlocked,
	) {
		t.Fatal("pre-live output transition allowed")
	}
}

func TestHelpDepartureConflictsTransferAndImmutableHistory(t *testing.T) {
	f := newFixture(t)
	p := f.change(f.helper.ID, f.create("competition_output"), "claim")
	f.exec(
		"INSERT INTO mentor_conflicts(event_id,mentor_id,competitor_id,kind)VALUES($1,$2,$3,'reporting')",
		f.eventID,
		f.helper.ID,
		f.member.ID,
	)
	if _, err := f.s.Read(
		t.Context(),
		f.helper.ID,
		f.eventID,
		p.Request.ID,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("conflicted helper retained private context")
	}
	current, err := f.s.Read(t.Context(), f.second.ID, f.eventID, p.Request.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Request.Status != "waiting" || current.Request.AssigneeID != nil ||
		current.Request.WaitingCycle != 2 ||
		current.History[len(current.History)-1].Action != "assignee_unavailable" {
		t.Fatalf("conflicted owner stranded request: %+v", current)
	}
	p = f.change(f.second.ID, current, "claim")
	transfer := Command{Action: "transfer", AssigneeID: &f.helper.ID, CommandID: uuid.New()}
	if _, err = f.s.Change(
		t.Context(),
		f.second.ID,
		f.eventID,
		p.Request.ID,
		p.Request.Revision,
		transfer,
	); !errors.Is(
		err,
		access.ErrInvalid,
	) {
		t.Fatal("transfer to conflicted helper accepted")
	}
	f.exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mentor'",
		f.eventID,
		f.second.ID,
	)
	recovered, err := f.s.Read(t.Context(), f.owner.ID, f.eventID, p.Request.ID)
	if err != nil || recovered.Request.Status != "waiting" {
		t.Fatal("departed helper stranded ownership")
	}
	if _, err = f.pool.Exec(
		t.Context(),
		"UPDATE help_transitions SET action='cancel' WHERE event_id=$1",
		f.eventID,
	); err == nil {
		t.Fatal("history was mutable")
	}
	f.exec("UPDATE events SET writes_allowed=false WHERE id=$1", f.eventID)
	if _, err = f.s.Read(t.Context(), f.member.ID, f.eventID, p.Request.ID); err != nil {
		t.Fatalf("archive erased readable operational history: %v", err)
	}
	if _, err = f.s.Change(
		t.Context(),
		f.member.ID,
		f.eventID,
		p.Request.ID,
		recovered.Request.Revision,
		Command{Action: "cancel", CommandID: uuid.New()},
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("archive accepted mutation")
	}
}

func TestHelpQueueCountsAndDueFollowupAreScopedAndDeduplicated(t *testing.T) {
	f := newFixture(t)
	p := f.create("logistics")
	queue, err := f.s.List(t.Context(), f.other.ID, f.eventID)
	if err != nil || len(queue.Items) != 0 || queue.Waiting != 0 || queue.Claimed != 0 {
		t.Fatalf("hidden queue counts exposed: %+v %v", queue, err)
	}
	f.now = f.now.Add(16 * time.Minute)
	for range 2 {
		if err = database.Within(
			t.Context(),
			f.pool,
			func(tx pgx.Tx, _ *dbgen.Queries) error { return f.s.MaintainInTransaction(t.Context(), tx, f.eventID) },
		); err != nil {
			t.Fatal(err)
		}
	}
	current, err := f.s.Read(t.Context(), f.member.ID, f.eventID, p.Request.ID)
	if err != nil || current.Request.Revision != 2 || len(current.History) != 2 ||
		current.History[1].Action != "overdue" {
		t.Fatalf("follow-up missing/duplicated: %+v %v", current, err)
	}
	queue, err = f.s.List(t.Context(), f.owner.ID, f.eventID)
	if err != nil || queue.Waiting != 1 || !queue.Items[0].Overdue ||
		queue.Items[0].ResponseSeconds != nil ||
		queue.Items[0].CompletionSeconds != nil {
		t.Fatalf("operational timing invented response/completion: %+v %v", queue, err)
	}
	resolved := f.change(f.logistics.ID, f.change(f.logistics.ID, current, "claim"), "resolve")
	reopened := f.change(f.member.ID, resolved, "reopen")
	f.now = f.now.Add(16 * time.Minute)
	if err = database.Within(
		t.Context(),
		f.pool,
		func(tx pgx.Tx, _ *dbgen.Queries) error { return f.s.MaintainInTransaction(t.Context(), tx, f.eventID) },
	); err != nil {
		t.Fatal(err)
	}
	final, err := f.s.Read(t.Context(), f.member.ID, f.eventID, p.Request.ID)
	if err != nil || final.Request.Revision != reopened.Request.Revision+1 ||
		final.Request.WaitingCycle != 2 {
		t.Fatal("new waiting cycle did not get independent follow-up")
	}
	var count int
	if err = f.pool.QueryRow(t.Context(), "SELECT count(*) FROM help_followups WHERE event_id=$1", f.eventID).
		Scan(&count); err != nil ||
		count != 2 {
		t.Fatalf("follow-up dedup count=%d %v", count, err)
	}
}

func TestNativeHelpFactsAuthorizeCurrentTeamAndHelpersWithoutOwnerFallback(t *testing.T) {
	f := newFixture(t)
	p := f.create("competition_output")
	comm := communications.Service{
		Pool:       f.pool,
		Now:        func() time.Time { return f.now },
		FactOwners: map[string]communications.FactOwner{"help": f.s},
	}
	template := communications.Template{
		ID:      uuid.New(),
		Name:    "Safe help status",
		Purpose: "help",
		Format:  "text",
		Subject: "Help request update",
		Body:    "Help is {{help.status}}, revision {{help.revision}}",
		FactID:  &p.Request.ID,
	}
	audience := communications.Audience{
		ID:                 uuid.New(),
		Name:               "Current help participants",
		Roles:              []string{"attendee", "mentor", "organiser", "logistics"},
		RegistrationStates: []string{},
		Teams:              []uuid.UUID{},
		FactID:             &p.Request.ID,
	}
	change := func(command communications.Command) communications.Page {
		t.Helper()
		command.Reason = "Synthetic native help source verification"
		current, err := comm.Read(t.Context(), f.owner.ID, f.eventID)
		if err != nil {
			t.Fatal(err)
		}
		next, err := comm.Change(t.Context(), f.owner.ID, f.eventID, current.Revision, command)
		if err != nil {
			t.Fatal(err)
		}
		return next
	}
	change(communications.Command{Action: "save_template", Template: &template})
	change(communications.Command{Action: "save_audience", Audience: &audience})
	preview := change(
		communications.Command{Action: "preview", TemplateID: template.ID, AudienceID: audience.ID},
	).Preview
	if preview == nil || len(preview.Recipients) != 4 {
		t.Fatalf("native current help audience: %+v", preview)
	}
	for _, person := range preview.Recipients {
		if person.PersonID == f.other.ID || person.PersonID == f.logistics.ID ||
			strings.Contains(person.Body, "PRIVATE") {
			t.Fatal("wrong recipient or private help prose exposed")
		}
	}
	err := database.Within(t.Context(), f.pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		actual, err := q.CommunicationFacts(t.Context(), f.eventID)
		if err != nil {
			return err
		}
		if len(actual) != 1 {
			t.Fatal("missing actual routing fact")
		}
		fact := actual[0]
		allowed, err := f.s.AuthorizeCommunicationFact(t.Context(), q, f.member.ID, fact)
		if err != nil || !allowed {
			t.Fatalf("current team denied: %v", err)
		}
		fact.Revision++
		if allowed, err = f.s.AuthorizeCommunicationFact(
			t.Context(),
			q,
			f.member.ID,
			fact,
		); err != nil ||
			allowed {
			t.Fatal("stale revision fell back to owner")
		}
		fact = actual[0]
		fact.ID = uuid.New()
		fact.ResourceID = &fact.ID
		fact.OwnerID = &f.member.ID
		if allowed, err = f.s.AuthorizeCommunicationFact(
			t.Context(),
			q,
			f.member.ID,
			fact,
		); err != nil ||
			allowed {
			t.Fatal("unknown native source fell back to owner")
		}
		sources, err := f.s.DiscoverNotificationSourcesInTransaction(t.Context(), tx, f.eventID)
		if err != nil || len(sources) != 1 || sources[0].Revision != p.Request.Revision {
			t.Fatal("invented source transition")
		}
		if _, err = f.s.ResolveNotificationAudienceInTransaction(
			t.Context(),
			tx,
			f.eventID,
			p.Request.ID,
			p.Request.Revision+1,
		); !errors.Is(
			err,
			access.ErrNotFound,
		) {
			t.Fatal("stale automation source accepted")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	f.exec(
		"DELETE FROM mentor_assignments WHERE event_id=$1 AND person_id=$2",
		f.eventID,
		f.helper.ID,
	)
	current := change(
		communications.Command{Action: "preview", TemplateID: template.ID, AudienceID: audience.ID},
	).Preview
	if current == nil || len(current.Recipients) != 3 {
		t.Fatalf("departed assignment retained message access: %+v", current)
	}
}

func TestHelpConfiguredThresholdAndApprovedMeetingRoute(t *testing.T) {
	f := newFixture(t)
	config, err := f.s.Settings(t.Context(), f.owner.ID, f.eventID)
	if err != nil {
		t.Fatal(err)
	}
	config.SetupThresholdSeconds = 60
	config.MeetingHosts = []string{"meet.example.test"}
	if _, err = f.s.Configure(
		t.Context(),
		f.helper.ID,
		f.eventID,
		config.Revision,
		config,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatalf("helper changed organiser thresholds: %v", err)
	}
	saved, err := f.s.Configure(t.Context(), f.owner.ID, f.eventID, config.Revision, config)
	if err != nil || saved.Revision != config.Revision+1 {
		t.Fatalf("configure thresholds: %+v %v", saved, err)
	}
	if _, err = f.s.Configure(
		t.Context(),
		f.owner.ID,
		f.eventID,
		config.Revision,
		config,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("stale settings silently overwrote")
	}
	in := Input{
		TeamID:      f.team,
		Category:    "setup",
		Description: "Synthetic approved meeting help",
		MeetingURL:  "https://meet.example.test/help-room",
		CommandID:   uuid.New(),
	}
	page, err := f.s.Create(t.Context(), f.member.ID, f.eventID, uuid.New(), in)
	if err != nil {
		t.Fatal(err)
	}
	in.CommandID = uuid.New()
	in.MeetingURL = "https://other.example.test/room"
	if _, err = f.s.Create(
		t.Context(),
		f.member.ID,
		f.eventID,
		uuid.New(),
		in,
	); !errors.Is(
		err,
		access.ErrInvalid,
	) {
		t.Fatal("unapproved meeting host persisted")
	}
	maintain := func() {
		t.Helper()
		if err = database.Within(
			t.Context(),
			f.pool,
			func(tx pgx.Tx, _ *dbgen.Queries) error { return f.s.MaintainInTransaction(t.Context(), tx, f.eventID) },
		); err != nil {
			t.Fatal(err)
		}
	}
	f.now = f.now.Add(59 * time.Second)
	maintain()
	before, err := f.s.Read(t.Context(), f.member.ID, f.eventID, page.Request.ID)
	if err != nil || before.Request.Revision != 1 {
		t.Fatal("follow-up fired before configured deadline")
	}
	f.now = f.now.Add(time.Second)
	maintain()
	maintain()
	after, err := f.s.Read(t.Context(), f.member.ID, f.eventID, page.Request.ID)
	if err != nil || after.Request.Revision != 2 {
		t.Fatal("exact deadline follow-up missing or repeated")
	}
	var retained int64
	if err = f.pool.QueryRow(t.Context(), "SELECT threshold_seconds FROM help_followups WHERE event_id=$1 AND request_id=$2", f.eventID, page.Request.ID).
		Scan(&retained); err != nil ||
		retained != 60 {
		t.Fatal("follow-up lost approved threshold provenance")
	}
}

func TestHelpRevocationTransactionWinsAgainstBlockedContextRead(t *testing.T) {
	f := newFixture(t)
	page := f.change(f.helper.ID, f.create("competition_output"), "claim")
	tx, err := f.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }()
	if _, err = dbgen.New(tx).LockEvent(t.Context(), f.eventID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(
		t.Context(),
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mentor'",
		f.eventID,
		f.helper.ID,
	); err != nil {
		t.Fatal(err)
	}
	started, done := make(chan struct{}), make(chan error, 1)
	go func() {
		close(started)
		_, readErr := f.s.Read(t.Context(), f.helper.ID, f.eventID, page.Request.ID)
		done <- readErr
	}()
	<-started
	if err = tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = <-done; !errors.Is(err, access.ErrNotFound) {
		t.Fatalf("read crossing committed helper revocation: %v", err)
	}
	reopened, err := f.s.Read(t.Context(), f.owner.ID, f.eventID, page.Request.ID)
	if err != nil || reopened.Request.Status != "waiting" || reopened.Request.AssigneeID != nil {
		t.Fatal("revoked claim remained stranded")
	}
}

func TestHelpSourceKeysetFiltersRequestedIDBeforeBoundsAndHasNoHistoryCap(t *testing.T) {
	f := newFixture(t)
	// Populate routing/transition metadata in bulk; domain-authorized creation and
	// recipient guards are exercised separately above. No provider is invoked.
	f.exec(
		"INSERT INTO policy_resources(event_id,id,kind) SELECT $1,md5('synthetic-help-'||n)::uuid,'help' FROM generate_series(1,1005) n",
		f.eventID,
	)
	f.exec(
		"INSERT INTO help_requests(event_id,id,team_id,requester_id,revision,category,description,location,meeting_url,status,waiting_cycle,requested_at,waiting_since,updated_at) SELECT event_id,id,$2,$3,1,'setup','PRIVATE-SYNTHETIC-CONTEXT','Synthetic desk','','waiting',1,$4,$4,$4 FROM policy_resources WHERE event_id=$1 AND kind='help'",
		f.eventID,
		f.team,
		f.member.ID,
		f.now,
	)
	f.exec(
		"INSERT INTO help_transitions(event_id,request_id,revision,action,status,occurred_at) SELECT event_id,id,revision,'create',status,updated_at FROM help_requests WHERE event_id=$1",
		f.eventID,
	)
	err := database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		all, err := f.s.DiscoverNotificationSourcesInTransaction(t.Context(), tx, f.eventID)
		if err != nil || len(all) != 1005 {
			t.Fatalf("source discovery permanently capped history: %d %v", len(all), err)
		}
		requested := all[len(all)-1].RequestID
		page, next, err := f.s.DiscoverNotificationSourcesPageInTransaction(
			t.Context(),
			tx,
			f.eventID,
			&requested,
			uuid.Nil,
			1,
		)
		if err != nil || len(page) != 1 || page[0].RequestID != requested || next != nil {
			t.Fatalf("requested source filtered after bound: %+v %v", page, err)
		}
		first, cursor, err := f.s.DiscoverNotificationSourcesPageInTransaction(
			t.Context(),
			tx,
			f.eventID,
			nil,
			uuid.Nil,
			2,
		)
		if err != nil || len(first) != 2 || cursor == nil {
			t.Fatal("source keyset first page")
		}
		second, _, err := f.s.DiscoverNotificationSourcesPageInTransaction(
			t.Context(),
			tx,
			f.eventID,
			nil,
			*cursor,
			2,
		)
		if err != nil || len(second) != 2 || second[0].RequestID == first[1].RequestID {
			t.Fatal("source keyset overlaps or stalls")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	queue, err := f.s.List(t.Context(), f.owner.ID, f.eventID)
	if err != nil || len(queue.Items) != 1005 || queue.Waiting != 1005 {
		t.Fatalf("queue permanently capped requests: %d %v", len(queue.Items), err)
	}
}

func TestHelpAuthorityFailureDoesNotBecomeARevokedGrant(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := f.s.authority(
		ctx,
		dbgen.New(f.pool),
		f.helper.ID,
		Request{EventID: f.eventID, TeamID: f.team, Category: "setup"},
	)
	if !errors.Is(err, context.Canceled) || errors.Is(err, access.ErrNotFound) {
		t.Fatalf("uncertain authority was treated as a revoked grant: %v", err)
	}
}
