package app

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/githubapp"
)

var lifecycleRepositoryName = regexp.MustCompile(`^hackbox-[a-f0-9]{32}-[a-f0-9]{32}$`)

// The external installation selects only its template and resources actually
// created from that template. No native team or repository mapping is seeded.
func (g *lifecycleGitHub) allowedToken(names []string, permissions map[string]string) bool {
	if len(names) != 1 || permissions["metadata"] != "read" {
		return false
	}
	name := names[0]
	if name == lifecycleGitHubTemplate {
		return len(permissions) == 1 || (len(permissions) == 3 && permissions["contents"] == "read" && permissions["administration"] == "write")
	}
	g.mu.Lock()
	_, exists := g.repositories[name]
	g.mu.Unlock()
	return exists && (len(permissions) == 1 || len(permissions) == 2 && (permissions["administration"] == "read" || permissions["administration"] == "write"))
}

func lifecycleRepositoryJSON(repository githubapp.Repository) map[string]any {
	return map[string]any{
		"id": repository.ID, "name": repository.Name,
		"full_name":   lifecycleGitHubOrganization + "/" + repository.Name,
		"owner":       map[string]any{"id": repository.OrganizationID, "login": lifecycleGitHubOrganization, "type": "Organization"},
		"description": repository.Description, "private": repository.Private,
		"template_repository": map[string]any{"id": repository.TemplateID},
		"archived":            repository.Archived, "disabled": false, "default_branch": "main",
	}
}

func (g *lifecycleGitHub) repositoryHTTP(w http.ResponseWriter, r *http.Request) bool {
	prefix := "/github-api/repos/" + lifecycleGitHubOrganization + "/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		return false
	}
	name := strings.TrimPrefix(r.URL.Path, prefix)
	if name == lifecycleGitHubTemplate+"/generate" {
		if r.Method != http.MethodPost || !g.repositoryToken(r, lifecycleGitHubTemplate, "metadata", "read") || !g.repositoryToken(r, lifecycleGitHubTemplate, "contents", "read") || !g.repositoryToken(r, lifecycleGitHubTemplate, "administration", "write") {
			w.WriteHeader(http.StatusForbidden)
			return true
		}
		var body struct {
			Owner              string `json:"owner"`
			Name               string `json:"name"`
			Description        string `json:"description"`
			Private            bool   `json:"private"`
			IncludeAllBranches bool   `json:"include_all_branches"`
		}
		decoder := json.NewDecoder(io.LimitReader(r.Body, 4096))
		decoder.DisallowUnknownFields()
		const marker = "hackbox-provisioning:"
		if decoder.Decode(&body) != nil || body.Owner != lifecycleGitHubOrganization || !lifecycleRepositoryName.MatchString(body.Name) || !body.Private || body.IncludeAllBranches || !strings.HasPrefix(body.Description, marker) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return true
		}
		if _, err := uuid.Parse(strings.TrimPrefix(body.Description, marker)); err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return true
		}
		g.mu.Lock()
		defer g.mu.Unlock()
		if _, exists := g.repositories[body.Name]; exists {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return true
		}
		repository := githubapp.Repository{ID: int64(1000 + len(g.repositories)), OrganizationID: lifecycleGitHubOrganizationID, Name: body.Name, Description: body.Description, Private: true, TemplateID: 100}
		g.repositories[body.Name] = repository
		g.proof.RepositoryCreates++
		w.WriteHeader(http.StatusCreated)
		lifecycleJSON(w, lifecycleRepositoryJSON(repository))
		return true
	}
	if r.Method != http.MethodGet || !lifecycleRepositoryName.MatchString(name) {
		return false
	}
	if !g.repositoryToken(r, name, "metadata", "read") {
		w.WriteHeader(http.StatusForbidden)
		return true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	repository, exists := g.repositories[name]
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		return true
	}
	g.proof.RepositoryReads++
	lifecycleJSON(w, lifecycleRepositoryJSON(repository))
	return true
}
