package githubapp

import (
	"context"
	"fmt"
	"net/http"
	"regexp"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// BranchObservation is a current ref observation, never a historical assertion.
// Only managed numeric repositories reach this seam; no arbitrary URL is fetched.
type (
	BranchObservation struct{ SHA, Status string }
	BranchProvider    interface {
		ObserveBranch(
			context.Context,
			Configuration,
			[]byte,
			int64,
			string,
			int64,
			string,
		) (BranchObservation, error)
	}
)

func (s Service) ObserveBranch(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	r RepositoryRequest,
	repositoryID int64,
	branch string,
) (BranchObservation, error) {
	row, err := q.GitHubAppIntegration(ctx, eventID)
	if err != nil || config(row) != r.Configuration || row.InstallationID != r.InstallationID {
		return BranchObservation{}, ErrIntegration
	}
	plain, err := s.Keys.Open(row.KeyID, row.PrivateKeyEncrypted, binding(row))
	if err != nil {
		return BranchObservation{}, ErrIntegration
	}
	defer clear(plain)
	p, ok := s.Provider.(BranchProvider)
	if !ok {
		return BranchObservation{}, ErrIntegration
	}
	if s.Provider.Inspect(ctx, r.Configuration, plain, r.InstallationID).Code != "healthy" {
		return BranchObservation{}, ErrIntegration
	}
	observation, err := p.ObserveBranch(
		ctx,
		r.Configuration,
		plain,
		r.InstallationID,
		r.Name,
		repositoryID,
		branch,
	)
	if err != nil {
		return BranchObservation{}, fmt.Errorf("observe managed branch: %w", err)
	}
	return observation, nil
}

var branchSHA = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func (p *RESTProvider) ObserveBranch(
	ctx context.Context,
	c Configuration,
	key []byte,
	installationID int64,
	name string,
	repositoryID int64,
	branch string,
) (BranchObservation, error) {
	if !namePattern.MatchString(name) || repositoryID <= 0 || branch == "" {
		return BranchObservation{}, ErrIntegration
	}
	client, err := p.scopedClient(c, key, installationID, []string{name}, "read")
	if err != nil {
		return BranchObservation{}, err
	}
	repo, response, err := client.Repositories.Get(ctx, c.Organization, name)
	if err != nil {
		if response != nil && response.StatusCode == http.StatusNotFound {
			return BranchObservation{Status: "repository_unavailable"}, nil
		}
		return BranchObservation{}, ErrIntegration
	}
	if repo.GetID() != repositoryID || repo.GetOwner().GetID() != c.OrganizationID ||
		!repo.GetPrivate() ||
		repo.GetArchived() ||
		repo.GetDisabled() {
		return BranchObservation{}, ErrRepositoryRejected
	}
	ref, response, err := client.Git.GetRef(ctx, c.Organization, name, "heads/"+branch)
	if err != nil {
		if response != nil {
			switch response.StatusCode {
			case http.StatusNotFound:
				return BranchObservation{Status: "branch_missing"}, nil
			case http.StatusConflict:
				return BranchObservation{Status: "empty_repository"}, nil
			}
		}
		return BranchObservation{}, ErrIntegration
	}
	if ref.GetRef() != "refs/heads/"+branch || ref.GetObject().GetType() != "commit" ||
		!branchSHA.MatchString(ref.GetObject().GetSHA()) {
		return BranchObservation{}, fmt.Errorf("invalid branch observation: %w", ErrIntegration)
	}
	return BranchObservation{SHA: ref.GetObject().GetSHA(), Status: "current_ref"}, nil
}
