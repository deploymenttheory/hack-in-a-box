package githubapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/google/go-github/v89/github"
	"github.com/google/uuid"
)

// AccessRequest retains immutable repository identity. User handles are resolved
// from GitHub's numeric account ID immediately before a provider mutation.
type AccessRequest struct {
	RepositoryRequest
	RepositoryID int64
}
type Collaborator struct {
	ID         int64  `json:"github_id"`
	Login      string `json:"login"`
	Permission string `json:"permission"`
}
type Invitation struct {
	ID         int64     `json:"id"`
	AccountID  int64     `json:"github_id"`
	Permission string    `json:"permission"`
	CreatedAt  time.Time `json:"created_at"`
	Expired    bool      `json:"expired"`
}
type AccessSnapshot struct {
	Collaborators []Collaborator `json:"collaborators"`
	Invitations   []Invitation   `json:"invitations"`
}
type AccessChange struct {
	AccountID    int64
	Permission   string
	Remove       bool
	InvitationID int64
}
type AccessResult struct {
	Invitation *Invitation
	Accepted   bool
}

var ErrAccessRejected = errors.New("repository access is restricted by provider policy")

type CollaboratorProvider interface {
	InspectCollaborators(context.Context, Configuration, []byte, int64, AccessRequest) (AccessSnapshot, error)
	ChangeCollaborator(context.Context, Configuration, []byte, int64, AccessRequest, AccessChange) (AccessResult, error)
}

func (s Service) accessOwner(ctx context.Context, q *dbgen.Queries, eventID uuid.UUID, request AccessRequest, change *AccessChange) (AccessSnapshot, AccessResult, error) {
	row, err := q.GitHubAppIntegration(ctx, eventID)
	if err != nil || config(row) != request.Configuration || row.InstallationID != request.InstallationID || request.RepositoryID <= 0 {
		return AccessSnapshot{}, AccessResult{}, ErrIntegration
	}
	key, err := s.Keys.Open(row.KeyID, row.PrivateKeyEncrypted, binding(row))
	if err != nil {
		return AccessSnapshot{}, AccessResult{}, ErrIntegration
	}
	defer clear(key)
	provider, ok := s.Provider.(CollaboratorProvider)
	if !ok {
		return AccessSnapshot{}, AccessResult{}, ErrIntegration
	}
	if s.Provider.Inspect(ctx, request.Configuration, key, row.InstallationID).Code != "healthy" {
		return AccessSnapshot{}, AccessResult{}, ErrIntegration
	}
	if change != nil {
		result, err := provider.ChangeCollaborator(ctx, request.Configuration, key, row.InstallationID, request, *change)
		return AccessSnapshot{}, result, err
	}
	snapshot, err := provider.InspectCollaborators(ctx, request.Configuration, key, row.InstallationID, request)
	return snapshot, AccessResult{}, err
}
func (s Service) InspectCollaborators(ctx context.Context, q *dbgen.Queries, eventID uuid.UUID, request AccessRequest) (AccessSnapshot, error) {
	snapshot, _, err := s.accessOwner(ctx, q, eventID, request, nil)
	return snapshot, err
}
func (s Service) ChangeCollaborator(ctx context.Context, q *dbgen.Queries, eventID uuid.UUID, request AccessRequest, change AccessChange) (AccessResult, error) {
	_, result, err := s.accessOwner(ctx, q, eventID, request, &change)
	return result, err
}

func (p *RESTProvider) accessClient(ctx context.Context, c Configuration, key []byte, installation int64, request AccessRequest, write bool) (*github.Client, error) {
	operation := "access_read"
	if write {
		operation = "invite"
	}
	client, err := p.scopedClient(c, key, installation, []string{request.Name}, operation)
	if err != nil {
		return nil, err
	}
	repo, _, err := client.Repositories.Get(ctx, c.Organization, request.Name)
	if err != nil || repo.GetID() != request.RepositoryID || repo.GetOwner().GetID() != c.OrganizationID || !repo.GetPrivate() || repo.GetArchived() || repo.GetDisabled() || repo.GetName() != request.Name || repo.GetDescription() != request.Marker {
		return nil, ErrIntegration
	}
	return client, nil
}
func (p *RESTProvider) InspectCollaborators(ctx context.Context, c Configuration, key []byte, installation int64, request AccessRequest) (AccessSnapshot, error) {
	out := AccessSnapshot{Collaborators: []Collaborator{}, Invitations: []Invitation{}}
	client, err := p.accessClient(ctx, c, key, installation, request, false)
	if err != nil {
		return out, err
	}
	for page := 1; page <= 10; page++ {
		users, response, err := client.Repositories.ListCollaborators(ctx, c.Organization, request.Name, &github.ListCollaboratorsOptions{Affiliation: "direct", ListOptions: github.ListOptions{Page: page, PerPage: 100}})
		if err != nil {
			return out, accessFailure(response, err)
		}
		for _, user := range users {
			if user.GetID() <= 0 {
				return out, ErrIntegration
			}
			out.Collaborators = append(out.Collaborators, Collaborator{ID: user.GetID(), Login: user.GetLogin(), Permission: user.GetRoleName()})
		}
		if response.NextPage == 0 {
			break
		}
		if page == 10 {
			return out, ErrIntegration
		}
	}
	for page := 1; page <= 10; page++ {
		invites, response, err := client.Repositories.ListInvitations(ctx, c.Organization, request.Name, &github.ListOptions{Page: page, PerPage: 100})
		if err != nil {
			return out, accessFailure(response, err)
		}
		for _, invitation := range invites {
			if invitation.GetID() <= 0 || invitation.GetInvitee().GetID() <= 0 {
				return out, ErrIntegration
			}
			out.Invitations = append(out.Invitations, Invitation{ID: invitation.GetID(), AccountID: invitation.GetInvitee().GetID(), Permission: invitation.GetPermissions(), CreatedAt: invitation.GetCreatedAt().Time, Expired: invitation.GetExpired()})
		}
		if response.NextPage == 0 {
			break
		}
		if page == 10 {
			return out, ErrIntegration
		}
	}
	return out, nil
}
func accessFailure(response *github.Response, err error) error {
	if response != nil && (response.StatusCode == 403 || response.StatusCode == 422) {
		return ErrAccessRejected
	}
	return fmt.Errorf("GitHub access request unavailable: %w", ErrIntegration)
}
func (p *RESTProvider) ChangeCollaborator(ctx context.Context, c Configuration, key []byte, installation int64, request AccessRequest, change AccessChange) (AccessResult, error) {
	if change.AccountID <= 0 || (!change.Remove && change.Permission != "push" && change.Permission != "pull") {
		return AccessResult{}, ErrIntegration
	}
	client, err := p.accessClient(ctx, c, key, installation, request, true)
	if err != nil {
		return AccessResult{}, err
	}
	user, response, err := client.Users.GetByID(ctx, change.AccountID)
	if err != nil {
		return AccessResult{}, accessFailure(response, err)
	}
	if user.GetID() != change.AccountID || user.GetLogin() == "" {
		return AccessResult{}, ErrIntegration
	}
	if change.Remove {
		if change.InvitationID > 0 {
			response, err = client.Repositories.DeleteInvitation(ctx, c.Organization, request.Name, change.InvitationID)
		} else {
			response, err = client.Repositories.RemoveCollaborator(ctx, c.Organization, request.Name, user.GetLogin())
		}
		if err != nil {
			return AccessResult{}, accessFailure(response, err)
		}
		return AccessResult{}, nil
	}
	invitation, response, err := client.Repositories.AddCollaborator(ctx, c.Organization, request.Name, user.GetLogin(), &github.RepositoryAddCollaboratorOptions{Permission: change.Permission})
	if err != nil {
		return AccessResult{}, accessFailure(response, err)
	}
	if response.StatusCode == http.StatusNoContent {
		return AccessResult{Accepted: true}, nil
	}
	if response.StatusCode != http.StatusCreated || invitation.GetID() <= 0 || invitation.GetInvitee().GetID() != change.AccountID || invitation.GetRepo().GetID() != request.RepositoryID {
		return AccessResult{}, ErrIntegration
	}
	value := Invitation{ID: invitation.GetID(), AccountID: change.AccountID, Permission: invitation.GetPermissions(), CreatedAt: invitation.GetCreatedAt().Time}
	return AccessResult{Invitation: &value}, nil
}
