package content

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/objectives"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
	"github.com/deploymenttheory/hack-in-a-box/internal/schedule"
)

func editor(ctx context.Context, q *dbgen.Queries, actor, eventID uuid.UUID) (dbgen.Event, error) {
	row, err := q.LockEvent(ctx, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, access.ErrNotFound
	}
	if err != nil {
		return row, fmt.Errorf("lock content edit: %w", err)
	}
	if err = (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		access.EventEdit,
	); err != nil {
		return row, fmt.Errorf("authorize content edit: %w", err)
	}
	return row, nil
}

func canonicalSchedule(row dbgen.Event, d *Document, kind string) error {
	if kind != "challenge" || row.PublishedAt.Valid {
		return nil
	}
	var c event.Configuration
	if err := json.Unmarshal(row.Configuration, &c); err != nil {
		return fmt.Errorf("decode challenge schedule: %w", err)
	}
	if c.LiveStart.IsZero() {
		return blocked(
			"missing_live_start",
			"Set the live hack dates before authoring the challenge.",
		)
	}
	due, err := schedule.ChallengePublication(c.LiveStart, c.Timezone, c.DSTChoice)
	if err != nil {
		return fmt.Errorf("resolve challenge publication: %w", err)
	}
	if d.ScheduledAt != nil && !d.ScheduledAt.Equal(due) {
		return blocked(
			"common_publication",
			"The challenge must use the common seven-day publication time.",
		)
	}
	d.ScheduledAt = &due
	return nil
}

func checkReferences(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, id uuid.UUID,
	d Document,
) error {
	for _, team := range d.Audience.Teams {
		row, err := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: eventID, ID: team})
		if err != nil || row.State == "disbanded" {
			return blocked("invalid_team", "Choose an active team from this event.")
		}
	}
	for _, fileID := range d.Attachments {
		file, err := q.File(ctx, dbgen.FileParams{EventID: eventID, ID: fileID})
		if err != nil || file.TeamID != nil {
			return blocked("invalid_attachment", "Choose an organiser attachment from this event.")
		}
		if access.RequireOrdinaryFile(ctx, q, eventID, fileID) != nil {
			return blocked(
				"invalid_attachment",
				"Use the reviewed media gallery for consent-dependent photographs.",
			)
		}
		refs, err := q.ContentFileItems(
			ctx,
			dbgen.ContentFileItemsParams{EventID: eventID, FileID: fileID},
		)
		if err != nil {
			return fmt.Errorf("read attachment references: %w", err)
		}
		for _, ref := range refs {
			if ref.ID != id {
				return blocked(
					"attachment_scope",
					"An attachment belongs to one content item. Upload a separate copy for another item.",
				)
			}
		}
	}
	return nil
}

func (s Service) Save(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	expected int64,
	input Input,
) (Page, error) {
	var result Page
	if id == uuid.Nil || expected < 0 {
		return result, access.ErrInvalid
	}
	if err := validate(input); err != nil {
		return result, err
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := editor(ctx, q, actor, eventID)
		if err != nil {
			return err
		}
		item, err := load(ctx, q, eventID, id)
		if errors.Is(err, access.ErrNotFound) && expected == 0 {
			if err = q.CreateContent(
				ctx,
				dbgen.CreateContentParams{EventID: eventID, ID: id, Kind: input.Kind},
			); err != nil {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) && pgErr.Code == "23505" {
					return access.ErrConflict
				}
				return fmt.Errorf("create content: %w", err)
			}
			item, err = load(ctx, q, eventID, id)
		}
		if err != nil {
			return err
		}
		if item.Revision != expected {
			return access.ErrConflict
		}
		if item.Kind != input.Kind {
			return blocked(
				"immutable_kind",
				"Content type cannot be changed. The challenge cannot be relabelled as a generic page.",
			)
		}
		d := input.Document
		if err = canonicalSchedule(row, &d, item.Kind); err != nil {
			return err
		}
		if d.Rubric != nil {
			if err = rubric.ValidateReference(ctx, q, eventID, d.Rubric, false); err != nil {
				return fmt.Errorf("validate authored rubric reference: %w", err)
			}
		}
		if err = objectives.ValidateDisclosuresInTransaction(
			ctx,
			tx,
			eventID,
			d.Objectives,
			false,
			false,
			s.now(),
		); err != nil {
			return fmt.Errorf("validate authored objective references: %w", err)
		}
		if err = checkReferences(ctx, q, eventID, id, d); err != nil {
			return err
		}
		if d.Attachments == nil {
			d.Attachments = []uuid.UUID{}
		}
		if d.Audience.Roles == nil {
			d.Audience.Roles = []string{}
		}
		if d.Audience.Teams == nil {
			d.Audience.Teams = []uuid.UUID{}
		}
		encoded, err := json.Marshal(d)
		if err != nil {
			return fmt.Errorf("encode content version: %w", err)
		}
		item.Version++
		item.Revision++
		item.State = "draft"
		item.ApprovedBy = nil
		item.ApprovedRevision = pgtype.Int8{}
		item.ScheduledAt = timestamp(d.ScheduledAt)
		if err = q.InsertContentVersion(
			ctx,
			dbgen.InsertContentVersionParams{
				EventID:   eventID,
				ContentID: id,
				Version:   item.Version,
				Document:  encoded,
				ActorID:   actor,
				CreatedAt: s.now(),
			},
		); err != nil {
			return fmt.Errorf("save content version: %w", err)
		}
		for _, file := range d.Attachments {
			if err = q.AddContentFile(
				ctx,
				dbgen.AddContentFileParams{
					EventID:   eventID,
					ContentID: id,
					Version:   item.Version,
					FileID:    file,
				},
			); err != nil {
				return fmt.Errorf("link attachment: %w", err)
			}
		}
		if err = saveState(ctx, q, item); err != nil {
			return err
		}
		if err = record(ctx, q, item, &actor, "saved", "", s.now()); err != nil {
			return err
		}
		result, err = projection(ctx, q, actor, item, true, 0)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("save content draft: %w", err)
	}
	return result, nil
}

func (s Service) Action(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	expected int64,
	action, reason string,
) (Page, error) {
	var result Page
	if len(reason) > 2000 {
		return result, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := editor(ctx, q, actor, eventID)
		if err != nil {
			return err
		}
		item, err := load(ctx, q, eventID, id)
		if err != nil {
			return err
		}
		if item.Revision != expected {
			return access.ErrConflict
		}
		if action == "release" {
			if item.ScheduledAt.Valid || (item.Kind == "challenge" && !row.PublishedAt.Valid) {
				return blocked(
					"scheduled_only",
					"This content must be published by its approved schedule.",
				)
			}
			if err = s.release(ctx, tx, row, &item, &actor); err != nil {
				return err
			}
		} else {
			if err = s.change(ctx, tx, q, row, &item, actor, action, reason); err != nil {
				return err
			}
		}
		result, err = projection(ctx, q, actor, item, true, 0)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("change content workflow: %w", err)
	}
	return result, nil
}

func (s Service) change(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	row dbgen.Event,
	item *dbgen.ContentItem,
	actor uuid.UUID,
	action, reason string,
) error {
	switch action {
	case "review":
		if item.State != "draft" {
			return access.ErrConflict
		}
		item.State = "review"
	case "approve":
		if item.State != "review" {
			return access.ErrConflict
		}
		d, err := document(ctx, q, *item, item.Version)
		if err != nil {
			return err
		}
		if strings.TrimSpace(d.Markdown) == "" && len(d.Attachments) == 0 {
			return blocked("empty_content", "Add page content or an attachment before approval.")
		}
		if strings.Contains(d.Markdown, "{{") || strings.Contains(d.Title, "{{") {
			return blocked(
				"unresolved_placeholder",
				"Resolve template placeholders before approval.",
			)
		}
		if err = objectives.ValidateDisclosuresInTransaction(
			ctx,
			tx,
			row.ID,
			d.Objectives,
			false,
			false,
			s.now(),
		); err != nil {
			return fmt.Errorf("validate approved objective references: %w", err)
		}
		if err = approveGuidance(d); err != nil {
			return err
		}
		if item.Kind == "challenge" {
			if err = rubric.ValidateReference(ctx, q, row.ID, d.Rubric, true); err != nil {
				return fmt.Errorf("validate approved rubric reference: %w", err)
			}
		}
		if err = canonicalSchedule(row, &d, item.Kind); err != nil {
			return err
		}
		if item.ScheduledAt.Valid && !item.ScheduledAt.Time.After(s.now()) {
			return blocked(
				"missed_schedule",
				"Choose a future release time. A missed challenge requires postponement.",
			)
		}
		if err = s.verifyAttachments(ctx, q, actor, row.ID, d); err != nil {
			return err
		}
		item.State = "approved"
		item.ApprovedBy = &actor
		item.ApprovedRevision = pgtype.Int8{Int64: item.Revision + 1, Valid: true}
	case "cancel":
		if item.State == "released" {
			return blocked(
				"already_released",
				"Released content requires withdrawal; downloaded copies cannot be recalled.",
			)
		}
		item.State = "cancelled"
		item.ApprovedBy = nil
		item.ApprovedRevision = pgtype.Int8{}
	case "withdraw":
		if strings.TrimSpace(reason) == "" {
			return access.ErrInvalid
		}
		if item.Kind == "challenge" && row.PublishedAt.Valid {
			return blocked(
				"challenge_disclosed",
				"The published challenge remains available, including after postponement.",
			)
		}
		item.State = "withdrawn"
		item.ReleasedVersion = pgtype.Int8{}
		item.ApprovedBy = nil
		item.ApprovedRevision = pgtype.Int8{}
	default:
		return access.ErrInvalid
	}
	item.Revision++
	if err := saveState(ctx, q, *item); err != nil {
		return err
	}
	return record(ctx, q, *item, &actor, action, reason, s.now())
}
