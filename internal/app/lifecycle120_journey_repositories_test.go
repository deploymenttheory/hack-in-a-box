package app

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/repositories"
)

// Request real durable work through the normal owner API, then allow the native
// worker's normal minute reconciliation to enqueue it. No direct handler,
// maintenance call, queue timestamp or native mapping supplies completion.
func (r *lifecycleJourney) provisionRepositories(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	for i := range r.events {
		item := &r.events[i]
		path := "/api/v1/events/" + item.view.ID.String() + "/repositories"
		if err := item.cohort.clients[3].call(ctx, http.MethodPost, path, item.view.Revision, map[string]string{"action": "queue"}, nil, http.StatusForbidden); err != nil {
			t.Fatal(err)
		}
		var pending []repositories.View
		if err := r.owner.call(ctx, http.MethodPost, path, item.view.Revision, map[string]string{"action": "queue"}, &pending, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if len(pending) != 6 {
			t.Fatal("native provisioning omitted a frozen team", len(pending))
		}
	}
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		ready := true
		ids := map[int64]bool{}
		for i := range r.events {
			item := &r.events[i]
			path := "/api/v1/events/" + item.view.ID.String() + "/repositories"
			var current []repositories.View
			if err := r.owner.call(ctx, http.MethodGet, path, -1, nil, &current, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if len(current) != 6 {
				t.Fatal("native repository census changed", len(current))
			}
			teams := map[uuid.UUID]bool{}
			for _, team := range item.teams.Teams {
				if team.State != "disbanded" {
					teams[team.ID] = true
				}
			}
			for _, repository := range current {
				if !teams[repository.TeamID] || repository.Name != repositories.Name(item.view.ID, repository.TeamID) || repository.Organization != lifecycleGitHubOrganization || repository.Unused {
					t.Fatal("native repository identity crossed frozen event/team scope")
				}
				delete(teams, repository.TeamID)
				if repository.State == "review" || repository.State == "failed" {
					t.Fatal("native repository requires operator review", repository.Diagnostic)
				}
				if repository.State != "ready" || repository.GitHubID == nil {
					ready = false
					continue
				}
				if *repository.GitHubID <= 100 || ids[*repository.GitHubID] {
					t.Fatal("provider repository identity reused across teams/events")
				}
				ids[*repository.GitHubID] = true
			}
			if len(teams) != 0 {
				t.Fatal("native repository omitted actual frozen team")
			}
			item.repositories = current
		}
		if ready {
			for _, item := range r.events {
				for _, team := range item.teams.Teams {
					if team.State == "disbanded" {
						continue
					}
					if len(team.Published) == 0 {
						t.Fatal("native frozen team lost its members")
					}
					member := r.participant(t, team.Published[0].ID)
					path := "/api/v1/events/" + item.view.ID.String() + "/repositories/" + team.ID.String() + "/invitations"
					if err := member.call(ctx, http.MethodGet, path, -1, nil, nil, http.StatusForbidden); err != nil {
						t.Fatal(err)
					}
				}
			}
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
	t.Fatal("real queued repository work did not produce every native ready mapping within normal reconciliation budget")
}
