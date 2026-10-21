package registration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
)

type Summary struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Venue           string    `json:"venue"`
	Timezone        string    `json:"timezone"`
	LiveStart       time.Time `json:"live_start"`
	LiveEnd         time.Time `json:"live_end"`
	AdmissionCutoff time.Time `json:"admission_cutoff"`
	TeamFreeze      time.Time `json:"team_freeze"`
	Stage           string    `json:"stage"`
	Open            bool      `json:"open"`
}
type (
	PublishedForm struct {
		Definition forms.Definition `json:"definition"`
		Version    int32            `json:"version"`
	}
	RegistrationPage struct {
		Branding      templates.Branding `json:"branding"`
		Event         Summary            `json:"event"`
		Forms         []PublishedForm    `json:"forms"`
		Registrations []View             `json:"registrations"`
	}
)

func summary(row dbgen.Event, now time.Time) (Summary, error) {
	c, err := configuration(row)
	if err != nil {
		return Summary{}, err
	}
	return Summary{
		ID:              row.ID,
		Name:            row.Name,
		Description:     c.Description,
		Venue:           c.Venue,
		Timezone:        c.Timezone,
		LiveStart:       c.LiveStart,
		LiveEnd:         c.LiveEnd,
		AdmissionCutoff: c.AdmissionCutoff,
		TeamFreeze:      c.TeamFreeze,
		Stage:           row.Stage,
		Open:            row.Stage == "registration" && admissionOpen(row, c, now),
	}, nil
}

func (s Service) Discover(
	ctx context.Context,
	actor, after uuid.UUID,
	limit int32,
) ([]Summary, error) {
	q := dbgen.New(s.Pool)
	if err := activePerson(ctx, q, actor); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 50 {
		return nil, access.ErrInvalid
	}
	rows, err := q.OpenRegistrationEvents(
		ctx,
		dbgen.OpenRegistrationEventsParams{ID: after, Limit: limit},
	)
	if err != nil {
		return nil, fmt.Errorf("discover registration events: %w", err)
	}
	result := make([]Summary, 0, len(rows))
	for _, row := range rows {
		v, err := summary(row, s.now())
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, nil
}

func (s Service) Page(ctx context.Context, actor, eventID uuid.UUID) (RegistrationPage, error) {
	result := RegistrationPage{Forms: []PublishedForm{}, Registrations: []View{}}
	q := dbgen.New(s.Pool)
	if err := activePerson(ctx, q, actor); err != nil {
		return result, err
	}
	row, err := q.Event(ctx, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, access.ErrNotFound
	}
	if err != nil {
		return result, fmt.Errorf("read registration event: %w", err)
	}
	own, err := s.Own(ctx, actor, eventID)
	if err != nil {
		return result, err
	}
	if row.Stage != "registration" && len(own) == 0 {
		return result, access.ErrNotFound
	}
	result.Event, err = summary(row, s.now())
	if err != nil {
		return result, err
	}
	result.Registrations = own
	result.Branding = templates.EmptyDefinitions().Branding
	definitions, readErr := q.EventDefinitions(ctx, eventID)
	if readErr != nil && !errors.Is(readErr, pgx.ErrNoRows) {
		return result, fmt.Errorf("read registration branding: %w", readErr)
	}
	if readErr == nil {
		selected := struct {
			Branding templates.Branding `json:"branding"`
		}{Branding: result.Branding}
		if err := json.Unmarshal(definitions.Definitions, &selected); err != nil {
			return result, fmt.Errorf("decode registration branding: %w", err)
		}
		result.Branding = selected.Branding
	}
	if !result.Event.Open {
		return result, nil
	}
	rows, err := q.PublishedRegistrationForms(ctx, eventID)
	if err != nil {
		return result, fmt.Errorf("read public form versions: %w", err)
	}
	for _, r := range rows {
		var d forms.Definition
		if err := json.Unmarshal(r.Definition, &d); err != nil {
			return result, fmt.Errorf("decode published form: %w", err)
		}
		result.Forms = append(result.Forms, PublishedForm{Definition: d, Version: r.Version})
	}
	return result, nil
}
