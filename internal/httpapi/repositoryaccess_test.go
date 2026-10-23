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

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/repositoryaccess"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

type accessHTTPProvider struct{}

func (accessHTTPProvider) InspectCollaborators(
	context.Context,
	*dbgen.Queries,
	uuid.UUID,
	githubapp.AccessRequest,
) (githubapp.AccessSnapshot, error) {
	return githubapp.AccessSnapshot{
		Collaborators: []githubapp.Collaborator{
			{ID: 9999, Login: "OTHER-COLLABORATOR-PRIVATE", Permission: "admin"},
		},
		Invitations: []githubapp.Invitation{},
	}, nil
}

func (accessHTTPProvider) ChangeCollaborator(
	context.Context,
	*dbgen.Queries,
	uuid.UUID,
	githubapp.AccessRequest,
	githubapp.AccessChange,
) (githubapp.AccessResult, error) {
	panic("HTTP review commands must not dispatch provider mutations")
}

func TestRepositoryAccessHTTPPrivacyCurrentAuthorityAndExactReview(t *testing.T) {
	pool := testdb.New(t, true)
	owner, member, other := testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool)
	eventID, team := testdb.Event(t, pool, owner.ID), uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(t.Context(), query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(
		"INSERT INTO hack_teams(id,event_id,name,state)VALUES($1,$2,'Synthetic team','published')",
		team,
		eventID,
	)
	exec("INSERT INTO competition_entries(event_id,team_id)VALUES($1,$2)", eventID, team)
	exec(
		"INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')",
		eventID,
		member.ID,
	)
	exec(
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot)VALUES($1,$2,$3,'published')",
		eventID,
		team,
		member.ID,
	)
	exec(
		"INSERT INTO github_accounts(person_id,github_id,login,token_encrypted)VALUES($1,101,'own-current',decode('00','hex'))",
		member.ID,
	)
	exec("INSERT INTO github_link_states(person_id,revision)VALUES($1,3)", member.ID)
	exec(
		"INSERT INTO github_app_integrations(event_id,app_id,app_slug,organization_id,organization,template_repository,credential_version,key_id,private_key_encrypted,installation_id)VALUES($1,42,'synthetic-app',99,'synthetic-org','template',1,'private-key-canary',decode('00','hex'),55)",
		eventID,
	)
	exec(
		"INSERT INTO team_repositories(event_id,team_id,organization_id,organization,name,app_id,installation_id,template_repository,requested_by,github_repository_id,state)VALUES($1,$2,99,'synthetic-org','synthetic-repo',42,55,'template',$3,800,'ready')",
		eventID,
		team,
		owner.ID,
	)
	cfg, err := config.Load(func(key string) string {
		if key == "HACKBOX_ENV" {
			return "test"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	auth, err := identity.New(t.Context(), pool, identity.Settings{Origin: cfg.PublicOrigin})
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Dependencies{
			Pool:             pool,
			Auth:             auth,
			RepositoryAccess: repositoryaccess.Service{Pool: pool, App: accessHTTPProvider{}},
		},
	)
	csrf := strings.Repeat("a", 32)
	tokens := map[uuid.UUID]string{}
	base := "/api/v1/events/" + eventID.String() + "/repositories/" + team.String()
	request := func(person dbgen.Person, method, path string, body any, validCSRF bool, revision int64) *httptest.ResponseRecorder {
		t.Helper()
		var encoded []byte
		if body != nil {
			encoded, _ = json.Marshal(body)
		}
		r := httptest.NewRequest(method, base+path, bytes.NewReader(encoded))
		if body != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("If-Match", fmt.Sprintf("\"%d\"", revision))
		if validCSRF {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		if tokens[person.ID] == "" {
			ctx, loadErr := auth.Sessions.Load(t.Context(), "")
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			auth.Sessions.Put(ctx, "person_id", person.ID.String())
			auth.Sessions.Put(ctx, "session_revision", person.SessionRevision)
			auth.Sessions.Put(ctx, "csrf", csrf)
			token, _, commitErr := auth.Sessions.Commit(ctx)
			if commitErr != nil {
				t.Fatal(commitErr)
			}
			tokens[person.ID] = token
		}
		r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: tokens[person.ID]})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		return w
	}
	check := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("got %d want %d: %s", w.Code, status, w.Body.String())
		}
		if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("private response cacheable")
		}
		for _, canary := range []string{"private-key-canary", "token_encrypted", "OTHER-COLLABORATOR-PRIVATE"} {
			if strings.Contains(w.Body.String(), canary) {
				t.Fatal("private provider detail exposed")
			}
		}
	}
	event, _ := dbgen.New(pool).Event(t.Context(), eventID)
	planID := uuid.New()
	body := map[string]any{
		"action":       "review",
		"plan_id":      planID,
		"plan_version": 0,
		"reason":       "Review current membership",
	}
	check(request(owner, "POST", "/access", body, false, event.Revision), 400)
	check(request(other, "POST", "/access", body, true, event.Revision), 403)
	check(request(member, "GET", "/access", nil, false, event.Revision), 403)
	check(request(owner, "POST", "/access", body, true, event.Revision+1), 409)
	w := request(owner, "POST", "/access", body, true, event.Revision)
	check(w, 200)
	var first repositoryaccess.Result
	if err = json.Unmarshal(w.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if first.Receipt == nil || first.Receipt.ID != planID || first.Current.GrantsOpen {
		t.Fatal("review receipt or publication state wrong", first)
	}
	secondID := uuid.New()
	body["plan_id"] = secondID
	check(request(owner, "POST", "/access", body, true, event.Revision), 200)
	body["plan_id"] = planID
	w = request(owner, "POST", "/access", body, true, event.Revision)
	check(w, 200)
	var repeated repositoryaccess.Result
	if err = json.Unmarshal(w.Body.Bytes(), &repeated); err != nil {
		t.Fatal(err)
	}
	if repeated.Receipt.ID != planID || repeated.Current.Plan.ID != secondID {
		t.Fatal("exact receipt conflated with latest plan", repeated)
	}
	body["action"] = "approve"
	body["plan_version"] = first.Receipt.Version
	check(request(owner, "POST", "/access", body, true, event.Revision), 409)
	check(request(member, "GET", "/invitations", nil, false, event.Revision), 403)
	exec("UPDATE events SET published_at=now() WHERE id=$1", eventID)
	w = request(member, "GET", "/invitations", nil, false, event.Revision)
	check(w, 200)
	if strings.Contains(w.Body.String(), "9999") ||
		strings.Contains(w.Body.String(), "fingerprint") ||
		strings.Contains(w.Body.String(), owner.ID.String()) {
		t.Fatal("other collaborator or organiser plan exposed")
	}
	check(request(other, "GET", "/invitations", nil, false, event.Revision), 403)
	exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		eventID,
		member.ID,
	)
	check(request(member, "GET", "/invitations", nil, false, event.Revision), 403)
	exec("UPDATE events SET writes_allowed=false WHERE id=$1", eventID)
	body["action"] = "review"
	check(request(owner, "POST", "/access", body, true, event.Revision), 403)
	check(request(owner, "GET", "/access", nil, false, event.Revision), 200)
}
