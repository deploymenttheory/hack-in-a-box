package repositoryaccess

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// desired composes the real published roster, current claim/resource authority
// and verified GitHub numeric identity inside the caller's event transaction.
func desired(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, e dbgen.Event, repository dbgen.TeamRepository, actor uuid.UUID, at time.Time) ([]Member, []Flag, error) {
	out, flags := []Member{}, []Flag{}
	team, err := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: e.ID, ID: repository.TeamID})
	if err != nil {
		return out, flags, fmt.Errorf("read access team: %w", err)
	}
	if team.State != "published" {
		return out, flags, nil
	}
	add := func(person uuid.UUID, role, permission string) error {
		authority, readErr := access.TeamPermissions(ctx, q, person, e.ID, team.ID)
		if errors.Is(readErr, access.ErrForbidden) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
		if !authority.Active || (role == "competitor" && !authority.Member) || (role == "mentor" && !authority.Mentor && !authority.Reviewer) {
			return nil
		}
		if slices.ContainsFunc(out, func(m Member) bool { return m.PersonID == person }) {
			return nil
		}
		revision, readErr := q.GitHubLinkRevision(ctx, person)
		if readErr != nil {
			return readErr
		}
		member := Member{PersonID: person, Role: role, Permission: permission, LinkRevision: revision}
		account, readErr := q.GitHubAccount(ctx, person)
		if errors.Is(readErr, pgx.ErrNoRows) {
			flags = append(flags, Flag{Code: "verified_account_required", PersonID: &person})
		} else if readErr != nil {
			return readErr
		} else {
			member.GitHubID = account.GithubID
		}
		out = append(out, member)
		return nil
	}
	members, err := q.HackTeamMembers(ctx, e.ID)
	if err != nil {
		return out, flags, fmt.Errorf("read published access membership: %w", err)
	}
	for _, member := range members {
		if member.TeamID == team.ID && member.Snapshot == "published" {
			if err = add(member.PersonID, "competitor", "push"); err != nil {
				return out, flags, err
			}
		}
	}
	roster, err := (mentors.Service{}).ReadInTransaction(ctx, tx, actor, e.ID)
	if err != nil {
		return out, flags, err
	}
	for _, assignment := range roster.Published {
		if assignment.TeamID != team.ID || at.Before(assignment.Window.Start) || !at.Before(assignment.Window.End) {
			continue
		}
		invalid := slices.ContainsFunc(roster.PublishedGaps, func(g mentors.Gap) bool {
			return g.Hard && (g.TeamID != nil && *g.TeamID == team.ID || g.PersonID != nil && *g.PersonID == assignment.PersonID)
		})
		if invalid {
			person := assignment.PersonID
			flags = append(flags, Flag{Code: "mentor_coverage_requires_review", PersonID: &person})
			continue
		}
		if err = add(assignment.PersonID, "mentor", "pull"); err != nil {
			return out, flags, err
		}
	}
	slices.SortFunc(out, func(a, b Member) int { return strings.Compare(a.PersonID.String(), b.PersonID.String()) })
	return out, flags, nil
}
func grantsOpen(e dbgen.Event, at time.Time) bool {
	return e.WritesAllowed && e.Stage != "cancelled" && e.Stage != "archived" && e.TeamFrozenAt.Valid && e.PublishedAt.Valid && !at.Before(e.PublishedAt.Time)
}
func request(repository dbgen.TeamRepository, integration dbgen.GithubAppIntegration) githubapp.AccessRequest {
	return githubapp.AccessRequest{RepositoryRequest: githubapp.RepositoryRequest{Configuration: githubapp.Configuration{AppID: repository.AppID, AppSlug: integration.AppSlug, OrganizationID: repository.OrganizationID, Organization: repository.Organization, TemplateRepository: repository.TemplateRepository}, InstallationID: repository.InstallationID, Name: repository.Name, Marker: "hackbox-provisioning:" + repository.Provenance.String()}, RepositoryID: repository.GithubRepositoryID.Int64}
}

// propose never adopts outside access. Only a clearly proven application-owned
// invitation/grant can yield a removal; uncertain effects remain read-only.
func propose(members []Member, grants []Grant, source githubapp.AccessSnapshot) ([]Action, []Flag) {
	actions, flags := []Action{}, []Flag{}
	desiredIDs := map[int64]Member{}
	for _, member := range members {
		if member.GitHubID > 0 {
			desiredIDs[member.GitHubID] = member
		}
	}
	existing := map[int64]githubapp.Collaborator{}
	pending := map[int64]githubapp.Invitation{}
	for _, user := range source.Collaborators {
		existing[user.ID] = user
	}
	for _, invitation := range source.Invitations {
		pending[invitation.AccountID] = invitation
	}
	owned := map[int64]Grant{}
	for _, grant := range grants {
		if grant.Managed && (grant.State == "pending" || grant.State == "accepted" || grant.State == "ambiguous" || grant.State == "expired") {
			owned[grant.Member.GitHubID] = grant
		}
	}
	for id, member := range desiredIDs {
		grant, managed := owned[id]
		if user, ok := existing[id]; ok {
			if !managed || permissionName(grant.Member.Permission) != user.Permission {
				value := id
				flags = append(flags, Flag{Code: "external_access_not_app_managed", GitHubID: &value})
			}
			continue
		}
		if invitation, ok := pending[id]; ok && !invitation.Expired {
			if !managed || grant.InvitationID == nil || *grant.InvitationID != invitation.ID || permissionName(grant.Member.Permission) != invitation.Permission {
				value := id
				flags = append(flags, Flag{Code: "external_invitation_not_app_managed", GitHubID: &value})
			}
			continue
		}
		uncertain := slices.ContainsFunc(grants, func(g Grant) bool { return g.Member.GitHubID == id && g.State == "ambiguous" })
		if uncertain {
			value := id
			flags = append(flags, Flag{Code: "external_effect_requires_reconciliation", GitHubID: &value})
			continue
		}
		actions = append(actions, Action{ID: uuid.New(), Kind: "invite", Member: member})
	}
	for id, grant := range owned {
		if _, wanted := desiredIDs[id]; wanted {
			continue
		}
		if grant.State == "ambiguous" {
			value := id
			flags = append(flags, Flag{Code: "external_effect_requires_reconciliation", GitHubID: &value})
			continue
		}
		user, collaborator := existing[id]
		invitation, invited := pending[id]
		proven := collaborator && user.Permission == permissionName(grant.Member.Permission) || invited && grant.InvitationID != nil && invitation.ID == *grant.InvitationID && invitation.Permission == permissionName(grant.Member.Permission)
		if proven {
			grantID := grant.ID
			actions = append(actions, Action{ID: uuid.New(), Kind: "remove", Member: grant.Member, GrantID: &grantID})
		} else if collaborator || invited {
			value := id
			flags = append(flags, Flag{Code: "managed_permission_changed", GitHubID: &value})
		}
	}
	for id := range existing {
		if _, managed := owned[id]; !managed {
			if _, wanted := desiredIDs[id]; !wanted {
				value := id
				flags = append(flags, Flag{Code: "unknown_external_collaborator", GitHubID: &value})
			}
		}
	}
	for id := range pending {
		if _, managed := owned[id]; !managed {
			if _, wanted := desiredIDs[id]; !wanted {
				value := id
				flags = append(flags, Flag{Code: "unknown_external_invitation", GitHubID: &value})
			}
		}
	}
	slices.SortFunc(actions, func(a, b Action) int {
		if a.Member.GitHubID < b.Member.GitHubID {
			return -1
		}
		if a.Member.GitHubID > b.Member.GitHubID {
			return 1
		}
		return 0
	})
	return actions, flags
}
