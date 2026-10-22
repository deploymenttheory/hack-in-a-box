package teams

import (
	"errors"
	"fmt"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestReviewedPublicationEmitsOneDurableChangeAndDraftsEmitNone(t *testing.T) {
	f := allocationDatabase(t, testdb.New(t, true), 4, 3)
	count := func(want int) {
		t.Helper()
		var got int
		if err := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM operations WHERE event_id=$1 AND kind='teams.published'", f.id).
			Scan(&got); err != nil ||
			got != want {
			t.Fatalf("publication events: %d want %d: %v", got, want, err)
		}
	}
	if err := f.act(Command{Action: "publish"}); err != nil {
		t.Fatal(err)
	}
	count(0) // unchanged published roster
	if err := f.act(Command{Action: "unlock", TeamID: f.groups[0]}); err != nil {
		t.Fatal(err)
	}
	old := f.revision()
	if err := f.act(
		Command{Action: "move", TeamID: f.groups[1], PersonID: f.people[1].ID},
	); err != nil {
		t.Fatal(err)
	}
	count(0)
	if _, err := f.s.Act(
		t.Context(),
		f.owner.ID,
		f.id,
		old,
		Command{Action: "publish", Reason: "Stale preview"},
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatalf("stale publication accepted: %v", err)
	}
	count(0)
	revision := f.revision() + 1
	if err := f.act(Command{Action: "publish"}); err != nil {
		t.Fatal(err)
	}
	count(1)
	var key string
	var approved bool
	if err := f.pool.QueryRow(t.Context(), "SELECT operation_key,approved FROM operations WHERE event_id=$1 AND kind='teams.published'", f.id).
		Scan(&key, &approved); err != nil ||
		key != fmt.Sprintf("teams-published:%d", revision) ||
		!approved {
		t.Fatalf("event not tied to committed publication: %q %v %v", key, approved, err)
	}
	members, err := f.q.HackTeamMembers(t.Context(), f.id)
	if err != nil {
		t.Fatal(err)
	}
	for _, member := range members {
		if member.PersonID == f.people[1].ID && member.TeamID != f.groups[1] {
			t.Fatal("publication did not atomically copy reviewed draft")
		}
	}
	if err = f.act(Command{Action: "publish"}); err != nil {
		t.Fatal(err)
	}
	count(1)
	// An organiser's stale screen cannot publish after cancellation/writes closure.
	f.exec(
		"UPDATE events SET stage='cancelled',writes_allowed=false,revision=revision+1 WHERE id=$1",
		f.id,
	)
	before := f.state()
	if err = f.act(Command{Action: "publish"}); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("cancelled event publication accepted: %v", err)
	}
	if before != f.state() {
		t.Fatal("cancelled publication changed durable state")
	}
}

func TestPublicationRechecksConfirmedPlacesAndFreezeAtCommit(t *testing.T) {
	f := allocationDatabase(t, testdb.New(t, true), 3)
	f.exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		f.id,
		f.people[1].ID,
	)
	before := f.state()
	if err := f.act(Command{Action: "publish"}); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("unconfirmed place published: %v", err)
	}
	if f.state() != before {
		t.Fatal("failed publication partially committed")
	}
	f.exec(
		"UPDATE competition_claims SET state='confirmed' WHERE event_id=$1 AND person_id=$2",
		f.id,
		f.people[1].ID,
	)
	revision := f.revision()
	f.now = f.c.TeamFreeze
	if _, err := f.s.Act(
		t.Context(),
		f.owner.ID,
		f.id,
		revision,
		Command{Action: "publish", Reason: "Review queued before cutoff"},
	); !errors.Is(
		err,
		ErrFrozen,
	) {
		t.Fatalf("queued publication crossed hard deadline: %v", err)
	}
}

func TestOrganiserCanCreateReplacementGroupOnlyAfterExplicitUnlock(t *testing.T) {
	f := allocationDatabase(t, testdb.New(t, true), 6, 2)
	command := Command{Action: "create", Name: "Reviewed replacement", PersonID: f.people[1].ID}
	before := f.state()
	if err := f.act(command); !errors.Is(err, ErrLocked) {
		t.Fatalf("new group bypassed locked source: %v", err)
	}
	if before != f.state() {
		t.Fatal("failed creation left a team, membership or entry")
	}
	if err := f.act(Command{Action: "unlock", TeamID: f.groups[0]}); err != nil {
		t.Fatal(err)
	}
	if err := f.act(command); err != nil {
		t.Fatal(err)
	}
	var published int
	if err := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM hack_team_members WHERE event_id=$1 AND team_id=$2 AND snapshot='published'", f.id, f.groups[0]).
		Scan(&published); err != nil ||
		published != 6 {
		t.Fatalf("draft creation changed published membership: %d %v", published, err)
	}
	var events int
	if err := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM operations WHERE event_id=$1", f.id).
		Scan(&events); err != nil ||
		events != 0 {
		t.Fatalf("draft creation emitted delivery: %d %v", events, err)
	}
}

func TestEmptyPublishedGroupRetiresOnlyAtExplicitPublication(t *testing.T) {
	f := allocationDatabase(t, testdb.New(t, true), 3, 3)
	if err := f.act(Command{Action: "unlock", TeamID: f.groups[0]}); err != nil {
		t.Fatal(err)
	}
	for _, person := range f.people[:3] {
		if err := f.act(
			Command{Action: "move", TeamID: f.groups[1], PersonID: person.ID},
		); err != nil {
			t.Fatal(err)
		}
	}
	before := f.state()
	if err := f.act(Command{Action: "disband", TeamID: f.groups[0]}); !errors.Is(err, ErrUnready) {
		t.Fatalf("draft retirement affected published group: %v", err)
	}
	if before != f.state() {
		t.Fatal("unpublished retirement changed state")
	}
	group, err := f.q.HackTeam(t.Context(), dbgen.HackTeamParams{EventID: f.id, ID: f.groups[0]})
	if err != nil || group.State != "published" {
		t.Fatalf("published team retired early: %+v %v", group, err)
	}
	if err = f.act(Command{Action: "publish"}); err != nil {
		t.Fatal(err)
	}
	group, err = f.q.HackTeam(t.Context(), dbgen.HackTeamParams{EventID: f.id, ID: f.groups[0]})
	if err != nil || group.State != "disbanded" {
		t.Fatalf("empty unlocked team not retired atomically: %+v %v", group, err)
	}
}
