package surveys

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/objectives"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestFrozenSurveyReportSuppressionReviewedProseCorrectionsAndExactMeasureVersion(t *testing.T) {
	f := setup(t)
	measureID, objectiveID := uuid.New(), uuid.New()
	definition := objectives.Definition{
		Description: "Learning objective",
		Measures: []objectives.Measure{
			{
				ID:          measureID,
				Description: "Self-reported learning",
				Method:      "Post-event survey",
				Unit:        "respondent",
			},
		},
	}
	plans := objectives.Service{Pool: f.pool, Clock: f.s.Now}
	objective, err := plans.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		objectiveID,
		0,
		objectives.Command{
			Action:     "save",
			Definition: &definition,
			Reason:     "Retain a precise questionnaire measurement definition",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	ref := objectives.MeasureReference{Objective: objective.Reference, MeasureID: measureID}
	f.definition.Measures = []QuestionMeasure{
		{QuestionID: f.choice, Measure: ref, Construct: "self_reported_learning"},
	}
	people := []uuid.UUID{f.person.ID, f.other.ID}
	for range 10 {
		p := testdb.Person(t, f.pool)
		f.exec(
			"INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')",
			f.event,
			p.ID,
		)
		people = append(people, p.ID)
	}
	cfg := f.publish()
	for i, person := range people {
		answers := forms.Answers{f.choice.String(): json.RawMessage(`"no"`)}
		if i < 6 {
			answers[f.choice.String()] = json.RawMessage(`"yes"`)
			answers[f.text.String()] = json.RawMessage(`"PRIVATE-FREE-TEXT-DO-NOT-RELEASE"`)
		}
		if _, err = f.s.Respond(
			t.Context(),
			person,
			f.event,
			f.id,
			0,
			ResponseCommand{CommandID: uuid.New(), FormVersion: cfg.Version, Answers: answers},
		); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = f.s.ChangeReport(
		t.Context(),
		f.owner.ID,
		f.event,
		f.id,
		0,
		ReportCommand{
			Action:       "review_text",
			ReviewID:     uuid.New(),
			ReviewedText: true,
			Summary:      "An early live report is forbidden",
		},
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("live cohort total was exposed", err)
	}
	if _, err = f.s.PublishedReport(
		t.Context(),
		f.person.ID,
		f.event,
		f.id,
	); !errors.Is(
		err,
		access.ErrNotFound,
	) {
		t.Fatal("unreviewed aggregate was published", err)
	}
	f.clock.Store(f.definition.ClosesAt.UnixNano())
	reviewID := uuid.New()
	command := ReportCommand{
		Action:       "review_text",
		ReviewID:     reviewID,
		ReviewedText: true,
		Summary:      "Organiser-reviewed themes: participants reported useful learning.",
	}
	review, err := f.s.ChangeReport(t.Context(), f.owner.ID, f.event, f.id, 0, command)
	if err != nil {
		t.Fatal(err)
	}
	if review.Report.Respondents == nil || *review.Report.Respondents != 12 ||
		review.Report.Questions[0].Suppressed ||
		!review.Report.Questions[1].Suppressed ||
		review.Report.Questions[0].Measure == nil ||
		review.Report.Questions[0].Measure.Measure != ref {
		t.Fatal("fixed suppressed aggregate or exact native measure reference incorrect")
	}
	if _, err = f.s.ChangeReport(
		t.Context(),
		f.owner.ID,
		f.event,
		f.id,
		review.Version,
		ReportCommand{Action: "publish", ReviewID: reviewID},
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("reviewed text skipped explicit approval", err)
	}
	for _, action := range []string{"approve", "publish"} {
		review, err = f.s.ChangeReport(
			t.Context(),
			f.owner.ID,
			f.event,
			f.id,
			review.Version,
			ReportCommand{Action: action, ReviewID: reviewID},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	first, err := f.s.PublishedReport(t.Context(), f.person.ID, f.event, f.id)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(first)
	if strings.Contains(string(raw), "PRIVATE-FREE-TEXT") ||
		strings.Contains(string(raw), f.person.ID.String()) ||
		strings.Contains(string(raw), "evidenced_adoption") {
		t.Fatal("authored response or false adoption evidence released")
	}
	// Native planning edits cannot silently retarget the retained question/measure.
	definition.Measures[0].Unit = "different future unit"
	if _, err = plans.Change(
		t.Context(),
		f.owner.ID,
		f.event,
		objectiveID,
		objective.CurrentVersion,
		objectives.Command{
			Action:     "save",
			Definition: &definition,
			Reason:     "Future planning correction",
		},
	); err != nil {
		t.Fatal(err)
	}
	// Subsequent membership changes cannot shrink or update the frozen totals.
	f.exec(
		"UPDATE competition_claims SET state='withdrawn' WHERE event_id=$1 AND person_id=$2",
		f.event,
		f.other.ID,
	)
	correction, err := f.s.ChangeReport(
		t.Context(),
		f.owner.ID,
		f.event,
		f.id,
		review.Version,
		ReportCommand{
			Action:       "review_text",
			ReviewID:     uuid.New(),
			ReviewedText: true,
			Summary:      "Corrected, explicitly reviewed summary; aggregate cohort is unchanged.",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	beforeCorrection, err := f.s.PublishedReport(t.Context(), f.person.ID, f.event, f.id)
	if err != nil || beforeCorrection.ReviewedSummary != first.ReviewedSummary {
		t.Fatal("draft prose changed wider release", err)
	}
	for _, action := range []string{"approve", "publish"} {
		correction, err = f.s.ChangeReport(
			t.Context(),
			f.owner.ID,
			f.event,
			f.id,
			correction.Version,
			ReportCommand{Action: action, ReviewID: correction.ReviewID},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	next, err := f.s.PublishedReport(t.Context(), f.person.ID, f.event, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if first.ReviewedTextVersion != review.Version ||
		next.ReviewedTextVersion != correction.Version ||
		first.PublishedAt == nil ||
		next.PublishedAt == nil {
		t.Fatal("released prose provenance omitted")
	}
	first.ReviewedTextVersion = 0
	next.ReviewedTextVersion = 0
	first.PublishedAt = nil
	next.PublishedAt = nil
	first.ReviewedSummary = ""
	next.ReviewedSummary = ""
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(next)
	if string(a) != string(b) {
		t.Fatal("prose correction recalculated a differencing cohort or retargeted measure")
	}
	f.clock.Store(f.definition.ClosesAt.AddDate(0, 0, 100).UnixNano())
	if err = f.s.Retain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = f.s.Retain(t.Context()); err != nil {
		t.Fatal(err)
	}
	var responses, invites, commands int
	if err = f.pool.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM survey_responses),(SELECT count(*) FROM survey_invitations),(SELECT count(*) FROM survey_commands)").
		Scan(&responses, &invites, &commands); err != nil ||
		responses != 0 ||
		invites != 0 ||
		commands != 0 {
		t.Fatal("retention kept identifiable survey records", responses, invites, commands, err)
	}
	retained, err := f.s.PublishedReport(t.Context(), f.person.ID, f.event, f.id)
	if err != nil || retained.Respondents == nil || *retained.Respondents != 12 {
		t.Fatal("erasure changed retained suppressed aggregate", err)
	}
	if _, err = f.s.Responses(
		t.Context(),
		f.owner.ID,
		f.event,
		f.id,
		uuid.Nil,
		20,
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("expired authored responses remained accessible", err)
	}
	f.exec("UPDATE events SET stage='archived',writes_allowed=false WHERE id=$1", f.event)
	if _, err = f.s.ChangeReport(
		t.Context(),
		f.owner.ID,
		f.event,
		f.id,
		correction.Version,
		ReportCommand{Action: "approve", ReviewID: correction.ReviewID},
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("report replay bypassed archive freeze", err)
	}
}

func TestSmallSurveyCannotPublishRawOrReviewedIdentifyingText(t *testing.T) {
	f := setup(t)
	f.publish()
	f.clock.Store(f.definition.ClosesAt.Add(time.Second).UnixNano())
	if _, err := f.s.ChangeReport(
		t.Context(),
		f.owner.ID,
		f.event,
		f.id,
		0,
		ReportCommand{
			Action:       "review_text",
			ReviewID:     uuid.New(),
			ReviewedText: true,
			Summary:      "This could identify the only respondent",
		},
	); !errors.Is(
		err,
		access.ErrInvalid,
	) {
		t.Fatal("small cohort promoted identifying summary", err)
	}
	review, err := f.s.ChangeReport(
		t.Context(),
		f.owner.ID,
		f.event,
		f.id,
		0,
		ReportCommand{Action: "review_text", ReviewID: uuid.New(), ReviewedText: true},
	)
	if err != nil || !review.Report.Suppressed || review.Report.Respondents != nil {
		t.Fatal("empty/unknown cohort became zero or exposed live count", err)
	}
}
