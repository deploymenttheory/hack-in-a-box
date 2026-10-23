// Package privacy owns operational personal fields and their retention policy.
package privacy

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
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
	"github.com/deploymenttheory/hack-in-a-box/internal/surveys"
)

type Fields struct {
	ContactEmail     string  `json:"contact_email"`
	ContactPhone     string  `json:"contact_phone"`
	Dietary          string  `json:"dietary"`
	Accessibility    string  `json:"accessibility"`
	PhotoConsent     string  `json:"photo_consent"`
	TravelDetails    *string `json:"travel_details,omitempty"`
	SocialAttendance *string `json:"social_attendance,omitempty"`
}
type View struct {
	ID               uuid.UUID `json:"id"`
	EventID          uuid.UUID `json:"event_id"`
	PersonID         uuid.UUID `json:"person_id"`
	DisplayName      string    `json:"display_name"`
	Revision         int64     `json:"revision"`
	ContactEmail     *string   `json:"contact_email,omitempty"`
	ContactPhone     *string   `json:"contact_phone,omitempty"`
	Dietary          *string   `json:"dietary,omitempty"`
	Accessibility    *string   `json:"accessibility,omitempty"`
	PhotoConsent     *string   `json:"photo_consent,omitempty"`
	TravelDetails    *string   `json:"travel_details,omitempty"`
	SocialAttendance *string   `json:"social_attendance,omitempty"`
}
type (
	Settings struct {
		RetentionDays int32 `json:"retention_days"`
		DraftDays     int32 `json:"draft_days"`
	}
	Service struct {
		Pool  *pgxpool.Pool
		Clock func() time.Time
	}
)

func (s Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
}

func validate(f Fields) error {
	if (f.TravelDetails != nil && len(*f.TravelDetails) > 2000) ||
		(f.SocialAttendance != nil && !slices.Contains([]string{"unknown", "yes", "no"}, *f.SocialAttendance)) {
		return access.ErrInvalid
	}
	if len(f.ContactEmail) > 320 || len(f.ContactPhone) > 80 || len(f.Dietary) > 2000 ||
		len(f.Accessibility) > 2000 ||
		!slices.Contains([]string{"unknown", "yes", "no"}, f.PhotoConsent) {
		return access.ErrInvalid
	}
	if f.ContactEmail != "" {
		address, err := mail.ParseAddress(f.ContactEmail)
		if err != nil || address.Address != f.ContactEmail {
			return access.ErrInvalid
		}
	}
	return nil
}

func active(ctx context.Context, q *dbgen.Queries, actor uuid.UUID) error {
	p, err := q.Person(ctx, actor)
	if err != nil || !p.Active {
		return access.ErrForbidden
	}
	return nil
}

func project(row dbgen.OperationalProfileRow, roles []string, self bool) View {
	v := View{
		ID:          row.ID,
		EventID:     row.EventID,
		PersonID:    row.PersonID,
		DisplayName: row.DisplayName,
		Revision:    row.Revision,
	}
	if self || slices.Contains(roles, "logistics") {
		v.ContactEmail = &row.ContactEmail
		v.ContactPhone = &row.ContactPhone
		v.Dietary = &row.Dietary
		v.Accessibility = &row.Accessibility
		v.TravelDetails = &row.TravelDetails
		v.SocialAttendance = &row.SocialAttendance
	}
	if self || slices.Contains(roles, "photographer") {
		v.PhotoConsent = &row.PhotoConsent
	}
	return v
}

func (s Service) Read(ctx context.Context, actor, event, person uuid.UUID) (View, error) {
	q := dbgen.New(s.Pool)
	if err := active(ctx, q, actor); err != nil {
		return View{}, err
	}
	if actor != person {
		if err := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			event,
			person,
			access.ProfileRead,
		); err != nil {
			return View{}, fmt.Errorf("authorize profile fields: %w", err)
		}
	}
	row, err := q.OperationalProfile(
		ctx,
		dbgen.OperationalProfileParams{EventID: event, PersonID: person},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return View{}, access.ErrNotFound
	}
	if err != nil {
		return View{}, fmt.Errorf("load profile: %w", err)
	}
	roles, err := q.ActiveRoles(ctx, dbgen.ActiveRolesParams{EventID: event, PersonID: actor})
	if err != nil {
		return View{}, fmt.Errorf("load field policy: %w", err)
	}
	return project(row, roles, actor == person), nil
}

func (s Service) Correct(
	ctx context.Context,
	actor, event uuid.UUID,
	revision int64,
	fields Fields,
) (View, error) {
	if err := validate(fields); err != nil {
		return View{}, err
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, event)
		if err != nil {
			return fmt.Errorf("lock profile event: %w", err)
		}
		if err := active(ctx, q, actor); err != nil {
			return err
		}
		if !row.WritesAllowed {
			return access.ErrForbidden
		}
		previous, err := q.OperationalProfile(
			ctx,
			dbgen.OperationalProfileParams{EventID: event, PersonID: actor},
		)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			if revision != 0 {
				return access.ErrConflict
			}
			visible, err := q.VisibleEvent(
				ctx,
				dbgen.VisibleEventParams{EventID: event, PersonID: actor},
			)
			if err != nil {
				return fmt.Errorf("check own profile scope: %w", err)
			}
			state, err := q.CompetitionState(
				ctx,
				dbgen.CompetitionStateParams{EventID: event, PersonID: actor},
			)
			if err != nil {
				return fmt.Errorf("check registration scope: %w", err)
			}
			if !visible.Bool && (state == "" || state == "withdrawn") {
				return access.ErrForbidden
			}
		case err != nil:
			return fmt.Errorf("read previous profile: %w", err)
		case previous.Revision != revision:
			return access.ErrConflict
		}
		travel, social := previous.TravelDetails, previous.SocialAttendance
		if social == "" {
			social = "unknown"
		}
		if fields.TravelDetails != nil {
			travel = *fields.TravelDetails
		}
		if fields.SocialAttendance != nil {
			social = *fields.SocialAttendance
		}
		saved, err := q.SaveOperationalProfile(
			ctx,
			dbgen.SaveOperationalProfileParams{
				EventID:          event,
				PersonID:         actor,
				ContactEmail:     strings.TrimSpace(fields.ContactEmail),
				ContactPhone:     fields.ContactPhone,
				Dietary:          fields.Dietary,
				Accessibility:    fields.Accessibility,
				PhotoConsent:     fields.PhotoConsent,
				TravelDetails:    travel,
				SocialAttendance: social,
			},
		)
		if err != nil {
			return fmt.Errorf("save corrected profile: %w", err)
		}
		if previous.PhotoConsent != saved.PhotoConsent {
			if err = auditPhotoConsent(
				ctx,
				q,
				actor,
				event,
				saved.ID,
				saved.PhotoConsent,
				saved.PhotoConsentRevision,
			); err != nil {
				return err
			}
		}
		return audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &event,
				TargetID: saved.ID,
				Action:   audit.ProfileCorrected,
				Reason:   audit.UserRequest,
			},
		)
	})
	if err != nil {
		return View{}, fmt.Errorf("correct operational profile: %w", err)
	}
	return s.Read(ctx, actor, event, actor)
}

func (s Service) Export(
	ctx context.Context,
	actor, event, cursor uuid.UUID,
	limit int32,
) ([]View, error) {
	if limit < 1 || limit > 100 {
		return nil, access.ErrInvalid
	}
	result := []View{}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(ctx, event); err != nil {
			return fmt.Errorf("lock profile export: %w", err)
		}
		if err := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			event,
			event,
			access.ProfileExport,
		); err != nil {
			return fmt.Errorf("authorize logistics export: %w", err)
		}
		rows, err := q.ListOperationalProfiles(
			ctx,
			dbgen.ListOperationalProfilesParams{EventID: event, ID: cursor, Limit: limit},
		)
		if err != nil {
			return fmt.Errorf("query logistics fields: %w", err)
		}
		for _, row := range rows {
			result = append(
				result,
				project(dbgen.OperationalProfileRow(row), []string{"logistics"}, false),
			)
		}
		count := len(result)
		return audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &event,
				TargetID: event,
				Action:   audit.ProfileExported,
				Reason:   audit.UserRequest,
				Metadata: audit.Metadata{Count: &count},
			},
		)
	})
	if err != nil {
		return nil, fmt.Errorf("export operational profiles: %w", err)
	}
	return result, nil
}

func (s Service) Delete(ctx context.Context, actor, event, person uuid.UUID) error {
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(ctx, event); err != nil {
			return fmt.Errorf("lock profile removal: %w", err)
		}
		if err := active(ctx, q, actor); err != nil {
			return err
		}
		if actor != person {
			if err := (access.EventPolicy{Queries: q}).Authorize(
				ctx,
				actor,
				event,
				person,
				access.ProfileManage,
			); err != nil {
				return fmt.Errorf("authorize removal: %w", err)
			}
		}
		removed, err := q.DeleteOperationalProfile(
			ctx,
			dbgen.DeleteOperationalProfileParams{EventID: event, PersonID: person},
		)
		if err != nil {
			return fmt.Errorf("remove profile: %w", err)
		}
		if err = surveys.ErasePersonInTransaction(
			ctx,
			tx,
			&actor,
			event,
			person,
			audit.UserRequest,
		); err != nil {
			return fmt.Errorf("remove identifiable survey records: %w", err)
		}
		if err := q.RemovePersonDrafts(
			ctx,
			dbgen.RemovePersonDraftsParams{EventID: event, PersonID: person},
		); err != nil {
			return fmt.Errorf("remove private drafts: %w", err)
		}
		if removed == 0 {
			return nil
		}
		if err := q.CountRemovedProfile(ctx, event); err != nil {
			return fmt.Errorf("retain anonymous count: %w", err)
		}
		return audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &event,
				TargetID: person,
				Action:   audit.ProfileRemoved,
				Reason:   audit.UserRequest,
			},
		)
	})
	if err != nil {
		return fmt.Errorf("delete operational profile: %w", err)
	}
	return nil
}

func (s Service) Policy(ctx context.Context, actor, event uuid.UUID) (Settings, error) {
	q := dbgen.New(s.Pool)
	if err := (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		event,
		event,
		access.StaffRead,
	); err != nil {
		return Settings{}, fmt.Errorf("authorize retention policy: %w", err)
	}
	row, err := q.PrivacyPolicy(ctx, event)
	if err != nil {
		return Settings{}, fmt.Errorf("read privacy settings: %w", err)
	}
	return Settings{RetentionDays: row.RetentionDays, DraftDays: row.DraftDays}, nil
}

func (s Service) Configure(
	ctx context.Context,
	actor, event uuid.UUID,
	revision int64,
	settings Settings,
) error {
	if settings.RetentionDays < 1 || settings.RetentionDays > 3650 || settings.DraftDays < 1 ||
		settings.DraftDays > 90 {
		return access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, event)
		if err != nil {
			return fmt.Errorf("lock retention settings: %w", err)
		}
		if err := (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			event,
			event,
			access.EventEdit,
		); err != nil {
			return fmt.Errorf("authorize retention settings: %w", err)
		}
		if row.Revision != revision {
			return access.ErrConflict
		}
		if err := q.SetPrivacyPolicy(
			ctx,
			dbgen.SetPrivacyPolicyParams{
				EventID:       event,
				RetentionDays: settings.RetentionDays,
				DraftDays:     settings.DraftDays,
			},
		); err != nil {
			return fmt.Errorf("save retention settings: %w", err)
		}
		if _, err := q.BumpEventRevision(ctx, event); err != nil {
			return fmt.Errorf("advance retention revision: %w", err)
		}
		return audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &event,
				TargetID: event,
				Action:   audit.PrivacyConfigured,
				Reason:   audit.OrganiserDecision,
			},
		)
	})
	if err != nil {
		return fmt.Errorf("configure retention: %w", err)
	}
	return nil
}

// Retain is idempotent worker maintenance. It locks events in sorted order, the
// same order as corrections, then rechecks current stage/settings before removal.
func (s Service) Retain(ctx context.Context) error {
	now := s.now()
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		rows, err := q.ExpiredProfiles(ctx, pgtype.Timestamptz{Time: now, Valid: true})
		if err != nil {
			return fmt.Errorf("find expired operational fields: %w", err)
		}
		for _, profile := range rows {
			row, err := q.LockEvent(ctx, profile.EventID)
			if err != nil {
				return fmt.Errorf("lock retention event: %w", err)
			}
			settings, err := q.PrivacyPolicy(ctx, profile.EventID)
			if err != nil {
				return fmt.Errorf("recheck retention policy: %w", err)
			}
			if !slices.Contains([]string{"completed", "cancelled", "archived"}, row.Stage) ||
				!row.CompletedAt.Valid ||
				RetentionDeadline(row.CompletedAt.Time, settings.RetentionDays).After(now) {
				continue
			}
			removed, err := q.DeleteOperationalProfile(
				ctx,
				dbgen.DeleteOperationalProfileParams{
					EventID:  profile.EventID,
					PersonID: profile.PersonID,
				},
			)
			if err != nil {
				return fmt.Errorf("purge operational fields: %w", err)
			}
			if removed == 0 {
				continue
			}
			if err := q.RemovePersonDrafts(
				ctx,
				dbgen.RemovePersonDraftsParams{
					EventID:  profile.EventID,
					PersonID: profile.PersonID,
				},
			); err != nil {
				return fmt.Errorf("purge retained person's drafts: %w", err)
			}
			if err := q.CountRemovedProfile(ctx, profile.EventID); err != nil {
				return fmt.Errorf("count retained aggregate: %w", err)
			}
			if err := audit.Append(
				ctx,
				q,
				audit.Record{
					EventID:  &profile.EventID,
					TargetID: profile.ID,
					Action:   audit.ProfileRemoved,
					Reason:   audit.RetentionPolicy,
				},
			); err != nil {
				return fmt.Errorf("audit retention: %w", err)
			}
		}
		if _, err := q.RemoveExpiredDrafts(ctx, now); err != nil {
			return fmt.Errorf("purge expired drafts: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("apply retention policy: %w", err)
	}
	return nil
}

// StoreDraft is used by domain owners inside their authorized transaction. IDs
// cannot be transferred between people or events; retention bounds the lifetime.
func StoreDraft(
	ctx context.Context,
	q *dbgen.Queries,
	event, person, id uuid.UUID,
	body []byte,
	now time.Time,
) error {
	if id == uuid.Nil || len(body) > 65536 {
		return access.ErrInvalid
	}
	settings, err := q.PrivacyPolicy(ctx, event)
	if err != nil {
		return fmt.Errorf("load draft lifetime: %w", err)
	}
	changed, err := q.SavePrivateDraft(
		ctx,
		dbgen.SavePrivateDraftParams{
			ID:        id,
			EventID:   event,
			PersonID:  person,
			Body:      body,
			ExpiresAt: now.AddDate(0, 0, int(settings.DraftDays)),
		},
	)
	if err != nil {
		return fmt.Errorf("store private draft: %w", err)
	}
	if changed != 1 {
		return access.ErrConflict
	}
	return nil
}
