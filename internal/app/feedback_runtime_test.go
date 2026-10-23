package app

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/feedback"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestFeedbackActualRuntimePrivateQuestionReviewInboxAndZeroCampaignOverdueMaintenance(
	t *testing.T,
) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	owner, member, other, mentor, logistics := testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	)
	eventID := testdb.Event(t, pool, owner.ID)
	team, otherTeam := uuid.New(), uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	c := event.DefaultConfiguration()
	c.Timezone = "UTC"
	c.MinimumAttendance = 2
	c.LiveStart = time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	c.SubmissionCutoff = c.LiveEnd.Add(-time.Hour)
	due, err := schedule.ChallengePublication(c.LiveStart, c.Timezone, c.DSTChoice)
	if err != nil {
		t.Fatal(err)
	}
	c.TeamFreeze = due.Add(-time.Hour)
	c.AdmissionCutoff = due.Add(-2 * time.Hour)
	now := due.Add(-time.Minute)
	raw, _ := json.Marshal(c)
	exec(
		"UPDATE events SET stage='preparation',configuration=$2,team_frozen_at=$3 WHERE id=$1",
		eventID,
		raw,
		c.TeamFreeze,
	)
	for _, id := range []uuid.UUID{team, otherTeam} {
		exec(
			"INSERT INTO hack_teams(event_id,id,name,state,frozen) VALUES($1,$2,'Synthetic feedback team','published',true)",
			eventID,
			id,
		)
		exec("INSERT INTO competition_entries(event_id,team_id) VALUES($1,$2)", eventID, id)
	}
	for _, pair := range [][2]uuid.UUID{{member.ID, team}, {other.ID, otherTeam}} {
		exec(
			"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
			eventID,
			pair[0],
		)
		exec(
			"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
			eventID,
			pair[1],
			pair[0],
		)
	}
	exec(
		"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mentor'),($1,$3,'logistics')",
		eventID,
		mentor.ID,
		logistics.ID,
	)
	exec(
		"INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at) VALUES($1,$2,$3,'primary','published',$4,$5)",
		eventID,
		team,
		mentor.ID,
		c.LiveStart,
		c.LiveEnd,
	)
	exec(
		"INSERT INTO event_gate_facts(event_id,attendance_ready,teams_ready,total_teams,frozen_teams,assigned_attendees) VALUES($1,true,true,2,2,2)",
		eventID,
	)
	ref := testdb.ApprovedRubric(t, pool, owner.ID, eventID)
	native := content.Service{Pool: pool, Clock: func() time.Time { return now }}
	challenge, err := native.Save(
		ctx,
		owner.ID,
		eventID,
		uuid.New(),
		0,
		content.Input{
			Kind: "challenge",
			Document: content.Document{
				Title:    "Common challenge",
				Markdown: "Same source for all teams",
				Audience: access.ContentAudience{Confirmed: true},
				Rubric:   &ref,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"review", "approve"} {
		challenge, err = native.Action(
			ctx,
			owner.ID,
			eventID,
			challenge.ID,
			challenge.Revision,
			action,
			"Synthetic explicit common release",
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	now = due
	if err = native.Dispatch(ctx, eventID, challenge.ID, challenge.Revision); err != nil {
		t.Fatal(err)
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
	runtime, err := newCommunicationRuntime(
		pool,
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	now = c.LiveStart.Add(time.Hour)
	exec("UPDATE events SET stage='live' WHERE id=$1", eventID)
	runtime.Clarifications.Clock = func() time.Time { return now }
	runtime.Feedback.Clock = func() time.Time { return now }
	runtime.Help.Clock = func() time.Time { return now }
	runtime.Inbox.Now = func() time.Time { return now }
	question, err := runtime.Clarifications.Ask(
		ctx,
		member.ID,
		eventID,
		clarification.QuestionInput{
			TeamID:    team,
			CommandID: uuid.New(),
			Topic:     "rubric",
			Body:      "PRIVATE ORIGINAL QUESTION MUST NOT AUTO-COPY",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	feedbackID := uuid.New()
	input := feedback.Input{
		TeamID:   team,
		Category: "competition_output",
		Title:    "Reviewed specific advice",
		Body:     "Selected hand-written team guidance",
		Sources: []feedback.Source{
			{Kind: "question", ID: question.ID, Version: question.Revision},
		},
	}
	if _, err = runtime.Feedback.Change(
		ctx,
		mentor.ID,
		eventID,
		feedbackID,
		0,
		uuid.New(),
		feedback.Command{Action: "save", Input: &input},
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("mentor gained private question authority", err)
	}
	p, err := runtime.Feedback.Change(
		ctx,
		owner.ID,
		eventID,
		feedbackID,
		0,
		uuid.New(),
		feedback.Command{Action: "save", Input: &input},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = runtime.Feedback.Read(
		ctx,
		mentor.ID,
		eventID,
		feedbackID,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("unreleased private-question-derived body leaked to category helper", err)
	}
	for _, action := range []string{"review", "publish"} {
		p, err = runtime.Feedback.Change(
			ctx,
			owner.ID,
			eventID,
			feedbackID,
			p.CurrentVersion,
			uuid.New(),
			feedback.Command{Action: action},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	page, err := runtime.Feedback.Read(ctx, member.ID, eventID, feedbackID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(page)
	if strings.Contains(string(encoded), "PRIVATE ORIGINAL") ||
		strings.Contains(string(encoded), question.ID.String()) {
		t.Fatal("raw private question/provenance copied to team")
	}
	// A later unreleased source-derived draft and its history remain private even
	// while the same helper can still read the previous deliberately released text.
	input.Body = "PRIVATE UNRELEASED SOURCE-DERIVED DRAFT"
	p, err = runtime.Feedback.Change(
		ctx,
		owner.ID,
		eventID,
		feedbackID,
		p.CurrentVersion,
		uuid.New(),
		feedback.Command{Action: "save", Input: &input},
	)
	if err != nil {
		t.Fatal(err)
	}
	helper, err := runtime.Feedback.Read(ctx, mentor.ID, eventID, feedbackID)
	if err != nil || helper.Draft != nil || helper.Published == nil {
		t.Fatal("source-derived draft authority", err)
	}
	history, err := runtime.Feedback.History(ctx, mentor.ID, eventID, feedbackID, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ = json.Marshal(history)
	if strings.Contains(string(encoded), "PRIVATE UNRELEASED") {
		t.Fatal("private draft history leaked")
	}
	inboxPage, err := runtime.Inbox.List(ctx, member.ID, nil, nil, 20)
	if err != nil || len(inboxPage.Items) != 1 {
		t.Fatal("actual publication inbox missing", err)
	}
	opened, err := runtime.Inbox.Open(ctx, member.ID, inboxPage.Items[0].ID, nil)
	if err != nil || opened.Destination == nil || opened.Destination.Kind != "feedback" ||
		opened.Destination.ResourceID != feedbackID {
		t.Fatal("real feedback destination missing", err)
	}
	encoded, _ = json.Marshal(opened)
	if strings.Contains(string(encoded), question.ID.String()) ||
		strings.Contains(string(encoded), "PRIVATE") ||
		strings.Contains(string(encoded), "Selected hand-written") {
		t.Fatal("notification contained source or published prose instead of fixed preview")
	}
	setupID := uuid.New()
	setup, err := runtime.Feedback.Change(
		ctx,
		owner.ID,
		eventID,
		setupID,
		0,
		uuid.New(),
		feedback.Command{
			Action: "save",
			Input: &feedback.Input{
				TeamID:   team,
				Category: "setup",
				Title:    "Operational kit follow-up",
				Body:     "Reviewed kit instructions",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"review", "publish"} {
		setup, err = runtime.Feedback.Change(
			ctx,
			owner.ID,
			eventID,
			setupID,
			setup.CurrentVersion,
			uuid.New(),
			feedback.Command{Action: action},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = runtime.Feedback.ChangeAction(
		ctx,
		owner.ID,
		eventID,
		setupID,
		uuid.New(),
		0,
		uuid.New(),
		feedback.ActionCommand{
			Action: "details",
			Input: &feedback.ActionInput{
				Reviewed: true,
				Title:    "Reviewed operational task",
				Body:     "Check kit against the shared checklist",
				OwnerID:  member.ID,
				DueAt:    now.Add(-time.Hour),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = runtime.maintainSupport(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var campaigns, notices int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM communication_campaign_versions WHERE event_id=$1", eventID).
		Scan(&campaigns); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM inbox_notifications WHERE event_id=$1 AND source_kind='feedback_followup'", eventID).
		Scan(&notices); err != nil {
		t.Fatal(err)
	}
	if campaigns != 0 || notices != 2 {
		t.Fatal("zero-campaign maintenance/recipient dedup", campaigns, notices)
	}
	staff, err := runtime.Inbox.List(ctx, logistics.ID, nil, nil, 20)
	if err != nil || len(staff.Items) != 1 {
		t.Fatal("category logistics follow-up unavailable", err)
	}
	detail, err := runtime.Inbox.Open(ctx, logistics.ID, staff.Items[0].ID, nil)
	if err != nil || detail.Destination == nil || detail.Destination.Kind != "feedback_followup" {
		t.Fatal("actual follow-up route unavailable", err)
	}
	if _, err = runtime.Feedback.Followup(
		ctx,
		logistics.ID,
		eventID,
		detail.Destination.ResourceID,
	); err != nil {
		t.Fatal(err)
	}
	exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='logistics'",
		eventID,
		logistics.ID,
	)
	staff, err = runtime.Inbox.List(ctx, logistics.ID, nil, nil, 20)
	if err != nil || len(staff.Items) != 0 || staff.Unread != 0 {
		t.Fatal("revoked target preview/count leak", err)
	}
	exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		eventID,
		member.ID,
	)
	inboxPage, err = runtime.Inbox.List(ctx, member.ID, nil, nil, 20)
	if err != nil || len(inboxPage.Items) != 0 || inboxPage.Unread != 0 {
		t.Fatal("current own-team inbox read/count leak", err)
	}
}
