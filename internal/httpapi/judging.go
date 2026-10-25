package httpapi

import (
	"context"
	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/judging"
	"strconv"
	"strings"
)

func judgeRevision(raw string) (int64, error) {
	n, err := strconv.ParseInt(strings.Trim(raw, "\""), 10, 64)
	if err != nil || n < 0 {
		return 0, access.ErrInvalid
	}
	return n, nil
}
func (h handlers) GetJudgeRound(ctx context.Context, r contract.GetJudgeRoundRequestObject) (contract.GetJudgeRoundResponseObject, error) {
	v, err := h.Judging.Round(ctx, actor(ctx).ID, r.EventId, r.RunId)
	if err != nil {
		return nil, err
	}
	return convertDTO[contract.GetJudgeRound200JSONResponse](v)
}
func (h handlers) ChangeJudgeRound(ctx context.Context, r contract.ChangeJudgeRoundRequestObject) (contract.ChangeJudgeRoundResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	n, err := judgeRevision(r.Params.IfMatch)
	if err != nil {
		return nil, err
	}
	c, err := convertDTO[judging.RoundCommand](r.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.Judging.ChangeRound(ctx, actor(ctx).ID, r.EventId, r.RunId, n, c)
	if err != nil {
		return nil, err
	}
	return convertDTO[contract.ChangeJudgeRound200JSONResponse](v)
}
func (h handlers) GetJudgeCard(ctx context.Context, r contract.GetJudgeCardRequestObject) (contract.GetJudgeCardResponseObject, error) {
	v, err := h.Judging.Brief(ctx, actor(ctx).ID, r.EventId, r.RunId, r.TeamId)
	if err != nil {
		return nil, err
	}
	return convertDTO[contract.GetJudgeCard200JSONResponse](v)
}
func (h handlers) ChangeJudgeCard(ctx context.Context, r contract.ChangeJudgeCardRequestObject) (contract.ChangeJudgeCardResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	n, err := judgeRevision(r.Params.IfMatch)
	if err != nil {
		return nil, err
	}
	c, err := convertDTO[judging.CardCommand](r.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.Judging.ChangeCard(ctx, actor(ctx).ID, r.EventId, r.RunId, r.TeamId, n, c)
	if err != nil {
		return nil, err
	}
	return convertDTO[contract.ChangeJudgeCard200JSONResponse](v)
}
func (h handlers) GetJudgePractice(ctx context.Context, r contract.GetJudgePracticeRequestObject) (contract.GetJudgePracticeResponseObject, error) {
	v, err := h.Judging.Practice(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, err
	}
	return convertDTO[contract.GetJudgePractice200JSONResponse](v)
}
func (h handlers) ChangeJudgePractice(ctx context.Context, r contract.ChangeJudgePracticeRequestObject) (contract.ChangeJudgePracticeResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	n, err := judgeRevision(r.Params.IfMatch)
	if err != nil {
		return nil, err
	}
	c, err := convertDTO[judging.PracticeCommand](r.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.Judging.ChangePractice(ctx, actor(ctx).ID, r.EventId, n, c)
	if err != nil {
		return nil, err
	}
	return convertDTO[contract.ChangeJudgePractice200JSONResponse](v)
}
