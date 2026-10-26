package app

import (
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/deploymenttheory/hack-in-a-box/internal/results"
	"github.com/google/uuid"
)

func (r *lifecycleJourney) adviseCurrentCategoryTie(t *testing.T, item *lifecycleJourneyEvent, page results.Page) results.Page {
	t.Helper()
	path := "/api/v1/events/" + item.view.ID.String() + "/results"
	advicePath := path + "/mentor-advisories"
	var selected *results.AdvisoryTarget
	for index, actor := range []string{"mentor-a", "mentor-b"} {
		client := item.cohort.staff[actor]
		var before results.AdvisoryPage
		if err := client.call(t.Context(), http.MethodGet, advicePath, -1, nil, &before, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if before.RunID != item.judgeRun || before.SourceBinding != page.Preview.Binding {
			t.Fatal("current mentor advisory source differs from prepared judge final")
		}
		for _, target := range before.Targets {
			if target.Own != nil {
				t.Fatal("new advisory author inherited another mentor's private contribution")
			}
		}
		var target *results.AdvisoryTarget
		for _, candidate := range before.Targets {
			if selected == nil || candidate.TieID == selected.TieID && candidate.TeamID == selected.TeamID {
				copy := candidate
				target = &copy
				break
			}
		}
		if target == nil || target.Version != 0 || target.SubmissionRevision != 1 {
			t.Fatal("selected mentor lacks an actual fresh current category target")
		}
		if selected == nil {
			selected = target
		}
		id := uuid.New()
		body := map[string]any{"request_id": id, "action": "advise", "version": target.Version, "source_binding": before.SourceBinding, "tie_id": target.TieID, "team_id": target.TeamID, "submission_revision": target.SubmissionRevision, "views": "PRIVATE synthetic independently authored " + actor + " category advisory; never a judge vote or copied score", "reason": "Explicit current assigned mentor advisory contribution"}
		var next results.AdvisoryPage
		if index == 0 {
			base := client.http.Transport
			if base == nil {
				base = http.DefaultTransport
			}
			loss := &lifecycleLostReplyTransport{base: base, target: client.origin + advicePath}
			var callErr error
			func() {
				previous := client.http.Transport
				client.http.Transport = loss
				defer func() { client.http.Transport = previous }()
				callErr = client.call(t.Context(), http.MethodPost, advicePath, before.Revision, body, &next, http.StatusOK)
			}()
			if !errors.Is(callErr, errLifecycleLostReply) || next.Receipt != nil || !loss.dropped.Load() || loss.posts.Load() != 1 {
				t.Fatal("controlled real accepted advisory reply loss was not exact and single-shot")
			}
			item.resultDecision.ControlledReplyLost = true
			item.resultDecision.ControlledReplyPOSTs = loss.posts.Load()
		} else if err := client.call(t.Context(), http.MethodPost, advicePath, before.Revision, body, &next, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		var recovered results.AdvisoryRecovery
		if err := client.call(t.Context(), http.MethodGet, advicePath+"/requests/"+id.String(), -1, nil, &recovered, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		ack := recovered.Receipt
		if ack == nil || ack.RequestID != id || ack.ActorID != client.person || ack.RunID != item.judgeRun || ack.Action != "advise" || ack.TieID != target.TieID || ack.TeamID != target.TeamID || ack.ExpectedRevision != before.Revision || ack.ExpectedVersion != 0 || ack.Version != 1 || ack.Fingerprint == "" {
			t.Fatal("original current mentor advisory acknowledgement lost source/actor/version")
		}
		if index > 0 && !reflect.DeepEqual(next.Receipt, ack) {
			t.Fatal("normal mentor advisory acknowledgement changed during recovery")
		}
		var current results.AdvisoryPage
		if err := client.call(t.Context(), http.MethodGet, advicePath, -1, nil, &current, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, candidate := range current.Targets {
			if candidate.TieID == target.TieID && candidate.TeamID == target.TeamID && candidate.Own != nil && candidate.Own.AuthorID == client.person && candidate.Own.Version == 1 {
				found = true
			}
		}
		if !found {
			t.Fatal("read-only advisory recovery did not retain exact own contribution")
		}
		item.resultDecision.Advisories = append(item.resultDecision.Advisories, *ack)
		r.persistWorkProgress(t)
	}
	after := r.currentResults(t, item)
	if after.Preview.Binding != page.Preview.Binding || after.Round.Version != page.Round.Version || !reflect.DeepEqual(after.Preview.Tallies, page.Preview.Tallies) || len(after.Round.Votes) != 0 || len(after.Advisories) != 2 {
		t.Fatal("mentor advice changed judge scoring, votes or prepared source")
	}
	for _, state := range after.TieStates {
		if state.VotesReceived != 0 || state.WinnerID != nil {
			t.Fatal("mentor advice counted as a final judge ballot")
		}
	}
	item.resultDecision.AdvisoryNoTallyOrVoteChange = true
	if selected == nil {
		t.Fatal("actual category advisory target missing")
	}
	var ballots results.BallotPage
	if err := item.cohort.staff["judge-a"].call(t.Context(), http.MethodGet, path+"/ballots", -1, nil, &ballots, http.StatusOK); err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, tie := range ballots.Ties {
		if tie.State.Tie.ID == selected.TieID && len(tie.Advisories) == 2 && tie.OwnVote == nil {
			seen = true
		}
	}
	if !seen {
		t.Fatal("intended judge panel lacks separately authored current category advice")
	}
	body := map[string]any{"request_id": uuid.New(), "action": "vote", "version": after.Round.Version, "source_binding": after.Preview.Binding, "tie_id": selected.TieID, "team_id": selected.TeamID, "evidence": []map[string]any{{"team_id": selected.TeamID, "note": "Negative proof: mentor advice grants no final judge ballot"}}, "reason": "Mentor cannot vote merely by contributing advice"}
	if err := item.cohort.staff["mentor-a"].call(t.Context(), http.MethodPost, path+"/ballots", after.Revision, body, nil, http.StatusForbidden); err != nil {
		t.Fatal(err)
	}
	item.resultDecision.MentorVoteDenied = true
	return r.currentResults(t, item)
}
