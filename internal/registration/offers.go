package registration

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/permissions"
)

type (
	Offer struct {
		ID        uuid.UUID `json:"id"`
		ExpiresAt time.Time `json:"expires_at"`
		State     string    `json:"state"`
	}
	OfferSettings struct {
		Revision    int64 `json:"revision"`
		Automated   bool  `json:"automated"`
		WorkingDays int32 `json:"working_days"`
	}
)

func settings(ctx context.Context, q *dbgen.Queries, id uuid.UUID) (OfferSettings, error) {
	r, err := q.RegistrationSettings(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return OfferSettings{WorkingDays: 2}, nil
	}
	if err != nil {
		return OfferSettings{}, fmt.Errorf("read offer policy: %w", err)
	}
	return OfferSettings{
		Revision:    r.Revision,
		Automated:   r.Automated,
		WorkingDays: r.WorkingDays,
	}, nil
}

func (s Service) OfferPolicy(ctx context.Context, actor, id uuid.UUID) (OfferSettings, error) {
	q := dbgen.New(s.Pool)
	if err := (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		id,
		id,
		access.EventEdit,
	); err != nil {
		return OfferSettings{}, fmt.Errorf("authorize offer policy: %w", err)
	}
	return settings(ctx, q, id)
}

func (s Service) SaveOfferPolicy(
	ctx context.Context,
	actor, id uuid.UUID,
	input OfferSettings,
) (OfferSettings, error) {
	var result OfferSettings
	if input.WorkingDays < 1 || input.WorkingDays > 30 {
		return result, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, id)
		if err != nil {
			return fmt.Errorf("lock offer policy: %w", err)
		}
		if err := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			id,
			id,
			access.EventEdit,
		); err != nil {
			return fmt.Errorf("authorize offer policy change: %w", err)
		}
		current, err := settings(ctx, q, id)
		if err != nil {
			return err
		}
		if current.Revision != input.Revision {
			return access.ErrConflict
		}
		if _, err = q.SaveRegistrationSettings(
			ctx,
			dbgen.SaveRegistrationSettingsParams{
				EventID:     id,
				Automated:   input.Automated,
				WorkingDays: input.WorkingDays,
				ApprovedBy:  actor,
			},
		); err != nil {
			return fmt.Errorf("save offer policy: %w", err)
		}
		if err := auditForm(ctx, q, actor, id, id); err != nil {
			return err
		}
		if err := s.reconcileOffers(ctx, tx, q, row); err != nil {
			return err
		}
		result, err = settings(ctx, q, id)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("update offer policy: %w", err)
	}
	return result, nil
}

// MaintainOffers is safe for several workers and repeated delivery. The durable
// settings, offers and FIFO queue reconstruct all work under the same event lock.
func (s Service) MaintainOffers(ctx context.Context) error {
	after := uuid.Nil
	for {
		ids, err := dbgen.New(s.Pool).OfferMaintenanceEvents(ctx, after)
		if err != nil {
			return fmt.Errorf("find offer work: %w", err)
		}
		for _, id := range ids {
			if err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
				row, err := q.LockEvent(ctx, id)
				if err != nil {
					return fmt.Errorf("lock offer work: %w", err)
				}
				return s.reconcileOffers(ctx, tx, q, row)
			}); err != nil {
				return fmt.Errorf("maintain offers: %w", err)
			}
			after = id
		}
		if len(ids) < 50 {
			return nil
		}
	}
}

func (s Service) reconcileOffers(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	row dbgen.Event,
) error {
	c, err := configuration(row)
	if err != nil {
		return err
	}
	now := s.now()
	open := admissionOpen(row, c, now)
	pending, err := q.PendingRegistrationOffers(ctx, row.ID)
	if err != nil {
		return fmt.Errorf("read pending offers: %w", err)
	}
	for _, offer := range pending {
		if open && now.Before(offer.ExpiresAt) {
			continue
		}
		r, err := q.Registration(
			ctx,
			dbgen.RegistrationParams{EventID: row.ID, ID: offer.RegistrationID},
		)
		if err != nil {
			return fmt.Errorf("read expiring registration: %w", err)
		}
		if err := resolveOffer(ctx, q, offer, "expired"); err != nil {
			return err
		}
		if _, err := changeAttendee(
			ctx,
			q,
			uuid.Nil,
			r,
			"declined",
			"offer_expired",
			"",
		); err != nil {
			return err
		}
	}
	if !open {
		return nil
	}
	policy, err := q.RegistrationSettings(ctx, row.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read automation approval: %w", err)
	}
	if !policy.Automated {
		return nil
	}
	if err := (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		policy.ApprovedBy,
		row.ID,
		row.ID,
		access.EventEdit,
	); err != nil {
		if errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("recheck automation authority: %w", err)
	}
	return s.fillOffers(ctx, tx, q, row, c, policy)
}

func (s Service) fillOffers(
	ctx context.Context,
	_ pgx.Tx,
	q *dbgen.Queries,
	row dbgen.Event,
	c event.Configuration,
	policy dbgen.RegistrationSetting,
) error {
	expiry, err := c.Calendar.Offset(s.now(), c.Timezone, int(policy.WorkingDays), c.DSTChoice)
	if err != nil {
		return fmt.Errorf("resolve offer deadline: %w", err)
	}
	if c.AdmissionCutoff.Before(expiry) {
		expiry = c.AdmissionCutoff
	}
	if c.TeamFreeze.Before(expiry) {
		expiry = c.TeamFreeze
	}
	if !expiry.After(s.now()) {
		return nil
	}
	// At most the event's bounded capacity is allocated per reconciliation.
	for {
		available, err := capacityAvailable(ctx, q, row.ID, c.Capacity)
		if err != nil {
			return err
		}
		if !available {
			return nil
		}
		r, err := q.NextWaitlistedRegistration(ctx, row.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read next waiting person: %w", err)
		}
		eligibility := activePerson(ctx, q, r.PersonID)
		if eligibility == nil {
			eligibility = permissions.SetRegistrationState(ctx, q, row.ID, r.PersonID, "waitlisted")
		}
		if eligibility != nil {
			if !errors.Is(eligibility, access.ErrForbidden) &&
				!errors.Is(eligibility, permissions.ErrIneligible) &&
				!errors.Is(eligibility, permissions.ErrAssessment) {
				return fmt.Errorf("check offer eligibility: %w", eligibility)
			}
			if _, err := changeAttendee(
				ctx,
				q,
				policy.ApprovedBy,
				r,
				"declined",
				"ineligible",
				"",
			); err != nil {
				return err
			}
			continue
		}
		offer, err := q.CreateRegistrationOffer(
			ctx,
			dbgen.CreateRegistrationOfferParams{
				EventID:        row.ID,
				RegistrationID: r.ID,
				ExpiresAt:      expiry,
			},
		)
		if err != nil {
			return fmt.Errorf("reserve offer: %w", err)
		}
		if _, err := changeAttendee(
			ctx,
			q,
			policy.ApprovedBy,
			r,
			"offered",
			"offered",
			"",
		); err != nil {
			return err
		}
		if _, err := q.InsertOperation(
			ctx,
			dbgen.InsertOperationParams{
				EventID:      row.ID,
				OperationKey: "registration-offer:" + offer.ID.String(),
				Kind:         "registration.offer",
				TargetID:     offer.ID,
				DueAt:        s.now(),
				Approved:     true,
			},
		); err != nil {
			return fmt.Errorf("queue offer notification: %w", err)
		}
	}
}

func resolveOffer(
	ctx context.Context,
	q *dbgen.Queries,
	offer dbgen.RegistrationOffer,
	state string,
) error {
	if err := q.ResolveRegistrationOffer(
		ctx,
		dbgen.ResolveRegistrationOfferParams{EventID: offer.EventID, ID: offer.ID, State: state},
	); err != nil {
		return fmt.Errorf("resolve offer: %w", err)
	}
	if err := q.CancelOfferNotification(
		ctx,
		dbgen.CancelOfferNotificationParams{
			EventID:      offer.EventID,
			OperationKey: "registration-offer:" + offer.ID.String(),
		},
	); err != nil {
		return fmt.Errorf("cancel obsolete offer notice: %w", err)
	}
	return nil
}

func changeAttendee(
	ctx context.Context,
	q *dbgen.Queries,
	actor uuid.UUID,
	r dbgen.Registration,
	state, action, reason string,
) (dbgen.Registration, error) {
	claim := state
	if state == "offered" {
		claim = "waitlisted"
	}
	if state == "declined" {
		claim = "withdrawn"
	}
	if r.Kind == "attendee" {
		if err := permissions.SetRegistrationState(
			ctx,
			q,
			r.EventID,
			r.PersonID,
			claim,
		); err != nil {
			return r, fmt.Errorf("update competitor eligibility: %w", err)
		}
	}
	updated, err := q.RegistrationState(
		ctx,
		dbgen.RegistrationStateParams{EventID: r.EventID, ID: r.ID, State: state},
	)
	if err != nil {
		return r, fmt.Errorf("update registration outcome: %w", err)
	}
	if err := registrationHistory(ctx, q, actor, updated, action, r.State, reason); err != nil {
		return r, err
	}
	return updated, nil
}

// Respond returns the authoritative state for replayed/expired offer actions.
func (s Service) Respond(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	revision int64,
	action string,
) (View, error) {
	var result View
	if action != "accept" && action != "decline" && action != "withdraw" {
		return result, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock offer response: %w", err)
		}
		if err := activePerson(ctx, q, actor); err != nil {
			return err
		}
		r, err := q.Registration(ctx, dbgen.RegistrationParams{EventID: eventID, ID: id})
		if err != nil || r.PersonID != actor {
			return access.ErrNotFound
		}
		if err := s.reconcileOffers(ctx, tx, q, row); err != nil {
			return err
		}
		r, err = q.Registration(ctx, dbgen.RegistrationParams{EventID: eventID, ID: id})
		if err != nil {
			return fmt.Errorf("reload offer response: %w", err)
		}
		if action != "withdraw" && r.State != "offered" || r.State == "withdrawn" ||
			r.State == "declined" {
			result, err = viewRegistration(ctx, q, r)
			return err
		}
		if r.Revision != revision {
			return access.ErrConflict
		}
		if !row.WritesAllowed {
			return ErrClosed
		}
		state := "withdrawn"
		if action == "accept" {
			state = "confirmed"
		}
		if action == "decline" {
			state = "declined"
		}
		if r.State == "selected" {
			if err := permissions.Relinquish(ctx, q, eventID, actor, r.Audience); err != nil {
				return fmt.Errorf("withdraw facilitator role: %w", err)
			}
		}
		if r.State == "offered" {
			offer, err := q.LatestRegistrationOffer(
				ctx,
				dbgen.LatestRegistrationOfferParams{EventID: eventID, RegistrationID: id},
			)
			if err != nil {
				return fmt.Errorf("read accepted offer: %w", err)
			}
			status := "declined"
			if action == "accept" {
				status = "accepted"
			}
			if err := resolveOffer(ctx, q, offer, status); err != nil {
				return err
			}
		}
		r, err = changeAttendee(ctx, q, actor, r, state, action, "")
		if err != nil {
			return err
		}
		if err := s.reconcileOffers(ctx, tx, q, row); err != nil {
			return err
		}
		result, err = viewRegistration(ctx, q, r)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("respond to registration: %w", err)
	}
	return result, nil
}
