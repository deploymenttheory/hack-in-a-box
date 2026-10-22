package presentation

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

func TestRehearsalActivationAndUrgentOverridePreservePlannedSequence(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	program := []Cue{
		{ID: uuid.New(), Title: "Planned opening", DurationSeconds: 120},
		{ID: uuid.New(), Title: "Planned building", DurationSeconds: 90},
		{ID: uuid.New(), Title: "Planned wrap up", DurationSeconds: 60},
	}
	f.program(program)
	pair := f.pair(f.mc.ID)
	index := 1
	f.change(f.mc.ID, Command{Action: "activate", Index: &index})
	before := f.snapshot(pair.Token)
	// Editing/rehearsing the next program never activates it on room screens.
	f.change(
		f.owner.ID,
		Command{
			Action: "save",
			Cues: []Cue{
				{ID: uuid.New(), Title: "Private rehearsal replacement", DurationSeconds: 300},
			},
		},
	)
	after := f.snapshot(pair.Token)
	if !reflect.DeepEqual(before.Current, after.Current) ||
		!reflect.DeepEqual(before.Next, after.Next) ||
		before.RemainingSeconds != after.RemainingSeconds {
		t.Fatal("draft rehearsal changed a live screen")
	}
	f.change(f.mc.ID, Command{Action: "back"})
	if p := f.snapshot(pair.Token); p.Current == nil || p.Current.Title != program[0].Title {
		t.Fatal("explicit back did not activate previous approved cue")
	}
	f.change(f.mc.ID, Command{Action: "activate", Index: &index})
	f.now = f.now.Add(17 * time.Second)
	urgent := Cue{
		ID:              uuid.New(),
		Title:           "Urgent room announcement",
		Text:            "Please hold for a room update",
		DurationSeconds: 30,
	}
	page := f.change(f.mc.ID, Command{Action: "override", Override: &urgent})
	if page.Override == nil || page.CurrentIndex != 1 || page.RemainingSeconds != 73 ||
		page.TimerEndsAt != nil ||
		!reflect.DeepEqual(page.Published, program) {
		t.Fatalf("override changed planned state: %+v", page)
	}
	f.now = f.now.Add(11 * time.Second)
	p := f.snapshot(pair.Token)
	if !p.OverrideActive || p.Current == nil || p.Current.Title != urgent.Title || p.Next == nil ||
		p.Next.Title != program[1].Title ||
		p.RemainingSeconds != 19 {
		t.Fatalf("urgent projection or return preview: %+v", p)
	}
	replacement := urgent
	replacement.ID = uuid.New()
	replacement.Title = "Updated urgent room announcement"
	page = f.change(f.mc.ID, Command{Action: "override", Override: &replacement})
	if page.CurrentIndex != 1 || page.RemainingSeconds != 73 {
		t.Fatal("replacing an urgent cue overwrote the original planned return state")
	}
	seconds := int32(45)
	f.change(f.mc.ID, Command{Action: "timer", Seconds: &seconds})
	f.change(f.mc.ID, Command{Action: "hold"})
	f.now = f.now.Add(13 * time.Second)
	p = f.snapshot(pair.Token)
	if p.RemainingSeconds != 45 || p.Mode != "held" {
		t.Fatal("urgent countdown did not hold independently")
	}
	f.change(f.mc.ID, Command{Action: "resume"})
	page, err := f.service.Read(ctx, f.owner.ID, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Change(
		ctx,
		f.owner.ID,
		f.id,
		page.Revision,
		Command{
			Action: "activate",
			Index:  &index,
			Reason: "Cannot skip planned cues while urgent override is active",
		},
	); !errors.Is(
		err,
		event.ErrBlocked,
	) {
		t.Fatalf("planned cue activation bypassed urgent state: %v", err)
	}
	// Both consoles reviewed the same urgent state; only one return may commit.
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, actor := range []uuid.UUID{f.owner.ID, f.mc.ID} {
		go func() {
			<-start
			_, err := f.service.Change(
				ctx,
				actor,
				f.id,
				page.Revision,
				Command{Action: "return_to_schedule", Reason: "Resume planned schedule"},
			)
			results <- err
		}()
	}
	close(start)
	committed, stale := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			committed++
		} else if errors.Is(err, access.ErrConflict) {
			stale++
		} else {
			t.Fatal(err)
		}
	}
	if committed != 1 || stale != 1 {
		t.Fatalf("two-console urgent return: %d/%d", committed, stale)
	}
	page, err = f.service.Read(ctx, f.owner.ID, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if page.Override != nil || page.CurrentIndex != 1 || page.RemainingSeconds != 73 ||
		page.Mode != "running" ||
		!reflect.DeepEqual(page.Published, program) {
		t.Fatal("return failed to restore the planned cue and its paused time")
	}
	p = f.snapshot(pair.Token)
	if p.OverrideActive || p.Current == nil || p.Current.Title != program[1].Title ||
		p.Next == nil ||
		p.Next.Title != program[2].Title ||
		p.RemainingSeconds != 73 {
		t.Fatalf("return projection: %+v", p)
	}
	f.now = f.now.Add(time.Second)
	if p = f.snapshot(pair.Token); p.RemainingSeconds != 72 {
		t.Fatal("planned timer did not resume from preserved remaining time")
	}
	f.change(f.owner.ID, Command{Action: "hold"})
	seconds = 65
	f.change(f.owner.ID, Command{Action: "timer", Seconds: &seconds})
	f.now = f.now.Add(3 * time.Second)
	if p = f.snapshot(pair.Token); p.RemainingSeconds != 65 || p.Mode != "held" {
		t.Fatal("planned countdown adjustment resumed a held timer")
	}
	// Activating a reviewed rehearsal is one atomic program/screen operation.
	index = 0
	f.change(f.owner.ID, Command{Action: "activate", Index: &index, FromDraft: true})
	p = f.snapshot(pair.Token)
	if p.Current == nil || p.Current.Title != "Private rehearsal replacement" ||
		p.RemainingSeconds != 300 {
		t.Fatal("explicit draft activation did not atomically approve and start the reviewed cue")
	}
}

func TestDisplayLastSeenIsFactualAndNeverChangesReviewedRevisions(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	paired := f.pair(f.mc.ID)
	q := dbgen.New(f.pool)
	page, err := f.service.Read(ctx, f.owner.ID, f.id)
	if err != nil {
		t.Fatal(err)
	}
	eventBefore, err := q.Event(ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Pairings) != 1 || page.Pairings[0].LastSeenAt != nil || !page.Pairings[0].Stale {
		t.Fatal("new pairing claimed a screen connection")
	}
	snapshot := f.snapshot(paired.Token)
	firstSeen := f.now
	page, err = f.service.Read(ctx, f.owner.ID, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if page.Pairings[0].LastSeenAt == nil || !page.Pairings[0].LastSeenAt.Equal(firstSeen) ||
		page.Pairings[0].Stale {
		t.Fatal("authorized snapshot did not record factual recent contact")
	}
	f.now = f.now.Add(4 * time.Second)
	f.snapshot(paired.Token)
	page, err = f.service.Read(ctx, f.owner.ID, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if !page.Pairings[0].LastSeenAt.Equal(firstSeen) {
		t.Fatal("heartbeat writes were not throttled")
	}
	f.now = f.now.Add(2 * time.Second)
	f.snapshot(paired.Token)
	page, err = f.service.Read(ctx, f.owner.ID, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if !page.Pairings[0].LastSeenAt.Equal(f.now) {
		t.Fatal("later successful snapshot did not refresh factual last-seen")
	}
	if page.Revision != paired.Revision || snapshot.Sequence != paired.Revision {
		t.Fatal("heartbeat invalidated a reviewed cue-control revision")
	}
	eventAfter, err := q.Event(ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if eventBefore.Revision != eventAfter.Revision {
		t.Fatal("heartbeat invalidated event/approval revisions")
	}
	// A controller command reviewed before all these polls still succeeds.
	if _, err = f.service.Change(
		ctx,
		f.owner.ID,
		f.id,
		paired.Revision,
		Command{
			Action: "save",
			Reason: "Review remains valid after display polls",
			Cues:   []Cue{{ID: uuid.New(), Title: "Reviewed after heartbeat"}},
		},
	); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(11 * time.Second)
	page, err = f.service.Read(ctx, f.owner.ID, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if !page.Pairings[0].Stale || page.Pairings[0].LastSeenAt == nil {
		t.Fatal("missing snapshots did not become stale while retaining factual last-seen")
	}
}
