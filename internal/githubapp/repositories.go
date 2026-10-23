package githubapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v89/github"
	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

var (
	ErrRepositoryAbsent   = errors.New("repository is absent or inaccessible")
	ErrRepositoryRejected = errors.New("repository request rejected")
)

// RepositoryRequest contains immutable, event-authorised resource names only.
// Credentials are decrypted inside the App owner and never returned to callers.
type RepositoryRequest struct {
	Configuration
	InstallationID int64
	Name           string
	Marker         string
}
type Repository struct {
	ID             int64
	OrganizationID int64
	Name           string
	Description    string
	Private        bool
	TemplateID     int64
	Archived       bool
}
type RepositoryProvider interface {
	LookupRepository(context.Context, Configuration, []byte, int64, string) (Repository, error)
	CreateRepository(
		context.Context,
		Configuration,
		[]byte,
		int64,
		string,
		string,
	) (Repository, error)
}

func (s Service) repositoryOperation(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	r RepositoryRequest,
	create bool,
) (Repository, error) {
	row, err := q.GitHubAppIntegration(ctx, eventID)
	if err != nil || config(row) != r.Configuration || row.InstallationID != r.InstallationID {
		return Repository{}, ErrIntegration
	}
	plain, err := s.Keys.Open(row.KeyID, row.PrivateKeyEncrypted, binding(row))
	if err != nil {
		return Repository{}, ErrIntegration
	}
	defer clear(plain)
	provider, ok := s.Provider.(RepositoryProvider)
	if !ok {
		return Repository{}, ErrIntegration
	}
	health := s.Provider.Inspect(ctx, r.Configuration, plain, r.InstallationID)
	if health.Code != "healthy" {
		return Repository{}, ErrIntegration
	}
	var repo Repository
	if create {
		repo, err = provider.CreateRepository(
			ctx,
			r.Configuration,
			plain,
			r.InstallationID,
			r.Name,
			r.Marker,
		)
	} else {
		repo, err = provider.LookupRepository(ctx, r.Configuration, plain, r.InstallationID, r.Name)
	}
	if err != nil {
		return Repository{}, fmt.Errorf("GitHub App repository operation: %w", err)
	}
	return repo, nil
}

func (s Service) LookupRepository(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	r RepositoryRequest,
) (Repository, error) {
	return s.repositoryOperation(ctx, q, eventID, r, false)
}

func (s Service) CreateRepository(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	r RepositoryRequest,
) (Repository, error) {
	return s.repositoryOperation(ctx, q, eventID, r, true)
}

func repositoryValue(r *github.Repository) Repository {
	return Repository{
		ID:             r.GetID(),
		OrganizationID: r.GetOwner().GetID(),
		Name:           r.GetName(),
		Description:    r.GetDescription(),
		Private:        r.GetPrivate(),
		TemplateID:     r.GetTemplateRepository().GetID(),
		Archived:       r.GetArchived() || r.GetDisabled(),
	}
}

func (p *RESTProvider) LookupRepository(
	ctx context.Context,
	c Configuration,
	key []byte,
	id int64,
	name string,
) (Repository, error) {
	if !namePattern.MatchString(name) {
		return Repository{}, ErrIntegration
	}
	client, err := p.scopedClient(c, key, id, []string{name}, "metadata")
	if err != nil {
		return Repository{}, err
	}
	repo, response, err := client.Repositories.Get(ctx, c.Organization, name)
	if err != nil {
		if response != nil && response.StatusCode == http.StatusNotFound {
			return Repository{}, ErrRepositoryAbsent
		}
		var tokenFailure *ghinstallation.HTTPError
		if errors.As(err, &tokenFailure) && tokenFailure.Response != nil &&
			tokenFailure.Response.StatusCode == 422 {
			return Repository{}, ErrRepositoryAbsent
		}
		// Repository-name scoped tokens reject names not selected in the installation.
		// This does not prove absence; callers must never retry an uncertain create.
		return Repository{}, ErrIntegration
	}
	return repositoryValue(repo), nil
}

func (p *RESTProvider) CreateRepository(
	ctx context.Context,
	c Configuration,
	key []byte,
	id int64,
	name, marker string,
) (Repository, error) {
	if !namePattern.MatchString(name) || !strings.HasPrefix(marker, "hackbox-provisioning:") {
		return Repository{}, ErrIntegration
	}
	client, err := p.scopedClient(c, key, id, []string{c.TemplateRepository}, "create")
	if err != nil {
		return Repository{}, err
	}
	repo, response, err := client.Repositories.CreateFromTemplate(
		ctx,
		c.Organization,
		c.TemplateRepository,
		&github.TemplateRepoRequest{
			Owner:              new(c.Organization),
			Name:               new(name),
			Description:        new(marker),
			Private:            new(true),
			IncludeAllBranches: new(false),
		},
	)
	if err != nil {
		if response != nil &&
			(response.StatusCode == 403 || response.StatusCode == 404 || response.StatusCode == 422) {
			return Repository{}, ErrRepositoryRejected
		}
		return Repository{}, ErrIntegration
	}
	return repositoryValue(repo), nil
}
