package app

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/config"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestNativeHelpRuntimeRecordsSafeInboxAndRevokesActualResource(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()

	cfg, err := config.Load(func(key string) string {
		if key == "HACKBOX_ENV" {
			return "test"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := newCommunicationRuntime(
		pool,
		cfg,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	owner := testdb.Person(t, pool)
	member := testdb.Person(t, pool)
	other := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	teamID := uuid.New()
	configuration := event.DefaultConfiguration()
	configuration.LiveStart = time.Now().Add(time.Hour)
	configuration.LiveEnd = configuration.LiveStart.Add(8 * time.Hour)
	raw, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(
		ctx,
		"UPDATE events SET configuration=$2 WHERE id=$1",
		eventID,
		raw,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(
		ctx,
		"INSERT INTO competition_claims(event_id,person_id,state) VALUES($1,$2,'confirmed'),($1,$3,'confirmed')",
		eventID,
		member.ID,
		other.ID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(
		ctx,
		"INSERT INTO hack_teams(event_id,id,name,state) VALUES($1,$2,'Synthetic','published')",
		eventID,
		teamID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(
		ctx,
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'published')",
		eventID,
		teamID,
		member.ID,
	); err != nil {
		t.Fatal(err)
	}
	requestID := uuid.New()
	detail, err := runtime.Help.Create(
		ctx,
		member.ID,
		eventID,
		requestID,
		help.Input{
			CommandID:   uuid.New(),
			TeamID:      teamID,
			Category:    "logistics",
			Description: "PRIVATEQUESTION-canary",
			Location:    "PRIVATELOCATION-canary",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	page, err := runtime.Inbox.List(ctx, member.ID, nil, nil, 20)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("actual source did not record own notification: %+v %v", page, err)
	}
	opened, err := runtime.Inbox.Open(ctx, member.ID, page.Items[0].ID, nil)
	if err != nil || opened.Destination == nil || opened.Destination.Kind != "help" ||
		opened.Destination.ResourceID != requestID {
		t.Fatalf("actual route unavailable: %+v %v", opened, err)
	}
	encoded, _ := json.Marshal(opened)
	if strings.Contains(string(encoded), "PRIVATE") {
		t.Fatal("private help context copied to notification")
	}
	if _, err = runtime.Help.Read(ctx, member.ID, eventID, requestID); err != nil {
		t.Fatal(err)
	}
	hidden, err := runtime.Inbox.List(ctx, other.ID, nil, nil, 20)
	if err != nil || len(hidden.Items) != 0 || hidden.Unread != 0 {
		t.Fatal("cross-team preview leak", err)
	}

	// Native followups must work with no automation binding and no provider call.
	clock := detail.Request.RequestedAt.Add(time.Minute)
	runtime.Help.Clock = func() time.Time { return clock }
	assertNativeCounts := func(want int) {
		t.Helper()
		var intents, bindings int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM inbox_notifications WHERE event_id=$1 AND recipient_id=$2", eventID, member.ID).
			Scan(&intents); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM communication_automation_versions WHERE event_id=$1", eventID).
			Scan(&bindings); err != nil {
			t.Fatal(err)
		}
		if intents != want || bindings != 0 {
			t.Fatalf("native intent count%d want%d; optional bindings%d", intents, want, bindings)
		}
	}
	if err = runtime.Help.Maintain(ctx); err != nil {
		t.Fatal(err)
	}
	assertNativeCounts(1)
	clock = clock.Add(16 * time.Minute)
	if _, err = pool.Exec(
		ctx,
		"UPDATE events SET stage='archived',writes_allowed=false WHERE id=$1",
		eventID,
	); err != nil {
		t.Fatal(err)
	}
	if err = runtime.Help.Maintain(ctx); err != nil {
		t.Fatal(err)
	}
	assertNativeCounts(1)
	if _, err = pool.Exec(
		ctx,
		"UPDATE events SET stage='draft',writes_allowed=true WHERE id=$1",
		eventID,
	); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = runtime.Help.Maintain(ctx); err != nil {
			t.Fatal(err)
		}
	}
	assertNativeCounts(2)
	if _, err = pool.Exec(
		ctx,
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		eventID,
		member.ID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = runtime.Inbox.Open(
		ctx,
		member.ID,
		page.Items[0].ID,
		nil,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("revoked inbox source remained readable", err)
	}
	read := true
	if _, err = runtime.Inbox.Open(
		ctx,
		member.ID,
		page.Items[0].ID,
		&read,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("revoked source read state changed", err)
	}
	if _, err = runtime.Help.Read(
		ctx,
		member.ID,
		eventID,
		detail.Request.ID,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("native resource bypassed revocation", err)
	}
}
