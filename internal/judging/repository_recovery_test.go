package judging

import (
	"errors"
	"strings"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestRepositoryRecoveryNeverMakesUnresolvedOrDisbandedBuildScoreable(t *testing.T) {
	f := setup(t, true)
	freeze := f.freeze()
	if _, err := f.reviewedCard(f.first.ID, 0, "save", nil, uuid.Nil, 1); !errors.Is(err, access.ErrConflict) {
		t.Fatal("unresolved repository became scoreable", err)
	}
	decision := f.decide(freeze, "reconcile", strings.Repeat("a", 40))
	id := decision.Decision.ID
	if _, err := f.reviewedCard(f.first.ID, 0, "submit", &id, uuid.Nil, 1); err != nil {
		t.Fatal(err)
	}
	var retained string
	if err := f.pool.QueryRow(t.Context(), "SELECT document::text FROM judge_cards WHERE event_id=$1 AND author_id=$2", f.event, f.first.ID).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	// Inject the native retired-team facts after legitimate reviewed scoring. The
	// actual consumers must revoke future access and preserve authored history.
	f.exec("UPDATE hack_teams SET state='disbanded' WHERE event_id=$1 AND id=$2", f.event, f.team)
	f.exec("UPDATE competition_entries SET active=false WHERE event_id=$1 AND team_id=$2", f.event, f.team)
	if _, err := f.s.Brief(t.Context(), f.first.ID, f.event, f.run, f.team); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("disbanded repository retained judge brief", err)
	}
	if _, err := f.reviewedCard(f.second.ID, 0, "save", &id, uuid.Nil, 1); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("disbanded repository accepted new score", err)
	}
	if err := database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		_, err := f.s.CompletionInTransaction(t.Context(), tx, f.owner.ID, f.event, f.run)
		return err
	}); !errors.Is(err, access.ErrConflict) {
		t.Fatal("disbanded-only event satisfied result completion", err)
	}
	var after string
	var cards int
	if err := f.pool.QueryRow(t.Context(), "SELECT document::text FROM judge_cards WHERE event_id=$1 AND author_id=$2", f.event, f.first.ID).Scan(&after); err != nil || after != retained {
		t.Fatal("retirement rewrote authored history", err)
	}
	if err := f.pool.QueryRow(t.Context(), "SELECT count(*) FROM judge_cards WHERE event_id=$1", f.event).Scan(&cards); err != nil || cards != 1 {
		t.Fatal("denied consumer retained new score", cards, err)
	}
}
