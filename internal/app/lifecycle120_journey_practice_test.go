package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

type lifecyclePracticeReceipt struct {
	JudgeID             uuid.UUID        `json:"judge_id"`
	FictionalEntryID    uuid.UUID        `json:"fictional_entry_id"`
	ActualRunID         uuid.UUID        `json:"actual_planned_run_id"`
	Rubric              rubric.Reference `json:"rubric"`
	Version             int64            `json:"practice_version"`
	RequestID           uuid.UUID        `json:"request_id"`
	Fingerprint         string           `json:"original_ack_fingerprint"`
	RealEntryReadDenied bool             `json:"early_real_entry_read_denied"`
	RealCardWriteDenied bool             `json:"early_real_card_write_denied"`
	ClosedAtLive        bool             `json:"practice_closed_at_live"`
}

// Real selected judge identities rehearse only the dedicated fictional owner.
// Round assignment is native planning; none of these requests starts or scores
// the actual competition run before live and frozen evidence are available.
func (r *lifecycleJourney) rehearseJudgePractice(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	for index := range r.events {
		item := &r.events[index]
		base := "/api/v1/events/" + item.view.ID.String()
		var configured rubric.Configuration
		if err := r.owner.call(ctx, http.MethodGet, base+"/rubric/configuration", -1, nil, &configured, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		var created rubric.Configuration
		if err := r.owner.call(ctx, http.MethodPost, base+"/rubric/configuration", configured.Revision, map[string]any{"action": "create_run", "version": configured.Reference.Version, "purpose": "judge_final", "reason": "Plan an independent final round; practice never scores this run"}, &created, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		for _, run := range created.Runs {
			if run.Purpose == "judge_final" {
				if item.judgeRun != uuid.Nil {
					t.Fatal("native practice planning created duplicate real final rounds")
				}
				item.judgeRun = run.ID
				if run.StartedAt != nil || run.Reference != item.rubric.Reference {
					t.Fatal("native planned final round was started or rebound by configuration")
				}
			}
		}
		if item.judgeRun == uuid.Nil {
			t.Fatal("native rubric owner did not acknowledge an actual planned judge-final run")
		}
		item.rubric = created
		judges := []*lifecycleClient{}
		judgeIDs := []uuid.UUID{}
		for _, actor := range []string{"judge-a", "judge-b", "judge-c"} {
			client := item.cohort.staff[actor]
			if client == nil {
				t.Fatal("actual selected judge identity missing", actor)
			}
			judges = append(judges, client)
			judgeIDs = append(judgeIDs, client.person)
		}
		path := fmt.Sprintf("%s/judging/%s", base, item.judgeRun)
		var round judging.Round
		if err := r.owner.call(ctx, http.MethodGet, path, -1, nil, &round, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		currentTeams := []uuid.UUID{}
		for _, team := range item.teams.Teams {
			if team.State == "disbanded" {
				continue
			}
			currentTeams = append(currentTeams, team.ID)
			var assigned judging.Receipt
			if err := r.owner.call(ctx, http.MethodPost, path, round.Revision, map[string]any{"request_id": uuid.New(), "action": "assign", "team_id": team.ID, "judge_ids": judgeIDs, "reason": "Three independently selected current judges, never mentor votes"}, &assigned, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if assigned.RunID != item.judgeRun || assigned.TeamID != team.ID || assigned.ActorID != r.owner.person {
				t.Fatal("native assignment acknowledged another round, team or actor")
			}
			if err := r.owner.call(ctx, http.MethodGet, path, -1, nil, &round, http.StatusOK); err != nil {
				t.Fatal(err)
			}
		}
		if round.RequiredJudges != 3 || len(round.Assignments) != 6 || len(currentTeams) != 6 {
			t.Fatal("native planned judging roster omitted a frozen team or configured judge")
		}
		for _, assignment := range round.Assignments {
			actual := map[uuid.UUID]bool{}
			for _, id := range assignment.JudgeIDs {
				actual[id] = true
			}
			if assignment.Missing != 0 || len(actual) != 3 {
				t.Fatal("native planned assignment is incomplete or duplicated")
			}
			for _, id := range judgeIDs {
				if !actual[id] {
					t.Fatal("native planned round substituted a judge")
				}
			}
		}
		for judgeIndex, judge := range judges {
			var raw json.RawMessage
			if err := judge.call(ctx, http.MethodGet, base+"/judge-practice", -1, nil, &raw, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(raw, []byte("PRIVATE synthetic judge guidance")) {
				t.Fatal("practice disclosed private judge guidance through public criteria")
			}
			var practice judging.Practice
			if err := json.Unmarshal(raw, &practice); err != nil {
				t.Fatal(err)
			}
			if practice.Card != nil || practice.FictionalEntryID == uuid.Nil || practice.Rubric != item.rubric.Reference || practice.Criteria.Mode != item.rubric.Definition.Mode {
				t.Fatal("native practice did not use its isolated approved rubric context")
			}
			for _, teamID := range currentTeams {
				if practice.FictionalEntryID == teamID {
					t.Fatal("fictional practice reused an actual frozen team identity")
				}
			}
			scores := []judging.Score{}
			for _, category := range practice.Criteria.Categories {
				maximum := category.MaxScore
				scores = append(scores, judging.Score{CategoryID: category.ID, Value: &maximum, Evidence: []judging.Evidence{{Note: "Fictional training maximum; never a competition output review"}}})
			}
			command := judging.PracticeCommand{RequestID: uuid.New(), Action: "submit", Scores: scores}
			var accepted judging.Receipt
			if os.Getenv("HACKBOX_LIFECYCLE_BROWSER_PHASES") == "ui-journey" {
				accepted = r.practiceInBrowser(t, item, index, judgeIndex, judge, practice, scores)
				command.RequestID = accepted.RequestID
			} else if err := judge.call(ctx, http.MethodPost, base+"/judge-practice", 0, command, &accepted, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if accepted.RequestID != command.RequestID || accepted.ActorID != judge.person || accepted.TeamID != practice.FictionalEntryID || accepted.RunID != uuid.Nil || accepted.Action != "practice_submit" || accepted.Revision != 1 || accepted.State != "submitted" {
				t.Fatal("native practice acknowledgement leaked into actual scoring namespace")
			}
			var current judging.Practice
			if err := judge.call(ctx, http.MethodGet, base+"/judge-practice", -1, nil, &current, http.StatusOK); err != nil {
				t.Fatal(err)
			}
			if current.Card == nil || current.Card.Version != 1 || current.Card.State != "submitted" || current.Card.Rubric != practice.Rubric || !reflect.DeepEqual(current.Card.Scores, scores) {
				t.Fatal("native practice did not retain its own submitted version")
			}
			realPath := fmt.Sprintf("%s/teams/%s", path, currentTeams[0])
			if err := judge.call(ctx, http.MethodGet, realPath, -1, nil, nil, http.StatusForbidden); err != nil {
				t.Fatal(err)
			}
			if err := judge.call(ctx, http.MethodPost, realPath, 0, map[string]any{"request_id": uuid.New(), "action": "submit", "submission_revision": 1, "scores": scores}, nil, http.StatusForbidden); err != nil {
				t.Fatal(err)
			}
			item.practice = append(item.practice, lifecyclePracticeReceipt{JudgeID: judge.person, FictionalEntryID: practice.FictionalEntryID, ActualRunID: item.judgeRun, Rubric: practice.Rubric, Version: current.Card.Version, RequestID: command.RequestID, Fingerprint: accepted.Fingerprint, RealEntryReadDenied: true, RealCardWriteDenied: true})
		}
		var unchanged rubric.Configuration
		if err := r.owner.call(ctx, http.MethodGet, base+"/rubric/configuration", -1, nil, &unchanged, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		for _, run := range unchanged.Runs {
			if run.ID == item.judgeRun && run.StartedAt != nil {
				t.Fatal("practice or denied real work started the real assessment run")
			}
		}
		if len(item.practice) != 3 {
			t.Fatal("not every required real judge completed distinct fictional practice")
		}
		if err := r.owner.call(ctx, http.MethodGet, base, -1, nil, &item.view, http.StatusOK); err != nil {
			t.Fatal(err)
		}
		if item.view.Stage != "preparation" {
			t.Fatal("practice bypassed the native scheduled live gate")
		}
		r.persistWorkProgress(t)
	}
}

func (r *lifecycleJourney) closeJudgePracticeAtLive(t *testing.T) {
	t.Helper()
	for index := range r.events {
		item := &r.events[index]
		for _, actor := range []string{"judge-a", "judge-b", "judge-c"} {
			judge := item.cohort.staff[actor]
			if judge == nil {
				t.Fatal("practice window check lost its actual selected judge")
			}
			if err := judge.call(t.Context(), http.MethodGet, "/api/v1/events/"+item.view.ID.String()+"/judge-practice", -1, nil, nil, http.StatusForbidden); err != nil {
				t.Fatal(err)
			}
			for i := range item.practice {
				if item.practice[i].JudgeID == judge.person {
					item.practice[i].ClosedAtLive = true
				}
			}
		}
	}
}
