package app

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/repositoryaccess"
)

type lifecycleAccessReceipt struct {
	TeamID      uuid.UUID `json:"team_id"`
	PlanID      uuid.UUID `json:"plan_id"`
	PlanVersion int64     `json:"plan_version"`
	Pending     int       `json:"pending_before_external_acceptance"`
	Accepted    int       `json:"accepted_after_native_refresh"`
}

func (r *lifecycleJourney) participant(t *testing.T, id uuid.UUID) *lifecycleClient {
	t.Helper()
	for _, client := range r.events[0].cohort.clients {
		if client.person == id {
			return client
		}
	}
	t.Fatal("native team member has no retained authenticated participant browser")
	return nil
}

func (r *lifecycleJourney) prepareRepositoryAccess(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	for index := range r.events {
		item := &r.events[index]
		for _, team := range item.teams.Teams {
			if team.State == "disbanded" {
				continue
			}
			if len(team.Published) < 4 || len(team.Published) > 6 {
				t.Fatal("native frozen team violates the admitted target-five cohort")
			}
			path := fmt.Sprintf("/api/v1/events/%s/repositories/%s/access", item.view.ID, team.ID)
			var before repositoryaccess.View
			if err := r.owner.call(ctx, http.MethodGet, path, -1, nil, &before, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if !before.CanManage || !before.GrantsOpen {
				t.Fatal("actual common release did not open reviewed repository access")
			}
			id := uuid.New()
			command := map[string]any{"action": "review", "plan_id": id, "reason": "Synthetic current frozen team repository access review"}
			member := r.participant(t, team.Published[0].ID)
			if err := member.call(ctx, http.MethodPost, path, before.Revision, command, nil, http.StatusForbidden); err != nil {
				t.Fatal(err)
			}
			var reviewed repositoryaccess.Result
			if err := r.owner.call(ctx, http.MethodPost, path, before.Revision, command, &reviewed, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			plan := reviewed.Receipt
			if plan == nil || plan.ID != id || plan.ApprovedAt != nil || len(plan.Flags) != 0 || len(plan.Members) != len(team.Published) || len(plan.Actions) != len(team.Published) {
				t.Fatal("native access plan omitted verified frozen membership or assumed approval")
			}
			people := map[uuid.UUID]bool{}
			for _, person := range team.Published {
				people[person.ID] = true
			}
			for _, member := range plan.Members {
				if !people[member.PersonID] || member.GitHubID <= 0 || member.Role != "competitor" || member.Permission != "push" {
					t.Fatal("native access plan expanded current frozen team authority")
				}
				delete(people, member.PersonID)
			}
			if len(people) != 0 {
				t.Fatal("native access plan omitted a current frozen person")
			}
			for _, action := range plan.Actions {
				if action.Kind != "invite" {
					t.Fatal("fresh native access plan fabricated existing collaborator ownership")
				}
			}
			command = map[string]any{"action": "approve", "plan_id": plan.ID, "plan_version": plan.Version, "reason": "Explicitly approve exact verified frozen team access"}
			var approved repositoryaccess.Result
			if err := r.owner.call(ctx, http.MethodPost, path, reviewed.Current.Revision, command, &approved, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if approved.Receipt == nil || approved.Receipt.ID != plan.ID || approved.Receipt.Version != plan.Version || approved.Receipt.ApprovedAt == nil {
				t.Fatal("native access approval lost original exact plan identity")
			}
			item.access = append(item.access, lifecycleAccessReceipt{TeamID: team.ID, PlanID: plan.ID, PlanVersion: plan.Version})
		}
	}
	// Let the actual worker reconcile durable operations and send invitations.
	// Pending is checked separately from the external person's later acceptance.
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		complete := true
		for index := range r.events {
			item := &r.events[index]
			for _, receipt := range item.access {
				path := fmt.Sprintf("/api/v1/events/%s/repositories/%s/access", item.view.ID, receipt.TeamID)
				var view repositoryaccess.View
				if err := r.owner.call(ctx, http.MethodGet, path, -1, nil, &view, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				if view.Plan == nil || view.Plan.ID != receipt.PlanID || len(view.Flags) != 0 {
					t.Fatal("native queued access lost approved current plan")
				}
				if len(view.Grants) != len(view.Plan.Members) {
					complete = false
					continue
				}
				for _, grant := range view.Grants {
					if grant.State == "failed" || grant.State == "ambiguous" || grant.Uncertain {
						t.Fatal("native invitation requires operator review", grant.Diagnostic)
					}
					if grant.State != "pending" || !grant.Managed || grant.InvitationID == nil {
						complete = false
					}
				}
			}
		}
		if complete {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
	for index := range r.events {
		item := &r.events[index]
		for receiptIndex := range item.access {
			receipt := &item.access[receiptIndex]
			path := fmt.Sprintf("/api/v1/events/%s/repositories/%s/access", item.view.ID, receipt.TeamID)
			var pending repositoryaccess.View
			if err := r.owner.call(ctx, http.MethodGet, path, -1, nil, &pending, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if pending.Plan == nil || len(pending.Grants) != len(pending.Plan.Members) {
				t.Fatal("real queued invitations did not cover frozen membership within reconciliation budget")
			}
			for _, grant := range pending.Grants {
				if grant.State != "pending" || grant.InvitationID == nil || !grant.Managed {
					t.Fatal("fresh external invitation was not separately pending")
				}
				client := r.participant(t, grant.Member.PersonID)
				var own repositoryaccess.OwnView
				ownPath := fmt.Sprintf("/api/v1/events/%s/repositories/%s/invitations", item.view.ID, receipt.TeamID)
				if err := client.call(ctx, http.MethodGet, ownPath, -1, nil, &own, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				if len(own.Grants) != 1 || own.Grants[0].ID != grant.ID || own.CurrentGitHubID == nil || *own.CurrentGitHubID != grant.Member.GitHubID {
					t.Fatal("own native invitation reader leaked other collaborators or lost verified identity")
				}
				r.acceptExternalInvitation(t, client, *grant.InvitationID)
			}
			receipt.Pending = len(pending.Grants)
			var refreshed repositoryaccess.Result
			if err := r.owner.call(ctx, http.MethodPost, path, pending.Revision, map[string]any{"action": "refresh", "reason": "Observe accepted external collaborator access without granting new authority"}, &refreshed, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if len(refreshed.Current.Grants) != receipt.Pending {
				t.Fatal("native accepted grant census changed")
			}
			for _, grant := range refreshed.Current.Grants {
				if grant.State != "accepted" || !grant.Managed || grant.Diagnostic != "" || grant.Uncertain {
					t.Fatal("native refresh did not independently verify accepted current collaborator")
				}
				client := r.participant(t, grant.Member.PersonID)
				var own repositoryaccess.OwnView
				ownPath := fmt.Sprintf("/api/v1/events/%s/repositories/%s/invitations", item.view.ID, receipt.TeamID)
				if err := client.call(ctx, http.MethodGet, ownPath, -1, nil, &own, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				if len(own.Grants) != 1 || own.Grants[0].ID != grant.ID || own.Grants[0].State != "accepted" || !own.Grants[0].Managed {
					t.Fatal("current own API did not retain only its accepted managed grant")
				}
			}
			var original repositoryaccess.Receipt
			if err := r.owner.call(ctx, http.MethodGet, path+"/receipts/"+receipt.PlanID.String()+"?action=approve", -1, nil, &original, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if original.Plan == nil || original.Plan.ID != receipt.PlanID || original.Plan.Version != receipt.PlanVersion || original.ActorID == nil || *original.ActorID != r.owner.person || original.CommandFingerprint == nil {
				t.Fatal("native original approval receipt lost exact actor/source/version")
			}
			var repeated repositoryaccess.Result
			if err := r.owner.call(ctx, http.MethodPost, path, refreshed.Current.Revision, map[string]any{"action": "approve", "plan_id": receipt.PlanID, "plan_version": receipt.PlanVersion, "reason": "Explicitly approve exact verified frozen team access"}, &repeated, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if repeated.Receipt == nil || repeated.Receipt.ID != receipt.PlanID || len(repeated.Current.Grants) != len(refreshed.Current.Grants) {
				t.Fatal("known approval replay replaced original access history")
			}
			receipt.Accepted = len(refreshed.Current.Grants)
		}
	}
}

func (r *lifecycleJourney) acceptExternalInvitation(t *testing.T, client *lifecycleClient, id int64) {
	t.Helper()
	transport, err := newLifecycleProviderTransport()
	if err != nil {
		t.Fatal(err)
	}
	previous := client.http.Transport
	client.http.Transport = transport
	defer func() { client.http.Transport = previous; transport.base.CloseIdleConnections() }()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, fmt.Sprintf("https://github.com/invitations/%d/accept", id), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", "https://github.com")
	response, err := client.http.Do(request)
	if err != nil {
		t.Fatal("synthetic authenticated external invitation acceptance unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatal("external invitation acceptance did not preserve authenticated numeric ownership", response.StatusCode)
	}
}
