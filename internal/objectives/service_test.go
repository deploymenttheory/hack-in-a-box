package objectives

import (
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestObjectivesNativeVersionsIsolationCurrentAuthorityAndConcurrentRevision(t *testing.T) {
	pool := testdb.New(t, true)
	schema, err := os.ReadFile("schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(
		t.Context(),
		strings.Split(string(schema), "-- +goose Down")[0],
	); err != nil {
		t.Fatal(err)
	}
	owner := testdb.Person(t, pool)
	stranger := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	otherEvent := testdb.Event(t, pool, stranger.ID)
	s := Service{Pool: pool}
	id, measure := uuid.New(), uuid.New()
	target := "5"
	d := Definition{
		Description:    "Private early challenge objective",
		ThemeSensitive: true,
		OwnerID:        &owner.ID,
		Measures: []Measure{
			{
				ID:            measure,
				Description:   "Working prototypes",
				Target:        &target,
				Unit:          "prototype",
				Method:        "Reviewed evidence",
				ReviewHorizon: "One month after event",
			},
		},
	}
	created, err := s.Change(
		t.Context(),
		owner.ID,
		eventID,
		id,
		0,
		Command{Action: "save", Definition: &d, Reason: "Define intent"},
	)
	if err != nil || created.CurrentVersion != 1 || created.Definition.Measures[0].Baseline != nil {
		t.Fatal("create configuration", created, err)
	}
	if _, err = s.Read(
		t.Context(),
		stranger.ID,
		eventID,
		id,
		0,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("nonorganiser early planning read", err)
	}
	if _, err = s.Read(
		t.Context(),
		stranger.ID,
		otherEvent,
		id,
		1,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("cross-event reference resolved", err)
	}
	d.Measures[0].Unit = "reviewed demonstration"
	revised, err := s.Change(
		t.Context(),
		owner.ID,
		eventID,
		id,
		1,
		Command{Action: "save", Definition: &d, Reason: "Clarify measurement"},
	)
	if err != nil || revised.Reference.Version != 2 {
		t.Fatal(revised, err)
	}
	retained, err := s.Read(t.Context(), owner.ID, eventID, id, 1)
	if err != nil || retained.Definition.Measures[0].Unit != "prototype" ||
		retained.CurrentVersion != 2 {
		t.Fatal("observation reference retargeted", retained, err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, e := s.Change(
				t.Context(),
				owner.ID,
				eventID,
				id,
				2,
				Command{Action: "save", Definition: &d, Reason: "Concurrent definition review"},
			)
			results <- e
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err = range results {
		if err == nil {
			successes++
		} else if errors.Is(err, access.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("revision race successes=%d conflicts=%d", successes, conflicts)
	}
	retired, err := s.Change(
		t.Context(),
		owner.ID,
		eventID,
		id,
		3,
		Command{Action: "retire", Reason: "No longer measured"},
	)
	if err != nil || !retired.Retired || retired.Reference.Version != 4 {
		t.Fatal(retired, err)
	}
	if _, err = pool.Exec(
		t.Context(),
		"UPDATE objective_versions SET retired=false WHERE event_id=$1 AND objective_id=$2",
		eventID,
		id,
	); err == nil {
		t.Fatal("immutable definition history changed")
	}
	if _, err = pool.Exec(
		t.Context(),
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='organiser'",
		eventID,
		owner.ID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Read(
		t.Context(),
		owner.ID,
		eventID,
		id,
		1,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("revoked organiser retained planning access", err)
	}
	if _, err = s.Change(
		t.Context(),
		owner.ID,
		eventID,
		id,
		4,
		Command{Action: "save", Definition: &d, Reason: "Revoked edit"},
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("revoked organiser edit", err)
	}
	if _, err = pool.Exec(
		t.Context(),
		"INSERT INTO event_memberships(event_id,person_id,role,active) VALUES($1,$2,'organiser',true)",
		eventID,
		stranger.ID,
	); err != nil {
		t.Fatal(err)
	}
	otherReader, err := s.Read(t.Context(), stranger.ID, eventID, id, 0)
	if err != nil || !slices.Contains(otherReader.PlanningGaps, "owner_unavailable") {
		t.Fatal("unavailable owner hidden as complete planning", otherReader, err)
	}
	listing, err := s.List(t.Context(), stranger.ID, eventID, uuid.Nil, 50)
	if err != nil || len(listing.Items) != 1 ||
		!slices.Contains(listing.Items[0].PlanningGaps, "owner_unavailable") {
		t.Fatal("owner planning gap inconsistent in list", listing, err)
	}
	var count int
	if err = pool.QueryRow(t.Context(), "SELECT count(*) FROM objective_versions WHERE event_id=$1", eventID).
		Scan(&count); err != nil ||
		count != 4 {
		t.Fatal("denied action appended history", count, err)
	}
}
