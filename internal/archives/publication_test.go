package archives

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
	"github.com/deploymenttheory/hack-in-a-box/internal/surveys"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

func TestArchivePublishedProjectExportUsesEventCursorAndCurrentWithdrawal(t *testing.T) {
	pool := testdb.New(t, true)
	owner, author, member := testdb.Person(t, pool), testdb.Person(t, pool), testdb.Person(t, pool)
	id, other := testdb.Event(t, pool, owner.ID), testdb.Event(t, pool, owner.ID)
	execute(
		t,
		pool,
		"INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')",
		id,
		member.ID,
	)
	now := time.Now().UTC().Truncate(time.Second)
	native := outcomes.Service{Pool: pool, Now: func() time.Time { return now }}
	var ownIDs []uuid.UUID
	for _, eventID := range []uuid.UUID{id, id, other} {
		project := uuid.New()
		proposal := outcomes.Command{
			ID:     uuid.New(),
			Action: "propose",
			Reason: "Synthetic private idea",
			Definition: &outcomes.Definition{
				Kind:          "pitch",
				Problem:       "PRIVATE-SOURCE-CANARY",
				Audience:      "Synthetic people",
				ProposedValue: "Unverified proposal",
			},
		}
		if _, err := native.Change(t.Context(), author.ID, project, 0, proposal); err != nil {
			t.Fatal(err)
		}
		if _, err := native.Change(
			t.Context(),
			owner.ID,
			project,
			1,
			outcomes.Command{
				ID:         uuid.New(),
				Action:     "adopt",
				Provenance: &outcomes.Provenance{EventID: &eventID},
				Reason:     "Explicit source",
			},
		); err != nil {
			t.Fatal(err)
		}
		fields := outcomes.PublicationFields{
			Problem:  "Reviewed published problem",
			Solution: "Reviewed solution",
			Audience: "organisation",
			Links:    []outcomes.ProjectLink{},
		}
		if _, err := native.ChangePublication(
			t.Context(),
			owner.ID,
			project,
			0,
			outcomes.PublicationCommand{
				ID:            uuid.New(),
				Action:        "save",
				SourceVersion: 2,
				Fields:        &fields,
				Reason:        "Review positive fields",
			},
		); err != nil {
			t.Fatal(err)
		}
		if _, err := native.ChangePublication(
			t.Context(),
			owner.ID,
			project,
			1,
			outcomes.PublicationCommand{
				ID:            uuid.New(),
				Action:        "publish",
				SourceVersion: 2,
				Reason:        "Explicit release",
			},
		); err != nil {
			t.Fatal(err)
		}
		if eventID == id {
			ownIDs = append(ownIDs, project)
		}
	}
	q := dbgen.New(pool)
	if err := q.InitializeEventFacts(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	row, err := q.Event(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	life := event.Service{Pool: pool}
	cancelled, err := life.Transition(
		t.Context(),
		owner.ID,
		id,
		row.Revision,
		"cancelled",
		"Synthetic archive",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = life.Transition(
		t.Context(),
		owner.ID,
		id,
		cancelled.Revision,
		"archived",
		"",
		false,
	); err != nil {
		t.Fatal(err)
	}
	service := Service{Pool: pool, Clock: func() time.Time { return now }}
	seen := map[uuid.UUID]bool{}
	cursor := ""
	for {
		result, err := service.Export(
			t.Context(),
			member.ID,
			id,
			ExportRequest{Source: "published-outcomes", Cursor: cursor, Limit: 1},
		)
		if err != nil {
			t.Fatal(err)
		}
		if result.Projects == nil || len(result.Projects.Items) != 1 {
			t.Fatal("missing native positive page", result)
		}
		raw, _ := json.Marshal(result)
		for _, private := range []string{"PRIVATE-SOURCE-CANARY", "owner_id", "author_id", "proposed_value", "evidence"} {
			if strings.Contains(string(raw), private) {
				t.Fatal("private source crossed publication export", private, string(raw))
			}
		}
		seen[result.Projects.Items[0].ID] = true
		if result.Projects.NextCursor == nil {
			break
		}
		cursor = result.Projects.NextCursor.String()
	}
	if len(seen) != 2 || !seen[ownIDs[0]] || !seen[ownIDs[1]] {
		t.Fatal("export cursor crossed event or omitted permitted projects", seen)
	}
	if _, err = native.ChangePublication(
		t.Context(),
		owner.ID,
		ownIDs[0],
		2,
		outcomes.PublicationCommand{
			ID:            uuid.New(),
			Action:        "withdraw",
			SourceVersion: 2,
			Reason:        "Withdraw current release",
		},
	); err != nil {
		t.Fatal(err)
	}
	result, err := service.Export(
		t.Context(),
		member.ID,
		id,
		ExportRequest{Source: "published-outcomes", Limit: 10},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Projects == nil || len(result.Projects.Items) != 1 ||
		result.Projects.Items[0].ID == ownIDs[0] {
		t.Fatal("withdrawn publication resurrected", result)
	}
}

func TestArchiveFeedbackExportRetainsOnlyPublishedSuppressedSnapshot(t *testing.T) {
	pool := testdb.New(t, true)
	owner, person, outsider := testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	), testdb.Person(
		t,
		pool,
	)
	id := testdb.Event(t, pool, owner.ID)
	surveyID, field := uuid.New(), uuid.New()
	now := time.Now().UTC().Truncate(time.Second)
	execute(
		t,
		pool,
		"INSERT INTO competition_claims(event_id,person_id,state)VALUES($1,$2,'confirmed')",
		id,
		person.ID,
	)
	dates := event.DefaultConfiguration()
	dates.LiveStart = now.Add(-12 * time.Hour)
	dates.LiveEnd = now.Add(-4 * time.Hour)
	dates.SubmissionCutoff = now.Add(-5 * time.Hour)
	dates.TeamFreeze = now.AddDate(0, 0, -8)
	dates.AdmissionCutoff = dates.TeamFreeze.Add(-time.Hour)
	raw, _ := json.Marshal(dates)
	execute(
		t,
		pool,
		"UPDATE events SET stage='completed',completed_at=$2,configuration=$3 WHERE id=$1",
		id,
		now.Add(-3*time.Hour),
		raw,
	)
	native := surveys.Service{Pool: pool, Now: func() time.Time { return now }}
	definition := surveys.Definition{
		Form: forms.Definition{
			ID:       surveyID,
			Name:     "Synthetic feedback",
			Audience: "attendee",
			Fields: []forms.Field{
				{ID: field, Label: "Private explanation", Type: "text", Required: true},
			},
		},
		OpensAt:       now.Add(-time.Hour),
		ClosesAt:      now.Add(time.Hour),
		MinimumCohort: 5,
		Measures:      []surveys.QuestionMeasure{},
	}
	config, err := native.Change(
		t.Context(),
		owner.ID,
		id,
		surveyID,
		0,
		surveys.Command{Action: "save", Definition: &definition},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"approve", "publish"} {
		config, err = native.Change(
			t.Context(),
			owner.ID,
			id,
			surveyID,
			config.Version,
			surveys.Command{Action: action},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = native.Respond(
		t.Context(),
		person.ID,
		id,
		surveyID,
		0,
		surveys.ResponseCommand{
			CommandID:   uuid.New(),
			FormVersion: config.Version,
			Answers: forms.Answers{
				field.String(): json.RawMessage(`"PRIVATE-RAW-RESPONSE-CANARY"`),
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	now = definition.ClosesAt
	reviewID := uuid.New()
	review, err := native.ChangeReport(
		t.Context(),
		owner.ID,
		id,
		surveyID,
		0,
		surveys.ReportCommand{
			Action:       "review_text",
			ReviewID:     reviewID,
			Summary:      "Reviewed broad themes",
			ReviewedText: true,
		},
	)
	if !errors.Is(err, access.ErrInvalid) {
		t.Fatal("suppressed cohort accepted identifying prose", err)
	}
	review, err = native.ChangeReport(
		t.Context(),
		owner.ID,
		id,
		surveyID,
		0,
		surveys.ReportCommand{Action: "review_text", ReviewID: reviewID, ReviewedText: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"approve", "publish"} {
		review, err = native.ChangeReport(
			t.Context(),
			owner.ID,
			id,
			surveyID,
			review.Version,
			surveys.ReportCommand{Action: action, ReviewID: reviewID},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	q := dbgen.New(pool)
	if err = q.InitializeEventFacts(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	row, err := q.Event(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = (event.Service{Pool: pool}).Transition(
		t.Context(),
		owner.ID,
		id,
		row.Revision,
		"archived",
		"Retain reviewed aggregate",
		false,
	); err != nil {
		t.Fatal(err)
	}
	service := Service{Pool: pool, Clock: func() time.Time { return now }, Surveys: native}
	result, err := service.Export(
		t.Context(),
		person.ID,
		id,
		ExportRequest{Source: "feedback", ResourceID: surveyID, Limit: 10},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Feedback == nil || !result.Feedback.Suppressed ||
		result.Feedback.Respondents != nil ||
		result.Feedback.ReviewedSummary != "" {
		t.Fatal("frozen suppression/prose changed", result)
	}
	raw, _ = json.Marshal(result)
	if strings.Contains(string(raw), "PRIVATE-RAW-RESPONSE-CANARY") ||
		strings.Contains(string(raw), person.ID.String()) {
		t.Fatal("authored response escaped suppressed export", string(raw))
	}
	if _, err = service.Export(
		t.Context(),
		outsider.ID,
		id,
		ExportRequest{Source: "feedback", ResourceID: surveyID, Limit: 10},
	); !errors.Is(
		err,
		access.ErrForbidden,
	) {
		t.Fatal("outsider gained feedback audience", err)
	}
}
