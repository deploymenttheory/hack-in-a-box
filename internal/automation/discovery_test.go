package automation

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func TestLifecycleDiscoveryFindsRequestedSourceBeyondHistoricalPage(t *testing.T) {
	f := newFixture(t)
	f.exec(
		`INSERT INTO operations(id,event_id,operation_key,kind,target_id,due_at,approved,state) SELECT gen_random_uuid(),$1,'historical-'||n,'teams.published',gen_random_uuid(),$2::timestamptz+n*interval '1 second',true,'delivered' FROM generate_series(1,1205) n`,
		f.id,
		f.now,
	)
	var wanted uuid.UUID
	if err := f.pool.QueryRow(t.Context(), `SELECT id FROM operations WHERE event_id=$1 ORDER BY due_at DESC LIMIT 1`, f.id).
		Scan(&wanted); err != nil {
		t.Fatal(err)
	}
	err := database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		sources, err := (builtinSource{service: *f.s, kind: "teams.published"}).DiscoverInTransaction(
			t.Context(),
			tx,
			f.id,
			Definition{SourceID: &wanted},
		)
		if err != nil {
			return err
		}
		if len(sources) != 1 || sources[0].ID != wanted {
			return fmt.Errorf("requested source beyond old page missing: %v", sources)
		}
		all, err := (builtinSource{service: *f.s, kind: "teams.published"}).DiscoverInTransaction(
			t.Context(),
			tx,
			f.id,
			Definition{},
		)
		if err != nil {
			return err
		}
		if len(all) != 1205 {
			return fmt.Errorf("historical sources truncated: %d", len(all))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleUnknownSourceSweepMakesBoundedProgressPerBindingVersion(t *testing.T) {
	f := newFixture(t)
	f.campaign("reminder", "Public message", true)
	page, _ := f.change(
		Command{
			Action: "save",
			Definition: &Definition{
				ID:                     uuid.New(),
				CampaignID:             f.campaignID,
				CampaignVersion:        f.campaignVersion,
				Trigger:                "teams.published",
				MaximumLatenessSeconds: 3600,
			},
			Reason: "Review publication journey",
		},
	)
	binding := page.Bindings[0]
	approvedPage, _ := f.change(
		Command{
			Action:    "approve",
			BindingID: binding.Definition.ID,
			Version:   binding.Version,
			Reason:    "Approve current publication journey",
		},
	)
	f.exec(
		`INSERT INTO operations(id,event_id,operation_key,kind,target_id,due_at,approved,state) SELECT gen_random_uuid(),$1,'historical-'||n,'teams.published',gen_random_uuid(),$2::timestamptz+n*interval '1 second',true,'delivered' FROM generate_series(1,605) n`,
		f.id,
		f.now.Add(-time.Hour),
	)
	for pass := 1; pass <= 4; pass++ {
		if err := f.s.reconcileEvent(t.Context(), f.id); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM communication_occurrences WHERE event_id=$1`, f.id).
			Scan(&count); err != nil {
			t.Fatal(err)
		}
		want := min(pass*200, 605)
		if count != want {
			t.Fatalf("pass %d retained %d unknown sources, want %d", pass, count, want)
		}
	}
	// Insert an earlier key after the sweep: a persisted high-water mark must not skip it.
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	f.exec(
		`INSERT INTO operations(id,event_id,operation_key,kind,target_id,due_at,approved,state) VALUES($1,$2,'late-earlier-key','teams.published',$3,$4,true,'delivered')`,
		id,
		f.id,
		uuid.New(),
		f.now.Add(-2*time.Hour),
	)
	if err := f.s.reconcileEvent(t.Context(), f.id); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM communication_occurrences WHERE event_id=$1 AND source_id=$2`, f.id, id).
		Scan(&count); err != nil ||
		count != 1 {
		t.Fatal("late earlier source skipped", count, err)
	}
	// An explicitly re-reviewed immutable binding version gets its own sweep.
	binding = approvedPage.Bindings[0]
	updated, _ := f.change(
		Command{
			Action:     "save",
			Definition: &binding.Definition,
			Version:    binding.Version,
			Reason:     "Review new binding generation",
		},
	)
	approved, _ := f.change(
		Command{
			Action:    "approve",
			BindingID: binding.Definition.ID,
			Version:   updated.Bindings[0].Version,
			Reason:    "Approve new immutable generation",
		},
	)
	if err := f.s.reconcileEvent(t.Context(), f.id); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM communication_occurrences WHERE event_id=$1 AND binding_version=$2`, f.id, approved.Bindings[0].Version).
		Scan(&count); err != nil ||
		count != 200 {
		t.Fatal("new binding generation lost sources or boundedness", count, err)
	}
}
