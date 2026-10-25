package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestJudgeHTTPIsolatedPracticeStrictContractAndCurrentAuthority(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	owner, judge, other := testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	q := dbgen.New(pool)
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'judge')",
		eventID,
		judge.ID,
	); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2027, 6, 10, 10, 0, 0, 0, time.UTC)
	dates := event.DefaultConfiguration()
	dates.LiveStart = now.Add(24 * time.Hour)
	dates.LiveEnd = dates.LiveStart.Add(8 * time.Hour)
	dates.SubmissionCutoff = dates.LiveStart.Add(6 * time.Hour)
	raw, _ := json.Marshal(dates)
	if _, err := pool.Exec(
		ctx,
		"UPDATE events SET configuration=$2,stage='preparation' WHERE id=$1",
		eventID,
		raw,
	); err != nil {
		t.Fatal(err)
	}
	rs := rubric.Service{Pool: pool, Clock: func() time.Time { return now }}
	definition := rubric.Default(rubric.AllTeams, eventID)
	definition.PrivateJudgeGuidance = "PRIVATE-GUIDANCE-NOT-IN-PRACTICE"
	saved, err := rs.Change(
		ctx,
		owner.ID,
		eventID,
		0,
		rubric.Command{
			Action:     "save",
			Definition: &definition,
			Reason:     "Synthetic practice rubric",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = rs.Change(
		ctx,
		owner.ID,
		eventID,
		saved.Revision,
		rubric.Command{
			Action:  "approve",
			Version: saved.Reference.Version,
			Reason:  "Synthetic practice approval",
		},
	); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(func(k string) string {
		if k == "HACKBOX_ENV" {
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
	router := NewRouter(
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Dependencies{
			Pool:    pool,
			Auth:    auth,
			Judging: judging.Service{Pool: pool, Now: func() time.Time { return now }},
		},
	)
	request := func(person dbgen.Person, method, path, version string, body any, validCSRF bool) *httptest.ResponseRecorder {
		t.Helper()
		session, err := auth.Sessions.Load(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		csrf := strings.Repeat("c", 32)
		auth.Sessions.Put(session, "person_id", person.ID.String())
		auth.Sessions.Put(session, "session_revision", person.SessionRevision)
		auth.Sessions.Put(session, "csrf", csrf)
		token, _, err := auth.Sessions.Commit(session)
		if err != nil {
			t.Fatal(err)
		}
		payload, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(payload))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("If-Match", version)
		if !validCSRF {
			csrf = "wrong"
		}
		r.Header.Set("X-CSRF-Token", csrf)
		r.AddCookie(&http.Cookie{Name: auth.Sessions.Cookie.Name, Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		return w
	}
	path := "/api/v1/events/" + eventID.String() + "/judge-practice"
	if w := request(other, "GET", path, "\"0\"", nil, true); w.Code != 403 {
		t.Fatal("unrelated actor practice", w.Code, w.Body)
	}
	w := request(judge, "GET", path, "\"0\"", nil, true)
	if w.Code != 200 || strings.Contains(w.Body.String(), "PRIVATE-") {
		t.Fatal("practice projection", w.Code, w.Body)
	}
	var p judging.Practice
	if err = json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	scores := []judging.Score{}
	for _, category := range p.Criteria.Categories {
		value := 5
		scores = append(
			scores,
			judging.Score{
				CategoryID: category.ID,
				Value:      &value,
				Evidence:   []judging.Evidence{{Note: "Fictional robot demonstration"}},
			},
		)
	}
	command := map[string]any{
		"request_id": uuid.New(),
		"action":     "submit",
		"scores":     scores,
		"team_id":    uuid.New(),
	}
	if w = request(judge, "POST", path, "\"0\"", command, true); w.Code != 400 {
		t.Fatal("practice accepted real target field", w.Code, w.Body)
	}
	delete(command, "team_id")
	if w = request(judge, "POST", path, "\"0\"", command, false); w.Code != 403 {
		t.Fatal("CSRF bypass", w.Code, w.Body)
	}
	w = request(judge, "POST", path, "\"0\"", command, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	var original judging.Receipt
	if err = json.Unmarshal(w.Body.Bytes(), &original); err != nil {
		t.Fatal(err)
	}
	w = request(judge, "POST", path, "\"0\"", command, true)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(original.Fingerprint)) {
		t.Fatal("exact receipt replay", w.Code, w.Body)
	}
	var production int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM judge_cards WHERE event_id=$1", eventID).
		Scan(&production); err != nil ||
		production != 0 {
		t.Fatal("practice entered production cards", production, err)
	}
	receiptPath := "/api/v1/events/" + eventID.String() + "/judging-receipts/" + original.RequestID.String()
	if w = request(other, "GET", receiptPath, "\"0\"", nil, true); w.Code != 403 {
		t.Fatal("receipt actor privacy", w.Code, w.Body)
	}
	if _, err = pool.Exec(
		ctx,
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2",
		eventID,
		judge.ID,
	); err != nil {
		t.Fatal(err)
	}
	if w = request(judge, "GET", path, "\"0\"", nil, true); w.Code != 403 {
		t.Fatal("role loss kept practice", w.Code, w.Body)
	}
	summaryPath := "/api/v1/events/" + eventID.String() + "/teams/" + uuid.New().
		String() +
		"/judge-summary"
	if w = request(other, "GET", summaryPath, "\"0\"", nil, true); w.Code != 403 {
		t.Fatal("unassigned actor factual draft authority", w.Code, w.Body)
	}
	if w = request(
		other,
		"POST",
		summaryPath,
		"\"0\"",
		map[string]any{
			"request_id":          uuid.New(),
			"action":              "save",
			"submission_revision": 1,
			"reason":              "Judge-facing facts",
			"qualification_total": 50,
		},
		true,
	); w.Code != 400 {
		t.Fatal("factual briefing admitted qualification field", w.Code, w.Body)
	}
	_ = q
}
