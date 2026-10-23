package httpapi

import (
	"bytes"
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

func (h handlers) GetSurveyDashboard(
	ctx context.Context,
	r contract.GetSurveyDashboardRequestObject,
) (contract.GetSurveyDashboardResponseObject, error) {
	v, err := h.surveyService().Dashboard(ctx, actor(ctx).ID, r.EventId, r.SurveyId)
	if err != nil {
		return nil, fmt.Errorf("survey dashboard: %w", err)
	}
	return convertDTO[contract.GetSurveyDashboard200JSONResponse](v)
}

func (h handlers) ExportSurveyResponses(
	ctx context.Context,
	r contract.ExportSurveyResponsesRequestObject,
) (contract.ExportSurveyResponsesResponseObject, error) {
	body, err := h.surveyService().ExportResponses(ctx, actor(ctx).ID, r.EventId, r.SurveyId)
	if err != nil {
		return nil, fmt.Errorf("restricted survey export: %w", err)
	}
	disposition := "attachment; filename=restricted-survey-responses.csv"
	return contract.ExportSurveyResponses200TextcsvResponse{
		Body:          bytes.NewReader(body),
		ContentLength: int64(len(body)),
		Headers: contract.ExportSurveyResponses200ResponseHeaders{
			ContentDisposition: &disposition,
		},
	}, nil
}
