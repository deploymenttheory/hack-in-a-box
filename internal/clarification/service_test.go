package clarification_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type fixture struct {
	t                           *testing.T
	pool                        *pgxpool.Pool
	eventID, owner, first, peer uuid.UUID
	other, team, otherTeam      uuid.UUID
	now, due                    time.Time
	service                     clarification.Service
	content                     content.Service
	challenge                   content.Page
}

func setup(t *testing.T) *fixture {
	t.Helper()
	return setupPool(t, testdb.New(t, true))
}

func setupPool(t *testing.T, pool *pgxpool.Pool) *fixture {
	t.Helper()
	f := &fixture{t: t, pool: pool}
	f.owner = testdb.Person(t, f.pool).ID
	f.first = testdb.Person(t, f.pool).ID
	f.peer = testdb.Person(t, f.pool).ID
	f.other = testdb.Person(t, f.pool).ID
	f.eventID = testdb.Event(t, f.pool, f.owner)
	c := event.DefaultConfiguration()
	c.Timezone = "Europe/London"
	c.MinimumAttendance = 2
	zone, _ := time.LoadLocation(c.Timezone)
	c.LiveStart = time.Date(2027, 3, 29, 9, 0, 0, 0, zone)
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	c.SubmissionCutoff = c.LiveEnd.Add(-time.Hour)
	f.due, _ = schedule.ChallengePublication(c.LiveStart, c.Timezone, c.DSTChoice)
	c.TeamFreeze = f.due.Add(-time.Hour)
	c.AdmissionCutoff = c.TeamFreeze.Add(-time.Hour)
	f.now = f.due.Add(-time.Minute)
	encoded, _ := json.Marshal(c)
	f.exec(
		"UPDATE events SET stage='preparation',configuration=$2,team_frozen_at=$3 WHERE id=$1",
		f.eventID,
		encoded,
		c.TeamFreeze,
	)
	f.team, f.otherTeam = uuid.New(), uuid.New()
	for _, team := range []uuid.UUID{f.team, f.otherTeam} {
		f.exec(
			"INSERT INTO hack_teams(event_id,id,name,state,frozen) VALUES($1,$2,'Synthetic question team','published',true)",
			f.eventID,
			team,
		)
		f.exec("INSERT INTO competition_entries(event_id,team_id) VALUES($1,$2)", f.eventID, team)
	}
	for _, pair := range [][2]uuid.UUID{{f.first, f.team}, {f.peer, f.team}, {f.other, f.otherTeam}} {
		f.exec(
			"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
			f.eventID,
			pair[0],
		)
		f.exec(
			"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
			f.eventID,
			pair[1],
			pair[0],
		)
	}
	f.exec(
		"INSERT INTO event_gate_facts(event_id,attendance_ready,teams_ready,total_teams,frozen_teams,assigned_attendees) VALUES($1,true,true,2,2,3)",
		f.eventID,
	)
	f.service = clarification.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	f.content = content.Service{Pool: f.pool, Clock: func() time.Time { return f.now }}
	ref := testdb.ApprovedRubric(t, f.pool, f.owner, f.eventID)
	var err error
	f.challenge, err = f.content.Save(t.Context(), f.owner, f.eventID, uuid.New(), 0, content.Input{
		Kind: "challenge",
		Document: content.Document{
			Title:    "Synthetic common challenge",
			Markdown: "Identical published challenge.",
			Audience: access.ContentAudience{Confirmed: true},
			Rubric:   &ref,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"review", "approve"} {
		f.challenge, err = f.content.Action(
			t.Context(),
			f.owner,
			f.eventID,
			f.challenge.ID,
			f.challenge.Revision,
			action,
			"Synthetic reviewed release",
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.t.Context(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) release() {
	f.t.Helper()
	f.now = f.due
	if err := f.content.Dispatch(
		f.t.Context(),
		f.eventID,
		f.challenge.ID,
		f.challenge.Revision,
	); err != nil {
		f.t.Fatal(err)
	}
}

func TestPrivateQuestionRechecksActualPublicationCurrentTeamAndStaff(t *testing.T) {
	f := setup(t)
	input := clarification.QuestionInput{
		TeamID:    f.team,
		CommandID: uuid.New(),
		Topic:     "rubric",
		Body:      "SYNTHETIC-PRIVATE-QUESTION How should the published anchor be interpreted?",
	}
	if q, err := f.service.Ask(
		t.Context(),
		f.first,
		f.eventID,
		input,
	); !errors.Is(err, access.ErrNotFound) || q.ID != uuid.Nil ||
		q.Body != "" {
		t.Fatalf("question available before actual common publication: %+v, %v", q, err)
	}
	f.release()
	q, err := f.service.Ask(t.Context(), f.first, f.eventID, input)
	if err != nil || q.Source.ChallengeID != f.challenge.ID ||
		q.Source.ChallengeVersion != f.challenge.Version ||
		q.Source.Rubric.Version < 1 {
		t.Fatalf("question did not bind actual owning source: %+v, %v", q, err)
	}
	again, err := f.service.Ask(t.Context(), f.first, f.eventID, input)
	if err != nil || again.ID != q.ID {
		t.Fatal("uncertain request retry duplicated private question", err)
	}
	changed := input
	changed.Body = "Different private request under the same command"
	if _, err = f.service.Ask(
		t.Context(),
		f.first,
		f.eventID,
		changed,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("command reuse substituted private content", err)
	}
	for _, actor := range []uuid.UUID{f.peer, f.owner} {
		if got, readErr := f.service.Question(
			t.Context(),
			actor,
			f.eventID,
			q.ID,
		); readErr != nil ||
			got.Body != input.Body {
			t.Fatal("current teammate or explicit staff lost private access", readErr)
		}
	}
	denied := func(actor, eventID uuid.UUID) {
		t.Helper()
		got, readErr := f.service.Question(t.Context(), actor, eventID, q.ID)
		if readErr == nil || got.ID != uuid.Nil || got.Body != "" || len(got.Attachments) > 0 {
			t.Fatalf("unauthorised private projection: %+v, %v", got, readErr)
		}
	}
	denied(f.other, f.eventID)
	secondInput := input
	secondInput.CommandID = uuid.New()
	secondInput.Body = "SYNTHETIC-PRIVATE-SECOND-QUESTION"
	secondQuestion, err := f.service.Ask(t.Context(), f.first, f.eventID, secondInput)
	if err != nil {
		t.Fatal(err)
	}
	firstPage, err := f.service.Questions(t.Context(), f.first, f.eventID, nil, uuid.Nil, 1)
	if err != nil || len(firstPage.Items) != 1 || firstPage.NextCursor == nil {
		t.Fatal("private question keyset page was not bounded", firstPage, err)
	}
	secondPage, err := f.service.Questions(
		t.Context(),
		f.first,
		f.eventID,
		nil,
		*firstPage.NextCursor,
		1,
	)
	if err != nil || len(secondPage.Items) != 1 || secondPage.NextCursor != nil ||
		firstPage.Items[0].ID == secondPage.Items[0].ID {
		t.Fatal("private question keyset repeated or omitted a row", secondPage, err)
	}
	seen := map[uuid.UUID]bool{firstPage.Items[0].ID: true, secondPage.Items[0].ID: true}
	if !seen[q.ID] || !seen[secondQuestion.ID] {
		t.Fatal("private pagination substituted another team's question")
	}
	otherQuestions, err := f.service.Questions(t.Context(), f.other, f.eventID, nil, uuid.Nil, 1)
	if err != nil || len(otherQuestions.Items) != 0 || otherQuestions.NextCursor != nil {
		t.Fatal("other team saw private questions or hidden pagination counts", otherQuestions, err)
	}
	if _, err = f.service.Questions(
		t.Context(),
		f.other,
		f.eventID,
		&f.team,
		uuid.Nil,
		1,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("caller-supplied team filter bypassed current membership", err)
	}
	triaged, err := f.service.Triage(t.Context(), f.owner, f.eventID, q.ID, q.Revision, "triage")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Triage(
		t.Context(),
		f.owner,
		f.eventID,
		q.ID,
		triaged.Revision,
		"close",
	); err != nil {
		t.Fatal(err)
	}
	history, err := f.service.QuestionHistory(t.Context(), f.peer, f.eventID, q.ID, uuid.Nil, 1)
	if err != nil || len(history.Items) != 1 || history.NextCursor == nil {
		t.Fatal("private action history did not expose a bounded current-team page", history, err)
	}
	actions := map[string]bool{history.Items[0].Action: true}
	for history.NextCursor != nil {
		history, err = f.service.QuestionHistory(
			t.Context(),
			f.peer,
			f.eventID,
			q.ID,
			*history.NextCursor,
			1,
		)
		if err != nil || len(history.Items) != 1 {
			t.Fatal("private history continuation failed", history, err)
		}
		actions[history.Items[0].Action] = true
	}
	if len(actions) != 3 || !actions["asked"] || !actions["triaged"] || !actions["closed"] {
		t.Fatal("private history lost a distinct audited action", actions)
	}
	otherEvent := testdb.Event(t, f.pool, f.owner)
	denied(f.owner, otherEvent)
	f.exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		f.eventID,
		f.first,
	)
	denied(f.first, f.eventID)
	f.exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='organiser'",
		f.eventID,
		f.owner,
	)
	denied(f.owner, f.eventID)
	f.exec("UPDATE events SET writes_allowed=false WHERE id=$1", f.eventID)
	if _, err = f.service.Question(t.Context(), f.peer, f.eventID, q.ID); err != nil {
		t.Fatal("archive erased current-authority historical question", err)
	}
	input.CommandID = uuid.New()
	if _, err = f.service.Ask(
		t.Context(),
		f.peer,
		f.eventID,
		input,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("archive accepted a new question", err)
	}
}
