package content_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestRequiredContentAcknowledgements(t *testing.T) {
	pool := testdb.New(t, true)
	t.Run("explicit version, idempotent concurrent retries and material update", func(t *testing.T) {
		f := newFixture(t, pool)
		in := input("page")
		in.Document.RequireAcknowledgement = true
		p := f.act(f.approve(f.save(content.Page{}, in)), "release")
		var wg sync.WaitGroup
		for range 6 {
			wg.Go(func() {
				got, err := f.s.Acknowledge(t.Context(), f.member.ID, f.id, p.ID, 1)
				if err != nil || got.AcknowledgedAt == nil {
					t.Errorf("acknowledge: %+v %v", got, err)
				}
			})
		}
		wg.Wait()
		var count int
		if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM content_acknowledgements WHERE event_id=$1", f.id).Scan(&count); err != nil || count != 1 {
			t.Fatalf("duplicate receipt %d %v", count, err)
		}
		if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM audit_events WHERE event_id=$1 AND action='content.acknowledged'", f.id).Scan(&count); err != nil || count != 1 {
			t.Fatalf("duplicate audit %d %v", count, err)
		}
		in.Document.Markdown = "Materially updated instructions"
		p = f.save(p, in)
		old, err := f.s.Read(t.Context(), f.member.ID, f.id, p.ID, false, 0)
		if err != nil || old.Version != 1 || old.AcknowledgedAt == nil {
			t.Fatal("private draft invalidated released receipt")
		}
		p = f.act(f.approve(p), "release")
		current, err := f.s.Read(t.Context(), f.member.ID, f.id, p.ID, false, 0)
		if err != nil || current.Version != 2 || current.AcknowledgedAt != nil {
			t.Fatal("receipt carried to new version")
		}
		if _, err = f.s.Acknowledge(t.Context(), f.member.ID, f.id, p.ID, 1); !errors.Is(err, access.ErrConflict) {
			t.Fatalf("stale version acknowledged: %v", err)
		}
		current, err = f.s.Acknowledge(t.Context(), f.member.ID, f.id, p.ID, 2)
		if err != nil || current.AcknowledgedAt == nil {
			t.Fatal(err)
		}
		if f.preview(p).AcknowledgedAt != nil {
			t.Fatal("another person's receipt exposed")
		}
		if _, err = pool.Exec(t.Context(), "UPDATE content_acknowledgements SET version=2 WHERE event_id=$1", f.id); err == nil {
			t.Fatal("receipt mutable")
		}
	})
	t.Run("current audience, event, withdrawal and writability enforced", func(t *testing.T) {
		f := newFixture(t, pool)
		in := input("page")
		in.Document.RequireAcknowledgement = true
		p := f.act(f.approve(f.save(content.Page{}, in)), "release")
		if _, err := f.s.Acknowledge(t.Context(), f.other.ID, f.id, p.ID, 1); !errors.Is(err, access.ErrNotFound) {
			t.Fatalf("outsider acknowledged: %v", err)
		}
		other := newFixture(t, pool)
		if _, err := f.s.Acknowledge(t.Context(), f.member.ID, other.id, p.ID, 1); !errors.Is(err, access.ErrNotFound) {
			t.Fatalf("cross-event receipt: %v", err)
		}
		if _, err := f.s.Acknowledge(t.Context(), f.member.ID, f.id, p.ID, 1); err != nil {
			t.Fatal(err)
		}
		f.exec("UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2", f.id, f.member.ID)
		if _, err := f.s.Acknowledge(t.Context(), f.member.ID, f.id, p.ID, 1); !errors.Is(err, access.ErrNotFound) {
			t.Fatalf("revoked retry accepted: %v", err)
		}
		f.exec("UPDATE competition_claims SET state='confirmed' WHERE event_id=$1 AND person_id=$2", f.id, f.member.ID)
		f.exec("UPDATE events SET writes_allowed=false WHERE id=$1", f.id)
		if _, err := f.s.Acknowledge(t.Context(), f.member.ID, f.id, p.ID, 1); !errors.Is(err, access.ErrForbidden) {
			t.Fatalf("read-only receipt: %v", err)
		}
		f.exec("UPDATE events SET writes_allowed=true WHERE id=$1", f.id)
		f.act(p, "withdraw")
		if _, err := f.s.Acknowledge(t.Context(), f.member.ID, f.id, p.ID, 1); !errors.Is(err, access.ErrNotFound) {
			t.Fatalf("withdrawn content acknowledged: %v", err)
		}
	})
	t.Run("optional reading and previews cannot create receipts", func(t *testing.T) {
		f := newFixture(t, pool)
		p := f.act(f.approve(f.save(content.Page{}, input("page"))), "release")
		_, err := f.s.Acknowledge(t.Context(), f.member.ID, f.id, p.ID, 1)
		expectBlocked(t, err)
		in := input("challenge")
		in.Document.RequireAcknowledgement = true
		p = f.approve(f.save(content.Page{}, in))
		for _, actor := range []uuid.UUID{f.member.ID, f.owner.ID} {
			if _, err = f.s.Acknowledge(t.Context(), actor, f.id, p.ID, 1); !errors.Is(err, access.ErrNotFound) {
				t.Fatalf("early acknowledgement exposed challenge: %v", err)
			}
		}
		f.now = f.due
		f.dispatch(p)
		if _, err = f.s.Acknowledge(t.Context(), f.member.ID, f.id, p.ID, 1); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("release races cannot acknowledge unseen updated text", func(t *testing.T) {
		f := newFixture(t, pool)
		in := input("page")
		in.Document.RequireAcknowledgement = true
		p := f.act(f.approve(f.save(content.Page{}, in)), "release")
		in.Document.Markdown = "Updated required reading"
		p = f.approve(f.save(p, in))
		var wg sync.WaitGroup
		wg.Go(func() {
			_, err := f.s.Acknowledge(t.Context(), f.member.ID, f.id, p.ID, 1)
			if err != nil && !errors.Is(err, access.ErrConflict) {
				t.Error(err)
			}
		})
		wg.Go(func() {
			_, err := f.s.Action(t.Context(), f.owner.ID, f.id, p.ID, p.Revision, "release", "")
			if err != nil {
				t.Error(err)
			}
		})
		wg.Wait()
		current, err := f.s.Read(t.Context(), f.member.ID, f.id, p.ID, false, 0)
		if err != nil || current.Version != 2 || current.AcknowledgedAt != nil {
			t.Fatal("old click acknowledged new version")
		}
	})
}

func TestReleasedContentSearchAndPagination(t *testing.T) {
	pool := testdb.New(t, true)
	f := newFixture(t, pool)
	in := input("page")
	in.Document.Title = "Findable released guide"
	first := f.act(f.approve(f.save(content.Page{}, in)), "release")
	in.Document.Title = "Another findable guide"
	f.act(f.approve(f.save(content.Page{}, in)), "release")
	in.Document.Title = "Secret unreleased title"
	f.save(content.Page{}, in)
	in.Document.Title = "Judge findable guidance"
	in.Document.Audience = access.ContentAudience{Roles: []string{"judge"}, Teams: []uuid.UUID{}}
	f.act(f.approve(f.save(content.Page{}, in)), "release")
	f.exec("INSERT INTO event_memberships(event_id,person_id,role,active) VALUES($1,$2,'judge',true)", f.id, f.other.ID)
	in = input("page")
	in.Document.Title = "Secret edited title"
	f.save(first, in)
	result, err := f.s.Search(t.Context(), f.member.ID, f.id, uuid.Nil, 1, false, " FINDABLE ")
	if err != nil || len(result.Items) != 1 || result.NextCursor == nil {
		t.Fatalf("search first page: %+v %v", result, err)
	}
	next, err := f.s.Search(t.Context(), f.member.ID, f.id, *result.NextCursor, 1, false, "findable")
	if err != nil || len(next.Items) != 1 || next.NextCursor != nil || next.Items[0].ID == result.Items[0].ID {
		t.Fatalf("search second page: %+v %v", next, err)
	}
	hidden, err := f.s.Search(t.Context(), f.member.ID, f.id, uuid.Nil, 30, false, "secret")
	if err != nil || len(hidden.Items) != 0 || hidden.NextCursor != nil {
		t.Fatal("private match exposed")
	}
	judges, err := f.s.Search(t.Context(), f.other.ID, f.id, uuid.Nil, 30, false, "findable")
	if err != nil || len(judges.Items) != 1 || judges.Items[0].Document.Title != "Judge findable guidance" {
		t.Fatalf("role search: %+v %v", judges, err)
	}
}
