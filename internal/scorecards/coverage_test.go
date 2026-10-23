package scorecards

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func TestCoordinationMetadataDoesNotDiscloseAnotherIndependentCard(t *testing.T) {
	f := setup(t)
	command := f.command(f.checkpoint, "save", false)
	card, err := f.s.Change(t.Context(), f.primary.ID, f.event, f.team, f.revision(), command)
	if err != nil {
		t.Fatal(err)
	}
	read := func(actor uuid.UUID) (Coverage, error) {
		var out Coverage
		err := database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
			var e error
			out, e = f.s.CoverageInTransaction(
				t.Context(),
				tx,
				actor,
				f.event,
				f.team,
				uuid.Nil,
				50,
			)
			return e
		})
		return out, err
	}
	second, err := read(f.second.ID)
	if err != nil || len(second.Items) != 1 || len(second.Items[0].Reviews) != 1 ||
		second.Items[0].Reviews[0].PersonID == nil ||
		*second.Items[0].Reviews[0].PersonID != f.second.ID ||
		second.Items[0].Reviews[0].State != "missing" {
		t.Fatal("second reviewer saw another author's draft completion metadata", second, err)
	}
	own, err := read(f.primary.ID)
	if err != nil || len(own.Items) != 1 || len(own.Items[0].Reviews) != 1 ||
		own.Items[0].Reviews[0].State != "draft" ||
		own.Items[0].Reviews[0].PendingCategories == nil ||
		*own.Items[0].Reviews[0].PendingCategories != len(card.Criteria.Categories) {
		t.Fatal("own pending categories became numeric poor scores", own, err)
	}
	owner, err := read(f.owner.ID)
	if err != nil || len(owner.Items) != 1 || len(owner.Items[0].Reviews) != 2 {
		t.Fatal("organiser lost completion coordination states", owner, err)
	}
	encoded, err := json.Marshal(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"value":`, `"scores":`, `"evidence":`, `"category_id":`, `"binding":`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("coordination metadata leaked private assessment fields", forbidden)
		}
	}
	if _, err = read(f.member.ID); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("competitor gained assessment coordination access", err)
	}
	f.now = f.config.LiveStart.Add(-time.Second)
	before, err := read(f.primary.ID)
	if err != nil || before.Availability != "not_open" || len(before.Items) != 0 {
		t.Fatal("pre-live assessment metadata opened early", before, err)
	}
	f.now = f.config.SubmissionCutoff
	f.exec(
		`UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mentor'`,
		f.event,
		f.primary.ID,
	)
	if _, err = read(f.primary.ID); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("revoked reader retained metadata", err)
	}
	owner, err = read(f.owner.ID)
	if err != nil || owner.Items[0].Reviews[0].State != "unavailable" ||
		owner.Items[0].Reviews[0].PendingCategories != nil {
		t.Fatal("revoked author became an outstanding numeric assessment", owner, err)
	}
}

func TestCoordinationReadDoesNotStartOrFreezeNativeAssessment(t *testing.T) {
	f := setup(t)
	if err := database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		_, e := f.s.CoverageInTransaction(
			t.Context(),
			tx,
			f.owner.ID,
			f.event,
			f.team,
			uuid.Nil,
			50,
		)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	run, err := dbgen.New(f.pool).
		RubricRun(t.Context(), dbgen.RubricRunParams{EventID: f.event, ID: f.checkpoint.Definition.RunID})
	if err != nil || run.StartedAt.Valid {
		t.Fatal("read-only coverage started/froze the native assessment", err)
	}
}
