package httpapi

import (
	"bytes"
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/archives"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

func (h handlers) archiveOwner() archives.Service {
	s := h.Archives
	if s.Pool == nil {
		s.Pool = h.Pool
	}
	if h.Outcomes != nil {
		s.Outcomes = *h.Outcomes
	}
	s.Scorecards = h.Scorecards
	s.Surveys = h.surveyService()
	return s
}

func (h handlers) ListArchivedEvents(
	ctx context.Context,
	r contract.ListArchivedEventsRequestObject,
) (contract.ListArchivedEventsResponseObject, error) {
	after := uuid.Nil
	if r.Params.After != nil {
		after = *r.Params.After
	}
	_, limit := page(nil, r.Params.Limit)
	result, err := h.archiveOwner().List(ctx, actor(ctx).ID, after, limit)
	if err != nil {
		return nil, fmt.Errorf("archived events: %w", err)
	}
	return convertDTO[contract.ListArchivedEvents200JSONResponse](result)
}

func (h handlers) GetEventArchive(
	ctx context.Context,
	r contract.GetEventArchiveRequestObject,
) (contract.GetEventArchiveResponseObject, error) {
	result, err := h.archiveOwner().Read(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("event archive: %w", err)
	}
	return convertDTO[contract.GetEventArchive200JSONResponse](result)
}

func (h handlers) ListArchiveReports(
	ctx context.Context,
	r contract.ListArchiveReportsRequestObject,
) (contract.ListArchiveReportsResponseObject, error) {
	after := uuid.Nil
	if r.Params.After != nil {
		after = *r.Params.After
	}
	_, limit := page(nil, r.Params.Limit)
	result, err := h.archiveOwner().Reports(ctx, actor(ctx).ID, after, limit)
	if err != nil {
		return nil, fmt.Errorf("archive reports: %w", err)
	}
	return convertDTO[contract.ListArchiveReports200JSONResponse](result)
}

func (h handlers) ExportArchiveSummary(
	ctx context.Context,
	r contract.ExportArchiveSummaryRequestObject,
) (contract.ExportArchiveSummaryResponseObject, error) {
	result, err := h.archiveOwner().ExportSummary(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("archive summary export: %w", err)
	}
	body, err := result.CSV()
	if err != nil {
		return nil, fmt.Errorf("encode archive export: %w", err)
	}
	disposition := "attachment; filename=archive-summary.csv"
	return contract.ExportArchiveSummary200TextcsvResponse{
		Body:          bytes.NewReader(body),
		ContentLength: int64(len(body)),
		Headers: contract.ExportArchiveSummary200ResponseHeaders{
			ContentDisposition: &disposition,
		},
	}, nil
}

func (h handlers) ExportArchiveSource(
	ctx context.Context,
	r contract.ExportArchiveSourceRequestObject,
) (contract.ExportArchiveSourceResponseObject, error) {
	input := archives.ExportRequest{Source: string(r.Source)}
	if r.Params.ResourceId != nil {
		input.ResourceID = *r.Params.ResourceId
	}
	if r.Params.After != nil {
		input.Cursor = *r.Params.After
	}
	if r.Params.Audience != nil {
		input.Audience = string(*r.Params.Audience)
	}
	_, input.Limit = page(nil, r.Params.Limit)
	result, err := h.archiveOwner().Export(ctx, actor(ctx).ID, r.EventId, input)
	if err != nil {
		return nil, fmt.Errorf("selected archive export: %w", err)
	}
	body, err := convertDTO[contract.ArchiveExport](result)
	if err != nil {
		return nil, fmt.Errorf("encode archive export: %w", err)
	}
	disposition := "attachment; filename=archive-source.json"
	return contract.ExportArchiveSource200JSONResponse{
		Body:    body,
		Headers: contract.ExportArchiveSource200ResponseHeaders{ContentDisposition: &disposition},
	}, nil
}
