package app

import (
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/repositoryaccess"
	"github.com/google/uuid"
)

func lifecyclePendingAccessProof() (repositoryaccess.View, lifecycleAccessReceipt) {
	event, team, plan, action := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	invitation := int64(20001)
	approved := time.Date(2026, time.March, 23, 9, 0, 0, 0, time.UTC)
	receipt := lifecycleAccessReceipt{EventID: event, TeamID: team, PlanID: plan, PlanVersion: 1, RepositoryID: 1000, Fingerprint: "original frozen membership", ActionIDs: []uuid.UUID{action}, ApproverID: uuid.New()}
	view := repositoryaccess.View{CanManage: true, GrantsOpen: true, Plan: &repositoryaccess.Plan{ID: plan, Version: 1, EventID: event, TeamID: team, RepositoryID: 1000, Fingerprint: receipt.Fingerprint, ApprovedAt: &approved, ApprovedBy: &receipt.ApproverID, Actions: []repositoryaccess.Action{{ID: action}}, Members: []repositoryaccess.Member{{}}}, Execution: []repositoryaccess.Execution{{ActionID: action, State: "delivered", Attempted: true}}, Grants: []repositoryaccess.Grant{{ID: action, State: "pending", Managed: true, InvitationID: &invitation}}}
	return view, receipt
}

func TestLifecycleAccessObservationWaitsForOriginalDispatchBarrier(t *testing.T) {
	view, receipt := lifecyclePendingAccessProof()
	view.Execution[0].State = "planned"
	view.Execution[0].Attempted = false
	view.Grants = nil
	if ready, err := lifecycleOriginalInvitationsPending(view, receipt); err != nil || ready {
		t.Fatal("planned original effect was not observed without acceptance", err)
	}
	// The actual native barrier persists precisely these states before calling
	// GitHub. Its visibility is not a failed approval or permission to retry.
	view.Execution[0].State = "ambiguous"
	view.Execution[0].Attempted = true
	view.Execution[0].Diagnostic = "external_effect_uncertain"
	view.Flags = []repositoryaccess.Flag{{Code: "external_effect_requires_reconciliation"}}
	if ready, err := lifecycleOriginalInvitationsPending(view, receipt); err != nil || ready {
		t.Fatal("in-flight original barrier incorrectly failed or opened acceptance", err)
	}
	view, receipt = lifecyclePendingAccessProof()
	if ready, err := lifecycleOriginalInvitationsPending(view, receipt); err != nil || !ready {
		t.Fatal("known delivered original effect did not open its pending invitation", err)
	}
}

func TestLifecycleAccessObservationNeverAcceptsDriftOrUnresolvedEvidence(t *testing.T) {
	cases := map[string]func(*repositoryaccess.View){
		"different-approver":               func(v *repositoryaccess.View) { v.Plan.ApprovedBy = nil },
		"new-plan-version":                 func(v *repositoryaccess.View) { v.Plan.Version++ },
		"changed-frozen-source":            func(v *repositoryaccess.View) { v.Plan.Fingerprint = "another source" },
		"different-repository":             func(v *repositoryaccess.View) { v.Plan.RepositoryID++ },
		"lost-management":                  func(v *repositoryaccess.View) { v.CanManage = false },
		"unapproved":                       func(v *repositoryaccess.View) { v.Plan.ApprovedAt = nil },
		"substituted-effect":               func(v *repositoryaccess.View) { v.Execution[0].ActionID = uuid.New() },
		"provider-success-without-attempt": func(v *repositoryaccess.View) { v.Execution[0].Attempted = false },
		"settled-effect-with-uncertain-flag": func(v *repositoryaccess.View) {
			v.Flags = []repositoryaccess.Flag{{Code: "external_effect_requires_reconciliation"}}
		},
		"unrelated-warning-during-dispatch": func(v *repositoryaccess.View) {
			v.Execution[0].State = "planned"
			v.Execution[0].Attempted = false
			v.Flags = []repositoryaccess.Flag{{Code: "unknown_external_collaborator"}}
		},
		"accepted-before-external-person": func(v *repositoryaccess.View) { v.Grants[0].State = "accepted" },
		"unmanaged-invitation":            func(v *repositoryaccess.View) { v.Grants[0].Managed = false },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			view, receipt := lifecyclePendingAccessProof()
			change(&view)
			if ready, err := lifecycleOriginalInvitationsPending(view, receipt); err == nil || ready {
				t.Fatal("unsafe current evidence opened external acceptance")
			}
		})
	}
}
