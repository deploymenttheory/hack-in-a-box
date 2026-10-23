package moderation

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestHuddleAndDecisionHistoryKeysetsBeyondOneThousandRecheckCurrentPanel(t *testing.T) {
	f := setup(t)
	s := f.huddles()
	h := f.create(s, f.checkpoint)
	// Explicit immutable metadata fixtures test paging only. Native create/open/
	// sign-off journeys and their exact authority are exercised separately.
	raw, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(
		`INSERT INTO moderation_huddle_versions(event_id,team_id,id,version,run_id,document,actor_id,reason,changed_at)SELECT $1,$2,$3,i,$4,jsonb_set($5::jsonb,'{version}',to_jsonb(i)),$6,'Synthetic retained history',$7 FROM generate_series(2,1006)i`,
		f.event,
		f.team,
		h.Definition.ID,
		h.Checkpoint.Definition.RunID,
		raw,
		f.owner.ID,
		f.now,
	)
	var before *int64
	versions := map[int64]bool{}
	for {
		page, e := s.History(t.Context(), f.owner.ID, f.event, f.team, h.Definition.ID, before, 100)
		if e != nil {
			t.Fatal(e)
		}
		if len(page.Items) > 100 {
			t.Fatal("unbounded history page")
		}
		for _, item := range page.Items {
			if versions[item.Version] {
				t.Fatal("history duplicate", item.Version)
			}
			versions[item.Version] = true
		}
		if page.NextCursor == nil {
			break
		}
		before = page.NextCursor
	}
	if len(versions) != 1006 {
		t.Fatal("history permanently truncated", len(versions))
	}
	raw, err = json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(
		`INSERT INTO moderation_huddle_versions(event_id,team_id,id,version,run_id,document,actor_id,reason,changed_at)SELECT $1,$2,ids.id,1,$3,jsonb_set($4::jsonb,'{definition,id}',to_jsonb(ids.id::text)),$5,'Synthetic intended metadata',$6 FROM(SELECT gen_random_uuid()id FROM generate_series(1,1005))ids`,
		f.event,
		f.team,
		h.Checkpoint.Definition.RunID,
		raw,
		f.owner.ID,
		f.now,
	)
	after := uuid.Nil
	ids := map[uuid.UUID]bool{}
	for {
		page, e := s.List(t.Context(), f.owner.ID, f.event, f.team, after, 100)
		if e != nil {
			t.Fatal(e)
		}
		if len(page.Items) > 100 {
			t.Fatal("unbounded huddle page")
		}
		for _, item := range page.Items {
			if ids[item.Definition.ID] {
				t.Fatal("huddle duplicate")
			}
			ids[item.Definition.ID] = true
		}
		if page.NextCursor == nil {
			break
		}
		after = *page.NextCursor
	}
	if len(ids) != 1006 {
		t.Fatal("huddle metadata permanently truncated", len(ids))
	}
	outsider := testdb.Person(t, f.pool)
	f.exec(
		`INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'organiser')`,
		f.event,
		outsider.ID,
	)
	definition := h.Definition
	definition.ModeratorID = outsider.ID
	current, err := s.Read(t.Context(), f.owner.ID, f.event, f.team, h.Definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		f.team,
		h.Definition.ID,
		f.revision(),
		Command{
			Action:     "configure",
			Version:    current.Version,
			Definition: &definition,
			Reason:     "Explicitly replace intended moderator before opening",
		},
	); err != nil {
		t.Fatal(err)
	}
	if _, err = s.History(
		t.Context(),
		f.owner.ID,
		f.event,
		f.team,
		h.Definition.ID,
		nil,
		100,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("former panel member retained historical private context", err)
	}
	f.exec(`UPDATE people SET active=false WHERE id=$1`, f.owner.ID)
	if _, err = s.List(
		t.Context(),
		f.owner.ID,
		f.event,
		f.team,
		after,
		100,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("revoked actor retained later cursor page", err)
	}
}
