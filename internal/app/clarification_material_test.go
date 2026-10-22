package app

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

func stageClarificationOwningChange(
	t *testing.T,
	service content.Service,
	actor, eventID, challengeID uuid.UUID,
	ref rubric.Reference,
) content.Page {
	t.Helper()
	current, err := service.Read(t.Context(), actor, eventID, challengeID, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	current, err = service.Save(
		t.Context(),
		actor,
		eventID,
		challengeID,
		current.Revision,
		content.Input{Kind: "challenge", Document: content.Document{
			Title:    "Reviewed common challenge correction",
			Markdown: "Updated common requirements apply through this actual owning release.",
			Audience: access.ContentAudience{Confirmed: true},
			Rubric:   &ref,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"review", "approve"} {
		current, err = service.Action(
			t.Context(),
			actor,
			eventID,
			challengeID,
			current.Revision,
			action,
			"Synthetic explicit owning change review",
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	return current
}

func verifyMaterialClarificationWorkflow(
	t *testing.T,
	runtime communicationRuntime,
	owningContent content.Service,
	owner, author, eventID, questionID, answerID uuid.UUID,
	clock func() time.Time,
) clarification.AnswerConfiguration {
	t.Helper()
	ctx := t.Context()
	current, err := runtime.Clarifications.AnswerConfiguration(ctx, owner, eventID, questionID)
	if err != nil {
		t.Fatal(err)
	}
	original, err := runtime.Clarifications.Question(ctx, owner, eventID, questionID)
	if err != nil {
		t.Fatal(err)
	}
	criteria := rubric.Service{Pool: runtime.Clarifications.Pool, Clock: clock}
	config, err := criteria.Configuration(ctx, owner, eventID, 0)
	if err != nil {
		t.Fatal(err)
	}
	change := func(command rubric.Command) {
		t.Helper()
		command.Reason = "Synthetic explicit criteria-change decision"
		config, err = criteria.Change(ctx, owner, eventID, config.Revision, command)
		if err != nil {
			t.Fatal(err)
		}
	}
	definition := config.Definition
	definition.Categories[0].Weight++
	change(rubric.Command{Action: "save", Definition: &definition})
	change(rubric.Command{Action: "approve", Version: config.Reference.Version})
	material := clarification.AnswerDocument{
		Title: "Reviewed material criteria notice",
		Body:  "SYNTHETIC-MATERIAL-CRITERIA-NOTICE",
		Kind:  "rubric_change",
	}
	if _, err = runtime.Clarifications.SaveAnswer(
		ctx,
		owner,
		eventID,
		questionID,
		current.Revision,
		material,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("approved but unreleased rubric draft bypassed the owning publication", err)
	}
	staged := stageClarificationOwningChange(
		t,
		owningContent,
		owner,
		eventID,
		original.Source.ChallengeID,
		config.Reference,
	)
	if _, err = owningContent.Action(
		ctx,
		owner,
		eventID,
		staged.ID,
		staged.Revision,
		"release",
		"Synthetic owning criteria replacement",
	); !errors.Is(
		err,
		event.ErrBlocked,
	) {
		t.Fatal("owning criteria release skipped impact and reviewed attendee notice", err)
	}
	if _, err = runtime.Clarifications.SaveAnswer(
		ctx,
		owner,
		eventID,
		questionID,
		current.Revision,
		material,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("failed owning release allowed a verbal material-change substitute", err)
	}
	unchanged, err := runtime.Clarifications.Public(ctx, author, eventID, answerID)
	if err != nil || !unchanged.Answer.SourceCurrent || unchanged.Answer.Source != original.Source {
		t.Fatal(
			"draft or failed owning release silently changed answer applicability",
			unchanged,
			err,
		)
	}
	change(
		rubric.Command{
			Action:       "review_impact",
			Version:      config.Reference.Version,
			ImpactReview: "SYNTHETIC-PRIVATE-CRITERIA-IMPACT",
			Notice:       "Reviewed common notice: the demo category weight has changed.",
		},
	)
	reviewEvidence := dbgen.ClarificationReviewedRubricReleaseParams{
		EventID:          eventID,
		RubricID:         config.Reference.ID,
		Version:          config.Reference.Version,
		PublicDigest:     config.Reference.Digest,
		ChallengeID:      staged.ID,
		ChallengeVersion: staged.Version,
		ReviewedNotice:   "Reviewed common notice: the demo category weight has changed.",
	}
	checkEvidence := func(params dbgen.ClarificationReviewedRubricReleaseParams, want bool) {
		t.Helper()
		actual, evidenceErr := dbgen.New(runtime.Clarifications.Pool).
			ClarificationReviewedRubricRelease(ctx, params)
		if evidenceErr != nil || actual != want {
			t.Fatal("retained owning publication evidence mismatch", actual, want, evidenceErr)
		}
	}
	checkEvidence(reviewEvidence, false)
	if _, err = owningContent.Action(
		ctx,
		owner,
		eventID,
		staged.ID,
		staged.Revision,
		"release",
		"Synthetic reviewed material publication",
	); err != nil {
		t.Fatal(err)
	}
	checkEvidence(reviewEvidence, true)
	for _, mismatch := range []string{"event", "challenge", "rubric", "version", "digest", "notice"} {
		wrong := reviewEvidence
		switch mismatch {
		case "event":
			wrong.EventID = uuid.New()
		case "challenge":
			wrong.ChallengeID = uuid.New()
		case "rubric":
			wrong.RubricID = uuid.New()
		case "version":
			wrong.Version++
		case "digest":
			wrong.PublicDigest = "unrelated-public-snapshot"
		case "notice":
			wrong.ReviewedNotice = "unrelated notice"
		}
		checkEvidence(wrong, false)
	}
	var notices int
	if err = runtime.Clarifications.Pool.QueryRow(ctx, "SELECT count(*) FROM rubric_notice_recipients WHERE event_id=$1 AND challenge_id=$2 AND challenge_version=$3", eventID, staged.ID, staged.Version).
		Scan(&notices); err != nil ||
		notices != 2 {
		t.Fatal("material source lacked owning reviewed attendee notice evidence", notices, err)
	}
	for _, kind := range []string{"clarification", "challenge_change"} {
		wrongKind := material
		wrongKind.Kind = kind
		if _, err = runtime.Clarifications.SaveAnswer(
			ctx,
			owner,
			eventID,
			questionID,
			current.Revision,
			wrongKind,
		); !errors.Is(
			err,
			access.ErrConflict,
		) {
			t.Fatal("changed criteria were silently relabelled as an unchanged source", kind, err)
		}
	}
	current, err = runtime.Clarifications.SaveAnswer(
		ctx,
		owner,
		eventID,
		questionID,
		current.Revision,
		material,
	)
	if err != nil {
		t.Fatal(err)
	}
	if current.Source.Rubric != config.Reference ||
		current.Source.ChallengeVersion != staged.Version {
		t.Fatal("material answer did not bind actual owning release and approved criteria")
	}
	for _, action := range []string{"approve", "publish"} {
		current, err = runtime.Clarifications.ActAnswer(
			ctx,
			owner,
			eventID,
			answerID,
			current.Revision,
			clarification.AnswerCommand{Action: action, Version: current.Version},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	page, err := runtime.Clarifications.Public(ctx, author, eventID, answerID)
	if err != nil || !page.Answer.SourceCurrent || page.Answer.Source.Rubric != config.Reference ||
		len(page.History) != 3 ||
		page.History[1].SourceCurrent {
		t.Fatal(
			"material notice replaced historical source or omitted the actual owning version",
			page,
			err,
		)
	}
	// A challenge-only change is independently permitted after its real owning
	// release, while keeping the exact already published rubric unchanged.
	nextQuestion, err := runtime.Clarifications.Ask(
		ctx,
		author,
		eventID,
		clarification.QuestionInput{
			TeamID:      original.TeamID,
			CommandID:   uuid.New(),
			Topic:       "challenge",
			Body:        "SYNTHETIC-PRIVATE-LATER-QUESTION",
			Attachments: []uuid.UUID{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	challengeNotice := clarification.AnswerDocument{
		Title: "Actual reviewed challenge change",
		Body:  "SYNTHETIC-COMMON-CHALLENGE-CHANGE",
		Kind:  "challenge_change",
	}
	if _, err = runtime.Clarifications.SaveAnswer(
		ctx,
		owner,
		eventID,
		nextQuestion.ID,
		0,
		challengeNotice,
	); !errors.Is(
		err,
		access.ErrConflict,
	) {
		t.Fatal("challenge-change label substituted an unreleased owning version", err)
	}
	next := stageClarificationOwningChange(
		t,
		owningContent,
		owner,
		eventID,
		staged.ID,
		config.Reference,
	)
	if _, err = owningContent.Action(
		ctx,
		owner,
		eventID,
		next.ID,
		next.Revision,
		"release",
		"Synthetic actual challenge-only release",
	); err != nil {
		t.Fatal(err)
	}
	nextAnswer, err := runtime.Clarifications.SaveAnswer(
		ctx,
		owner,
		eventID,
		nextQuestion.ID,
		0,
		challengeNotice,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"approve", "publish"} {
		nextAnswer, err = runtime.Clarifications.ActAnswer(
			ctx,
			owner,
			eventID,
			nextAnswer.ID,
			nextAnswer.Revision,
			clarification.AnswerCommand{Action: action, Version: nextAnswer.Version},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	if nextAnswer.Source.Rubric != config.Reference ||
		nextAnswer.Source.ChallengeVersion != next.Version {
		t.Fatal("challenge-only notice failed exact released source binding")
	}
	return current
}
