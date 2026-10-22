package rubric

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
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
		Pool  *pgxpool.Pool
		Clock func() time.Time
	}
	Run struct {
		ID         uuid.UUID  `json:"id"`
		Reference  Reference  `json:"reference"`
		Purpose    string     `json:"purpose"`
		StartedAt  *time.Time `json:"started_at"`
		PreviousID *uuid.UUID `json:"previous_id"`
	}
	Configuration struct {
		Revision         int64      `json:"revision"`
		Reference        Reference  `json:"reference"`
		Definition       Definition `json:"definition"`
		Approved         bool       `json:"approved"`
		PublishedVersion *int64     `json:"published_version"`
		Runs             []Run      `json:"runs"`
	}
)

type Command struct {
	Action        string      `json:"action"`
	Version       int64       `json:"version"`
	Definition    *Definition `json:"definition"`
	Reason        string      `json:"reason"`
	ImpactReview  string      `json:"impact_review"`
	Notice        string      `json:"notice"`
	Purpose       string      `json:"purpose"`
	PreviousRunID *uuid.UUID  `json:"previous_run_id"`
}

func (s Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

func blocked(code, message string) error {
	return &event.Blocked{Issues: []event.Issue{{Field: "rubric", Code: code, Message: message}}}
}

func authorize(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID uuid.UUID,
	write bool,
) (dbgen.Event, error) {
	row, err := q.LockEvent(ctx, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, access.ErrNotFound
	}
	if err != nil {
		return row, fmt.Errorf("lock rubric event: %w", err)
	}
	roles, _, err := access.ContentReader(ctx, q, actor, eventID)
	if err != nil {
		return row, fmt.Errorf("authorize rubric organiser: %w", err)
	}
	if !slices.Contains(roles, "organiser") {
		return row, access.ErrForbidden
	}
	if write {
		if err = (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.EventEdit,
		); err != nil {
			return row, fmt.Errorf("authorize rubric edit: %w", err)
		}
	}
	return row, nil
}

func loadDefinition(
	ctx context.Context,
	q *dbgen.Queries,
	h dbgen.EventRubric,
	version int64,
) (Definition, dbgen.RubricVersion, error) {
	row, err := q.RubricVersion(
		ctx,
		dbgen.RubricVersionParams{EventID: h.EventID, RubricID: h.ID, Version: version},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Definition{}, row, access.ErrNotFound
	}
	if err != nil {
		return Definition{}, row, fmt.Errorf("load rubric snapshot: %w", err)
	}
	var d Definition
	if err = json.Unmarshal(row.Document, &d); err != nil {
		return d, row, fmt.Errorf("decode rubric snapshot: %w", err)
	}
	if d.Validate() != nil || d.PublicDigest() != row.PublicDigest {
		return d, row, blocked("invalid_snapshot", "The retained rubric snapshot is invalid.")
	}
	return d, row, nil
}

func page(
	ctx context.Context,
	q *dbgen.Queries,
	h dbgen.EventRubric,
	version int64,
) (Configuration, error) {
	if version == 0 {
		version = h.LatestVersion
	}
	d, row, err := loadDefinition(ctx, q, h, version)
	if err != nil {
		return Configuration{}, err
	}
	out := Configuration{
		Revision:   h.Revision,
		Reference:  Reference{ID: h.ID, Version: version, Digest: row.PublicDigest},
		Definition: d,
		Approved:   h.ApprovedVersion.Valid && h.ApprovedVersion.Int64 == version,
		Runs:       []Run{},
	}
	if h.PublishedVersion.Valid {
		published := h.PublishedVersion.Int64
		out.PublishedVersion = &published
	}
	runs, err := q.CurrentRubricRuns(ctx, h.EventID)
	if err != nil {
		return out, fmt.Errorf("read rubric runs: %w", err)
	}
	for _, run := range runs {
		_, version, err := loadDefinition(ctx, q, h, run.RubricVersion)
		if err != nil {
			return out, err
		}
		item := Run{
			ID: run.ID,
			Reference: Reference{
				ID:      h.ID,
				Version: run.RubricVersion,
				Digest:  version.PublicDigest,
			},
			Purpose:    run.Purpose,
			PreviousID: run.PreviousID,
		}
		if run.StartedAt.Valid {
			at := run.StartedAt.Time
			item.StartedAt = &at
		}
		out.Runs = append(out.Runs, item)
	}
	return out, nil
}

func (s Service) Configuration(
	ctx context.Context,
	actor, eventID uuid.UUID,
	version int64,
) (Configuration, error) {
	var out Configuration
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, err := authorize(ctx, q, actor, eventID, false); err != nil {
			return err
		}
		h, err := q.EventRubric(ctx, eventID)
		if errors.Is(err, pgx.ErrNoRows) && version == 0 {
			out = Configuration{Definition: Default(AllTeams, eventID), Runs: []Run{}}
			return nil
		}
		if err != nil {
			return fmt.Errorf("read rubric configuration: %w", err)
		}
		out, err = page(ctx, q, h, version)
		return err
	})
	if err != nil {
		return out, fmt.Errorf("rubric configuration: %w", err)
	}
	return out, nil
}

func saveHeader(
	ctx context.Context,
	q *dbgen.Queries,
	h dbgen.EventRubric,
) (dbgen.EventRubric, error) {
	row, err := q.SaveRubricHeader(
		ctx,
		dbgen.SaveRubricHeaderParams{
			EventID:          h.EventID,
			Revision:         h.Revision,
			LatestVersion:    h.LatestVersion,
			ApprovedVersion:  h.ApprovedVersion,
			PublishedVersion: h.PublishedVersion,
		},
	)
	if err != nil {
		return row, fmt.Errorf("save rubric state: %w", err)
	}
	return row, nil
}

func record(
	ctx context.Context,
	q *dbgen.Queries,
	h *dbgen.EventRubric,
	actor *uuid.UUID,
	version int64,
	action, reason string,
	at time.Time,
) error {
	h.Revision++
	if _, err := saveHeader(ctx, q, *h); err != nil {
		return err
	}
	if err := q.RecordRubricChange(
		ctx,
		dbgen.RecordRubricChangeParams{
			EventID:    h.EventID,
			RubricID:   h.ID,
			Revision:   h.Revision,
			Version:    version,
			Action:     action,
			ActorID:    actor,
			Reason:     reason,
			OccurredAt: at,
		},
	); err != nil {
		return fmt.Errorf("record rubric version decision: %w", err)
	}
	if _, err := q.BumpEventRevision(ctx, h.EventID); err != nil {
		return fmt.Errorf("record rubric event change: %w", err)
	}
	if err := audit.Append(
		ctx,
		q,
		audit.Record{
			ActorID:  actor,
			EventID:  &h.EventID,
			TargetID: h.ID,
			Action:   audit.RubricChanged,
			Reason:   audit.OrganiserDecision,
			Metadata: audit.Metadata{Revision: &h.Revision},
		},
	); err != nil {
		return fmt.Errorf("audit rubric version decision: %w", err)
	}
	return nil
}

func (s Service) Change(
	ctx context.Context,
	actor, eventID uuid.UUID,
	expected int64,
	input Command,
) (Configuration, error) {
	if expected < 0 || strings.TrimSpace(input.Reason) == "" || len(input.Reason) > 2000 {
		return Configuration{}, access.ErrInvalid
	}
	if !slices.Contains(
		[]string{"save", "approve", "review_impact", "create_run", "restart_run"},
		input.Action,
	) {
		return Configuration{}, access.ErrInvalid
	}
	if input.Action == "save" && (input.Definition == nil || input.Definition.Validate() != nil) {
		return Configuration{}, access.ErrInvalid
	}
	var out Configuration
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := authorize(ctx, q, actor, eventID, true)
		if err != nil {
			return err
		}
		h, err := q.EventRubric(ctx, eventID)
		if errors.Is(err, pgx.ErrNoRows) && expected == 0 && input.Action == "save" {
			h = dbgen.EventRubric{EventID: eventID, ID: uuid.New()}
			if err = q.CreateEventRubric(
				ctx,
				dbgen.CreateEventRubricParams{EventID: eventID, ID: h.ID},
			); err != nil {
				return fmt.Errorf("create rubric: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("read rubric state: %w", err)
		}
		if h.Revision != expected {
			return access.ErrConflict
		}
		switch input.Action {
		case "save":
			err = s.save(ctx, q, &h, actor, *input.Definition, input.Reason)
		case "approve":
			err = s.approve(ctx, q, &h, actor, input.Version, input.Reason)
		case "review_impact":
			err = s.reviewImpact(ctx, q, row, &h, actor, input)
		case "create_run", "restart_run":
			err = s.changeRun(ctx, q, &h, actor, input)
		}
		if err != nil {
			return err
		}
		out, err = page(ctx, q, h, 0)
		return err
	})
	if err != nil {
		return out, fmt.Errorf("change rubric: %w", err)
	}
	return out, nil
}

func (s Service) save(
	ctx context.Context,
	q *dbgen.Queries,
	h *dbgen.EventRubric,
	actor uuid.UUID,
	d Definition,
	reason string,
) error {
	private, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("encode rubric definition: %w", err)
	}
	public, err := json.Marshal(d.PublicDefinition)
	if err != nil {
		return fmt.Errorf("encode common rubric: %w", err)
	}
	h.LatestVersion++
	if err = q.InsertRubricVersion(
		ctx,
		dbgen.InsertRubricVersionParams{
			EventID:        h.EventID,
			RubricID:       h.ID,
			Version:        h.LatestVersion,
			Document:       private,
			PublicDocument: public,
			PublicDigest:   d.PublicDigest(),
			ActorID:        actor,
			CreatedAt:      s.now(),
		},
	); err != nil {
		return fmt.Errorf("retain rubric version: %w", err)
	}
	return record(ctx, q, h, &actor, h.LatestVersion, "saved", reason, s.now())
}

func (s Service) approve(
	ctx context.Context,
	q *dbgen.Queries,
	h *dbgen.EventRubric,
	actor uuid.UUID,
	version int64,
	reason string,
) error {
	if version != h.LatestVersion || version < 1 {
		return access.ErrConflict
	}
	if _, _, err := loadDefinition(ctx, q, *h, version); err != nil {
		return err
	}
	if _, err := q.RubricApproval(
		ctx,
		dbgen.RubricApprovalParams{EventID: h.EventID, RubricID: h.ID, Version: version},
	); err == nil {
		return access.ErrConflict
	} else if !errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return fmt.Errorf("read rubric approval: %w", err)
	}
	if err := q.ApproveRubricVersion(
		ctx,
		dbgen.ApproveRubricVersionParams{
			EventID:    h.EventID,
			RubricID:   h.ID,
			Version:    version,
			ActorID:    actor,
			Reason:     reason,
			ApprovedAt: s.now(),
		},
	); err != nil {
		return fmt.Errorf("approve rubric snapshot: %w", err)
	}
	h.ApprovedVersion = pgtype.Int8{Int64: version, Valid: true}
	return record(ctx, q, h, &actor, version, "approved", reason, s.now())
}
