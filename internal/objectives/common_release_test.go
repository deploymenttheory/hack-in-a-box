package objectives_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/objectives"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestObjectiveThemeDisclosurePublishesOnlyInsideActualGuardedCommonChallenge(t *testing.T) {
	pool := testdb.New(t, true)
	owner, attendee := testdb.Person(t, pool), testdb.Person(t, pool)
	eventID, team := testdb.Event(t, pool, owner.ID), uuid.New()
	c := event.DefaultConfiguration()
	c.MinimumAttendance = 1
	c.LiveStart = time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	c.SubmissionCutoff = c.LiveEnd
	due, err := schedule.ChallengePublication(c.LiveStart, c.Timezone, c.DSTChoice)
	if err != nil {
		t.Fatal(err)
	}
	c.TeamFreeze = due.Add(-time.Hour)
	c.AdmissionCutoff = c.TeamFreeze.Add(-time.Hour)
	now := due.Add(-time.Minute)
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(t.Context(), sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(
		"UPDATE events SET stage='preparation',configuration=$2,team_frozen_at=$3 WHERE id=$1",
		eventID,
		raw,
		c.TeamFreeze,
	)
	exec(
		"INSERT INTO event_gate_facts(event_id,attendance_ready,teams_ready,total_teams,frozen_teams,assigned_attendees) VALUES($1,true,true,1,1,1)",
		eventID,
	)
	exec(
		"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
		eventID,
		attendee.ID,
	)
	exec(
		"INSERT INTO hack_teams(event_id,id,name,state,frozen) VALUES($1,$2,'Confirmed synthetic team','published',true)",
		eventID,
		team,
	)
	exec(
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
		eventID,
		team,
		attendee.ID,
	)
	nativeRubric := testdb.ApprovedRubric(t, pool, owner.ID, eventID)
	s := objectives.Service{Pool: pool, Clock: func() time.Time { return now }}
	cs := content.Service{Pool: pool, Clock: func() time.Time { return now }}
	id, measure, challengeID := uuid.New(), uuid.New(), uuid.New()
	d := objectives.Definition{
		Description:    "PRIVATE COMMON THEME OBJECTIVE",
		ThemeSensitive: true,
		Measures: []objectives.Measure{
			{ID: measure, Description: "Selected success measure"},
		},
	}
	objective, err := s.Change(
		t.Context(),
		owner.ID,
		eventID,
		id,
		0,
		objectives.Command{Action: "save", Definition: &d, Reason: "Plan theme objective"},
	)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := cs.Save(
		t.Context(),
		owner.ID,
		eventID,
		challengeID,
		0,
		content.Input{
			Kind: "challenge",
			Document: content.Document{
				Title:    "Common challenge",
				Markdown: "Guarded common challenge text",
				Rubric:   &nativeRubric,
				Audience: access.ContentAudience{Confirmed: true},
				Objectives: []objectives.Disclosure{
					{Objective: objective.Reference, MeasureIDs: []uuid.UUID{measure}},
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"review", "approve"} {
		challenge, err = cs.Action(
			t.Context(),
			owner.ID,
			eventID,
			challengeID,
			challenge.Revision,
			action,
			"Explicit common objective disclosure",
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.Public(
		t.Context(),
		attendee.ID,
		eventID,
		challengeID,
		objective.Reference,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("approved future theme disclosed early", err)
	}
	if err = cs.Dispatch(t.Context(), eventID, challengeID, challenge.Revision); err != nil {
		t.Fatal("early scheduler check", err)
	}
	if _, err = s.Public(
		t.Context(),
		attendee.ID,
		eventID,
		challengeID,
		objective.Reference,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("early guarded dispatch disclosed objective", err)
	}
	now = due
	if err = cs.Dispatch(t.Context(), eventID, challengeID, challenge.Revision); err != nil {
		t.Fatal(err)
	}
	public, err := s.Public(t.Context(), attendee.ID, eventID, challengeID, objective.Reference)
	if err != nil || public.Description != "PRIVATE COMMON THEME OBJECTIVE" ||
		len(public.Measures) != 1 {
		t.Fatal("actual common release failed to disclose exact selected definition", public, err)
	}
	exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		eventID,
		attendee.ID,
	)
	if _, err = s.Public(
		t.Context(),
		attendee.ID,
		eventID,
		challengeID,
		objective.Reference,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("current confirmation revocation ignored", err)
	}
}
