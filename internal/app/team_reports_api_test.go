package app

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

func TestTeamReportAPIAdminPreparationExplicitAmendedReleaseAndExactRecovery(t *testing.T) {
	f := newResultsSourceFixture(t, rubric.AllTeams)
	r := f.runtime
	confirmResultsForTeamReport(t, f)
	completeReportEvent(t, f)
	request := qualificationHTTPRequest(t)
	path := fmt.Sprintf("/api/v1/events/%s/teams/%s/feedback-report", r.eventID, r.teamID)
	status, raw := request(f.admin, r.owner, "GET", path, nil, 0, true)
	if status != 200 {
		t.Fatal("native report preparation", status, string(raw))
	}
	p := resultAPIDecode[contract.TeamFeedbackPage](t, "TeamFeedbackPage", raw)
	if p.CurrentSource == nil || !p.CurrentSource.ScoresAvailable || p.Latest != nil ||
		p.Receipt != nil {
		t.Fatal("current native report source", p)
	}
	body := contract.TeamFeedbackBody{
		Title:      "Reviewed team feedback",
		Summary:    "Explicit organiser prose, never raw assessment notes",
		Highlights: []string{"A clear workflow"},
		NextSteps:  []string{"Validate the next hypothesis"},
	}
	cmd := contract.TeamFeedbackCommand{
		RequestId:     uuid.New(),
		Action:        "save",
		SourceBinding: p.CurrentSource.Binding,
		Body:          &body,
		Reason:        "Reviewed report draft",
	}
	for _, method := range []string{"GET", "POST"} {
		var value any
		if method == "POST" {
			value = cmd
		}
		status, raw = request(f.public, r.owner, method, path, value, 0, true)
		if status != 403 {
			t.Fatal("regular portal exposed private preparation", method, status, string(raw))
		}
	}
	status, _ = request(f.admin, f.judges[0], "GET", path, nil, 0, true)
	if status != 403 {
		t.Fatal("judge saw organiser preparation", status)
	}
	match := "\"0\""
	for _, probe := range []struct {
		name     string
		value    any
		revision *string
		csrf     string
		want     int
	}{
		{"missing_if_match", cmd, nil, strings.Repeat("c", 32), 400},
		{"missing_csrf", cmd, &match, "", 400},
		{"wrong_csrf", cmd, &match, strings.Repeat("z", 32), 403},
		{"arbitrary_private_fields", map[string]any{"request_id": uuid.New(), "action": "save", "source_binding": cmd.SourceBinding, "reason": "No private source import", "raw_notes": "PRIVATE"}, &match, strings.Repeat("c", 32), 400},
	} {
		status, raw = resultAPIPost(
			t,
			f.admin,
			r.owner,
			path,
			probe.value,
			probe.revision,
			probe.csrf,
		)
		if status != probe.want {
			t.Fatal(probe.name, status, string(raw))
		}
	}
	status, raw = request(f.admin, r.owner, "POST", path, cmd, 0, true)
	if status != 200 {
		t.Fatal("save report draft", status, string(raw))
	}
	p = resultAPIDecode[contract.TeamFeedbackPage](t, "TeamFeedbackPage", raw)
	original := p.Receipt
	status, raw = request(f.public, r.competitor, "GET", path+"/released", nil, 0, true)
	if status != 200 ||
		resultAPIDecode[contract.ReleasedTeamFeedback](
			t,
			"ReleasedTeamFeedback",
			raw,
		).Report != nil {
		t.Fatal("draft leaked", status, string(raw))
	}
	release := contract.TeamFeedbackCommand{
		RequestId:     uuid.New(),
		Action:        "release",
		SourceBinding: cmd.SourceBinding,
		Reason:        "Deliberate prose-only own-team release",
	}
	status, raw = request(f.admin, r.owner, "POST", path, release, 1, true)
	if status != 200 {
		t.Fatal("release", status, string(raw))
	}
	p = resultAPIDecode[contract.TeamFeedbackPage](t, "TeamFeedbackPage", raw)
	status, raw = request(f.public, r.competitor, "GET", path+"/released", nil, 0, true)
	if status != 200 {
		t.Fatal("own released prose", status, string(raw))
	}
	own := resultAPIDecode[contract.ReleasedTeamFeedback](t, "ReleasedTeamFeedback", raw)
	if own.Report == nil || own.Report.FinalScores != nil || own.Report.Version != 2 {
		t.Fatal("scores implicitly released", own)
	}
	include := true
	cmd.RequestId = uuid.New()
	cmd.IncludeFinalScores = &include
	scoreBinding := p.CurrentSource.FinalScores.Binding
	cmd.ScoreBinding = &scoreBinding
	body.Summary = "Explicitly amended own-team feedback"
	cmd.Body = &body
	status, raw = request(f.admin, r.owner, "POST", path, cmd, 2, true)
	if status != 200 {
		t.Fatal("amend draft", status, string(raw))
	}
	status, raw = request(f.public, r.competitor, "GET", path+"/released", nil, 0, true)
	own = resultAPIDecode[contract.ReleasedTeamFeedback](t, "ReleasedTeamFeedback", raw)
	if status != 200 || own.Report.Version != 2 || own.Report.FinalScores != nil {
		t.Fatal("correction implicitly replaced published report", status, string(raw))
	}
	release.RequestId = uuid.New()
	status, raw = request(f.admin, r.owner, "POST", path, release, 3, true)
	if status != 200 {
		t.Fatal("explicit amended release", status, string(raw))
	}
	status, raw = request(f.public, r.competitor, "GET", path+"/released", nil, 0, true)
	own = resultAPIDecode[contract.ReleasedTeamFeedback](t, "ReleasedTeamFeedback", raw)
	if status != 200 || own.Report.Version != 4 || own.Report.FinalScores == nil ||
		own.Report.FinalScores.Total == nil ||
		own.Report.FinalScores.Total.Numerator != "42" {
		t.Fatal("amended own score report", status, string(raw))
	}
	for _, private := range []string{"rank", "awards", "audience", resultsMentorPrivateNote, "PRIVATE"} {
		if strings.Contains(string(raw), private) {
			t.Fatal("recipient projection leaked", private, string(raw))
		}
	}
	status, _ = request(f.public, f.judges[0], "GET", path+"/released", nil, 0, true)
	if status != 403 {
		t.Fatal("judge accessed participant report", status)
	}
	// Exact original receipt survives newer versions and is recoverable on either portal.
	receiptPath := path + "/requests/" + original.RequestId.String()
	status, raw = request(f.public, r.owner, "GET", receiptPath, nil, 0, true)
	recovered := resultAPIDecode[contract.TeamFeedbackRecovery](t, "TeamFeedbackRecovery", raw)
	if status != 200 || !reflect.DeepEqual(original, recovered.Receipt) {
		t.Fatal("original receipt lost", status, string(raw))
	}
	status, _ = request(f.admin, f.judges[0], "GET", receiptPath, nil, 0, true)
	if status != 403 {
		t.Fatal("other actor recovered organiser receipt", status)
	}
	status, raw = request(f.admin, r.owner, "GET", path+"/history", nil, 0, true)
	history := resultAPIDecode[contract.TeamFeedbackHistory](t, "TeamFeedbackHistory", raw)
	if status != 200 || len(history.Items) != 4 || history.Items[0].Version != 4 ||
		history.Items[3].Version != 1 {
		t.Fatal("retained amended history", status, string(raw))
	}
	status, raw = request(f.admin, r.owner, "GET", path, nil, 0, true)
	p = resultAPIDecode[contract.TeamFeedbackPage](t, "TeamFeedbackPage", raw)
	if status != 200 || p.Receipt != nil {
		t.Fatal("current GET manufactured receipt", status, string(raw))
	}
	status, raw = request(
		f.public,
		r.competitor,
		"GET",
		fmt.Sprintf("/api/v1/events/%s/results/announcements", r.eventID),
		nil,
		0,
		true,
	)
	if status != 200 ||
		len(
			resultAPIDecode[contract.ResultAnnouncements](t, "ResultAnnouncements", raw).Items,
		) != 0 {
		t.Fatal("report release implicitly announced results", status, string(raw))
	}
}
