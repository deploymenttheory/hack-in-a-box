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

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestContentHTTPContractAndProtectedAttachments(t *testing.T) {
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
	owner, member, stranger := testdb.Person(
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
	if err = q.SetCompetitionState(
		ctx,
		dbgen.SetCompetitionStateParams{EventID: eventID, PersonID: member.ID, State: "confirmed"},
	); err != nil {
		t.Fatal(err)
	}
	store, err := files.OpenLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	router := NewRouter(
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Dependencies{
			Pool:    pool,
			Auth:    auth,
			Content: content.Service{Pool: pool},
			Files: files.Service{
				Pool:       pool,
				Storage:    store,
				Policy:     access.EventPolicy{Queries: q},
				MaxBytes:   1000,
				EventQuota: 10000,
			},
		},
	)
	request := func(person dbgen.Person, method, path string, body any, revision int64, csrfOK bool) *httptest.ResponseRecorder {
		t.Helper()
		session, err := auth.Sessions.Load(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		csrf := strings.Repeat("a", 32)
		auth.Sessions.Put(session, "person_id", person.ID.String())
		auth.Sessions.Put(session, "session_revision", person.SessionRevision)
		auth.Sessions.Put(session, "csrf", csrf)
		token, _, err := auth.Sessions.Commit(session)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("If-Match", fmt.Sprintf("\"%d\"", revision))
		r.Header.Set("Idempotency-Key", uuid.NewString())
		if !csrfOK {
			csrf = strings.Repeat("b", 32)
		}
		r.Header.Set("X-CSRF-Token", csrf)
		r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		return w
	}
	check := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("want %d got %d: %s", status, w.Code, w.Body)
		}
	}
	decode := func(w *httptest.ResponseRecorder) content.Page {
		t.Helper()
		check(w, 200)
		var p content.Page
		if err = json.Unmarshal(w.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	base := "/api/v1/events/" + eventID.String()
	id := uuid.New()
	path := base + "/content/" + id.String()
	row, err := q.Event(ctx, eventID)
	if err != nil {
		t.Fatal(err)
	}
	uploaded := request(
		owner,
		"POST",
		base+"/files",
		map[string]any{
			"filename":   "notes.txt",
			"media_type": "text/plain",
			"content":    []byte("Synthetic content"),
		},
		row.Revision,
		true,
	)
	check(uploaded, 201)
	var file struct {
		ID uuid.UUID `json:"id"`
	}
	if err = json.Unmarshal(uploaded.Body.Bytes(), &file); err != nil {
		t.Fatal(err)
	}
	filePath := base + "/files/" + file.ID.String()
	in := content.Input{
		Kind: "page",
		Document: content.Document{
			Title:    "Embargoed title",
			Markdown: "Draft excerpt",
			Audience: access.ContentAudience{
				Confirmed: true,
				Roles:     []string{},
				Teams:     []uuid.UUID{},
			},
			Attachments: []uuid.UUID{file.ID},
		},
	}
	check(request(owner, "PUT", path, in, 0, false), 403)
	check(request(member, "PUT", path, in, 0, true), 403)
	p := decode(request(owner, "PUT", path, in, 0, true))
	check(request(member, "GET", path, nil, 0, true), 404)
	check(request(member, "GET", path+"?preview=true", nil, 0, true), 404)
	check(request(member, "GET", filePath, nil, 0, true), 403)
	check(request(stranger, "GET", base+"/content", nil, 0, true), 403)
	list := request(member, "GET", base+"/content", nil, 0, true)
	check(list, 200)
	if strings.Contains(list.Body.String(), "Embargoed") ||
		strings.Contains(list.Body.String(), "Draft excerpt") {
		t.Fatal("embargoed metadata leaked")
	}
	check(request(owner, "GET", path+"?preview=true", nil, 0, true), 200)
	action := func(p content.Page, a string) content.Page {
		t.Helper()
		return decode(
			request(
				owner,
				"POST",
				path+"/actions",
				map[string]string{"action": a, "reason": "Synthetic decision"},
				p.Revision,
				true,
			),
		)
	}
	p = action(action(p, "review"), "approve")
	p = action(p, "release")
	check(request(member, "GET", path, nil, 0, true), 200)
	check(request(member, "GET", filePath, nil, 0, true), 200)
	check(request(stranger, "GET", path, nil, 0, true), 404)
	check(request(stranger, "GET", filePath, nil, 0, true), 403)
	in.Document.Markdown = "New private draft"
	p = decode(request(owner, "PUT", path, in, p.Revision, true))
	released := decode(request(member, "GET", path, nil, 0, true))
	if released.Version != 1 || released.Document.Markdown != "Draft excerpt" {
		t.Fatal("draft superseded released content")
	}
	check(request(owner, "PUT", path, in, p.Revision-1, true), 409)
	check(request(member, "GET", path+"?preview=true&version=1", nil, 0, true), 404)
	check(request(owner, "GET", path+"?preview=true&version=1", nil, 0, true), 200)
	check(request(owner, "GET", path+"/history?limit=2", nil, 0, true), 200)
	check(request(member, "GET", path+"/history", nil, 0, true), 403)
	check(request(member, "GET", base+"/notices", nil, 0, true), 200)
	check(request(stranger, "GET", base+"/notices", nil, 0, true), 403)
	p = action(p, "withdraw")
	if !strings.Contains(p.Message, "cannot be recalled") {
		t.Fatal("copy limitation missing")
	}
	check(request(member, "GET", path, nil, 0, true), 404)
	check(request(member, "GET", filePath, nil, 0, true), 403)
	check(request(owner, "GET", filePath, nil, 0, true), 403)
}
