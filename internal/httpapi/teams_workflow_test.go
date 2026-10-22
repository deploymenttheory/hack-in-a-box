package httpapi

import (
	"bytes"
	"encoding/base64"
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
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/teamsworkflow"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestTeamsWorkflowHTTPSecretAndCurrentAuthorityBoundary(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	owner, outsider := testdb.Person(t, pool), testdb.Person(t, pool)
	eventID, connectionID := testdb.Event(t, pool, owner.ID), uuid.New()
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
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	keys, err := secretbox.Parse(
		`{"active":"synthetic","keys":{"synthetic":"` + base64.StdEncoding.EncodeToString(
			bytes.Repeat([]byte{19}, 32),
		) + `"}}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	service := &teamsworkflow.Service{Pool: pool, Keys: keys}
	service.Jobs, err = jobs.New(
		pool,
		logger,
		map[string]jobs.Handler{teamsworkflow.HandlerKind(): service},
	)
	if err != nil {
		t.Fatal(err)
	}
	// Seed encrypted synthetic configuration, never resolve or contact Microsoft.
	endpoint := "https://synthetic.environment.api.powerplatform.com/powerautomate/automations/direct/workflows/012345678901234567890123456789ab/triggers/manual/paths/invoke?api-version=1&sig=PRIVATE-ENDPOINT-CANARY"
	keyID, ciphertext, err := keys.Seal(
		[]byte(endpoint),
		[]byte("hackbox:teams-workflow:"+eventID.String()+":"+connectionID.String()+":1"),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(
		ctx,
		`INSERT INTO teams_workflow_connections(id,event_id,version,label,owner_label,co_owner_label,key_id,ciphertext,active,updated_at) VALUES($1,$2,1,'Channel','Owner','Co-owner',$3,$4,true,$5)`,
		connectionID,
		eventID,
		keyID,
		ciphertext,
		time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(cfg, logger, Dependencies{Pool: pool, Auth: auth, TeamsWorkflow: service})
	csrf := strings.Repeat("a", 32)
	request := func(person dbgen.Person, method, path string, body any, rev int64, goodCSRF bool) *httptest.ResponseRecorder {
		t.Helper()
		var reader io.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			reader = bytes.NewReader(raw)
		}
		r := httptest.NewRequest(method, path, reader)
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("If-Match", fmt.Sprintf("\"%d\"", rev))
		if goodCSRF {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		if person.ID != uuid.Nil {
			session, e := auth.Sessions.Load(ctx, "")
			if e != nil {
				t.Fatal(e)
			}
			auth.Sessions.Put(session, "person_id", person.ID.String())
			auth.Sessions.Put(session, "session_revision", person.SessionRevision)
			auth.Sessions.Put(session, "csrf", csrf)
			token, _, e := auth.Sessions.Commit(session)
			if e != nil {
				t.Fatal(e)
			}
			r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: token})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		if strings.Contains(w.Body.String(), "PRIVATE-") ||
			strings.Contains(w.Body.String(), "ciphertext") ||
			strings.Contains(w.Body.String(), "key_id") ||
			strings.Contains(w.Body.String(), "powerplatform.com") {
			t.Fatalf("secret escaped HTTP projection: %s", w.Body)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("secret administration response must not be cached")
		}
		return w
	}
	check := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("want %d got %d: %s", status, w.Code, w.Body)
		}
	}
	base := "/api/v1/events/" + eventID.String() + "/teams-workflow"
	check(request(owner, "GET", base, nil, 0, true), 200)
	check(request(outsider, "GET", base, nil, 0, true), 403)
	check(request(dbgen.Person{}, "GET", base, nil, 0, true), 401)
	event, err := dbgen.New(pool).Event(ctx, eventID)
	if err != nil {
		t.Fatal(err)
	}
	testID := uuid.New()
	body := map[string]any{"request_id": testID}
	check(request(owner, "POST", base+"/test", body, event.Revision, false), 403)
	check(request(owner, "POST", base+"/test", body, event.Revision-1, true), 409)
	check(request(outsider, "POST", base+"/test", body, event.Revision, true), 403)
	check(request(owner, "POST", base+"/test", body, event.Revision, true), 200)
	// Repeated intent is idempotent even after its original revision advances.
	check(request(owner, "POST", base+"/test", body, event.Revision, true), 200)
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM teams_workflow_deliveries WHERE event_id=$1", eventID).
		Scan(&count); err != nil ||
		count != 1 {
		t.Fatalf("queued intents %d: %v", count, err)
	}
	check(
		request(
			owner,
			"POST",
			base+"/broadcasts/"+uuid.NewString(),
			map[string]any{"version": 0},
			0,
			true,
		),
		400,
	)
	// Current role changes apply immediately to metadata and queued setup actions.
	_, err = pool.Exec(
		ctx,
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2",
		eventID,
		owner.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	check(request(owner, "GET", base, nil, 0, true), 403)
}
