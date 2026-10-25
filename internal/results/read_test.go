package results

import (
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestPrivatePreparationRequiresCurrentOrganiserAndReportsMissingNativeRun(t *testing.T) {
	pool := testdb.New(t, true)
	owner := testdb.Person(t, pool)
	judge := testdb.Person(t, pool)
	outsider := testdb.Person(t, pool)
	eventID := testdb.Event(t, pool, owner.ID)
	now := time.Date(2027, 6, 20, 16, 0, 0, 0, time.UTC)
	dates := event.DefaultConfiguration()
	dates.SubmissionCutoff = now.Add(-time.Hour)
	raw, err := json.Marshal(dates)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(t.Context(), "UPDATE events SET stage='assessment',configuration=$2,published_at=$3 WHERE id=$1", eventID, raw, now.AddDate(0, 0, -7)); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(t.Context(), "INSERT INTO event_memberships(event_id,person_id,role) VALUES($1,$2,'judge')", eventID, judge.ID); err != nil {
		t.Fatal(err)
	}
	service := Service{Pool: pool, Now: func() time.Time { return now }}
	page, err := service.Read(t.Context(), owner.ID, eventID)
	if err != nil || page.Current || len(page.Preview.Blockers) != 1 || page.Preview.Blockers[0] != "current_assessment_run_required" || len(page.Preview.Tallies) != 0 || len(page.Announcements) != 0 {
		t.Fatalf("missing source was presented as ready: %+v %v", page, err)
	}
	for _, actor := range []struct {
		name string
		id   uuid.UUID
	}{{"judge", judge.ID}, {"outsider", outsider.ID}} {
		if _, err = service.Read(t.Context(), actor.id, eventID); !errors.Is(err, access.ErrForbidden) {
			t.Fatalf("%s read private result preparation: %v", actor.name, err)
		}
	}
	foreign := testdb.Event(t, pool, outsider.ID)
	if _, err = service.Read(t.Context(), owner.ID, foreign); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("foreign event preparation exposed: %v", err)
	}
}
