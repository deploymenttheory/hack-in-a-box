package app

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/teamreports"
)

// Exercise the actual amendment-release path which exceeded the native request
// deadline in the connected journey. Native confirmation, private authorship,
// audience and exact original acknowledgement remain part of the operation.
func TestTeamFeedbackAmendmentReleaseWithDatabaseRoundTripCost(t *testing.T) {
	f := newResultsSourceFixture(t, rubric.MentorQualification)
	r := f.runtime
	confirmResultsForTeamReport(t, f)
	request := qualificationHTTPRequest(t)
	path := fmt.Sprintf("/api/v1/events/%s/teams/%s/feedback-report", r.eventID, r.teamID)
	status, raw := request(f.admin, r.owner, "GET", path, nil, 0, true)
	if status != 200 {
		t.Fatalf("native preparation HTTP%d", status)
	}
	page := resultAPIDecode[contract.TeamFeedbackPage](t, "TeamFeedbackPage", raw)
	if page.CurrentSource == nil || page.CurrentSource.FinalScores == nil {
		t.Fatal("actual native confirmed score source missing")
	}
	body := contract.TeamFeedbackBody{
		Title:      "Native reviewed own-team report",
		Summary:    "Explicit organiser authorship",
		Highlights: []string{"Demonstrated workflow"},
		NextSteps:  []string{"Test the next hypothesis"},
	}
	setup := teamreports.Service{Pool: r.pool, Now: r.clock.read, Results: &f.service}
	include := true
	for i, action := range []string{"save", "release", "save"} {
		command := contract.TeamFeedbackCommand{
			RequestId:     uuid.New(),
			Action:        contract.TeamFeedbackCommandAction(action),
			SourceBinding: page.CurrentSource.Binding,
			Reason:        "Synthetic deliberate report version",
		}
		if action == "save" {
			command.Body = &body
			if i == 0 {
				command.IncludeFinalScores = &include
				command.ScoreBinding = &page.CurrentSource.FinalScores.Binding
			}
		}
		encoded, err := json.Marshal(command)
		if err != nil {
			t.Fatal(err)
		}
		var native teamreports.Command
		if err = json.Unmarshal(encoded, &native); err != nil {
			t.Fatal(err)
		}
		accepted, err := setup.Change(
			t.Context(),
			r.owner.ID,
			r.eventID,
			r.teamID,
			int64(i),
			native,
		)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err = json.Marshal(accepted)
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(encoded, &page); err != nil {
			t.Fatal(err)
		}
	}
	if page.Version != 3 || page.Latest == nil || page.Latest.FinalScores != nil ||
		!page.CurrentSource.ScoresAvailable {
		t.Fatal("amendment must explicitly withhold still-available confirmed scores")
	}
	tracer := &scoringQueryDelay{
		delay:  4 * time.Millisecond,
		names:  map[string]int{},
		locked: make(chan struct{}),
	}
	cfg := r.pool.Config()
	cfg.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	original := r.pool
	r.pool = pool
	surface := r.surface(t, "admin")
	r.pool = original
	command := contract.TeamFeedbackCommand{
		RequestId:     uuid.New(),
		Action:        "release",
		SourceBinding: page.CurrentSource.Binding,
		Reason:        "Deliberate amended release under the existing API budget",
	}
	worker := observeScoringMaintenance(t, r, tracer)
	tracer.enabled.Store(true)
	began := time.Now()
	status, raw = request(surface, r.owner, "POST", path, command, 3, true)
	elapsed := time.Since(began)
	tracer.enabled.Store(false)
	observed := <-worker
	t.Logf(
		"native feedback HTTP%d elapsed=%s queries=%d worker_blocked_by_api=%t worker_elapsed=%s",
		status,
		elapsed,
		tracer.calls.Load(),
		observed.blocked,
		observed.elapsed,
	)
	if observed.err != nil || !observed.blocked {
		t.Fatalf("native maintenance contention not verified: %v", observed.err)
	}
	if status != 200 {
		t.Fatalf(
			"native amended feedback release exceeded unchanged request deadline: HTTP%d",
			status,
		)
	}
	accepted := resultAPIDecode[contract.TeamFeedbackPage](t, "TeamFeedbackPage", raw)
	if accepted.Receipt == nil || accepted.Receipt.RequestId != command.RequestId ||
		accepted.Version != 4 ||
		accepted.Latest == nil ||
		accepted.Latest.State != "released" ||
		accepted.Latest.FinalScores != nil ||
		accepted.CurrentSource == nil ||
		!accepted.CurrentSource.ScoresAvailable ||
		!reflect.DeepEqual(accepted.Latest.Body, page.Latest.Body) {
		t.Fatal("bounded release lost exact authoring/score choice/acknowledgement")
	}
	status, raw = request(
		surface,
		r.owner,
		"GET",
		path+"/requests/"+command.RequestId.String(),
		nil,
		0,
		true,
	)
	if status != 200 {
		t.Fatal("original acknowledgement unavailable")
	}
	var recovery struct {
		Receipt *contract.TeamFeedbackReceipt `json:"receipt"`
	}
	if err := json.Unmarshal(raw, &recovery); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(recovery.Receipt, accepted.Receipt) {
		t.Fatal("current read substituted original acknowledgement")
	}
}
