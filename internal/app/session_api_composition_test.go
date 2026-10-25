package app

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/sessionbooking"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestSessionAPICompositionSharesClockAndEventReconciliation(t *testing.T) {
	r := newLiveRehearsal(t)
	admin := r.surface(t, "admin")
	ctx := t.Context()
	current, err := admin.deps.Events.Read(ctx, r.owner.ID, r.eventID)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	configuration := current.Configuration
	configuration.Schedule = []event.AgendaItem{{ID: id, Title: "Synthetic shared-clock session", Audience: []string{"attendee"}, Anchor: "live_start"}}
	raw, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.pool.Exec(ctx, "UPDATE events SET configuration=$2 WHERE id=$1", r.eventID, raw); err != nil {
		t.Fatal(err)
	}
	opens, openChoice := testdb.WallTimeForInstant(t, r.clock.read().Add(-time.Hour), configuration.Timezone)
	closes, closeChoice := testdb.WallTimeForInstant(t, r.clock.read().Add(time.Hour), configuration.Timezone)
	configured, err := admin.deps.SessionBookings.Configure(ctx, r.owner.ID, r.eventID, id, 0, sessionbooking.Definition{
		DurationMinutes: 30, Capacity: 1,
		BookingOpens:  sessionbooking.LocalTime{Wall: opens, DSTChoice: openChoice},
		BookingCloses: sessionbooking.LocalTime{Wall: closes, DSTChoice: closeChoice},
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err := admin.deps.SessionBookings.Read(ctx, r.competitor.ID, r.eventID, id)
	if err != nil || !page.CanReserve {
		t.Fatal("composed booking reader did not use the shared domain clock", page, err)
	}
	current, err = admin.deps.Events.Read(ctx, r.owner.ID, r.eventID)
	if err != nil {
		t.Fatal(err)
	}
	configuration.Schedule[0].Title = "Synthetic revised session"
	if _, err = admin.deps.Events.Update(ctx, r.owner.ID, r.eventID, current.Revision, event.Input{ID: r.eventID, Name: current.Name, Slug: current.Slug, Configuration: configuration, Reason: "Synthetic composed session source change"}); err != nil {
		t.Fatal(err)
	}
	page, err = admin.deps.SessionBookings.Read(ctx, r.competitor.ID, r.eventID, id)
	if err != nil || page.Title != configuration.Schedule[0].Title || page.Revision <= configured.Revision {
		t.Fatal("composed event update did not reconcile its session source", page, err)
	}
	r.clock.set(r.clock.read().Add(time.Hour))
	page, err = admin.deps.SessionBookings.Read(ctx, r.competitor.ID, r.eventID, id)
	if err != nil || page.CanReserve {
		t.Fatal("composed booking reader did not observe the exact closing instant", page, err)
	}
}
