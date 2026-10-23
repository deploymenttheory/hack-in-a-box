package httpapi

import (
	"context"
	"fmt"
	"math"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/surveys"
)

func (h handlers) surveyService() surveys.Service {
	if h.Surveys != nil {
		return *h.Surveys
	}
	return surveys.Service{Pool: h.Pool}
}

func (h handlers) ListOwnSurveys(
	ctx context.Context,
	r contract.ListOwnSurveysRequestObject,
) (contract.ListOwnSurveysResponseObject, error) {
	after := uuid.Nil
	if r.Params.After != nil {
		after = *r.Params.After
	}
	_, limit := page(nil, r.Params.Limit)
	v, err := h.surveyService().OwnDirectory(ctx, actor(ctx).ID, r.EventId, after, limit)
	if err != nil {
		return nil, fmt.Errorf("survey OwnDirectory: %w", err)
	}
	return convertDTO[contract.ListOwnSurveys200JSONResponse](v)
}

func (h handlers) ListSurveyConfigurations(
	ctx context.Context,
	r contract.ListSurveyConfigurationsRequestObject,
) (contract.ListSurveyConfigurationsResponseObject, error) {
	after := uuid.Nil
	if r.Params.After != nil {
		after = *r.Params.After
	}
	_, limit := page(nil, r.Params.Limit)
	v, err := h.surveyService().Configurations(ctx, actor(ctx).ID, r.EventId, after, limit)
	if err != nil {
		return nil, fmt.Errorf("survey Configurations: %w", err)
	}
	return convertDTO[contract.ListSurveyConfigurations200JSONResponse](v)
}

func (h handlers) GetSurveyConfiguration(
	ctx context.Context,
	r contract.GetSurveyConfigurationRequestObject,
) (contract.GetSurveyConfigurationResponseObject, error) {
	v, err := h.surveyService().Configuration(ctx, actor(ctx).ID, r.EventId, r.SurveyId)
	if err != nil {
		return nil, fmt.Errorf("survey Configuration: %w", err)
	}
	return convertDTO[contract.GetSurveyConfiguration200JSONResponse](v)
}

func (h handlers) GetOwnSurvey(
	ctx context.Context,
	r contract.GetOwnSurveyRequestObject,
) (contract.GetOwnSurveyResponseObject, error) {
	v, err := h.surveyService().ReadOwn(ctx, actor(ctx).ID, r.EventId, r.SurveyId)
	if err != nil {
		return nil, fmt.Errorf("survey ReadOwn: %w", err)
	}
	return convertDTO[contract.GetOwnSurvey200JSONResponse](v)
}

func (h handlers) RespondToSurvey(
	ctx context.Context,
	r contract.RespondToSurveyRequestObject,
) (contract.RespondToSurveyResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[surveys.ResponseCommand](r.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.surveyService().
		Respond(ctx, actor(ctx).ID, r.EventId, r.SurveyId, revision(r.Params.IfMatch), input)
	if err != nil {
		return nil, fmt.Errorf("survey Respond: %w", err)
	}
	return convertDTO[contract.RespondToSurvey200JSONResponse](v)
}

func (h handlers) ChangeSurveyConfiguration(
	ctx context.Context,
	r contract.ChangeSurveyConfigurationRequestObject,
) (contract.ChangeSurveyConfigurationResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[surveys.Command](r.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.surveyService().
		Change(ctx, actor(ctx).ID, r.EventId, r.SurveyId, revision(r.Params.IfMatch), input)
	if err != nil {
		return nil, fmt.Errorf("survey Change: %w", err)
	}
	return convertDTO[contract.ChangeSurveyConfiguration200JSONResponse](v)
}

func (h handlers) ListSurveyResponses(
	ctx context.Context,
	r contract.ListSurveyResponsesRequestObject,
) (contract.ListSurveyResponsesResponseObject, error) {
	after := uuid.Nil
	if r.Params.After != nil {
		after = *r.Params.After
	}
	_, limit := page(nil, r.Params.Limit)
	v, err := h.surveyService().Responses(ctx, actor(ctx).ID, r.EventId, r.SurveyId, after, limit)
	if err != nil {
		return nil, fmt.Errorf("survey Responses: %w", err)
	}
	return convertDTO[contract.ListSurveyResponses200JSONResponse](v)
}

func (h handlers) GetSurveyResponseHistory(
	ctx context.Context,
	r contract.GetSurveyResponseHistoryRequestObject,
) (contract.GetSurveyResponseHistoryResponseObject, error) {
	before := int64(math.MaxInt64)
	if r.Params.BeforeRevision != nil {
		before = *r.Params.BeforeRevision
	}
	_, limit := page(nil, r.Params.Limit)
	v, err := h.surveyService().
		ResponseHistory(ctx, actor(ctx).ID, r.EventId, r.SurveyId, r.PersonId, before, limit)
	if err != nil {
		return nil, fmt.Errorf("survey ResponseHistory: %w", err)
	}
	return convertDTO[contract.GetSurveyResponseHistory200JSONResponse](v)
}

func (h handlers) GetPublishedSurveyReport(
	ctx context.Context,
	r contract.GetPublishedSurveyReportRequestObject,
) (contract.GetPublishedSurveyReportResponseObject, error) {
	v, err := h.surveyService().PublishedReport(ctx, actor(ctx).ID, r.EventId, r.SurveyId)
	if err != nil {
		return nil, fmt.Errorf("survey PublishedReport: %w", err)
	}
	return convertDTO[contract.GetPublishedSurveyReport200JSONResponse](v)
}

func (h handlers) GetSurveyReportReview(
	ctx context.Context,
	r contract.GetSurveyReportReviewRequestObject,
) (contract.GetSurveyReportReviewResponseObject, error) {
	v, err := h.surveyService().ReportReview(ctx, actor(ctx).ID, r.EventId, r.SurveyId)
	if err != nil {
		return nil, fmt.Errorf("survey ReportReview: %w", err)
	}
	return convertDTO[contract.GetSurveyReportReview200JSONResponse](v)
}

func (h handlers) ChangeSurveyReport(
	ctx context.Context,
	r contract.ChangeSurveyReportRequestObject,
) (contract.ChangeSurveyReportResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	input, err := convertDTO[surveys.ReportCommand](r.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.surveyService().
		ChangeReport(ctx, actor(ctx).ID, r.EventId, r.SurveyId, revision(r.Params.IfMatch), input)
	if err != nil {
		return nil, fmt.Errorf("survey ChangeReport: %w", err)
	}
	return convertDTO[contract.ChangeSurveyReport200JSONResponse](v)
}
