package registration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/forms"
	"github.com/deploymenttheory/hack-in-a-box/internal/permissions"
)

var (
	ErrClosed    = errors.New("new registrations are closed for this event")
	ErrDuplicate = errors.New(
		"you already have an active registration for this event and application type",
	)
	ErrCapacity = errors.New("no unreserved attendee place is available")
)

type Application struct {
	ID                  uuid.UUID     `json:"id"`
	FormID              uuid.UUID     `json:"form_id"`
	FormVersion         int32         `json:"form_version"`
	Answers             forms.Answers `json:"answers"`
	TeamPreference      string        `json:"team_preference"`
	InvitationReference string        `json:"invitation_reference"`
	FallbackToAttendee  bool          `json:"fallback_to_attendee"`
}
type View struct {
	Offer               *Offer    `json:"offer"`
	ID                  uuid.UUID `json:"id"`
	EventID             uuid.UUID `json:"event_id"`
	PersonID            uuid.UUID `json:"person_id"`
	FormID              uuid.UUID `json:"form_id"`
	FormVersion         int32     `json:"form_version"`
	Audience            string    `json:"audience"`
	Kind                string    `json:"kind"`
	State               string    `json:"state"`
	Sequence            int64     `json:"sequence"`
	Revision            int64     `json:"revision"`
	TeamPreference      string    `json:"team_preference"`
	InvitationReference string    `json:"invitation_reference"`
	FallbackToAttendee  bool      `json:"fallback_to_attendee"`
	WaitlistPosition    *int64    `json:"waitlist_position"`
}

func viewRegistration(ctx context.Context, q *dbgen.Queries, r dbgen.Registration) (View, error) {
	v := View{
		ID:                  r.ID,
		EventID:             r.EventID,
		PersonID:            r.PersonID,
		FormID:              r.FormID,
		FormVersion:         r.FormVersion,
		Audience:            r.Audience,
		Kind:                r.Kind,
		State:               r.State,
		Sequence:            r.Sequence,
		Revision:            r.Revision,
		TeamPreference:      r.TeamPreference,
		InvitationReference: r.InvitationReference,
		FallbackToAttendee:  r.FallbackToAttendee,
	}
	if r.State == "waitlisted" {
		position, err := q.WaitlistPosition(
			ctx,
			dbgen.WaitlistPositionParams{EventID: r.EventID, Sequence: r.Sequence},
		)
		if err != nil {
			return v, fmt.Errorf("read waitlist position: %w", err)
		}
		v.WaitlistPosition = &position
	}
	offer, err := q.LatestRegistrationOffer(
		ctx,
		dbgen.LatestRegistrationOfferParams{EventID: r.EventID, RegistrationID: r.ID},
	)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return v, fmt.Errorf("read registration offer: %w", err)
	}
	if err == nil {
		v.Offer = &Offer{ID: offer.ID, ExpiresAt: offer.ExpiresAt, State: offer.State}
	}
	return v, nil
}

func activePerson(ctx context.Context, q *dbgen.Queries, id uuid.UUID) error {
	p, err := q.Person(ctx, id)
	if err != nil || !p.Active {
		return access.ErrForbidden
	}
	return nil
}

func configuration(row dbgen.Event) (event.Configuration, error) {
	c := event.DefaultConfiguration()
	if err := json.Unmarshal(row.Configuration, &c); err != nil {
		return c, fmt.Errorf("decode registration configuration: %w", err)
	}
	return c, nil
}

func admissionOpen(row dbgen.Event, c event.Configuration, now time.Time) bool {
	return row.WritesAllowed &&
		slices.Contains([]string{"registration", "preparation"}, row.Stage) &&
		!row.TeamFrozenAt.Valid &&
		now.Before(c.AdmissionCutoff) &&
		now.Before(c.TeamFreeze)
}

func capacityAvailable(
	ctx context.Context,
	q *dbgen.Queries,
	id uuid.UUID,
	capacity int,
) (bool, error) {
	confirmed, err := q.ConfirmedAttendees(ctx, id)
	if err != nil {
		return false, fmt.Errorf("count confirmed places: %w", err)
	}
	reserved, err := q.ReservedAttendees(ctx, id)
	if err != nil {
		return false, fmt.Errorf("count reserved places: %w", err)
	}
	return confirmed+reserved < int64(capacity), nil
}

func registrationHistory(
	ctx context.Context,
	q *dbgen.Queries,
	actor uuid.UUID,
	row dbgen.Registration,
	action, previous, reason string,
) error {
	var actorID *uuid.UUID
	reasonCode := audit.UserRequest
	if actor != uuid.Nil {
		actorID = &actor
	} else {
		reasonCode = audit.DeadlinePolicy
	}

	if err := q.RecordRegistrationHistory(
		ctx,
		dbgen.RecordRegistrationHistoryParams{
			EventID:        row.EventID,
			RegistrationID: row.ID,
			ActorID:        actorID,
			Action:         action,
			FromState:      previous,
			ToState:        row.State,
			Reason:         reason,
		},
	); err != nil {
		return fmt.Errorf("record registration history: %w", err)
	}
	if err := audit.Append(
		ctx,
		q,
		audit.Record{
			ActorID:  actorID,
			EventID:  &row.EventID,
			TargetID: row.ID,
			Action:   audit.RegistrationChanged,
			Reason:   reasonCode,
		},
	); err != nil {
		return fmt.Errorf("audit registration: %w", err)
	}
	if _, err := q.BumpEventRevision(ctx, row.EventID); err != nil {
		return fmt.Errorf("advance registration event revision: %w", err)
	}
	return nil
}

func loadPublishedDefinition(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	input Application,
) (forms.Definition, error) {
	var d forms.Definition
	v, err := q.RegistrationFormVersion(
		ctx,
		dbgen.RegistrationFormVersionParams{
			EventID: eventID,
			FormID:  input.FormID,
			Version: input.FormVersion,
		},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, access.ErrNotFound
	}
	if err != nil {
		return d, fmt.Errorf("read submitted form: %w", err)
	}
	if err := json.Unmarshal(v.Definition, &d); err != nil {
		return d, fmt.Errorf("decode submitted form: %w", err)
	}
	return d, nil
}

func fingerprint(input Application) (string, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("encode application identity: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func (s Service) Apply(
	ctx context.Context,
	actor, eventID uuid.UUID,
	input Application,
) (View, error) {
	var result View
	if input.ID == uuid.Nil || input.FormID == uuid.Nil || input.FormVersion < 1 ||
		input.FormVersion > 1000 ||
		!slices.Contains([]string{"captain", "join", "solo"}, input.TeamPreference) ||
		len(input.InvitationReference) > 160 {
		return result, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, eventID)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock application event: %w", err)
		}
		if err := activePerson(ctx, q, actor); err != nil {
			return err
		}
		d, err := loadPublishedDefinition(ctx, q, eventID, input)
		if err != nil {
			return err
		}
		checked := forms.Check(d, input.Answers)
		if len(checked.Issues) > 0 {
			return &forms.Invalid{Issues: checked.Issues}
		}
		input.Answers = checked.Answers
		hash, err := fingerprint(input)
		if err != nil {
			return err
		}
		existing, err := q.Registration(
			ctx,
			dbgen.RegistrationParams{EventID: eventID, ID: input.ID},
		)
		if err == nil {
			if existing.PersonID != actor ||
				(existing.RequestHash != "" && existing.RequestHash != hash) {
				return access.ErrConflict
			}
			result, err = viewRegistration(ctx, q, existing)
			return err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read application retry: %w", err)
		}
		kind, state, err := s.allocateApplication(ctx, q, actor, row, input, d.Audience)
		if err != nil {
			return err
		}
		created, err := q.InsertRegistration(
			ctx,
			dbgen.InsertRegistrationParams{
				ID:                  input.ID,
				EventID:             eventID,
				PersonID:            actor,
				FormID:              input.FormID,
				FormVersion:         input.FormVersion,
				Audience:            d.Audience,
				Kind:                kind,
				State:               state,
				RequestHash:         hash,
				TeamPreference:      input.TeamPreference,
				InvitationReference: input.InvitationReference,
				FallbackToAttendee:  input.FallbackToAttendee,
			},
		)
		if err != nil {
			return fmt.Errorf("record application: %w", err)
		}
		previousResponse, err := q.FormResponse(
			ctx,
			dbgen.FormResponseParams{EventID: eventID, PersonID: actor, FormID: input.FormID},
		)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read previous response: %w", err)
		}
		if err := StoreResponse(
			ctx,
			tx,
			eventID,
			actor,
			input.FormID,
			input.FormVersion,
			previousResponse.Revision,
			input.Answers,
		); err != nil {
			return err
		}
		if err := q.SetRegistrationResponseRevision(
			ctx,
			dbgen.SetRegistrationResponseRevisionParams{
				EventID:          eventID,
				ID:               created.ID,
				ResponseRevision: previousResponse.Revision + 1,
			},
		); err != nil {
			return fmt.Errorf("link submitted response: %w", err)
		}
		if err := event.RecordAttendanceReady(ctx, tx, eventID); err != nil {
			return fmt.Errorf("record verified attendance: %w", err)
		}
		if err := registrationHistory(ctx, q, actor, created, "applied", "", ""); err != nil {
			return err
		}
		result, err = viewRegistration(ctx, q, created)
		return err
	})
	if err != nil {
		var constraint *pgconn.PgError
		if errors.As(err, &constraint) && constraint.Code == "23505" {
			return result, access.ErrConflict
		}
		return result, fmt.Errorf("apply for event: %w", err)
	}
	return result, nil
}

func allocate(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID uuid.UUID,
	kind string,
	capacity int,
) (string, error) {
	if kind != "attendee" {
		return "received", nil
	}
	available, err := capacityAvailable(ctx, q, eventID, capacity)
	if err != nil {
		return "", err
	}
	waiting, err := q.WaitingAttendees(ctx, eventID)
	if err != nil {
		return "", fmt.Errorf("check FIFO queue: %w", err)
	}
	state := "waitlisted"
	if available && waiting == 0 {
		state = "confirmed"
	}
	if err := permissions.SetRegistrationState(ctx, q, eventID, actor, state); err != nil {
		return "", fmt.Errorf("enforce competition eligibility: %w", err)
	}
	return state, nil
}

func (s Service) Own(ctx context.Context, actor, eventID uuid.UUID) ([]View, error) {
	q := dbgen.New(s.Pool)
	if err := activePerson(ctx, q, actor); err != nil {
		return nil, err
	}
	rows, err := q.OwnRegistrations(
		ctx,
		dbgen.OwnRegistrationsParams{EventID: eventID, PersonID: actor},
	)
	if err != nil {
		return nil, fmt.Errorf("read own registrations: %w", err)
	}
	result := make([]View, 0, len(rows))
	for _, row := range rows {
		v, err := viewRegistration(ctx, q, row)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, nil
}

func (s Service) Promote(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	revision int64,
	reason string,
) (View, error) {
	var result View
	if strings.TrimSpace(reason) == "" || len(reason) > 2000 {
		return result, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock promotion event: %w", err)
		}
		if err := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.EventEdit,
		); err != nil {
			return fmt.Errorf("authorize promotion: %w", err)
		}
		c, err := configuration(row)
		if err != nil {
			return err
		}
		if !admissionOpen(row, c, s.now()) {
			return ErrClosed
		}
		registration, err := q.Registration(ctx, dbgen.RegistrationParams{EventID: eventID, ID: id})
		if err != nil {
			return access.ErrNotFound
		}
		if registration.Revision != revision {
			return access.ErrConflict
		}
		if registration.Kind != "attendee" || registration.State != "waitlisted" {
			return access.ErrInvalid
		}
		if err := activePerson(ctx, q, registration.PersonID); err != nil {
			return err
		}
		available, err := capacityAvailable(ctx, q, eventID, c.Capacity)
		if err != nil {
			return err
		}
		if !available {
			return ErrCapacity
		}
		if err := permissions.SetRegistrationState(
			ctx,
			q,
			eventID,
			registration.PersonID,
			"confirmed",
		); err != nil {
			return fmt.Errorf("check promotion eligibility: %w", err)
		}
		registration, err = q.RegistrationState(
			ctx,
			dbgen.RegistrationStateParams{EventID: eventID, ID: id, State: "confirmed"},
		)
		if err != nil {
			return fmt.Errorf("promote registration: %w", err)
		}
		if err := registrationHistory(
			ctx,
			q,
			actor,
			registration,
			"manual_promotion",
			"waitlisted",
			reason,
		); err != nil {
			return err
		}
		result, err = viewRegistration(ctx, q, registration)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("promote attendee: %w", err)
	}
	return result, nil
}

func (s Service) allocateApplication(
	ctx context.Context,
	q *dbgen.Queries,
	actor uuid.UUID,
	row dbgen.Event,
	input Application,
	audience string,
) (string, string, error) {
	c, err := configuration(row)
	if err != nil {
		return "", "", err
	}
	if row.Stage != "registration" || !admissionOpen(row, c, s.now()) {
		return "", "", ErrClosed
	}
	form, err := q.RegistrationForm(
		ctx,
		dbgen.RegistrationFormParams{EventID: row.ID, ID: input.FormID},
	)
	if err != nil {
		return "", "", fmt.Errorf("read published form: %w", err)
	}
	if !form.PublishedVersion.Valid || form.PublishedVersion.Int32 != input.FormVersion {
		return "", "", access.ErrConflict
	}
	kind := "facilitator"
	if audience == "attendee" {
		kind = "attendee"
	}
	if _, err := q.ActiveRegistration(
		ctx,
		dbgen.ActiveRegistrationParams{EventID: row.ID, PersonID: actor, Kind: kind},
	); err == nil {
		return "", "", ErrDuplicate
	} else if !errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return "", "", fmt.Errorf("check active registration: %w", err)
	}
	state, err := allocate(ctx, q, actor, row.ID, kind, c.Capacity)
	return kind, state, err
}
