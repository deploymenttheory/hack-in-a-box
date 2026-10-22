package app

import (
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/deploymenttheory/hack-in-a-box/internal/content"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/permissions"
)

func newApprovedRaceAnswer(
	t *testing.T,
	runtime communicationRuntime,
	owner, author, approver, eventID, teamID uuid.UUID,
) clarification.AnswerConfiguration {
	t.Helper()
	question, err := runtime.Clarifications.Ask(
		t.Context(),
		author,
		eventID,
		clarification.QuestionInput{
			TeamID:      teamID,
			CommandID:   uuid.New(),
			Topic:       "challenge",
			Body:        "SYNTHETIC-PRIVATE-RACE-QUESTION",
			Attachments: []uuid.UUID{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := runtime.Clarifications.SaveAnswer(
		t.Context(),
		owner,
		eventID,
		question.ID,
		0,
		clarification.AnswerDocument{
			Title: "Common exact-source interpretation",
			Body:  "SYNTHETIC-COMMON-RACE-ANSWER",
			Kind:  "clarification",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	answer, err = runtime.Clarifications.ActAnswer(
		t.Context(),
		approver,
		eventID,
		answer.ID,
		answer.Revision,
		clarification.AnswerCommand{Action: "approve", Version: answer.Version},
	)
	if err != nil {
		t.Fatal(err)
	}
	return answer
}

func assertRacePublication(
	t *testing.T,
	runtime communicationRuntime,
	eventID, answerID uuid.UUID,
	publishErr error,
) int {
	t.Helper()
	var publications, notifications int
	if err := runtime.Clarifications.Pool.QueryRow(t.Context(), "SELECT count(*) FROM clarification_publications WHERE event_id=$1 AND answer_id=$2", eventID, answerID).
		Scan(&publications); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Clarifications.Pool.QueryRow(t.Context(), "SELECT count(*) FROM inbox_notifications WHERE event_id=$1 AND source_kind='clarification' AND source_id=$2", eventID, answerID).
		Scan(&notifications); err != nil {
		t.Fatal(err)
	}
	want := 0
	if publishErr == nil {
		want = 1
	}
	if publications != want || notifications != 2*want {
		t.Fatal(
			"racing source/authority transition left a partial or duplicated publication",
			publications,
			notifications,
			publishErr,
		)
	}
	return publications
}

func verifyClarificationSourceAndRoleRaces(
	t *testing.T,
	runtime communicationRuntime,
	owningContent content.Service,
	owner, author, secondStaff, eventID, teamID, originalQuestionID uuid.UUID,
) {
	t.Helper()
	ctx := t.Context()
	answer := newApprovedRaceAnswer(t, runtime, owner, author, owner, eventID, teamID)
	next := stageClarificationOwningChange(
		t,
		owningContent,
		owner,
		eventID,
		answer.Source.ChallengeID,
		answer.Source.Rubric,
	)
	var wg sync.WaitGroup
	var publishErr, releaseErr error
	wg.Go(func() {
		_, publishErr = runtime.Clarifications.ActAnswer(
			ctx,
			owner,
			eventID,
			answer.ID,
			answer.Revision,
			clarification.AnswerCommand{Action: "publish", Version: answer.Version},
		)
	})
	wg.Go(func() {
		_, releaseErr = owningContent.Action(
			ctx,
			owner,
			eventID,
			next.ID,
			next.Revision,
			"release",
			"Synthetic concurrent owning publication",
		)
	})
	wg.Wait()
	if releaseErr != nil || (publishErr != nil && !errors.Is(publishErr, access.ErrConflict)) {
		t.Fatal("owning-source race failed outside its valid serial orders", releaseErr, publishErr)
	}
	if assertRacePublication(t, runtime, eventID, answer.ID, publishErr) == 1 {
		page, err := runtime.Clarifications.Public(ctx, author, eventID, answer.ID)
		if err != nil || page.Answer.SourceCurrent || page.Answer.Source != answer.Source {
			t.Fatal(
				"source race silently rebound a retained official answer to the later release",
				page,
				err,
			)
		}
	} else if _, err := runtime.Clarifications.Public(ctx, author, eventID, answer.ID); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("unpublished losing answer became common during source race", err)
	}
	original, err := runtime.Clarifications.AnswerConfiguration(
		ctx,
		owner,
		eventID,
		originalQuestionID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = runtime.Clarifications.SaveAnswer(
		ctx,
		owner,
		eventID,
		originalQuestionID,
		original.Revision,
		clarification.AnswerDocument{
			Title: "Retained reviewed criteria notice",
			Body:  "The actual published criteria change remains authoritative after a later challenge-only release.",
			Kind:  "rubric_change",
		},
	); err != nil {
		t.Fatal(
			"later challenge-only publication erased the retained owning criteria-review evidence",
			err,
		)
	}
	roles := permissions.Service{Pool: runtime.Clarifications.Pool}
	changeRole := func(active bool) error {
		row, err := dbgen.New(runtime.Clarifications.Pool).Event(ctx, eventID)
		if err != nil {
			return err
		}
		_, err = roles.Change(ctx, permissions.Change{
			Actor:                             owner,
			Event:                             eventID,
			Person:                            secondStaff,
			Role:                              "organiser",
			Active:                            active,
			Revision:                          row.Revision,
			AcknowledgeCompetitionRestriction: true,
		})
		return err
	}
	for _, revokedActor := range []string{"approver", "publisher"} {
		if err := changeRole(true); err != nil {
			t.Fatal(err)
		}
		approver, publisher := secondStaff, owner
		if revokedActor == "publisher" {
			approver, publisher = owner, secondStaff
		}
		current := newApprovedRaceAnswer(t, runtime, owner, author, approver, eventID, teamID)
		var roleErr, publicationErr error
		wg.Go(func() {
			_, publicationErr = runtime.Clarifications.ActAnswer(
				ctx,
				publisher,
				eventID,
				current.ID,
				current.Revision,
				clarification.AnswerCommand{Action: "publish", Version: current.Version},
			)
		})
		wg.Go(func() { roleErr = changeRole(false) })
		wg.Wait()
		if roleErr != nil ||
			(publicationErr != nil && !errors.Is(publicationErr, access.ErrNotFound)) {
			t.Fatal(
				"native staff-revocation race failed outside its valid serial orders",
				revokedActor,
				roleErr,
				publicationErr,
			)
		}
		if assertRacePublication(t, runtime, eventID, current.ID, publicationErr) == 1 {
			if page, err := runtime.Clarifications.Public(
				ctx,
				author,
				eventID,
				current.ID,
			); err != nil ||
				page.Answer.ID != current.ID {
				t.Fatal(
					"later staff revocation erased an already completed common publication",
					err,
				)
			}
		}
		if _, err := runtime.Clarifications.ActAnswer(
			ctx,
			secondStaff,
			eventID,
			current.ID,
			current.Revision,
			clarification.AnswerCommand{Action: "publish", Version: current.Version},
		); !errors.Is(
			err,
			access.ErrNotFound,
		) {
			t.Fatal("revoked publisher retained access through an idempotent action", err)
		}
	}
}
