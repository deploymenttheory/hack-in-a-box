package httpapi

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
)

func (h handlers) GetTeamSubmission(
	ctx context.Context,
	r contract.GetTeamSubmissionRequestObject,
) (contract.GetTeamSubmissionResponseObject, error) {
	v, err := h.Submissions.Read(ctx, actor(ctx).ID, r.EventId, r.TeamId)
	if err != nil {
		return nil, fmt.Errorf("team submission: %w", err)
	}
	return convertDTO[contract.GetTeamSubmission200JSONResponse](v)
}

func (h handlers) SubmitTeamEvidence(
	ctx context.Context,
	r contract.SubmitTeamEvidenceRequestObject,
) (contract.SubmitTeamEvidenceResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	cmd, err := convertDTO[submissions.Command](r.Body)
	if err != nil {
		return nil, err
	}
	v, err := h.Submissions.Submit(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.TeamId,
		revision(r.Params.IfMatch),
		cmd,
	)
	if err != nil {
		return nil, fmt.Errorf("submit evidence: %w", err)
	}
	return convertDTO[contract.SubmitTeamEvidence200JSONResponse](v)
}

func (h handlers) GetTeamDraft(
	ctx context.Context,
	r contract.GetTeamDraftRequestObject,
) (contract.GetTeamDraftResponseObject, error) {
	v, err := h.Submissions.Draft(ctx, actor(ctx).ID, r.EventId, r.TeamId)
	if err != nil {
		return nil, fmt.Errorf("private team draft: %w", err)
	}
	return convertDTO[contract.GetTeamDraft200JSONResponse](v)
}

func (h handlers) SaveTeamDraft(
	ctx context.Context,
	r contract.SaveTeamDraftRequestObject,
) (contract.SaveTeamDraftResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	items, err := convertDTO[[]submissions.Evidence](r.Body.Evidence)
	if err != nil {
		return nil, err
	}
	v, err := h.Submissions.SaveDraft(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.TeamId,
		revision(r.Params.IfMatch),
		items,
	)
	if err != nil {
		return nil, fmt.Errorf("save private team draft: %w", err)
	}
	return convertDTO[contract.SaveTeamDraft200JSONResponse](v)
}

func (h handlers) TeamSubmissionHistory(
	ctx context.Context,
	r contract.TeamSubmissionHistoryRequestObject,
) (contract.TeamSubmissionHistoryResponseObject, error) {
	before := int64(0)
	if r.Params.Before != nil {
		before = *r.Params.Before
	}
	_, limit := page(nil, r.Params.Limit)
	v, err := h.Submissions.History(ctx, actor(ctx).ID, r.EventId, r.TeamId, before, limit)
	if err != nil {
		return nil, fmt.Errorf("submission history: %w", err)
	}
	return convertDTO[contract.TeamSubmissionHistory200JSONResponse](v)
}

func (h handlers) ReviewTeamSubmission(
	ctx context.Context,
	r contract.ReviewTeamSubmissionRequestObject,
) (contract.ReviewTeamSubmissionResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	v, err := h.Submissions.Review(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.TeamId,
		revision(r.Params.IfMatch),
		r.Body.SubmissionRevision,
		string(r.Body.Status),
		r.Body.Reason,
	)
	if err != nil {
		return nil, fmt.Errorf("verify submission: %w", err)
	}
	return convertDTO[contract.ReviewTeamSubmission200JSONResponse](v)
}

func (h handlers) SetSubmissionOutcome(
	ctx context.Context,
	r contract.SetSubmissionOutcomeRequestObject,
) (contract.SetSubmissionOutcomeResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	v, err := h.Submissions.Outcome(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.TeamId,
		revision(r.Params.IfMatch),
		string(r.Body.Outcome),
		r.Body.Reason,
	)
	if err != nil {
		return nil, fmt.Errorf("submission outcome: %w", err)
	}
	return convertDTO[contract.SetSubmissionOutcome200JSONResponse](v)
}
