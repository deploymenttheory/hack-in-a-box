package registration

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
)

type Response struct {
	Definition forms.Definition `json:"definition"`
	Answers    forms.Answers    `json:"answers"`
	Revision   int64            `json:"revision"`
}

func response(ctx context.Context, q *dbgen.Queries, r dbgen.Registration) (Response, error) {
	var result Response
	d, err := loadPublishedDefinition(
		ctx,
		q,
		r.EventID,
		Application{FormID: r.FormID, FormVersion: r.FormVersion},
	)
	if err != nil {
		return result, err
	}
	v, err := q.RegistrationResponseVersion(
		ctx,
		dbgen.RegistrationResponseVersionParams{
			EventID:  r.EventID,
			PersonID: r.PersonID,
			FormID:   r.FormID,
			Revision: r.ResponseRevision,
		},
	)
	if err != nil {
		return result, access.ErrNotFound
	}
	result.Definition = d
	result.Revision = r.Revision
	if err := json.Unmarshal(v.Answers, &result.Answers); err != nil {
		return result, fmt.Errorf("decode private response: %w", err)
	}
	return result, nil
}

func (s Service) OwnResponse(ctx context.Context, actor, eventID, id uuid.UUID) (Response, error) {
	q := dbgen.New(s.Pool)
	if err := activePerson(ctx, q, actor); err != nil {
		return Response{}, err
	}
	r, err := q.Registration(ctx, dbgen.RegistrationParams{EventID: eventID, ID: id})
	if err != nil || r.PersonID != actor {
		return Response{}, access.ErrNotFound
	}
	return response(ctx, q, r)
}

func (s Service) CorrectResponse(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	revision int64,
	answers forms.Answers,
) (Response, error) {
	var result Response
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock response correction: %w", err)
		}
		if err := activePerson(ctx, q, actor); err != nil {
			return err
		}
		r, err := q.Registration(ctx, dbgen.RegistrationParams{EventID: eventID, ID: id})
		if err != nil || r.PersonID != actor {
			return access.ErrNotFound
		}
		if r.Revision != revision {
			return access.ErrConflict
		}
		if !row.WritesAllowed || r.State == "withdrawn" || r.State == "declined" {
			return ErrClosed
		}
		previous, err := q.FormResponse(
			ctx,
			dbgen.FormResponseParams{EventID: eventID, PersonID: actor, FormID: r.FormID},
		)
		if err != nil {
			return access.ErrNotFound
		}
		if err := StoreResponse(
			ctx,
			tx,
			eventID,
			actor,
			r.FormID,
			r.FormVersion,
			previous.Revision,
			answers,
		); err != nil {
			return err
		}
		if err := q.SetRegistrationResponseRevision(
			ctx,
			dbgen.SetRegistrationResponseRevisionParams{
				EventID:          eventID,
				ID:               id,
				ResponseRevision: previous.Revision + 1,
			},
		); err != nil {
			return fmt.Errorf("link corrected response: %w", err)
		}
		r, err = q.RegistrationState(
			ctx,
			dbgen.RegistrationStateParams{EventID: eventID, ID: id, State: r.State},
		)
		if err != nil {
			return fmt.Errorf("advance response revision: %w", err)
		}
		if err := registrationHistory(
			ctx,
			q,
			actor,
			r,
			"response_corrected",
			r.State,
			"",
		); err != nil {
			return err
		}
		result, err = response(ctx, q, r)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("correct registration response: %w", err)
	}
	return result, nil
}
