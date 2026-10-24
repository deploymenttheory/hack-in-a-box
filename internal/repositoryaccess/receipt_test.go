package repositoryaccess

import (
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestOriginalRepositoryReceiptCurrentAuthorityAndNoEffectsPostgres(t *testing.T) {
	pool := testdb.New(t, true)
	f := setupAccess(t, pool)
	first := f.review()
	f.approve(first)
	// A newer plan and unrelated event revision cannot replace a historical receipt.
	latest := f.review()
	f.exec("UPDATE events SET revision=revision+1 WHERE id=$1", f.event)
	current, err := dbgen.New(pool).Event(t.Context(), f.event)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Change(t.Context(), f.owner, f.event, f.team, current.Revision-1, Command{Action: "approve", PlanID: first.ID, PlanVersion: first.Version, Reason: "Approve exact reviewed access"}); !errors.Is(err, access.ErrConflict) {
		t.Fatalf("old tuple did not retain conflict: %v", err)
	}
	// No provider is supplied: receipt lookup must be independent of live GitHub.
	reader := Service{Pool: pool}
	counts := func() [3]int64 {
		t.Helper()
		var out [3]int64
		if err := pool.QueryRow(t.Context(), `SELECT revision,(SELECT count(*) FROM audit_events WHERE event_id=$1),(SELECT count(*) FROM operations WHERE event_id=$1) FROM events WHERE id=$1`, f.event).Scan(&out[0], &out[1], &out[2]); err != nil {
			t.Fatal(err)
		}
		return out
	}
	before := counts()
	for range 2 {
		review, err := reader.ReadReceipt(t.Context(), f.owner, f.event, f.team, first.ID, "review")
		if err != nil {
			t.Fatal(err)
		}
		if review.ActorID == nil || *review.ActorID != f.owner || review.Plan == nil || !reflect.DeepEqual(*review.Plan, first) || review.Plan.ID == latest.ID || review.Plan.ApprovedAt != nil {
			t.Fatalf("original review not immutable: %#v", review)
		}
		approved, err := reader.ReadReceipt(t.Context(), f.owner, f.event, f.team, first.ID, "approve")
		if err != nil {
			t.Fatal(err)
		}
		if approved.Plan == nil || approved.Plan.ID != first.ID || approved.Plan.Version != first.Version || approved.Plan.ApprovedBy == nil || *approved.Plan.ApprovedBy != f.owner || approved.Plan.ApprovalReason != "Approve exact reviewed access" || approved.Plan.ApprovedAt == nil || approved.ActorID == nil || *approved.ActorID != f.owner {
			t.Fatalf("original approval not independently read: %#v", approved)
		}
	}
	if after := counts(); after != before {
		t.Fatalf("receipt read changed event/audit/jobs: %v to %v", before, after)
	}
	absent, err := reader.ReadReceipt(t.Context(), f.owner, f.event, f.team, uuid.New(), "review")
	if err != nil || absent.Plan != nil || absent.ActorID != nil {
		t.Fatalf("missing receipt fabricated: %#v %v", absent, err)
	}
	pending, err := reader.ReadReceipt(t.Context(), f.owner, f.event, f.team, latest.ID, "approve")
	if err != nil || pending.Plan != nil || pending.ActorID != nil {
		t.Fatalf("unapproved plan fabricated approval: %#v %v", pending, err)
	}
	other := testdb.Person(t, pool)
	f.exec("INSERT INTO event_memberships(event_id,person_id,role,active)VALUES($1,$2,'organiser',true)", f.event, other.ID)
	hidden, err := reader.ReadReceipt(t.Context(), other.ID, f.event, f.team, first.ID, "review")
	if err != nil || hidden.Plan != nil || hidden.ActorID != nil {
		t.Fatalf("another actor's command exposed: %#v %v", hidden, err)
	}
	otherEvent := testdb.Event(t, pool, other.ID)
	if _, err = reader.ReadReceipt(t.Context(), f.owner, otherEvent, f.team, first.ID, "review"); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("wrong event authority: %v", err)
	}
	if _, err = reader.ReadReceipt(t.Context(), f.member, f.event, f.team, first.ID, "review"); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("member read organiser receipt: %v", err)
	}
	f.exec("UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='organiser'", f.event, f.owner)
	if _, err = reader.ReadReceipt(t.Context(), f.owner, f.event, f.team, first.ID, "review"); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("revoked actor read old receipt: %v", err)
	}
	f.exec("UPDATE event_memberships SET active=true WHERE event_id=$1 AND person_id=$2 AND role='organiser'", f.event, f.owner)
	if _, err = pool.Exec(t.Context(), "UPDATE team_repositories SET github_repository_id=github_repository_id+1000 WHERE event_id=$1 AND team_id=$2", f.event, f.team); err == nil {
		t.Fatal("immutable native numeric repository was replaced")
	}
	retained, err := reader.ReadReceipt(t.Context(), f.owner, f.event, f.team, first.ID, "review")
	if err != nil || retained.Plan == nil || retained.Plan.RepositoryID != first.RepositoryID {
		t.Fatalf("original numeric repository binding lost: %#v %v", retained, err)
	}
	for _, action := range []string{"refresh", "", "invalid"} {
		if _, err = reader.ReadReceipt(t.Context(), f.owner, f.event, f.team, first.ID, action); !errors.Is(err, access.ErrInvalid) {
			t.Fatalf("invalid receipt action accepted: %q %v", action, err)
		}
	}
}
