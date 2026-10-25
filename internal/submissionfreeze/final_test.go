package submissionfreeze

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestExactFinalSeamRequiresNativeCurrentActorAndDoesNotGateNonRepositoryEvidence(t *testing.T) {
	pool := testdb.New(t, true)
	f := newFixture(t, pool)
	f.submit("url")
	f.now = f.cfg.SubmissionCutoff
	native, err := native(*f.service).Read(t.Context(), f.owner, f.event, f.team)
	if err != nil {
		t.Fatal(err)
	}
	// No repository capture source exists yet. A real file/URL-only frozen final
	// must not acquire a GitHub requirement merely because the worker is delayed.
	err = database.Within(t.Context(), pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(t.Context(), f.event); err != nil {
			return err
		}
		out, err := f.service.EvidenceForFinalInTransaction(
			t.Context(),
			tx,
			f.owner,
			f.event,
			f.team,
			native.EntryID,
			native.Final.Revision,
		)
		if err != nil {
			return err
		}
		if out.Status != "not_applicable" || out.SHA != "" {
			t.Fatal(out)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = database.Within(t.Context(), pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(t.Context(), f.event); err != nil {
			return err
		}
		_, err := f.service.EvidenceForFinalInTransaction(
			t.Context(),
			tx,
			f.other,
			f.event,
			f.team,
			native.EntryID,
			native.Final.Revision,
		)
		return err
	})
	if !errors.Is(err, access.ErrForbidden) {
		t.Fatal("unscoped exact-final metadata granted access", err)
	}
	f.exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		f.event,
		f.member,
	)
	err = database.Within(t.Context(), pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(t.Context(), f.event); err != nil {
			return err
		}
		_, err := f.service.EvidenceForFinalInTransaction(
			t.Context(),
			tx,
			f.member,
			f.event,
			f.team,
			native.EntryID,
			native.Final.Revision,
		)
		return err
	})
	if !errors.Is(err, access.ErrForbidden) {
		t.Fatal("former team member retained exact-final seam", err)
	}
}
