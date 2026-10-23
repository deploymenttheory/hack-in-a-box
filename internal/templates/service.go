package templates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/objectives"
)

type Service struct{ Pool *pgxpool.Pool }

func authorize(ctx context.Context, q *dbgen.Queries, actor uuid.UUID, row dbgen.Template) error {
	person, err := q.Person(ctx, actor)
	if err != nil || !person.Active || row.OwnerID != actor {
		return access.ErrForbidden
	}
	return nil
}

func view(ctx context.Context, q *dbgen.Queries, row dbgen.Template, version int32) (View, error) {
	if version == 0 {
		version = row.LatestVersion
	}
	snapshot, err := q.TemplateVersion(
		ctx,
		dbgen.TemplateVersionParams{TemplateID: row.ID, Version: version},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, access.ErrNotFound
	}
	if err != nil {
		return View{}, fmt.Errorf("read immutable template version: %w", err)
	}
	var bundle Bundle
	if err := json.Unmarshal(snapshot.Configuration, &bundle); err != nil {
		return View{}, fmt.Errorf("decode template snapshot: %w", err)
	}
	result := View{
		ID:            row.ID,
		Name:          row.Name,
		Revision:      row.Revision,
		LatestVersion: row.LatestVersion,
		Version:       version,
		Configuration: bundle,
	}
	if row.PublishedVersion.Valid {
		v := row.PublishedVersion.Int32
		result.PublishedVersion = &v
	}
	return result, nil
}

func templateAudit(
	ctx context.Context,
	q *dbgen.Queries,
	actor, id uuid.UUID,
	action audit.Action,
) error {
	if err := audit.Append(
		ctx,
		q,
		audit.Record{
			ActorID:  &actor,
			TargetID: id,
			Action:   action,
			Reason:   audit.OrganiserDecision,
		},
	); err != nil {
		return fmt.Errorf("record template audit: %w", err)
	}
	return nil
}

func (s Service) Create(ctx context.Context, actor uuid.UUID, input Input) (View, error) {
	var result View
	if input.ID == uuid.Nil || strings.TrimSpace(input.Name) == "" || len(input.Name) > 160 {
		return result, access.ErrInvalid
	}
	if err := Validate(input.Configuration); err != nil {
		return result, err
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		allowed, err := q.CanCreateEvent(ctx, actor)
		if err != nil {
			return fmt.Errorf("check template creation: %w", err)
		}
		if !allowed {
			return access.ErrForbidden
		}
		if err := q.LockCreationID(ctx, input.ID.String()); err != nil {
			return fmt.Errorf("lock template identity: %w", err)
		}
		data, err := json.Marshal(input.Configuration)
		if err != nil {
			return fmt.Errorf("encode template snapshot: %w", err)
		}
		existing, err := q.Template(ctx, input.ID)
		if err == nil {
			if err := authorize(ctx, q, actor, existing); err != nil {
				return err
			}
			initial, err := q.TemplateVersion(
				ctx,
				dbgen.TemplateVersionParams{TemplateID: input.ID, Version: 1},
			)
			if err != nil {
				return fmt.Errorf("load initial snapshot: %w", err)
			}
			var original Bundle
			if err := json.Unmarshal(initial.Configuration, &original); err != nil {
				return fmt.Errorf("decode initial snapshot: %w", err)
			}
			canonical, _ := json.Marshal(original)
			if existing.Name != input.Name || string(canonical) != string(data) {
				return access.ErrConflict
			}
			result, err = view(ctx, q, existing, 1)
			return err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("check existing template: %w", err)
		}
		row, err := q.CreateTemplate(
			ctx,
			dbgen.CreateTemplateParams{ID: input.ID, OwnerID: actor, Name: input.Name},
		)
		if err != nil {
			return fmt.Errorf("create template: %w", err)
		}
		if err := q.NewTemplateVersion(
			ctx,
			dbgen.NewTemplateVersionParams{TemplateID: row.ID, Version: 1, Configuration: data},
		); err != nil {
			return fmt.Errorf("save template version: %w", err)
		}
		if err := templateAudit(ctx, q, actor, row.ID, audit.TemplateCreated); err != nil {
			return err
		}
		result, err = view(ctx, q, row, 1)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("create reusable template: %w", err)
	}
	return result, nil
}

func (s Service) Read(ctx context.Context, actor, id uuid.UUID, version int32) (View, error) {
	q := dbgen.New(s.Pool)
	row, err := q.Template(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, access.ErrNotFound
	}
	if err != nil {
		return View{}, fmt.Errorf("load template: %w", err)
	}
	if err := authorize(ctx, q, actor, row); err != nil {
		return View{}, err
	}
	return view(ctx, q, row, version)
}

func (s Service) List(ctx context.Context, actor, cursor uuid.UUID, limit int32) ([]View, error) {
	if limit < 1 || limit > 50 {
		return nil, access.ErrInvalid
	}
	q := dbgen.New(s.Pool)
	p, err := q.Person(ctx, actor)
	if err != nil || !p.Active {
		return nil, access.ErrForbidden
	}
	rows, err := q.ListTemplates(
		ctx,
		dbgen.ListTemplatesParams{OwnerID: actor, ID: cursor, Limit: limit},
	)
	if err != nil {
		return nil, fmt.Errorf("list owned templates: %w", err)
	}
	result := make([]View, 0, len(rows))
	for _, row := range rows {
		v, err := view(ctx, q, row, 0)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, nil
}

func (s Service) Edit(
	ctx context.Context,
	actor, id uuid.UUID,
	revision int64,
	input Input,
) (View, error) {
	var result View
	if strings.TrimSpace(input.Name) == "" || len(input.Name) > 160 {
		return result, access.ErrInvalid
	}
	if err := Validate(input.Configuration); err != nil {
		return result, err
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockTemplate(ctx, id)
		if err != nil {
			return fmt.Errorf("lock template: %w", err)
		}
		if err := authorize(ctx, q, actor, row); err != nil {
			return err
		}
		if row.Revision != revision {
			return access.ErrConflict
		}
		if row.LatestVersion >= 1000 {
			return access.ErrInvalid
		}
		data, err := json.Marshal(input.Configuration)
		if err != nil {
			return fmt.Errorf("encode new version: %w", err)
		}
		row, err = q.EditTemplate(ctx, dbgen.EditTemplateParams{ID: id, Name: input.Name})
		if err != nil {
			return fmt.Errorf("advance template version: %w", err)
		}
		if err := q.NewTemplateVersion(
			ctx,
			dbgen.NewTemplateVersionParams{
				TemplateID:    id,
				Version:       row.LatestVersion,
				Configuration: data,
			},
		); err != nil {
			return fmt.Errorf("append template version: %w", err)
		}
		if err := templateAudit(ctx, q, actor, id, audit.TemplateUpdated); err != nil {
			return err
		}
		result, err = view(ctx, q, row, 0)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("edit template: %w", err)
	}
	return result, nil
}

func (s Service) Publish(
	ctx context.Context,
	actor, id uuid.UUID,
	revision int64,
	version int32,
) (View, error) {
	var result View
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockTemplate(ctx, id)
		if err != nil {
			return fmt.Errorf("lock template publication: %w", err)
		}
		if err := authorize(ctx, q, actor, row); err != nil {
			return err
		}
		if row.Revision != revision {
			return access.ErrConflict
		}
		if _, err := q.TemplateVersion(
			ctx,
			dbgen.TemplateVersionParams{TemplateID: id, Version: version},
		); err != nil {
			return access.ErrNotFound
		}
		if row.PublishedVersion.Valid && row.PublishedVersion.Int32 == version {
			result, err = view(ctx, q, row, version)
			return err
		}
		if err := q.RecordTemplatePublication(
			ctx,
			dbgen.RecordTemplatePublicationParams{TemplateID: id, Version: version},
		); err != nil {
			return fmt.Errorf("record version publication: %w", err)
		}
		row, err = q.PublishTemplate(
			ctx,
			dbgen.PublishTemplateParams{
				ID:               id,
				PublishedVersion: pgtype.Int4{Int32: version, Valid: true},
			},
		)
		if err != nil {
			return fmt.Errorf("publish template version: %w", err)
		}
		if err := templateAudit(ctx, q, actor, id, audit.TemplatePublished); err != nil {
			return err
		}
		result, err = view(ctx, q, row, version)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("publish reusable template: %w", err)
	}
	return result, nil
}

func snapshot(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	actor, id uuid.UUID,
) (Bundle, error) {
	if err := (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		id,
		id,
		access.StaffRead,
	); err != nil {
		return Bundle{}, fmt.Errorf("authorize configuration snapshot: %w", err)
	}
	row, err := q.Event(ctx, id)
	if err != nil {
		return Bundle{}, fmt.Errorf("load snapshot event: %w", err)
	}
	result := Bundle{Event: event.DefaultConfiguration(), Definitions: EmptyDefinitions()}
	if err := json.Unmarshal(row.Configuration, &result.Event); err != nil {
		return result, fmt.Errorf("decode snapshot configuration: %w", err)
	}
	definitions, err := q.EventDefinitions(ctx, id)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return result, fmt.Errorf("load definition snapshot: %w", err)
	}
	if err == nil {
		if err = json.Unmarshal(definitions.Definitions, &result.Definitions); err != nil {
			return result, fmt.Errorf("decode definitions: %w", err)
		}
	}
	result.Definitions.Objectives, err = objectives.SnapshotInTransaction(ctx, tx, actor, id)
	if err != nil {
		return result, fmt.Errorf("snapshot native objectives: %w", err)
	}
	return result, nil
}

func (s Service) Snapshot(ctx context.Context, actor, id uuid.UUID) (Bundle, error) {
	var result Bundle
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		var err error
		result, err = snapshot(ctx, tx, q, actor, id)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("snapshot event configuration: %w", err)
	}
	return result, nil
}

func (s Service) SaveDefinitions(
	ctx context.Context,
	actor, id uuid.UUID,
	revision int64,
	definitions Definitions,
) (Bundle, error) {
	var result Bundle
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, id)
		if err != nil {
			return fmt.Errorf("lock definitions: %w", err)
		}
		if err := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			id,
			id,
			access.EventEdit,
		); err != nil {
			return fmt.Errorf("authorize definition edit: %w", err)
		}
		if row.Revision != revision {
			return access.ErrConflict
		}
		result, err = snapshot(ctx, tx, q, actor, id)
		if err != nil {
			return err
		}
		if row.PublishedAt.Valid &&
			!reflect.DeepEqual(result.Definitions.Deliverables, definitions.Deliverables) {
			return &event.Blocked{
				Issues: []event.Issue{
					{
						Field:   "deliverables",
						Code:    "published_requirements",
						Message: "Competition requirements are fixed once the challenge is published.",
					},
				},
			}
		}
		if definitions.Objectives == nil {
			definitions.Objectives = result.Definitions.Objectives
		}
		if !reflect.DeepEqual(definitions.Objectives, result.Definitions.Objectives) {
			return access.ErrInvalid
		}
		result.Definitions = definitions
		if err := Validate(result); err != nil {
			return err
		}
		data, err := json.Marshal(definitions)
		if err != nil {
			return fmt.Errorf("encode definitions: %w", err)
		}
		if err := q.SaveEventDefinitions(
			ctx,
			dbgen.SaveEventDefinitionsParams{EventID: id, Definitions: data},
		); err != nil {
			return fmt.Errorf("save definition snapshot: %w", err)
		}
		if _, err := q.BumpEventRevision(ctx, id); err != nil {
			return fmt.Errorf("advance definition revision: %w", err)
		}
		return audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &id,
				TargetID: id,
				Action:   audit.DefinitionsUpdated,
				Reason:   audit.OrganiserDecision,
			},
		)
	})
	if err != nil {
		return result, fmt.Errorf("update definitions: %w", err)
	}
	return result, nil
}

func (s Service) CloneTemplate(
	ctx context.Context,
	actor, id uuid.UUID,
	input CloneInput,
) (event.View, error) {
	return s.clone(ctx, actor, id, input, true)
}

func (s Service) CloneEvent(
	ctx context.Context,
	actor, id uuid.UUID,
	input CloneInput,
) (event.View, error) {
	return s.clone(ctx, actor, id, input, false)
}

func (s Service) clone(
	ctx context.Context,
	actor, source uuid.UUID,
	input CloneInput,
	fromTemplate bool,
) (event.View, error) {
	var result event.View
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		bundle, err := s.cloneSource(ctx, tx, q, actor, source, input.Version, fromTemplate)
		if err != nil {
			return err
		}
		identity, err := json.Marshal(struct {
			Actor    uuid.UUID  `json:"actor"`
			Source   uuid.UUID  `json:"source"`
			Input    CloneInput `json:"input"`
			Template bool       `json:"template"`
			Bundle   Bundle     `json:"bundle"`
		}{actor, source, input, fromTemplate, bundle})
		if err != nil {
			return fmt.Errorf("encode clone identity: %w", err)
		}
		hash := sha256.Sum256(identity)
		key := hex.EncodeToString(hash[:])
		if err := q.LockCreationID(ctx, input.ID.String()); err != nil {
			return fmt.Errorf("lock clone identity: %w", err)
		}
		existing, err := q.EventDefinitions(ctx, input.ID)
		if err == nil {
			if existing.CloneKey != key {
				return access.ErrConflict
			}
			result, err = (event.Service{Pool: s.Pool}).Read(ctx, actor, input.ID)
			if err != nil {
				return fmt.Errorf("read existing clone: %w", err)
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("check existing clone: %w", err)
		}
		if _, err := q.Event(ctx, input.ID); err == nil {
			return access.ErrConflict
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("check new event identity: %w", err)
		}
		cloned, err := Remap(bundle, input)
		if err != nil {
			return err
		}
		result, err = (event.Service{Pool: s.Pool}).CreateIn(
			ctx,
			tx,
			actor,
			event.Input{
				ID:                                input.ID,
				Name:                              input.Name,
				Slug:                              input.Slug,
				Configuration:                     cloned.Event,
				AcknowledgeCompetitionRestriction: input.AcknowledgeCompetitionRestriction,
			},
		)
		if err != nil {
			return fmt.Errorf("create cloned event: %w", err)
		}
		data, err := json.Marshal(cloned.Definitions)
		if err != nil {
			return fmt.Errorf("encode independent definitions: %w", err)
		}
		params := dbgen.SaveEventDefinitionsParams{
			EventID:     result.ID,
			Definitions: data,
			CloneKey:    key,
		}
		if fromTemplate {
			params.SourceTemplateID = &source
			params.SourceVersion = pgtype.Int4{Int32: input.Version, Valid: true}
		}
		if err := q.SaveEventDefinitions(ctx, params); err != nil {
			return fmt.Errorf("commit independent snapshot: %w", err)
		}
		if err := (objectives.Service{Pool: s.Pool}).ImportInTransaction(
			ctx,
			tx,
			actor,
			result.ID,
			cloned.Definitions.Objectives,
		); err != nil {
			return fmt.Errorf("materialise independent objective configuration: %w", err)
		}
		return audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &result.ID,
				TargetID: result.ID,
				Action:   audit.EventCloned,
				Reason:   audit.OrganiserDecision,
				Metadata: audit.Metadata{ReferenceID: &source},
			},
		)
	})
	if err != nil {
		return result, fmt.Errorf("clone reusable configuration: %w", err)
	}
	return result, nil
}

func (s Service) cloneSource(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	actor, source uuid.UUID,
	version int32,
	fromTemplate bool,
) (Bundle, error) {
	if !fromTemplate {
		if _, err := q.LockEvent(ctx, source); err != nil {
			return Bundle{}, fmt.Errorf("lock source snapshot: %w", err)
		}
		return snapshot(ctx, tx, q, actor, source)
	}
	row, err := q.LockTemplate(ctx, source)
	if err != nil {
		return Bundle{}, fmt.Errorf("lock source template: %w", err)
	}
	if err := authorize(ctx, q, actor, row); err != nil {
		return Bundle{}, err
	}
	published, err := q.TemplatePublished(
		ctx,
		dbgen.TemplatePublishedParams{TemplateID: source, Version: version},
	)
	if err != nil {
		return Bundle{}, fmt.Errorf("check published version: %w", err)
	}
	if !published {
		return Bundle{}, access.ErrInvalid
	}
	v, err := view(ctx, q, row, version)
	if err != nil {
		return Bundle{}, err
	}
	return v.Configuration, nil
}
