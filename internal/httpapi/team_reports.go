package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/teamreports"
)

func (h handlers) GetTeamFeedbackReport(
	ctx context.Context,
	r contract.GetTeamFeedbackReportRequestObject,
) (contract.GetTeamFeedbackReportResponseObject, error) {
	if !h.ResultsAdminSurface || h.TeamReports.Pool == nil {
		return nil, access.ErrForbidden
	}
	value, err := h.TeamReports.Read(ctx, actor(ctx).ID, r.EventId, r.TeamId)
	if err != nil {
		return nil, fmt.Errorf("read private team report: %w", err)
	}
	return convertDTO[contract.GetTeamFeedbackReport200JSONResponse](value)
}

func (h handlers) ChangeTeamFeedbackReport(
	ctx context.Context,
	r contract.ChangeTeamFeedbackReportRequestObject,
) (contract.ChangeTeamFeedbackReportResponseObject, error) {
	if !h.ResultsAdminSurface || h.TeamReports.Pool == nil {
		return nil, access.ErrForbidden
	}
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	expected, err := judgeRevision(r.Params.IfMatch)
	if err != nil {
		return nil, fmt.Errorf("report revision: %w", err)
	}
	command, err := convertDTO[teamreports.Command](r.Body)
	if err != nil {
		return nil, fmt.Errorf("report command: %w", err)
	}
	value, err := h.TeamReports.Change(ctx, actor(ctx).ID, r.EventId, r.TeamId, expected, command)
	if err != nil {
		return nil, fmt.Errorf("change private team report: %w", err)
	}
	return convertDTO[contract.ChangeTeamFeedbackReport200JSONResponse](value)
}

func (h handlers) GetTeamFeedbackReportHistory(
	ctx context.Context,
	r contract.GetTeamFeedbackReportHistoryRequestObject,
) (contract.GetTeamFeedbackReportHistoryResponseObject, error) {
	if !h.ResultsAdminSurface || h.TeamReports.Pool == nil {
		return nil, access.ErrForbidden
	}
	value, err := h.TeamReports.History(ctx, actor(ctx).ID, r.EventId, r.TeamId)
	if err != nil {
		return nil, fmt.Errorf("read team report history: %w", err)
	}
	items, err := convertDTO[[]contract.TeamFeedbackRecord](value)
	if err != nil {
		return nil, fmt.Errorf("project team report history: %w", err)
	}
	return contract.GetTeamFeedbackReportHistory200JSONResponse{Items: items}, nil
}

func (h handlers) GetTeamFeedbackReportReceipt(
	ctx context.Context,
	r contract.GetTeamFeedbackReportReceiptRequestObject,
) (contract.GetTeamFeedbackReportReceiptResponseObject, error) {
	if h.TeamReports.Pool == nil {
		return nil, access.ErrForbidden
	}
	value, err := h.TeamReports.OriginalReceipt(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.TeamId,
		r.RequestId,
	)
	if err != nil {
		return nil, fmt.Errorf("recover original team report acknowledgement: %w", err)
	}
	return convertDTO[contract.GetTeamFeedbackReportReceipt200JSONResponse](struct {
		Receipt *teamreports.Receipt `json:"receipt"`
	}{value})
}

func (h handlers) GetReleasedTeamFeedbackReport(
	ctx context.Context,
	r contract.GetReleasedTeamFeedbackReportRequestObject,
) (contract.GetReleasedTeamFeedbackReportResponseObject, error) {
	if h.TeamReports.Pool == nil {
		return nil, access.ErrForbidden
	}
	value, err := h.TeamReports.ReadReleased(ctx, actor(ctx).ID, r.EventId, r.TeamId)
	if err != nil {
		return nil, fmt.Errorf("read released own-team report: %w", err)
	}
	return convertDTO[contract.GetReleasedTeamFeedbackReport200JSONResponse](struct {
		Report *teamreports.Released `json:"report"`
	}{value})
}
