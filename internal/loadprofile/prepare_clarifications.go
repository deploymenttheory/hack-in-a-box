package loadprofile

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
)

type PreparedClarification struct {
	EventID  uuid.UUID                         `json:"event_id"`
	Question clarification.Question            `json:"private_question_acknowledgement"`
	Review   clarification.AnswerConfiguration `json:"staff_publication_acknowledgement"`
	Public   clarification.PublicPage          `json:"confirmed_member_common_read"`
}

// PublishClarification keeps private question prose distinct from an explicitly
// authored common answer. Its source is derived by the native owner from the
// real common release; neither fixture input nor an answer label can create it.
func (p *NativePreparer) PublishClarification(ctx context.Context, owner, member *Session,
	eventID, teamID uuid.UUID, common clarification.Source, privateCanary, needle string,
) (PreparedClarification, error) {
	var prepared PreparedClarification
	if owner == nil || member == nil || eventID == uuid.Nil || teamID == uuid.Nil ||
		common.ChallengeID == uuid.Nil ||
		common.ChallengeVersion < 1 ||
		privateCanary == "" ||
		needle == "" {
		return prepared, fmt.Errorf(
			"%w: actual actors/common source and distinct public/private text required",
			errCohort,
		)
	}
	prepared.EventID = eventID
	base := "/api/v1/events/" + eventID.String()
	question, err := p.askPrivateClarification(ctx, member, eventID, teamID, common, privateCanary)
	if err != nil {
		return prepared, err
	}
	prepared.Question = question
	document := clarification.AnswerDocument{
		Title: needle + " · " + teamID.String(),
		Body:  "Reviewed common guidance for the synthetic service demonstration.",
		Kind:  "clarification",
	}
	zero := int64(0)
	review, err := prepareCall[clarification.AnswerConfiguration](
		ctx,
		p,
		owner,
		http.MethodPut,
		base+"/questions/"+question.ID.String()+"/answer",
		&zero,
		document,
	)
	if err != nil {
		return prepared, err
	}
	if review.ID == uuid.Nil || review.QuestionID != question.ID || review.Version != 1 ||
		review.Source != common ||
		review.Document != document {
		return prepared, fmt.Errorf(
			"%w: original answer authoring changed exact question/source/document",
			errCohort,
		)
	}
	review, err = p.publishReviewedClarification(ctx, owner, base, review, common)
	if err != nil {
		return prepared, err
	}
	prepared.Review = review
	if !review.Approved || !review.Published {
		return prepared, fmt.Errorf("%w: native answer approval/publication incomplete", errCohort)
	}
	prepared.Public, err = prepareCall[clarification.PublicPage](
		ctx,
		p,
		member,
		http.MethodGet,
		base+"/clarifications/"+review.ID.String(),
		nil,
		nil,
	)
	if err != nil {
		return prepared, err
	}
	public := prepared.Public.Answer
	if public.ID != review.ID || public.Version != review.Version || public.Document != document ||
		public.Source != common ||
		!public.Current ||
		!public.SourceCurrent {
		return prepared, fmt.Errorf(
			"%w: confirmed reader did not receive exact current common answer",
			errCohort,
		)
	}
	return prepared, nil
}

func (p *NativePreparer) askPrivateClarification(
	ctx context.Context,
	member *Session,
	eventID, teamID uuid.UUID,
	common clarification.Source,
	privateCanary string,
) (clarification.Question, error) {
	input := clarification.QuestionInput{
		TeamID:      teamID,
		CommandID:   uuid.New(),
		Topic:       "challenge",
		Body:        privateCanary,
		Attachments: []uuid.UUID{},
	}
	question, err := prepareCall[clarification.Question](
		ctx,
		p,
		member,
		http.MethodPost,
		"/api/v1/events/"+eventID.String()+"/questions",
		nil,
		input,
	)
	if err != nil {
		return clarification.Question{}, err
	}
	if question.ID == uuid.Nil || question.TeamID != teamID ||
		question.AuthorID != member.Person() ||
		question.Revision != 1 ||
		question.Source != common ||
		question.Body != privateCanary {
		return clarification.Question{}, fmt.Errorf(
			"%w: original private question changed author/team/common source",
			errCohort,
		)
	}
	return question, nil
}

func (p *NativePreparer) publishReviewedClarification(
	ctx context.Context,
	owner *Session,
	base string,
	review clarification.AnswerConfiguration,
	common clarification.Source,
) (clarification.AnswerConfiguration, error) {
	var err error
	for _, action := range []string{"approve", "publish"} {
		before := review
		review, err = prepareCall[clarification.AnswerConfiguration](
			ctx,
			p,
			owner,
			http.MethodPost,
			base+"/clarifications/"+before.ID.String()+"/actions",
			&before.Revision,
			clarification.AnswerCommand{Action: action, Version: before.Version},
		)
		if err != nil {
			return review, err
		}
		if review.ID != before.ID || review.Version != before.Version || review.Source != common ||
			review.Revision != before.Revision+1 {
			return review, fmt.Errorf(
				"%w: original common-answer action retargeted reviewed source",
				errCohort,
			)
		}
	}
	return review, nil
}
