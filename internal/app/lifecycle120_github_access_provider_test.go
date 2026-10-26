package app

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

type lifecycleGitHubInvitation struct {
	ID         int64
	Repository string
	AccountID  int64
	Permission string
	CreatedAt  time.Time
}

func (g *lifecycleGitHub) accessToken(r *http.Request, name string, write bool) bool {
	level := "read"
	if write {
		level = "write"
	}
	return g.repositoryToken(r, name, "metadata", "read") && (g.repositoryToken(r, name, "administration", level) || !write && g.repositoryToken(r, name, "administration", "write"))
}

func (g *lifecycleGitHub) invitationJSON(invitation lifecycleGitHubInvitation) map[string]any {
	// Caller holds g.mu so the resource and its invitation are one snapshot.
	return map[string]any{"id": invitation.ID, "invitee": map[string]any{"id": invitation.AccountID}, "repository": map[string]any{"id": g.repositories[invitation.Repository].ID}, "permissions": invitation.Permission, "created_at": invitation.CreatedAt, "expired": false}
}

func (g *lifecycleGitHub) accessHTTP(w http.ResponseWriter, r *http.Request) bool {
	if strings.HasPrefix(r.URL.Path, "/github/invitations/") {
		return g.acceptInvitation(w, r)
	}
	if strings.HasPrefix(r.URL.Path, "/github-api/user/") {
		raw := strings.TrimPrefix(r.URL.Path, "/github-api/user/")
		id, err := strconv.ParseInt(raw, 10, 64)
		if r.Method != http.MethodGet || err != nil || g.logins[int(id-5000)] == "" {
			w.WriteHeader(http.StatusNotFound)
			return true
		}
		g.mu.Lock()
		names := make([]string, 0, len(g.repositories))
		for name := range g.repositories {
			names = append(names, name)
		}
		g.mu.Unlock()
		allowed := false
		for _, name := range names {
			allowed = allowed || g.accessToken(r, name, true)
		}
		if !allowed {
			w.WriteHeader(http.StatusForbidden)
			return true
		}
		lifecycleJSON(w, map[string]any{"id": id, "login": g.logins[int(id-5000)], "type": "User"})
		return true
	}
	prefix := "/github-api/repos/" + lifecycleGitHubOrganization + "/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/")
	if len(parts) < 2 || !lifecycleRepositoryName.MatchString(parts[0]) {
		return false
	}
	name, resource := parts[0], parts[1]
	if resource != "collaborators" && resource != "invitations" {
		return false
	}
	write := r.Method != http.MethodGet
	if !g.accessToken(r, name, write) {
		w.WriteHeader(http.StatusForbidden)
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, exists := g.repositories[name]; !exists {
		w.WriteHeader(http.StatusNotFound)
		return true
	}
	if len(parts) == 2 && r.Method == http.MethodGet {
		if resource == "collaborators" {
			if r.URL.Query().Get("affiliation") != "all" {
				w.WriteHeader(http.StatusBadRequest)
				return true
			}
			ids := make([]int64, 0, len(g.collaborators[name]))
			for id := range g.collaborators[name] {
				ids = append(ids, id)
			}
			slices.Sort(ids)
			values := []map[string]any{}
			for _, id := range ids {
				values = append(values, map[string]any{"id": id, "login": g.logins[int(id-5000)], "role_name": g.collaborators[name][id]})
			}
			lifecycleJSON(w, values)
		} else {
			ids := []int64{}
			for id, invite := range g.invitations {
				if invite.Repository == name {
					ids = append(ids, id)
				}
			}
			slices.Sort(ids)
			values := []map[string]any{}
			for _, id := range ids {
				values = append(values, g.invitationJSON(g.invitations[id]))
			}
			lifecycleJSON(w, values)
		}
		return true
	}
	if len(parts) == 3 && resource == "collaborators" && r.Method == http.MethodPut {
		actor, valid := g.actors[strings.TrimPrefix(parts[2], "lifecycle-")]
		var body struct {
			Permission string `json:"permission"`
		}
		decoder := json.NewDecoder(io.LimitReader(r.Body, 4096))
		decoder.DisallowUnknownFields()
		if !valid || decoder.Decode(&body) != nil || (body.Permission != "push" && body.Permission != "pull") {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return true
		}
		account := int64(5000 + actor)
		permission := "read"
		if body.Permission == "push" {
			permission = "write"
		}
		if _, exists := g.collaborators[name][account]; exists {
			w.WriteHeader(http.StatusNoContent)
			return true
		}
		for _, invite := range g.invitations {
			if invite.Repository == name && invite.AccountID == account {
				w.WriteHeader(http.StatusCreated)
				lifecycleJSON(w, g.invitationJSON(invite))
				return true
			}
		}
		g.proof.InvitationCreates++
		invite := lifecycleGitHubInvitation{ID: int64(20000 + g.proof.InvitationCreates), Repository: name, AccountID: account, Permission: permission, CreatedAt: time.Now().UTC()}
		g.invitations[invite.ID] = invite
		w.WriteHeader(http.StatusCreated)
		lifecycleJSON(w, g.invitationJSON(invite))
		return true
	}
	w.WriteHeader(http.StatusNotFound)
	return true
}

// This is external synthetic browser behavior, never a native grant mutation.
// An actual OAuth-authenticated GitHub cookie plus exact Origin is required;
// native refresh must independently observe the resulting collaborator later.
func (g *lifecycleGitHub) acceptInvitation(w http.ResponseWriter, r *http.Request) bool {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/github/invitations/"), "/")
	if r.Method != http.MethodPost || len(parts) != 2 || parts[1] != "accept" || r.Header.Get("Origin") != "https://github.com" {
		w.WriteHeader(http.StatusForbidden)
		return true
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	cookie, cookieErr := r.Cookie("lifecycle_github_browser")
	if err != nil || cookieErr != nil {
		w.WriteHeader(http.StatusForbidden)
		return true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	user, authenticated := g.webSessions[cookie.Value]
	invite, exists := g.invitations[id]
	if !authenticated || !time.Now().Before(user.Expires) || !exists || invite.AccountID != int64(5000+user.Actor) {
		w.WriteHeader(http.StatusForbidden)
		return true
	}
	if g.collaborators[invite.Repository] == nil {
		g.collaborators[invite.Repository] = map[int64]string{}
	}
	g.collaborators[invite.Repository][invite.AccountID] = invite.Permission
	delete(g.invitations, id)
	g.proof.InvitationAcceptances++
	w.WriteHeader(http.StatusNoContent)
	return true
}
