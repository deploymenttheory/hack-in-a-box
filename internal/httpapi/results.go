package httpapi

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/results"
)

func (h handlers) GetResults(
	ctx context.Context,
	r contract.GetResultsRequestObject,
) (contract.GetResultsResponseObject, error) {
	if !h.ResultsAdminSurface || h.Results.Pool == nil {
		return nil, access.ErrForbidden
	}
	value, err := h.Results.Read(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("read private results: %w", err)
	}
	dto, err := resultPageDTO(value)
	if err != nil {
		return nil, fmt.Errorf("project private results: %w", err)
	}
	return contract.GetResults200JSONResponse(dto), nil
}

func (h handlers) ChangeResults(
	ctx context.Context,
	r contract.ChangeResultsRequestObject,
) (contract.ChangeResultsResponseObject, error) {
	if !h.ResultsAdminSurface || h.Results.Pool == nil {
		return nil, access.ErrForbidden
	}
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	expected, err := judgeRevision(r.Params.IfMatch)
	if err != nil {
		return nil, fmt.Errorf("results revision: %w", err)
	}
	command, err := convertDTO[results.Command](r.Body)
	if err != nil {
		return nil, fmt.Errorf("results command: %w", err)
	}
	value, err := h.Results.Change(ctx, actor(ctx).ID, r.EventId, expected, command)
	if err != nil {
		return nil, fmt.Errorf("change private results: %w", err)
	}
	dto, err := resultPageDTO(value)
	if err != nil {
		return nil, fmt.Errorf("project changed results: %w", err)
	}
	return contract.ChangeResults200JSONResponse(dto), nil
}

func (h handlers) GetResultBallots(
	ctx context.Context,
	r contract.GetResultBallotsRequestObject,
) (contract.GetResultBallotsResponseObject, error) {
	if h.Results.Pool == nil {
		return nil, access.ErrForbidden
	}
	value, err := h.Results.Ballots(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("read own result ballots: %w", err)
	}
	return convertDTO[contract.GetResultBallots200JSONResponse](value)
}

func (h handlers) VoteResultBallot(
	ctx context.Context,
	r contract.VoteResultBallotRequestObject,
) (contract.VoteResultBallotResponseObject, error) {
	if h.Results.Pool == nil {
		return nil, access.ErrForbidden
	}
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	expected, err := judgeRevision(r.Params.IfMatch)
	if err != nil {
		return nil, fmt.Errorf("result ballot revision: %w", err)
	}
	command, err := convertDTO[results.Command](r.Body)
	if err != nil {
		return nil, fmt.Errorf("result ballot command: %w", err)
	}
	value, err := h.Results.Vote(ctx, actor(ctx).ID, r.EventId, expected, command)
	if err != nil {
		return nil, fmt.Errorf("record own result ballot: %w", err)
	}
	return convertDTO[contract.VoteResultBallot200JSONResponse](value)
}

func (h handlers) GetResultReceipt(
	ctx context.Context,
	r contract.GetResultReceiptRequestObject,
) (contract.GetResultReceiptResponseObject, error) {
	if h.Results.Pool == nil {
		return nil, access.ErrForbidden
	}
	value, err := h.Results.OriginalReceipt(ctx, actor(ctx).ID, r.EventId, r.RequestId)
	if err != nil {
		return nil, fmt.Errorf("recover original result acknowledgement: %w", err)
	}
	return convertDTO[contract.GetResultReceipt200JSONResponse](value)
}

func (h handlers) GetResultAnnouncements(
	ctx context.Context,
	r contract.GetResultAnnouncementsRequestObject,
) (contract.GetResultAnnouncementsResponseObject, error) {
	if h.Results.Pool == nil {
		return nil, access.ErrForbidden
	}
	value, err := h.Results.Released(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("read released result announcements: %w", err)
	}
	items, err := convertDTO[[]contract.ResultAnnouncement](value)
	if err != nil {
		return nil, fmt.Errorf("project released result announcements: %w", err)
	}
	return contract.GetResultAnnouncements200JSONResponse{Items: items}, nil
}

// Native missing/scrubbed sources stay explicit absence in the public contract.
// Never invent approved rubric defaults for version-zero preparation.
func resultPageDTO(value results.Page) (contract.ResultPage, error) {
	dto, err := convertDTO[contract.ResultPage](value)
	if err != nil {
		return dto, fmt.Errorf("convert result preparation: %w", err)
	}
	for _, preview := range []*contract.ResultPreview{&dto.Preview, &dto.Round.Source} {
		if preview.Reference == nil || preview.Reference.Version < 1 {
			preview.Reference = nil
			preview.Criteria = nil
		}
		if preview.JudgeRunId != nil && *preview.JudgeRunId == uuid.Nil {
			preview.JudgeRunId = nil
		}
	}
	return dto, nil
}
