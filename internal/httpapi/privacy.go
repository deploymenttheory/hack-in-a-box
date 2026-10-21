package httpapi

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
)

func (h handlers) GetOperationalProfile(
	ctx context.Context,
	r contract.GetOperationalProfileRequestObject,
) (contract.GetOperationalProfileResponseObject, error) {
	view, err := h.Privacy.Read(ctx, actor(ctx).ID, r.EventId, r.PersonId)
	if err != nil {
		return nil, fmt.Errorf("read operational profile: %w", err)
	}
	result, err := convertDTO[contract.GetOperationalProfile200JSONResponse](view)
	return result, err
}

func (h handlers) CorrectOperationalProfile(
	ctx context.Context,
	r contract.CorrectOperationalProfileRequestObject,
) (contract.CorrectOperationalProfileResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	if r.PersonId != actor(ctx).ID {
		return nil, access.ErrForbidden
	}
	fields, err := convertDTO[privacy.Fields](r.Body)
	if err != nil {
		return nil, err
	}
	view, err := h.Privacy.Correct(
		ctx,
		actor(ctx).ID,
		r.EventId,
		revision(r.Params.IfMatch),
		fields,
	)
	if err != nil {
		return nil, fmt.Errorf("correct operational profile: %w", err)
	}
	result, err := convertDTO[contract.CorrectOperationalProfile200JSONResponse](view)
	return result, err
}

func (h handlers) DeleteOperationalProfile(
	ctx context.Context,
	r contract.DeleteOperationalProfileRequestObject,
) (contract.DeleteOperationalProfileResponseObject, error) {
	if err := h.Privacy.Delete(ctx, actor(ctx).ID, r.EventId, r.PersonId); err != nil {
		return nil, fmt.Errorf("delete operational profile: %w", err)
	}
	return contract.DeleteOperationalProfile200JSONResponse{Removed: true}, nil
}

func (h handlers) ExportOperationalProfiles(
	ctx context.Context,
	r contract.ExportOperationalProfilesRequestObject,
) (contract.ExportOperationalProfilesResponseObject, error) {
	cursor := uuid.Nil
	if r.Params.After != nil {
		cursor = *r.Params.After
	}
	_, limit := page(nil, r.Params.Limit)
	rows, err := h.Privacy.Export(ctx, actor(ctx).ID, r.EventId, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("export operational profiles: %w", err)
	}
	items, err := convertDTO[[]contract.OperationalProfile](rows)
	if err != nil {
		return nil, err
	}
	body := contract.ProfilePage{Items: items}
	if len(rows) == int(limit) {
		last := rows[len(rows)-1].ID
		body.NextCursor = &last
	}
	disposition := "attachment; filename=operational-profiles.json"
	return contract.ExportOperationalProfiles200JSONResponse{
		Body: body,
		Headers: contract.ExportOperationalProfiles200ResponseHeaders{
			ContentDisposition: &disposition,
		},
	}, nil
}

func (h handlers) GetPrivacySettings(
	ctx context.Context,
	r contract.GetPrivacySettingsRequestObject,
) (contract.GetPrivacySettingsResponseObject, error) {
	settings, err := h.Privacy.Policy(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("read privacy settings: %w", err)
	}
	return contract.GetPrivacySettings200JSONResponse{
		RetentionDays: int(settings.RetentionDays),
		DraftDays:     int(settings.DraftDays),
	}, nil
}

func (h handlers) SetPrivacySettings(
	ctx context.Context,
	r contract.SetPrivacySettingsRequestObject,
) (contract.SetPrivacySettingsResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	if r.Body.RetentionDays < 1 || r.Body.RetentionDays > 3650 || r.Body.DraftDays < 1 ||
		r.Body.DraftDays > 90 {
		return nil, access.ErrInvalid
	}
	if err := h.Privacy.Configure(
		ctx,
		actor(ctx).ID,
		r.EventId,
		revision(r.Params.IfMatch),
		privacy.Settings{
			RetentionDays: int32(r.Body.RetentionDays),
			DraftDays:     int32(r.Body.DraftDays),
		},
	); err != nil {
		return nil, fmt.Errorf("configure privacy settings: %w", err)
	}
	return contract.SetPrivacySettings200JSONResponse(*r.Body), nil
}
