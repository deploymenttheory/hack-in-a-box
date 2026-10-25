package sessionbooking

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/maildelivery"
)

func proofFor(
	config notificationConfiguration,
	occ notificationOccurrence,
) communications.ScopeApproval {
	return communications.ScopeApproval{
		Kind:          "native_session_booking",
		ID:            config.ID,
		Binding:       occ.Binding,
		SourceID:      occ.ID,
		SourceVersion: occ.SessionRevision,
	}
}

// ResolveInTransaction is a typed scope multiplexer. Only a truly absent session
// occurrence may reach the existing automation owner; denied/stale/unavailable
// session sources never fall back to generic campaign authority.
func (s Service) ResolveInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	version int64,
	scope communications.DeliveryScope,
) (communications.ScopeAudience, error) {
	occ, err := loadOccurrence(ctx, tx, eventID, scope.OccurrenceID)
	if errors.Is(err, access.ErrNotFound) {
		if s.Fallback == nil {
			return communications.ScopeAudience{}, access.ErrNotFound
		}
		result, fallbackErr := s.Fallback.ResolveInTransaction(
			ctx,
			tx,
			eventID,
			campaignID,
			version,
			scope,
		)
		return result, contextError("resolve existing automation scope", fallbackErr)
	}
	if err != nil {
		return communications.ScopeAudience{}, err
	}
	st, config, err := s.currentNotification(ctx, tx, occ, true)
	if err != nil {
		return communications.ScopeAudience{}, err
	}
	_ = st
	if config.ID != campaignID || config.Version != version {
		return communications.ScopeAudience{}, access.ErrConflict
	}
	proof := proofFor(config, occ)
	return communications.ScopeAudience{
		Binding:         occ.Binding,
		RecipientIDs:    []uuid.UUID{occ.PersonID},
		RequireComplete: true,
		Approval:        &proof,
	}, nil
}

func (s Service) currentNotification(
	ctx context.Context,
	tx pgx.Tx,
	occ notificationOccurrence,
	delivery bool,
) (scope, notificationConfiguration, error) {
	st, err := loadRecord(ctx, tx, occ.PersonID, occ.EventID, occ.SessionID, false)
	if err != nil {
		return st, notificationConfiguration{}, err
	}
	config, err := notificationConfig(ctx, tx, occ.EventID, occ.SessionID)
	if err != nil {
		return st, config, err
	}
	q := dbgen.New(tx)
	person, err := q.Person(ctx, occ.PersonID)
	if err != nil {
		return st, config, contextError("read notification identity", err)
	}
	if !person.Active {
		return st, config, access.ErrForbidden
	}
	if !delivery {
		return st, config, nil
	}
	if !st.row.WritesAllowed || occ.SourceState != "approved" || !config.Approved ||
		config.Version != occ.ConfigurationVersion ||
		((occ.State == "booked" || occ.State == "waitlisted") && st.revision != occ.SessionRevision) {
		return st, config, access.ErrForbidden
	}
	if err = (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		config.Actor,
		occ.EventID,
		occ.EventID,
		access.EventEdit,
	); err != nil {
		return st, config, contextError("check current notification approval", err)
	}
	var state string
	var revision, calendarSequence int64
	err = tx.QueryRow(ctx, `SELECT state,revision,calendar_sequence FROM session_bookings WHERE event_id=$1 AND session_id=$2 AND person_id=$3`, occ.EventID, occ.SessionID, occ.PersonID).
		Scan(&state, &revision, &calendarSequence)
	if errors.Is(err, pgx.ErrNoRows) {
		return st, config, access.ErrNotFound
	}
	if err != nil {
		return st, config, contextError("read current booking source", err)
	}
	if state != occ.State || revision != occ.BookingRevision ||
		calendarSequence != occ.CalendarSequence {
		return st, config, access.ErrConflict
	}
	if state == "booked" || state == "waitlisted" {
		if !st.confirmed || !st.audience || st.cancelled || st.needsReview {
			return st, config, access.ErrForbidden
		}
		if err = (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			occ.PersonID,
			occ.EventID,
			occ.EventID,
			access.EventRead,
		); err != nil {
			return st, config, contextError("check current session audience", err)
		}
	}
	return st, config, nil
}

func (s Service) ResolveSessionNotificationInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	version int64,
	proof communications.ScopeApproval,
	person uuid.UUID,
) (communications.ApprovedCampaign, error) {
	occ, err := loadOccurrence(ctx, tx, eventID, proof.SourceID)
	if err != nil {
		return communications.ApprovedCampaign{}, err
	}
	st, config, err := s.currentNotification(ctx, tx, occ, true)
	if err != nil {
		return communications.ApprovedCampaign{}, err
	}
	if proof != proofFor(config, occ) || config.ID != campaignID || version != config.Version ||
		(person != uuid.Nil && person != occ.PersonID) {
		return communications.ApprovedCampaign{}, access.ErrForbidden
	}
	body := "Your session booking has been cancelled."
	switch occ.State {
	case "booked":
		body = "Your session place is booked. Open your event workspace for current details."
	case "waitlisted":
		body = "You are on the session waitlist. You do not have a reserved session place."
	}
	calendar := &maildelivery.Calendar{
		EventID:   eventID,
		SessionID: &occ.SessionID,
		PersonID:  &occ.PersonID,
		Sequence:  occ.CalendarSequence,
		Cancelled: occ.State != "booked",
	}
	if !calendar.Cancelled {
		calendar.Start = st.start
		calendar.End = st.end
		calendar.Timezone = st.source.Timezone
		calendar.Summary = st.item.Title
		calendar.WorkspaceURL = strings.TrimRight(
			s.WorkspaceOrigin,
			"/",
		) + "/events/" + eventID.String()
	}
	return communications.ApprovedCampaign{
		SessionCalendar: calendar,
		ScopeApproval:   &proof,
		Purpose:         "reminder",
		Format:          "text",
		Definition: communications.CampaignDefinition{
			ID:              config.ID,
			Channels:        config.Channels,
			IncludeCalendar: true,
		},
		Version: config.Version,
		DueAt:   s.now(),
		Approval: communications.CampaignApproval{
			ActorID:          config.Actor,
			ChannelRevisions: config.Revisions,
			Binding:          config.Binding,
		},
		Preview: communications.Preview{
			Recipients: []communications.Recipient{
				{
					PersonID:   occ.PersonID,
					Subject:    "Session booking update",
					Body:       body,
					Unresolved: []string{},
				},
			},
		},
	}, nil
}

// Historical minimal cancellations remain readable by their active recipient
// without event access. Ordinary notices still require the current session
// audience. Neither path exposes a meeting URL or reauthorizes delivery.
func (s Service) AuthorizeSessionInboxInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, actor uuid.UUID,
	proof communications.ScopeApproval,
) (bool, error) {
	occ, err := loadOccurrence(ctx, tx, eventID, proof.SourceID)
	if err != nil {
		return false, err
	}
	if occ.PersonID != actor || proof.Kind != "native_session_booking" ||
		proof.Binding != occ.Binding ||
		proof.SourceVersion != occ.SessionRevision {
		return false, access.ErrNotFound
	}
	st, _, err := s.currentNotification(ctx, tx, occ, false)
	if err != nil {
		return false, err
	}
	var approved bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM session_notification_configurations c JOIN session_notification_approvals a ON a.configuration_id=c.id WHERE c.event_id=$1 AND c.session_id=$2 AND c.version=$3 AND c.id=$4)`, eventID, occ.SessionID, occ.ConfigurationVersion, proof.ID).
		Scan(&approved); err != nil {
		return false, contextError("authorize retained session approval", err)
	}
	if !approved {
		return false, access.ErrNotFound
	}
	if occ.State == "cancelled" || occ.State == "ineligible" {
		return true, nil
	}
	if !st.confirmed || !st.audience || st.cancelled || st.needsReview {
		return false, access.ErrNotFound
	}
	if err = (access.EventPolicy{Queries: dbgen.New(tx)}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		access.EventRead,
	); err != nil {
		return false, contextError("authorize retained session reader", err)
	}
	return false, nil
}

type Cancellation struct {
	SessionID uuid.UUID `json:"session_id"`
	PersonID  uuid.UUID `json:"person_id"`
	Sequence  int64     `json:"sequence"`
}

func (s Service) OwnCancellation(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
) (Cancellation, error) {
	var result Cancellation
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		person, err := q.Person(ctx, actor)
		if err != nil {
			return contextError("read cancellation identity", err)
		}
		if !person.Active {
			return access.ErrForbidden
		}
		var state string
		var sequence int64
		err = tx.QueryRow(ctx, `SELECT b.state,b.calendar_sequence FROM session_bookings b JOIN bookable_sessions s ON s.event_id=b.event_id AND s.id=b.session_id WHERE b.event_id=$1 AND b.session_id=$2 AND b.person_id=$3`, eventID, id, actor).
			Scan(&state, &sequence)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return contextError("read own cancellation", err)
		}
		if state != "cancelled" && state != "ineligible" {
			return access.ErrNotFound
		}
		result = Cancellation{SessionID: id, PersonID: actor, Sequence: sequence}
		return nil
	})
	return result, contextError("read own cancellation", err)
}
