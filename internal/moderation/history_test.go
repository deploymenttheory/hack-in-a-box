package moderation

import (
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestHistoryDoesNotGrantNewModeratorAccessToFormerPanelSnapshots(t *testing.T) {
	f := setup(t)
	s := f.huddles()
	h := f.create(s, f.checkpoint)
	newModerator := testdb.Person(t, f.pool)
	f.exec(
		`INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'organiser')`,
		f.event,
		newModerator.ID,
	)
	definition := h.Definition
	definition.ModeratorID = newModerator.ID
	next := f.change(
		s,
		h,
		f.owner.ID,
		Command{
			Action:     "configure",
			Definition: &definition,
			Reason:     "Explicitly replace intended panel moderator",
		},
	)
	page, err := s.History(t.Context(), newModerator.ID, f.event, f.team, h.Definition.ID, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Version != next.Version {
		t.Fatal("current moderator lost current history", page)
	}
	if page.NextCursor != nil {
		older, err := s.History(
			t.Context(),
			newModerator.ID,
			f.event,
			f.team,
			h.Definition.ID,
			page.NextCursor,
			1,
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(older.Items) != 0 {
			t.Fatal("latest panel granted access to former private panel", older)
		}
	}
	primary, err := s.History(t.Context(), f.primary.ID, f.event, f.team, h.Definition.ID, nil, 1)
	if err != nil || len(primary.Items) != 1 || primary.NextCursor == nil {
		t.Fatal("continuing intended panel member lost legitimate cursor", primary, err)
	}
	older, err := s.History(
		t.Context(),
		f.primary.ID,
		f.event,
		f.team,
		h.Definition.ID,
		primary.NextCursor,
		1,
	)
	if err != nil || len(older.Items) != 1 || older.Items[0].Version != h.Version {
		t.Fatal("continuing panel member lost legitimate history", older, err)
	}
}
