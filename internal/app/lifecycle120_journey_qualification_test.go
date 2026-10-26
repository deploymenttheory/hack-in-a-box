package app

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/qualification"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/scorecards"
	"github.com/google/uuid"
)

type lifecycleQualificationReceipt struct {
	RunID                    uuid.UUID                              `json:"run_id"`
	FinalistIDs              []uuid.UUID                            `json:"finalist_ids"`
	ReserveIDs               []uuid.UUID                            `json:"reserve_ids"`
	SignoffBinding           string                                 `json:"signoff_binding"`
	SignedCardCount          int                                    `json:"signed_card_count"`
	VoteCount                int                                    `json:"vote_count"`
	DecisionCount            int                                    `json:"decision_count"`
	MissingVoteSignoffDenied bool                                   `json:"missing_vote_signoff_denied"`
	Commands                 []lifecycleQualificationCommandReceipt `json:"commands"`
}

type lifecycleQualificationCommandReceipt struct {
	RequestID        uuid.UUID `json:"request_id"`
	ActorID          uuid.UUID `json:"actor_id"`
	Action           string    `json:"action"`
	Fingerprint      string    `json:"fingerprint"`
	ExpectedRevision int64     `json:"expected_revision"`
	ExpectedVersion  int64     `json:"expected_version"`
	Version          int64     `json:"version"`
}

// Every command uses the current native revision/version once; the original
// actor-bound acknowledgement is recovered through GET, never by replaying POST.
func lifecycleChangeQualification(t *testing.T, client *lifecycleClient, path string, page qualification.Page, body map[string]any, receipt *lifecycleQualificationReceipt) qualification.Page {
	t.Helper()
	id := uuid.New()
	body["request_id"] = id
	body["version"] = page.Round.Version
	body["reason"] = "Recorded synthetic qualification decision against reviewed current frozen output"
	var next qualification.Page
	if err := client.call(t.Context(), http.MethodPost, path, page.Revision, body, &next, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	ack := next.Receipt
	if ack == nil || ack.RequestID != id || ack.RunID != receipt.RunID || ack.Action != body["action"] || ack.ExpectedRevision != page.Revision || ack.ExpectedVersion != page.Round.Version || ack.Version != next.Round.Version || next.Round.ActorID != client.person || ack.Fingerprint == "" {
		t.Fatal("qualification command lost exact actor/source/version acknowledgement")
	}
	var recovered qualification.Recovery
	if err := client.call(t.Context(), http.MethodGet, path+"/commands/"+id.String(), -1, nil, &recovered, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	if recovered.Receipt == nil || *recovered.Receipt != *ack {
		t.Fatal("original qualification acknowledgement changed on read-only recovery")
	}
	receipt.Commands = append(receipt.Commands, lifecycleQualificationCommandReceipt{RequestID: id, ActorID: client.person, Action: ack.Action, Fingerprint: ack.Fingerprint, ExpectedRevision: ack.ExpectedRevision, ExpectedVersion: ack.ExpectedVersion, Version: ack.Version})
	return next
}

func (r *lifecycleJourney) qualifyFrozenFinalists(t *testing.T) {
	t.Helper()
	for index := range r.events {
		item := &r.events[index]
		if item.rubric.Definition.Mode != rubric.MentorQualification {
			continue
		}
		checkpoint := item.review.FinalCheckpoint
		if checkpoint == nil || len(item.review.FinalHuddles) != 4 {
			t.Fatal("qualification lacks actual planned and signed final mentor sources")
		}
		path := fmt.Sprintf("/api/v1/events/%s/mentor-qualification/%s", item.view.ID, checkpoint.Definition.RunID)
		receipt := &lifecycleQualificationReceipt{RunID: checkpoint.Definition.RunID}
		var page qualification.Page
		if err := r.owner.call(t.Context(), http.MethodGet, path, -1, nil, &page, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		page = lifecycleChangeQualification(t, r.owner, path, page, map[string]any{"action": "configure_panel", "panel": qualification.Panel{MentorIDs: []uuid.UUID{item.cohort.staff["mentor-a"].person, item.cohort.staff["mentor-b"].person}, OrganiserID: r.owner.person}}, receipt)
		frozen := []uuid.UUID{}
		for _, team := range page.Teams {
			if team.Outcome == "frozen" {
				frozen = append(frozen, team.TeamID)
			}
		}
		if len(frozen) != 4 {
			t.Fatal("native qualification census changed genuine frozen entries")
		}
		for _, id := range frozen {
			var current *qualification.TeamStatus
			for _, team := range page.Teams {
				if team.TeamID == id {
					copy := team
					current = &copy
				}
			}
			if current == nil || current.Source == nil || !current.CanSaveCard || !current.Eligible || len(current.Blockers) != 0 || current.CardVersion != 0 {
				t.Fatal("current frozen entry lacks signed independent qualification source")
			}
			scores := []scorecards.CategoryScore{}
			for _, category := range page.Criteria.Categories {
				value := category.MaxScore
				scores = append(scores, scorecards.CategoryScore{CategoryID: category.ID, Value: &value, Evidence: []scorecards.EvidenceReference{{Kind: "note", Note: "Explicitly moderated current frozen final; independent judge cards remain fresh"}}})
			}
			body := map[string]any{"action": "save_card", "freeze_binding": current.FreezeBinding, "card": qualification.CardInput{TeamID: id, Version: 0, Scores: scores}}
			if current.Source.RepositoryDecisionID != nil {
				body["repository_decision_id"] = *current.Source.RepositoryDecisionID
			}
			page = lifecycleChangeQualification(t, r.owner, path, page, body, receipt)
			var saved *qualification.TeamStatus
			for _, team := range page.Teams {
				if team.TeamID == id {
					copy := team
					saved = &copy
				}
			}
			if saved == nil || !saved.CanSignCard || saved.CardVersion != 1 || saved.CardState != "draft" || !saved.CardSourceCurrent {
				t.Fatal("native qualification did not retain exact reviewed current card")
			}
			body = map[string]any{"action": "sign_card", "team_id": id, "card_version": saved.CardVersion, "freeze_binding": saved.FreezeBinding}
			if saved.Source != nil && saved.Source.RepositoryDecisionID != nil {
				body["repository_decision_id"] = *saved.Source.RepositoryDecisionID
			}
			page = lifecycleChangeQualification(t, r.owner, path, page, body, receipt)
		}
		page = lifecycleChangeQualification(t, r.owner, path, page, map[string]any{"action": "propose"}, receipt)
		if page.Round.Proposal == nil || len(page.Round.Proposal.Ties) == 0 || page.Capabilities.CanSignOff {
			t.Fatal("equal synthetic final cards bypassed unresolved native panel votes")
		}
		before := page.Round.Version
		if err := r.owner.call(t.Context(), http.MethodPost, path, page.Revision, map[string]any{"request_id": uuid.New(), "action": "sign_off", "version": before, "reason": "Negative proof: do not sign without the required panel ballots"}, nil, http.StatusConflict); err != nil {
			t.Fatal(err)
		}
		if err := r.owner.call(t.Context(), http.MethodGet, path, -1, nil, &page, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if page.Round.Version != before || page.Round.Signoff != nil {
			t.Fatal("denied missing-vote signoff mutated qualification")
		}
		receipt.MissingVoteSignoffDenied = true
		for step := 0; !page.Capabilities.CanSignOff; step++ {
			if step >= 32 {
				t.Fatal("bounded native qualification seat workflow did not converge")
			}
			var active *qualification.TieAction
			for _, action := range page.TieActions {
				if len(action.CandidateTeamIDs) > 0 {
					copy := action
					active = &copy
					break
				}
			}
			if active == nil || active.RequiredVotes != 2 {
				t.Fatal("native qualification omitted a current two-mentor seat")
			}
			candidate := active.CandidateTeamIDs[0]
			for _, actor := range []string{"mentor-a", "mentor-b"} {
				client := item.cohort.staff[actor]
				if err := client.call(t.Context(), http.MethodGet, path, -1, nil, &page, http.StatusOK); err != nil {
					t.Fatal(err)
				}
				canVote := false
				for _, action := range page.TieActions {
					if action.TieID == active.TieID && action.Seat == active.Seat && action.CanVote && !action.HasVoted {
						canVote = true
					}
				}
				if !canVote {
					t.Fatal("current selected qualification panel member cannot cast its own missing vote")
				}
				page = lifecycleChangeQualification(t, client, path, page, map[string]any{"action": "vote_tie", "tie_id": active.TieID, "seat": active.Seat, "team_id": candidate, "evidence": []scorecards.EvidenceReference{{Kind: "note", Note: "Recorded explicit preference from current reviewed final output"}}}, receipt)
			}
			if err := r.owner.call(t.Context(), http.MethodGet, path, -1, nil, &page, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			ready := false
			for _, action := range page.TieActions {
				if action.TieID == active.TieID && action.Seat == active.Seat && action.VotesComplete && action.CanResolve && !action.Casting && len(action.ResolveTeamIDs) == 1 && action.ResolveTeamIDs[0] == candidate {
					ready = true
				}
			}
			if !ready {
				t.Fatal("complete unanimous qualification ballots did not identify one current leader")
			}
			page = lifecycleChangeQualification(t, r.owner, path, page, map[string]any{"action": "resolve_tie", "tie_id": active.TieID, "seat": active.Seat, "team_id": candidate, "evidence": []scorecards.EvidenceReference{{Kind: "note", Note: "Resolve the actual complete panel ballot; never choose a hidden default winner"}}}, receipt)
		}
		page = lifecycleChangeQualification(t, r.owner, path, page, map[string]any{"action": "sign_off"}, receipt)
		if page.Round.Signoff == nil || page.Round.Signoff.ActorID != r.owner.person || page.Round.Signoff.Binding == "" || page.Round.Proposal == nil || len(page.Round.Proposal.FinalistIDs) != 3 || len(page.Round.Proposal.ReserveIDs) != 1 {
			t.Fatal("native qualification did not retain current explicit finalist and reserve selection")
		}
		selected := map[uuid.UUID]bool{}
		for _, id := range append(append([]uuid.UUID{}, page.Round.Proposal.FinalistIDs...), page.Round.Proposal.ReserveIDs...) {
			if selected[id] {
				t.Fatal("native finalist/reserve selection overlapped")
			}
			selected[id] = true
		}
		if len(selected) != len(frozen) {
			t.Fatal("signed selection lost a genuine frozen team")
		}
		for _, id := range frozen {
			if !selected[id] {
				t.Fatal("selection substituted a nonfrozen team")
			}
		}
		receipt.FinalistIDs = page.Round.Proposal.FinalistIDs
		receipt.ReserveIDs = page.Round.Proposal.ReserveIDs
		receipt.SignoffBinding = page.Round.Signoff.Binding
		receipt.SignedCardCount = len(page.Round.Cards)
		receipt.VoteCount = len(page.Round.Votes)
		receipt.DecisionCount = len(page.Round.Decisions)
		item.review.Qualification = receipt
		r.persistWorkProgress(t)
	}
}
