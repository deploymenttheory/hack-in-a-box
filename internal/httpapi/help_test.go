package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestHelpHTTPCurrentTeamContextAndLiveReviewGate(t *testing.T) {
	pool := testdb.New(t, true)

	f := newEmbargoFixture(t, pool, time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC))
	service := help.Service{Pool: pool, Clock: func() time.Time { return f.now }}
	f.router = NewRouter(
		f.cfg,
		slog.New(slog.NewJSONHandler(&f.logs, nil)),
		Dependencies{Pool: pool, Auth: f.auth, Help: service},
	)
	member, other := f.members[0][0], f.members[1][0]
	id := uuid.New()
	path := f.base() + "/help/" + id.String()
	input := map[string]any{
		"team_id":     f.teams[0],
		"category":    "setup",
		"description": f.prefix + " PRIVATE PROBLEM",
		"location":    f.prefix + " PRIVATE LOCATION",
		"meeting_url": "",
	}
	f.expect(f.request(dbgen.Person{}, "GET", path, nil, 0), 401)
	f.expect(f.request(other, "PUT", path, input, 0), 404)
	created := f.request(member, "PUT", path, input, 0)
	f.expect(created, 200)
	var page help.Page
	if err := json.Unmarshal(created.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Request.Category != "setup" || page.Request.Status != "waiting" ||
		page.Request.RequesterID != member.ID {
		t.Fatalf("created help state: %+v", page)
	}
	badCSRF := httptest.NewRequest(
		"POST",
		path+"/actions",
		bytes.NewBufferString(`{"action":"cancel","assignee_id":null}`),
	)
	badCSRF.Header.Set("Content-Type", "application/json")
	badCSRF.Header.Set("Origin", f.cfg.PublicOrigin)
	badCSRF.Header.Set("If-Match", `"1"`)
	badCSRF.Header.Set("Idempotency-Key", uuid.NewString())
	badCSRF.Header.Set("X-CSRF-Token", strings.Repeat("x", 32))
	badCSRF.AddCookie(&http.Cookie{Name: "hackbox_session", Value: f.tokens[member.ID]})
	denied := httptest.NewRecorder()
	f.router.ServeHTTP(denied, badCSRF)
	f.expect(denied, 403)
	f.expect(
		f.request(
			f.owner,
			"POST",
			path+"/actions",
			map[string]any{"action": "claim", "assignee_id": nil},
			page.Request.Revision,
		),
		200,
	)
	f.denied(f.request(other, "GET", path, nil, 0), 404)
	f.denied(f.request(f.mentor, "GET", path, nil, 0), 404)
	ownQueue := f.request(member, "GET", f.base()+"/help", nil, 0)
	f.expect(ownQueue, 200)
	if strings.Contains(ownQueue.Body.String(), "PRIVATE") {
		t.Fatal("queue contains private problem/location")
	}
	otherQueue := f.request(other, "GET", f.base()+"/help", nil, 0)
	f.expect(otherQueue, 200)
	var queue help.Queue
	if err := json.Unmarshal(
		otherQueue.Body.Bytes(),
		&queue,
	); err != nil || len(queue.Items) != 0 || queue.Waiting != 0 ||
		queue.Claimed != 0 {
		t.Fatal("hidden help counts exposed")
	}
	settings := f.request(f.owner, "GET", f.base()+"/help-settings", nil, 0)
	f.expect(settings, 200)
	f.expect(f.request(member, "GET", f.base()+"/help-settings", nil, 0), 403)
	f.exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		f.id,
		member.ID,
	)
	f.denied(f.request(member, "GET", path, nil, 0), 404)
	f.denied(
		f.request(
			member,
			"POST",
			path+"/actions",
			map[string]any{"action": "cancel", "assignee_id": nil},
			2,
		),
		404,
	)
	input["team_id"] = f.teams[1]
	input["category"] = "competition_output"
	f.expect(f.request(other, "PUT", f.base()+"/help/"+uuid.NewString(), input, 0), 409)
	if strings.Contains(f.logs.String(), "PRIVATE") {
		t.Fatal("private help prose/location logged")
	}
}

func TestHelpHTTPContextAndTransferProjectionStayCurrentActorScoped(t *testing.T) {
	pool := testdb.New(t, true)
	f := newEmbargoFixture(t, pool, time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC))
	service := help.Service{Pool: pool, Clock: func() time.Time { return f.now }}
	f.router = NewRouter(
		f.cfg,
		slog.New(slog.NewJSONHandler(&f.logs, nil)),
		Dependencies{Pool: pool, Auth: f.auth, Help: service},
	)
	id := uuid.New()
	path := f.base() + "/help/" + id.String()
	f.expect(
		f.request(
			f.members[0][0],
			"PUT",
			path,
			map[string]any{
				"team_id":     f.teams[0],
				"category":    "setup",
				"description": "PRIVATE HTTP HELP",
				"location":    "PRIVATE HTTP LOCATION",
				"meeting_url": "",
			},
			0,
		),
		200,
	)
	claimed := f.request(
		f.owner,
		"POST",
		path+"/actions",
		map[string]any{"action": "claim", "assignee_id": nil},
		1,
	)
	f.expect(claimed, 200)
	var owner help.Page
	if err := json.Unmarshal(claimed.Body.Bytes(), &owner); err != nil {
		t.Fatal(err)
	}
	if !owner.CanTransfer || owner.AssigneeName != f.owner.DisplayName ||
		owner.TransferTargets == nil {
		t.Fatal("missing authorised owner projection")
	}
	current := f.request(f.members[0][0], "GET", path, nil, 0)
	f.expect(current, 200)
	var member help.Page
	if err := json.Unmarshal(current.Body.Bytes(), &member); err != nil {
		t.Fatal(err)
	}
	if member.CanTransfer || len(member.TransferTargets) != 0 ||
		member.AssigneeName != f.owner.DisplayName {
		t.Fatal("requester received a transfer directory")
	}
	queue := f.request(f.members[0][0], "GET", f.base()+"/help", nil, 0)
	f.expect(queue, 200)
	var own help.Queue
	if err := json.Unmarshal(queue.Body.Bytes(), &own); err != nil {
		t.Fatal(err)
	}
	if !own.CanRequest || own.RequestTeam == nil || own.RequestTeam.ID != f.teams[0] {
		t.Fatal("incorrect own request-team projection")
	}
	organiserQueue := f.request(f.owner, "GET", f.base()+"/help", nil, 0)
	f.expect(organiserQueue, 200)
	var staff help.Queue
	if err := json.Unmarshal(organiserQueue.Body.Bytes(), &staff); err != nil {
		t.Fatal(err)
	}
	if staff.CanRequest || staff.RequestTeam != nil {
		t.Fatal("organiser received a participant team directory")
	}
	if strings.Contains(f.logs.String(), "PRIVATE HTTP") {
		t.Fatal("private request context logged")
	}
}
