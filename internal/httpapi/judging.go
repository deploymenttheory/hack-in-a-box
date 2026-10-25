package httpapi

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
)

func judgeRevision(raw string) (int64, error) {
	n, err := strconv.ParseInt(strings.Trim(raw, "\""), 10, 64)
	if err != nil || n < 0 {
		return 0, access.ErrInvalid
	}
	return n, nil
}

func (h handlers) GetJudgeRound(
	ctx context.Context,
	r contract.GetJudgeRoundRequestObject,
) (contract.GetJudgeRoundResponseObject, error) {
	v, err := h.Judging.Round(ctx, actor(ctx).ID, r.EventId, r.RunId)
	if err != nil {
		return nil, fmt.Errorf("judging request: %w", err)
	}
	return convertDTO[contract.GetJudgeRound200JSONResponse](v)
}

func (h handlers) ChangeJudgeRound(
	ctx context.Context,
	r contract.ChangeJudgeRoundRequestObject,
) (contract.ChangeJudgeRoundResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	n, err := judgeRevision(r.Params.IfMatch)
	if err != nil {
		return nil, fmt.Errorf("judging request: %w", err)
	}
	c, err := convertDTO[judging.RoundCommand](r.Body)
	if err != nil {
		return nil, fmt.Errorf("judging request: %w", err)
	}
	v, err := h.Judging.ChangeRound(ctx, actor(ctx).ID, r.EventId, r.RunId, n, c)
	if err != nil {
		return nil, fmt.Errorf("judging request: %w", err)
	}
	return convertDTO[contract.ChangeJudgeRound200JSONResponse](v)
}

func (h handlers) GetJudgeCard(
	ctx context.Context,
	r contract.GetJudgeCardRequestObject,
) (contract.GetJudgeCardResponseObject, error) {
	v, err := h.Judging.Brief(ctx, actor(ctx).ID, r.EventId, r.RunId, r.TeamId)
	if err != nil {
		return nil, fmt.Errorf("judging request: %w", err)
	}
	return convertDTO[contract.GetJudgeCard200JSONResponse](v)
}

func (h handlers) ChangeJudgeCard(
	ctx context.Context,
	r contract.ChangeJudgeCardRequestObject,
) (contract.ChangeJudgeCardResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	n, err := judgeRevision(r.Params.IfMatch)
	if err != nil {
		return nil, fmt.Errorf("judging request: %w", err)
	}
	c, err := convertDTO[judging.CardCommand](r.Body)
	if err != nil {
		return nil, fmt.Errorf("judging request: %w", err)
	}
	v, err := h.Judging.ChangeCard(ctx, actor(ctx).ID, r.EventId, r.RunId, r.TeamId, n, c)
	if err != nil {
		return nil, fmt.Errorf("judging request: %w", err)
	}
	return convertDTO[contract.ChangeJudgeCard200JSONResponse](v)
}

func (h handlers) GetJudgePractice(
	ctx context.Context,
	r contract.GetJudgePracticeRequestObject,
) (contract.GetJudgePracticeResponseObject, error) {
	v, err := h.Judging.Practice(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("judging request: %w", err)
	}
	return convertDTO[contract.GetJudgePractice200JSONResponse](v)
}

func (h handlers) ChangeJudgePractice(
	ctx context.Context,
	r contract.ChangeJudgePracticeRequestObject,
) (contract.ChangeJudgePracticeResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	n, err := judgeRevision(r.Params.IfMatch)
	if err != nil {
		return nil, fmt.Errorf("judging request: %w", err)
	}
	c, err := convertDTO[judging.PracticeCommand](r.Body)
	if err != nil {
		return nil, fmt.Errorf("judging request: %w", err)
	}
	v, err := h.Judging.ChangePractice(ctx, actor(ctx).ID, r.EventId, n, c)
	if err != nil {
		return nil, fmt.Errorf("judging request: %w", err)
	}
	return convertDTO[contract.ChangeJudgePractice200JSONResponse](v)
}

func (h handlers) GetJudgeRounds(
	ctx context.Context,
	r contract.GetJudgeRoundsRequestObject,
) (contract.GetJudgeRoundsResponseObject, error) {
	v, err := h.Judging.Rounds(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("discover judging: %w", err)
	}
	return convertDTO[contract.GetJudgeRounds200JSONResponse](v)
}

func (h handlers) GetJudgeReceipt(
	ctx context.Context,
	r contract.GetJudgeReceiptRequestObject,
) (contract.GetJudgeReceiptResponseObject, error) {
	v, err := h.Judging.OriginalReceipt(ctx, actor(ctx).ID, r.EventId, r.RequestId)
	if err != nil {
		return nil, fmt.Errorf("recover judge acknowledgement: %w", err)
	}
	return convertDTO[contract.GetJudgeReceipt200JSONResponse](v)
}

func (h handlers) GetJudgeSummary(
	ctx context.Context,
	r contract.GetJudgeSummaryRequestObject,
) (contract.GetJudgeSummaryResponseObject, error) {
	requestID := uuid.Nil
	if r.Params.RequestId != nil {
		requestID = *r.Params.RequestId
	}
	v, err := h.Judging.Summary(ctx, actor(ctx).ID, r.EventId, r.TeamId, requestID)
	if err != nil {
		return nil, fmt.Errorf("judge summary request: %w", err)
	}
	return convertDTO[contract.GetJudgeSummary200JSONResponse](v)
}

func (h handlers) ChangeJudgeSummary(
	ctx context.Context,
	r contract.ChangeJudgeSummaryRequestObject,
) (contract.ChangeJudgeSummaryResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	n, err := judgeRevision(r.Params.IfMatch)
	if err != nil {
		return nil, fmt.Errorf("judge summary revision: %w", err)
	}
	c, err := convertDTO[judging.SummaryCommand](r.Body)
	if err != nil {
		return nil, fmt.Errorf("judge summary command: %w", err)
	}
	v, err := h.Judging.ChangeSummary(ctx, actor(ctx).ID, r.EventId, r.TeamId, n, c)
	if err != nil {
		return nil, fmt.Errorf("judge summary request: %w", err)
	}
	return convertDTO[contract.ChangeJudgeSummary200JSONResponse](v)
}

func (h handlers) GetJudgeCardStates(
	ctx context.Context,
	r contract.GetJudgeCardStatesRequestObject,
) (contract.GetJudgeCardStatesResponseObject, error) {
	v, err := h.Judging.CardStates(ctx, actor(ctx).ID, r.EventId, r.RunId, r.TeamId)
	if err != nil {
		return nil, fmt.Errorf("organiser judge card metadata: %w", err)
	}
	return convertDTO[contract.GetJudgeCardStates200JSONResponse](v)
}

func (h handlers) GetJudgePresentationOrder(
	ctx context.Context,
	r contract.GetJudgePresentationOrderRequestObject,
) (contract.GetJudgePresentationOrderResponseObject, error) {
	value, err := h.Presentation.ReadJudgeOrder(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("current public judging order: %w", err)
	}
	return convertDTO[contract.GetJudgePresentationOrder200JSONResponse](value)
}
