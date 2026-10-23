package media

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
)

const maxPhotoPixels = 16_000_000

func checkedImage(data []byte) (image.Image, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 8192 ||
		config.Height > 8192 ||
		config.Width*config.Height > maxPhotoPixels ||
		(format != "png" && format != "jpeg") {
		return nil, access.ErrInvalid
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, access.ErrInvalid
	}
	return img, nil
}

type collectorPolicy struct{ q *dbgen.Queries }

func (p collectorPolicy) WithQueries(
	q *dbgen.Queries,
) access.Policy {
	return collectorPolicy{q: q}
}

func (p collectorPolicy) Authorize(
	ctx context.Context,
	actor, eventID, target uuid.UUID,
	action access.Action,
) error {
	if action != access.FileWrite || target != eventID {
		return access.ErrForbidden
	}
	row, err := p.q.Event(ctx, eventID)
	if err != nil || !row.WritesAllowed {
		return access.ErrForbidden
	}
	roles, _, err := access.ContentReader(ctx, p.q, actor, eventID)
	if err != nil {
		return fmt.Errorf("resolve current media policy: %w", err)
	}
	for _, role := range roles {
		if role == "organiser" || role == "photographer" {
			return nil
		}
	}
	return access.ErrForbidden
}

// CollectPhoto keeps storage quotas/checksums and idempotency in the native file
// owner. The protected association is committed atomically with the first draft.
func (s Service) CollectPhoto(ctx context.Context, input Upload) (Item, error) {
	var out Item
	if input.ID == uuid.Nil || input.File.TeamID != nil || input.Metadata.Kind != "photo" ||
		validateMetadata(input.Metadata) != nil ||
		len(input.Reason) < 1 ||
		len(input.Reason) > 2000 {
		return out, access.ErrInvalid
	}
	if input.File.MediaType != "image/png" && input.File.MediaType != "image/jpeg" {
		return out, access.ErrInvalid
	}
	if _, err := checkedImage(input.File.Content); err != nil {
		return out, err
	}
	native := s.Files
	native.Pool = s.Pool
	native.Policy = collectorPolicy{q: dbgen.New(s.Pool)}
	binding := fingerprint(struct {
		ID       uuid.UUID
		Metadata Metadata
		Reason   string
	}{input.ID, input.Metadata, input.Reason})
	_, err := native.UploadTransactional(
		ctx,
		input.File,
		func(ctx context.Context, tx pgx.Tx, file dbgen.File, created bool) error {
			q := dbgen.New(tx)
			sc, e := s.guard(ctx, tx, input.File.Actor, input.File.Event, true)
			if e != nil {
				return e
			}
			if !created {
				receipt, e := q.MediaCommand(
					ctx,
					dbgen.MediaCommandParams{
						EventID:   input.File.Event,
						ActorID:   input.File.Actor,
						CommandID: input.File.Key,
					},
				)
				if e != nil || receipt.ID != input.ID || receipt.Fingerprint != binding {
					return access.ErrConflict
				}
				_, d, e := currentDocument(ctx, q, input.File.Event, input.ID)
				if e != nil {
					return e
				}
				if d.FileID == nil || *d.FileID != file.ID || !owned(sc, input.File.Actor, d) {
					return access.ErrConflict
				}
				out, e = s.project(ctx, tx, input.File.Actor, sc, d)
				if e == nil {
					out.AcknowledgedVersion = &receipt.Version
				}
				return e
			}
			if e = q.CreateMediaHead(
				ctx,
				dbgen.CreateMediaHeadParams{
					EventID: input.File.Event,
					ID:      input.ID,
					OwnerID: input.File.Actor,
				},
			); e != nil {
				return fmt.Errorf("create photo draft: %w", e)
			}
			d := Document{
				ID:        input.ID,
				OwnerID:   input.File.Actor,
				Version:   1,
				State:     "draft",
				Metadata:  input.Metadata,
				FileID:    &file.ID,
				ActorID:   &input.File.Actor,
				Action:    "upload",
				Reason:    input.Reason,
				ChangedAt: s.now(),
			}
			if e = q.InsertMediaFile(
				ctx,
				dbgen.InsertMediaFileParams{
					EventID: input.File.Event,
					MediaID: input.ID,
					FileID:  file.ID,
				},
			); e != nil {
				return fmt.Errorf("associate protected photo: %w", e)
			}
			if e = s.persist(ctx, q, input.File.Event, d, audit.MediaChanged); e != nil {
				return e
			}
			if e = q.RecordMediaCommand(
				ctx,
				dbgen.RecordMediaCommandParams{
					EventID:     input.File.Event,
					ID:          input.ID,
					ActorID:     input.File.Actor,
					CommandID:   input.File.Key,
					Fingerprint: binding,
					Version:     1,
				},
			); e != nil {
				return fmt.Errorf("retain photo upload receipt: %w", e)
			}
			out, e = s.project(ctx, tx, input.File.Actor, sc, d)
			if e == nil {
				out.AcknowledgedVersion = &d.Version
			}
			return e
		},
	)
	if err != nil {
		return out, fmt.Errorf("resolve current media policy: %w", err)
	}
	return out, nil
}

// Original authorizes the exact current native media before reading any bytes.
// No independent file grants or guessed file IDs can substitute for this check.
func (s Service) Original(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
) (dbgen.File, []byte, error) {
	var file dbgen.File
	var data []byte
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		sc, err := s.guard(ctx, tx, actor, eventID, false)
		if err != nil {
			return err
		}
		_, d, err := currentDocument(ctx, q, eventID, id)
		if err != nil {
			return err
		}
		item, err := s.project(ctx, tx, actor, sc, d)
		if err != nil {
			return err
		}
		if !item.CanReadOriginal || d.FileID == nil {
			return access.ErrForbidden
		}
		associated, err := q.MediaFile(ctx, dbgen.MediaFileParams{EventID: eventID, MediaID: id})
		if err != nil || associated != *d.FileID {
			return access.ErrForbidden
		}
		file, err = q.File(ctx, dbgen.FileParams{EventID: eventID, ID: associated})
		if err != nil {
			return fmt.Errorf("read protected photo metadata: %w", err)
		}
		data, err = files.ReadAuthorizedStored(ctx, s.Files.Storage, file)
		if err != nil {
			return fmt.Errorf("resolve current media policy: %w", err)
		}
		return nil
	})
	if err != nil {
		return file, data, fmt.Errorf("resolve current media policy: %w", err)
	}
	return file, data, nil
}

func (s Service) Thumbnail(ctx context.Context, actor, eventID, id uuid.UUID) ([]byte, error) {
	_, data, err := s.Original(ctx, actor, eventID, id)
	if err != nil {
		return nil, err
	}
	img, err := checkedImage(data)
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	const side = 640
	if w > side || h > side {
		if w >= h {
			h = h * side / w
			w = side
		} else {
			w = w * side / h
			h = side
		}
		if w < 1 {
			w = 1
		}
		if h < 1 {
			h = 1
		}
	}
	thumb := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := img.At(bounds.Min.X+x*bounds.Dx()/w, bounds.Min.Y+y*bounds.Dy()/h)
			thumb.SetNRGBA(x, y, color.NRGBAModel.Convert(c).(color.NRGBA))
		}
	}
	var encoded bytes.Buffer
	if err = png.Encode(&encoded, thumb); err != nil {
		return nil, fmt.Errorf("encode protected photo thumbnail: %w", err)
	}
	return encoded.Bytes(), nil
}
