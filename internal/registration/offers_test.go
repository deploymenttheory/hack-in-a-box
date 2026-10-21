package registration

import (
	"errors"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestOffersReserveFIFOAndReplayAcceptance(t *testing.T) {
	f := newRegistrationFixture(t, 1)
	ctx := t.Context()
	q := dbgen.New(f.pool)
	a, b, c := testdb.Person(t, f.pool), testdb.Person(t, f.pool), testdb.Person(t, f.pool)
	first, err := f.service.Apply(ctx, a.ID, f.event.ID, f.application())
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.service.Apply(ctx, b.ID, f.event.ID, f.application())
	if err != nil {
		t.Fatal(err)
	}
	third, err := f.service.Apply(ctx, c.ID, f.event.ID, f.application())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Respond(
		ctx,
		a.ID,
		f.event.ID,
		first.ID,
		first.Revision,
		"withdraw",
	); err != nil {
		t.Fatal(err)
	}
	reserved, err := q.ReservedAttendees(ctx, f.event.ID)
	if err != nil || reserved != 0 {
		t.Fatalf("unapproved automation: %d %v", reserved, err)
	}
	if _, err := f.service.SaveOfferPolicy(
		ctx,
		f.owner.ID,
		f.event.ID,
		OfferSettings{Automated: true, WorkingDays: 2},
	); err != nil {
		t.Fatal(err)
	}
	if err := f.service.MaintainOffers(ctx); err != nil {
		t.Fatal(err)
	}
	own, err := f.service.Own(ctx, b.ID, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	offered := own[0]
	if offered.State != "offered" || offered.Offer == nil {
		t.Fatalf("missing FIFO offer: %+v", offered)
	}
	count, err := q.ConfirmedAttendees(ctx, f.event.ID)
	if err != nil || count != 0 {
		t.Fatalf("offer counted as attendance: %d %v", count, err)
	}
	accepted, err := f.service.Respond(ctx, b.ID, f.event.ID, second.ID, offered.Revision, "accept")
	if err != nil || accepted.State != "confirmed" {
		t.Fatalf("accept: %+v %v", accepted, err)
	}
	replay, err := f.service.Respond(ctx, b.ID, f.event.ID, second.ID, offered.Revision, "accept")
	if err != nil || replay.Revision != accepted.Revision {
		t.Fatalf("replay: %+v %v", replay, err)
	}
	if _, err := f.service.Respond(
		ctx,
		c.ID,
		f.event.ID,
		second.ID,
		accepted.Revision,
		"withdraw",
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatalf("cross-person response: %v", err)
	}
	if _, err := f.service.Respond(
		ctx,
		b.ID,
		f.event.ID,
		second.ID,
		accepted.Revision,
		"withdraw",
	); err != nil {
		t.Fatal(err)
	}
	last, err := f.service.Own(ctx, c.ID, f.event.ID)
	if err != nil || last[0].ID != third.ID || last[0].State != "offered" {
		t.Fatalf("next FIFO offer: %+v %v", last, err)
	}
	var notices int
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM operations WHERE event_id=$1 AND kind='registration.offer'", f.event.ID).
		Scan(&notices); err != nil {
		t.Fatal(err)
	}
	if notices != 2 {
		t.Fatalf("repeated notices: %d", notices)
	}
}

func TestOfferCutoffAndCorrectionsKeepOriginalVersion(t *testing.T) {
	f := newRegistrationFixture(t, 1)
	ctx := t.Context()
	a, b := testdb.Person(t, f.pool), testdb.Person(t, f.pool)
	first, err := f.service.Apply(ctx, a.ID, f.event.ID, f.application())
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := f.service.Apply(ctx, b.ID, f.event.ID, f.application())
	if err != nil {
		t.Fatal(err)
	}
	corrected, err := f.service.CorrectResponse(
		ctx,
		b.ID,
		f.event.ID,
		waiting.ID,
		waiting.Revision,
		f.application().Answers,
	)
	if err != nil || corrected.Revision != waiting.Revision+1 {
		t.Fatalf("correct: %+v %v", corrected, err)
	}
	if _, err := f.service.CorrectResponse(
		ctx,
		b.ID,
		f.event.ID,
		waiting.ID,
		waiting.Revision,
		f.application().Answers,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatalf("stale correction: %v", err)
	}
	f.service.Now = func() time.Time { return f.event.Configuration.AdmissionCutoff.Add(-time.Hour) }
	if _, err := f.service.SaveOfferPolicy(
		ctx,
		f.owner.ID,
		f.event.ID,
		OfferSettings{Automated: true, WorkingDays: 2},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Respond(
		ctx,
		a.ID,
		f.event.ID,
		first.ID,
		first.Revision,
		"withdraw",
	); err != nil {
		t.Fatal(err)
	}
	own, err := f.service.Own(ctx, b.ID, f.event.ID)
	if err != nil {
		t.Fatal(err)
	}
	offered := own[0]
	if offered.Offer == nil ||
		!offered.Offer.ExpiresAt.Equal(f.event.Configuration.AdmissionCutoff) {
		t.Fatalf("uncapped offer: %+v", offered)
	}
	f.service.Now = func() time.Time { return f.event.Configuration.AdmissionCutoff }
	expired, err := f.service.Respond(ctx, b.ID, f.event.ID, offered.ID, offered.Revision, "accept")
	if err != nil || expired.State != "declined" || expired.Offer.State != "expired" {
		t.Fatalf("cutoff acceptance: %+v %v", expired, err)
	}
	if err := f.service.MaintainOffers(ctx); err != nil {
		t.Fatal(err)
	}
	reserved, err := dbgen.New(f.pool).ReservedAttendees(ctx, f.event.ID)
	if err != nil || reserved != 0 {
		t.Fatalf("reservation after cutoff: %d %v", reserved, err)
	}
}
