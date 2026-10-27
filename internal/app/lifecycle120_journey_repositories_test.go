package app

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/repositories"
	"github.com/deploymenttheory/hack-in-a-box/internal/teams"
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
		if err := item.cohort.clients[3].call(
			ctx,
			http.MethodPost,
			path,
			item.view.Revision,
			map[string]string{"action": "queue"},
			nil,
			http.StatusForbidden,
		); err != nil {
			t.Fatal(err)
		}
		var pending []repositories.View
		if err := r.owner.call(
			ctx,
			http.MethodPost,
			path,
			item.view.Revision,
			map[string]string{"action": "queue"},
			&pending,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if len(pending) != 7 {
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
			if err := r.owner.call(
				ctx,
				http.MethodGet,
				path,
				-1,
				nil,
				&current,
				http.StatusOK,
			); err != nil {
				t.Fatal(err)
			}
			if len(current) != 7 {
				t.Fatal("native repository census changed", len(current))
			}
			teams := map[uuid.UUID]bool{}
			for _, team := range item.teams.Teams {
				if team.State != "disbanded" {
					teams[team.ID] = true
				}
			}
			unused := 0
			for _, repository := range current {
				if repository.Unused {
					unused++
					original := false
					for _, before := range item.repositories {
						if before.TeamID == repository.TeamID {
							before.Unused = true
							original = reflect.DeepEqual(before, repository)
						}
					}
					if !original || teams[repository.TeamID] || repository.State != "ready" ||
						repository.GitHubID == nil {
						t.Fatal(
							"retired team lost or substituted its original already-provisioned repository",
						)
					}
					continue
				}
				if !teams[repository.TeamID] ||
					repository.Name != repositories.Name(item.view.ID, repository.TeamID) ||
					repository.Organization != lifecycleGitHubOrganization ||
					repository.Unused {
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
			if len(teams) != 0 || unused != 1 {
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
					if err := member.call(
						ctx,
						http.MethodGet,
						path,
						-1,
						nil,
						nil,
						http.StatusForbidden,
					); err != nil {
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
	t.Fatal(
		"real queued repository work did not produce every native ready mapping within normal reconciliation budget",
	)
}

// Provision before the genuine under-three remediation. The original mapping
// must survive retirement as unused; later queueing must never recreate it.
func (r *lifecycleJourney) provisionInitialRepositories(
	t *testing.T,
	item *lifecycleJourneyEvent,
	page teams.Page,
) {
	t.Helper()
	base := "/api/v1/events/" + item.view.ID.String()
	if len(page.Teams) != 7 || page.Frozen {
		t.Fatal("initial repository case needs the original seven mutable published teams")
	}
	if err := r.owner.call(
		t.Context(),
		http.MethodGet,
		base,
		-1,
		nil,
		&item.view,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	r.configureGitHubApp(t, item)
	var pending []repositories.View
	if err := r.owner.call(
		t.Context(),
		http.MethodPost,
		base+"/repositories",
		item.view.Revision,
		map[string]string{"action": "queue"},
		&pending,
		http.StatusOK,
	); err != nil {
		t.Fatal(err)
	}
	if len(pending) != 7 {
		t.Fatal("initial native queue omitted a real published team")
	}
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		var current []repositories.View
		if err := r.owner.call(
			t.Context(),
			http.MethodGet,
			base+"/repositories",
			-1,
			nil,
			&current,
			http.StatusOK,
		); err != nil {
			t.Fatal(err)
		}
		if len(current) != 7 {
			t.Fatal("initial native repository census changed")
		}
		ready := true
		ids := map[int64]bool{}
		for _, repository := range current {
			if repository.Unused || repository.State == "failed" || repository.State == "review" {
				t.Fatal("initial original mapping is unusable", repository.Diagnostic)
			}
			if repository.State != "ready" || repository.GitHubID == nil {
				ready = false
				continue
			}
			if *repository.GitHubID <= 100 || ids[*repository.GitHubID] ||
				repository.Name != repositories.Name(item.view.ID, repository.TeamID) ||
				repository.Organization != lifecycleGitHubOrganization {
				t.Fatal("initial repository identity is not scoped to its original native team")
			}
			ids[*repository.GitHubID] = true
		}
		if ready {
			item.repositories = current
			return
		}
		select {
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
	t.Fatal(
		"actual initial repository work did not complete within the normal reconciliation budget",
	)
}

func (r *lifecycleJourney) verifyRetiredRepositoriesUnscored(t *testing.T) {
	t.Helper()
	for i := range r.events {
		item := &r.events[i]
		retired := 0
		for _, repository := range item.repositories {
			if !repository.Unused {
				continue
			}
			retired++
			if repository.GitHubID == nil || repository.State != "ready" {
				t.Fatal("retired original repository lost provenance")
			}
			var actual []repositories.View
			base := "/api/v1/events/" + item.view.ID.String()
			if err := r.owner.call(
				t.Context(),
				http.MethodGet,
				base+"/repositories",
				-1,
				nil,
				&actual,
				http.StatusOK,
			); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, current := range actual {
				if current.TeamID == repository.TeamID {
					found = reflect.DeepEqual(current, repository)
				}
			}
			if !found {
				t.Fatal("archived retired repository was deleted, reassigned or substituted")
			}
			cutoff := false
			for _, receipt := range item.cutoff {
				if receipt.TeamID == repository.TeamID {
					cutoff = receipt.Outcome == "withdrawn" && receipt.SelectedSHA == "" &&
						receipt.DecisionID == nil
				}
			}
			if !cutoff {
				t.Fatal("retired repository entered the final submission or SHA-selection census")
			}
			for _, id := range item.judgeFinal.EligibleTeamIDs {
				if id == repository.TeamID {
					t.Fatal("retired repository became eligible for real judging")
				}
			}
			for _, outcome := range item.outcomes {
				if outcome.Provenance.TeamID != nil &&
					*outcome.Provenance.TeamID == repository.TeamID {
					t.Fatal("retired repository acquired a scored team outcome")
				}
			}
			for _, client := range item.cohort.clients {
				var rows []repositories.View
				// Withdrawn people may not read event repositories; current people may only
				// see their current team, never the discarded mapping.
				state := ""
				for index, person := range item.cohort.clients {
					if person.person == client.person {
						state = item.cohort.registrations[index].State
					}
				}
				if state != "confirmed" {
					continue
				}
				if err := client.call(
					t.Context(),
					http.MethodGet,
					base+"/repositories",
					-1,
					nil,
					&rows,
					http.StatusOK,
				); err != nil {
					t.Fatal(fmt.Errorf("current participant repository privacy: %w", err))
				}
				for _, row := range rows {
					if row.TeamID == repository.TeamID {
						t.Fatal(
							"discarded repository leaked into a current participant's team list",
						)
					}
				}
			}
		}
		if retired != 1 {
			t.Fatal(
				"native journey lacks exactly one actually provisioned retired repository per event",
			)
		}
	}
}
