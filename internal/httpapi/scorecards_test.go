package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
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
	"github.com/deploymenttheory/hack-in-a-box/internal/attendance"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/inbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/presentation"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/deploymenttheory/hack-in-a-box/internal/teams"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestScorecardHTTPAuthorPrivacyClockEvidenceAndCurrentSession(t *testing.T) {
	ctx := t.Context()
	pool := testdb.New(t, true)
	owner, mentor, second, member := testdb.Person(
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
	eventID, teamID, entryID := testdb.Event(t, pool, owner.ID), uuid.New(), uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	dates := event.DefaultConfiguration()
	dates.Timezone = "Europe/London"
	dates.MinimumAttendance = 1
	dates.LiveStart = time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	dates.LiveEnd = dates.LiveStart.Add(8 * time.Hour)
	dates.SubmissionCutoff = dates.LiveEnd.Add(-time.Hour)
	due, err := schedule.ChallengePublication(dates.LiveStart, dates.Timezone, dates.DSTChoice)
	if err != nil {
		t.Fatal(err)
	}
	dates.TeamFreeze = due.Add(-time.Hour)
	dates.AdmissionCutoff = dates.TeamFreeze.Add(-time.Hour)
	now := due.Add(-time.Minute)
	raw, _ := json.Marshal(dates)
	exec(
		"UPDATE events SET stage='preparation',configuration=$2,team_frozen_at=$3 WHERE id=$1",
		eventID,
		raw,
		dates.TeamFreeze,
	)
	exec(
		"INSERT INTO event_gate_facts(event_id,attendance_ready,teams_ready,total_teams,frozen_teams,assigned_attendees) VALUES($1,true,true,1,1,1)",
		eventID,
	)
	exec(
		"INSERT INTO hack_teams(event_id,id,name,state,frozen) VALUES($1,$2,'HTTP private score team','published',true)",
		eventID,
		teamID,
	)
	exec(
		"INSERT INTO competition_entries(id,event_id,team_id) VALUES($1,$2,$3)",
		entryID,
		eventID,
		teamID,
	)
	exec(
		"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
		eventID,
		member.ID,
	)
	exec(
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
		eventID,
		teamID,
		member.ID,
	)
	exec(
		"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mentor'),($1,$3,'mentor')",
		eventID,
		mentor.ID,
		second.ID,
	)
	exec(
		"INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at) VALUES($1,$2,$3,'primary','published',$5,$6),($1,$2,$4,'second','published',$5,$6)",
		eventID,
		teamID,
		mentor.ID,
		second.ID,
		dates.LiveStart,
		dates.LiveEnd,
	)
	exec(
		"INSERT INTO policy_resources(event_id,id,kind) VALUES($1,$2,'team'),($1,$3,'assessment')",
		eventID,
		teamID,
		entryID,
	)
	ref := testdb.ApprovedRubric(t, pool, owner.ID, eventID)
	publisher := content.Service{Pool: pool, Clock: func() time.Time { return now }}
	challenge, err := publisher.Save(
		ctx,
		owner.ID,
		eventID,
		uuid.New(),
		0,
		content.Input{
			Kind: "challenge",
			Document: content.Document{
				Title:    "HTTP actual common challenge",
				Markdown: "Synthetic actual guarded release",
				Rubric:   &ref,
				Audience: access.ContentAudience{Confirmed: true},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"review", "approve"} {
		challenge, err = publisher.Action(
			ctx,
			owner.ID,
			eventID,
			challenge.ID,
			challenge.Revision,
			action,
			"HTTP synthetic approved release",
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	now = due
	if err = publisher.Dispatch(ctx, eventID, challenge.ID, challenge.Revision); err != nil {
		t.Fatal(err)
	}
	rs := rubric.Service{Pool: pool, Clock: func() time.Time { return now }}
	configuration, err := rs.Configuration(ctx, owner.ID, eventID, 0)
	if err != nil {
		t.Fatal(err)
	}
	configuration, err = rs.Change(
		ctx,
		owner.ID,
		eventID,
		configuration.Revision,
		rubric.Command{
			Action:  "create_run",
			Purpose: "checkpoint",
			Version: ref.Version,
			Reason:  "HTTP native checkpoint run",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	run := configuration.Runs[0].ID
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
	logistics, mc := testdb.Person(t, pool), testdb.Person(t, pool)
	exec(
		"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'logistics'),($1,$3,'mc')",
		eventID,
		logistics.ID,
		mc.ID,
	)
	room := presentation.Service{Pool: pool}
	composer := communications.Service{Pool: pool, Now: func() time.Time { return now }}
	mailbox := inbox.Service{
		Pool:           pool,
		Communications: &composer,
		Now:            func() time.Time { return now },
	}
	composer.Channels = map[string]communications.ChannelProvider{"in_app": &mailbox}
	var logs bytes.Buffer
	router := NewRouter(
		cfg,
		slog.New(slog.NewTextHandler(&logs, nil)),
		Dependencies{
			Pool:       pool,
			Auth:       auth,
			Scorecards: scorecards.Service{Pool: pool, Now: func() time.Time { return now }},
			Teams:      teams.Service{Pool: pool},
			Attendance: attendance.Service{
				Pool: pool,
			},
			Presentation:   room,
			Communications: composer,
			Inbox:          mailbox,
		},
	)
	revision := func() int64 {
		t.Helper()
		row, err := dbgen.New(pool).Event(ctx, eventID)
		if err != nil {
			t.Fatal(err)
		}
		return row.Revision
	}
	const csrf = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	tokens := map[uuid.UUID]string{}
	request := func(person dbgen.Person, method, path string, body any, rev int64, goodCSRF bool) *httptest.ResponseRecorder {
		t.Helper()
		if person.ID != uuid.Nil && tokens[person.ID] == "" {
			session, err := auth.Sessions.Load(ctx, "")
			if err != nil {
				t.Fatal(err)
			}
			auth.Sessions.Put(session, "person_id", person.ID.String())
			auth.Sessions.Put(session, "session_revision", person.SessionRevision)
			auth.Sessions.Put(session, "csrf", csrf)
			token, _, err := auth.Sessions.Commit(session)
			if err != nil {
				t.Fatal(err)
			}
			tokens[person.ID] = token
		}
		var reader io.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			reader = bytes.NewReader(raw)
		}
		r := httptest.NewRequest(method, path, reader)
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("If-Match", fmt.Sprintf("\"%d\"", rev))
		if body != nil {
			r.Header.Set("Content-Type", "application/json")
		}
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
		return w
	}
	check := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("status%d want%d: %s", w.Code, status, w.Body.String())
		}
		if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("private scores cacheable")
		}
	}
	base := "/api/v1/events/" + eventID.String()
	path := base + "/teams/" + teamID.String() + "/scorecards"
	cpPath := base + "/scorecard-checkpoints"
	zone, _ := time.LoadLocation(dates.Timezone)
	command := scorecards.CheckpointCommand{
		Definition: scorecards.CheckpointDefinition{
			ID:             uuid.New(),
			Name:           "HTTP configured checkpoint",
			RunID:          run,
			OpensWallTime:  dates.LiveStart.In(zone).Format("2006-01-02T15:04:05"),
			ClosesWallTime: dates.SubmissionCutoff.In(zone).Format("2006-01-02T15:04:05"),
		},
		Reason: "HTTP explicit checkpoint",
	}
	check(request(member, http.MethodPost, cpPath, command, revision(), true), http.StatusForbidden)
	check(request(owner, http.MethodPost, cpPath, command, revision(), false), http.StatusForbidden)
	w := request(owner, http.MethodPost, cpPath, command, revision(), true)
	check(w, http.StatusOK)
	var checkpoint scorecards.Checkpoint
	if err = json.Unmarshal(w.Body.Bytes(), &checkpoint); err != nil {
		t.Fatal(err)
	}
	cardCommand := scorecards.CardCommand{
		Action:            "save",
		CheckpointID:      checkpoint.Definition.ID,
		CheckpointVersion: checkpoint.Version,
		Scores:            []scorecards.CategoryScore{},
	}
	check(
		request(mentor, http.MethodPost, path, cardCommand, revision(), true),
		http.StatusForbidden,
	)
	now = dates.LiveStart
	exec("UPDATE events SET stage='live' WHERE id=$1", eventID)
	check(
		request(dbgen.Person{}, http.MethodGet, path, nil, revision(), true),
		http.StatusUnauthorized,
	)
	check(request(member, http.MethodGet, path, nil, revision(), true), http.StatusForbidden)
	check(request(owner, http.MethodGet, path, nil, revision(), true), http.StatusForbidden)
	assigned := base + "/teams/" + teamID.String() + "/scorecard-checkpoints"
	check(request(mentor, http.MethodGet, assigned, nil, revision(), true), http.StatusOK)
	category := checkpoint.Criteria.Categories[0]
	value := category.MinScore
	cardCommand.Scores = []scorecards.CategoryScore{
		{
			CategoryID: category.ID,
			Value:      &value,
			Evidence: []scorecards.EvidenceReference{
				{Kind: "note", Note: "HTTP-PRIVATE-SCORE-EVIDENCE-CANARY"},
			},
		},
	}
	stale := revision()
	w = request(mentor, http.MethodPost, path, cardCommand, stale, true)
	check(w, http.StatusOK)
	var card scorecards.Card
	if err = json.Unmarshal(w.Body.Bytes(), &card); err != nil {
		t.Fatal(err)
	}
	if len(card.Scores) != 5 || !card.Scores[1].Pending || card.Scores[1].Value != nil ||
		card.Revision != stale+1 {
		t.Fatal("HTTP pending/author/revision contract lost")
	}
	check(request(mentor, http.MethodPost, path, cardCommand, stale, true), http.StatusConflict)
	w = request(second, http.MethodGet, path, nil, revision(), true)
	check(w, http.StatusOK)
	if strings.Contains(w.Body.String(), "HTTP-PRIVATE-SCORE-EVIDENCE-CANARY") {
		t.Fatal("independent mentor saw another score")
	}
	historyPath := path + "/" + card.Reference.ID.String() + "/history"
	check(request(second, http.MethodGet, historyPath, nil, revision(), true), http.StatusForbidden)
	cardCommand.Version = 1
	cardCommand.Action = "submit"
	w = request(mentor, http.MethodPost, path, cardCommand, revision(), true)
	check(w, http.StatusOK)
	w = request(mentor, http.MethodGet, historyPath+"?limit=1", nil, revision(), true)
	check(w, http.StatusOK)
	var history scorecards.History
	if err = json.Unmarshal(w.Body.Bytes(), &history); err != nil || history.NextCursor == nil {
		t.Fatal("history cursor unavailable", err)
	}
	check(
		request(
			mentor,
			http.MethodGet,
			fmt.Sprintf("%s?cursor=%d&limit=1", historyPath, *history.NextCursor),
			nil,
			revision(),
			true,
		),
		http.StatusOK,
	)
	check(
		request(
			owner,
			http.MethodGet,
			path+"/panel?context_id="+uuid.New().String(),
			nil,
			revision(),
			true,
		),
		http.StatusForbidden,
	)
	check(
		request(
			mentor,
			http.MethodPost,
			path,
			map[string]any{
				"action":             "signoff",
				"checkpoint_id":      checkpoint.Definition.ID,
				"checkpoint_version": 1,
				"version":            2,
				"scores":             []any{},
			},
			revision(),
			true,
		),
		http.StatusBadRequest,
	)
	participant := request(member, http.MethodGet, base+"/teams", nil, revision(), true)
	check(participant, http.StatusOK)
	if strings.Contains(participant.Body.String(), "HTTP-PRIVATE-SCORE-EVIDENCE-CANARY") {
		t.Fatal("participant projection leaked scores")
	}
	privateFree := func(w *httptest.ResponseRecorder, required string) {
		t.Helper()
		check(w, http.StatusOK)
		if !strings.Contains(w.Body.String(), required) {
			t.Fatal("privacy fixture unpopulated", w.Body.String())
		}
		if strings.Contains(w.Body.String(), "HTTP-PRIVATE-SCORE-EVIDENCE-CANARY") ||
			strings.Contains(w.Body.String(), "category_id") ||
			strings.Contains(w.Body.String(), "rubric_version") {
			t.Fatal("private scorecard entered unrelated payload")
		}
	}
	privateFree(
		request(logistics, http.MethodGet, base+"/logistics", nil, revision(), true),
		member.ID.String(),
	)
	privateFree(
		request(logistics, http.MethodGet, base+"/logistics/export", nil, revision(), true),
		member.ID.String(),
	)
	program, err := room.Read(ctx, owner.ID, eventID)
	if err != nil {
		t.Fatal(err)
	}
	firstCue := 0
	for _, command := range []presentation.Command{
		{Action: "save", Reason: "Synthetic approved public cue", Cues: []presentation.Cue{{ID: uuid.New(), Title: "Approved public room cue", DurationSeconds: 60}}},
		{Action: "activate", FromDraft: true, Index: &firstCue, Reason: "Explicitly approve synthetic room program"},
	} {
		program, err = room.Change(ctx, owner.ID, eventID, program.Revision, command)
		if err != nil {
			t.Fatal(err)
		}
	}
	privateFree(
		request(mc, http.MethodGet, base+"/presentation", nil, revision(), true),
		"Approved public room cue",
	)
	paired, err := room.Pair(
		ctx,
		owner.ID,
		eventID,
		program.Revision,
		presentation.PairInput{
			Label:   "Synthetic private-note exclusion test",
			Minutes: 60,
			Reason:  "Synthetic verification",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, base+"/display/snapshot", nil)
	r.Header.Set("Authorization", "Bearer "+paired.Token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, r)
	validateResponse(t, r, w)
	privateFree(w, "Approved public room cue")
	template := communications.Template{
		ID:      uuid.New(),
		Name:    "Synthetic public reminder",
		Purpose: "reminder",
		Format:  "text",
		Subject: "Event reminder",
		Body:    "Hello {{person.name}}",
	}
	audience := communications.Audience{
		ID:                 uuid.New(),
		Name:               "Current mentors",
		Roles:              []string{"mentor"},
		Teams:              []uuid.UUID{},
		RegistrationStates: []string{},
	}
	for _, command := range []communications.Command{{Action: "save_template", Template: &template}, {Action: "save_audience", Audience: &audience}} {
		command.Reason = "Synthetic projection privacy verification"
		if _, err = composer.Change(ctx, owner.ID, eventID, revision(), command); err != nil {
			t.Fatal(err)
		}
	}
	privateFree(
		request(
			owner,
			http.MethodPost,
			base+"/communications",
			map[string]any{
				"action":      "preview",
				"template_id": template.ID,
				"audience_id": audience.ID,
				"reason":      "Synthetic actual notification preview",
			},
			revision(),
			true,
		),
		mentor.ID.String(),
	)
	definition := communications.CampaignDefinition{
		ID:         uuid.New(),
		Name:       "Synthetic privacy reminder",
		TemplateID: template.ID,
		AudienceID: audience.ID,
		Channels:   []string{"in_app"},
		Timing: communications.CampaignTiming{
			Kind:    "relative",
			Anchor:  "live_start",
			Minutes: 1,
		},
	}
	var campaign communications.Campaign
	for _, action := range []string{"save", "preview", "approve"} {
		command := communications.CampaignCommand{
			Action:  action,
			Version: campaign.Version,
			Reason:  "Synthetic notification privacy verification",
		}
		if action == "save" {
			command.Definition = &definition
		}
		result, changeErr := composer.ChangeCampaign(
			ctx,
			owner.ID,
			eventID,
			definition.ID,
			revision(),
			command,
		)
		if changeErr != nil {
			var blocked *event.Blocked
			if errors.As(changeErr, &blocked) {
				t.Fatalf("%s campaign blockers: %#v", action, blocked.Issues)
			}
			t.Fatal(changeErr)
		}
		campaign = result.Campaign.Campaign
	}
	now = campaign.Approval.DueAt
	if err = database.Within(ctx, pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		count, queueErr := mailbox.QueueApprovedInTransaction(
			ctx,
			tx,
			eventID,
			definition.ID,
			campaign.Version,
		)
		if queueErr == nil && count != 2 {
			t.Fatalf("notification privacy fixture queued=%d want2", count)
		}
		return queueErr
	}); err != nil {
		t.Fatal(err)
	}
	w = request(mentor, http.MethodGet, "/api/v1/inbox", nil, revision(), true)
	check(w, http.StatusOK)
	var notifications inbox.Page
	if err = json.Unmarshal(
		w.Body.Bytes(),
		&notifications,
	); err != nil ||
		len(notifications.Items) != 1 {
		t.Fatalf("notification privacy fixture did not deliver exactly one message: %v", err)
	}
	privateFree(w, notifications.Items[0].ID.String())
	privateFree(
		request(
			mentor,
			http.MethodGet,
			"/api/v1/inbox/"+notifications.Items[0].ID.String(),
			nil,
			revision(),
			true,
		),
		"Hello ",
	)
	exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mentor'",
		eventID,
		mentor.ID,
	)
	for _, target := range []string{path, historyPath, assigned} {
		w = request(mentor, http.MethodGet, target, nil, revision(), true)
		check(w, http.StatusForbidden)
		if strings.Contains(w.Body.String(), "HTTP-PRIVATE-SCORE-EVIDENCE-CANARY") {
			t.Fatal("revoked cookie retained scores")
		}
	}
	if strings.Contains(logs.String(), "HTTP-PRIVATE-SCORE-EVIDENCE-CANARY") {
		t.Fatal("private scoring entered request logs")
	}
}
