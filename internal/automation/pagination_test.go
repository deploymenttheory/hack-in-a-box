package automation

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestCurrentBindingControllerAndIndependentStatusPagesHaveNoLifetimeCap(t *testing.T) {
	f := newFixture(t)
	f.campaign("reminder", "Reviewed scheduled notice", true)
	prototype := Binding{
		Definition: Definition{
			ID:                     uuid.Nil,
			CampaignID:             f.campaignID,
			CampaignVersion:        f.campaignVersion,
			Trigger:                "scheduled",
			MaximumLatenessSeconds: 3600,
		},
		Version: 1,
		ActorID: f.owner.ID,
	}
	raw, _ := json.Marshal(prototype)
	f.exec(
		`WITH ids AS(SELECT gen_random_uuid() id FROM generate_series(1,1005)) INSERT INTO communication_automation_versions(event_id,id,version,campaign_id,campaign_version,document,actor_id,reason)SELECT $1,id,1,$2,$3,jsonb_set($4::jsonb,'{definition,id}',to_jsonb(id::text)),$5,'Synthetic current-binding paging fixture' FROM ids`,
		f.id,
		f.campaignID,
		f.campaignVersion,
		raw,
		f.owner.ID,
	)
	var last uuid.UUID
	if err := f.pool.QueryRow(t.Context(), `SELECT max(id::text)::uuid FROM communication_automation_versions WHERE event_id=$1`, f.id).
		Scan(&last); err != nil {
		t.Fatal(err)
	}
	f.change(
		Command{
			Action:    "approve",
			BindingID: last,
			Version:   1,
			Reason:    "Approve final binding beyond the old cap",
		},
	)
	if err := f.s.reconcileEvent(t.Context(), f.id); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM communication_occurrences WHERE event_id=$1 AND binding_id=$2`, f.id, last).
		Scan(&count); err != nil ||
		count != 1 {
		t.Fatalf("controller lost approved binding beyond old cap: %d %v", count, err)
	}
	f.exec(
		`INSERT INTO communication_occurrences(event_id,id,binding_id,binding_version,source_kind,source_id,source_revision,source_key,due_at,deadline,scope_binding,state)SELECT $1,gen_random_uuid(),$2,2,'synthetic',gen_random_uuid(),1,'fixture:'||n,$3,$3,'retained','cancelled' FROM generate_series(1,1005)n`,
		f.id,
		last,
		f.now,
	)
	seenBindings, seenOccurrences := map[uuid.UUID]bool{}, map[uuid.UUID]bool{}
	bindingsAfter, occurrencesAfter := uuid.Nil, uuid.Nil
	for {
		page, err := f.s.ReadPage(
			t.Context(),
			f.owner.ID,
			f.id,
			bindingsAfter,
			occurrencesAfter,
			100,
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Bindings) > 100 || len(page.Occurrences) > 100 {
			t.Fatal("status response exceeded page bound")
		}
		for _, b := range page.Bindings {
			if seenBindings[b.Definition.ID] {
				t.Fatal("binding repeated across pages")
			}
			seenBindings[b.Definition.ID] = true
			bindingsAfter = b.Definition.ID
		}
		for _, o := range page.Occurrences {
			if seenOccurrences[o.ID] {
				t.Fatal("occurrence repeated across pages")
			}
			seenOccurrences[o.ID] = true
			occurrencesAfter = o.ID
		}
		if page.NextBindingCursor == nil && page.NextOccurrenceCursor == nil {
			break
		}
	}
	if len(seenBindings) != 1005 || len(seenOccurrences) != 1006 {
		t.Fatalf(
			"status lifetime truncation: bindings%d occurrences%d",
			len(seenBindings),
			len(seenOccurrences),
		)
	}
	empty, err := f.s.ReadPage(t.Context(), f.owner.ID, f.id, bindingsAfter, occurrencesAfter, 100)
	if err != nil || len(empty.Bindings) != 0 || len(empty.Occurrences) != 0 {
		t.Fatalf("exhausted cursors did not stay empty: %+v %v", empty, err)
	}
	// The two collections can be independently exhausted/restarted.
	unequal, err := f.s.ReadPage(t.Context(), f.owner.ID, f.id, bindingsAfter, uuid.Nil, 1)
	if err != nil || len(unequal.Bindings) != 0 || len(unequal.Occurrences) != 1 ||
		unequal.NextOccurrenceCursor == nil {
		t.Fatalf("independent collection cursor lost: %+v %v", unequal, err)
	}
	f.exec(
		`UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2`,
		f.id,
		f.owner.ID,
	)
	if _, err = f.s.ReadPage(
		t.Context(),
		f.owner.ID,
		f.id,
		bindingsAfter,
		occurrencesAfter,
		100,
	); err == nil {
		t.Fatal("continuation cursor bypassed current authority")
	}
}
