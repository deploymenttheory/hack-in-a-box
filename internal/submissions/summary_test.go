package submissions_test

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
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestFactualOutputMetadataUsesFrozenSourceWithoutAssessmentOrDraftText(t *testing.T) {
	f := newFixture(t, testdb.New(t, true))
	current := f.read(f.member.ID, f.team)
	f.submit(
		f.member.ID,
		current.Revision,
		f.evidence("PRIVATE-OUTPUT-LABEL"),
		"PRIVATE-REVIEW-REASON",
	)
	read := func(actor uuid.UUID, teams []uuid.UUID) ([]submissions.OutputSummary, error) {
		var out []submissions.OutputSummary
		err := database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
			var e error
			out, e = f.s.SummariesInTransaction(t.Context(), tx, actor, f.id, teams)
			return e
		})
		return out, err
	}
	live, err := read(f.owner.ID, []uuid.UUID{f.team})
	if err != nil || len(live) != 1 || live[0].FrozenRevision != nil ||
		len(live[0].Artifacts) != 0 {
		t.Fatal("live formal output pretended to be frozen judge facts", live, err)
	}
	f.now = f.c.SubmissionCutoff
	if _, err = read(f.other.ID, []uuid.UUID{f.team}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("unrelated person gained formal output metadata", err)
	}
	var settled bool
	if err = f.pool.QueryRow(t.Context(), `SELECT frozen_at IS NOT NULL FROM submission_states WHERE event_id=$1 AND team_id=$2`, f.id, f.team).
		Scan(&settled); err != nil ||
		settled {
		t.Fatal("unauthorised metadata read settled another team's state", settled, err)
	}
	frozen, err := read(f.owner.ID, []uuid.UUID{f.team, f.otherTeam})
	if err != nil || len(frozen) != 2 || frozen[0].Outcome != "frozen" ||
		frozen[0].FrozenRevision == nil ||
		frozen[0].SubmittedAt == nil ||
		len(frozen[0].Artifacts) != 1 ||
		frozen[0].Artifacts[0].Kind != "repository" ||
		frozen[0].Artifacts[0].Count != 1 ||
		frozen[1].Outcome != "not_submitted" ||
		len(frozen[1].Artifacts) != 0 {
		t.Fatal("frozen/not-submitted facts diverged from native cutoff outcome", frozen, err)
	}
	encoded, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"PRIVATE-OUTPUT-LABEL", "PRIVATE-REVIEW-REASON", "https://", `"scores":`, `"reason":`, `"evidence":`, `"definitions":`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("factual projection disclosed private/assessment payload", forbidden)
		}
	}
	f.now = f.now.Add(time.Minute)
	f.exec(
		`UPDATE competition_entries SET active=false WHERE event_id=$1 AND team_id=$2`,
		f.id,
		f.team,
	)
	withdrawn, err := read(f.owner.ID, []uuid.UUID{f.team})
	if err != nil || withdrawn[0].Outcome != "withdrawn" || withdrawn[0].FrozenRevision != nil ||
		len(withdrawn[0].Artifacts) != 0 {
		t.Fatal("withdrawn unused output still presented as assessable", withdrawn, err)
	}
}
