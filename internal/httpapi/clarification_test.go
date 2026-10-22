package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/inbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

// The HTTP fixture uses the real native inbox and native source authorizer. The
// production factory composition is independently exercised in internal/app.
type clarificationHTTPBridge struct {
	Answers *clarification.Service
	Inbox   *inbox.Service
}

func (b clarificationHTTPBridge) AuthorizeNotification(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	source inbox.Source,
) (inbox.Destination, error) {
	if err := b.Answers.AuthorizeNotificationInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		source.ID,
		source.Version,
	); err != nil {
		return inbox.Destination{}, fmt.Errorf("authorise native HTTP source: %w", err)
	}
	return inbox.Destination{Kind: "clarification", ResourceID: source.ID}, nil
}

func (b clarificationHTTPBridge) RecordClarificationInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, answerID uuid.UUID,
	version int64,
	people []uuid.UUID,
) error {
	for _, person := range people {
		if err := b.Inbox.RecordResourceInTransaction(
			ctx,
			tx,
			eventID,
			person,
			inbox.Source{Kind: "clarification", ID: answerID, Version: version},
		); err != nil {
			return fmt.Errorf("record native HTTP inbox source: %w", err)
		}
	}
	return nil
}

func TestClarificationHTTPCurrentTeamPrivacyCSRFActualPublicationAndPrivateAttachment(
	t *testing.T,
) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	owner, first, other := testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	c := event.DefaultConfiguration()
	c.MinimumAttendance, c.Timezone = 2, "Europe/London"
	zone, _ := time.LoadLocation(c.Timezone)
	c.LiveStart = time.Date(2027, 3, 29, 9, 0, 0, 0, zone)
	c.LiveEnd, c.SubmissionCutoff = c.LiveStart.Add(8*time.Hour), c.LiveStart.Add(7*time.Hour)
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
	teamID := uuid.New()
	for _, pair := range [][2]uuid.UUID{{first.ID, teamID}, {other.ID, uuid.New()}} {
		exec(
			"INSERT INTO hack_teams(event_id,id,name,state,frozen) VALUES($1,$2,'Synthetic HTTP team','published',true)",
			eventID,
			pair[1],
		)
		exec("INSERT INTO competition_entries(event_id,team_id) VALUES($1,$2)", eventID, pair[1])
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
		"INSERT INTO event_gate_facts(event_id,attendance_ready,teams_ready,total_teams,frozen_teams,assigned_attendees) VALUES($1,true,true,2,2,2)",
		eventID,
	)
	ref := testdb.ApprovedRubric(t, pool, owner.ID, eventID)
	clock := func() time.Time { return now }
	contentService := content.Service{Pool: pool, Clock: clock}
	challenge, err := contentService.Save(
		ctx,
		owner.ID,
		eventID,
		uuid.New(),
		0,
		content.Input{
			Kind: "challenge",
			Document: content.Document{
				Title:    "Common HTTP challenge",
				Markdown: "Identical common source",
				Audience: access.ContentAudience{Confirmed: true},
				Rubric:   &ref,
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"review", "approve"} {
		challenge, err = contentService.Action(
			ctx,
			owner.ID,
			eventID,
			challenge.ID,
			challenge.Revision,
			action,
			"Synthetic reviewed source",
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	storage, err := files.OpenLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	native := &clarification.Service{Pool: pool, Clock: clock, Storage: storage}
	notifications := &inbox.Service{Pool: pool, Now: clock}
	bridge := clarificationHTTPBridge{Answers: native, Inbox: notifications}
	notifications.Owners = map[string]inbox.ResourceOwner{"clarification": bridge}
	native.Notifications = bridge
	cfg, err := config.Load(func(key string) string {
		if key == "HACKBOX_ENV" {
			return "test"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	auth, err := identity.New(ctx, pool, identity.Settings{Origin: cfg.PublicOrigin})
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	router := NewRouter(
		cfg,
		slog.New(slog.NewJSONHandler(&logs, nil)),
		Dependencies{Pool: pool, Auth: auth, Clarifications: *native, Inbox: *notifications},
	)
	csrf := strings.Repeat("a", 32)
	tokens := map[uuid.UUID]string{}
	request := func(person dbgen.Person, method, path string, body any, revision int64, goodCSRF bool) *httptest.ResponseRecorder {
		t.Helper()
		if person.ID != uuid.Nil && tokens[person.ID] == "" {
			session, loadErr := auth.Sessions.Load(ctx, "")
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			auth.Sessions.Put(session, "person_id", person.ID.String())
			auth.Sessions.Put(session, "session_revision", person.SessionRevision)
			auth.Sessions.Put(session, "csrf", csrf)
			token, _, commitErr := auth.Sessions.Commit(session)
			if commitErr != nil {
				t.Fatal(commitErr)
			}
			tokens[person.ID] = token
		}
		var reader io.Reader
		if body != nil {
			encoded, _ := json.Marshal(body)
			reader = bytes.NewReader(encoded)
		}
		r := httptest.NewRequest(method, path, reader)
		if body != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("If-Match", fmt.Sprintf("\"%d\"", revision))
		if goodCSRF {
			r.Header.Set("X-CSRF-Token", csrf)
		} else {
			r.Header.Set("X-CSRF-Token", strings.Repeat("b", 32))
		}
		if person.ID != uuid.Nil {
			r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: tokens[person.ID]})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("private/common clarification response was cacheable")
		}
		return w
	}
	check := func(w *httptest.ResponseRecorder, status int, private bool) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("want%d got%d %s", status, w.Code, w.Body)
		}
		if !private &&
			(strings.Contains(w.Body.String(), "SYNTHETIC-PRIVATE") || strings.Contains(w.Body.String(), "private-question-file")) {
			t.Fatal("private question or attachment escaped its authorised response")
		}
	}
	positiveAttachment := func(w *httptest.ResponseRecorder) {
		t.Helper()
		if w.Code != 200 || w.Body.String() != "SYNTHETIC-PRIVATE-ATTACHMENT" {
			t.Errorf("positive authorised byte download failed: status=%d body=%s", w.Code, w.Body)
		}
	}
	base := "/api/v1/events/" + eventID.String()
	input := clarification.QuestionInput{
		TeamID:      teamID,
		CommandID:   uuid.New(),
		Topic:       "rubric",
		Body:        "SYNTHETIC-PRIVATE-QUESTION",
		Attachments: []uuid.UUID{},
	}
	check(request(first, "POST", base+"/questions", input, 0, true), 404, false)
	check(request(owner, "GET", base+"/questions", nil, 0, true), 404, false)
	check(request(other, "GET", base+"/clarifications", nil, 0, true), 404, false)
	now = due
	if err = contentService.Dispatch(ctx, eventID, challenge.ID, challenge.Revision); err != nil {
		t.Fatal(err)
	}
	policy := access.EventPolicy{Queries: dbgen.New(pool), Clock: clock}
	workingFiles := files.Service{
		Pool:       pool,
		Storage:    storage,
		Policy:     policy,
		MaxBytes:   1024,
		EventQuota: 4096,
	}
	currentEvent, err := dbgen.New(pool).Event(ctx, eventID)
	if err != nil {
		t.Fatal(err)
	}
	file, err := workingFiles.Upload(
		ctx,
		files.Upload{
			Actor:     first.ID,
			Event:     eventID,
			Key:       uuid.New(),
			TeamID:    &teamID,
			Revision:  currentEvent.Revision,
			Filename:  "private-question-file.txt",
			MediaType: "text/plain",
			Content:   []byte("SYNTHETIC-PRIVATE-ATTACHMENT"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	input.Attachments = []uuid.UUID{file.ID}
	check(request(dbgen.Person{}, "POST", base+"/questions", input, 0, true), 401, false)
	check(request(first, "POST", base+"/questions", input, 0, false), 403, false)
	malformed := map[string]any{
		"team_id":     teamID,
		"command_id":  uuid.New(),
		"topic":       "logistics",
		"body":        input.Body,
		"attachments": []uuid.UUID{},
	}
	check(request(first, "POST", base+"/questions", malformed, 0, true), 400, false)
	w := request(first, "POST", base+"/questions", input, 0, true)
	check(w, 200, true)
	var question clarification.Question
	if err = json.Unmarshal(w.Body.Bytes(), &question); err != nil {
		t.Fatal(err)
	}
	questionPath := base + "/questions/" + question.ID.String()
	check(request(other, "GET", questionPath, nil, 0, true), 404, false)
	check(request(other, "GET", questionPath+"/history", nil, 0, true), 404, false)
	check(request(owner, "GET", questionPath, nil, 0, true), 200, true)
	attachmentPath := questionPath + "/attachments/" + file.ID.String()
	positiveAttachment(request(first, "GET", attachmentPath, nil, 0, true))
	check(request(other, "GET", attachmentPath, nil, 0, true), 404, false)
	check(request(owner, "GET", attachmentPath, nil, 0, true), 404, false)
	check(
		request(
			first,
			"POST",
			questionPath+"/triage",
			map[string]string{"action": "triage"},
			question.Revision,
			true,
		),
		404,
		false,
	)
	check(
		request(
			owner,
			"POST",
			questionPath+"/triage",
			map[string]string{"action": "triage"},
			question.Revision,
			true,
		),
		200,
		true,
	)
	document := clarification.AnswerDocument{
		Title: "Reviewed common answer",
		Body:  "SYNTHETIC-COMMON-ANSWER",
		Kind:  "clarification",
	}
	check(request(other, "PUT", questionPath+"/answer", document, 0, true), 404, false)
	w = request(owner, "PUT", questionPath+"/answer", document, 0, true)
	check(w, 200, false)
	var answer clarification.AnswerConfiguration
	if err = json.Unmarshal(w.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	answerPath := base + "/clarifications/" + answer.ID.String()
	check(request(other, "GET", answerPath, nil, 0, true), 404, false)
	command := clarification.AnswerCommand{Action: "publish", Version: answer.Version}
	check(request(owner, "POST", answerPath+"/actions", command, answer.Revision, true), 409, false)
	command.Action = "approve"
	w = request(owner, "POST", answerPath+"/actions", command, answer.Revision, true)
	check(w, 200, false)
	if err = json.Unmarshal(w.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	command.Action = "publish"
	check(
		request(owner, "POST", answerPath+"/actions", command, answer.Revision, false),
		403,
		false,
	)
	check(request(owner, "POST", answerPath+"/actions", command, answer.Revision, true), 200, false)
	w = request(other, "GET", answerPath, nil, 0, true)
	check(w, 200, false)
	if strings.Contains(w.Body.String(), question.ID.String()) ||
		strings.Contains(w.Body.String(), teamID.String()) ||
		!strings.Contains(w.Body.String(), document.Body) {
		t.Fatal("common HTTP projection inherited a private relation or lost its reviewed answer")
	}
	check(
		request(
			other,
			"GET",
			base+"/clarifications?search=SYNTHETIC-PRIVATE-QUESTION",
			nil,
			0,
			true,
		),
		200,
		false,
	)
	check(request(other, "GET", answerPath+"/history", nil, 0, true), 200, false)
	now = c.LiveStart
	positiveAttachment(request(owner, "GET", attachmentPath, nil, 0, true))
	exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		eventID,
		first.ID,
	)
	check(request(first, "GET", questionPath, nil, 0, true), 404, false)
	check(request(first, "GET", attachmentPath, nil, 0, true), 404, false)
	exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='organiser'",
		eventID,
		owner.ID,
	)
	check(request(owner, "GET", questionPath, nil, 0, true), 404, false)
	check(request(owner, "GET", attachmentPath, nil, 0, true), 404, false)
	if strings.Contains(logs.String(), input.Body) ||
		strings.Contains(logs.String(), "SYNTHETIC-PRIVATE-ATTACHMENT") ||
		strings.Contains(logs.String(), document.Body) {
		t.Fatal("HTTP logs contained question, answer or attachment prose")
	}
}
