package repositoryaccess

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestProposalNeverRemovesUnknownAccessAndDeduplicatesPending(t *testing.T) {
	person, grantID := uuid.New(), uuid.New()
	invitationID := int64(500)
	member := Member{
		PersonID:     person,
		GitHubID:     101,
		LinkRevision: 2,
		Permission:   "push",
		Role:         "competitor",
	}
	grant := Grant{
		ID:           grantID,
		Member:       member,
		Managed:      true,
		State:        "pending",
		InvitationID: &invitationID,
	}
	source := githubapp.AccessSnapshot{
		Collaborators: []githubapp.Collaborator{
			{ID: 999, Login: "unknown-external", Permission: "admin"},
		},
		Invitations: []githubapp.Invitation{{ID: 500, AccountID: 101, Permission: "write"}},
	}
	actions, flags := propose([]Member{member}, []Grant{grant}, source)
	if len(actions) != 0 || len(flags) != 1 || flags[0].Code != "unknown_external_collaborator" {
		t.Fatal("pending invitation duplicated or unknown modified", actions, flags)
	}
	actions, _ = propose(nil, []Grant{grant}, source)
	if len(actions) != 1 || actions[0].Kind != "remove" || actions[0].GrantID == nil ||
		*actions[0].GrantID != grantID ||
		actions[0].Member.GitHubID != 101 {
		t.Fatal("removal lacked managed provenance", actions)
	}
	grant.Managed = false
	actions, _ = propose(nil, []Grant{grant}, source)
	if len(actions) != 0 {
		t.Fatal("unmanaged removal was proposed", actions)
	}
	grant.Managed = true
	grant.State = "ambiguous"
	actions, flags = propose([]Member{member}, []Grant{grant}, githubapp.AccessSnapshot{})
	if len(actions) != 0 || len(flags) != 1 ||
		flags[0].Code != "external_effect_requires_reconciliation" {
		t.Fatal("uncertain effect was retried", actions, flags)
	}
	grant.State = "revoked"
	actions, flags = propose(
		nil,
		[]Grant{grant},
		githubapp.AccessSnapshot{
			Collaborators: []githubapp.Collaborator{{ID: 101, Permission: "write"}},
		},
	)
	if len(actions) != 0 || len(flags) != 1 || flags[0].Code != "unknown_external_collaborator" {
		t.Fatal("later external regrant was adopted", actions, flags)
	}
}

func TestNativeDesiredAccessUsesPublishedConfirmedRosterAndVerifiedRevision(t *testing.T) {
	pool := testdb.New(t, true)
	owner, competitor, draft := testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	)
	eventID, teamID := testdb.Event(t, pool, owner.ID), uuid.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(t.Context(), sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(
		"INSERT INTO hack_teams(id,event_id,name,state,frozen)VALUES($1,$2,'Actual published team','published',true)",
		teamID,
		eventID,
	)
	exec("INSERT INTO competition_entries(event_id,team_id)VALUES($1,$2)", eventID, teamID)
	for _, p := range []uuid.UUID{competitor.ID, draft.ID} {
		exec(
			"INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')",
			eventID,
			p,
		)
	}
	exec(
		"INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot)VALUES($1,$2,$3,'published'),($1,$2,$4,'draft')",
		eventID,
		teamID,
		competitor.ID,
		draft.ID,
	)
	exec(
		"INSERT INTO github_accounts(person_id,github_id,login,token_encrypted)VALUES($1,101,'renamed-account',decode('00','hex'))",
		competitor.ID,
	)
	exec("INSERT INTO github_link_states(person_id,revision)VALUES($1,7)", competitor.ID)
	var current []Member
	load := func() {
		t.Helper()
		err := database.Within(t.Context(), pool, func(tx pgx.Tx, q *dbgen.Queries) error {
			event, err := q.LockEvent(t.Context(), eventID)
			if err != nil {
				return err
			}
			current, _, err = desired(
				context.Background(),
				tx,
				q,
				event,
				dbgen.TeamRepository{TeamID: teamID},
				owner.ID,
				time.Now(),
			)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	load()
	if len(current) != 1 || current[0].PersonID != competitor.ID || current[0].GitHubID != 101 ||
		current[0].LinkRevision != 7 ||
		current[0].Permission != "push" {
		t.Fatal("draft or unverified identity gained access", current)
	}
	exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		eventID,
		competitor.ID,
	)
	load()
	if len(current) != 0 {
		t.Fatal("frozen stale assignment retained current access", current)
	}
	var retained int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM hack_team_members WHERE event_id=$1 AND snapshot='published'", eventID).
		Scan(&retained); err != nil ||
		retained != 1 {
		t.Fatal("reconciliation changed frozen assignments", retained, err)
	}
}

func TestExpiredInvitationRequiresOwnedCancellationAndUnknownExpiryIsFlagged(t *testing.T) {
	member := Member{PersonID: uuid.New(), GitHubID: 101, Permission: "push", Role: "competitor"}
	invitationID := int64(500)
	source := githubapp.AccessSnapshot{
		Invitations: []githubapp.Invitation{
			{ID: invitationID, AccountID: 101, Permission: "write", Expired: true},
		},
	}
	actions, flags := propose([]Member{member}, nil, source)
	if len(actions) != 0 || len(flags) != 1 ||
		flags[0].Code != "external_invitation_not_app_managed" {
		t.Fatal("unknown expired invitation modified", actions, flags)
	}
	grant := Grant{
		ID:           uuid.New(),
		Member:       member,
		Managed:      true,
		State:        "expired",
		InvitationID: &invitationID,
	}
	actions, flags = propose([]Member{member}, []Grant{grant}, source)
	if len(actions) != 2 || actions[0].Kind != "remove" || actions[1].Kind != "invite" ||
		len(flags) != 0 {
		t.Fatal("owned expired invitation was not sequenced", actions, flags)
	}
	grant.Uncertain = true
	actions, _ = propose([]Member{member}, []Grant{grant}, source)
	if len(actions) != 0 {
		t.Fatal("observed expiry erased uncertain effect barrier")
	}
}

func TestMentorWindowChangesNativeAccessFingerprint(t *testing.T) {
	start, end := time.Now().UTC(), time.Now().UTC().Add(time.Hour)
	member := Member{
		PersonID:      uuid.New(),
		GitHubID:      101,
		Permission:    "pull",
		Role:          "mentor",
		CoverageStart: &start,
		CoverageEnd:   &end,
	}
	other := member
	later := end.Add(time.Minute)
	other.CoverageEnd = &later
	provenance := uuid.New()
	if sameMember(member, other) ||
		fingerprint(
			800,
			provenance,
			[]Member{member},
		) == fingerprint(
			800,
			provenance,
			[]Member{other},
		) {
		t.Fatal("changed published mentor window retained old approval")
	}
}
