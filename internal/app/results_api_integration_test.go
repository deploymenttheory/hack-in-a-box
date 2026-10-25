package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/results"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

func resultAPIDecode[T any](t *testing.T, name string, raw []byte) T {
	t.Helper()
	var value T
	resultsSourceRequire(t, json.Unmarshal(raw, &value))
	var document any
	resultsSourceRequire(t, json.Unmarshal(raw, &document))
	spec, err := contract.GetSpec()
	resultsSourceRequire(t, err)
	resultsSourceRequire(t, spec.Components.Schemas[name].Value.VisitJSON(document))
	return value
}

func resultAPIPost(
	t *testing.T,
	surface rehearsalSurface,
	person dbgen.Person,
	path string,
	body any,
	match *string,
	csrf string,
) (int, []byte) {
	t.Helper()
	raw, err := json.Marshal(body)
	resultsSourceRequire(t, err)
	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		surface.server.URL+path,
		bytes.NewReader(raw),
	)
	resultsSourceRequire(t, err)
	request.AddCookie(surface.cookie(t, person))
	request.Header.Set("Origin", surface.server.URL)
	request.Header.Set("Content-Type", "application/json")
	if match != nil {
		request.Header.Set("If-Match", *match)
	}
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
	}
	response, err := http.DefaultClient.Do(request)
	resultsSourceRequire(t, err)
	defer func() { _ = response.Body.Close() }()
	raw, err = io.ReadAll(response.Body)
	resultsSourceRequire(t, err)
	if !strings.Contains(response.Header.Get("Cache-Control"), "no-store") {
		t.Fatal("results response was cacheable")
	}
	return response.StatusCode, raw
}

func TestResultsAPIPrivateAdministratorScopeAbsentSourcesAndStrictCommands(t *testing.T) {
	r := newLiveRehearsal(t)
	admin, public := r.surface(t, "admin"), r.surface(t, "public")
	event, err := admin.deps.Events.Read(t.Context(), r.owner.ID, r.eventID)
	resultsSourceRequire(t, err)
	r.clock.set(event.Configuration.SubmissionCutoff)
	request := qualificationHTTPRequest(t)
	path := fmt.Sprintf("/api/v1/events/%s/results", r.eventID)
	status, raw := request(admin, r.owner, "GET", path, nil, 0, true)
	if status != 200 {
		t.Fatal("actual absent-source read", status, string(raw))
	}
	p := resultAPIDecode[contract.ResultPage](t, "ResultPage", raw)
	if p.Preview.Criteria != nil || p.Preview.Reference != nil || p.Preview.JudgeRunId != nil ||
		p.Current ||
		len(p.Preview.Blockers) == 0 ||
		p.Round.Source.Criteria != nil ||
		p.Round.Source.Reference != nil {
		t.Fatal("absent native source became fake published criteria", p)
	}
	command := contract.ResultCommand{
		RequestId:     uuid.New(),
		Action:        "prepare",
		Version:       0,
		SourceBinding: strings.Repeat("a", 64),
		Reason:        "Actual captured results review",
	}
	for _, method := range []string{"GET", "POST"} {
		var body any
		if method == "POST" {
			body = command
		}
		status, raw = request(public, r.owner, method, path, body, p.Revision, true)
		if status != 403 {
			t.Fatal("organiser bypassed private administrator surface", method, status, string(raw))
		}
	}
	status, _ = request(admin, r.mentor, "GET", path, nil, 0, true)
	if status != 403 {
		t.Fatal("admin surface granted mentor private result preparation", status)
	}
	for _, c := range []struct {
		name  string
		body  any
		match *string
		csrf  string
		want  int
	}{
		{"missing_revision", command, nil, strings.Repeat("c", 32), 400},
		{"malformed_revision", command, func() *string { v := "1"; return &v }(), strings.Repeat("c", 32), 400},
		{"missing_csrf", command, func() *string { v := fmt.Sprintf("\"%d\"", p.Revision); return &v }(), "", 400},
		{"unknown_property", map[string]any{"request_id": uuid.New(), "action": "prepare", "version": 0, "source_binding": strings.Repeat("a", 64), "reason": "Captured review", "raw_cards": []int{10}}, func() *string { v := fmt.Sprintf("\"%d\"", p.Revision); return &v }(), strings.Repeat("c", 32), 400},
		{"judge_action_on_admin_command", map[string]any{"request_id": uuid.New(), "action": "vote", "version": 0, "source_binding": strings.Repeat("a", 64), "reason": "Captured review"}, func() *string { v := fmt.Sprintf("\"%d\"", p.Revision); return &v }(), strings.Repeat("c", 32), 400},
		{"wrong_csrf", command, func() *string { v := fmt.Sprintf("\"%d\"", p.Revision); return &v }(), strings.Repeat("z", 32), 403},
	} {
		t.Run(c.name, func(t *testing.T) {
			status, raw := resultAPIPost(t, admin, r.owner, path, c.body, c.match, c.csrf)
			if status != c.want {
				t.Fatal("strict mutation boundary", status, string(raw))
			}
		})
	}
	var versions int
	resultsSourceRequire(
		t,
		r.pool.QueryRow(t.Context(), "SELECT count(*) FROM result_versions WHERE event_id=$1", r.eventID).
			Scan(&versions),
	)
	if versions != 0 {
		t.Fatal("rejected probes mutated result history", versions)
	}
}

func TestResultsAPIExplicitQualificationReleaseOriginalACKAndSafeAudience(t *testing.T) {
	prizeID := uuid.New()
	f := newResultsSourceFixtureWithSetup(
		t,
		rubric.MentorQualification,
		func(d *rubric.Definition) {
			d.Prizes = []rubric.Prize{
				{
					ID:          prizeID,
					Name:        "Explicit configured top award",
					Description: "Native prize metadata only",
				},
			}
		},
		nil,
	)
	r := f.runtime
	request := qualificationHTTPRequest(t)
	path := fmt.Sprintf("/api/v1/events/%s/results", r.eventID)
	if !f.admin.deps.ResultsAdminSurface || f.public.deps.ResultsAdminSurface {
		t.Fatal("runtime surface flags disagree with independent portals")
	}
	roomOwner, ok := f.admin.deps.Presentation.Results.(*results.Service)
	if !ok || roomOwner.Pool != r.pool ||
		roomOwner.Judging.Finalists != f.admin.deps.Judging.Finalists ||
		roomOwner.Qualification != roomOwner.Judging.Finalists ||
		!roomOwner.Now().Equal(r.clock.read()) {
		t.Fatal("room/API results graph lost real qualifier or shared clock")
	}
	status, raw := request(f.admin, r.owner, "GET", path, nil, 0, true)
	if status != 200 {
		t.Fatal(status, string(raw))
	}
	p := resultAPIDecode[contract.ResultPage](t, "ResultPage", raw)
	if p.Preview.Selection == nil || p.Preview.FinalistsBinding == "" ||
		p.Preview.Criteria == nil ||
		p.Preview.Reference == nil ||
		p.Receipt != nil {
		t.Fatal("real private selection missing or GET invented an ACK", p)
	}
	originalRevision := p.Revision
	prepare := contract.ResultCommand{
		RequestId:     uuid.New(),
		Action:        "prepare",
		Version:       p.Round.Version,
		SourceBinding: p.Preview.FinalistsBinding,
		Reason:        "Explicit exact native qualification preparation",
	}
	status, raw = request(f.admin, r.owner, "POST", path, prepare, originalRevision, true)
	if status != 200 {
		t.Fatal("native API prepare", status, string(raw))
	}
	prepared := resultAPIDecode[contract.ResultPage](t, "ResultPage", raw)
	if prepared.Receipt == nil || prepared.Receipt.RequestId != prepare.RequestId ||
		prepared.Receipt.ExpectedRevision != originalRevision ||
		prepared.Receipt.ExpectedVersion != prepare.Version {
		t.Fatal("prepare acknowledgement lost exact reviewed intent", prepared)
	}
	status, raw = request(f.admin, r.owner, "POST", path, prepare, originalRevision, true)
	if status != 200 {
		t.Fatal("exact original replay", status, string(raw))
	}
	repeated := resultAPIDecode[contract.ResultPage](t, "ResultPage", raw)
	if !reflect.DeepEqual(prepared.Receipt, repeated.Receipt) {
		t.Fatal("replay adopted later current metadata")
	}
	receiptPath := path + "/requests/" + prepare.RequestId.String()
	status, raw = request(f.public, r.owner, "GET", receiptPath, nil, 0, true)
	if status != 200 {
		t.Fatal("safe own original receipt on other portal", status, string(raw))
	}
	recovery := resultAPIDecode[contract.ResultRecovery](t, "ResultRecovery", raw)
	if !reflect.DeepEqual(prepared.Receipt, recovery.Receipt) {
		t.Fatal("current read replaced original receipt")
	}
	status, raw = request(f.public, f.judges[0], "GET", receiptPath, nil, 0, true)
	if status != 403 {
		t.Fatal("another actor acquired original organiser acknowledgement", status, string(raw))
	}

	releasedPath := path + "/announcements"
	status, raw = request(f.public, r.competitor, "GET", releasedPath, nil, 0, true)
	if status != 200 {
		t.Fatal(status, string(raw))
	}
	if len(
		resultAPIDecode[contract.ResultAnnouncements](t, "ResultAnnouncements", raw).Items,
	) != 0 {
		t.Fatal("preparation auto-published finalists")
	}
	status, raw = request(f.admin, r.owner, "GET", path, nil, 0, true)
	if status != 200 {
		t.Fatal(status, string(raw))
	}
	p = resultAPIDecode[contract.ResultPage](t, "ResultPage", raw)
	if p.Receipt != nil {
		t.Fatal("fresh GET claimed original prepare success")
	}
	publish := contract.ResultCommand{
		RequestId:     uuid.New(),
		Action:        "publish_finalists",
		Version:       p.Round.Version,
		SourceBinding: p.Preview.FinalistsBinding,
		Reason:        "Explicit separate finalist publication",
	}
	status, raw = request(f.admin, r.owner, "POST", path, publish, p.Revision, true)
	if status != 200 {
		t.Fatal("actual finalist publication", status, string(raw))
	}
	announced := resultAPIDecode[contract.ResultPage](t, "ResultPage", raw)
	if announced.Receipt == nil || announced.Receipt.PublicationId == nil {
		t.Fatal("publication lacked exact native ACK", announced)
	}
	status, raw = request(f.public, r.competitor, "GET", releasedPath, nil, 0, true)
	if status != 200 {
		t.Fatal(status, string(raw))
	}
	safe := resultAPIDecode[contract.ResultAnnouncements](t, "ResultAnnouncements", raw)
	if len(safe.Items) != 1 || safe.Items[0].Kind != "finalists" ||
		len(safe.Items[0].Finalists) != 1 ||
		safe.Items[0].Finalists[0].Id != r.teamID ||
		len(safe.Items[0].Awards) != 0 ||
		strings.Contains(string(raw), resultsMentorPrivateNote) ||
		strings.Contains(string(raw), "source_binding") {
		t.Fatal("released audience gained private or embargoed data", string(raw))
	}
	status, _ = request(f.public, r.competitor, "GET", path, nil, 0, true)
	if status != 403 {
		t.Fatal("released audience gained private preparation", status)
	}
	// Fresh judge scores remain independent of the mentor qualification, and
	// confirmation itself must not lift the separate category-award embargo.
	f.complete(t)
	status, raw = request(f.admin, r.owner, "GET", path, nil, 0, true)
	if status != 200 {
		t.Fatal(status, string(raw))
	}
	p = resultAPIDecode[contract.ResultPage](t, "ResultPage", raw)
	if len(p.Preview.Tallies) != 1 || p.Preview.Tallies[0].Total.Numerator != "42" ||
		p.Preview.Tallies[0].Total.Denominator != "1" {
		t.Fatal("HTTP projection lost exact string rational source", p)
	}
	fullPrepare := contract.ResultCommand{
		RequestId:     uuid.New(),
		Version:       p.Round.Version,
		Action:        "prepare",
		SourceBinding: p.Preview.Binding,
		Reason:        "Fresh exact independent judge source",
	}
	status, raw = request(f.admin, r.owner, "POST", path, fullPrepare, p.Revision, true)
	if status != 200 {
		t.Fatal(status, string(raw))
	}
	p = resultAPIDecode[contract.ResultPage](t, "ResultPage", raw)
	awards := []contract.ResultAward{
		{
			PrizeId:  prizeID,
			Name:     "Explicit configured top award",
			TeamId:   r.teamID,
			Amount:   "1250.50",
			Currency: "GBP",
		},
	}
	confirm := contract.ResultCommand{
		RequestId:     uuid.New(),
		Version:       p.Round.Version,
		Action:        "confirm",
		SourceBinding: p.Preview.Binding,
		Awards:        &awards,
		Reason:        "Explicit exact winner and metadata confirmation",
	}
	status, raw = request(f.admin, r.owner, "POST", path, confirm, p.Revision, true)
	if status != 200 {
		t.Fatal("native typed confirmation", status, string(raw))
	}
	p = resultAPIDecode[contract.ResultPage](t, "ResultPage", raw)
	if p.Round.Confirmation == nil || len(p.Round.Confirmation.Awards) != 1 ||
		p.Round.Confirmation.Awards[0].Amount != "1250.50" {
		t.Fatal("confirmation converted decimal string metadata", p)
	}
	status, raw = request(f.public, r.competitor, "GET", releasedPath, nil, 0, true)
	if status != 200 {
		t.Fatal(status, string(raw))
	}
	if len(
		resultAPIDecode[contract.ResultAnnouncements](t, "ResultAnnouncements", raw).Items,
	) != 1 ||
		strings.Contains(string(raw), "1250.50") {
		t.Fatal("confirmation implicitly announced awards", string(raw))
	}
	publishAwards := contract.ResultCommand{
		RequestId:     uuid.New(),
		Version:       p.Round.Version,
		Action:        "publish_awards",
		SourceBinding: p.Preview.Binding,
		Reason:        "Explicit separate safe award announcement",
	}
	status, raw = request(f.admin, r.owner, "POST", path, publishAwards, p.Revision, true)
	if status != 200 {
		t.Fatal("native typed award publication", status, string(raw))
	}
	resultAPIDecode[contract.ResultPage](t, "ResultPage", raw)
	status, raw = request(f.public, r.competitor, "GET", releasedPath, nil, 0, true)
	if status != 200 {
		t.Fatal(status, string(raw))
	}
	awarded := resultAPIDecode[contract.ResultAnnouncements](t, "ResultAnnouncements", raw)
	var awardSnapshot *contract.ResultAnnouncement
	for i := range awarded.Items {
		if awarded.Items[i].Kind == "awards" {
			if awardSnapshot != nil {
				t.Fatal("duplicate award announcement", awarded)
			}
			awardSnapshot = &awarded.Items[i]
		}
	}
	if len(awarded.Items) != 2 || awardSnapshot == nil || len(awardSnapshot.Awards) != 1 ||
		awardSnapshot.Awards[0].Amount != "1250.50" ||
		awardSnapshot.Awards[0].Currency != "GBP" ||
		awardSnapshot.Awards[0].Team.Id != r.teamID {
		t.Fatal("explicit safe award metadata missing", awarded)
	}
}

func TestResultsAPIBallotsBothPortalsPreserveOwnReceiptsAndBlockPrematureConfirmation(
	t *testing.T,
) {
	f, second := newTiedResultsFixture(t)
	r := f.runtime
	request := qualificationHTTPRequest(t)
	path := fmt.Sprintf("/api/v1/events/%s/results", r.eventID)
	status, raw := request(f.admin, r.owner, "GET", path, nil, 0, true)
	if status != 200 {
		t.Fatal(status, string(raw))
	}
	p := resultAPIDecode[contract.ResultPage](t, "ResultPage", raw)
	prepare := contract.ResultCommand{
		RequestId:     uuid.New(),
		Action:        "prepare",
		Version:       p.Round.Version,
		SourceBinding: p.Preview.Binding,
		Reason:        "Explicit current tied native source",
	}
	status, raw = request(f.admin, r.owner, "POST", path, prepare, p.Revision, true)
	if status != 200 {
		t.Fatal(status, string(raw))
	}
	prepared := resultAPIDecode[contract.ResultPage](t, "ResultPage", raw)
	assertConfirmationBlocked := func() {
		t.Helper()
		status, raw := request(f.admin, r.owner, "GET", path, nil, 0, true)
		if status != 200 {
			t.Fatal(status, string(raw))
		}
		p := resultAPIDecode[contract.ResultPage](t, "ResultPage", raw)
		command := contract.ResultCommand{
			RequestId:     uuid.New(),
			Action:        "confirm",
			Version:       p.Round.Version,
			SourceBinding: p.Preview.Binding,
			Reason:        "Attempt before every required native judge decision",
		}
		status, raw = request(f.admin, r.owner, "POST", path, command, p.Revision, true)
		if status != 409 {
			t.Fatal(
				"incomplete native ballots allowed confirmation or wrong failure",
				status,
				string(raw),
			)
		}
	}
	assertConfirmationBlocked()
	ballotPath := path + "/ballots"
	status, raw = request(f.public, f.judges[0], "GET", ballotPath, nil, 0, true)
	if status != 200 {
		t.Fatal(status, string(raw))
	}
	ballots := resultAPIDecode[contract.ResultBallotPage](t, "ResultBallotPage", raw)
	if len(ballots.Ties) < 1 || ballots.Version != prepared.Round.Version {
		t.Fatal("actual assigned native ties absent", ballots)
	}
	tie := ballots.Ties[0].State.Tie
	vote := contract.ResultVoteCommand{
		RequestId:     uuid.New(),
		Version:       ballots.Version,
		Action:        "vote",
		SourceBinding: ballots.SourceBinding,
		TieId:         tie.Id,
		TeamId:        r.teamID,
		Reason:        "Independent reviewed native tie ballot",
		Evidence: []contract.ResultVoteEvidence{
			{
				TeamId:        r.teamID,
				DeliverableId: &f.deliverable,
				Note:          "OWN-FIRST-JUDGE-PRIVATE-BALLOT",
			},
			{
				TeamId:        second,
				DeliverableId: &f.deliverable,
				Note:          "Reviewed exact competing frozen output",
			},
		},
	}
	status, raw = request(f.public, f.judges[0], "POST", ballotPath, vote, ballots.Revision, true)
	if status != 200 {
		t.Fatal("actual regular judge ballot", status, string(raw))
	}
	accepted := resultAPIDecode[contract.ResultBallotPage](t, "ResultBallotPage", raw)
	if accepted.Receipt == nil || accepted.Receipt.RequestId != vote.RequestId ||
		accepted.Receipt.ActorId != f.judges[0].ID {
		t.Fatal("vote ACK lost exact actor/command", accepted)
	}
	status, raw = request(f.admin, f.judges[1], "GET", ballotPath, nil, 0, true)
	if status != 200 {
		t.Fatal(status, string(raw))
	}
	other := resultAPIDecode[contract.ResultBallotPage](t, "ResultBallotPage", raw)
	if strings.Contains(string(raw), "OWN-FIRST-JUDGE-PRIVATE-BALLOT") {
		t.Fatal("other judge read private ballot evidence")
	}
	secondVote := vote
	secondVote.RequestId = uuid.New()
	secondVote.Version = other.Version
	secondVote.TeamId = second
	secondVote.Reason = "Explicit independent split vote"
	status, raw = request(
		f.admin,
		f.judges[1],
		"POST",
		ballotPath,
		secondVote,
		other.Revision,
		true,
	)
	if status != 200 {
		t.Fatal("actual admin judge ballot", status, string(raw))
	}
	resultAPIDecode[contract.ResultBallotPage](t, "ResultBallotPage", raw)
	assertConfirmationBlocked()
	receiptPath := path + "/requests/" + vote.RequestId.String()
	status, raw = request(f.public, f.judges[0], "GET", receiptPath, nil, 0, true)
	if status != 200 {
		t.Fatal(status, string(raw))
	}
	own := resultAPIDecode[contract.ResultRecovery](t, "ResultRecovery", raw)
	if !reflect.DeepEqual(own.Receipt, accepted.Receipt) {
		t.Fatal("original ballot ACK retargeted to another vote/current revision")
	}
	status, raw = request(f.admin, f.judges[1], "GET", receiptPath, nil, 0, true)
	if status != 403 {
		t.Fatal("other judge acquired an original ACK", status, string(raw))
	}
}
