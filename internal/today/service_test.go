package today_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/presentation"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
	"github.com/deploymenttheory/hack-in-a-box/internal/today"
)

func TestTodayCurrentRolesPublishedRoomParityAndReadIsolation(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	q := dbgen.New(pool)
	owner, attendee, multi, outsider := testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	)
	id := testdb.Event(t, pool, owner.ID)
	now := time.Date(2027, 6, 20, 10, 0, 0, 0, time.UTC)
	native := presentation.Service{Pool: pool, Clock: func() time.Time { return now }}
	service := today.Service{
		Events:       event.Service{Pool: pool, Clock: func() time.Time { return now }},
		Presentation: native,
	}
	if err := q.SetCompetitionState(
		ctx,
		dbgen.SetCompetitionStateParams{EventID: id, PersonID: attendee.ID, State: "confirmed"},
	); err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"mentor", "judge"} {
		if err := q.SetMembership(
			ctx,
			dbgen.SetMembershipParams{EventID: id, PersonID: multi.ID, Role: role, Active: true},
		); err != nil {
			t.Fatal(err)
		}
	}
	// An authorised read with no room program cannot create presentation state.
	first, err := service.Read(ctx, attendee.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	if first.Program.Current != nil || first.Program.Next != nil || first.Program.Sequence != 0 ||
		!first.Program.ServerNow.Equal(now) ||
		first.ParticipationState != "confirmed" ||
		len(first.Roles) != 0 {
		t.Fatalf("empty/current context: %+v", first)
	}
	var count int
	if err = pool.QueryRow(ctx, "SELECT count(*) FROM presentation_states WHERE event_id=$1", id).
		Scan(&count); err != nil ||
		count != 0 {
		t.Fatalf("Today initialised room state: %d %v", count, err)
	}
	change := func(c presentation.Command) presentation.Page {
		t.Helper()
		page, e := native.Read(ctx, owner.ID, id)
		if e != nil {
			t.Fatal(e)
		}
		c.Reason = "Synthetic reviewed room action"
		page, e = native.Change(ctx, owner.ID, id, page.Revision, c)
		if e != nil {
			t.Fatal(e)
		}
		return page
	}
	secret := "UNRELEASED-TODAY-DRAFT-CANARY"
	change(
		presentation.Command{
			Action: "save",
			Cues:   []presentation.Cue{{ID: uuid.New(), Title: secret, DurationSeconds: 60}},
		},
	)
	hidden, err := service.Read(ctx, multi.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(hidden)
	if strings.Contains(string(data), secret) ||
		!reflect.DeepEqual(hidden.Roles, []string{"judge", "mentor"}) {
		t.Fatalf("role/draft projection: %s", data)
	}
	// Staff schedule approval is not permission to disclose raw role-specific text.
	if _, err = pool.Exec(
		ctx,
		`UPDATE events SET configuration=jsonb_set(configuration,'{schedule}',$2::jsonb) WHERE id=$1`,
		id,
		`[{"id":"13dd5046-ef02-44d3-80ea-80d67aceca36","title":"PRIVATE-STAFF-AGENDA-CANARY","audience":["judge"],"anchor":"live_start","days":0,"minutes":0,"business_days":false}]`,
	); err != nil {
		t.Fatal(err)
	}
	cues := []presentation.Cue{
		{ID: uuid.New(), Title: "Published current room activity", DurationSeconds: 120},
		{ID: uuid.New(), Title: "Published next room activity", DurationSeconds: 60},
	}
	change(presentation.Command{Action: "save", Cues: cues})
	change(presentation.Command{Action: "publish"})
	change(presentation.Command{Action: "advance"})
	page, err := native.Read(ctx, owner.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	pairing, err := native.Pair(
		ctx,
		owner.ID,
		id,
		page.Revision,
		presentation.PairInput{
			Label:   "Synthetic parity screen",
			Minutes: 60,
			Reason:  "Parity verification",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := service.Read(ctx, multi.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	pairings, err := q.DisplayPairings(ctx, id)
	if err != nil || len(pairings) != 1 || pairings[0].LastSeenAt.Valid {
		t.Fatalf("Today fabricated a display heartbeat: %+v %v", pairings, err)
	}
	display, err := native.Snapshot(ctx, id, pairing.Token)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual.Program.Current, display.Current) ||
		!reflect.DeepEqual(actual.Program.Next, display.Next) ||
		actual.Program.Sequence != display.Sequence ||
		actual.Program.Mode != display.Mode ||
		actual.Program.RemainingSeconds != display.RemainingSeconds ||
		!reflect.DeepEqual(actual.Program.TimerEndsAt, display.TimerEndsAt) {
		t.Fatalf("Today differs from native room program: %+v %+v", actual.Program, display)
	}
	data, _ = json.Marshal(actual)
	for _, private := range []string{secret, "PRIVATE-STAFF-AGENDA-CANARY", pairing.Token, `"pairings":`, `"draft":`, `"can_control":`} {
		if strings.Contains(string(data), private) {
			t.Fatalf("private controller/schedule value %q leaked", private)
		}
	}
	if _, err = service.Read(ctx, attendee.ID, uuid.New()); !errors.Is(err, access.ErrNotFound) {
		t.Fatalf("unknown event: %v", err)
	}
	// Each request reads current grants; context selection cannot retain a revoked role.
	if err = q.SetMembership(
		ctx,
		dbgen.SetMembershipParams{EventID: id, PersonID: multi.ID, Role: "mentor", Active: false},
	); err != nil {
		t.Fatal(err)
	}
	actual, err = service.Read(ctx, multi.ID, id)
	if err != nil || !reflect.DeepEqual(actual.Roles, []string{"judge"}) {
		t.Fatalf("revoked role retained: %+v %v", actual.Roles, err)
	}
	if err = q.SetMembership(
		ctx,
		dbgen.SetMembershipParams{EventID: id, PersonID: multi.ID, Role: "judge", Active: false},
	); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Read(ctx, multi.ID, id); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("revoked reader: %v", err)
	}
	if _, err = service.Read(ctx, outsider.ID, id); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("unrelated reader: %v", err)
	}
	if _, err = pool.Exec(
		ctx,
		"UPDATE events SET stage='archived',writes_allowed=false WHERE id=$1",
		id,
	); err != nil {
		t.Fatal(err)
	}
	archived, err := service.Read(ctx, attendee.ID, id)
	if err != nil || archived.Program.Current != nil || archived.Program.Next != nil ||
		archived.Program.TimerEndsAt != nil {
		t.Fatalf("archived room program revived: %+v %v", archived.Program, err)
	}
	if _, err = pool.Exec(
		ctx,
		"UPDATE people SET active=false WHERE id=$1",
		attendee.ID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Read(ctx, attendee.ID, id); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("disabled reader: %v", err)
	}
}
