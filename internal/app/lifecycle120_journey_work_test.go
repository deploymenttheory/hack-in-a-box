package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

type lifecycleWorkReceipt struct {
	lifecycleExternalWorkReceipt
	TeamID           uuid.UUID `json:"team_id"`
	DomainObservedAt time.Time `json:"domain_observed_at"`
}

func (r *lifecycleJourney) externalWork(t *testing.T, client *lifecycleClient, teamID uuid.UUID, name, parent, content string) lifecycleWorkReceipt {
	t.Helper()
	ctx := t.Context()
	transport, err := newLifecycleProviderTransport()
	if err != nil {
		t.Fatal(err)
	}
	previous := client.http.Transport
	client.http.Transport = transport
	defer func() { client.http.Transport = previous; transport.base.CloseIdleConnections() }()
	body, _ := json.Marshal(map[string]string{"parent_sha": parent, "content": content})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://github.com/repositories/"+name+"/commits", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", "https://github.com")
	request.Header.Set("Content-Type", "application/json")
	response, err := client.http.Do(request)
	if err != nil {
		t.Fatal("strict synthetic external work request unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatal("authenticated accepted collaborator could not create external work", response.StatusCode)
	}
	var external lifecycleExternalWorkReceipt
	if err := json.NewDecoder(response.Body).Decode(&external); err != nil {
		t.Fatal(err)
	}
	if external.ID == uuid.Nil || external.Name != name || len(external.SHA) != 40 || external.ParentSHA != parent || len(external.ContentSHA256) != 64 {
		t.Fatal("external work did not return its own bounded receipt")
	}
	var phase struct {
		Now time.Time `json:"now"`
	}
	if err := r.control.call(ctx, http.MethodGet, "/clock", nil, &phase); err != nil {
		t.Fatal(err)
	}
	return lifecycleWorkReceipt{lifecycleExternalWorkReceipt: external, TeamID: teamID, DomainObservedAt: phase.Now}
}

func (r *lifecycleJourney) buildDuringPreparation(t *testing.T) {
	t.Helper()
	for index := range r.events {
		item := &r.events[index]
		for _, team := range item.teams.Teams {
			if team.State == "disbanded" {
				continue
			}
			name := ""
			var repositoryID int64
			for _, repository := range item.repositories {
				if repository.TeamID == team.ID && repository.GitHubID != nil {
					name = repository.Name
					repositoryID = *repository.GitHubID
				}
			}
			if name == "" {
				t.Fatal("current frozen team has no actual provisioned repository")
			}
			member := r.participant(t, team.Published[0].ID)
			work := r.externalWork(t, member, team.ID, name, "", "Synthetic reviewed challenge preparation work")
			if work.RepositoryID != repositoryID || !work.DomainObservedAt.Equal(r.timeline.Publication) || !work.DomainObservedAt.Before(r.timeline.Live) {
				t.Fatal("actual preparation work lost its repository or preweek phase")
			}
			item.work = append(item.work, work)
			path := fmt.Sprintf("/api/v1/events/%s/teams/%s/submission-freeze", item.view.ID, team.ID)
			var before struct {
				Revision     int64 `json:"revision"`
				CanConfigure bool  `json:"can_configure"`
			}
			if err := r.owner.call(t.Context(), http.MethodGet, path, -1, nil, &before, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if !before.CanConfigure {
				t.Fatal("native ready repository branch configuration is unavailable before cutoff")
			}
			var configured struct {
				Branch string `json:"branch"`
				Action string `json:"action"`
			}
			if err := r.owner.call(t.Context(), http.MethodPost, path, before.Revision, map[string]any{"request_id": uuid.New(), "action": "configure", "branch": "main", "reason": "Review managed repository main branch before formal output cutoff"}, &configured, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if configured.Branch != "main" || configured.Action != "configure" {
				t.Fatal("native branch configuration did not acknowledge the actual reviewed command")
			}
		}
		if len(item.work) != 6 {
			t.Fatal("preweek work omitted a current team")
		}
	}
}
