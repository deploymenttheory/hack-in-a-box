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

	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/inbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

// This is an authenticated HTTP/database contract test with synthetic identities,
// not an OIDC browser journey or evidence of reminder transport delivery.
func TestCutoverOutcomeReviewHTTPCommandsAndCurrentReader(t *testing.T) {
	pool := testdb.New(t, true)
	f := newEmbargoFixture(t, pool, time.Now().UTC().AddDate(0, 0, 20))
	if err := database.Migrate(t.Context(), pool, 0); err != nil {
		t.Fatal(err)
	}
	composer := &communications.Service{Pool: pool, Now: func() time.Time { return f.now }}
	mailbox := &inbox.Service{Pool: pool, Communications: composer, Now: func() time.Time { return f.now }}
	composer.Channels = map[string]communications.ChannelProvider{"in_app": mailbox}
	native := &outcomes.Service{Pool: pool, Composer: composer, Now: func() time.Time { return f.now }}
	f.router = NewRouter(f.cfg, slog.New(slog.NewJSONHandler(&f.logs, nil)), Dependencies{Pool: pool, Auth: f.auth, Outcomes: native})
	request := func(actor dbgen.Person, method, path string, body any, revision int64, status int) *httptest.ResponseRecorder {
		t.Helper()
		w := f.request(actor, method, path, body, revision)
		f.expect(w, status)
		validateResponse(t, httptest.NewRequest(method, path, nil), w)
		return w
	}
	outcomeID, reviewID := uuid.New(), uuid.New()
	path := "/api/v1/outcomes/" + outcomeID.String()
	plans := path + "/reviews"
	change := plans + "/" + reviewID.String()
	read := "/api/v1/outcome-reviews/" + reviewID.String()
	owner := f.stranger // No membership in the source event.
	commands := []struct {
		actor   dbgen.Person
		command outcomes.Command
	}{
		{owner, outcomes.Command{ID: uuid.New(), Action: "propose", Reason: "Synthetic HTTP review contract", Definition: &outcomes.Definition{Kind: "pitch", Problem: "Restricted review source", Audience: "Synthetic people", ProposedValue: "Proposed benefit"}}},
		{f.owner, outcomes.Command{ID: uuid.New(), Action: "adopt", Reason: "Explicit event adoption", Provenance: &outcomes.Provenance{EventID: &f.id}}},
		{f.owner, outcomes.Command{ID: uuid.New(), Action: "assign", Reason: "Assign ordinary follow-through owner", OwnerID: &owner.ID}},
		{f.owner, outcomes.Command{ID: uuid.New(), Action: "moderate", Reason: "Accept follow-through", Status: "accepted"}},
	}
	for i, c := range commands {
		request(c.actor, "PUT", path, c.command, int64(i), 200)
	}
	schedule := outcomes.ReviewSchedule{Days: 30, WallTime: f.now.AddDate(0, 0, 30).Format("2006-01-02T15:04:05"), Timezone: "UTC", Channels: []string{"in_app"}}
	message := outcomes.ReviewMessage{Subject: "PRIVATE organiser composition", Body: "Hello {{recipient.name}}. Review {{outcome.url}} at {{review.date}}."}
	save := outcomes.PlanCommand{ID: uuid.New(), Action: "save", SourceVersion: 4, Schedule: &schedule, Message: &message, Reason: "PRIVATE organiser review reason"}
	request(owner, "PUT", change, save, 0, 403)
	request(dbgen.Person{}, "PUT", change, save, 0, 401)
	// Missing CSRF is rejected at the real adapter, without creating a plan.
	raw, err := json.Marshal(save)
	if err != nil {
		t.Fatal(err)
	}
	noCSRF := httptest.NewRequest("PUT", change, bytes.NewReader(raw))
	noCSRF.Header.Set("Content-Type", "application/json")
	noCSRF.Header.Set("Origin", f.cfg.PublicOrigin)
	noCSRF.Header.Set("If-Match", `"0"`)
	noCSRF.AddCookie(&http.Cookie{Name: "hackbox_session", Value: f.tokens[f.owner.ID]})
	denied := httptest.NewRecorder()
	f.router.ServeHTTP(denied, noCSRF)
	f.expect(denied, 400)
	validateResponse(t, noCSRF, denied)
	badCSRF := httptest.NewRequest("PUT", change, bytes.NewReader(raw))
	badCSRF.Header = noCSRF.Header.Clone()
	badCSRF.Header.Set("X-CSRF-Token", strings.Repeat("x", 32))
	denied = httptest.NewRecorder()
	f.router.ServeHTTP(denied, badCSRF)
	f.expect(denied, 403)
	validateResponse(t, badCSRF, denied)
	decode := func(w *httptest.ResponseRecorder) outcomes.PlanView {
		t.Helper()
		var v outcomes.PlanView
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	assertPlan := func(v outcomes.PlanView, version int64, state string) {
		t.Helper()
		if v.ID != reviewID || v.OutcomeID != outcomeID || v.OwnerID != owner.ID || v.SourceVersion != 4 || v.Version != version || v.CurrentVersion != version || v.State != state || v.EffectiveState != state {
			t.Fatalf("wrong review identity/version/state: %+v", v)
		}
	}
	draft := decode(request(f.owner, "PUT", change, save, 0, 200))
	assertPlan(draft, 1, "draft")
	if draft.ActorID != f.owner.ID || draft.CommandID != save.ID || draft.Message != message {
		t.Fatal("draft lost actor, command or reviewed composition", draft)
	}
	approve := outcomes.PlanCommand{ID: uuid.New(), Action: "approve", SourceVersion: 4, Reason: "Explicit exact reminder approval"}
	request(f.owner, "PUT", change, approve, 0, 409)
	staleSource := approve
	staleSource.ID, staleSource.SourceVersion = uuid.New(), 3
	request(f.owner, "PUT", change, staleSource, 1, 409)
	approved := decode(request(f.owner, "PUT", change, approve, 1, 200))
	assertPlan(approved, 2, "approved")
	if approved.ActorID != f.owner.ID || approved.CommandID != approve.ID {
		t.Fatal("approval actor/command mismatch", approved)
	}
	// Exact replay recovers the same immutable command; it creates no new version.
	replay := decode(request(f.owner, "PUT", change, approve, 1, 200))
	assertPlan(replay, 2, "approved")
	receipt := decode(request(f.owner, "GET", read+"?command_id="+approve.ID.String(), nil, 0, 200))
	if receipt.Version != 2 || receipt.CommandID != approve.ID {
		t.Fatal("wrong exact approval receipt", receipt)
	}
	request(owner, "GET", read+"?command_id="+approve.ID.String(), nil, 0, 404)
	request(f.owner, "GET", read+"?version=2&command_id="+approve.ID.String(), nil, 0, 400)
	assertPrivateReader := func(w *httptest.ResponseRecorder) {
		t.Helper()
		if strings.Contains(w.Body.String(), "PRIVATE") || strings.Contains(w.Body.String(), f.owner.ID.String()) {
			t.Fatal("organiser composition or actor leaked to ordinary owner", w.Body.String())
		}
	}
	current := request(owner, "GET", read, nil, 0, 200)
	assertPlan(decode(current), 2, "approved")
	assertPrivateReader(current)
	pageResponse := request(owner, "GET", plans, nil, 0, 200)
	assertPrivateReader(pageResponse)
	var page outcomes.PlanPage
	if err := json.Unmarshal(pageResponse.Body.Bytes(), &page); err != nil || len(page.Items) != 1 {
		t.Fatal("wrong owner plan list", page, err)
	}
	assertPlan(page.Items[0], 2, "approved")
	request(owner, "GET", f.base(), nil, 0, 403)
	// An organiser of another event has no authority over this review resource.
	other := testdb.Person(t, pool)
	otherEvent := testdb.Event(t, pool, other.ID)
	request(other, "GET", read, nil, 0, 404)
	request(other, "PUT", change, approve, 2, 404)
	wrongOutcome := uuid.New()
	proposal := commands[0].command
	proposal.ID = uuid.New()
	request(owner, "PUT", "/api/v1/outcomes/"+wrongOutcome.String(), proposal, 0, 200)
	adopt := commands[1].command
	adopt.ID = uuid.New()
	adopt.Provenance = &outcomes.Provenance{EventID: &otherEvent}
	request(other, "PUT", "/api/v1/outcomes/"+wrongOutcome.String(), adopt, 1, 200)
	request(other, "PUT", "/api/v1/outcomes/"+wrongOutcome.String()+"/reviews/"+reviewID.String(), approve, 2, 404)
	f.exec("INSERT INTO event_memberships(event_id,person_id,role,active) VALUES($1,$2,'organiser',true)", f.id, other.ID)
	request(other, "GET", read, nil, 0, 200)
	f.exec("UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2", f.id, other.ID)
	request(other, "GET", read, nil, 0, 404)
	cancel := outcomes.PlanCommand{ID: uuid.New(), Action: "cancel", SourceVersion: 4, Reason: "Cancel exact optional reminder"}
	request(owner, "PUT", change, cancel, 2, 403)
	assertPlan(decode(request(f.owner, "PUT", change, cancel, 2, 200)), 3, "cancelled")
	assertPlan(decode(request(owner, "GET", read, nil, 0, 200)), 3, "cancelled")
	historical := decode(request(owner, "GET", read+"?version=2", nil, 0, 200))
	if historical.Version != 2 || historical.CurrentVersion != 3 || historical.State != "approved" {
		t.Fatal("historical reminder lost immutable version", historical)
	}
	var versions int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM outcome_review_versions WHERE review_id=$1", reviewID).Scan(&versions); err != nil || versions != 3 {
		t.Fatal("denials or replay appended extra review versions", versions, err)
	}
	// Same session cookie must observe account revocation at the next reader call.
	f.exec("UPDATE people SET active=false WHERE id=$1", owner.ID)
	request(owner, "GET", read, nil, 0, 401)
	if strings.Contains(f.logs.String(), "PRIVATE") {
		t.Fatal("private review composition in logs")
	}
}
