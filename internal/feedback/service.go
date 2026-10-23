package feedback

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

type (
	Service struct {
		Pool          *pgxpool.Pool
		Clock         func() time.Time
		Sources       map[string]SourcePolicy
		Notifications NotificationSink
	}
	NotificationSink interface {
		RecordFeedbackInTransaction(
			context.Context,
			pgx.Tx,
			uuid.UUID,
			uuid.UUID,
			int64,
			[]uuid.UUID,
		) error
		RecordFeedbackFollowupInTransaction(
			context.Context,
			pgx.Tx,
			uuid.UUID,
			uuid.UUID,
			uuid.UUID,
		) error
	}
)

func (s Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC().Truncate(time.Microsecond)
	}
	return time.Now().UTC().Truncate(time.Microsecond)
}

func wrap(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", action, err)
}

func lock(ctx context.Context, q *dbgen.Queries, id uuid.UUID) (dbgen.Event, error) {
	row, err := q.LockEvent(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, access.ErrNotFound
	}
	return row, wrap("lock feedback event", err)
}

func (s Service) authority(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID, teamID uuid.UUID,
	category string,
) (dbgen.HelpAuthorityRow, error) {
	var a dbgen.HelpAuthorityRow
	if err := (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		access.EventRead,
	); err != nil {
		return a, wrap("authorize feedback event reader", err)
	}
	a, err := q.HelpAuthority(
		ctx,
		dbgen.HelpAuthorityParams{
			EventID:  eventID,
			TeamID:   teamID,
			PersonID: actor,
			Category: category,
			NowAt:    s.now(),
		},
	)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !a.Active) {
		return a, access.ErrNotFound
	}
	if err != nil {
		return a, wrap("current feedback authority", err)
	}
	active, err := q.HelpTeamActive(ctx, dbgen.HelpTeamActiveParams{EventID: eventID, ID: teamID})
	if err != nil {
		return a, wrap("current feedback team", err)
	}
	if !active || (!a.TeamMember && !a.Organiser && !a.Helper) {
		return a, access.ErrNotFound
	}
	return a, nil
}

func (s Service) gate(row dbgen.Event, category string) error {
	if category != "competition_output" {
		return nil
	}
	var cfg event.Configuration
	if err := json.Unmarshal(row.Configuration, &cfg); err != nil {
		return wrap("decode feedback live start", err)
	}
	if !row.PublishedAt.Valid || s.now().Before(row.PublishedAt.Time) || cfg.LiveStart.IsZero() ||
		s.now().Before(cfg.LiveStart) {
		return access.ErrForbidden
	}
	return nil
}

func decodeDocument(raw []byte) (Document, error) {
	var d Document
	err := json.Unmarshal(raw, &d)
	return d, wrap("decode feedback version", err)
}

func document(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, id uuid.UUID,
	v int64,
) (Document, error) {
	raw, err := q.FeedbackVersion(
		ctx,
		dbgen.FeedbackVersionParams{EventID: eventID, ID: id, Version: v},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Document{}, access.ErrNotFound
	}
	if err != nil {
		return Document{}, wrap("read feedback version", err)
	}
	return decodeDocument(raw)
}

func head(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, id uuid.UUID,
) (dbgen.FeedbackHead, error) {
	h, err := q.FeedbackHead(ctx, dbgen.FeedbackHeadParams{EventID: eventID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return h, access.ErrNotFound
	}
	return h, wrap("read feedback head", err)
}

func digest(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", wrap("encode reviewed feedback", err)
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}

func payload(d Document) Input {
	return Input{
		TeamID:   d.TeamID,
		Category: d.Category,
		Title:    d.Title,
		Body:     d.Body,
		Sources:  d.Sources,
		HelpID:   d.HelpID,
	}
}

func (s Service) sourceAuthority(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	d Document,
) error {
	for _, ref := range d.Sources {
		owner := s.Sources[ref.Kind]
		if owner == nil {
			return access.ErrForbidden
		}
		if err := owner.AuthorizeInTransaction(ctx, tx, actor, eventID, d.TeamID, ref); err != nil {
			return wrap("native reviewed-feedback source", err)
		}
	}
	if d.HelpID != nil {
		owner := s.Sources["help"]
		if owner == nil {
			return access.ErrForbidden
		}
		if err := owner.AuthorizeInTransaction(
			ctx,
			tx,
			actor,
			eventID,
			d.TeamID,
			Source{Kind: "help", ID: *d.HelpID},
		); err != nil {
			return wrap("native originating help link", err)
		}
	}
	return nil
}

func retain(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	d Document,
	published pgtype.Int8,
) error {
	raw, err := json.Marshal(d)
	if err != nil {
		return wrap("encode immutable feedback", err)
	}
	if err = q.InsertFeedbackVersion(
		ctx,
		dbgen.InsertFeedbackVersionParams{
			EventID:  eventID,
			ID:       d.ID,
			Version:  d.Version,
			Document: raw,
		},
	); err != nil {
		return wrap("retain feedback version", err)
	}
	return wrap(
		"advance feedback pointer",
		q.UpdateFeedbackHead(
			ctx,
			dbgen.UpdateFeedbackHeadParams{
				EventID:          eventID,
				ID:               d.ID,
				CurrentVersion:   d.Version,
				PublishedVersion: published,
			},
		),
	)
}

func duplicate(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, actor, commandID, target uuid.UUID,
	kind, hash string,
) (int64, bool, error) {
	if commandID == uuid.Nil {
		return 0, false, access.ErrInvalid
	}
	old, err := q.FeedbackCommand(
		ctx,
		dbgen.FeedbackCommandParams{EventID: eventID, ActorID: actor, CommandID: commandID},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, wrap("read feedback command", err)
	}
	if old.TargetID != target || old.Kind != kind || old.PayloadDigest != hash {
		return 0, false, access.ErrConflict
	}
	return old.ResultVersion, true, nil
}

func command(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, actor, id, target uuid.UUID,
	kind, hash string,
	v int64,
) error {
	return wrap(
		"retain feedback command",
		q.InsertFeedbackCommand(
			ctx,
			dbgen.InsertFeedbackCommandParams{
				EventID:       eventID,
				ActorID:       actor,
				CommandID:     id,
				TargetID:      target,
				Kind:          kind,
				PayloadDigest: hash,
				ResultVersion: v,
			},
		),
	)
}

func record(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, actor, target uuid.UUID,
	v int64,
) error {
	return wrap(
		"audit reviewed support transition",
		audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &eventID,
				TargetID: target,
				Action:   audit.FeedbackChanged,
				Reason:   audit.UserRequest,
				Metadata: audit.Metadata{Revision: &v},
			},
		),
	)
}

func (s Service) Change(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	expected int64,
	commandID uuid.UUID,
	in Command,
) (Page, error) {
	var out Page
	if id == uuid.Nil || expected < 0 ||
		!slices.Contains([]string{"save", "review", "publish", "withdraw"}, in.Action) ||
		(in.Action == "save") != (in.Input != nil) {
		return out, access.ErrInvalid
	}
	if in.Input != nil {
		if err := in.Input.validate(); err != nil {
			return out, err
		}
	}
	hash, err := digest(struct {
		Expected int64   `json:"expected"`
		Command  Command `json:"command"`
	}{expected, in})
	if err != nil {
		return out, err
	}
	err = database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		var err error
		out, err = s.changeInTransaction(
			ctx,
			tx,
			q,
			actor,
			eventID,
			id,
			expected,
			commandID,
			in,
			hash,
		)
		return err
	})
	return out, wrap("change reviewed team feedback", err)
}

func (s Service) prepareChange(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	actor, id uuid.UUID,
	expected int64,
	in Command,
) (dbgen.FeedbackHead, Document, bool, error) {
	h, err := head(ctx, q, row.ID, id)
	fresh := errors.Is(err, access.ErrNotFound)
	var d Document
	if err != nil && !fresh {
		return h, d, fresh, err
	}
	if fresh && (expected != 0 || in.Input == nil) {
		return h, d, fresh, access.ErrNotFound
	}
	if fresh {
		d = Document{
			ID:       id,
			TeamID:   in.Input.TeamID,
			Category: in.Input.Category,
			AuthorID: actor,
			Sources:  []Source{},
		}
	} else {
		d, err = document(ctx, q, row.ID, id, h.CurrentVersion)
		if err != nil {
			return h, d, fresh, err
		}
	}
	a, err := s.authority(ctx, q, actor, row.ID, d.TeamID, d.Category)
	if err != nil {
		return h, d, fresh, err
	}
	if !a.Organiser && !a.Helper {
		return h, d, fresh, access.ErrForbidden
	}
	return h, d, fresh, s.gate(row, d.Category)
}

func (s Service) changeInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	actor, eventID, id uuid.UUID,
	expected int64,
	commandID uuid.UUID,
	in Command,
	hash string,
) (Page, error) {
	var out Page
	row, err := lock(ctx, q, eventID)
	if err != nil {
		return out, err
	}
	if !row.WritesAllowed {
		return out, access.ErrForbidden
	}
	h, d, fresh, err := s.prepareChange(ctx, q, row, actor, id, expected, in)
	if err != nil {
		return out, err
	}
	v, replay, err := duplicate(ctx, q, eventID, actor, commandID, id, "feedback", hash)
	if err != nil {
		return out, err
	}
	if replay {
		if _, err = document(ctx, q, eventID, id, v); err != nil {
			return out, err
		}
		out, err = s.project(ctx, tx, q, row, actor, h)
		out.CommandVersion = &v
		return out, err
	}
	if !fresh && expected != h.CurrentVersion {
		return out, access.ErrConflict
	}
	d, published, err := s.applyDocument(ctx, tx, q, row, actor, in, h, d)
	if err != nil {
		return out, err
	}
	d.Version++
	d.ChangedAt = s.now()
	if fresh {
		if err = q.InsertFeedbackHead(
			ctx,
			dbgen.InsertFeedbackHeadParams{
				EventID:  eventID,
				ID:       id,
				TeamID:   d.TeamID,
				Category: d.Category,
			},
		); err != nil {
			return out, wrap("create feedback identity", err)
		}
	}
	if err = retain(ctx, q, eventID, d, published); err != nil {
		return out, err
	}
	if err = command(
		ctx,
		q,
		eventID,
		actor,
		commandID,
		id,
		"feedback",
		hash,
		d.Version,
	); err != nil {
		return out, err
	}
	if err = record(ctx, q, eventID, actor, id, d.Version); err != nil {
		return out, err
	}
	h, err = head(ctx, q, eventID, id)
	if err != nil {
		return out, err
	}
	out, err = s.project(ctx, tx, q, row, actor, h)
	if err != nil {
		return out, err
	}
	out.CommandVersion = &d.Version
	if in.Action == "publish" {
		return out, s.notify(ctx, tx, q, row, h, d.Version)
	}
	return out, nil
}

func (s Service) applyDocument(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	row dbgen.Event,
	actor uuid.UUID,
	in Command,
	h dbgen.FeedbackHead,
	d Document,
) (Document, pgtype.Int8, error) {
	published := h.PublishedVersion
	switch in.Action {
	case "save":
		if d.TeamID != in.Input.TeamID || d.Category != in.Input.Category {
			return d, published, access.ErrInvalid
		}
		d.Title, d.Body, d.Sources, d.HelpID = in.Input.Title, in.Input.Body, in.Input.Sources, in.Input.HelpID
		if d.Sources == nil {
			d.Sources = []Source{}
		}
		d.State = "draft"
		d.AuthorID = actor
		d.ReviewerID = nil
		d.ReviewedDigest = ""
		d.PublishedAt = nil
		if err := s.sourceAuthority(ctx, tx, actor, row.ID, d); err != nil {
			return d, published, err
		}
	case "review":
		if d.State != "draft" {
			return d, published, access.ErrConflict
		}
		if err := s.sourceAuthority(ctx, tx, actor, row.ID, d); err != nil {
			return d, published, err
		}
		d.State = "reviewed"
		d.ReviewerID = &actor
		hash, err := digest(payload(d))
		if err != nil {
			return d, published, err
		}
		d.ReviewedDigest = hash
	case "publish":
		if err := s.validateRelease(ctx, tx, q, row, actor, d); err != nil {
			return d, published, err
		}
		now := s.now()
		d.PublishedAt = &now
		d.State = "published"
		published = pgtype.Int8{Int64: d.Version + 1, Valid: true}
	case "withdraw":
		if !h.PublishedVersion.Valid {
			return d, published, access.ErrConflict
		}
		published = pgtype.Int8{}
		d.State = "withdrawn"
		d.PublishedAt = nil
	}
	return d, published, nil
}

func (s Service) validateRelease(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	row dbgen.Event,
	actor uuid.UUID,
	d Document,
) error {
	actual, err := digest(payload(d))
	if err != nil {
		return err
	}
	if d.State != "reviewed" || d.ReviewerID == nil || actual != d.ReviewedDigest {
		return access.ErrConflict
	}
	if err = s.sourceAuthority(ctx, tx, actor, row.ID, d); err != nil {
		return err
	}
	reviewer, err := s.authority(ctx, q, *d.ReviewerID, row.ID, d.TeamID, d.Category)
	if err != nil {
		return err
	}
	if !reviewer.Organiser && !reviewer.Helper {
		return access.ErrForbidden
	}
	return s.sourceAuthority(ctx, tx, *d.ReviewerID, row.ID, d)
}
