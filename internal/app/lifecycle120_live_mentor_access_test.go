package app

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/repositoryaccess"
	"github.com/google/uuid"
)

// Current mentor repository authority begins at its published live window.
// The original T7 competitor-only approval remains historical; a human reviews
// and approves a distinct current source before any additional read grant.
func (r *lifecycleJourney) prepareLiveMentorAccess(t *testing.T) {
	t.Helper()
	for index := range r.events {
		item := &r.events[index]
		for _, original := range item.access {
			path := fmt.Sprintf("/api/v1/events/%s/repositories/%s/access", item.view.ID, original.TeamID)
			var before repositoryaccess.View
			if err := r.owner.call(t.Context(), http.MethodGet, path, -1, nil, &before, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			// Maintenance can legitimately replace the latest projection with an
			// unapproved live-coverage draft. Wait for that normal worker effect
			// deliberately; history is proved by its own immutable receipt.
			// Maintenance runs once per minute; include a full tick plus
			// execution margin without relying on the faster reconcile cadence.
			deadline := time.Now().Add(120 * time.Second)
			for before.Plan != nil && before.Plan.ID == original.PlanID && time.Now().Before(deadline) {
				select {
				case <-t.Context().Done():
					t.Fatal(t.Context().Err())
				case <-time.After(100 * time.Millisecond):
				}
				if err := r.owner.call(t.Context(), http.MethodGet, path, -1, nil, &before, http.StatusOK); err != nil {
					t.Fatal(err)
				}
			}
			if before.Plan == nil || before.Plan.ID == original.PlanID || before.Plan.Version <= original.PlanVersion || before.Plan.ApprovedAt != nil || before.Plan.ApprovedBy != nil || !before.CanManage || !before.GrantsOpen {
				t.Fatal("normal live roster maintenance did not retain a separate unapproved current plan")
			}
			verifyLifecycleLiveMentorPlan(t, item, *before.Plan, original.Accepted)
			var historical repositoryaccess.Receipt
			if err := r.owner.call(t.Context(), http.MethodGet, path+"/receipts/"+original.PlanID.String()+"?action=approve", -1, nil, &historical, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			old := historical.Plan
			if historical.Action != "approve" || historical.ActorID == nil || *historical.ActorID != original.ApproverID || historical.CommandFingerprint == nil || *historical.CommandFingerprint == "" || old == nil || old.ID != original.PlanID || old.Version != original.PlanVersion || old.Fingerprint != original.Fingerprint || old.EventID != original.EventID || old.TeamID != original.TeamID || old.RepositoryID != original.RepositoryID || old.ApprovedBy == nil || *old.ApprovedBy != original.ApproverID || old.ApprovedAt == nil || len(old.Members) != original.Accepted {
				t.Fatal("immutable preparation approval lost its original actor, source, version or roster")
			}
			var historicalActions []uuid.UUID
			for _, action := range old.Actions {
				historicalActions = append(historicalActions, action.ID)
			}
			if !reflect.DeepEqual(historicalActions, original.ActionIDs) {
				t.Fatal("historical preparation approval changed its exact original action identities")
			}

			id := uuid.New()
			var reviewed repositoryaccess.Result
			if err := r.owner.call(t.Context(), http.MethodPost, path, before.Revision, map[string]any{"action": "review", "plan_id": id, "reason": "Review newly active published mentor coverage; preserve earlier competitor grants"}, &reviewed, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			plan := reviewed.Receipt
			if plan == nil || plan.ID != id || plan.ID == original.PlanID || plan.ApprovedAt != nil || len(plan.Flags) != 0 || len(plan.Actions) != 2 || len(plan.Members) != original.Accepted+2 {
				t.Fatal("live source did not isolate two new current mentor effects")
			}
			verifyLifecycleLiveMentorPlan(t, item, *plan, original.Accepted)
			if plan.ID == before.Plan.ID || plan.Fingerprint != before.Plan.Fingerprint || !reflect.DeepEqual(plan.Members, before.Plan.Members) {
				t.Fatal("deliberate live review did not preserve the exact unapproved current roster under its distinct reviewed identity")
			}
			var approved repositoryaccess.Result
			if err := r.owner.call(t.Context(), http.MethodPost, path, reviewed.Current.Revision, map[string]any{"action": "approve", "plan_id": plan.ID, "plan_version": plan.Version, "reason": "Explicitly approve only two newly active read-only mentor invitations"}, &approved, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if approved.Receipt == nil || approved.Receipt.ID != id || approved.Receipt.Version != plan.Version || approved.Receipt.ApprovedAt == nil || approved.Receipt.Fingerprint != plan.Fingerprint {
				t.Fatal("native live approval changed reviewed source")
			}
			actions := []uuid.UUID{}
			for _, action := range plan.Actions {
				actions = append(actions, action.ID)
			}
			item.liveAccess = append(item.liveAccess, lifecycleAccessReceipt{TeamID: original.TeamID, PlanID: id, PlanVersion: plan.Version, RepositoryID: plan.RepositoryID, EventID: item.view.ID, Fingerprint: plan.Fingerprint, ActionIDs: actions, ApproverID: r.owner.person, Pending: 2, Accepted: len(plan.Members)})
			r.persistWorkProgress(t)
		}
	}
	deadline := time.Now().Add(120 * time.Second)
	for {
		all := true
		for index := range r.events {
			item := &r.events[index]
			for _, receipt := range item.liveAccess {
				path := fmt.Sprintf("/api/v1/events/%s/repositories/%s/access", item.view.ID, receipt.TeamID)
				var view repositoryaccess.View
				if err := r.owner.call(t.Context(), http.MethodGet, path, -1, nil, &view, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				if view.Plan == nil || view.Plan.ID != receipt.PlanID || view.Plan.Version != receipt.PlanVersion || view.Plan.Fingerprint != receipt.Fingerprint || !view.CanManage || !view.GrantsOpen || len(view.Execution) != 2 {
					t.Fatal("live mentor effect source/authority drifted")
				}
				ids := map[uuid.UUID]bool{}
				for _, id := range receipt.ActionIDs {
					ids[id] = true
				}
				delivered := true
				for _, effect := range view.Execution {
					if !ids[effect.ActionID] {
						t.Fatal("live plan substituted an invitation effect")
					}
					delete(ids, effect.ActionID)
					switch {
					case effect.State == "delivered" && effect.Attempted && effect.Diagnostic == "":
					case effect.State == "planned" && !effect.Attempted && effect.Diagnostic == "":
						delivered = false
					case effect.State == "ambiguous" && effect.Attempted && effect.Diagnostic == "external_effect_uncertain":
						delivered = false
					default:
						t.Fatal("live effect needs explicit recovery, not automatic replay")
					}
				}
				if len(ids) != 0 {
					t.Fatal("live plan lost an approved effect")
				}
				for _, flag := range view.Flags {
					if delivered || flag.Code != "external_effect_requires_reconciliation" && flag.Code != "external_invitation_not_app_managed" {
						t.Fatal("live source has a nontransient native flag requiring review", flag.Code)
					}
				}
				if !delivered {
					all = false
					continue
				}
				if len(view.Flags) != 0 || len(view.Grants) != receipt.Accepted {
					t.Fatal("delivered live effects did not retain clean current grants")
				}
			}
		}
		if all {
			break
		}
		if !time.Now().Before(deadline) {
			t.Fatal("actual queued mentor invitations did not deliver within normal budget")
		}
		select {
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
	for index := range r.events {
		item := &r.events[index]
		for _, receipt := range item.liveAccess {
			path := fmt.Sprintf("/api/v1/events/%s/repositories/%s/access", item.view.ID, receipt.TeamID)
			var view repositoryaccess.View
			if err := r.owner.call(t.Context(), http.MethodGet, path, -1, nil, &view, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			pending := 0
			for _, grant := range view.Grants {
				if grant.Member.Role == "competitor" {
					if grant.State != "accepted" || !grant.Managed || grant.Member.Permission != "push" {
						t.Fatal("live expansion altered existing competitor access")
					}
					continue
				}
				if grant.Member.Role != "mentor" || grant.Member.Permission != "pull" || grant.State != "pending" || grant.InvitationID == nil || !grant.Managed {
					t.Fatal("new mentor grant was not distinctly pending read-only access")
				}
				var client, external *lifecycleClient
				for _, actor := range []string{"mentor-a", "mentor-b"} {
					candidate := item.cohort.staff[actor]
					if candidate.person == grant.Member.PersonID {
						client = candidate
						external = r.events[0].cohort.staff[actor]
					}
				}
				if client == nil || external == nil || external.person != client.person {
					t.Fatal("unexpected mentor account in current source")
				}
				var own repositoryaccess.OwnView
				ownPath := fmt.Sprintf("/api/v1/events/%s/repositories/%s/invitations", item.view.ID, receipt.TeamID)
				if err := client.call(t.Context(), http.MethodGet, ownPath, -1, nil, &own, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				if len(own.Grants) != 1 || own.Grants[0].ID != grant.ID || own.CurrentGitHubID == nil || *own.CurrentGitHubID != grant.Member.GitHubID {
					t.Fatal("live own mentor invitation disclosed another person or numeric account")
				}
				r.acceptExternalInvitation(t, external, *grant.InvitationID)
				pending++
			}
			if pending != 2 {
				t.Fatal("live expansion did not require two separate external acceptances")
			}
			var refreshed repositoryaccess.Result
			if err := r.owner.call(t.Context(), http.MethodPost, path, view.Revision, map[string]any{"action": "refresh", "reason": "Independently observe two actual external mentor acceptances"}, &refreshed, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if refreshed.Current.Plan == nil || refreshed.Current.Plan.ID != receipt.PlanID || len(refreshed.Current.Flags) != 0 || len(refreshed.Current.Grants) != receipt.Accepted {
				t.Fatal("native refresh lost exact approved live source")
			}
			for _, grant := range refreshed.Current.Grants {
				if grant.State != "accepted" || !grant.Managed || grant.Uncertain || grant.Diagnostic != "" {
					t.Fatal("native provider observation did not verify current accepted grants")
				}
				if grant.Member.Role == "mentor" {
					var client *lifecycleClient
					for _, actor := range []string{"mentor-a", "mentor-b"} {
						candidate := item.cohort.staff[actor]
						if candidate.person == grant.Member.PersonID {
							client = candidate
						}
					}
					if client == nil {
						t.Fatal("accepted mentor grant substituted selected identity")
					}
					var own repositoryaccess.OwnView
					if err := client.call(t.Context(), http.MethodGet, fmt.Sprintf("/api/v1/events/%s/repositories/%s/invitations", item.view.ID, receipt.TeamID), -1, nil, &own, http.StatusOK); err != nil {
						t.Fatal(err)
					}
					if len(own.Grants) != 1 || own.Grants[0].ID != grant.ID || own.Grants[0].State != "accepted" || own.Grants[0].Member.Permission != "pull" {
						t.Fatal("native mentor own reader lost accepted read-only grant")
					}
				}
			}
			var original repositoryaccess.Receipt
			if err := r.owner.call(t.Context(), http.MethodGet, path+"/receipts/"+receipt.PlanID.String()+"?action=approve", -1, nil, &original, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if original.Plan == nil || original.Plan.ID != receipt.PlanID || original.Plan.Version != receipt.PlanVersion || original.Plan.Fingerprint != receipt.Fingerprint || original.ActorID == nil || *original.ActorID != r.owner.person || original.CommandFingerprint == nil {
				t.Fatal("live original approval acknowledgement changed actor/source/version")
			}
		}
		r.persistWorkProgress(t)
	}
	// Explicit refresh is not poll proof. A distinct later normal live minute
	// makes maintenance queue a new observation; only that worker can satisfy it.
	r.livePollAt = r.timeline.Live.Add(time.Minute)
	if err := r.control.advance(t.Context(), r.livePollAt); err != nil {
		t.Fatal(err)
	}
}

// Automatic roster maintenance never approves additional external access. Both
// the worker draft and the deliberate human review must contain exactly these
// two new current read-only mentor effects, retaining all accepted competitors.
func verifyLifecycleLiveMentorPlan(t *testing.T, item *lifecycleJourneyEvent, plan repositoryaccess.Plan, accepted int) {
	t.Helper()
	if plan.ApprovedAt != nil || plan.ApprovedBy != nil || len(plan.Flags) != 0 || len(plan.Actions) != 2 || len(plan.Members) != accepted+2 {
		t.Fatal("live plan inferred approval or changed current mentor/competitor census")
	}
	mentors := map[uuid.UUID]bool{item.cohort.staff["mentor-a"].person: true, item.cohort.staff["mentor-b"].person: true}
	for _, action := range plan.Actions {
		member := action.Member
		if action.Kind != "invite" || !mentors[member.PersonID] || member.Role != "mentor" || member.Permission != "pull" || member.CoverageStart == nil || member.CoverageEnd == nil || !member.CoverageStart.Equal(item.view.Configuration.LiveStart) || !member.CoverageEnd.Equal(item.view.Configuration.LiveEnd) {
			t.Fatal("live review expanded beyond exact current read-only mentor coverage")
		}
		delete(mentors, member.PersonID)
	}
	if len(mentors) != 0 {
		t.Fatal("live review omitted a selected mentor")
	}
}
