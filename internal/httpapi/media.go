package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"mime"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/httpapi/contract"
	"github.com/deploymenttheory/hack-in-a-box/internal/media"
)

func (h handlers) ListMedia(
	ctx context.Context,
	r contract.ListMediaRequestObject,
) (contract.ListMediaResponseObject, error) {
	cursor := uuid.Nil
	if r.Params.After != nil {
		cursor = *r.Params.After
	}
	_, limit := page(nil, r.Params.Limit)
	view, err := h.Media.List(ctx, actor(ctx).ID, r.EventId, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("current reviewed media: %w", err)
	}
	return convertDTO[contract.ListMedia200JSONResponse](view)
}

func (h handlers) GetMedia(
	ctx context.Context,
	r contract.GetMediaRequestObject,
) (contract.GetMediaResponseObject, error) {
	view, err := h.Media.Read(ctx, actor(ctx).ID, r.EventId, r.MediaId)
	if err != nil {
		return nil, fmt.Errorf("current native media: %w", err)
	}
	return convertDTO[contract.GetMedia200JSONResponse](view)
}

func (h handlers) ChangeMedia(
	ctx context.Context,
	r contract.ChangeMediaRequestObject,
) (contract.ChangeMediaResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	command, err := convertDTO[media.Command](r.Body)
	if err != nil {
		return nil, err
	}
	view, err := h.Media.Change(
		ctx,
		actor(ctx).ID,
		r.EventId,
		r.MediaId,
		revision(r.Params.IfMatch),
		command,
	)
	if err != nil {
		return nil, fmt.Errorf("explicit media review intent: %w", err)
	}
	return convertDTO[contract.ChangeMedia200JSONResponse](view)
}

func (h handlers) UploadMediaPhoto(
	ctx context.Context,
	r contract.UploadMediaPhotoRequestObject,
) (contract.UploadMediaPhotoResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	metadata, err := convertDTO[media.Metadata](r.Body.Metadata)
	if err != nil {
		return nil, err
	}
	view, err := h.Media.CollectPhoto(
		ctx,
		media.Upload{
			ID:       r.Body.Id,
			Metadata: metadata,
			Reason:   r.Body.Reason,
			File: files.Upload{
				Actor:     actor(ctx).ID,
				Event:     r.EventId,
				Key:       r.Params.IdempotencyKey,
				Revision:  revision(r.Params.IfMatch),
				Filename:  r.Body.Filename,
				MediaType: string(r.Body.MediaType),
				Content:   r.Body.Content,
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("protected photographer upload: %w", err)
	}
	return convertDTO[contract.UploadMediaPhoto200JSONResponse](view)
}

func (h handlers) GetMediaOriginal(
	ctx context.Context,
	r contract.GetMediaOriginalRequestObject,
) (contract.GetMediaOriginalResponseObject, error) {
	file, data, err := h.Media.Original(ctx, actor(ctx).ID, r.EventId, r.MediaId)
	if err != nil {
		return nil, fmt.Errorf("protected current media original: %w", err)
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": file.Filename})
	return contract.GetMediaOriginal200ApplicationoctetStreamResponse{
		Body:          bytes.NewReader(data),
		ContentLength: int64(len(data)),
		Headers: contract.GetMediaOriginal200ResponseHeaders{
			ContentDisposition: &disposition,
		},
	}, nil
}

func (h handlers) GetMediaThumbnail(
	ctx context.Context,
	r contract.GetMediaThumbnailRequestObject,
) (contract.GetMediaThumbnailResponseObject, error) {
	data, err := h.Media.Thumbnail(ctx, actor(ctx).ID, r.EventId, r.MediaId)
	if err != nil {
		return nil, fmt.Errorf("protected current media thumbnail: %w", err)
	}
	disposition := mime.FormatMediaType(
		"inline",
		map[string]string{"filename": "reviewed-thumbnail.png"},
	)
	return contract.GetMediaThumbnail200ApplicationoctetStreamResponse{
		Body:          bytes.NewReader(data),
		ContentLength: int64(len(data)),
		Headers: contract.GetMediaThumbnail200ResponseHeaders{
			ContentDisposition: &disposition,
		},
	}, nil
}

func (h handlers) GetOwnPhotoConsent(
	ctx context.Context,
	r contract.GetOwnPhotoConsentRequestObject,
) (contract.GetOwnPhotoConsentResponseObject, error) {
	view, err := h.Privacy.OwnPhotoConsent(ctx, actor(ctx).ID, r.EventId)
	if err != nil {
		return nil, fmt.Errorf("own native photo choice: %w", err)
	}
	return convertDTO[contract.GetOwnPhotoConsent200JSONResponse](view)
}

func (h handlers) SetOwnPhotoConsent(
	ctx context.Context,
	r contract.SetOwnPhotoConsentRequestObject,
) (contract.SetOwnPhotoConsentResponseObject, error) {
	if r.Body == nil {
		return nil, access.ErrInvalid
	}
	view, err := h.Privacy.SetOwnPhotoConsent(
		ctx,
		actor(ctx).ID,
		r.EventId,
		revision(r.Params.IfMatch),
		string(r.Body.Choice),
	)
	if err != nil {
		return nil, fmt.Errorf("change own native photo choice: %w", err)
	}
	return convertDTO[contract.SetOwnPhotoConsent200JSONResponse](view)
}

func (h handlers) GetMediaConsentGuidance(
	ctx context.Context,
	r contract.GetMediaConsentGuidanceRequestObject,
) (contract.GetMediaConsentGuidanceResponseObject, error) {
	cursor := uuid.Nil
	if r.Params.After != nil {
		cursor = *r.Params.After
	}
	_, limit := page(nil, r.Params.Limit)
	var view contract.GetMediaConsentGuidance200JSONResponse
	err := database.Within(ctx, h.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		native, err := h.Privacy.PhotoGuideInTransaction(
			ctx,
			tx,
			actor(ctx).ID,
			r.EventId,
			cursor,
			limit,
		)
		if err != nil {
			return fmt.Errorf("minimal photographer consent guidance: %w", err)
		}
		view, err = convertDTO[contract.GetMediaConsentGuidance200JSONResponse](native)
		return err
	})
	if err != nil {
		return view, fmt.Errorf("resolve current media policy: %w", err)
	}
	return view, nil
}
