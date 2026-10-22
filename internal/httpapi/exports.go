package httpapi

import (
	"bytes"
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
)

func (h handlers) ExportReadiness(
	ctx context.Context,
	r contract.ExportReadinessRequestObject,
) (contract.ExportReadinessResponseObject, error) {
	report, err := h.Preparation.Export(ctx, actor(ctx).ID, r.EventId, string(r.Params.Audience))
	if err != nil {
		return nil, fmt.Errorf("readiness download: %w", err)
	}
	body, err := report.CSV()
	if err != nil {
		return nil, fmt.Errorf("render export: %w", err)
	}
	disposition := "attachment; filename=readiness-" + string(r.Params.Audience) + ".csv"
	return contract.ExportReadiness200TextcsvResponse{
		Body:          bytes.NewReader(body),
		ContentLength: int64(len(body)),
		Headers:       contract.ExportReadiness200ResponseHeaders{ContentDisposition: &disposition},
	}, nil
}

func (h handlers) DownloadRunSheet(
	ctx context.Context,
	r contract.DownloadRunSheetRequestObject,
) (contract.DownloadRunSheetResponseObject, error) {
	report, err := h.Preparation.Export(ctx, actor(ctx).ID, r.EventId, string(r.Params.Audience))
	if err != nil {
		return nil, fmt.Errorf("run sheet download: %w", err)
	}
	body, err := report.HTML()
	if err != nil {
		return nil, fmt.Errorf("render export: %w", err)
	}
	disposition := "attachment; filename=run-sheet-" + string(r.Params.Audience) + ".html"
	return contract.DownloadRunSheet200TexthtmlResponse{
		Body:          bytes.NewReader(body),
		ContentLength: int64(len(body)),
		Headers: contract.DownloadRunSheet200ResponseHeaders{
			ContentDisposition: &disposition,
		},
	}, nil
}
