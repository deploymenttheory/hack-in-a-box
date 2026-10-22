package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
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
	"github.com/deploymenttheory/hack-in-a-box/internal/presentation"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestPresentationHTTPContractsAndCapabilityBoundary(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	owner, outsider := testdb.Person(t, pool), testdb.Person(t, pool)
	id := testdb.Event(t, pool, owner.ID)
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
	service := presentation.Service{Pool: pool}
	router := NewRouter(
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		Dependencies{Pool: pool, Auth: auth, Presentation: service},
	)
	request := func(person dbgen.Person, method, path string, body any, rev int64, csrf bool, token string) *httptest.ResponseRecorder {
		t.Helper()
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", strings.Repeat("x", 32))
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("If-Match", fmt.Sprintf("\"%d\"", rev))
		if person.ID != uuid.Nil {
			session, loadErr := auth.Sessions.Load(ctx, "")
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			auth.Sessions.Put(session, "person_id", person.ID.String())
			auth.Sessions.Put(session, "session_revision", person.SessionRevision)
			auth.Sessions.Put(session, "csrf", strings.Repeat("c", 32))
			cookie, _, commitErr := auth.Sessions.Commit(session)
			if commitErr != nil {
				t.Fatal(commitErr)
			}
			r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: cookie})
		}
		if csrf {
			r.Header.Set("X-CSRF-Token", strings.Repeat("c", 32))
		}
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		return w
	}
	base := "/api/v1/events/" + id.String()
	path := base + "/presentation"
	for _, p := range []dbgen.Person{{}, outsider} {
		w := request(p, "GET", path, nil, 0, false, "")
		want := 403
		if p.ID == uuid.Nil {
			want = 401
		}
		if w.Code != want {
			t.Fatalf("controller authority: %d %s", w.Code, w.Body)
		}
	}
	w := request(owner, "GET", path, nil, 0, false, "")
	if w.Code != 200 {
		t.Fatalf("controller: %d %s", w.Code, w.Body)
	}
	var page presentation.Page
	if err = json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	cue := presentation.Cue{
		ID:              uuid.New(),
		Title:           "Approved synthetic cue",
		Text:            "Public room only",
		DurationSeconds: 120,
	}
	command := presentation.Command{
		Action: "save",
		Reason: "Room program draft",
		Cues:   []presentation.Cue{cue},
	}
	second := cue
	second.ID = uuid.New()
	second.Title = "Second approved cue"
	command.Cues = append(command.Cues, second)
	if w = request(owner, "POST", path, command, page.Revision, false, ""); w.Code != 403 {
		t.Fatalf("CSRF omitted: %d", w.Code)
	}
	if w = request(
		owner,
		"POST",
		path,
		map[string]any{"action": "save", "reason": "Do not accept private fields", "scores": 123},
		page.Revision,
		true,
		"",
	); w.Code != 400 {
		t.Fatalf("private extra field accepted: %d", w.Code)
	}
	w = request(owner, "POST", path, command, page.Revision, true, "")
	if w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	if err = json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	stale := page.Revision - 1
	index, seconds := 0, int32(45)
	urgent := presentation.Cue{ID: uuid.New(), Title: "Urgent room notice", DurationSeconds: 10}
	for _, control := range []presentation.Command{
		{Action: "activate", Index: &index, FromDraft: true},
		{Action: "advance"},
		{Action: "back"},
		{Action: "timer", Seconds: &seconds},
		{Action: "hold"},
		{Action: "resume"},
		{Action: "override", Override: &urgent},
		{Action: "return_to_schedule"},
	} {
		control.Reason = "Explicit room control"
		action := control.Action
		w = request(
			owner,
			"POST",
			path,
			control,
			page.Revision,
			true,
			"",
		)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", action, w.Code, w.Body)
		}
		if err = json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
	}
	if w = request(
		owner,
		"POST",
		path,
		presentation.Command{Action: "hold", Reason: "Stale control"},
		stale,
		true,
		"",
	); w.Code != 409 {
		t.Fatalf("stale revision: %d", w.Code)
	}
	w = request(
		owner,
		"POST",
		path+"/pairings",
		presentation.PairInput{Label: "Synthetic room", Minutes: 60, Reason: "Approved screen"},
		page.Revision,
		true,
		"",
	)
	if w.Code != 201 {
		t.Fatalf("pair: %d %s", w.Code, w.Body)
	}
	var pairing presentation.Paired
	if err = json.Unmarshal(w.Body.Bytes(), &pairing); err != nil {
		t.Fatal(err)
	}
	var raster bytes.Buffer
	if err = png.Encode(&raster, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	assetID := uuid.New()
	defs := templates.EmptyDefinitions()
	defs.Branding.Assets = []templates.Asset{
		{ID: assetID, Kind: "logo", MediaType: "image/png", Content: raster.Bytes()},
	}
	rawDefinitions, _ := json.Marshal(defs)
	if _, err = pool.Exec(
		ctx,
		"INSERT INTO event_definitions(event_id,definitions) VALUES($1,$2)",
		id,
		rawDefinitions,
	); err != nil {
		t.Fatal(err)
	}
	assetPath := base + "/display/assets/" + assetID.String()
	if w = request(
		dbgen.Person{},
		"GET",
		assetPath,
		nil,
		0,
		false,
		pairing.Token,
	); w.Code != 200 || !bytes.Equal(w.Body.Bytes(), raster.Bytes()) || w.Header().Get("Content-Type") != "image/png" ||
		!strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("safe capability image: %d %s", w.Code, w.Body)
	}
	if w = request(owner, "GET", assetPath, nil, 0, false, ""); w.Code != 401 {
		t.Fatalf("cookie became image capability: %d", w.Code)
	}
	if w = request(
		dbgen.Person{},
		"GET",
		base+"/display/assets/"+uuid.NewString(),
		nil,
		0,
		false,
		pairing.Token,
	); w.Code != 404 {
		t.Fatalf("arbitrary file ID: %d %s", w.Code, w.Body)
	}
	if w = request(
		dbgen.Person{},
		"GET",
		assetPath+"?token="+pairing.Token,
		nil,
		0,
		false,
		pairing.Token,
	); w.Code != 400 {
		t.Fatalf("image URL credentials: %d", w.Code)
	}
	if w = request(
		dbgen.Person{},
		"GET",
		base+"/files/"+assetID.String(),
		nil,
		0,
		false,
		pairing.Token,
	); w.Code != 401 {
		t.Fatalf("display token became ordinary file permission: %d", w.Code)
	}
	display := base + "/display/snapshot"
	w = request(dbgen.Person{}, "GET", display, nil, 0, false, pairing.Token)
	if w.Code != 200 || !strings.Contains(w.Body.String(), cue.Title) ||
		!strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("display capability: %d %s", w.Code, w.Body)
	}
	if w = request(owner, "GET", display, nil, 0, false, ""); w.Code != 401 {
		t.Fatalf("person cookie became display capability: %d", w.Code)
	}
	if w = request(dbgen.Person{}, "GET", base, nil, 0, false, pairing.Token); w.Code != 401 {
		t.Fatalf("display capability became person session: %d", w.Code)
	}
	if w = request(
		dbgen.Person{},
		"GET",
		display+"?token="+pairing.Token,
		nil,
		0,
		false,
		pairing.Token,
	); w.Code != 400 {
		t.Fatalf("URL token accepted: %d", w.Code)
	}
	foreign := testdb.Event(t, pool, owner.ID)
	if w = request(
		dbgen.Person{},
		"GET",
		"/api/v1/events/"+foreign.String()+"/display/snapshot",
		nil,
		0,
		false,
		pairing.Token,
	); w.Code != 401 {
		t.Fatalf("cross-event capability: %d", w.Code)
	}
	w = request(
		owner,
		"POST",
		path+"/pairings/"+pairing.Pairing.ID.String()+"/revoke",
		map[string]string{"reason": "Screen retired"},
		pairing.Revision,
		true,
		"",
	)
	if w.Code != 200 {
		t.Fatalf("revoke: %d %s", w.Code, w.Body)
	}
	if w = request(dbgen.Person{}, "GET", assetPath, nil, 0, false, pairing.Token); w.Code != 401 {
		t.Fatalf("revoked image capability: %d", w.Code)
	}
	if w = request(dbgen.Person{}, "GET", display, nil, 0, false, pairing.Token); w.Code != 401 {
		t.Fatalf("revoked capability: %d", w.Code)
	}
}

func TestDisplayStreamFlushesBeyondJSONTimeoutAndRechecksRevocation(t *testing.T) {
	for _, mode := range []string{"pairing", "mc", "organiser", "dependency"} {
		t.Run(mode, func(t *testing.T) {
			pool := testdb.New(t, true)
			ctx := t.Context()
			owner, mc := testdb.Person(t, pool), testdb.Person(t, pool)
			id := testdb.Event(t, pool, owner.ID)
			if _, err := pool.Exec(
				ctx,
				"INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'mc')",
				id,
				mc.ID,
			); err != nil {
				t.Fatal(err)
			}
			service := presentation.Service{Pool: pool}
			actor := mc.ID
			if mode == "organiser" {
				actor = owner.ID
			}
			page, err := service.Read(ctx, actor, id)
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range []presentation.Command{{Action: "save", Cues: []presentation.Cue{{ID: uuid.New(), Title: "Current approved cue", DurationSeconds: 60}, {ID: uuid.New(), Title: "Next approved cue"}}}, {Action: "publish"}, {Action: "advance"}} {
				c.Reason = "Synthetic room approval"
				page, err = service.Change(ctx, actor, id, page.Revision, c)
				if err != nil {
					t.Fatal(err)
				}
			}
			paired, err := service.Pair(
				ctx,
				actor,
				id,
				page.Revision,
				presentation.PairInput{
					Label:   "Synthetic screen",
					Minutes: 60,
					Reason:  "Approved screen",
				},
			)
			if err != nil {
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
			cfg.RequestTimeout = 40 * time.Millisecond
			var logs bytes.Buffer
			router := NewRouter(
				cfg,
				slog.New(slog.NewTextHandler(&logs, nil)),
				Dependencies{Pool: pool, Presentation: service},
			)
			router.Get("/slow-json", func(w http.ResponseWriter, r *http.Request) {
				select {
				case <-r.Context().Done():
					return
				case <-time.After(time.Second):
					writeJSON(w, 200, map[string]string{"status": "late"})
				}
			})
			server := httptest.NewUnstartedServer(router)
			server.Config.WriteTimeout = 80 * time.Millisecond
			server.Start()
			defer server.Close()
			client := server.Client()
			client.Timeout = 7 * time.Second
			req, err := http.NewRequestWithContext(
				ctx,
				"GET",
				server.URL+"/api/v1/events/"+id.String()+"/display/stream",
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+paired.Token)
			req.Header.Set("Last-Event-ID", "0")
			response, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			if response.StatusCode != 200 ||
				response.Header.Get("Content-Type") != "text/event-stream" ||
				response.Header.Get("Set-Cookie") != "" {
				t.Fatalf("stream headers: %d %v", response.StatusCode, response.Header)
			}
			reader := bufio.NewReader(response.Body)
			frame := func() string {
				t.Helper()
				var b strings.Builder
				for {
					line, readErr := reader.ReadString('\n')
					if readErr != nil {
						t.Fatal(readErr)
					}
					b.WriteString(line)
					if line == "\n" {
						return b.String()
					}
				}
			}
			first := frame()
			if !strings.Contains(first, fmt.Sprintf("id: %d", paired.Revision)) ||
				!strings.Contains(first, "Current approved cue") ||
				!strings.Contains(first, "event: snapshot") {
				t.Fatalf("stale reconnect did not receive current snapshot: %s", first)
			}
			// A later flushed frame proves both the buffering JSON timeout and the
			// server's default write deadline were replaced only for this stream.
			second := frame()
			if !strings.Contains(second, "event: snapshot") {
				t.Fatalf("stream stopped at finite timeout: %s", second)
			}
			slow, err := client.Get(server.URL + "/slow-json")
			if err != nil {
				t.Fatal(err)
			}
			if slow.StatusCode != 503 {
				t.Fatalf("finite JSON timeout changed: %d", slow.StatusCode)
			}
			_ = slow.Body.Close()
			if mode == "dependency" {
				if _, err = pool.Exec(
					ctx,
					"ALTER TABLE display_pairings RENAME TO unavailable_display_pairings",
				); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if _, restoreErr := pool.Exec(
						context.WithoutCancel(ctx),
						"ALTER TABLE unavailable_display_pairings RENAME TO display_pairings",
					); restoreErr != nil {
						t.Error(restoreErr)
					}
				}()
			} else if mode == "pairing" {
				page, err = service.Read(ctx, owner.ID, id)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = service.Revoke(
					ctx,
					owner.ID,
					id,
					paired.Pairing.ID,
					page.Revision,
					"Synthetic revocation",
				); err != nil {
					t.Fatal(err)
				}
			} else {
				role := mode
				if mode == "mc" {
					role = "mc"
				}
				if _, err = pool.Exec(
					ctx,
					"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role=$3",
					id,
					actor,
					role,
				); err != nil {
					t.Fatal(err)
				}
			}
			expectedEvent := "event: revoked"
			if mode == "dependency" {
				expectedEvent = "event: reconnect"
			}
			revoked := ""
			for range 3 {
				revoked = frame()
				if strings.Contains(revoked, expectedEvent) {
					break
				}
			}
			if !strings.Contains(revoked, expectedEvent) {
				t.Fatalf("open stream did not close on %s revocation: %s", mode, revoked)
			}
			_, err = reader.ReadByte()
			if err != io.EOF {
				t.Fatalf("stream remained open: %v", err)
			}
			_ = response.Body.Close()
			server.Close()
			if strings.Contains(logs.String(), paired.Token) ||
				strings.Contains(logs.String(), "Bearer") ||
				strings.Contains(first, "pairings") ||
				strings.Contains(first, "person_id") {
				t.Fatal("display secret/account data entered payload or logs")
			}
		})
	}
}
