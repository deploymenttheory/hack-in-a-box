package httpapi

import (
	"bytes"
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

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/identity"
	"github.com/deploymenttheory/hack-in-a-box/internal/media"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestMediaHTTPCurrentConsentSessionCSRFProtectedBytesAndMinimalGuidance(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	owner, photographer, attendee := testdb.Person(
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
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(
		`INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'photographer')`,
		eventID,
		photographer.ID,
	)
	exec(
		`INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')`,
		eventID,
		attendee.ID,
	)
	now := time.Date(2027, 6, 20, 9, 0, 0, 0, time.UTC)
	nativePrivacy := privacy.Service{Pool: pool, Clock: func() time.Time { return now }}
	if _, err := nativePrivacy.Correct(
		ctx,
		attendee.ID,
		eventID,
		0,
		privacy.Fields{
			PhotoConsent:  "yes",
			Dietary:       "SENSITIVE-MEDIA-DIETARY",
			Accessibility: "SENSITIVE-MEDIA-ACCESS",
			ContactEmail:  "sensitive-media@example.test",
		},
	); err != nil {
		t.Fatal(err)
	}
	storage, err := files.OpenLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	fileService := files.Service{
		Pool:       pool,
		Storage:    storage,
		Policy:     access.EventPolicy{Queries: dbgen.New(pool)},
		MaxBytes:   1 << 20,
		EventQuota: 8 << 20,
	}
	native := media.Service{
		Pool:    pool,
		Clock:   func() time.Time { return now },
		Privacy: nativePrivacy,
		Files:   fileService,
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
	var logs bytes.Buffer
	router := NewRouter(
		cfg,
		slog.New(slog.NewJSONHandler(&logs, nil)),
		Dependencies{
			Pool:    pool,
			Auth:    auth,
			Privacy: nativePrivacy,
			Media:   native,
			Files:   fileService,
		},
	)
	csrf := strings.Repeat("a", 32)
	tokens := map[uuid.UUID]string{}
	request := func(person dbgen.Person, method, path string, body any, revision int64, goodCSRF bool) *httptest.ResponseRecorder {
		t.Helper()
		if person.ID != uuid.Nil && tokens[person.ID] == "" {
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
			tokens[person.ID] = token
		}
		var reader io.Reader
		if body != nil {
			wire, _ := json.Marshal(body)
			reader = bytes.NewReader(wire)
		}
		r := httptest.NewRequest(method, path, reader)
		r.Header.Set("Origin", cfg.PublicOrigin)
		r.Header.Set("If-Match", fmt.Sprintf("\"%d\"", revision))
		if body != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		if goodCSRF {
			r.Header.Set("X-CSRF-Token", csrf)
		} else {
			r.Header.Set("X-CSRF-Token", strings.Repeat("b", 32))
		}
		if person.ID != uuid.Nil {
			r.AddCookie(&http.Cookie{Name: "hackbox_session", Value: tokens[person.ID]})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		validateResponse(t, r, w)
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("media/consent response cacheable", w.Header())
		}
		return w
	}
	check := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("want%d got%d %s", status, w.Code, w.Body)
		}
	}
	base := "/api/v1/events/" + eventID.String()
	guide := base + "/media/consent-guidance"
	w := request(photographer, "GET", guide, nil, 1, true)
	check(w, 200)
	for _, secret := range []string{"SENSITIVE-MEDIA", "sensitive-media@example.test", "contact_email", "dietary", "accessibility", "token"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("minimal guidance disclosed unrelated/private field", secret, w.Body)
		}
	}
	check(request(attendee, "GET", guide, nil, 1, true), 403)
	check(request(dbgen.Person{}, "GET", base+"/media", nil, 1, true), 401)
	own, err := nativePrivacy.OwnPhotoConsent(ctx, attendee.ID, eventID)
	if err != nil {
		t.Fatal(err)
	}
	check(
		request(
			attendee,
			"PUT",
			base+"/photo-consent",
			map[string]string{"choice": "no"},
			own.Revision,
			false,
		),
		403,
	)
	var photo bytes.Buffer
	_ = png.Encode(&photo, image.NewNRGBA(image.Rect(0, 0, 2, 2)))
	row, _ := dbgen.New(pool).Event(ctx, eventID)
	item, err := native.CollectPhoto(
		ctx,
		media.Upload{
			ID: uuid.New(),
			Metadata: media.Metadata{
				Kind:   "photo",
				Title:  "HTTP gallery",
				People: []uuid.UUID{attendee.ID},
			},
			Reason: "Synthetic native HTTP upload",
			File: files.Upload{
				Actor:     photographer.ID,
				Event:     eventID,
				Key:       uuid.New(),
				Revision:  row.Revision,
				Filename:  "photo.png",
				MediaType: "image/png",
				Content:   photo.Bytes(),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	path := base + "/media/" + item.ID.String()
	check(request(attendee, "GET", path, nil, 1, true), 404)
	for _, action := range []string{"review", "publish"} {
		row, _ = dbgen.New(pool).Event(ctx, eventID)
		command := media.Command{
			Action:    action,
			Version:   item.Version,
			CommandID: uuid.New(),
			Reason:    "Explicit native HTTP " + action,
		}
		if action == "review" {
			command.Review = &media.Review{ContentReviewed: true, IdentificationReviewed: true}
		}
		w = request(owner, "PUT", path, command, row.Revision, true)
		check(w, 200)
		if err = json.Unmarshal(w.Body.Bytes(), &item); err != nil {
			t.Fatal(err)
		}
	}

	historyPath := path + "/history"
	check(request(attendee, "GET", historyPath, nil, 1, true), 403)
	nonowner := testdb.Person(t, pool)
	exec(`INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'photographer')`, eventID, nonowner.ID)
	check(request(nonowner, "GET", historyPath, nil, 1, true), 403)
	otherEvent := testdb.Event(t, pool, owner.ID)
	check(request(owner, "GET", "/api/v1/events/"+otherEvent.String()+"/media/"+item.ID.String()+"/history", nil, 1, true), 404)
	history := request(photographer, "GET", historyPath+"?limit=2", nil, 1, true)
	check(history, 200)
	var metadata struct {
		Items []map[string]json.RawMessage `json:"items"`
		Next  *int64                       `json:"next_before_version"`
	}
	if err = json.Unmarshal(history.Body.Bytes(), &metadata); err != nil {
		t.Fatal(err)
	}
	if len(metadata.Items) != 2 || metadata.Next == nil || *metadata.Next != 2 {
		t.Fatal("history paging", history.Body)
	}
	for _, entry := range metadata.Items {
		if len(entry) != 3 || entry["version"] == nil || entry["action"] == nil || entry["changed_at"] == nil {
			t.Fatal("history private field leak", history.Body)
		}
	}
	history = request(owner, "GET", historyPath+"?before_version=2&limit=2", nil, 1, true)
	check(history, 200)
	if err = json.Unmarshal(history.Body.Bytes(), &metadata); err != nil {
		t.Fatal(err)
	}
	if len(metadata.Items) != 1 || metadata.Next != nil {
		t.Fatal("history next page", history.Body)
	}
	check(request(owner, "GET", historyPath+"?before_version=0", nil, 1, true), 400)
	check(request(attendee, "GET", path+"/original", nil, 1, true), 200)
	thumbnail := request(attendee, "GET", path+"/thumbnail", nil, 1, true)
	check(thumbnail, 200)
	if _, err := png.Decode(bytes.NewReader(thumbnail.Body.Bytes())); err != nil {
		t.Fatalf("native protected thumbnail must contain a decodable PNG: %v (bytes=%d)", err, thumbnail.Body.Len())
	}
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	thumbnailRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+path+"/thumbnail", nil)
	if err != nil {
		t.Fatal(err)
	}
	thumbnailRequest.AddCookie(&http.Cookie{Name: "hackbox_session", Value: tokens[attendee.ID]})
	streamed, err := server.Client().Do(thumbnailRequest)
	if err != nil {
		t.Fatal(err)
	}
	streamedBytes, err := io.ReadAll(streamed.Body)
	_ = streamed.Body.Close()
	if err != nil || streamed.StatusCode != http.StatusOK {
		t.Fatalf("native protected thumbnail transport: status=%d bytes=%d err=%v", streamed.StatusCode, len(streamedBytes), err)
	}
	if _, err := png.Decode(bytes.NewReader(streamedBytes)); err != nil {
		t.Fatalf("native protected thumbnail transport must contain PNG: %v (bytes=%d content-length=%s)", err, len(streamedBytes), streamed.Header.Get("Content-Length"))
	}
	file, _, err := native.Original(ctx, attendee.ID, eventID, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	check(request(owner, "GET", base+"/files/"+file.ID.String(), nil, 1, true), 403)
	exec(
		`UPDATE events SET stage='archived',writes_allowed=false,completed_at=$2 WHERE id=$1`,
		eventID,
		now,
	)
	check(
		request(
			attendee,
			"PUT",
			base+"/photo-consent",
			map[string]string{"choice": "no"},
			own.Revision,
			true,
		),
		200,
	)
	check(request(attendee, "GET", path+"/original", nil, 1, true), 404)
	check(request(attendee, "GET", path+"/thumbnail", nil, 1, true), 404)
	check(request(owner, "GET", path+"/original", nil, 1, true), 403)
	check(request(owner, "GET", historyPath, nil, 1, true), 200)
	check(request(photographer, "GET", historyPath, nil, 1, true), 200)
	exec(
		`UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='photographer'`,
		eventID,
		photographer.ID,
	)
	check(request(photographer, "GET", guide, nil, 1, true), 403)
	check(request(photographer, "GET", historyPath, nil, 1, true), 403)
	exec(`UPDATE people SET session_revision=session_revision+1 WHERE id=$1`, attendee.ID)
	check(request(attendee, "GET", base+"/photo-consent", nil, 1, true), 401)
	for _, secret := range []string{"SENSITIVE-MEDIA", "sensitive-media@example.test", csrf} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("private data/session leaked into request logs", secret)
		}
	}
}
