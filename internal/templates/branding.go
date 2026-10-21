package templates

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // Only non-executable raster formats are accepted.
	_ "image/png"
	"math"
	"slices"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

type BrandView struct {
	Revision int64    `json:"revision"`
	Branding Branding `json:"branding"`
}

func brandError(field, message string) error {
	return &event.Blocked{
		Issues: []event.Issue{
			{Field: "branding." + field, Code: "invalid_branding", Message: message},
		},
	}
}

func luminance(hex string) float64 {
	var result float64
	for i, weight := range []float64{0.2126, 0.7152, 0.0722} {
		n, _ := strconv.ParseUint(hex[1+i*2:3+i*2], 16, 8)
		v := float64(n) / 255
		if v <= 0.04045 {
			v /= 12.92
		} else {
			v = math.Pow((v+0.055)/1.055, 2.4)
		}
		result += v * weight
	}
	return result
}

func contrast(a, b string) float64 {
	x, y := luminance(a), luminance(b)
	return (math.Max(x, y) + 0.05) / (math.Min(x, y) + 0.05)
}

func validateBranding(b Branding) error {
	for _, value := range []string{b.Primary, b.Accent, b.Background, b.Foreground} {
		if !colour.MatchString(value) {
			return brandError("colours", "Use six-digit hexadecimal colours, for example #173a35.")
		}
	}
	if contrast(b.Foreground, b.Background) < 4.5 {
		return brandError(
			"foreground",
			"Text and background need a contrast ratio of at least 4.5:1. Choose darker text or a lighter background.",
		)
	}
	if contrast(b.Primary, b.Background) < 4.5 {
		return brandError(
			"primary",
			"Primary and background need a contrast ratio of at least 4.5:1 for readable links and controls.",
		)
	}
	if len(b.Assets) > 3 {
		return brandError("assets", "Use at most one logo, icon and hero image.")
	}
	kinds := map[string]bool{}
	ids := map[uuid.UUID]bool{}
	for _, asset := range b.Assets {
		if asset.ID == uuid.Nil || ids[asset.ID] ||
			!slices.Contains([]string{"logo", "icon", "hero"}, asset.Kind) ||
			kinds[asset.Kind] {
			return brandError(
				"assets",
				"Each image needs a unique identity and kind: logo, icon or hero.",
			)
		}
		kinds[asset.Kind], ids[asset.ID] = true, true
		if len(asset.Content) > 512*1024 {
			return brandError(asset.Kind, "Keep each image at or below 512 KiB.")
		}
		info, format, err := image.DecodeConfig(bytes.NewReader(asset.Content))
		if err != nil || info.Width < 1 || info.Height < 1 || info.Width > 4096 ||
			info.Height > 4096 {
			return brandError(
				asset.Kind,
				"Choose a valid PNG or JPEG no larger than 4096 pixels per side.",
			)
		}
		if (format != "png" || asset.MediaType != "image/png") &&
			(format != "jpeg" || asset.MediaType != "image/jpeg") {
			return brandError(
				asset.Kind,
				"The image bytes must match the PNG or JPEG content type.",
			)
		}
		if _, _, err := image.Decode(bytes.NewReader(asset.Content)); err != nil {
			return brandError(
				asset.Kind,
				"The image is incomplete or corrupt. Export a fresh PNG or JPEG.",
			)
		}
	}
	return nil
}

func readBrand(ctx context.Context, q *dbgen.Queries, id uuid.UUID) (Branding, error) {
	b := EmptyDefinitions().Branding
	row, err := q.EventDefinitions(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return b, nil
	}
	if err != nil {
		return b, fmt.Errorf("read event branding: %w", err)
	}
	var definitions Definitions
	if err := json.Unmarshal(row.Definitions, &definitions); err != nil {
		return b, fmt.Errorf("decode event branding: %w", err)
	}
	return definitions.Branding, nil
}

// ReadBrand exposes branding without exposing forms, role briefs or message drafts.
func (s Service) ReadBrand(ctx context.Context, actor, id uuid.UUID) (BrandView, error) {
	q := dbgen.New(s.Pool)
	if err := (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		id,
		id,
		access.EventRead,
	); err != nil {
		return BrandView{}, fmt.Errorf("authorize branding: %w", err)
	}
	row, err := q.Event(ctx, id)
	if err != nil {
		return BrandView{}, fmt.Errorf("read branding revision: %w", err)
	}
	b, err := readBrand(ctx, q, id)
	return BrandView{Revision: row.Revision, Branding: b}, err
}

func (s Service) SaveBrand(
	ctx context.Context,
	actor, id uuid.UUID,
	revision int64,
	brand Branding,
) (BrandView, error) {
	var result BrandView
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, id)
		if err != nil {
			return fmt.Errorf("lock branding: %w", err)
		}
		if err := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			id,
			id,
			access.EventEdit,
		); err != nil {
			return fmt.Errorf("authorize branding save: %w", err)
		}
		if row.Revision != revision {
			return access.ErrConflict
		}
		if err := validateBranding(brand); err != nil {
			return err
		}
		bundle, err := snapshot(ctx, q, actor, id)
		if err != nil {
			return err
		}
		bundle.Definitions.Branding = brand
		data, err := json.Marshal(bundle.Definitions)
		if err != nil {
			return fmt.Errorf("encode branding: %w", err)
		}
		if err := q.SaveEventDefinitions(
			ctx,
			dbgen.SaveEventDefinitionsParams{EventID: id, Definitions: data},
		); err != nil {
			return fmt.Errorf("save branding: %w", err)
		}
		if _, err := q.BumpEventRevision(ctx, id); err != nil {
			return fmt.Errorf("advance branding revision: %w", err)
		}
		if err := audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &id,
				TargetID: id,
				Action:   audit.DefinitionsUpdated,
				Reason:   audit.OrganiserDecision,
			},
		); err != nil {
			return fmt.Errorf("audit branding: %w", err)
		}
		result = BrandView{Revision: revision + 1, Branding: brand}
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("update event branding: %w", err)
	}
	return result, nil
}
