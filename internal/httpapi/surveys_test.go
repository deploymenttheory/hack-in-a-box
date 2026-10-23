package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/surveys"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestSurveyHTTPPrivateResponsesNativePrivacyErasureAndCurrentAuthority(t *testing.T) {
	pool := testdb.New(t, true)
	f := newEmbargoFixture(t, pool, time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC))
	if err := database.Migrate(t.Context(), pool, 0); err != nil {
		t.Fatal(err)
	}
	f.now = f.c.LiveEnd.Add(time.Hour)
	f.exec("UPDATE events SET stage='completed',completed_at=$2 WHERE id=$1", f.id, f.c.LiveEnd)
	native := surveys.Service{Pool: pool, Now: func() time.Time { return f.now }}
	f.router = NewRouter(
		f.cfg,
		slog.New(slog.NewJSONHandler(&f.logs, nil)),
		Dependencies{
			Pool:    pool,
			Auth:    f.auth,
			Surveys: &native,
			Privacy: privacy.Service{Pool: pool, Clock: func() time.Time { return f.now }},
		},
	)
	id, question := uuid.New(), uuid.New()
	path := f.base() + "/surveys/" + id.String()
	member, peer := f.members[0][0], f.members[0][1]
	definition := surveys.Definition{
		Form: forms.Definition{
			ID:       id,
			Name:     "Post-event feedback",
			Audience: "attendee",
			Fields: []forms.Field{
				{
					ID:       question,
					Label:    "Private explanation",
					Type:     "text",
					Required: true,
					Choices:  []string{},
				},
			},
		},
		OpensAt:       f.now.Add(-time.Minute),
		ClosesAt:      f.now.Add(time.Hour),
		MinimumCohort: 5,
		Measures:      []surveys.QuestionMeasure{},
	}
	f.expect(f.request(dbgen.Person{}, "GET", path, nil, 0), 401)
	f.expect(
		f.request(
			member,
			"PUT",
			path+"/configuration",
			surveys.Command{Action: "save", Definition: &definition},
			0,
		),
		403,
	)
	f.expect(
		f.request(
			f.owner,
			"PUT",
			path+"/configuration",
			surveys.Command{Action: "save", Definition: &definition},
			0,
		),
		200,
	)
	for _, action := range []string{"approve", "publish"} {
		f.expect(
			f.request(f.owner, "PUT", path+"/configuration", surveys.Command{Action: action}, 1),
			200,
		)
	}
	f.expect(
		f.request(f.owner, "GET", path, nil, 0),
		403,
	) // staff is not silently an invited attendee
	read := f.request(member, "GET", path, nil, 0)
	f.expect(read, 200)
	var own surveys.ParticipantSurvey
	if err := json.Unmarshal(read.Body.Bytes(), &own); err != nil || own.Timezone != f.c.Timezone {
		t.Fatal("survey must use its event timezone", own.Timezone, f.c.Timezone, err)
	}
	if own.Response != nil || !strings.Contains(read.Body.String(), `"response":null`) {
		t.Fatal("unanswered survey must remain null, never a fabricated revision-zero response", read.Body.String())
	}
	if !strings.Contains(read.Header().Get("Cache-Control"), "no-store") ||
		!strings.Contains(read.Body.String(), surveys.ResponseVisibility) {
		t.Fatal("missing private response contract")
	}
	f.expect(f.request(f.stranger, "GET", path, nil, 0), 403)
	f.expect(f.request(peer, "GET", path+"/responses", nil, 0), 403)
	input := surveys.ResponseCommand{
		CommandID:   uuid.New(),
		FormVersion: 1,
		Answers: forms.Answers{
			question.String(): json.RawMessage(`"PRIVATE-SURVEY-RESPONSE-CANARY"`),
		},
	}
	// Current cookie without the CSRF header cannot submit or leak input in errors.
	req := httptest.NewRequest(
		"PUT",
		path+"/response",
		strings.NewReader(
			`{"command_id":"`+input.CommandID.String()+`","form_version":1,"answers":{"`+question.String()+`":"PRIVATE-SURVEY-RESPONSE-CANARY"}}`,
		),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", `"0"`)
	req.Header.Set("Origin", f.cfg.PublicOrigin)
	req.AddCookie(&http.Cookie{Name: "hackbox_session", Value: f.tokens[member.ID]})
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	f.expect(w, 400)
	saved := f.request(member, "PUT", path+"/response", input, 0)
	f.expect(saved, 200)
	f.expect(f.request(member, "PUT", path+"/response", input, 0), 200)
	for _, p := range []string{path, f.base() + "/surveys", path + "/report"} {
		got := f.request(peer, "GET", p, nil, 0)
		if strings.Contains(got.Body.String(), "PRIVATE-SURVEY-RESPONSE-CANARY") {
			t.Fatal("peer received another author's answer")
		}
	}
	directory := f.request(member, "GET", f.base()+"/surveys", nil, 0)
	f.expect(directory, 200)
	if strings.Contains(directory.Body.String(), "PRIVATE-SURVEY-RESPONSE-CANARY") ||
		strings.Contains(directory.Body.String(), "answers") {
		t.Fatal("directory overfetched private answers")
	}
	authored := f.request(f.owner, "GET", path+"/responses", nil, 0)
	f.expect(authored, 200)
	if !strings.Contains(authored.Body.String(), member.ID.String()) ||
		!strings.Contains(authored.Body.String(), "PRIVATE-SURVEY-RESPONSE-CANARY") {
		t.Fatal("organiser authorship missing")
	}
	f.expect(
		f.request(member, "GET", path+"/responses/"+member.ID.String()+"/history", nil, 0),
		403,
	)
	f.expect(
		f.request(f.owner, "GET", path+"/responses/"+member.ID.String()+"/history", nil, 0),
		200,
	)
	f.now = definition.ClosesAt
	f.expect(f.request(member, "PUT", path+"/response", input, 0), 403)
	closed := f.request(member, "GET", path, nil, 0)
	f.expect(closed, 200)
	if !strings.Contains(closed.Body.String(), `"can_respond":false`) {
		t.Fatal("closed reader still offers writes")
	}
	// Exercise the generated HTTP report DTO, not just the native aggregate:
	// absent measure provenance and suppressed counts must remain JSON null.
	reviewID := uuid.New()
	for _, action := range []string{"review_text", "approve", "publish"} {
		expected := int64(1)
		if action == "review_text" {
			expected = 0
		}
		f.expect(f.request(f.owner, "PUT", path+"/report/review", surveys.ReportCommand{
			Action: action, ReviewID: reviewID, ReviewedText: action == "review_text",
		}, expected), 200)
	}
	released := f.request(peer, "GET", path+"/report", nil, 0)
	f.expect(released, 200)
	var report struct {
		Respondents json.RawMessage `json:"respondents"`
		Questions   []struct {
			Measure  json.RawMessage `json:"measure"`
			Answered json.RawMessage `json:"answered"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(released.Body.Bytes(), &report); err != nil ||
		string(report.Respondents) != "null" || len(report.Questions) != 1 ||
		string(report.Questions[0].Measure) != "null" || string(report.Questions[0].Answered) != "null" {
		t.Fatal("absent measure and suppressed counts must remain null", released.Body.String(), err)
	}
	// Native privacy removal works even without an operational profile; no orphan
	// response revision/command/invitation survives as a second private store.
	f.expect(f.request(member, "DELETE", f.base()+"/profiles/"+member.ID.String(), nil, 0), 200)
	for _, table := range []string{"survey_responses", "survey_response_heads", "survey_commands", "survey_invitations"} {
		var n int
		if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM "+table+" WHERE event_id=$1 AND person_id=$2", f.id, member.ID).
			Scan(&n); err != nil ||
			n != 0 {
			t.Fatal("privacy removal left survey records", table, n, err)
		}
	}
	f.expect(f.request(member, "GET", path, nil, 0), 403)
	f.exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		f.id,
		peer.ID,
	)
	f.expect(f.request(peer, "GET", path, nil, 0), 403)
	var audit []byte
	if err := pool.QueryRow(t.Context(), "SELECT coalesce(jsonb_agg(to_jsonb(a)),'[]') FROM audit_events a WHERE event_id=$1", f.id).
		Scan(&audit); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(audit), "PRIVATE-SURVEY-RESPONSE-CANARY") ||
		strings.Contains(f.logs.String(), "PRIVATE-SURVEY-RESPONSE-CANARY") {
		t.Fatal("answer leaked in audit/logs")
	}
}
