// Package registration owns versioned applications, attendee places and offers.
package registration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
)

type Service struct {
	Pool *pgxpool.Pool
	Now  func() time.Time
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

type FormView struct {
	Definition       forms.Definition `json:"definition"`
	Revision         int64            `json:"revision"`
	Version          int32            `json:"version"`
	PublishedVersion *int32           `json:"published_version"`
}

func formView(ctx context.Context, q *dbgen.Queries, row dbgen.RegistrationForm) (FormView, error) {
	result := FormView{Revision: row.Revision, Version: row.LatestVersion}
	v, err := q.RegistrationFormVersion(
		ctx,
		dbgen.RegistrationFormVersionParams{
			EventID: row.EventID,
			FormID:  row.ID,
			Version: row.LatestVersion,
		},
	)
	if err != nil {
		return result, fmt.Errorf("read form version: %w", err)
	}
	if err := json.Unmarshal(v.Definition, &result.Definition); err != nil {
		return result, fmt.Errorf("decode form version: %w", err)
	}
	if row.PublishedVersion.Valid {
		result.PublishedVersion = new(row.PublishedVersion.Int32)
	}
	return result, nil
}

func (s Service) Forms(ctx context.Context, actor, eventID uuid.UUID) ([]FormView, error) {
	q := dbgen.New(s.Pool)
	if err := (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		access.StaffRead,
	); err != nil {
		return nil, fmt.Errorf("authorize forms: %w", err)
	}
	rows, err := q.RegistrationForms(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("list forms: %w", err)
	}
	result := make([]FormView, 0, len(rows))
	seen := map[uuid.UUID]bool{}
	for _, row := range rows {
		v, err := formView(ctx, q, row)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
		seen[row.ID] = true
	}
	// Cloned definitions are editable drafts; importing them does not publish them.
	snapshot, err := (templates.Service{Pool: s.Pool}).Snapshot(ctx, actor, eventID)
	if err != nil {
		return nil, fmt.Errorf("read cloned form drafts: %w", err)
	}
	for _, d := range snapshot.Definitions.Forms {
		if !seen[d.ID] {
			result = append(result, FormView{Definition: d})
		}
	}
	return result, nil
}

func (s Service) SaveForm(
	ctx context.Context,
	actor, eventID uuid.UUID,
	revision int64,
	d forms.Definition,
) (FormView, error) {
	var result FormView
	if err := forms.Validate(d); err != nil {
		return result, fmt.Errorf("validate registration form: %w", err)
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(ctx, eventID); err != nil {
			return fmt.Errorf("lock form event: %w", err)
		}
		if err := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.EventEdit,
		); err != nil {
			return fmt.Errorf("authorize form edit: %w", err)
		}
		row, err := q.RegistrationForm(
			ctx,
			dbgen.RegistrationFormParams{EventID: eventID, ID: d.ID},
		)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			if revision != 0 {
				return access.ErrConflict
			}
			rows, err := q.RegistrationForms(ctx, eventID)
			if err != nil {
				return fmt.Errorf("count forms: %w", err)
			}
			if len(rows) >= 50 {
				return access.ErrInvalid
			}
			row, err = q.NewRegistrationForm(
				ctx,
				dbgen.NewRegistrationFormParams{EventID: eventID, ID: d.ID},
			)
			if err != nil {
				return fmt.Errorf("create form: %w", err)
			}
		case err != nil:
			return fmt.Errorf("read editable form: %w", err)
		default:
			if row.Revision != revision {
				return access.ErrConflict
			}
			if row.LatestVersion >= 1000 {
				return access.ErrInvalid
			}
			old, err := formView(ctx, q, row)
			if err != nil {
				return err
			}
			if row.PublishedVersion.Valid && old.Definition.Audience != d.Audience {
				return &forms.Invalid{
					Issues: []forms.Issue{
						{
							Field:   "audience",
							Code:    "audience_locked",
							Message: "Create a separate form to change the audience after publication.",
						},
					},
				}
			}
			row, err = q.AdvanceRegistrationForm(
				ctx,
				dbgen.AdvanceRegistrationFormParams{EventID: eventID, ID: d.ID},
			)
			if err != nil {
				return fmt.Errorf("advance form: %w", err)
			}
		}
		data, err := json.Marshal(d)
		if err != nil {
			return fmt.Errorf("encode form: %w", err)
		}
		if err := q.NewRegistrationFormVersion(
			ctx,
			dbgen.NewRegistrationFormVersionParams{
				EventID:    eventID,
				FormID:     d.ID,
				Version:    row.LatestVersion,
				Definition: data,
			},
		); err != nil {
			return fmt.Errorf("save form version: %w", err)
		}
		if err := syncFormDefinition(ctx, q, eventID, d); err != nil {
			return err
		}
		if err := auditForm(ctx, q, actor, eventID, d.ID); err != nil {
			return err
		}
		result, err = formView(ctx, q, row)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("save registration form: %w", err)
	}
	return result, nil
}

func syncFormDefinition(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	d forms.Definition,
) error {
	definitions := templates.EmptyDefinitions()
	row, err := q.EventDefinitions(ctx, eventID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read reusable form definition: %w", err)
	}
	if err == nil {
		if err := json.Unmarshal(row.Definitions, &definitions); err != nil {
			return fmt.Errorf("decode reusable forms: %w", err)
		}
	}
	found := false
	for i, old := range definitions.Forms {
		if old.ID == d.ID {
			definitions.Forms[i] = d
			found = true
		}
	}
	if !found {
		if len(definitions.Forms) >= 50 {
			return access.ErrInvalid
		}
		definitions.Forms = append(definitions.Forms, d)
	}
	eventRow, err := q.Event(ctx, eventID)
	if err != nil {
		return fmt.Errorf("read form event configuration: %w", err)
	}
	configuration := event.DefaultConfiguration()
	if err := json.Unmarshal(eventRow.Configuration, &configuration); err != nil {
		return fmt.Errorf("decode form event configuration: %w", err)
	}
	if err := templates.Validate(
		templates.Bundle{Event: configuration, Definitions: definitions},
	); err != nil {
		return fmt.Errorf("validate reusable form references: %w", err)
	}
	data, err := json.Marshal(definitions)
	if err != nil {
		return fmt.Errorf("encode reusable forms: %w", err)
	}
	if err := q.SaveEventDefinitions(
		ctx,
		dbgen.SaveEventDefinitionsParams{EventID: eventID, Definitions: data},
	); err != nil {
		return fmt.Errorf("sync reusable forms: %w", err)
	}
	if _, err := q.BumpEventRevision(ctx, eventID); err != nil {
		return fmt.Errorf("advance form configuration revision: %w", err)
	}
	return nil
}

func auditForm(ctx context.Context, q *dbgen.Queries, actor, eventID, id uuid.UUID) error {
	if err := audit.Append(
		ctx,
		q,
		audit.Record{
			ActorID:  &actor,
			EventID:  &eventID,
			TargetID: id,
			Action:   audit.DefinitionsUpdated,
			Reason:   audit.OrganiserDecision,
		},
	); err != nil {
		return fmt.Errorf("audit form change: %w", err)
	}
	return nil
}

func (s Service) PublishForm(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	revision int64,
	version int32,
	acknowledge bool,
) (FormView, error) {
	var result FormView
	if !acknowledge {
		return result, &forms.Invalid{
			Issues: []forms.Issue{
				{
					Field:   "acknowledge_response_versions",
					Code:    "acknowledgment_required",
					Message: "Confirm that existing responses keep their original version and new required questions apply to new responses.",
				},
			},
		}
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(ctx, eventID); err != nil {
			return fmt.Errorf("lock form publication: %w", err)
		}
		if err := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.EventEdit,
		); err != nil {
			return fmt.Errorf("authorize form publication: %w", err)
		}
		row, err := q.RegistrationForm(ctx, dbgen.RegistrationFormParams{EventID: eventID, ID: id})
		if err != nil {
			return fmt.Errorf("read form publication: %w", err)
		}
		if row.Revision != revision {
			return access.ErrConflict
		}
		if version != row.LatestVersion {
			return access.ErrConflict
		}
		if row.PublishedVersion.Valid && row.PublishedVersion.Int32 == version {
			result, err = formView(ctx, q, row)
			return err
		}
		if err := q.RecordFormPublication(
			ctx,
			dbgen.RecordFormPublicationParams{EventID: eventID, FormID: id, Version: version},
		); err != nil {
			return fmt.Errorf("record form publication: %w", err)
		}
		row, err = q.PublishRegistrationForm(
			ctx,
			dbgen.PublishRegistrationFormParams{
				EventID:          eventID,
				ID:               id,
				PublishedVersion: pgtype.Int4{Int32: version, Valid: true},
			},
		)
		if err != nil {
			return fmt.Errorf("publish form: %w", err)
		}
		if err := auditForm(ctx, q, actor, eventID, id); err != nil {
			return err
		}
		result, err = formView(ctx, q, row)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("publish registration form: %w", err)
	}
	return result, nil
}

func (s Service) PreviewForm(
	ctx context.Context,
	actor, eventID uuid.UUID,
	d forms.Definition,
	answers forms.Answers,
) (forms.Result, error) {
	if err := (access.EventPolicy{Queries: dbgen.New(s.Pool)}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		access.StaffRead,
	); err != nil {
		return forms.Result{}, fmt.Errorf("authorize form preview: %w", err)
	}
	if err := forms.Validate(d); err != nil {
		return forms.Result{}, fmt.Errorf("validate preview: %w", err)
	}
	return forms.Check(d, answers), nil
}

// StoreResponse runs in the caller's registration transaction. The caller must
// authorize the person and choose the current published version for new requests.
func StoreResponse(
	ctx context.Context,
	tx pgx.Tx,
	eventID, personID, formID uuid.UUID,
	version int32,
	revision int64,
	answers forms.Answers,
) error {
	q := dbgen.New(tx)
	if _, err := q.LockEvent(ctx, eventID); err != nil {
		return fmt.Errorf("lock response event: %w", err)
	}
	published, err := q.WasFormPublished(
		ctx,
		dbgen.WasFormPublishedParams{EventID: eventID, FormID: formID, Version: version},
	)
	if err != nil {
		return fmt.Errorf("check response version: %w", err)
	}
	if !published {
		return access.ErrInvalid
	}
	v, err := q.RegistrationFormVersion(
		ctx,
		dbgen.RegistrationFormVersionParams{EventID: eventID, FormID: formID, Version: version},
	)
	if err != nil {
		return fmt.Errorf("read response definition: %w", err)
	}
	var definition forms.Definition
	if err := json.Unmarshal(v.Definition, &definition); err != nil {
		return fmt.Errorf("decode response definition: %w", err)
	}
	checked := forms.Check(definition, answers)
	if len(checked.Issues) > 0 {
		return &forms.Invalid{Issues: checked.Issues}
	}
	current, err := q.FormResponse(
		ctx,
		dbgen.FormResponseParams{EventID: eventID, PersonID: personID, FormID: formID},
	)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if revision != 0 {
			return access.ErrConflict
		}
	case err != nil:
		return fmt.Errorf("read response revision: %w", err)
	case current.Revision != revision:
		return access.ErrConflict
	}
	row, err := q.AdvanceFormResponse(
		ctx,
		dbgen.AdvanceFormResponseParams{EventID: eventID, PersonID: personID, FormID: formID},
	)
	if err != nil {
		return fmt.Errorf("advance response: %w", err)
	}
	data, err := json.Marshal(checked.Answers)
	if err != nil {
		return fmt.Errorf("encode answers: %w", err)
	}
	if err := q.NewFormResponseVersion(
		ctx,
		dbgen.NewFormResponseVersionParams{
			EventID:     eventID,
			PersonID:    personID,
			FormID:      formID,
			Revision:    row.Revision,
			FormVersion: version,
			Answers:     data,
		},
	); err != nil {
		return fmt.Errorf("save versioned response: %w", err)
	}
	return nil
}
