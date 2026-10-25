package qualification

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestQualificationHistoryKeepsOriginalAudienceAndExclusiveVersionPages(t *testing.T) {
	f, s, run := receiptFixture(t)
	first, err := s.Change(t.Context(), f.owner.ID, f.event, run, f.revision(), panelCommand(f))
	if err != nil {
		t.Fatal(err)
	}
	outsider := testdb.Person(t, f.pool)
	f.exec(
		"INSERT INTO event_memberships(event_id,person_id,role)VALUES($1,$2,'mentor')",
		f.event,
		outsider.ID,
	)
	replacement := panelCommand(f)
	replacement.Version = first.Round.Version
	replacement.Panel.MentorIDs = []uuid.UUID{outsider.ID}
	replacement.Reason = "A newly recorded result audience"
	if _, err = s.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		run,
		f.revision(),
		replacement,
	); err != nil {
		t.Fatal(err)
	}
	reader, err := s.History(t.Context(), outsider.ID, f.event, run, nil, 20)
	if err != nil || len(reader.Items) != 1 || reader.Items[0].Snapshot.Version != 2 ||
		reader.Items[0].Current {
		t.Fatal("new panel member acquired old audience or current selection", reader, err)
	}
	if _, err = s.History(
		t.Context(),
		f.primary.ID,
		f.event,
		run,
		nil,
		20,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("removed panel member read former private history", err)
	}
	owner, err := s.History(t.Context(), f.owner.ID, f.event, run, nil, 1)
	if err != nil || len(owner.Items) != 1 || owner.NextCursor == nil ||
		owner.Items[0].Snapshot.Version != 2 {
		t.Fatal("history first keyset", owner, err)
	}
	older, err := s.History(t.Context(), f.owner.ID, f.event, run, owner.NextCursor, 1)
	if err != nil || len(older.Items) != 1 || older.Items[0].Snapshot.Version != 1 ||
		older.NextCursor != nil {
		t.Fatal("history exclusive keyset", older, err)
	}
}
