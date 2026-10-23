package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"mime"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

func (h handlers) GetClarificationWorkspace(
	ctx context.Context,
	r contract.GetClarificationWorkspaceRequestObject,
) (contract.GetClarificationWorkspaceResponseObject, error) {
	v, err := h.Clarifications.Workspace(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("current clarification workspace: %w", err)
	}
	return convertDTO[contract.GetClarificationWorkspace200JSONResponse](v)
}

func (h handlers) GetClarificationReview(
	ctx context.Context,
	r contract.GetClarificationReviewRequestObject,
) (contract.GetClarificationReviewResponseObject, error) {
	v, err := h.Clarifications.Review(ctx, actor(ctx).ID, r.EventId, r.QuestionId)
	if err != nil {
		return nil, fmt.Errorf("private official answer review: %w", err)
	}
	return convertDTO[contract.GetClarificationReview200JSONResponse](v)
}

func (h handlers) SearchPublishedClarifications(
	ctx context.Context,
	r contract.SearchPublishedClarificationsRequestObject,
) (contract.SearchPublishedClarificationsResponseObject, error) {
	search := ""
	if r.Params.Search != nil {
		search = *r.Params.Search
	}
	cursor, limit := clarificationPage(r.Params.Cursor, r.Params.Limit)
	v, err := h.Clarifications.SearchPage(ctx, actor(ctx).ID, r.EventId, search, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("published common clarifications: %w", err)
	}
	return convertDTO[contract.SearchPublishedClarifications200JSONResponse](v)
}

func clarificationPage(cursor *uuid.UUID, limit *int) (uuid.UUID, int32) {
	id := uuid.Nil
	if cursor != nil {
		id = *cursor
	}
	_, size := page(nil, limit)
	return id, size
}

func (h handlers) ListTeamQuestions(
	ctx context.Context,
	r contract.ListTeamQuestionsRequestObject,
) (contract.ListTeamQuestionsResponseObject, error) {
	cursor, limit := clarificationPage(r.Params.Cursor, r.Params.Limit)
	v, err := h.Clarifications.Questions(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.Params.TeamId,
		cursor,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("private team question list: %w", err)
	}
	return convertDTO[contract.ListTeamQuestions200JSONResponse](v)
}

func (h handlers) AskTeamQuestion(
	ctx context.Context,
	r contract.AskTeamQuestionRequestObject,
) (contract.AskTeamQuestionResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	in, err := convertDTO[clarification.QuestionInput](r.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.Clarifications.Ask(ctx, actor(ctx).ID, r.EventId, in)
	if err != nil {
		return nil, fmt.Errorf("ask private team question: %w", err)
	}
	return convertDTO[contract.AskTeamQuestion200JSONResponse](v)
}

func (h handlers) GetPrivateTeamQuestion(
	ctx context.Context,
	r contract.GetPrivateTeamQuestionRequestObject,
) (contract.GetPrivateTeamQuestionResponseObject, error) {
	v, err := h.Clarifications.Question(ctx, actor(ctx).ID, r.EventId, r.QuestionId)
	if err != nil {
		return nil, fmt.Errorf("read private team question: %w", err)
	}
	return convertDTO[contract.GetPrivateTeamQuestion200JSONResponse](v)
}

func (h handlers) GetPrivateQuestionHistory(
	ctx context.Context,
	r contract.GetPrivateQuestionHistoryRequestObject,
) (contract.GetPrivateQuestionHistoryResponseObject, error) {
	cursor, limit := clarificationPage(r.Params.Cursor, r.Params.Limit)
	v, err := h.Clarifications.QuestionHistory(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.QuestionId,
		cursor,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("private question audit history: %w", err)
	}
	return convertDTO[contract.GetPrivateQuestionHistory200JSONResponse](v)
}

func (h handlers) TriageTeamQuestion(
	ctx context.Context,
	r contract.TriageTeamQuestionRequestObject,
) (contract.TriageTeamQuestionResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	v, err := h.Clarifications.Triage(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.QuestionId,
		revision(r.Params.IfMatch),
		string(r.Body.Action),
	)
	if err != nil {
		return nil, fmt.Errorf("triage private question: %w", err)
	}
	return convertDTO[contract.TriageTeamQuestion200JSONResponse](v)
}

func (h handlers) GetClarificationConfiguration(
	ctx context.Context,
	r contract.GetClarificationConfigurationRequestObject,
) (contract.GetClarificationConfigurationResponseObject, error) {
	v, err := h.Clarifications.AnswerConfiguration(ctx, actor(ctx).ID, r.EventId, r.QuestionId)
	if err != nil {
		return nil, fmt.Errorf("private official answer draft: %w", err)
	}
	return convertDTO[contract.GetClarificationConfiguration200JSONResponse](v)
}

func (h handlers) SaveClarificationDraft(
	ctx context.Context,
	r contract.SaveClarificationDraftRequestObject,
) (contract.SaveClarificationDraftResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	in, err := convertDTO[clarification.AnswerDocument](r.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.Clarifications.SaveAnswer(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.QuestionId,
		revision(r.Params.IfMatch),
		in,
	)
	if err != nil {
		return nil, fmt.Errorf("author exact official answer draft: %w", err)
	}
	return convertDTO[contract.SaveClarificationDraft200JSONResponse](v)
}

func (h handlers) ReviewClarification(
	ctx context.Context,
	r contract.ReviewClarificationRequestObject,
) (contract.ReviewClarificationResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	in, err := convertDTO[clarification.AnswerCommand](r.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.Clarifications.ActAnswer(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.AnswerId,
		revision(r.Params.IfMatch),
		in,
	)
	if err != nil {
		return nil, fmt.Errorf("review exact official answer version: %w", err)
	}
	return convertDTO[contract.ReviewClarification200JSONResponse](v)
}

func (h handlers) GetPublishedClarificationHistory(
	ctx context.Context,
	r contract.GetPublishedClarificationHistoryRequestObject,
) (contract.GetPublishedClarificationHistoryResponseObject, error) {
	cursor, size := page(r.Params.Cursor, r.Params.Limit)
	v, err := h.Clarifications.PublishedHistory(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.AnswerId,
		cursor,
		size,
	)
	if err != nil {
		return nil, fmt.Errorf("published common correction history: %w", err)
	}
	return convertDTO[contract.GetPublishedClarificationHistory200JSONResponse](v)
}

func (h handlers) GetQuestionAttachment(
	ctx context.Context,
	r contract.GetQuestionAttachmentRequestObject,
) (contract.GetQuestionAttachmentResponseObject, error) {
	file, data, err := h.Clarifications.Attachment(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.QuestionId,
		r.FileId,
	)
	if err != nil {
		return nil, fmt.Errorf("private question attachment: %w", err)
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": file.Filename})
	return contract.GetQuestionAttachment200ApplicationoctetStreamResponse{
		Body: bytes.NewReader(data), ContentLength: file.SizeBytes,
		Headers: contract.GetQuestionAttachment200ResponseHeaders{ContentDisposition: &disposition},
	}, nil
}

func (h handlers) GetPublishedClarification(
	ctx context.Context,
	r contract.GetPublishedClarificationRequestObject,
) (contract.GetPublishedClarificationResponseObject, error) {
	v, err := h.Clarifications.Public(ctx, actor(ctx).ID, r.EventId, r.AnswerId)
	if err != nil {
		return nil, fmt.Errorf("canonical common clarification: %w", err)
	}
	return convertDTO[contract.GetPublishedClarification200JSONResponse](v)
}
