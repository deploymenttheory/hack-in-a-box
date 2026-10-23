package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/privacy"
)

type scope struct {
	event                              dbgen.Event
	organiser, photographer, confirmed bool
}

func (s Service) guard(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	write bool,
) (scope, error) {
	var out scope
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, access.ErrNotFound
	}
	if err != nil {
		return out, fmt.Errorf("lock media current scope: %w", err)
	}
	roles, confirmed, err := access.ContentReader(ctx, q, actor, eventID)
	if err != nil {
		return out, fmt.Errorf("authorize current media actor: %w", err)
	}
	out = scope{
		event:        row,
		organiser:    slices.Contains(roles, "organiser"),
		photographer: slices.Contains(roles, "photographer"),
		confirmed:    confirmed,
	}
	if !out.organiser && !out.photographer && !confirmed {
		return out, access.ErrForbidden
	}
	if write && (!row.WritesAllowed || (!out.organiser && !out.photographer)) {
		return out, access.ErrForbidden
	}
	return out, nil
}

func validateMetadata(m Metadata) error {
	if !slices.Contains([]string{"photo", "recording"}, m.Kind) ||
		strings.TrimSpace(m.Title) == "" ||
		len(m.Title) > 200 ||
		len(m.Caption) > 4000 ||
		len(m.People) > 100 ||
		len(m.RecordingURL) > 2048 ||
		strings.ContainsRune(m.Title+m.Caption+m.RecordingURL, 0) {
		return access.ErrInvalid
	}
	if m.CapturedAt != nil &&
		(m.CapturedAt.IsZero() || m.CapturedAt.Year() < 1900 || m.CapturedAt.Year() > 9999) {
		return access.ErrInvalid
	}
	if m.Kind == "photo" && m.RecordingURL != "" {
		return access.ErrInvalid
	}
	if m.Kind == "recording" {
		u, err := url.Parse(m.RecordingURL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil ||
			strings.TrimSpace(m.RecordingURL) != m.RecordingURL ||
			u.Opaque != "" {
			return access.ErrInvalid
		}
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range m.People {
		if id == uuid.Nil || seen[id] {
			return access.ErrInvalid
		}
		seen[id] = true
	}
	return nil
}

func currentDocument(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, id uuid.UUID,
) (dbgen.EventMedium, Document, error) {
	head, err := q.MediaHead(ctx, dbgen.MediaHeadParams{EventID: eventID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return head, Document{}, access.ErrNotFound
	}
	if err != nil {
		return head, Document{}, fmt.Errorf("read current media head: %w", err)
	}
	row, err := q.MediaVersion(
		ctx,
		dbgen.MediaVersionParams{EventID: eventID, ID: id, Version: head.Version},
	)
	if err != nil {
		return head, Document{}, fmt.Errorf("read immutable media version: %w", err)
	}
	var document Document
	if err = json.Unmarshal(row.Document, &document); err != nil {
		return head, document, fmt.Errorf("decode immutable media metadata: %w", err)
	}
	if document.ID != id || document.OwnerID != head.OwnerID || document.Version != head.Version ||
		document.State != head.State {
		return head, document, access.ErrConflict
	}
	return head, document, nil
}

func owned(scope scope, actor uuid.UUID, d Document) bool {
	return scope.organiser || (scope.photographer && actor == d.OwnerID)
}

func (s Service) reviewConsents(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	d Document,
) ([]privacy.PhotoConsentBinding, error) {
	if d.Review == nil || !d.Review.ContentReviewed || !d.Review.IdentificationReviewed ||
		(len(d.Metadata.People) == 0 && !d.Review.NoIdentifiablePeople) ||
		(len(d.Metadata.People) > 0 && d.Review.NoIdentifiablePeople) {
		return nil, access.ErrConflict
	}
	values, err := s.nativePrivacy().
		PhotoConsentsInTransaction(ctx, tx, actor, eventID, d.Metadata.People)
	if err != nil {
		return nil, fmt.Errorf("resolve native media consent review: %w", err)
	}
	bindings := make([]privacy.PhotoConsentBinding, 0, len(values))
	for _, value := range values {
		if !value.Eligible || value.Choice != "yes" || value.Token == "" {
			return nil, access.ErrConflict
		}
		bindings = append(
			bindings,
			privacy.PhotoConsentBinding{PersonID: value.PersonID, Token: value.Token},
		)
	}
	return bindings, nil
}

func (s Service) reviewValid(ctx context.Context, tx pgx.Tx, eventID uuid.UUID, d Document) error {
	if d.Review == nil || !d.Review.ContentReviewed || !d.Review.IdentificationReviewed ||
		d.ReviewedBy == nil ||
		d.ReviewedAt == nil ||
		len(d.Metadata.People) != len(d.Consent) ||
		(len(d.Metadata.People) == 0 && !d.Review.NoIdentifiablePeople) ||
		(len(d.Metadata.People) > 0 && d.Review.NoIdentifiablePeople) {
		return access.ErrConflict
	}
	for i, id := range d.Metadata.People {
		if d.Consent[i].PersonID != id {
			return access.ErrConflict
		}
	}
	// Current reviewer role is part of continuing publication authority.
	roles, _, err := access.ContentReader(ctx, dbgen.New(tx), *d.ReviewedBy, eventID)
	if err != nil {
		return fmt.Errorf("recheck native media reviewer: %w", err)
	}
	if !slices.Contains(roles, "organiser") {
		return access.ErrConflict
	}
	if err = s.nativePrivacy().
		ValidatePhotoConsentsInTransaction(ctx, tx, eventID, d.Consent); err != nil {
		return fmt.Errorf("recheck reviewed media consent: %w", err)
	}
	return nil
}

func published(scope scope, d Document, now time.Time) bool {
	return d.State == "released" && d.ReleasedAt != nil && !now.Before(*d.ReleasedAt) &&
		(!d.Metadata.ChallengeRelated || (scope.event.PublishedAt.Valid && !now.Before(scope.event.PublishedAt.Time)))
}

func (s Service) nativePrivacy() privacy.Service {
	p := s.Privacy
	p.Pool = s.Pool
	p.Clock = s.now
	return p
}
