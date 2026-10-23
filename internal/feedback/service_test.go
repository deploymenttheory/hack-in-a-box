package feedback

import (
	"context"
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
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/deploymenttheory/hack-in-a-box/internal/observations"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type fixture struct {
	t                                                *testing.T
	pool                                             *pgxpool.Pool
	s                                                Service
	event, team, otherTeam                           uuid.UUID
	owner, mentor, logistics, member, peer, outsider dbgen.Person
	now                                              time.Time
	cfg                                              event.Configuration
}

func setup(t *testing.T) *fixture {
	t.Helper()
	return setupPool(t, testdb.New(t, true))
}

func setupPool(t *testing.T, pool *pgxpool.Pool) *fixture {
	t.Helper()
	f := &fixture{
		t:         t,
		pool:      pool,
		owner:     testdb.Person(t, pool),
		mentor:    testdb.Person(t, pool),
		logistics: testdb.Person(t, pool),
		member:    testdb.Person(t, pool),
		peer:      testdb.Person(t, pool),
		outsider:  testdb.Person(t, pool),
		team:      uuid.New(),
		otherTeam: uuid.New(),
	}
	f.event = testdb.Event(t, pool, f.owner.ID)
	f.cfg = event.DefaultConfiguration()
	f.cfg.Timezone = "UTC"
	f.cfg.LiveStart = time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	f.cfg.LiveEnd = f.cfg.LiveStart.Add(8 * time.Hour)
	f.cfg.SubmissionCutoff = f.cfg.LiveEnd.Add(-time.Hour)
	f.now = f.cfg.LiveStart.Add(time.Hour)
	encoded, _ := json.Marshal(f.cfg)
	f.exec(
		"UPDATE events SET configuration=$2,published_at=$3,stage='live' WHERE id=$1",
		f.event,
		encoded,
		f.cfg.LiveStart.Add(-7*24*time.Hour),
	)
	for _, id := range []uuid.UUID{f.team, f.otherTeam} {
		f.exec(
			"INSERT INTO hack_teams(event_id,id,name,state,frozen) VALUES($1,$2,'Synthetic feedback team','published',true)",
			f.event,
			id,
		)
		f.exec("INSERT INTO competition_entries(event_id,team_id) VALUES($1,$2)", f.event, id)
	}
	for _, pair := range [][2]uuid.UUID{{f.member.ID, f.team}, {f.peer.ID, f.team}, {f.outsider.ID, f.otherTeam}} {
		f.exec(
			"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
			f.event,
			pair[0],
		)
		f.exec(
			"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
			f.event,
			pair[1],
			pair[0],
		)
	}
	f.exec(
		"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mentor'),($1,$3,'logistics')",
		f.event,
		f.mentor.ID,
		f.logistics.ID,
	)
	f.exec(
		"INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at) VALUES($1,$2,$3,'primary','published',$4,$5)",
		f.event,
		f.team,
		f.mentor.ID,
		f.cfg.LiveStart.Add(-2*time.Hour),
		f.cfg.LiveEnd,
	)
	f.s = Service{
		Pool:  pool,
		Clock: func() time.Time { return f.now },
		Sources: map[string]SourcePolicy{
			"help": helpOwner{
				Service: help.Service{Pool: pool, Clock: func() time.Time { return f.now }},
			},
			"observation": observationOwner{
				Service: observations.Service{Pool: pool, Now: func() time.Time { return f.now }},
			},
		},
	}
	return f
}

func (f *fixture) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.t.Context(), query, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) count(query string, args ...any) int {
	f.t.Helper()
	var n int
	if err := f.pool.QueryRow(f.t.Context(), query, args...).Scan(&n); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func (f *fixture) create(category string) (uuid.UUID, Page) {
	f.t.Helper()
	id := uuid.New()
	p, err := f.s.Change(
		f.t.Context(),
		f.mentor.ID,
		f.event,
		id,
		0,
		uuid.New(),
		Command{
			Action: "save",
			Input: &Input{
				TeamID:   f.team,
				Category: category,
				Title:    "Reviewed team guidance",
				Body:     "Hand-written advice selected for this team",
				Sources:  []Source{},
			},
		},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	return id, p
}

func (f *fixture) publish(id uuid.UUID, p Page) Page {
	f.t.Helper()
	for _, action := range []string{"review", "publish"} {
		var err error
		p, err = f.s.Change(
			f.t.Context(),
			f.mentor.ID,
			f.event,
			id,
			p.CurrentVersion,
			uuid.New(),
			Command{Action: action},
		)
		if err != nil {
			f.t.Fatal(action, err)
		}
	}
	return p
}

func (f *fixture) task(id, owner uuid.UUID) (uuid.UUID, ActionView) {
	f.t.Helper()
	taskID := uuid.New()
	p, err := f.s.ChangeAction(
		f.t.Context(),
		f.mentor.ID,
		f.event,
		id,
		taskID,
		0,
		uuid.New(),
		ActionCommand{
			Action: "details",
			Input: &ActionInput{
				Reviewed: true,
				Title:    "Reviewed integration follow-up",
				Body:     "Run the published test scenario",
				OwnerID:  owner,
				DueAt:    f.now.Add(-time.Hour),
			},
		},
	)
	if err != nil {
		f.t.Fatal(err)
	}
	return taskID, p
}

type helpOwner struct{ Service help.Service }

func (o helpOwner) AuthorizeInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
	ref Source,
) error {
	return o.Service.AuthorizeReferenceInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		teamID,
		ref.ID,
		ref.Version,
	)
}

type observationOwner struct{ Service observations.Service }

func (o observationOwner) AuthorizeInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
	ref Source,
) error {
	return o.Service.AuthorizeEvidenceInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		teamID,
		ref.ID,
		ref.Version,
	)
}

func denied(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, access.ErrForbidden) && !errors.Is(err, access.ErrNotFound) {
		t.Fatalf("wanted current denial, got %v", err)
	}
}

func TestDeliberatelyReviewedFeedbackTeamIsolationDraftHistoryAndAcknowledgements(t *testing.T) {
	f := setup(t)
	id, p := f.create("competition_output")
	_, err := f.s.Read(t.Context(), f.member.ID, f.event, id)
	denied(t, err)
	_, err = f.s.Change(
		t.Context(),
		f.mentor.ID,
		f.event,
		id,
		1,
		uuid.New(),
		Command{Action: "publish"},
	)
	if !errors.Is(err, access.ErrConflict) {
		t.Fatal("unreviewed body published", err)
	}
	p = f.publish(id, p)
	member, err := f.s.Read(t.Context(), f.member.ID, f.event, id)
	if err != nil || member.Draft != nil || member.Published == nil || member.CurrentVersion != 3 {
		t.Fatalf("released team projection %+v %v", member, err)
	}
	_, err = f.s.Read(t.Context(), f.outsider.ID, f.event, id)
	denied(t, err)
	_, err = f.s.Read(t.Context(), f.logistics.ID, f.event, id)
	denied(t, err)
	in := payload(*p.Draft)
	in.Body = "UNRELEASED PRIVATE DRAFT"
	p, err = f.s.Change(
		t.Context(),
		f.mentor.ID,
		f.event,
		id,
		3,
		uuid.New(),
		Command{Action: "save", Input: &in},
	)
	if err != nil {
		t.Fatal(err)
	}
	member, err = f.s.Read(t.Context(), f.member.ID, f.event, id)
	if err != nil || member.Published.Body == in.Body || member.CurrentVersion != 3 {
		t.Fatal("draft replaced released body", err)
	}
	hist, err := f.s.History(t.Context(), f.member.ID, f.event, id, 0, 1)
	if err != nil || len(hist.Items) != 1 || hist.Items[0].State != "published" {
		t.Fatal("private review history leaked", err)
	}
	commandID := uuid.New()
	ack, err := f.s.Acknowledge(t.Context(), f.member.ID, f.event, id, 3, commandID)
	if err != nil || len(ack.Acknowledgements) != 1 || ack.Acknowledgements[0].Version != 3 {
		t.Fatal("exact release acknowledgement", err)
	}
	_, err = f.s.Acknowledge(t.Context(), f.member.ID, f.event, id, 3, commandID)
	if err != nil {
		t.Fatal("ack replay", err)
	}
	if f.count("SELECT count(*) FROM feedback_acknowledgements WHERE event_id=$1", f.event) != 1 {
		t.Fatal("duplicate acknowledgement")
	}
	_, err = f.s.Acknowledge(t.Context(), f.mentor.ID, f.event, id, 3, uuid.New())
	denied(t, err)
	f.exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		f.event,
		f.member.ID,
	)
	_, err = f.s.Acknowledge(t.Context(), f.member.ID, f.event, id, 3, commandID)
	denied(t, err)
	if _, err = f.s.Change(
		t.Context(),
		f.mentor.ID,
		f.event,
		id,
		p.CurrentVersion,
		uuid.New(),
		Command{Action: "withdraw"},
	); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.Read(t.Context(), f.peer.ID, f.event, id)
	denied(t, err)
}

func TestReviewedSupportActionsOwnTeamStatusOwnersVersionRaceAndSeparateHistory(t *testing.T) {
	f := setup(t)
	id, p := f.create("setup")
	f.publish(id, p)
	taskID, task := f.task(id, f.member.ID)
	candidates, err := f.s.Candidates(t.Context(), f.mentor.ID, f.event, id)
	if err != nil {
		t.Fatal(err)
	}
	hasMember, hasStaff := false, false
	for _, c := range candidates {
		hasMember = hasMember || c.PersonID == f.member.ID
		hasStaff = hasStaff || c.PersonID == f.logistics.ID
		if c.PersonID == f.outsider.ID {
			t.Fatal("cross-team owner target")
		}
	}
	if !hasMember || !hasStaff {
		t.Fatal("missing native team/staff owner")
	}
	_, err = f.s.Candidates(t.Context(), f.member.ID, f.event, id)
	denied(t, err)
	_, err = f.s.ChangeAction(
		t.Context(),
		f.peer.ID,
		f.event,
		id,
		taskID,
		1,
		uuid.New(),
		ActionCommand{Action: "status", Status: "done"},
	)
	denied(t, err)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.s.ChangeAction(
				t.Context(),
				f.member.ID,
				f.event,
				id,
				taskID,
				1,
				uuid.New(),
				ActionCommand{Action: "status", Status: "done"},
			)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	ok, conflict := 0, 0
	for err := range results {
		if err == nil {
			ok++
		} else if errors.Is(err, access.ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatal("concurrent support mutation lost version", ok, conflict)
	}
	history, err := f.s.ActionHistory(t.Context(), f.member.ID, f.event, id, taskID, 0, 50)
	if err != nil || len(history.Items) != 2 || history.Items[0].Status != "done" ||
		history.Items[1].Version != task.Action.Version || history.Items[0].ChangedAs != "team_owner" || history.Items[0].Change != "status" || history.Items[1].ChangedAs != "helper" {
		t.Fatal("support state history", err)
	}
	if f.count("SELECT count(*) FROM mentor_scorecard_versions WHERE event_id=$1", f.event) != 0 ||
		f.count("SELECT count(*) FROM help_transitions WHERE event_id=$1", f.event) != 0 {
		t.Fatal("operational task altered score/help state")
	}
	f.exec("UPDATE events SET writes_allowed=false,stage='archived' WHERE id=$1", f.event)
	if _, err = f.s.Read(t.Context(), f.member.ID, f.event, id); err != nil {
		t.Fatal("archive lost published read", err)
	}
	_, err = f.s.ChangeAction(
		t.Context(),
		f.member.ID,
		f.event,
		id,
		taskID,
		2,
		uuid.New(),
		ActionCommand{Action: "status", Status: "in_progress"},
	)
	denied(t, err)
}

func TestOverdueThresholdCategoryTargetsDedupNewCycleAndCurrentRole(t *testing.T) {
	f := setup(t)
	cfg, err := f.s.Configure(
		t.Context(),
		f.owner.ID,
		f.event,
		1,
		Settings{SetupSeconds: 60, LogisticsSeconds: 120, OutputSeconds: 180},
	)
	if err != nil {
		t.Fatal(err)
	}
	setupID, p := f.create("setup")
	f.publish(setupID, p)
	setupTask, _ := f.task(setupID, f.logistics.ID)
	outputID, p := f.create("competition_output")
	f.publish(outputID, p)
	f.task(outputID, f.mentor.ID)
	for range 2 {
		if err = f.s.Maintain(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.count(
		"SELECT count(*) FROM feedback_escalations WHERE event_id=$1",
		f.event,
	); n != 3 {
		t.Fatal("category/recipient dedup", n)
	}
	if n := f.count(
		"SELECT count(*) FROM feedback_escalations WHERE event_id=$1 AND recipient_id=$2",
		f.event,
		f.member.ID,
	); n != 0 {
		t.Fatal("team received staff escalation")
	}
	if n := f.count(
		"SELECT count(*) FROM feedback_escalations e JOIN feedback_action_heads a ON a.event_id=e.event_id AND a.id=e.action_id WHERE a.feedback_id=$1 AND recipient_id=$2",
		outputID,
		f.logistics.ID,
	); n != 0 {
		t.Fatal("logistics received output escalation")
	}
	var notice uuid.UUID
	if err = f.pool.QueryRow(t.Context(), "SELECT id FROM feedback_escalations WHERE event_id=$1 AND action_id=$2 AND recipient_id=$3", f.event, setupTask, f.logistics.ID).
		Scan(&notice); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Followup(t.Context(), f.logistics.ID, f.event, notice); err != nil {
		t.Fatal("current category follow-up", err)
	}
	f.exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='logistics'",
		f.event,
		f.logistics.ID,
	)
	_, err = f.s.Followup(t.Context(), f.logistics.ID, f.event, notice)
	denied(t, err)
	if _, err = f.s.Configure(
		t.Context(),
		f.owner.ID,
		f.event,
		cfg.Version,
		Settings{SetupSeconds: 61, LogisticsSeconds: 121, OutputSeconds: 181},
	); err != nil {
		t.Fatal(err)
	}
	if err = f.s.Maintain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if n := f.count(
		"SELECT count(*) FROM feedback_escalations WHERE event_id=$1 AND recipient_id=$2",
		f.event,
		f.logistics.ID,
	); n != 1 {
		t.Fatal("revoked target received new prompt", n)
	}
	history, err := f.s.Actions(t.Context(), f.owner.ID, f.event, setupID, uuid.Nil, 50)
	if err != nil || history.Items[0].OwnerAvailable {
		t.Fatal("departed owner incorrectly available", err)
	}
}

func TestNativeObservationSourceIsExplicitlyReviewedNeverAutomaticallyCopied(t *testing.T) {
	f := setup(t)
	obs := observations.Service{Pool: f.pool, Now: func() time.Time { return f.now }}
	noteID := uuid.New()
	eventRow, err := dbgen.New(f.pool).Event(t.Context(), f.event)
	if err != nil {
		t.Fatal(err)
	}
	_, err = obs.Change(
		t.Context(),
		f.mentor.ID,
		f.event,
		f.team,
		eventRow.Revision,
		observations.Command{
			Action: "observation",
			ID:     noteID,
			Evidence: observations.Evidence{
				Category: "technical",
				Evidence: "PRIVATE RAW OBSERVATION",
				Outcome:  "PRIVATE OUTCOME",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	input := Input{
		TeamID:   f.team,
		Category: "competition_output",
		Title:    "Reviewed public action",
		Body:     "Selected hand-written suggestion only",
		Sources:  []Source{{Kind: "observation", ID: noteID, Version: 1}},
	}
	p, err := f.s.Change(
		t.Context(),
		f.mentor.ID,
		f.event,
		id,
		0,
		uuid.New(),
		Command{Action: "save", Input: &input},
	)
	if err != nil {
		t.Fatal(err)
	}
	p = f.publish(id, p)
	member, err := f.s.Read(t.Context(), f.member.ID, f.event, id)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(member)
	if strings.Contains(string(raw), "PRIVATE") || strings.Contains(string(raw), noteID.String()) ||
		len(member.Published.Sources) != 0 {
		t.Fatal("private source data leaked into team feedback")
	}
	before := f.count("SELECT count(*) FROM mentor_observation_versions WHERE event_id=$1", f.event)
	if before != 1 {
		t.Fatal("reviewed sharing altered observation history")
	}
	f.exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mentor'",
		f.event,
		f.mentor.ID,
	)
	_, err = f.s.Change(
		t.Context(),
		f.mentor.ID,
		f.event,
		id,
		p.CurrentVersion,
		uuid.New(),
		Command{Action: "withdraw"},
	)
	denied(t, err)
	if _, err = f.s.Read(t.Context(), f.member.ID, f.event, id); err != nil {
		t.Fatal("deliberately released feedback requires old author grant", err)
	}
}

func TestOperationalSupportPreliveAndCompetitionOutputCommonPublicationGate(t *testing.T) {
	f := setup(t)
	f.now = f.cfg.LiveStart.Add(-time.Hour)
	f.exec("UPDATE events SET published_at=NULL WHERE id=$1", f.event)
	id, p := f.create("setup")
	f.publish(id, p)
	if _, err := f.s.Read(t.Context(), f.member.ID, f.event, id); err != nil {
		t.Fatal(err)
	}
	_, err := f.s.Change(
		t.Context(),
		f.mentor.ID,
		f.event,
		uuid.New(),
		0,
		uuid.New(),
		Command{
			Action: "save",
			Input: &Input{
				TeamID:   f.team,
				Category: "competition_output",
				Title:    "Not live",
				Body:     "Cannot assess output",
			},
		},
	)
	denied(t, err)
	if f.count("SELECT count(*) FROM mentor_observation_versions WHERE event_id=$1", f.event) != 0 {
		t.Fatal("support became formal pre-live review")
	}
}

func TestOriginatingHelpLinkNeverCopiesPrivateRequestOrConflatesCompletion(t *testing.T) {
	f := setup(t)
	native := help.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	requestID := uuid.New()
	request, err := native.Create(
		t.Context(),
		f.member.ID,
		f.event,
		requestID,
		help.Input{
			TeamID:      f.team,
			Category:    "setup",
			Description: "PRIVATE REQUEST ORIGINAL TEXT",
			Location:    "PRIVATE DESK LOCATION",
			CommandID:   uuid.New(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	p, err := f.s.Change(
		t.Context(),
		f.mentor.ID,
		f.event,
		id,
		0,
		uuid.New(),
		Command{
			Action: "save",
			Input: &Input{
				TeamID:   f.team,
				Category: "setup",
				Title:    "Reviewed next step",
				Body:     "Collect the replacement cable",
				HelpID:   &requestID,
				Sources: []Source{
					{Kind: "help", ID: requestID, Version: request.Request.Revision},
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	f.publish(id, p)
	taskID, _ := f.task(id, f.member.ID)
	for _, action := range []string{"claim", "resolve"} {
		request, err = native.Change(
			t.Context(),
			f.owner.ID,
			f.event,
			requestID,
			request.Request.Revision,
			help.Command{Action: action, CommandID: uuid.New()},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	actions, err := f.s.Actions(t.Context(), f.member.ID, f.event, id, uuid.Nil, 50)
	if err != nil || actions.Items[0].Action.Status != "open" {
		t.Fatal("closing help completed support action", err)
	}
	if _, err = f.s.ChangeAction(
		t.Context(),
		f.member.ID,
		f.event,
		id,
		taskID,
		1,
		uuid.New(),
		ActionCommand{Action: "status", Status: "done"},
	); err != nil {
		t.Fatal(err)
	}
	current, err := native.Read(t.Context(), f.member.ID, f.event, requestID)
	if err != nil || current.Request.Status != "resolved" || len(current.History) != 3 {
		t.Fatal("action completion altered native help history", err)
	}
	page, err := f.s.Read(t.Context(), f.member.ID, f.event, id)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(page)
	if strings.Contains(string(raw), "PRIVATE") {
		t.Fatal("request text/location automatically copied")
	}
	if page.Published.HelpID == nil || *page.Published.HelpID != requestID {
		t.Fatal("authorised originating link missing")
	}
	if f.count("SELECT count(*) FROM mentor_scorecard_versions WHERE event_id=$1", f.event) != 0 ||
		f.count(
			"SELECT count(*) FROM mentor_observation_versions WHERE event_id=$1",
			f.event,
		) != 0 {
		t.Fatal("help/support altered assessment records")
	}
}

func TestFeedbackAndActionCommandsReplayExactResultsOnlyUnderCurrentAuthority(t *testing.T) {
	f := setup(t)
	id := uuid.New()
	commandID := uuid.New()
	in := Command{
		Action: "save",
		Input: &Input{
			TeamID:   f.team,
			Category: "setup",
			Title:    "Reviewed guidance",
			Body:     "Explicit own-team guidance",
		},
	}
	p, err := f.s.Change(t.Context(), f.mentor.ID, f.event, id, 0, commandID, in)
	if err != nil {
		t.Fatal(err)
	}
	p = f.publish(id, p)
	replay, err := f.s.Change(t.Context(), f.mentor.ID, f.event, id, 0, commandID, in)
	if err != nil || replay.CommandVersion == nil || *replay.CommandVersion != 1 ||
		replay.CurrentVersion != 3 {
		t.Fatal("replay confused retained command result with latest version", err)
	}
	changed := *in.Input
	changed.Body = "Different content"
	_, err = f.s.Change(
		t.Context(),
		f.mentor.ID,
		f.event,
		id,
		0,
		commandID,
		Command{Action: "save", Input: &changed},
	)
	if !errors.Is(err, access.ErrConflict) {
		t.Fatal("same command id accepted different content", err)
	}
	f.exec("UPDATE people SET active=false WHERE id=$1", f.mentor.ID)
	_, err = f.s.Change(t.Context(), f.mentor.ID, f.event, id, 0, commandID, in)
	denied(t, err)
	if f.count("SELECT count(*) FROM feedback_versions WHERE event_id=$1", f.event) != 3 {
		t.Fatal("replay appended another version")
	}
}

func TestFeedbackWriterWaitingBehindNativeRoleRemovalRechecksCurrentAuthority(t *testing.T) {
	f := setup(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	q := dbgen.New(tx)
	if _, err = q.LockEvent(ctx, f.event); err != nil {
		t.Fatal(err)
	}
	var holder int32
	if err = tx.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&holder); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(
		ctx,
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mentor'",
		f.event,
		f.mentor.ID,
	); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := f.s.Change(
			ctx,
			f.mentor.ID,
			f.event,
			uuid.New(),
			0,
			uuid.New(),
			Command{
				Action: "save",
				Input: &Input{
					TeamID:   f.team,
					Category: "competition_output",
					Title:    "Cannot publish after removal",
					Body:     "Blocked current output advice",
				},
			},
		)
		done <- err
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err = f.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))", holder).
			Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err := <-done:
			t.Fatal("writer bypassed native event lock", err)
		case <-ctx.Done():
			t.Fatal("writer never reached native lock")
		case <-ticker.C:
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		denied(t, err)
	case <-ctx.Done():
		t.Fatal("blocked writer failed to finish")
	}
	if f.count("SELECT count(*) FROM feedback_versions WHERE event_id=$1", f.event) != 0 {
		t.Fatal("removed mentor wrote support feedback")
	}
}
