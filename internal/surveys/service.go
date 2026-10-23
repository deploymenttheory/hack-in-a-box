package surveys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/objectives"
)

func authorize(ctx context.Context, q *dbgen.Queries, actor, eventID uuid.UUID) error {
	err := (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		access.StaffRead,
	)
	return wrap("authorize survey organiser", err)
}

func load(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, id uuid.UUID,
	version int64,
) (Configuration, error) {
	var out Configuration
	row, err := q.SurveyVersion(
		ctx,
		dbgen.SurveyVersionParams{EventID: eventID, SurveyID: id, Version: version},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, access.ErrNotFound
	}
	if err != nil {
		return out, fmt.Errorf("read survey definition: %w", err)
	}
	if err = json.Unmarshal(row.Document, &out.Definition); err != nil {
		return out, fmt.Errorf("decode survey definition: %w", err)
	}
	out.ID, out.Version, out.AuthorID, out.CreatedAt = id, version, row.ActorID, row.CreatedAt
	if approval, e := q.SurveyApproval(
		ctx,
		dbgen.SurveyApprovalParams{EventID: eventID, SurveyID: id, Version: version},
	); e == nil {
		out.ApprovedAt = &approval.ApprovedAt
	} else if !errors.Is(
		e,
		pgx.ErrNoRows,
	) {
		return out, fmt.Errorf("read survey approval: %w", e)
	}
	if publication, e := q.SurveyPublication(
		ctx,
		dbgen.SurveyPublicationParams{EventID: eventID, SurveyID: id},
	); e == nil &&
		publication.Version == version {
		out.PublishedAt = &publication.PublishedAt
	} else if e != nil &&
		!errors.Is(e, pgx.ErrNoRows) {
		return out, fmt.Errorf("read survey publication: %w", e)
	}
	return out, nil
}

func current(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, id uuid.UUID,
) (dbgen.SurveyHead, Configuration, error) {
	head, err := q.SurveyHead(ctx, dbgen.SurveyHeadParams{EventID: eventID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return head, Configuration{}, access.ErrNotFound
	}
	if err != nil {
		return head, Configuration{}, fmt.Errorf("read survey: %w", err)
	}
	cfg, err := load(ctx, q, eventID, id, head.CurrentVersion)
	cfg.Cancelled = head.Cancelled
	return head, cfg, err
}

func currentRole(p dbgen.CommunicationPeopleRow, role string) bool {
	return (role == "attendee" && p.CompetitionState == "confirmed") ||
		slices.Contains(p.Roles, role)
}

func (s Service) people(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, personID uuid.UUID,
	role string,
) ([]uuid.UUID, error) {
	rows, err := q.CommunicationPeople(
		ctx,
		dbgen.CommunicationPeopleParams{
			EventID:  eventID,
			PersonID: personID,
			At:       s.now(),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("read current survey audience: %w", err)
	}
	if len(rows) > 10000 {
		return nil, access.ErrInvalid
	}
	out := []uuid.UUID{}
	for _, p := range rows {
		if currentRole(p, role) {
			out = append(out, p.ID)
		}
	}
	return out, nil
}

func (s Service) resolveMeasures(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	d Definition,
) error {
	for _, m := range d.Measures {
		if _, err := objectives.ResolveMeasureInTransaction(
			ctx,
			tx,
			actor,
			eventID,
			m.Measure,
		); err != nil {
			return fmt.Errorf("resolve exact survey measure: %w", err)
		}
	}
	return nil
}

func record(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID, id uuid.UUID,
	action audit.Action,
	version int64,
) error {
	reason := audit.OrganiserDecision
	if action == audit.SurveyResponded {
		reason = audit.UserRequest
	}
	err := audit.Append(
		ctx,
		q,
		audit.Record{
			ActorID:  &actor,
			EventID:  &eventID,
			TargetID: id,
			Action:   action,
			Reason:   reason,
			Metadata: audit.Metadata{PreviousRevision: &version},
		},
	)
	return wrap("record survey action", err)
}

func (s Service) Change(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	expected int64,
	c Command,
) (Configuration, error) {
	var out Configuration
	if err := c.validate(id, expected); err != nil {
		return out, err
	}

	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		e, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock survey event: %w", err)
		}
		if err = authorize(ctx, q, actor, eventID); err != nil {
			return err
		}
		if !e.WritesAllowed {
			return access.ErrForbidden
		}
		if err = s.privateCurrent(ctx, q, e); err != nil {
			return err
		}
		head, cfg, err := current(ctx, q, eventID, id)
		if errors.Is(err, access.ErrNotFound) && c.Action == "save" && expected == 0 {
			head = dbgen.SurveyHead{EventID: eventID, ID: id}
			err = nil
		}
		if err != nil {
			return err
		}
		if head.CurrentVersion != expected {
			return access.ErrConflict
		}
		if c.Action == "save" {
			if err = s.saveVersion(ctx, tx, q, actor, e, id, &head, c.Definition); err != nil {
				return err
			}
		} else {
			if head.Cancelled {
				return access.ErrConflict
			}
			switch c.Action {
			case "approve":
				if head.PublishedVersion.Valid || !cfg.Definition.ClosesAt.After(s.now()) {
					return access.ErrConflict
				}
				if err = s.resolveMeasures(ctx, tx, actor, eventID, cfg.Definition); err != nil {
					return err
				}
				if err = q.ApproveSurvey(
					ctx,
					dbgen.ApproveSurveyParams{
						EventID:    eventID,
						SurveyID:   id,
						Version:    expected,
						ActorID:    actor,
						ApprovedAt: s.now(),
					},
				); err != nil {
					return fmt.Errorf("approve survey version: %w", err)
				}
			case "publish":
				if head.PublishedVersion.Valid {
					return access.ErrConflict
				}
				if err = s.publishVersion(
					ctx,
					tx,
					q,
					actor,
					eventID,
					id,
					expected,
					cfg,
				); err != nil {
					return err
				}
				head.PublishedVersion = pgtype.Int8{Int64: expected, Valid: true}
			case "cancel":
				head.Cancelled = true
			}
		}
		if err = q.UpdateSurveyHead(
			ctx,
			dbgen.UpdateSurveyHeadParams{
				EventID:          eventID,
				ID:               id,
				CurrentVersion:   head.CurrentVersion,
				PublishedVersion: head.PublishedVersion,
				Cancelled:        head.Cancelled,
			},
		); err != nil {
			return fmt.Errorf("retain current survey configuration: %w", err)
		}
		if err = record(ctx, q, actor, eventID, id, audit.SurveyChanged, expected); err != nil {
			return err
		}
		out, err = load(ctx, q, eventID, id, head.CurrentVersion)
		out.Cancelled = head.Cancelled
		return err
	})
	if err != nil {
		return out, fmt.Errorf("change survey: %w", err)
	}
	return out, nil
}

func (s Service) saveVersion(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	actor uuid.UUID,
	e dbgen.Event,
	id uuid.UUID,
	head *dbgen.SurveyHead,
	definition *Definition,
) error {
	eventID := e.ID
	var err error
	if head.PublishedVersion.Valid || head.Cancelled {
		return access.ErrConflict
	}
	if !definition.ClosesAt.After(s.now()) {
		return access.ErrInvalid
	}
	var dates event.Configuration
	if err = json.Unmarshal(e.Configuration, &dates); err != nil {
		return fmt.Errorf("read survey event dates: %w", err)
	}
	if definition.OpensAt.Before(dates.LiveEnd) {
		return access.ErrInvalid
	}
	if err = s.resolveMeasures(ctx, tx, actor, eventID, *definition); err != nil {
		return err
	}
	version := head.CurrentVersion + 1
	if head.CurrentVersion == 0 {
		if err = q.InsertSurveyHead(
			ctx,
			dbgen.InsertSurveyHeadParams{EventID: eventID, ID: id},
		); err != nil {
			return fmt.Errorf("create survey: %w", err)
		}
	}
	normalized := *definition
	normalized.Form.Fields = append([]forms.Field{}, definition.Form.Fields...)
	for i := range normalized.Form.Fields {
		if normalized.Form.Fields[i].Choices == nil {
			normalized.Form.Fields[i].Choices = []string{}
		}
	}
	if normalized.Measures == nil {
		normalized.Measures = []QuestionMeasure{}
	}
	body, encodeErr := json.Marshal(normalized)
	if encodeErr != nil {
		return fmt.Errorf("encode survey definition: %w", encodeErr)
	}
	if err = q.InsertSurveyVersion(
		ctx,
		dbgen.InsertSurveyVersionParams{
			EventID:   eventID,
			SurveyID:  id,
			Version:   version,
			Document:  body,
			ActorID:   actor,
			CreatedAt: s.now(),
		},
	); err != nil {
		return fmt.Errorf("retain survey version: %w", err)
	}
	head.CurrentVersion = version
	return nil
}

func (s Service) publishVersion(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	actor, eventID, id uuid.UUID,
	expected int64,
	cfg Configuration,
) error {
	var err error
	if !cfg.Definition.ClosesAt.After(s.now()) {
		return access.ErrConflict
	}
	approval, readErr := q.SurveyApproval(
		ctx,
		dbgen.SurveyApprovalParams{EventID: eventID, SurveyID: id, Version: expected},
	)
	if readErr != nil {
		return access.ErrConflict
	}
	if err = authorize(ctx, q, approval.ActorID, eventID); err != nil {
		return err
	}
	if err = s.resolveMeasures(ctx, tx, actor, eventID, cfg.Definition); err != nil {
		return err
	}
	people, readErr := s.people(ctx, q, eventID, uuid.Nil, cfg.Definition.Form.Audience)
	if readErr != nil {
		return readErr
	}
	if err = q.PublishSurvey(
		ctx,
		dbgen.PublishSurveyParams{
			EventID:     eventID,
			SurveyID:    id,
			Version:     expected,
			ActorID:     actor,
			PublishedAt: s.now(),
		},
	); err != nil {
		return fmt.Errorf("publish survey: %w", err)
	}
	for _, person := range people {
		if err = q.InviteSurveyPerson(
			ctx,
			dbgen.InviteSurveyPersonParams{
				EventID:   eventID,
				SurveyID:  id,
				PersonID:  person,
				Version:   expected,
				InvitedAt: s.now(),
			},
		); err != nil {
			return fmt.Errorf("retain survey invitation: %w", err)
		}
	}
	return nil
}

func wrap(op string, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

func (c Command) validate(id uuid.UUID, expected int64) error {
	if id == uuid.Nil || expected < 0 ||
		!slices.Contains([]string{"save", "approve", "publish", "cancel"}, c.Action) ||
		(c.Action == "save") != (c.Definition != nil) {
		return access.ErrInvalid
	}
	if c.Definition != nil {
		if err := c.Definition.Validate(); err != nil {
			return err
		}
		if c.Definition.Form.ID != id {
			return access.ErrInvalid
		}
	}
	return nil
}
