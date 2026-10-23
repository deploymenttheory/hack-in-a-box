package media

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func (s Service) reader(ctx context.Context, tx pgx.Tx, actor, eventID uuid.UUID) error {
	roles, confirmed, err := access.ContentReader(ctx, dbgen.New(tx), actor, eventID)
	if err != nil {
		if err != nil {
			return fmt.Errorf("resolve current media policy: %w", err)
		}
		return nil
	}
	allowed, err := access.ContentAudienceAllows(
		ctx,
		dbgen.New(tx),
		actor,
		eventID,
		access.ContentAudience{Confirmed: true, Roles: []string{"organiser", "photographer"}},
		roles,
		confirmed,
	)
	if err != nil {
		if err != nil {
			return fmt.Errorf("resolve current media policy: %w", err)
		}
		return nil
	}
	if !allowed {
		return access.ErrForbidden
	}
	return nil
}

func (s Service) project(
	ctx context.Context,
	tx pgx.Tx,
	actor uuid.UUID,
	sc scope,
	d Document,
) (Item, error) {
	staff := owned(sc, actor, d)
	valid := s.reviewValid(ctx, tx, sc.event.ID, d) == nil
	if !staff && (!published(sc, d, s.now()) || !valid) {
		return Item{}, access.ErrNotFound
	}
	out := Item{
		ChallengeRelated: d.Metadata.ChallengeRelated,
		ID:               d.ID,
		Version:          d.Version,
		Kind:             d.Metadata.Kind,
		Title:            d.Metadata.Title,
		Caption:          d.Metadata.Caption,
		CapturedAt:       d.Metadata.CapturedAt,
		State:            d.State,
		People:           []PersonIndicator{},
		ConsentState:     "not_reviewed",
	}
	if valid {
		out.ConsentState = "reviewed"
	}
	// Tagged drafts can be identified using safe metadata, but no image/link is
	// disclosed while an identified person has absent/unknown/revoked consent.
	currentConsent, people, err := s.currentConsents(ctx, tx, actor, sc.event.ID, d, staff, valid)
	if err != nil {
		return Item{}, err
	}
	out.People = people
	if d.Review != nil && !valid {
		currentConsent = false
	}
	out.CanReadOriginal = currentConsent && d.State != "withdrawn" &&
		(staff || (published(sc, d, s.now()) && valid)) &&
		d.FileID != nil
	if currentConsent && d.State != "withdrawn" && (staff || valid) {
		out.RecordingURL = d.Metadata.RecordingURL
	}
	out.CanEdit = staff && sc.event.WritesAllowed
	out.CanReview = sc.organiser && sc.event.WritesAllowed && d.State != "withdrawn" &&
		currentConsent
	out.CanPublish = sc.organiser && sc.event.WritesAllowed && d.State == "reviewed" && valid &&
		(!d.Metadata.ChallengeRelated || (sc.event.PublishedAt.Valid && !s.now().Before(sc.event.PublishedAt.Time)))
	out.CanWithdraw = staff && sc.event.WritesAllowed && d.State != "withdrawn"
	return out, nil
}

func (s Service) List(
	ctx context.Context,
	actor, eventID, cursor uuid.UUID,
	limit int32,
) (Page, error) {
	out := Page{Items: []Item{}}
	if limit < 1 || limit > 100 {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		sc, err := s.guard(ctx, tx, actor, eventID, false)
		if err != nil {
			return err
		}
		if err = s.reader(ctx, tx, actor, eventID); err != nil {
			return err
		}
		out.Revision = sc.event.Revision
		out.CanCollect = sc.event.WritesAllowed && (sc.organiser || sc.photographer)
		out.CanReview = sc.event.WritesAllowed && sc.organiser
		rows, err := q.MediaPage(
			ctx,
			dbgen.MediaPageParams{
				EventID:      eventID,
				ActorID:      actor,
				Cursor:       cursor,
				Organiser:    sc.organiser,
				Photographer: sc.photographer,
				PageSize:     limit,
			},
		)
		if err != nil {
			return fmt.Errorf("read current media page: %w", err)
		}
		for _, row := range rows {
			_, d, e := currentDocument(ctx, q, eventID, row.ID)
			if e != nil {
				return e
			}
			item, e := s.project(ctx, tx, actor, sc, d)
			if errors.Is(e, access.ErrNotFound) {
				continue
			}
			if e != nil {
				return e
			}
			out.Items = append(out.Items, item)
		}
		if len(rows) == int(limit) {
			id := rows[len(rows)-1].ID
			out.NextCursor = &id
		}
		return nil
	})
	if err != nil {
		return out, fmt.Errorf("resolve current media policy: %w", err)
	}
	return out, nil
}

func (s Service) Read(ctx context.Context, actor, eventID, id uuid.UUID) (Item, error) {
	var out Item
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		sc, err := s.guard(ctx, tx, actor, eventID, false)
		if err != nil {
			return err
		}
		_, d, err := currentDocument(ctx, q, eventID, id)
		if err != nil {
			return err
		}
		out, err = s.project(ctx, tx, actor, sc, d)
		return err
	})
	if err != nil {
		return out, fmt.Errorf("resolve current media policy: %w", err)
	}
	return out, nil
}

// ResolveReferenceInTransaction exposes only a current released native route.
// It is not an organiser preview and never returns subject tags or external URLs.
func (s Service) ResolveReferenceInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, id uuid.UUID,
	version int64,
) (PublishedReference, error) {
	var out PublishedReference
	sc, err := s.guard(ctx, tx, actor, eventID, false)
	if err != nil {
		return out, err
	}
	if err = s.reader(ctx, tx, actor, eventID); err != nil {
		return out, err
	}
	head, d, err := currentDocument(ctx, dbgen.New(tx), eventID, id)
	if err != nil {
		return out, err
	}
	if version != d.Version || !head.ReleasedVersion.Valid ||
		head.ReleasedVersion.Int64 != version ||
		!published(sc, d, s.now()) {
		return out, access.ErrNotFound
	}
	if err = s.reviewValid(ctx, tx, eventID, d); err != nil {
		return out, access.ErrNotFound
	}
	return PublishedReference{
		ID:      id,
		Version: version,
		Kind:    d.Metadata.Kind,
		Title:   d.Metadata.Title,
		URL:     fmt.Sprintf("/events/%s/media/%s", eventID, id),
	}, nil
}

func (s Service) currentConsents(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	d Document,
	staff, valid bool,
) (bool, []PersonIndicator, error) {
	people := []PersonIndicator{}
	currentConsent := true
	if len(d.Metadata.People) > 0 {
		if staff {
			values, err := s.nativePrivacy().
				PhotoConsentsInTransaction(ctx, tx, actor, eventID, d.Metadata.People)
			if err != nil {
				if err != nil {
					return false, nil, fmt.Errorf("resolve current media policy: %w", err)
				}
				return false, nil, nil
			}
			for _, value := range values {
				people = append(
					people,
					PersonIndicator{
						ID:       value.PersonID,
						Name:     value.DisplayName,
						Consent:  value.Choice,
						Eligible: value.Eligible,
					},
				)
				if !value.Eligible || value.Choice != "yes" {
					currentConsent = false
				}
			}
		} else {
			currentConsent = valid
		}
	}
	return currentConsent, people, nil
}
