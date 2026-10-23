package outcomes

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/objectives"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func proposal() Command {
	return Command{
		ID:     uuid.New(),
		Action: "propose",
		Reason: "Submit an organisation idea for review",
		Definition: &Definition{
			Kind:          "pitch",
			Problem:       "PRIVATE original idea",
			Audience:      "People with this problem",
			ProposedValue: "A proposed benefit, not an achieved result",
		},
	}
}

func TestNativeOrganisationMemberPitchOwnershipHistoryArchiveAndRevocation(t *testing.T) {
	pool := testdb.New(t, true)
	organiser := testdb.Person(t, pool)
	author := testdb.Person(t, pool)
	stranger := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, organiser.ID)
	now := time.Now().UTC().Truncate(time.Second)
	s := Service{Pool: pool, Now: func() time.Time { return now }}
	id := uuid.New()
	create := proposal()
	created, err := s.Change(t.Context(), author.ID, id, 0, create)
	if err != nil || created.Version.Version != 1 || created.Provenance.EventID != nil ||
		created.Capability.CanModerate {
		t.Fatal("ordinary no-event-role pitch", created, err)
	}
	if err = (access.EventPolicy{Queries: dbgen.New(pool)}).Authorize(
		t.Context(),
		author.ID,
		eventID,
		eventID,
		access.EventRead,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("fixture unexpectedly grants event access", err)
	}
	if _, err = s.Read(t.Context(), stranger.ID, id, 0); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("private pitch exposed", err)
	}
	own, err := s.List(t.Context(), author.ID, uuid.Nil, "own", nil, 25)
	if err != nil || len(own.Items) != 1 {
		t.Fatal("ordinary own list", own, err)
	}
	if _, err = s.Owners(
		t.Context(),
		author.ID,
		id,
		uuid.Nil,
		25,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("ordinary author enumerated owners", err)
	}
	adopted, err := s.Change(
		t.Context(),
		organiser.ID,
		id,
		1,
		Command{
			ID:         uuid.New(),
			Action:     "adopt",
			Provenance: &Provenance{EventID: &eventID},
			Reason:     "Adopt this submitted idea into an event",
		},
	)
	if err != nil || adopted.Provenance.EventID == nil || *adopted.Provenance.EventID != eventID {
		t.Fatal("deliberate adoption", adopted, err)
	}
	// The DTO contains the association UUID, never an implicit draft-event name or
	// configuration, and the author still has no generic event read grant.
	if _, err = s.Read(t.Context(), author.ID, id, 0); err != nil {
		t.Fatal("own adopted pitch unavailable to ordinary author", err)
	}
	if err = (access.EventPolicy{Queries: dbgen.New(pool)}).Authorize(
		t.Context(),
		author.ID,
		eventID,
		eventID,
		access.EventRead,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("adoption broadened event policy", err)
	}
	candidates, err := s.Owners(t.Context(), organiser.ID, id, uuid.Nil, 25)
	if err != nil || len(candidates.Items) != 2 {
		t.Fatal("scoped owner candidates", candidates, err)
	}
	assigned, err := s.Change(
		t.Context(),
		organiser.ID,
		id,
		2,
		Command{
			ID:      uuid.New(),
			Action:  "assign",
			OwnerID: &author.ID,
			Reason:  "Explicitly delegate follow-through to the ordinary author",
		},
	)
	if err != nil || assigned.OwnerID == nil || *assigned.OwnerID != author.ID {
		t.Fatal("assign ordinary owner", assigned, err)
	}
	measureID, objectiveID := uuid.New(), uuid.New()
	target := "10"
	definition := objectives.Definition{
		Description: "Private planning objective",
		Measures: []objectives.Measure{
			{ID: measureID, Description: "Dated adoption", Target: &target, Unit: "adopting team"},
		},
	}
	objectiveService := objectives.Service{Pool: pool}
	if _, err = objectiveService.Change(
		t.Context(),
		organiser.ID,
		eventID,
		objectiveID,
		0,
		objectives.Command{Action: "save", Definition: &definition, Reason: "Planning intent"},
	); err != nil {
		t.Fatal(err)
	}
	ref := objectives.MeasureReference{
		Objective: objectives.Reference{ID: objectiveID, Version: 1},
		MeasureID: measureID,
	}
	accepted, err := s.Change(
		t.Context(),
		organiser.ID,
		id,
		3,
		Command{
			ID:       uuid.New(),
			Action:   "moderate",
			Status:   "accepted",
			Measures: []objectives.MeasureReference{ref},
			Reason:   "Review and accept an exact measurable follow-up",
		},
	)
	if err != nil || len(accepted.Measures) != 1 {
		t.Fatal("attach exact measure", accepted, err)
	}
	definition.Measures[0].Unit = "a changed planning unit"
	if _, err = objectiveService.Change(
		t.Context(),
		organiser.ID,
		eventID,
		objectiveID,
		1,
		objectives.Command{
			Action:     "save",
			Definition: &definition,
			Reason:     "Later independent planning revision",
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(
		t.Context(),
		"UPDATE events SET stage='completed',completed_at=$2 WHERE id=$1",
		eventID,
		now,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(
		t.Context(),
		"INSERT INTO event_gate_facts(event_id) VALUES($1) ON CONFLICT DO NOTHING",
		eventID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = (event.Service{Pool: pool}).Transition(
		t.Context(),
		organiser.ID,
		eventID,
		1,
		"archived",
		"Archive competition records without losing approved follow-through",
		false,
	); err != nil {
		t.Fatal(err)
	}
	if err = (access.EventPolicy{Queries: dbgen.New(pool)}).Authorize(
		t.Context(),
		organiser.ID,
		eventID,
		eventID,
		access.EventEdit,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("archive event write unexpectedly available", err)
	}
	selfReport := Evidence{
		ID:       uuid.New(),
		Basis:    "self_report",
		Claim:    "The owner reports initial interest",
		Measures: []objectives.MeasureReference{ref},
	}
	evidence, err := s.Change(
		t.Context(),
		author.ID,
		id,
		4,
		Command{
			ID:       uuid.New(),
			Action:   "evidence",
			Evidence: &selfReport,
			Reason:   "Record labelled self-reported follow-through",
		},
	)
	if err != nil || !evidence.Capability.Archived || evidence.EvidenceAddedID == nil ||
		evidence.Measures[0] != ref {
		t.Fatal("dedicated archived owner update", evidence, err)
	}
	evidencePage, err := s.Evidence(t.Context(), author.ID, id, uuid.Nil, 25)
	if err != nil || len(evidencePage.Items) != 1 || evidencePage.Items[0].Value != nil ||
		evidencePage.Items[0].Measures[0] != ref {
		t.Fatal("restricted evidence page", evidencePage, err)
	}
	history, err := s.History(t.Context(), author.ID, id, 0, 3)
	if err != nil || len(history.Items) != 3 || history.NextCursor == nil {
		t.Fatal("own paginated history", history, err)
	}
	replay, err := s.Change(t.Context(), author.ID, id, 0, create)
	if err != nil || replay.Version.Version != 1 || replay.CurrentVersion != 5 {
		t.Fatal("exact original command replay", replay, err)
	}
	closed, err := s.Change(
		t.Context(),
		author.ID,
		id,
		5,
		Command{
			ID:     uuid.New(),
			Action: "progress",
			Status: "closed",
			Reason: "Finish this follow-through",
		},
	)
	if err != nil || closed.Status != "closed" {
		t.Fatal("ordinary owner closure", closed, err)
	}
	if _, err = pool.Exec(
		t.Context(),
		"UPDATE people SET active=false WHERE id=$1",
		author.ID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Change(
		t.Context(),
		author.ID,
		id,
		0,
		create,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("inactive actor replay bypass", err)
	}
	now = now.AddDate(0, 0, 91)
	if _, err = s.Read(t.Context(), organiser.ID, id, 0); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("expired restricted history remained visible", err)
	}
}

func TestNativeOutcomeConcurrentCommandsAndExactRetainedReplay(t *testing.T) {
	pool := testdb.New(t, true)
	person := testdb.Person(t, pool)
	s := Service{Pool: pool}
	id := uuid.New()
	create := proposal()
	if _, err := s.Change(t.Context(), person.ID, id, 0, create); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			next := proposal()
			next.Action = "revise"
			_, err := s.Change(t.Context(), person.ID, id, 1, next)
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	wins, conflicts := 0, 0
	for err := range results {
		if err == nil {
			wins++
		} else if errors.Is(err, access.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("concurrent revisions: winners%d conflicts%d", wins, conflicts)
	}
	changed := create
	changed.Reason = "Different intent reusing an old command"
	if _, err := s.Change(
		t.Context(),
		person.ID,
		id,
		0,
		changed,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("idempotency key accepted a different request", err)
	}
	replay, err := s.Change(t.Context(), person.ID, id, 0, create)
	if err != nil || replay.Version.Version != 1 || replay.CurrentVersion != 2 {
		t.Fatal("retained receipt retargeted", replay, err)
	}
}

func TestNativePolicyRevisionTracksActualChangesAndPreservesNoop(t *testing.T) {
	pool := testdb.New(t, true)
	actor := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, actor.ID)
	q := dbgen.New(pool)
	service := privacy.Service{Pool: pool}
	cases := []struct {
		retention, draft int32
		revision         int64
	}{{90, 7, 0}, {90, 7, 0}, {60, 7, 1}, {60, 7, 1}, {90, 7, 2}, {90, 8, 3}, {90, 7, 4}}
	for _, item := range cases {
		current, err := q.Event(t.Context(), eventID)
		if err != nil {
			t.Fatal(err)
		}
		if err = service.Configure(
			t.Context(),
			actor.ID,
			eventID,
			current.Revision,
			privacy.Settings{RetentionDays: item.retention, DraftDays: item.draft},
		); err != nil {
			t.Fatal(err)
		}
		revision, err := q.OutcomePrivacyRevision(t.Context(), eventID)
		if err != nil || revision != item.revision {
			t.Fatalf(
				"policy %d/%d: revision%d wanted%d err%v",
				item.retention,
				item.draft,
				revision,
				item.revision,
				err,
			)
		}
	}
}

func TestNativeTeamOutputRequiresCommonPublicationLiveClockAndCurrentTeam(t *testing.T) {
	pool := testdb.New(t, true)
	organiser := testdb.Person(t, pool)
	member := testdb.Person(t, pool)
	other := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, organiser.ID)
	teamID := uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(t.Context(), sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, person := range []uuid.UUID{member.ID, other.ID} {
		exec(
			"INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')",
			eventID,
			person,
		)
	}
	exec(
		"INSERT INTO hack_teams(event_id,id,name,state)VALUES($1,$2,'Synthetic outcome team','published')",
		eventID,
		teamID,
	)
	exec(
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot)VALUES($1,$2,$3,'published')",
		eventID,
		teamID,
		member.ID,
	)
	exec("INSERT INTO competition_entries(event_id,team_id)VALUES($1,$2)", eventID, teamID)
	now := time.Now().UTC().Truncate(time.Second)
	live := now.Add(time.Hour)
	config := event.DefaultConfiguration()
	config.LiveStart = live
	config.LiveEnd = live.Add(8 * time.Hour)
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	exec(
		"UPDATE events SET configuration=$2,published_at=$3 WHERE id=$1",
		eventID,
		raw,
		now.Add(-7*24*time.Hour),
	)
	service := Service{Pool: pool, Now: func() time.Time { return now }}
	command := proposal()
	command.Definition.Kind = "team_outcome"
	command.Provenance = &Provenance{EventID: &eventID, TeamID: &teamID}
	id := uuid.New()
	if _, err = service.Change(
		t.Context(),
		member.ID,
		id,
		0,
		command,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("prelive competition-output capture", err)
	}
	now = live.Add(time.Minute)
	exec("UPDATE events SET published_at=NULL WHERE id=$1", eventID)
	if _, err = service.Change(
		t.Context(),
		member.ID,
		id,
		0,
		command,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("clock without actual common publication", err)
	}
	exec("UPDATE events SET published_at=$2 WHERE id=$1", eventID, live.Add(-7*24*time.Hour))
	if _, err = service.Change(t.Context(), member.ID, id, 0, command); err != nil {
		t.Fatal("valid own-team follow-through", err)
	}
	if _, err = service.Read(t.Context(), other.ID, id, 0); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("another attendee read restricted team outcome", err)
	}
	// Changed live timing is rechecked on reads, not only at initial creation.
	config.LiveStart = now.Add(time.Hour)
	config.LiveEnd = config.LiveStart.Add(8 * time.Hour)
	raw, err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	exec("UPDATE events SET configuration=$2 WHERE id=$1", eventID, raw)
	if _, err = service.Read(
		t.Context(),
		organiser.ID,
		id,
		0,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("rescheduled prelive organiser review", err)
	}
	now = config.LiveStart.Add(time.Minute)
	if _, err = service.Read(t.Context(), member.ID, id, 0); err != nil {
		t.Fatal(err)
	}
	exec("DELETE FROM hack_team_members WHERE event_id=$1 AND person_id=$2", eventID, member.ID)
	if _, err = service.Read(t.Context(), member.ID, id, 0); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("departed team member retained private output access", err)
	}
}
