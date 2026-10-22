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
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/inbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestInboxHTTPRecipientIsolationCurrentResourceCSRFAndNoStore(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	owner := testdb.Person(t, pool)
	recipient := testdb.Person(t, pool)
	other := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	resource := uuid.New()
	for _, person := range []dbgen.Person{recipient, other} {
		if _, err := pool.Exec(
			ctx,
			"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed')",
			eventID,
			person.ID,
		); err != nil {
			t.Fatal(err)
		}
	}
	_, err := pool.Exec(
		ctx,
		"INSERT INTO content_items(event_id,id,kind,revision,version,state) VALUES($1,$2,'page',1,1,'draft')",
		eventID,
		resource,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(
		ctx,
		"INSERT INTO content_versions(event_id,content_id,version,document,actor_id,created_at) VALUES($1,$2,1,$3,$4,now())",
		eventID,
		resource,
		[]byte(
			`{"title":"SECRET-SOURCE-TITLE","markdown":"Synthetic body","audience":{"confirmed":true,"roles":[],"teams":[]}}`,
		),
		owner.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(
		ctx,
		"UPDATE content_items SET state='released',released_version=1,released_at=$3 WHERE event_id=$1 AND id=$2",
		eventID,
		resource,
		time.Now().Add(-time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}
	service := inbox.Service{Pool: pool}
	if err = database.Within(ctx, pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		return service.RecordResourceInTransaction(
			ctx,
			tx,
			eventID,
			recipient.ID,
			inbox.Source{Kind: "content", ID: resource, Version: 1},
		)
	}); err != nil {
		t.Fatal(err)
	}
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
	router := NewRouter(
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Dependencies{Pool: pool, Auth: auth, Inbox: service},
	)
	request := func(person dbgen.Person, method, path string, body any, csrf bool) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", cfg.PublicOrigin)
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
	if w := request(dbgen.Person{}, "GET", "/api/v1/inbox", nil, false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	w := request(recipient, "GET", "/api/v1/inbox", nil, false)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") ||
		strings.Contains(w.Body.String(), "SECRET") {
		t.Fatalf("private list: %d %s", w.Code, w.Body)
	}
	var page inbox.Page
	if err = json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Items) != 1 {
		t.Fatalf("list: %+v %v", page, err)
	}
	path := "/api/v1/inbox/" + page.Items[0].ID.String()
	if w = request(other, "GET", path, nil, false); w.Code != 404 {
		t.Fatalf("other recipient open: %d", w.Code)
	}
	if w = request(
		recipient,
		"POST",
		path+"/read",
		map[string]bool{"read": true},
		false,
	); w.Code != 403 {
		t.Fatalf("CSRF: %d", w.Code)
	}
	if w = request(
		recipient,
		"POST",
		path+"/read",
		map[string]bool{"read": true},
		true,
	); w.Code != 200 {
		t.Fatalf("own read: %d %s", w.Code, w.Body)
	}
	if _, err = pool.Exec(
		ctx,
		"UPDATE content_items SET state='withdrawn' WHERE event_id=$1 AND id=$2",
		eventID,
		resource,
	); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"GET", "POST"} {
		target := path
		var body any
		if method == "POST" {
			target += "/read"
			body = map[string]bool{"read": false}
		}
		if w = request(recipient, method, target, body, true); w.Code != 404 {
			t.Fatalf("revoked %s: %d %s", method, w.Code, w.Body)
		}
	}
	w = request(recipient, "GET", "/api/v1/inbox", nil, false)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	if err = json.Unmarshal(
		w.Body.Bytes(),
		&page,
	); err != nil || len(page.Items) != 0 ||
		page.Unread != 0 {
		t.Fatalf("revoked counts: %+v %v", page, err)
	}
}
