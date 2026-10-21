package registration

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/permissions"
)

type (
	Applicant struct {
		Registration View   `json:"registration"`
		DisplayName  string `json:"display_name"`
	}
	History struct {
		Action string    `json:"action"`
		From   string    `json:"from"`
		To     string    `json:"to"`
		Reason string    `json:"reason"`
		At     time.Time `json:"at"`
	}
	ApplicantDetail struct {
		Applicant         Applicant `json:"applicant"`
		Response          *Response `json:"response"`
		History           []History `json:"history"`
		FallbackAvailable bool      `json:"fallback_available"`
	}
)

func (s Service) Applicants(
	ctx context.Context,
	actor, eventID uuid.UUID,
	after int64,
	limit int32,
) ([]Applicant, error) {
	q := dbgen.New(s.Pool)
	if err := (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		access.StaffRead,
	); err != nil {
		return nil, fmt.Errorf("authorize applicants: %w", err)
	}
	if after < 0 || limit < 1 || limit > 50 {
		return nil, access.ErrInvalid
	}
	rows, err := q.ListRegistrations(
		ctx,
		dbgen.ListRegistrationsParams{EventID: eventID, Sequence: after, Limit: limit},
	)
	if err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}
	result := make([]Applicant, 0, len(rows))
	for _, r := range rows {
		v, err := viewRegistration(ctx, q, r.Registration)
		if err != nil {
			return nil, err
		}
		result = append(result, Applicant{Registration: v, DisplayName: r.DisplayName})
	}
	return result, nil
}

func (s Service) Applicant(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
) (ApplicantDetail, error) {
	var result ApplicantDetail
	q := dbgen.New(s.Pool)
	if err := (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		access.StaffRead,
	); err != nil {
		return result, fmt.Errorf("authorize applicant answers: %w", err)
	}
	r, err := q.Registration(ctx, dbgen.RegistrationParams{EventID: eventID, ID: id})
	if err != nil {
		return result, access.ErrNotFound
	}
	p, err := q.Person(ctx, r.PersonID)
	if err != nil {
		return result, fmt.Errorf("read applicant identity: %w", err)
	}
	v, err := viewRegistration(ctx, q, r)
	if err != nil {
		return result, err
	}
	result.Applicant = Applicant{Registration: v, DisplayName: p.DisplayName}
	answers, err := response(ctx, q, r)
	if err != nil && !errors.Is(err, access.ErrNotFound) {
		return result, err
	}
	if err == nil {
		result.Response = &answers
	}
	records, err := q.RegistrationHistory(
		ctx,
		dbgen.RegistrationHistoryParams{EventID: eventID, RegistrationID: id},
	)
	if err != nil {
		return result, fmt.Errorf("read application history: %w", err)
	}
	result.History = make([]History, 0, len(records))
	for _, h := range records {
		result.History = append(
			result.History,
			History{
				Action: h.Action,
				From:   h.FromState,
				To:     h.ToState,
				Reason: h.Reason,
				At:     h.CreatedAt,
			},
		)
	}
	row, err := q.Event(ctx, eventID)
	if err != nil {
		return result, fmt.Errorf("read fallback event: %w", err)
	}
	result.FallbackAvailable, err = s.fallbackAvailable(ctx, q, row, r)
	return result, err
}

func (s Service) fallbackAvailable(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	r dbgen.Registration,
) (bool, error) {
	c, err := configuration(row)
	if err != nil {
		return false, err
	}
	if !r.FallbackToAttendee || r.Kind != "facilitator" ||
		!slices.Contains([]string{"declined", "backup"}, r.State) ||
		row.Stage != "registration" ||
		!admissionOpen(row, c, s.now()) {
		return false, nil
	}
	if err := activePerson(ctx, q, r.PersonID); err != nil {
		return false, nil
	}
	excluded, err := q.CompetitionExcluded(
		ctx,
		dbgen.CompetitionExcludedParams{EventID: row.ID, PersonID: r.PersonID},
	)
	if err != nil {
		return false, fmt.Errorf("check fallback eligibility: %w", err)
	}
	if excluded {
		return false, nil
	}
	roles, err := q.ActiveRoles(ctx, dbgen.ActiveRolesParams{EventID: row.ID, PersonID: r.PersonID})
	if err != nil {
		return false, fmt.Errorf("check fallback roles: %w", err)
	}
	if slices.Contains(roles, "mentor") || slices.Contains(roles, "judge") {
		return false, nil
	}
	_, err = q.ActiveRegistration(
		ctx,
		dbgen.ActiveRegistrationParams{EventID: row.ID, PersonID: r.PersonID, Kind: "attendee"},
	)
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("check existing fallback registration: %w", err)
	}
	return true, nil
}

type ReviewInput struct {
	Action      string `json:"action"`
	Reason      string `json:"reason"`
	Acknowledge bool   `json:"acknowledge_competition_restriction"`
}

func (s Service) Review(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	revision int64,
	input ReviewInput,
) (View, error) {
	var result View
	if !slices.Contains([]string{"selected", "backup", "declined", "invite"}, input.Action) ||
		strings.TrimSpace(input.Reason) == "" ||
		len(input.Reason) > 2000 {
		return result, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock applicant decision: %w", err)
		}
		if err := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.StaffManage,
		); err != nil {
			return fmt.Errorf("authorize applicant decision: %w", err)
		}
		r, err := q.Registration(ctx, dbgen.RegistrationParams{EventID: eventID, ID: id})
		if err != nil {
			return access.ErrNotFound
		}
		if r.Revision != revision {
			return access.ErrConflict
		}
		if r.Kind != "facilitator" || r.State == "withdrawn" {
			return access.ErrInvalid
		}
		if r.State == "declined" && (input.Action == "selected" || input.Action == "backup") {
			if _, err := q.ActiveRegistration(
				ctx,
				dbgen.ActiveRegistrationParams{
					EventID:  eventID,
					PersonID: r.PersonID,
					Kind:     "facilitator",
				},
			); err == nil {
				return ErrDuplicate
			} else if !errors.Is(
				err,
				pgx.ErrNoRows,
			) {
				return fmt.Errorf("check newer facilitator application: %w", err)
			}
		}

		if input.Action == "invite" {
			return s.inviteFallback(ctx, q, row, r, actor, input.Reason, &result)
		}
		if r.State == input.Action {
			result, err = viewRegistration(ctx, q, r)
			return err
		}
		if input.Action == "selected" || r.State == "selected" {
			if _, err := permissions.ChangeWithin(
				ctx,
				q,
				permissions.Change{
					Actor:                             actor,
					Event:                             eventID,
					Person:                            r.PersonID,
					Role:                              r.Audience,
					Active:                            input.Action == "selected",
					Revision:                          row.Revision,
					AcknowledgeCompetitionRestriction: input.Acknowledge,
				},
			); err != nil {
				return fmt.Errorf("apply selected staff role: %w", err)
			}
		}
		updated, err := q.RegistrationState(
			ctx,
			dbgen.RegistrationStateParams{EventID: eventID, ID: id, State: input.Action},
		)
		if err != nil {
			return fmt.Errorf("record facilitator decision: %w", err)
		}
		if err := registrationHistory(
			ctx,
			q,
			actor,
			updated,
			"reviewed",
			r.State,
			input.Reason,
		); err != nil {
			return err
		}
		result, err = viewRegistration(ctx, q, updated)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("review application: %w", err)
	}
	return result, nil
}

func (s Service) inviteFallback(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	r dbgen.Registration,
	actor uuid.UUID,
	reason string,
	result *View,
) error {
	available, err := s.fallbackAvailable(ctx, q, row, r)
	if err != nil {
		return err
	}
	if !available {
		return access.ErrInvalid
	}
	already, err := q.HasRegistrationInvitation(
		ctx,
		dbgen.HasRegistrationInvitationParams{
			EventID:      row.ID,
			OperationKey: "registration-fallback:" + r.ID.String(),
		},
	)
	if err != nil {
		return fmt.Errorf("read previous fallback invitation: %w", err)
	}
	if already {
		*result, err = viewRegistration(ctx, q, r)
		return err
	}
	if _, err := q.InsertOperation(
		ctx,
		dbgen.InsertOperationParams{
			EventID:      row.ID,
			OperationKey: "registration-fallback:" + r.ID.String(),
			Kind:         "registration.fallback",
			TargetID:     r.ID,
			DueAt:        s.now(),
			Approved:     true,
		},
	); err != nil {
		return fmt.Errorf("queue separate application invitation: %w", err)
	}
	if err := registrationHistory(
		ctx,
		q,
		actor,
		r,
		"fallback_invited",
		r.State,
		reason,
	); err != nil {
		return err
	}
	*result, err = viewRegistration(ctx, q, r)
	return err
}
