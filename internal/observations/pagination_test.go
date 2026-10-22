package observations

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
)

func TestObservationPaginationRetainsEveryAcceptedNoteAndRechecksAuthority(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	f.exec(`INSERT INTO mentor_observation_versions(event_id,team_id,id,version,author_id,observed_at,changed_at,category,evidence,outcome)
 SELECT $1,$2,gen_random_uuid(),1,$3,$4,$4,'technical','Synthetic retained evidence','Synthetic outcome' FROM generate_series(1,1005)`, f.event, f.team, f.primary.ID, f.now)
	accepted := input()
	page, err := f.s.Change(ctx, f.primary.ID, f.event, f.team, f.revision(), accepted)
	if err != nil || len(page.Observations) != 50 || page.NextCursor == nil {
		t.Fatalf("accepted write returned an error or unbounded response: count=%d err=%v", len(page.Observations), err)
	}
	seen := map[uuid.UUID]bool{}
	var cursor *uuid.UUID
	for {
		page, err = f.s.ReadPage(ctx, f.primary.ID, f.event, f.team, ReadOptions{Cursor: cursor, Limit: 73})
		if err != nil || len(page.Observations) > 73 {
			t.Fatalf("bounded current-note page unavailable: %v", err)
		}
		for _, note := range page.Observations {
			if seen[note.ID] {
				t.Fatal("current-note cursor repeated an observation")
			}
			seen[note.ID] = true
		}
		if page.NextCursor == nil {
			break
		}
		cursor = page.NextCursor
	}
	if len(seen) != 1006 || !seen[accepted.ID] {
		t.Fatalf("accepted notes were lost: count=%d accepted=%t", len(seen), seen[accepted.ID])
	}
	f.exec("UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mentor'", f.event, f.primary.ID)
	if _, err = f.s.ReadPage(ctx, f.primary.ID, f.event, f.team, ReadOptions{Cursor: cursor, Limit: 73}); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("a cursor retained revoked authority: %v", err)
	}
}

func TestObservationAndStatusHistoryCursorsRetainEveryVersion(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	id := uuid.New()
	f.exec(`INSERT INTO mentor_observation_versions(event_id,team_id,id,version,author_id,observed_at,changed_at,category,evidence,outcome)
 SELECT $1,$2,$3,n,$4,$5,$5,'technical','Synthetic retained evidence','Synthetic outcome' FROM generate_series(1,1005) n`, f.event, f.team, id, f.primary.ID, f.now)
	f.exec(`INSERT INTO mentor_team_status_versions(event_id,team_id,version,actor_id,state,reason,next_action,changed_at)
 SELECT $1,$2,n,$3,'on_track','Synthetic retained reason','Synthetic action',$4 FROM generate_series(1,1005) n`, f.event, f.team, f.primary.ID, f.now)
	note := input()
	note.ID, note.Version = id, 1005
	if _, err := f.s.Change(ctx, f.primary.ID, f.event, f.team, f.revision(), note); err != nil {
		t.Fatal("valid retained observation edit failed", err)
	}
	if _, err := f.s.Change(ctx, f.second.ID, f.event, f.team, f.revision(), Command{Action: "status", Version: 1005, State: "at_risk", Reason: "Synthetic updated operational need"}); err != nil {
		t.Fatal("valid operational status failed", err)
	}
	options := HistoryOptions{ObservationID: id, Limit: 83}
	notes, statuses := map[int64]bool{}, map[int64]bool{}
	for {
		page, err := f.s.HistoryPage(ctx, f.second.ID, f.event, f.team, options)
		if err != nil || len(page.Observations) > 83 || len(page.Statuses) > 83 {
			t.Fatalf("bounded history page failed: %v", err)
		}
		for _, note := range page.Observations {
			if notes[note.Version] || note.AuthorID != f.primary.ID {
				t.Fatal("history repeated a version or changed original authorship")
			}
			notes[note.Version] = true
		}
		for _, status := range page.Statuses {
			if statuses[status.Version] {
				t.Fatal("status history repeated a version")
			}
			statuses[status.Version] = true
		}
		if page.NextObservationCursor == nil && page.NextStatusCursor == nil {
			break
		}
		options.ObservationCursor, options.StatusCursor = page.NextObservationCursor, page.NextStatusCursor
	}
	if len(notes) != 1006 || len(statuses) != 1006 || !notes[1] || !statuses[1] {
		t.Fatalf("retained history was truncated: notes=%d statuses=%d", len(notes), len(statuses))
	}
	end := int64(1)
	options.ObservationCursor, options.StatusCursor = &end, &end
	page, err := f.s.HistoryPage(ctx, f.primary.ID, f.event, f.team, options)
	if err != nil || len(page.Observations) != 0 || len(page.Statuses) != 0 {
		t.Fatalf("valid history exhaustion returned an error: %v", err)
	}
	f.exec("UPDATE mentor_assignments SET snapshot='draft' WHERE event_id=$1 AND person_id=$2", f.event, f.primary.ID)
	if _, err = f.s.HistoryPage(ctx, f.primary.ID, f.event, f.team, options); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("history cursor retained a withdrawn assignment: %v", err)
	}
}
