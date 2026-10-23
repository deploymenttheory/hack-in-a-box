package privacy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

type PhotoConsent struct {
	PersonID    uuid.UUID `json:"person_id"`
	DisplayName string    `json:"display_name"`
	Choice      string    `json:"choice"`
	Eligible    bool      `json:"eligible"`
	Token       string    `json:"-"`
}

// PhotoConsentBinding is internal immutable review provenance. It exposes no
// profile fields or choice history and cannot grant any media/file authority.
type PhotoConsentBinding struct {
	PersonID uuid.UUID `json:"person_id"`
	Token    string    `json:"token"`
}
type PhotoGuidance struct {
	Items      []PhotoConsent `json:"items"`
	NextCursor *uuid.UUID     `json:"next_cursor"`
}
type OwnPhotoConsent struct {
	Choice    string `json:"choice"`
	Revision  int64  `json:"revision"`
	CanOptIn  bool   `json:"can_opt_in"`
	CanRevoke bool   `json:"can_revoke"`
}

func photoRetention(
	ctx context.Context,
	q *dbgen.Queries,
	event dbgen.Event,
	now time.Time,
) (bool, error) {
	policy, err := q.PrivacyPolicy(ctx, event.ID)
	if err != nil {
		return false, fmt.Errorf("read photo-consent retention: %w", err)
	}
	expired := event.CompletedAt.Valid &&
		slices.Contains([]string{"completed", "cancelled", "archived"}, event.Stage) &&
		!now.Before(RetentionDeadline(event.CompletedAt.Time, policy.RetentionDays))
	return expired, nil
}

func photoSubject(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, subject uuid.UUID,
	expired bool,
) (PhotoConsent, error) {
	out := PhotoConsent{PersonID: subject, Choice: "unknown"}
	row, err := q.PhotoConsentSubject(
		ctx,
		dbgen.PhotoConsentSubjectParams{EventID: eventID, PersonID: subject},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("read native photo-consent subject: %w", err)
	}
	out.Eligible = row.Active && row.Eligible && !expired
	if !out.Eligible {
		return out, nil
	}
	out.DisplayName = row.DisplayName
	out.Choice = row.PhotoConsent
	if row.ProfileID != nil && row.ConsentRevision > 0 {
		sum := sha256.Sum256(
			[]byte(row.ProfileID.String() + ":" + strconv.FormatInt(row.ConsentRevision, 10)),
		)
		out.Token = hex.EncodeToString(sum[:])
	}
	return out, nil
}

func photoGuideAuthority(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	now time.Time,
) (*dbgen.Queries, bool, error) {
	q := dbgen.New(tx)
	event, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return q, false, fmt.Errorf("lock photo-consent event: %w", err)
	}
	roles, _, err := access.ContentReader(ctx, q, actor, eventID)
	if err != nil {
		return q, false, fmt.Errorf("authorize consent guide reader: %w", err)
	}
	if !slices.Contains(roles, "photographer") && !slices.Contains(roles, "organiser") {
		return q, false, access.ErrForbidden
	}
	expired, err := photoRetention(ctx, q, event, now)
	return q, expired, err
}

// PhotoConsentsInTransaction projects only current consent guidance for the
// current event's eligible people. It never returns contact/registration fields.
func (s Service) PhotoConsentsInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	ids []uuid.UUID,
) ([]PhotoConsent, error) {
	out := []PhotoConsent{}
	if len(ids) > 100 {
		return out, access.ErrInvalid
	}
	q, expired, err := photoGuideAuthority(ctx, tx, actor, eventID, s.now())
	if err != nil {
		return out, err
	}
	for _, id := range ids {
		value, e := photoSubject(ctx, q, eventID, id, expired)
		if e != nil {
			return out, e
		}
		out = append(out, value)
	}
	return out, nil
}

func (s Service) PhotoGuideInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, cursor uuid.UUID,
	limit int32,
) (PhotoGuidance, error) {
	out := PhotoGuidance{Items: []PhotoConsent{}}
	if limit < 1 || limit > 100 {
		return out, access.ErrInvalid
	}
	q, expired, err := photoGuideAuthority(ctx, tx, actor, eventID, s.now())
	if err != nil {
		return out, err
	}
	if expired {
		return out, nil
	}
	ids, err := q.PhotoConsentRoster(
		ctx,
		dbgen.PhotoConsentRosterParams{EventID: eventID, Cursor: cursor, PageSize: limit + 1},
	)
	if err != nil {
		return out, fmt.Errorf("page current photographer consent guide: %w", err)
	}
	if len(ids) > int(limit) {
		ids = ids[:limit]
		last := ids[len(ids)-1]
		out.NextCursor = &last
	}
	for _, id := range ids {
		value, e := photoSubject(ctx, q, eventID, id, false)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, value)
	}
	return out, nil
}

// ValidatePhotoConsentsInTransaction checks persisted media review provenance.
// The owning caller must separately authorize the actual resource/current reader;
// this check grants no person, gallery, original-file or thumbnail access.
func (s Service) ValidatePhotoConsentsInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	bindings []PhotoConsentBinding,
) error {
	if len(bindings) > 100 {
		return access.ErrInvalid
	}
	q := dbgen.New(tx)
	event, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("lock reviewed photo-consent validation: %w", err)
	}
	expired, err := photoRetention(ctx, q, event, s.now())
	if err != nil {
		return err
	}
	seen := map[uuid.UUID]bool{}
	for _, binding := range bindings {
		if binding.PersonID == uuid.Nil || binding.Token == "" || seen[binding.PersonID] {
			return access.ErrConflict
		}
		seen[binding.PersonID] = true
		value, e := photoSubject(ctx, q, eventID, binding.PersonID, expired)
		if e != nil {
			return e
		}
		if !value.Eligible || value.Choice != "yes" || value.Token != binding.Token {
			return access.ErrConflict
		}
	}
	return nil
}

func (s Service) OwnPhotoConsent(
	ctx context.Context,
	actor, eventID uuid.UUID,
) (OwnPhotoConsent, error) {
	var out OwnPhotoConsent
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		var e error
		out, e = s.ownPhotoConsent(ctx, tx, actor, eventID)
		return e
	})
	if err != nil {
		return out, fmt.Errorf("read own photo consent: %w", err)
	}
	return out, nil
}

func (s Service) ownPhotoConsent(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
) (OwnPhotoConsent, error) {
	out := OwnPhotoConsent{Choice: "unknown"}
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return out, fmt.Errorf("lock own photo-consent scope: %w", err)
	}
	if err = active(ctx, q, actor); err != nil {
		return out, err
	}
	profile, err := q.OperationalProfile(
		ctx,
		dbgen.OperationalProfileParams{EventID: eventID, PersonID: actor},
	)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, fmt.Errorf("read own native photo consent: %w", err)
	}
	state, subjectErr := q.PhotoConsentSubject(
		ctx,
		dbgen.PhotoConsentSubjectParams{EventID: eventID, PersonID: actor},
	)
	if subjectErr != nil {
		return out, fmt.Errorf("read own event consent scope: %w", subjectErr)
	}
	if errors.Is(err, pgx.ErrNoRows) && !state.Eligible {
		return out, access.ErrForbidden
	}
	expired, err := photoRetention(ctx, q, row, s.now())
	if err != nil {
		return out, err
	}
	if !expired && profile.ID != uuid.Nil {
		out.Choice = profile.PhotoConsent
		out.Revision = profile.Revision
	}
	out.CanOptIn = row.WritesAllowed && state.Eligible && !expired
	out.CanRevoke = !expired && (profile.ID != uuid.Nil || state.Eligible)
	return out, nil
}

func (s Service) SetOwnPhotoConsent(
	ctx context.Context,
	actor, eventID uuid.UUID,
	revision int64,
	choice string,
) (OwnPhotoConsent, error) {
	if !slices.Contains([]string{"unknown", "yes", "no"}, choice) {
		return OwnPhotoConsent{}, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		current, e := s.ownPhotoConsent(ctx, tx, actor, eventID)
		if e != nil {
			return e
		}
		if current.Revision != revision {
			return access.ErrConflict
		}
		if (choice == "yes" && !current.CanOptIn) || (choice != "yes" && !current.CanRevoke) {
			return access.ErrForbidden
		}
		saved, e := q.SetOwnPhotoConsent(
			ctx,
			dbgen.SetOwnPhotoConsentParams{EventID: eventID, PersonID: actor, PhotoConsent: choice},
		)
		if e != nil {
			return fmt.Errorf("save own photo-consent choice: %w", e)
		}
		return auditPhotoConsent(
			ctx,
			q,
			actor,
			eventID,
			saved.ID,
			choice,
			saved.PhotoConsentRevision,
		)
	})
	if err != nil {
		return OwnPhotoConsent{}, fmt.Errorf("change own photo consent: %w", err)
	}
	return s.OwnPhotoConsent(ctx, actor, eventID)
}

func auditPhotoConsent(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID, profileID uuid.UUID,
	choice string,
	revision int64,
) error {
	action := audit.PhotoConsentUnknown
	switch choice {
	case "yes":
		action = audit.PhotoConsentGranted
	case "no":
		action = audit.PhotoConsentRevoked
	}
	err := audit.Append(
		ctx,
		q,
		audit.Record{
			ActorID:  &actor,
			EventID:  &eventID,
			TargetID: profileID,
			Action:   action,
			Reason:   audit.UserRequest,
			Metadata: audit.Metadata{Revision: &revision},
		},
	)
	if err != nil {
		return fmt.Errorf("append native decision audit: %w", err)
	}
	return nil
}
