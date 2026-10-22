package rubric_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestReleasedQuestionSourceUsesActualCommonPublicationAndCurrentReader(t *testing.T) {
	f := publicationSetup(t)
	ref := f.configuration.Reference
	challenge := f.challenge(content.Page{}, &ref)
	read := func(actor uuid.UUID, now time.Time) (rubric.ReleasedSource, error) {
		var result rubric.ReleasedSource
		err := database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
			var err error
			result, err = rubric.ReleasedSourceForReaderInTransaction(
				t.Context(),
				tx,
				actor,
				f.eventID,
				now,
			)
			return err
		})
		return result, err
	}
	denied := func(actor uuid.UUID, now time.Time) {
		t.Helper()
		result, err := read(actor, now)
		if !errors.Is(err, access.ErrNotFound) || result != (rubric.ReleasedSource{}) {
			t.Fatalf("unreleased or revoked question source disclosed: %+v, %v", result, err)
		}
	}
	denied(f.owner.ID, f.due)
	denied(f.first.ID, f.due)
	f.now = f.due
	if err := f.content.Dispatch(
		t.Context(),
		f.eventID,
		challenge.ID,
		challenge.Revision,
	); err != nil {
		t.Fatal(err)
	}
	denied(f.owner.ID, f.due.Add(-time.Nanosecond))
	for _, actor := range []uuid.UUID{f.first.ID, f.second.ID, f.owner.ID} {
		result, err := read(actor, f.due)
		if err != nil || result.ChallengeID != challenge.ID ||
			result.ChallengeVersion != challenge.Version ||
			result.Reference != ref {
			t.Fatalf("common exact source mismatch: %+v, %v", result, err)
		}
	}
	outsider := testdb.Person(t, f.pool)
	f.exec(
		"INSERT INTO event_memberships(event_id,person_id,role,active) VALUES($1,$2,'mentor',true)",
		f.eventID,
		outsider.ID,
	)
	denied(outsider.ID, f.due)
	f.exec("UPDATE events SET writes_allowed=false WHERE id=$1", f.eventID)
	if _, err := read(f.first.ID, f.due); err != nil {
		t.Fatal("archive erased current-authority historical source", err)
	}
	f.exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		f.eventID,
		f.first.ID,
	)
	denied(f.first.ID, f.due)
	f.exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='organiser'",
		f.eventID,
		f.owner.ID,
	)
	denied(f.owner.ID, f.due)
}
