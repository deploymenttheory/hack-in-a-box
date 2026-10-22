package app

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

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type failingClarificationSink struct {
	base clarification.NotificationSink
}

func (s failingClarificationSink) RecordClarificationInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, answerID uuid.UUID,
	version int64,
	people []uuid.UUID,
) error {
	if err := s.base.RecordClarificationInTransaction(
		ctx,
		tx,
		eventID,
		answerID,
		version,
		people,
	); err != nil {
		return err
	}
	return errors.New("synthetic native inbox commit failure")
}

func TestClarificationRuntimePublishesOnlyReviewedCommonAnswersAndActualPrivateInboxReferences(
	t *testing.T,
) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	owner, author, other := testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	c := event.DefaultConfiguration()
	c.Timezone, c.MinimumAttendance = "Europe/London", 2
	zone, _ := time.LoadLocation(c.Timezone)
	c.LiveStart = time.Date(2027, 3, 29, 9, 0, 0, 0, zone)
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	c.SubmissionCutoff = c.LiveEnd.Add(-time.Hour)
	due, _ := schedule.ChallengePublication(c.LiveStart, c.Timezone, c.DSTChoice)
	c.TeamFreeze, c.AdmissionCutoff = due.Add(-time.Hour), due.Add(-2*time.Hour)
	now := due.Add(-time.Minute)
	raw, _ := json.Marshal(c)
	exec(
		"UPDATE events SET stage='preparation',configuration=$2,team_frozen_at=$3 WHERE id=$1",
		eventID,
		raw,
		c.TeamFreeze,
	)
	for _, person := range []uuid.UUID{author.ID, other.ID} {
		team := uuid.New()
		exec(
			"INSERT INTO hack_teams(event_id,id,name,state,frozen) VALUES($1,$2,'Synthetic inbox team','published',true)",
			eventID,
			team,
		)
		exec("INSERT INTO competition_entries(event_id,team_id) VALUES($1,$2)", eventID, team)
		exec(
			"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
			eventID,
			person,
		)
		exec(
			"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
			eventID,
			team,
			person,
		)
	}
	exec(
		"INSERT INTO event_gate_facts(event_id,attendance_ready,teams_ready,total_teams,frozen_teams,assigned_attendees) VALUES($1,true,true,2,2,2)",
		eventID,
	)
	ref := testdb.ApprovedRubric(t, pool, owner.ID, eventID)
	nativeContent := content.Service{Pool: pool, Clock: func() time.Time { return now }}
	challenge, err := nativeContent.Save(
		ctx,
		owner.ID,
		eventID,
		uuid.New(),
		0,
		content.Input{Kind: "challenge", Document: content.Document{
			Title:    "Synthetic shared challenge",
			Markdown: "Common published source",
			Audience: access.ContentAudience{Confirmed: true},
			Rubric:   &ref,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"review", "approve"} {
		challenge, err = nativeContent.Action(
			ctx,
			owner.ID,
			eventID,
			challenge.ID,
			challenge.Revision,
			action,
			"Synthetic owning release",
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	now = due
	if err = nativeContent.Dispatch(ctx, eventID, challenge.ID, challenge.Revision); err != nil {
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
	runtime.Clarifications.Clock = func() time.Time { return now }
	runtime.Inbox.Now = func() time.Time { return now }
	var teamID uuid.UUID
	if err = pool.QueryRow(ctx, "SELECT team_id FROM hack_team_members WHERE event_id=$1 AND person_id=$2 AND snapshot='published'", eventID, author.ID).
		Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	question, err := runtime.Clarifications.Ask(
		ctx,
		author.ID,
		eventID,
		clarification.QuestionInput{
			TeamID:    teamID,
			CommandID: uuid.New(),
			Topic:     "rubric",
			Body:      "SYNTHETIC-PRIVATE-QUESTION-DO-NOT-PUBLISH",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := runtime.Clarifications.SaveAnswer(
		ctx,
		owner.ID,
		eventID,
		question.ID,
		0,
		clarification.AnswerDocument{
			Title: "Common anchor interpretation",
			Body:  "SYNTHETIC-COMMON-ANSWER: all teams use the same approved anchor.",
			Kind:  "clarification",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = runtime.Clarifications.Public(
		ctx,
		other.ID,
		eventID,
		answer.ID,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("draft answer disclosed before explicit review/publication", err)
	}
	answer, err = runtime.Clarifications.ActAnswer(
		ctx,
		owner.ID,
		eventID,
		answer.ID,
		answer.Revision,
		clarification.AnswerCommand{Action: "approve", Version: answer.Version},
	)
	if err != nil {
		t.Fatal(err)
	}
	baseSink := runtime.Clarifications.Notifications
	runtime.Clarifications.Notifications = failingClarificationSink{base: baseSink}
	if _, err = runtime.Clarifications.ActAnswer(
		ctx,
		owner.ID,
		eventID,
		answer.ID,
		answer.Revision,
		clarification.AnswerCommand{Action: "publish", Version: answer.Version},
	); err == nil {
		t.Fatal("failed inbox transaction reported successful publication")
	}
	var count int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM clarification_publications WHERE event_id=$1", eventID).
		Scan(&count); err != nil ||
		count != 0 {
		t.Fatal("publication survived inbox rollback", count, err)
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM inbox_notifications WHERE event_id=$1 AND source_kind='clarification'", eventID).
		Scan(&count); err != nil ||
		count != 0 {
		t.Fatal("partial inbox references survived publication rollback", count, err)
	}
	runtime.Clarifications.Notifications = baseSink
	publishRevision := answer.Revision
	answer, err = runtime.Clarifications.ActAnswer(
		ctx,
		owner.ID,
		eventID,
		answer.ID,
		publishRevision,
		clarification.AnswerCommand{Action: "publish", Version: answer.Version},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = runtime.Clarifications.ActAnswer(
		ctx,
		owner.ID,
		eventID,
		answer.ID,
		publishRevision,
		clarification.AnswerCommand{Action: "publish", Version: answer.Version},
	); err != nil {
		t.Fatal("uncertain publication retry did not reuse retained result", err)
	}
	for _, person := range []uuid.UUID{author.ID, other.ID} {
		inbox, listErr := runtime.Inbox.List(ctx, person, nil, nil, 10)
		if listErr != nil || len(inbox.Items) != 1 {
			t.Fatal("actual common answer notification missing or duplicated", inbox, listErr)
		}
		detail, openErr := runtime.Inbox.Open(ctx, person, inbox.Items[0].ID, nil)
		if openErr != nil || detail.Destination == nil ||
			detail.Destination.Kind != "clarification" ||
			detail.Destination.ResourceID != answer.ID ||
			detail.Body != "" {
			t.Fatal("native inbox did not resolve safe canonical destination", detail, openErr)
		}
		page, readErr := runtime.Clarifications.Public(ctx, person, eventID, answer.ID)
		if readErr != nil || page.Answer.Version != 1 || !page.Answer.SourceCurrent {
			t.Fatal("common attendee projection unavailable", readErr)
		}
		serialized, _ := json.Marshal(page)
		for _, private := range []string{"SYNTHETIC-PRIVATE-QUESTION", question.ID.String(), teamID.String(), "question_id", "attachments"} {
			if strings.Contains(string(serialized), private) {
				t.Fatal("common answer disclosed private question data", private)
			}
		}
	}
	search, err := runtime.Clarifications.Search(
		ctx,
		other.ID,
		eventID,
		"SYNTHETIC-PRIVATE-QUESTION",
	)
	if err != nil || len(search.Items) != 0 {
		t.Fatal("public search disclosed private question existence", search, err)
	}
	var audit string
	if err = pool.QueryRow(ctx, "SELECT coalesce(jsonb_agg(to_jsonb(a))::text,'') FROM audit_events a WHERE event_id=$1", eventID).
		Scan(&audit); err != nil ||
		strings.Contains(audit, "SYNTHETIC-PRIVATE-QUESTION") ||
		strings.Contains(audit, "SYNTHETIC-COMMON-ANSWER") {
		t.Fatal("generic audit contained prose", err)
	}
	for _, kind := range []string{"challenge_change", "rubric_change"} {
		if _, err = runtime.Clarifications.SaveAnswer(
			ctx,
			owner.ID,
			eventID,
			question.ID,
			answer.Revision,
			clarification.AnswerDocument{
				Title: "Unreleased material change",
				Body:  "This must not bypass the owning release.",
				Kind:  kind,
			},
		); !errors.Is(
			err,
			access.ErrConflict,
		) {
			t.Fatal("a material label bypassed the actual owning version workflow", kind, err)
		}
	}
	answer, err = runtime.Clarifications.SaveAnswer(
		ctx,
		owner.ID,
		eventID,
		question.ID,
		answer.Revision,
		clarification.AnswerDocument{
			Title: "Corrected common interpretation",
			Body:  "SYNTHETIC-COMMON-CORRECTION: the same anchor still applies to every team.",
			Kind:  "clarification",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	prior, err := runtime.Clarifications.Public(ctx, other.ID, eventID, answer.ID)
	if err != nil || prior.Answer.Version != 1 || len(prior.History) != 1 {
		t.Fatal("a draft correction silently replaced the reviewed common answer", err)
	}
	approver := testdb.Person(t, pool)
	exec(
		"INSERT INTO event_memberships(event_id,person_id,role,active) VALUES($1,$2,'organiser',true)",
		eventID,
		approver.ID,
	)
	answer, err = runtime.Clarifications.ActAnswer(
		ctx,
		approver.ID,
		eventID,
		answer.ID,
		answer.Revision,
		clarification.AnswerCommand{Action: "approve", Version: answer.Version},
	)
	if err != nil {
		t.Fatal(err)
	}
	exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='organiser'",
		eventID,
		approver.ID,
	)
	if _, err = runtime.Clarifications.ActAnswer(
		ctx,
		owner.ID,
		eventID,
		answer.ID,
		answer.Revision,
		clarification.AnswerCommand{Action: "publish", Version: answer.Version},
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("revoked answer approver remained eligible to authorise a publication", err)
	}
	exec(
		"UPDATE event_memberships SET active=true WHERE event_id=$1 AND person_id=$2 AND role='organiser'",
		eventID,
		approver.ID,
	)
	var wg sync.WaitGroup
	failures := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			_, publishErr := runtime.Clarifications.ActAnswer(
				ctx,
				owner.ID,
				eventID,
				answer.ID,
				answer.Revision,
				clarification.AnswerCommand{Action: "publish", Version: answer.Version},
			)
			failures <- publishErr
		})
	}
	wg.Wait()
	close(failures)
	for publishErr := range failures {
		if publishErr != nil {
			t.Fatal(
				"concurrent duplicate publication did not retain the same official revision",
				publishErr,
			)
		}
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM clarification_publications WHERE event_id=$1", eventID).
		Scan(&count); err != nil ||
		count != 2 {
		t.Fatal("correction publication duplicated or overwrote retained history", count, err)
	}
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM inbox_notifications WHERE event_id=$1 AND source_kind='clarification'", eventID).
		Scan(&count); err != nil ||
		count != 4 {
		t.Fatal("concurrent correction duplicated a recipient notification", count, err)
	}
	corrected, err := runtime.Clarifications.Public(ctx, other.ID, eventID, answer.ID)
	if err != nil || corrected.Answer.Version != 2 || len(corrected.History) != 2 ||
		corrected.History[1].Current ||
		!strings.Contains(corrected.History[1].Document.Body, "SYNTHETIC-COMMON-ANSWER") {
		t.Fatal("official correction lost prior authored/published history", corrected, err)
	}
	historyPage, err := runtime.Clarifications.PublishedHistory(
		ctx,
		other.ID,
		eventID,
		answer.ID,
		0,
		1,
	)
	if err != nil || len(historyPage.Items) != 1 || historyPage.Items[0].Version != 2 ||
		historyPage.NextCursor == nil {
		t.Fatal("published history was not bounded by exact version", historyPage, err)
	}
	historyPage, err = runtime.Clarifications.PublishedHistory(
		ctx,
		other.ID,
		eventID,
		answer.ID,
		*historyPage.NextCursor,
		1,
	)
	if err != nil || len(historyPage.Items) != 1 || historyPage.Items[0].Version != 1 ||
		historyPage.NextCursor != nil {
		t.Fatal(
			"published history continuation skipped or duplicated a correction",
			historyPage,
			err,
		)
	}
	if _, err = pool.Exec(
		ctx,
		"UPDATE clarification_versions SET document='{}' WHERE event_id=$1",
		eventID,
	); err == nil {
		t.Fatal("retained official answer versions were mutable")
	}
	answer = verifyMaterialClarificationWorkflow(
		t,
		runtime,
		nativeContent,
		owner.ID,
		author.ID,
		eventID,
		question.ID,
		answer.ID,
		func() time.Time { return now },
	)
	verifyClarificationSourceAndRoleRaces(
		t,
		runtime,
		nativeContent,
		owner.ID,
		author.ID,
		approver.ID,
		eventID,
		teamID,
		question.ID,
	)
	exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		eventID,
		other.ID,
	)
	if page, listErr := runtime.Inbox.List(
		ctx,
		other.ID,
		nil,
		nil,
		10,
	); listErr != nil || len(page.Items) != 0 ||
		page.Unread != 0 {
		t.Fatal("withdrawal retained private recipient notifications", page, listErr)
	}
	if page, readErr := runtime.Clarifications.Public(
		ctx,
		other.ID,
		eventID,
		answer.ID,
	); !errors.Is(readErr, access.ErrNotFound) ||
		page.Answer.ID != uuid.Nil {
		t.Fatal("withdrawal retained canonical common-answer access", page, readErr)
	}
	exec("UPDATE events SET writes_allowed=false WHERE id=$1", eventID)
	if page, readErr := runtime.Clarifications.Public(
		ctx,
		author.ID,
		eventID,
		answer.ID,
	); readErr != nil || !page.Archived ||
		page.Answer.Version != answer.Version {
		t.Fatal("archive erased currently authorised common answer history", page, readErr)
	}
	if _, err = runtime.Clarifications.ActAnswer(
		ctx,
		owner.ID,
		eventID,
		answer.ID,
		answer.Revision,
		clarification.AnswerCommand{Action: "publish", Version: answer.Version},
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("archive allowed an answer write", err)
	}
}
