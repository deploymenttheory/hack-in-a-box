package app

import (
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/google/uuid"
)

type lifecycleHelpIntent struct {
	CommandID        uuid.UUID `json:"command_id"`
	ActorID          uuid.UUID `json:"actor_id"`
	Action           string    `json:"action"`
	ExpectedRevision int64     `json:"expected_revision"`
	ResponseRevision int64     `json:"response_revision"`
	HTTPStatus       int       `json:"http_status"`
}

type lifecycleHelpReceipt struct {
	TeamID             uuid.UUID             `json:"team_id"`
	EarlyRequestID     uuid.UUID             `json:"early_request_id"`
	EarlyCommandID     uuid.UUID             `json:"early_command_id"`
	EarlyOutputDenied  bool                  `json:"early_output_denied"`
	EarlyRequestAbsent bool                  `json:"early_request_absent"`
	RequestID          uuid.UUID             `json:"request_id"`
	Intents            []lifecycleHelpIntent `json:"intents"`
	WinnerID           uuid.UUID             `json:"winner_id"`
	LoserID            uuid.UUID             `json:"loser_id"`
	ClaimSuccesses     int                   `json:"claim_successes"`
	ClaimConflicts     int                   `json:"claim_conflicts"`
	OtherTeamDenied    bool                  `json:"other_team_denied"`
	Resolved           bool                  `json:"resolved"`
	FinalRevision      int64                 `json:"final_revision"`
}

func lifecycleOutputHelpInput(teamID uuid.UUID) map[string]any {
	return map[string]any{"team_id": teamID, "category": "competition_output", "description": "PRIVATE synthetic request for help demonstrating the current team's work.", "location": "Synthetic team desk", "meeting_url": ""}
}

func lifecycleHelpAction(action string) map[string]any {
	return map[string]any{"action": action, "assignee_id": nil}
}

func (r *lifecycleJourney) denyEarlyOutputHelp(t *testing.T) {
	t.Helper()
	for i := range r.events {
		item := &r.events[i]
		var teamID uuid.UUID
		for _, team := range item.teams.Teams {
			if team.State == "published" {
				teamID = team.ID
				break
			}
		}
		if teamID == uuid.Nil {
			t.Fatal("early help lacks an actual published team")
		}
		proof := &item.liveHelp
		proof.TeamID, proof.EarlyRequestID, proof.EarlyCommandID = teamID, uuid.New(), uuid.New()
		own, _, _ := r.feedbackReaders(t, item, teamID)
		path := "/api/v1/events/" + item.view.ID.String() + "/help/" + proof.EarlyRequestID.String()
		var denied struct {
			Error struct {
				Code     string        `json:"code"`
				Blockers []event.Issue `json:"blockers"`
			} `json:"error"`
		}
		if err := own.callIntent(t.Context(), http.MethodPut, path, 0, lifecycleOutputHelpInput(teamID), &denied, http.StatusConflict, proof.EarlyCommandID); err != nil {
			t.Fatal(err)
		}
		blocked := false
		for _, issue := range denied.Error.Blockers {
			blocked = blocked || issue.Code == "help_output_before_live" && issue.Field == "help"
		}
		if denied.Error.Code != "event_blocked" || !blocked {
			t.Fatal("prelive output help did not retain the actual live gate")
		}
		proof.EarlyOutputDenied = true
		if err := own.call(t.Context(), http.MethodGet, path, -1, nil, nil, http.StatusNotFound); err != nil {
			t.Fatal(err)
		}
		proof.EarlyRequestAbsent = true
		r.persistWorkProgress(t)
	}
}

func (r *lifecycleJourney) raceLiveHelp(t *testing.T) {
	t.Helper()
	for i := range r.events {
		item := &r.events[i]
		proof := &item.liveHelp
		if !proof.EarlyOutputDenied || !proof.EarlyRequestAbsent {
			t.Fatal("live help lost the retained prelive denial")
		}
		own, other, _ := r.feedbackReaders(t, item, proof.TeamID)
		proof.RequestID = uuid.New()
		path := "/api/v1/events/" + item.view.ID.String() + "/help/" + proof.RequestID.String()
		createID := uuid.New()
		var created help.Page
		if err := own.callIntent(t.Context(), http.MethodPut, path, 0, lifecycleOutputHelpInput(proof.TeamID), &created, http.StatusOK, createID); err != nil {
			t.Fatal(err)
		}
		if created.Request.ID != proof.RequestID || created.Request.TeamID != proof.TeamID || created.Request.RequesterID != own.person || created.Request.Revision != 1 || created.Request.Status != "waiting" || created.Request.AssigneeID != nil {
			t.Fatal("native help creation inferred an assignment or wrong source")
		}
		proof.Intents = append(proof.Intents, lifecycleHelpIntent{CommandID: createID, ActorID: own.person, Action: "create", ExpectedRevision: 0, ResponseRevision: 1, HTTPStatus: http.StatusOK})
		if err := other.call(t.Context(), http.MethodGet, path, -1, nil, nil, http.StatusNotFound); err != nil {
			t.Fatal(err)
		}
		proof.OtherTeamDenied = true
		type result struct {
			client    *lifecycleClient
			commandID uuid.UUID
			page      help.Page
			err       error
		}
		attempts := []result{{client: item.cohort.staff["mentor-a"], commandID: uuid.New()}, {client: item.cohort.staff["mentor-b"], commandID: uuid.New()}}
		for _, attempt := range attempts {
			var ready help.Page
			if err := attempt.client.call(t.Context(), http.MethodGet, path, -1, nil, &ready, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if !ready.CanClaim || ready.Request.Revision != 1 {
				t.Fatal("selected live helper lacks current claim authority")
			}
		}
		start := make(chan struct{})
		var workers sync.WaitGroup
		for index := range attempts {
			workers.Add(1)
			go func(index int) {
				defer workers.Done()
				<-start
				a := &attempts[index]
				a.err = a.client.callIntent(t.Context(), http.MethodPost, path+"/actions", 1, lifecycleHelpAction("claim"), &a.page, http.StatusOK, a.commandID)
			}(index)
		}
		close(start)
		workers.Wait()
		var winner *lifecycleClient
		for _, attempt := range attempts {
			status := http.StatusOK
			revision := int64(0)
			if attempt.err == nil {
				proof.ClaimSuccesses++
				winner = attempt.client
				proof.WinnerID = attempt.client.person
				revision = attempt.page.Request.Revision
				if revision != 2 || attempt.page.Request.Status != "claimed" || attempt.page.Request.AssigneeID == nil || *attempt.page.Request.AssigneeID != attempt.client.person {
					t.Fatal("successful claim lacks actual native ownership")
				}
			} else {
				var conflict *lifecycleStatusError
				if !errors.As(attempt.err, &conflict) || conflict.Actual != http.StatusConflict {
					t.Fatal("help race returned a non-conflict failure")
				}
				status = conflict.Actual
				proof.ClaimConflicts++
				proof.LoserID = attempt.client.person
			}
			proof.Intents = append(proof.Intents, lifecycleHelpIntent{CommandID: attempt.commandID, ActorID: attempt.client.person, Action: "claim", ExpectedRevision: 1, ResponseRevision: revision, HTTPStatus: status})
		}
		if proof.ClaimSuccesses != 1 || proof.ClaimConflicts != 1 || winner == nil || proof.WinnerID == proof.LoserID {
			t.Fatal("concurrent native help claims did not have exactly one winner")
		}
		var current help.Page
		if err := own.call(t.Context(), http.MethodGet, path, -1, nil, &current, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if current.Request.Revision != 2 || current.Request.AssigneeID == nil || *current.Request.AssigneeID != proof.WinnerID || len(current.History) != 2 {
			t.Fatal("losing help claim mutated native ownership/history")
		}
		resolveID := uuid.New()
		var resolved help.Page
		if err := winner.callIntent(t.Context(), http.MethodPost, path+"/actions", current.Request.Revision, lifecycleHelpAction("resolve"), &resolved, http.StatusOK, resolveID); err != nil {
			t.Fatal(err)
		}
		if resolved.Request.Revision != 3 || resolved.Request.Status != "resolved" || resolved.Request.ResolvedAt == nil || len(resolved.History) != 3 {
			t.Fatal("actual winning helper did not resolve the request once")
		}
		proof.Intents = append(proof.Intents, lifecycleHelpIntent{CommandID: resolveID, ActorID: winner.person, Action: "resolve", ExpectedRevision: 2, ResponseRevision: 3, HTTPStatus: http.StatusOK})
		proof.Resolved = true
		proof.FinalRevision = resolved.Request.Revision
		r.persistWorkProgress(t)
	}
}
