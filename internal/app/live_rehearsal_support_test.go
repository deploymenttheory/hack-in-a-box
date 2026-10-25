package app

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi"
)

type rehearsalSupport struct {
	questionID uuid.UUID
	answerID   uuid.UUID
	helpID     uuid.UUID
}

func seedRehearsalSupport(
	t *testing.T,
	r *liveRehearsal,
	deps httpapi.Dependencies,
) rehearsalSupport {
	t.Helper()
	question, err := deps.Clarifications.Ask(
		t.Context(),
		r.competitor.ID,
		r.eventID,
		clarification.QuestionInput{
			TeamID:    r.teamID,
			CommandID: uuid.New(),
			Topic:     "challenge",
			Body:      "PRIVATE-QUESTION-ORIGIN: synthetic team-only question",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := deps.Clarifications.SaveAnswer(
		t.Context(),
		r.owner.ID,
		r.eventID,
		question.ID,
		0,
		clarification.AnswerDocument{
			Title: "Rehearsal official answer",
			Body:  "Initial approved common answer.",
			Kind:  "clarification",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"approve", "publish"} {
		answer, err = deps.Clarifications.ActAnswer(
			t.Context(),
			r.owner.ID,
			r.eventID,
			answer.ID,
			answer.Revision,
			clarification.AnswerCommand{Action: action, Version: answer.Version},
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	helpID := uuid.New()
	if _, err := deps.Help.Create(
		t.Context(),
		r.competitor.ID,
		r.eventID,
		helpID,
		help.Input{
			TeamID:      r.teamID,
			CommandID:   uuid.New(),
			Category:    "setup",
			Description: "PRIVATE-HELP-DESCRIPTION: synthetic team setup request",
			Location:    "Synthetic rehearsal room",
		},
	); err != nil {
		t.Fatal(err)
	}
	return rehearsalSupport{questionID: question.ID, answerID: answer.ID, helpID: helpID}
}

func (s rehearsalSupport) correct(
	ctx context.Context,
	r *liveRehearsal,
	deps httpapi.Dependencies,
) error {
	current, err := deps.Clarifications.AnswerConfiguration(
		ctx,
		r.owner.ID,
		r.eventID,
		s.questionID,
	)
	if err != nil {
		return err
	}
	current, err = deps.Clarifications.SaveAnswer(
		ctx,
		r.owner.ID,
		r.eventID,
		s.questionID,
		current.Revision,
		clarification.AnswerDocument{
			Title: "Rehearsal official answer",
			Body:  "Corrected approved common answer applies to every current confirmed competitor.",
			Kind:  "clarification",
		},
	)
	if err != nil {
		return err
	}
	for _, action := range []string{"approve", "publish"} {
		current, err = deps.Clarifications.ActAnswer(
			ctx,
			r.owner.ID,
			r.eventID,
			current.ID,
			current.Revision,
			clarification.AnswerCommand{Action: action, Version: current.Version},
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func TestLiveRehearsalSupportClockPropagation(t *testing.T) {
	r := newLiveRehearsal(t)
	admin := r.surface(t, "admin")
	support := seedRehearsalSupport(t, r, admin.deps)
	items, err := admin.deps.Inbox.List(t.Context(), r.competitor.ID, nil, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items.Items {
		opened, err := admin.deps.Inbox.Open(t.Context(), r.competitor.ID, item.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		if opened.Destination != nil && opened.Destination.Kind == "clarification" {
			found = true
			if opened.Destination.ResourceID != support.answerID {
				t.Fatal("actual inbox owner changed the native answer reference")
			}
		}
	}
	if !found {
		t.Fatal("native common-answer publication produced no resolvable inbox destination")
	}
	if _, err := admin.deps.Clarifications.Public(
		t.Context(),
		r.mentor.ID,
		r.eventID,
		support.answerID,
	); err == nil {
		t.Fatal("controlled clock must not grant a mentor confirmed-participant source access")
	}
}
