package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/presentation"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

// The rehearsal changes domain time, never authentication expiry or transport
// deadlines. A mutex also makes concurrent SSE reads safe under the race detector.
type rehearsalClock struct {
	mu  sync.RWMutex
	now time.Time
}

func (c *rehearsalClock) read() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.now
}

func (c *rehearsalClock) set(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
}

type rehearsalSurface struct {
	server *httptest.Server
	deps   httpapi.Dependencies
	auth   *identity.Auth
}

type liveRehearsal struct {
	pool                          *pgxpool.Pool
	clock                         *rehearsalClock
	owner, mc, mentor, competitor dbgen.Person
	eventID                       uuid.UUID
}

func newLiveRehearsal(t *testing.T) *liveRehearsal {
	t.Helper()
	r := &liveRehearsal{
		pool:  testdb.New(t, true),
		clock: &rehearsalClock{now: time.Date(2027, 3, 22, 9, 0, 0, 0, time.UTC)},
	}
	r.owner, r.mc, r.mentor, r.competitor = testdb.Person(
		t,
		r.pool,
	), testdb.Person(
		t,
		r.pool,
	), testdb.Person(
		t,
		r.pool,
	), testdb.Person(
		t,
		r.pool,
	)
	r.eventID = testdb.Event(t, r.pool, r.owner.ID)
	for role, person := range map[string]dbgen.Person{"mc": r.mc, "mentor": r.mentor} {
		if _, err := r.pool.Exec(
			t.Context(),
			"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,$3)",
			r.eventID,
			person.ID,
			role,
		); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

func (r *liveRehearsal) surface(t *testing.T, surface string) rehearsalSurface {
	t.Helper()
	cfg, err := config.Load(func(key string) string {
		switch key {
		case "HACKBOX_ENV":
			return "test"
		case "HACKBOX_SURFACE":
			return surface
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(nil)
	cfg.PublicOrigin = "http://" + server.Listener.Addr().String()
	cfg.WorkspaceOrigin = cfg.PublicOrigin
	directory := "dist"
	if surface == "admin" {
		directory = "dist-admin"
	}
	cfg.WebDir, err = filepath.Abs(filepath.Join("..", "..", "web", directory))
	if err != nil {
		t.Fatal(err)
	}
	auth, err := identity.New(
		t.Context(),
		r.pool,
		identity.Settings{Origin: cfg.PublicOrigin, CookiePrefix: "hackbox_" + surface},
	)
	if err != nil {
		t.Fatal(err)
	}
	storage, err := files.OpenLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	deps, err := newAPIDependencies(
		t.Context(),
		r.pool,
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		storage,
		auth,
	)
	if err != nil {
		t.Fatal(err)
	}
	deps.Events.Clock, deps.Presentation.Clock, deps.Submissions.Clock = r.clock.read, r.clock.read, r.clock.read
	deps.Preparation.Now, deps.Teams.Now = r.clock.read, r.clock.read
	deps.Help.Clock, deps.Clarifications.Clock = r.clock.read, r.clock.read
	server.Config.Handler = httpapi.NewRouter(
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		deps,
	)
	server.Start()
	t.Cleanup(server.Close)
	return rehearsalSurface{server: server, deps: deps, auth: auth}
}

func (s rehearsalSurface) cookie(t *testing.T, actor dbgen.Person) *http.Cookie {
	t.Helper()
	session, err := s.auth.Sessions.Load(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	s.auth.Sessions.Put(session, "person_id", actor.ID.String())
	s.auth.Sessions.Put(session, "session_revision", actor.SessionRevision)
	s.auth.Sessions.Put(session, "csrf", strings.Repeat("c", 32))
	s.auth.Sessions.Put(session, "draft_scope", uuid.NewString())
	token, _, err := s.auth.Sessions.Commit(session)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: s.auth.Sessions.Cookie.Name, Value: token, Path: "/", HttpOnly: true}
}

func TestLiveRehearsalNativeAuthority(t *testing.T) {
	r := newLiveRehearsal(t)
	admin, public := r.surface(t, "admin"), r.surface(t, "public")
	path := "/api/v1/events/" + r.eventID.String() + "/presentation"
	for _, probe := range []struct {
		name    string
		surface rehearsalSurface
		actor   dbgen.Person
		status  int
	}{
		{"mc controls", admin, r.mc, 200},
		{"competitor denied", admin, r.competitor, 403},
		{"independent public surface uses current actor policy", public, r.mc, 200},
	} {
		t.Run(probe.name, func(t *testing.T) {
			request, err := http.NewRequestWithContext(
				t.Context(),
				http.MethodGet,
				probe.surface.server.URL+path,
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			request.AddCookie(probe.surface.cookie(t, probe.actor))
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != probe.status {
				t.Fatalf("status=%d body=%s", response.StatusCode, body)
			}
			if !strings.Contains(response.Header.Get("Cache-Control"), "no-store") {
				t.Fatal("native API must not be cached")
			}
			if probe.status == 200 {
				var page presentation.Page
				if err := json.Unmarshal(body, &page); err != nil {
					t.Fatal(err)
				}
				if !page.ServerNow.Equal(r.clock.read()) {
					t.Fatal("native presentation did not use controlled domain clock")
				}
			}
		})
	}
	// A valid administrator cookie does not authenticate the independent public surface.
	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		public.server.URL+path,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(admin.cookie(t, r.mc))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("cross-surface cookie authenticated: %d", response.StatusCode)
	}
	// Context construction cannot itself enqueue provider work.
	var jobs int
	if err := r.pool.QueryRow(context.Background(), "SELECT count(*) FROM river_job").
		Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 0 {
		t.Fatalf("composition enqueued %d provider jobs", jobs)
	}
}
