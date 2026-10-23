package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func fingerprint(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func validCommand(c Command) bool {
	return c.CommandID != uuid.Nil && c.Version >= 0 && strings.TrimSpace(c.Reason) != "" &&
		len(c.Reason) <= 2000
}

// Change records an explicit versioned intent. Retrying the identical command
// acknowledges its retained accepted version only after current authority is
// checked. Metadata and access flags always describe the CURRENT safe version.
func (s Service) Change(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	expectedRevision int64,
	c Command,
) (Item, error) {
	var out Item
	if id == uuid.Nil || !validCommand(c) {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		sc, err := s.guard(ctx, tx, actor, eventID, true)
		if err != nil {
			return err
		}
		head, d, readErr := currentDocument(ctx, q, eventID, id)
		exists := readErr == nil
		if readErr != nil && !errors.Is(readErr, access.ErrNotFound) {
			return readErr
		}
		if exists && !owned(sc, actor, d) {
			return access.ErrForbidden
		}
		receipt, err := q.MediaCommand(
			ctx,
			dbgen.MediaCommandParams{EventID: eventID, ActorID: actor, CommandID: c.CommandID},
		)
		if err == nil {
			if receipt.ID != id || receipt.Fingerprint != fingerprint(c) {
				return access.ErrConflict
			}
			out, err = s.project(ctx, tx, actor, sc, d)
			if err == nil {
				out.AcknowledgedVersion = &receipt.Version
			}
			return err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read media command receipt: %w", err)
		}
		if sc.event.Revision != expectedRevision || (!exists && c.Version != 0) ||
			(exists && head.Version != c.Version) {
			return access.ErrConflict
		}
		if !exists {
			if c.Action != "save" || c.Metadata == nil || c.Metadata.Kind != "recording" {
				return access.ErrInvalid
			}
			if err = q.CreateMediaHead(
				ctx,
				dbgen.CreateMediaHeadParams{EventID: eventID, ID: id, OwnerID: actor},
			); err != nil {
				return fmt.Errorf("create media draft: %w", err)
			}
			d = Document{ID: id, OwnerID: actor}
		}
		if err = s.applyCommand(ctx, tx, actor, sc, &d, c, exists); err != nil {
			return err
		}
		d.Version++
		d.ActorID = &actor
		d.Action = c.Action
		d.Reason = c.Reason
		d.ChangedAt = s.now()
		if err = s.persist(ctx, q, eventID, d, audit.MediaChanged); err != nil {
			return err
		}
		if err = q.RecordMediaCommand(
			ctx,
			dbgen.RecordMediaCommandParams{
				EventID:     eventID,
				ID:          id,
				ActorID:     actor,
				CommandID:   c.CommandID,
				Fingerprint: fingerprint(c),
				Version:     d.Version,
			},
		); err != nil {
			return fmt.Errorf("retain media command receipt: %w", err)
		}
		out, err = s.project(ctx, tx, actor, sc, d)
		if err == nil {
			out.AcknowledgedVersion = &d.Version
		}
		return err
	})
	if err != nil {
		return out, fmt.Errorf("resolve current media policy: %w", err)
	}
	return out, nil
}

func clearReview(d *Document) {
	d.Review = nil
	d.Consent = nil
	d.ReviewedBy = nil
	d.ReviewedAt = nil
	d.ReleasedBy = nil
	d.ReleasedAt = nil
}

func (s Service) persist(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	d Document,
	action audit.Action,
) error {
	wire, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("encode media version: %w", err)
	}
	if err = q.InsertMediaVersion(
		ctx,
		dbgen.InsertMediaVersionParams{
			EventID:   eventID,
			ID:        d.ID,
			Version:   d.Version,
			Document:  wire,
			ActorID:   d.ActorID,
			Action:    d.Action,
			Reason:    d.Reason,
			CreatedAt: d.ChangedAt,
		},
	); err != nil {
		return fmt.Errorf("append immutable media version: %w", err)
	}
	released := pgtype.Int8{}
	if d.State == "released" {
		released = pgtype.Int8{Int64: d.Version, Valid: true}
	}
	if err = q.SaveMediaHead(
		ctx,
		dbgen.SaveMediaHeadParams{
			EventID:         eventID,
			ID:              d.ID,
			Version:         d.Version,
			State:           d.State,
			ReleasedVersion: released,
		},
	); err != nil {
		return fmt.Errorf("advance media version: %w", err)
	}
	if _, err = q.BumpEventRevision(ctx, eventID); err != nil {
		return fmt.Errorf("advance media event revision: %w", err)
	}
	reason := audit.UserRequest
	if action == audit.MediaRevoked {
		reason = audit.ConsentPolicy
	}
	err = audit.Append(
		ctx,
		q,
		audit.Record{
			ActorID:  d.ActorID,
			EventID:  &eventID,
			TargetID: d.ID,
			Action:   action,
			Reason:   reason,
			Metadata: audit.Metadata{Revision: &d.Version},
		},
	)
	if err != nil {
		return fmt.Errorf("append native decision audit: %w", err)
	}
	return nil
}

func (s Service) applyCommand(
	ctx context.Context,
	tx pgx.Tx,
	actor uuid.UUID,
	sc scope,
	d *Document,
	c Command,
	exists bool,
) error {
	var err error
	switch c.Action {
	case "save":
		if c.Metadata == nil || validateMetadata(*c.Metadata) != nil {
			return access.ErrInvalid
		}
		if exists && c.Metadata.Kind != d.Metadata.Kind {
			return access.ErrInvalid
		}
		d.Metadata = *c.Metadata
		d.State = "draft"
		clearReview(d)
	case "review":
		if !sc.organiser || c.Review == nil || !exists || d.State == "withdrawn" {
			return access.ErrForbidden
		}
		d.Review = c.Review
		d.Consent, err = s.reviewConsents(ctx, tx, actor, sc.event.ID, *d)
		if err != nil {
			return err
		}
		now := s.now()
		d.ReviewedBy = &actor
		d.ReviewedAt = &now
		d.ReleasedBy = nil
		d.ReleasedAt = nil
		d.State = "reviewed"
	case "publish":
		if !sc.organiser || d.State != "reviewed" {
			return access.ErrForbidden
		}
		if d.Metadata.ChallengeRelated &&
			(!sc.event.PublishedAt.Valid || s.now().Before(sc.event.PublishedAt.Time)) {
			return access.ErrConflict
		}
		if err = s.reviewValid(ctx, tx, sc.event.ID, *d); err != nil {
			return err
		}
		now := s.now()
		d.ReleasedBy = &actor
		d.ReleasedAt = &now
		d.State = "released"
	case "withdraw":
		if !exists {
			return access.ErrNotFound
		}
		d.State = "withdrawn"
		d.ReleasedAt = nil
		d.ReleasedBy = nil
	default:
		return access.ErrInvalid
	}
	return nil
}
