package httpapi

import (
	"bytes"
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

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/sessionbooking"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestSessionBookingHTTPCurrentScopeReceiptsAndFieldAllowlists(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
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
	owner, member, facilitator, outsider := testdb.Person(
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
	id := testdb.Event(t, pool, owner.ID)
	session := uuid.New()
	c := event.DefaultConfiguration()
	c.Timezone = "Europe/London"
	c.LiveStart = time.Date(2027, 1, 20, 10, 0, 0, 0, time.UTC)
	c.Schedule = []event.AgendaItem{
		{
			ID:       session,
			Title:    "Synthetic HTTP session",
			Audience: []string{"attendee"},
			Anchor:   "live_start",
		},
	}
	raw, _ := json.Marshal(c)
	if _, err = pool.Exec(
		ctx,
		`UPDATE events SET configuration=$2 WHERE id=$1`,
		id,
		raw,
	); err != nil {
		t.Fatal(err)
	}
	for _, person := range []dbgen.Person{member, facilitator} {
		if err = q.SetCompetitionState(
			ctx,
			dbgen.SetCompetitionStateParams{EventID: id, PersonID: person.ID, State: "confirmed"},
		); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2027, 1, 19, 12, 0, 0, 0, time.UTC)
	service := sessionbooking.Service{Pool: pool, Now: func() time.Time { return now }}
	router := NewRouter(
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Dependencies{Pool: pool, Auth: auth, SessionBookings: service},
	)
	request := func(person dbgen.Person, method, path string, body any, revision int64, csrfOK bool) *httptest.ResponseRecorder {
		t.Helper()
		sessionCtx, err := auth.Sessions.Load(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		csrf := strings.Repeat("a", 32)
		auth.Sessions.Put(sessionCtx, "person_id", person.ID.String())
		auth.Sessions.Put(sessionCtx, "session_revision", person.SessionRevision)
		auth.Sessions.Put(sessionCtx, "csrf", csrf)
		token, _, err := auth.Sessions.Commit(sessionCtx)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("If-Match", fmt.Sprintf("\"%d\"", revision))
		if !csrfOK {
			csrf = strings.Repeat("x", 32)
		}
		r.Header.Set("X-CSRF-Token", csrf)
		r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		return w
	}
	base := "/api/v1/events/" + id.String() + "/sessions"
	path := base + "/" + session.String()
	d := sessionbooking.Definition{
		Capacity:        1,
		DurationMinutes: 60,
		Room:            "Synthetic room",
		MeetingURL:      "https://example.test/PRIVATE-MEETING",
		FacilitatorIDs:  []uuid.UUID{facilitator.ID},
		BookingOpens:    sessionbooking.LocalTime{Wall: "2027-01-18T10:00:00"},
		BookingCloses:   sessionbooking.LocalTime{Wall: "2027-01-20T09:00:00"},
	}
	if w := request(owner, "PUT", path, d, 0, false); w.Code != 403 {
		t.Fatal("missing CSRF accepted", w.Code, w.Body)
	}
	if w := request(owner, "PUT", path, d, 0, true); w.Code != 200 {
		t.Fatal("native configure", w.Code, w.Body)
	}
	if w := request(
		member,
		"GET",
		path,
		nil,
		0,
		true,
	); w.Code != 200 || strings.Contains(w.Body.String(), "PRIVATE-MEETING") || strings.Contains(w.Body.String(), "facilitator_ids") ||
		!strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("unbooked reader leaked private configuration/link", w.Code, w.Body)
	}
	command := map[string]any{"request_id": uuid.New(), "action": "reserve"}
	w := request(member, "POST", path+"/booking", command, 0, true)
	if w.Code != 200 {
		t.Fatal("native reserve", w.Code, w.Body)
	}
	ack := w.Body.String()
	if w = request(
		member,
		"POST",
		path+"/booking",
		command,
		0,
		true,
	); w.Code != 200 ||
		w.Body.String() != ack {
		t.Fatal("lost response retry changed original ACK", w.Code, w.Body)
	}
	if w = request(
		member,
		"GET",
		path,
		nil,
		0,
		true,
	); w.Code != 200 ||
		!strings.Contains(w.Body.String(), "PRIVATE-MEETING") {
		t.Fatal("booked reader denied meeting", w.Code, w.Body)
	}
	if w = request(outsider, "GET", path, nil, 0, true); w.Code != 403 {
		t.Fatal("unrelated person saw session", w.Code, w.Body)
	}
	if w = request(member, "GET", path+"/attendance", nil, 0, true); w.Code != 403 {
		t.Fatal("ordinary attendee saw roster", w.Code, w.Body)
	}
	w = request(facilitator, "GET", path+"/attendance", nil, 0, true)
	if w.Code != 200 {
		t.Fatal("assigned facilitator denied", w.Code, w.Body)
	}
	var roster struct {
		Items []map[string]any `json:"items"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &roster); err != nil || len(roster.Items) != 1 {
		t.Fatal("roster response", err)
	}
	for key := range roster.Items[0] {
		if key != "person_id" && key != "display_name" && key != "present" && key != "revision" {
			t.Fatal("private field in session roster", key)
		}
	}
	d.FacilitatorIDs = []uuid.UUID{}
	if w = request(owner, "PUT", path, d, 1, true); w.Code != 200 {
		t.Fatal("remove exact assignment", w.Code, w.Body)
	}
	if w = request(facilitator, "GET", path+"/attendance", nil, 0, true); w.Code != 403 {
		t.Fatal("former facilitator retained roster", w.Code, w.Body)
	}
	if w = request(member, "POST", path+"/cancel", nil, 2, true); w.Code != 403 {
		t.Fatal("attendee cancelled session for everyone", w.Code, w.Body)
	}
}
