package repositoryaccess

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
)

func candidate(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, teamID, person uuid.UUID,
	role, permission string,
) (Member, *Flag, bool, error) {
	authority, err := access.TeamPermissions(ctx, q, person, eventID, teamID)
	if errors.Is(err, access.ErrForbidden) {
		return Member{}, nil, false, nil
	}
	if err != nil {
		return Member{}, nil, false, fmt.Errorf("read current repository member authority: %w", err)
	}
	allowed := authority.Active &&
		(role == "competitor" && authority.Member || role == "mentor" && (authority.Mentor || authority.Reviewer))
	if !allowed {
		return Member{}, nil, false, nil
	}
	revision, err := q.GitHubLinkRevision(ctx, person)
	if err != nil {
		return Member{}, nil, false, fmt.Errorf(
			"read current verified repository identity revision: %w",
			err,
		)
	}
	member := Member{PersonID: person, Role: role, Permission: permission, LinkRevision: revision}
	account, err := q.GitHubAccount(ctx, person)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return member, &Flag{Code: "verified_account_required", PersonID: &person}, true, nil
	case err != nil:
		return member, nil, false, fmt.Errorf("read verified repository account: %w", err)
	default:
		member.GitHubID = account.GithubID
		return member, nil, true, nil
	}
}

func invalidCoverage(gaps []mentors.Gap, teamID, personID uuid.UUID) bool {
	return slices.ContainsFunc(gaps, func(g mentors.Gap) bool {
		return g.Hard &&
			(g.TeamID != nil && *g.TeamID == teamID || g.PersonID != nil && *g.PersonID == personID)
	})
}

// desired composes the owning published roster, current claim/resource authority
// and verified numeric identity in the caller's existing event transaction.
func desired(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	e dbgen.Event,
	repository dbgen.TeamRepository,
	actor uuid.UUID,
	at time.Time,
) ([]Member, []Flag, error) {
	out, flags := []Member{}, []Flag{}
	team, err := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: e.ID, ID: repository.TeamID})
	if err != nil {
		return out, flags, fmt.Errorf("read access team: %w", err)
	}
	if team.State != "published" || e.Stage == "cancelled" || e.Stage == "archived" {
		return out, flags, nil
	}
	add := func(person uuid.UUID, role, permission string, coverage *mentors.Assignment) error {
		if slices.ContainsFunc(out, func(m Member) bool { return m.PersonID == person }) {
			return nil
		}
		member, flag, allowed, readErr := candidate(ctx, q, e.ID, team.ID, person, role, permission)
		if readErr != nil {
			return readErr
		}
		if !allowed {
			return nil
		}
		if coverage != nil {
			start, end := coverage.Window.Start, coverage.Window.End
			member.CoverageStart = &start
			member.CoverageEnd = &end
		}
		out = append(out, member)
		if flag != nil {
			flags = append(flags, *flag)
		}
		return nil
	}
	members, err := q.HackTeamMembers(ctx, e.ID)
	if err != nil {
		return out, flags, fmt.Errorf("read published access membership: %w", err)
	}
	for _, member := range members {
		if member.TeamID == team.ID && member.Snapshot == "published" {
			if err = add(member.PersonID, "competitor", "push", nil); err != nil {
				return out, flags, err
			}
		}
	}
	roster, err := (mentors.Service{}).ReadInTransaction(ctx, tx, actor, e.ID)
	if err != nil {
		return out, flags, fmt.Errorf("read owning published mentor coverage: %w", err)
	}
	for _, assignment := range roster.Published {
		if assignment.TeamID != team.ID || at.Before(assignment.Window.Start) ||
			!at.Before(assignment.Window.End) {
			continue
		}
		if invalidCoverage(roster.PublishedGaps, team.ID, assignment.PersonID) {
			person := assignment.PersonID
			flags = append(flags, Flag{Code: "mentor_coverage_requires_review", PersonID: &person})
			continue
		}
		if err = add(assignment.PersonID, "mentor", "pull", &assignment); err != nil {
			return out, flags, err
		}
	}
	slices.SortFunc(
		out,
		func(a, b Member) int { return strings.Compare(a.PersonID.String(), b.PersonID.String()) },
	)
	return out, flags, nil
}

func grantsOpen(e dbgen.Event, at time.Time) bool {
	return e.WritesAllowed && e.Stage != "cancelled" && e.Stage != "archived" &&
		e.TeamFrozenAt.Valid &&
		e.PublishedAt.Valid &&
		!at.Before(e.PublishedAt.Time)
}

func request(r dbgen.TeamRepository, a dbgen.GithubAppIntegration) githubapp.AccessRequest {
	return githubapp.AccessRequest{
		RepositoryRequest: githubapp.RepositoryRequest{
			Configuration: githubapp.Configuration{
				AppID:              r.AppID,
				AppSlug:            a.AppSlug,
				OrganizationID:     r.OrganizationID,
				Organization:       r.Organization,
				TemplateRepository: r.TemplateRepository,
			},
			InstallationID: r.InstallationID,
			Name:           r.Name,
			Marker:         "hackbox-provisioning:" + r.Provenance.String(),
		},
		RepositoryID: r.GithubRepositoryID.Int64,
	}
}

type evidence struct {
	collaborators map[int64]githubapp.Collaborator
	invitations   map[int64]githubapp.Invitation
	owned         map[int64]Grant
	grants        []Grant
}

func currentEvidence(grants []Grant, source githubapp.AccessSnapshot) evidence {
	out := evidence{
		collaborators: map[int64]githubapp.Collaborator{},
		invitations:   map[int64]githubapp.Invitation{},
		owned:         map[int64]Grant{},
		grants:        grants,
	}
	for _, user := range source.Collaborators {
		out.collaborators[user.ID] = user
	}
	for _, invitation := range source.Invitations {
		out.invitations[invitation.AccountID] = invitation
	}
	for _, grant := range grants {
		if !grant.Managed {
			continue
		}
		active := grant.State == "pending" || grant.State == "accepted" ||
			grant.State == "ambiguous"
		invitation, present := out.invitations[grant.Member.GitHubID]
		knownExpired := grant.State == "expired" && present && grant.InvitationID != nil &&
			*grant.InvitationID == invitation.ID
		if active || knownExpired {
			out.owned[grant.Member.GitHubID] = grant
		}
	}
	return out
}

func (e evidence) matches(grant Grant) bool {
	user, collaborator := e.collaborators[grant.Member.GitHubID]
	invitation, invited := e.invitations[grant.Member.GitHubID]
	return collaborator && user.Permission == permissionName(grant.Member.Permission) ||
		invited && grant.InvitationID != nil && invitation.ID == *grant.InvitationID &&
			invitation.Permission == permissionName(grant.Member.Permission)
}

func removal(grant Grant) Action {
	id := grant.ID
	return Action{
		ID:            uuid.New(),
		Kind:          "remove",
		Member:        grant.Member,
		GrantID:       &id,
		PreviousState: grant.State,
	}
}
func accessFlag(code string, id int64) Flag { return Flag{Code: code, GitHubID: &id} }
func (e evidence) addition(member Member) ([]Action, []Flag) {
	id := member.GitHubID
	uncertain := slices.ContainsFunc(
		e.grants,
		func(g Grant) bool { return g.Member.GitHubID == id && (g.Uncertain || g.State == "ambiguous") },
	)
	if uncertain {
		return nil, []Flag{accessFlag("external_effect_requires_reconciliation", id)}
	}
	grant, managed := e.owned[id]
	if managed && grant.Member.Permission != member.Permission && e.matches(grant) {
		return []Action{removal(grant), {ID: uuid.New(), Kind: "invite", Member: member}}, nil
	}
	if user, present := e.collaborators[id]; present {
		if !managed || permissionName(grant.Member.Permission) != user.Permission {
			return nil, []Flag{accessFlag("external_access_not_app_managed", id)}
		}
		return nil, nil
	}
	if invitation, present := e.invitations[id]; present {
		if !managed || grant.InvitationID == nil || *grant.InvitationID != invitation.ID ||
			permissionName(grant.Member.Permission) != invitation.Permission {
			return nil, []Flag{accessFlag("external_invitation_not_app_managed", id)}
		}
		if invitation.Expired {
			return []Action{removal(grant), {ID: uuid.New(), Kind: "invite", Member: member}}, nil
		}
		return nil, nil
	}
	return []Action{{ID: uuid.New(), Kind: "invite", Member: member}}, nil
}

func (e evidence) removal(grant Grant) ([]Action, []Flag) {
	id := grant.Member.GitHubID
	if grant.Uncertain || grant.State == "ambiguous" {
		return nil, []Flag{accessFlag("external_effect_requires_reconciliation", id)}
	}
	if e.matches(grant) {
		return []Action{removal(grant)}, nil
	}
	_, collaborator := e.collaborators[id]
	_, invited := e.invitations[id]
	if collaborator || invited {
		return nil, []Flag{accessFlag("managed_permission_changed", id)}
	}
	return nil, nil
}

func (e evidence) unknown(desired map[int64]Member) []Flag {
	flags := []Flag{}
	for id := range e.collaborators {
		_, owned := e.owned[id]
		_, wanted := desired[id]
		if !owned && !wanted {
			flags = append(flags, accessFlag("unknown_external_collaborator", id))
		}
	}
	for id := range e.invitations {
		_, owned := e.owned[id]
		_, wanted := desired[id]
		if !owned && !wanted {
			flags = append(flags, accessFlag("unknown_external_invitation", id))
		}
	}
	return flags
}

// propose never adopts outside access. Only clearly proven app-managed grants
// may yield removals; uncertain effects stay read-only until reconciled.
func propose(members []Member, grants []Grant, source githubapp.AccessSnapshot) ([]Action, []Flag) {
	actions, flags := []Action{}, []Flag{}
	desired := map[int64]Member{}
	current := currentEvidence(grants, source)
	for _, member := range members {
		if member.GitHubID > 0 {
			desired[member.GitHubID] = member
		}
	}
	for _, member := range desired {
		next, warnings := current.addition(member)
		actions = append(actions, next...)
		flags = append(flags, warnings...)
	}
	for id, grant := range current.owned {
		if _, wanted := desired[id]; !wanted {
			next, warnings := current.removal(grant)
			actions = append(actions, next...)
			flags = append(flags, warnings...)
		}
	}
	flags = append(flags, current.unknown(desired)...)
	slices.SortFunc(actions, func(a, b Action) int {
		if a.Member.GitHubID != b.Member.GitHubID {
			return cmp.Compare(a.Member.GitHubID, b.Member.GitHubID)
		}
		return strings.Compare(b.Kind, a.Kind)
	})
	return actions, flags
}
