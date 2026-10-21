package templates

import (
	"errors"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestConcurrentCloneRetriesAndCompetingConfigurationEdits(t *testing.T) {
	pool := testdb.New(t, true)
	ctx := t.Context()
	owner := testdb.Person(t, pool)
	testdb.Event(t, pool, owner.ID)
	s := Service{Pool: pool}
	input := cloneInput()
	template, err := s.Create(
		ctx,
		owner.ID,
		Input{ID: input.ID, Name: "Starter", Configuration: starter(true).Configuration},
	)
	if err != nil {
		t.Fatal(err)
	}
	template, err = s.Publish(ctx, owner.ID, template.ID, template.Revision, 1)
	if err != nil {
		t.Fatal(err)
	}
	clone := cloneInput()
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() { <-start; _, err := s.CloneTemplate(ctx, owner.ID, template.ID, clone); results <- err }()
	}
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	q := dbgen.New(pool)
	row, err := q.Event(ctx, clone.ID)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE target_id=$1 AND action='event.created'", clone.ID).
		Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("retry created %d events", count)
	}
	bundle, err := s.Snapshot(ctx, owner.ID, clone.ID)
	if err != nil {
		t.Fatal(err)
	}
	brand := bundle.Definitions.Branding
	brand.Primary = "#233c78"
	config := bundle.Event
	config.Venue = "Updated venue"
	start = make(chan struct{})
	go func() { <-start; _, err := s.SaveBrand(ctx, owner.ID, clone.ID, row.Revision, brand); results <- err }()
	go func() {
		<-start
		_, err := (event.Service{Pool: pool}).Update(
			ctx,
			owner.ID,
			clone.ID,
			row.Revision,
			event.Input{Name: row.Name, Slug: row.Slug, Configuration: config},
		)
		results <- err
	}()
	close(start)
	success, conflict := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			success++
		case errors.Is(err, access.ErrConflict):
			conflict++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("concurrent edits: successes=%d conflicts=%d", success, conflict)
	}
	after, err := s.Snapshot(ctx, owner.ID, clone.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Definitions.Tasks) != len(bundle.Definitions.Tasks) {
		t.Fatal("concurrent update lost definitions")
	}
	if (after.Definitions.Branding.Primary == brand.Primary) == (after.Event.Venue == config.Venue) {
		t.Fatal("both stale edits applied, or neither applied")
	}
	unchanged, err := s.Read(ctx, owner.ID, template.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Configuration.Definitions.Branding.Primary == brand.Primary ||
		unchanged.Configuration.Event.Venue == config.Venue {
		t.Fatal("destination edit leaked into source template")
	}
}
