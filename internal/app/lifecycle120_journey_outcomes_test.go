package app

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
	"github.com/google/uuid"
)

type lifecycleShowcaseReceipt struct {
	TeamOutcomes  int       `json:"team_outcomes"`
	NonWinner     uuid.UUID `json:"non_winning_team_id"`
	ProjectID     uuid.UUID `json:"project_id"`
	Version       int64     `json:"publication_version"`
	SearchFound   bool      `json:"search_found"`
	PrivateDenied bool      `json:"private_source_denied"`
	Withdrawn     bool      `json:"withdrawn_denied"`
}

// Every idea is created through its current team's native owner, including teams
// without an award. The public projection receives a separate organiser review;
// organisation discovery must never grant access to its restricted source.
func (r *lifecycleJourney) captureTeamOutcomesAndShowcase(t *testing.T) {
	t.Helper()
	stranger, err := signInLifecycle(t.Context(), "http://public:8080", "http://provider:8080", "stranger")
	if err != nil {
		t.Fatal(err)
	}
	for i := range r.events {
		item := &r.events[i]
		winners := map[uuid.UUID]bool{}
		for _, award := range item.resultDecision.Awards {
			winners[award.TeamID] = true
		}
		frozen := map[uuid.UUID]bool{}
		for _, source := range item.cutoff {
			frozen[source.TeamID] = source.Outcome == "frozen"
		}
		var selected *outcomes.View
		for _, team := range item.teams.Teams {
			if team.State != "published" || len(team.Published) == 0 {
				continue
			}
			client := r.participant(t, team.Published[0].ID)
			id := uuid.New()
			path := "/api/v1/outcomes/" + id.String()
			definition := outcomes.Definition{Kind: "team_outcome", Problem: "Restricted synthetic team learning " + id.String(), Audience: "Event follow-up owner", ProposedValue: "Private estimated benefit requiring evidence", Solution: "Synthetic team implementation", BacklogURL: "https://backlog.example.test/items/" + id.String()}
			var current outcomes.View
			change := func(actor *lifecycleClient, input outcomes.Command) {
				t.Helper()
				input.ID, input.Reason = uuid.New(), "Explicit synthetic follow-up decision"
				before := current.Version.Version
				if err := actor.call(t.Context(), http.MethodPut, path, before, input, &current, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				if current.Version.Version != before+1 || current.CommandID != input.ID {
					t.Fatal("outcome lost its exact original acknowledgement")
				}
			}
			change(client, outcomes.Command{Action: "propose", Definition: &definition, Provenance: &outcomes.Provenance{EventID: &item.view.ID, TeamID: &team.ID}})
			change(r.owner, outcomes.Command{Action: "moderate", Status: "accepted"})
			change(r.owner, outcomes.Command{Action: "assign", OwnerID: &client.person})
			change(client, outcomes.Command{Action: "progress", Status: "in_progress"})
			change(client, outcomes.Command{Action: "evidence", Evidence: &outcomes.Evidence{ID: uuid.New(), Basis: "estimate", Claim: "Synthetic estimate; adoption has not been demonstrated"}})
			observed := r.timeline.Completed
			change(client, outcomes.Command{Action: "evidence", Evidence: &outcomes.Evidence{ID: uuid.New(), Basis: "demonstrated_adoption", Claim: "Synthetic observed usage", Source: "https://evidence.example.test/observations/" + id.String(), ObservedAt: &observed}})
			var evidence outcomes.EvidencePage
			if err := client.call(t.Context(), http.MethodGet, path+"/evidence", -1, nil, &evidence, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if len(evidence.Items) != 2 {
				t.Fatal("outcome lost distinct estimated and evidenced observations")
			}
			bases := map[string]bool{}
			for _, value := range evidence.Items {
				bases[value.Basis] = true
			}
			if !bases["estimate"] || !bases["demonstrated_adoption"] {
				t.Fatal("estimated outcome was conflated with demonstrated adoption")
			}
			item.outcomes = append(item.outcomes, current)
			if selected == nil && !winners[team.ID] && frozen[team.ID] {
				copy := current
				selected = &copy
			}
		}
		if len(item.outcomes) == 0 || selected == nil {
			t.Fatal("showcase lacks a real frozen non-winner and all-team outcomes")
		}
		path := "/api/v1/outcomes/" + selected.ID.String()
		publicPath := "/api/v1/projects/" + selected.ID.String()
		fields := outcomes.PublicationFields{Problem: "Reviewed synthetic project " + selected.ID.String(), Solution: "Approved public description", Audience: "organisation", Links: []outcomes.ProjectLink{}, Media: []outcomes.MediaReference{}}
		var publication outcomes.PublicationView
		publish := func(action string) {
			t.Helper()
			command := outcomes.PublicationCommand{ID: uuid.New(), Action: action, SourceVersion: selected.Version.Version, Reason: "Separate deliberate positive-projection review"}
			if action == "save" {
				command.Fields = &fields
			}
			if err := r.owner.call(t.Context(), http.MethodPut, path+"/publication", publication.Version, command, &publication, http.StatusOK); err != nil {
				t.Fatal(err)
			}
		}
		publish("save")
		if err := stranger.call(t.Context(), http.MethodGet, publicPath, -1, nil, nil, http.StatusNotFound); err != nil {
			t.Fatal(err)
		}
		publish("publish")
		var project outcomes.Project
		if err := stranger.call(t.Context(), http.MethodGet, publicPath, -1, nil, &project, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if project.ID != selected.ID || project.Problem != fields.Problem || project.Solution != fields.Solution || len(project.Awards) != 0 || project.SourceReadable || project.EventName != nil || project.TeamName != nil {
			t.Fatal("non-winning reviewed project broadened private source or award access")
		}
		if err := stranger.call(t.Context(), http.MethodGet, path, -1, nil, nil, http.StatusNotFound); err != nil {
			t.Fatal(err)
		}
		var page outcomes.ProjectPage
		query := "/api/v1/projects?event_id=" + item.view.ID.String() + "&search=" + url.QueryEscape(selected.ID.String())
		if err := stranger.call(t.Context(), http.MethodGet, query, -1, nil, &page, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 1 || page.Items[0].ID != selected.ID {
			t.Fatal("reviewed non-winner was absent from organisation search")
		}
		publish("withdraw")
		if err := stranger.call(t.Context(), http.MethodGet, publicPath, -1, nil, nil, http.StatusNotFound); err != nil {
			t.Fatal(err)
		}
		if err := stranger.call(t.Context(), http.MethodGet, query, -1, nil, &page, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 0 {
			t.Fatal("withdrawn project remained discoverable")
		}
		item.showcase = lifecycleShowcaseReceipt{TeamOutcomes: len(item.outcomes), NonWinner: *selected.Provenance.TeamID, ProjectID: selected.ID, Version: publication.Version, SearchFound: true, PrivateDenied: true, Withdrawn: true}
	}
}
