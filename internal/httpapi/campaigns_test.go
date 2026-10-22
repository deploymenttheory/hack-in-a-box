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

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type campaignHTTPProvider struct{}

func (campaignHTTPProvider) Readiness(
	context.Context,
	uuid.UUID,
	uuid.UUID,
) (communications.ChannelReadiness, error) {
	return communications.ChannelReadiness{
		Available:         true,
		ConfigRevision:    "synthetic-v1",
		SelfTestAvailable: true,
	}, nil
}

func TestCampaignHTTPContractsCurrentAuthorityAndOrganiserOnlyTestIntent(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	owner, outsider, competitor := testdb.Person(
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
	q := dbgen.New(pool)
	c := event.DefaultConfiguration()
	c.LiveStart = time.Date(2029, 6, 20, 9, 0, 0, 0, time.UTC)
	c.LiveEnd = c.LiveStart.Add(8 * time.Hour)
	c.SubmissionCutoff = c.LiveEnd.Add(-time.Hour)
	raw, _ := json.Marshal(c)
	if _, err := pool.Exec(
		ctx,
		"UPDATE events SET configuration=$2 WHERE id=$1",
		eventID,
		raw,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
		eventID,
		competitor.ID,
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
	outbox, err := jobs.New(
		pool,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		map[string]jobs.Handler{},
	)
	if err != nil {
		t.Fatal(err)
	}
	service := communications.Service{
		Pool:     pool,
		Jobs:     outbox,
		Channels: map[string]communications.ChannelProvider{"smtp": campaignHTTPProvider{}},
	}
	template := communications.Template{
		ID:      uuid.New(),
		Name:    "Synthetic message",
		Purpose: "reminder",
		Format:  "html",
		Subject: "Hello {{person.name}}",
		Body:    "Literal <script>alert(1)</script> {{event.name}}",
	}
	audience := communications.Audience{
		ID:                 uuid.New(),
		Name:               "Confirmed competitors",
		Roles:              []string{"attendee"},
		RegistrationStates: []string{},
		Teams:              []uuid.UUID{},
	}
	revision := func() int64 {
		t.Helper()
		row, readErr := q.Event(ctx, eventID)
		if readErr != nil {
			t.Fatal(readErr)
		}
		return row.Revision
	}
	for _, command := range []communications.Command{{Action: "save_template", Template: &template}, {Action: "save_audience", Audience: &audience}} {
		command.Reason = "Synthetic source setup"
		if _, err = service.Change(ctx, owner.ID, eventID, revision(), command); err != nil {
			t.Fatal(err)
		}
	}
	router := NewRouter(
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Dependencies{Pool: pool, Auth: auth, Communications: service},
	)
	request := func(person dbgen.Person, method, path string, body any, rev int64, csrf bool) *httptest.ResponseRecorder {
		t.Helper()
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("If-Match", fmt.Sprintf("\"%d\"", rev))
		r.Header.Set("X-CSRF-Token", strings.Repeat("x", 32))
		if person.ID != uuid.Nil {
			session, loadErr := auth.Sessions.Load(ctx, "")
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			auth.Sessions.Put(session, "person_id", person.ID.String())
			auth.Sessions.Put(session, "session_revision", person.SessionRevision)
			auth.Sessions.Put(session, "csrf", strings.Repeat("c", 32))
			value, _, commitErr := auth.Sessions.Commit(session)
			if commitErr != nil {
				t.Fatal(commitErr)
			}
			r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: value})
		}
		if csrf {
			r.Header.Set("X-CSRF-Token", strings.Repeat("c", 32))
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		return w
	}
	base := "/api/v1/events/" + eventID.String() + "/communications/campaigns"
	for _, p := range []dbgen.Person{{}, outsider} {
		want := 403
		if p.ID == uuid.Nil {
			want = 401
		}
		if w := request(p, "GET", base, nil, 0, false); w.Code != want {
			t.Fatalf("campaign index authority: %d %s", w.Code, w.Body)
		}
	}
	d := communications.CampaignDefinition{
		ID:              uuid.New(),
		Name:            "Synthetic email campaign",
		TemplateID:      template.ID,
		AudienceID:      audience.ID,
		Channels:        []string{"smtp"},
		IncludeCalendar: true,
		Timing: communications.CampaignTiming{
			Kind:   "relative",
			Anchor: "live_start",
			Days:   -1,
		},
	}
	path := base + "/" + d.ID.String()
	command := communications.CampaignCommand{
		Action:     "save",
		Definition: &d,
		Reason:     "Review scheduled email",
	}
	if w := request(owner, "POST", path, command, revision(), false); w.Code != 403 {
		t.Fatalf("CSRF bypass: %d", w.Code)
	}
	for _, wall := range []string{"2027-03-28T01:30:00", "2027-10-31T01:30:00"} {
		invalid := d
		invalid.Timing = communications.CampaignTiming{Kind: "fixed", WallTime: wall}
		attempt := command
		attempt.Definition = &invalid
		before := revision()
		response := request(owner, "POST", path, attempt, before, true)
		if response.Code != 409 || !strings.Contains(response.Body.String(), "campaign_timing_invalid") {
			t.Fatalf("DST timing must be a reviewed validation failure: %d %s", response.Code, response.Body)
		}
		if revision() != before {
			t.Fatal("rejected DST timing changed event revision")
		}
	}
	w := request(owner, "POST", path, command, revision(), true)
	if w.Code != 200 {
		t.Fatalf("save contract: %d %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("campaign response cacheable")
	}
	var detail communications.CampaignDetail
	if err = json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	stale := detail.Revision - 1
	w = request(
		owner,
		"POST",
		path,
		communications.CampaignCommand{
			Action:  "preview",
			Version: detail.Campaign.Campaign.Version,
			Reason:  "Review exact audience",
		},
		detail.Revision,
		true,
	)
	if w.Code != 200 {
		t.Fatalf("preview contract: %d %s", w.Code, w.Body)
	}
	if err = json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Preview == nil || len(detail.Preview.Recipients) != 1 ||
		strings.Contains(detail.Preview.Recipients[0].Body, "<script>") {
		t.Fatal("preview leaked wrong audience or active markup")
	}
	if w = request(
		owner,
		"POST",
		path,
		communications.CampaignCommand{
			Action:  "approve",
			Version: detail.Campaign.Campaign.Version,
			Reason:  "Stale approval",
		},
		stale,
		true,
	); w.Code != 409 {
		t.Fatalf("stale approval accepted: %d", w.Code)
	}
	if w = request(
		owner,
		"POST",
		path,
		map[string]any{
			"action":       "test_self",
			"version":      detail.Campaign.Campaign.Version,
			"channel":      "smtp",
			"reason":       "Do not accept another target",
			"recipient_id": competitor.ID,
		},
		detail.Revision,
		true,
	); w.Code != 400 {
		t.Fatalf("arbitrary recipient accepted: %d", w.Code)
	}
	w = request(
		owner,
		"POST",
		path,
		communications.CampaignCommand{
			Action:  "test_self",
			Version: detail.Campaign.Campaign.Version,
			Channel: "smtp",
			Reason:  "Explicit self test",
		},
		detail.Revision,
		true,
	)
	if w.Code != 200 {
		t.Fatalf("self-test contract: %d %s", w.Code, w.Body)
	}
	if err = json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.TestRequestID == nil {
		t.Fatal("missing typed queued-test reference")
	}
	row, err := q.CommunicationTestRequest(
		ctx,
		dbgen.CommunicationTestRequestParams{EventID: eventID, ID: *detail.TestRequestID},
	)
	if err != nil {
		t.Fatal(err)
	}
	if row.ActorID != owner.ID {
		t.Fatal("self test targeted another person")
	}
	if _, err = pool.Exec(
		ctx,
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='organiser'",
		eventID,
		owner.ID,
	); err != nil {
		t.Fatal(err)
	}
	if w = request(owner, "GET", path, nil, 0, false); w.Code != 403 {
		t.Fatalf("revoked campaign access persisted: %d", w.Code)
	}
}
