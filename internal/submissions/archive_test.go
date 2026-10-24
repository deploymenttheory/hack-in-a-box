package submissions_test

import (
	"errors"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

// Cancellation before a hack followed by archival is a real lifecycle path:
// there need not ever have been a formal submission outcome to preserve.
func TestNeverHeldArchiveDoesNotInventSubmissionOutcomes(t *testing.T) {
	pool := testdb.New(t, true)
	f := newFixture(t, pool)
	f.now = f.c.LiveStart.Add(-time.Hour)
	events := event.Service{Pool: pool, Clock: func() time.Time { return f.now }, SettleSubmissions: f.s.Settle}
	row, err := f.q.Event(t.Context(), f.id)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := events.Transition(t.Context(), f.owner.ID, f.id, row.Revision, "cancelled", "Synthetic never-held cancellation", false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = events.Transition(t.Context(), f.owner.ID, f.id, cancelled.Revision, "archived", "", false)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		t.Helper()
		var result string
		err := pool.QueryRow(t.Context(), `SELECT json_build_object(
   'states',(SELECT json_agg(s ORDER BY s.team_id) FROM submission_states s WHERE event_id=$1),
   'changes',(SELECT json_agg(c ORDER BY c.team_id,c.revision) FROM submission_changes c WHERE event_id=$1),
   'facts',(SELECT row_to_json(f) FROM event_gate_facts f WHERE event_id=$1),
   'event',(SELECT row_to_json(e) FROM events e WHERE id=$1))::text`, f.id).Scan(&result)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	before := snapshot()
	f.now = f.c.SubmissionCutoff.Add(time.Hour)
	if err = f.s.Maintain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(); after != before {
		t.Fatalf("maintenance mutated archived competition: before=%s after=%s", before, after)
	}
	if _, err = f.s.Read(t.Context(), f.owner.ID, f.id, f.team); !errors.Is(err, access.ErrNotFound) {
		t.Fatalf("missing archived outcome must remain unavailable: %v", err)
	}
	if _, err = f.s.History(t.Context(), f.owner.ID, f.id, f.team, 0, 10); !errors.Is(err, access.ErrNotFound) {
		t.Fatalf("history invented archived outcome: %v", err)
	}
	if after := snapshot(); after != before {
		t.Fatalf("archive reads mutated competition: before=%s after=%s", before, after)
	}
}
