package mentors

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func TestPanelAuthorityPreservesCurrentRoleTeamAssignmentAndConflictRules(t *testing.T) {
	f := coverageDatabase(t)
	actor, team := f.mentors[0].ID, f.teams[0]
	check := func(person, target uuid.UUID, qualification, allowed bool) {
		t.Helper()
		err := database.Within(t.Context(), f.pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
			if qualification {
				return AuthorizeQualificationPanelInTransaction(
					t.Context(),
					tx,
					person,
					f.id,
					target,
				)
			}
			return AuthorizeModerationPanelInTransaction(t.Context(), tx, person, f.id, target)
		})
		if allowed && err != nil {
			t.Fatal("current permitted panel actor denied", err)
		}
		if !allowed && !errors.Is(err, access.ErrForbidden) {
			t.Fatal("current panel boundary did not deny access", err)
		}
	}
	check(f.owner.ID, team, false, true)
	check(actor, team, true, true) // Qualification owner separately enforces its recorded panel.
	check(actor, team, false, false)
	f.exec(
		"INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at)VALUES($1,$2,$3,'primary','published',$4,$5)",
		f.id,
		team,
		actor,
		f.live.Start,
		f.live.End,
	)
	check(actor, team, false, true)
	f.exec(
		"INSERT INTO mentor_conflicts(event_id,mentor_id,competitor_id,kind)VALUES($1,$2,$3,'reporting')",
		f.id,
		actor,
		f.members[0].ID,
	)
	check(actor, team, true, false)
	check(actor, team, false, false)
	f.exec("DELETE FROM mentor_conflicts WHERE event_id=$1 AND mentor_id=$2", f.id, actor)
	f.exec(
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot)VALUES($1,$2,$3,'draft')",
		f.id,
		team,
		actor,
	)
	check(actor, team, true, false)
	check(actor, team, false, false)
	f.exec("DELETE FROM hack_team_members WHERE event_id=$1 AND person_id=$2", f.id, actor)
	f.exec(
		"UPDATE event_memberships SET active=false WHERE event_id=$1 AND person_id=$2 AND role='mentor'",
		f.id,
		actor,
	)
	check(actor, team, true, false)
	check(actor, team, false, false)
	f.exec(
		"UPDATE event_memberships SET active=true WHERE event_id=$1 AND person_id=$2 AND role='mentor'",
		f.id,
		actor,
	)
	f.exec("UPDATE people SET active=false WHERE id=$1", actor)
	check(actor, team, true, false)
	check(actor, team, false, false)
	f.exec("UPDATE people SET active=true WHERE id=$1", actor)
	f.exec("UPDATE hack_teams SET state='draft' WHERE event_id=$1 AND id=$2", f.id, team)
	check(actor, team, true, false)
	check(f.owner.ID, team, false, false)
	check(actor, uuid.New(), true, false)
}
