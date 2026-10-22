// Package credentials owns supplied team secrets, current membership checks and
// durable disclosure auditing. It never provisions or revokes provider accounts.
package credentials

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
)

type (
	Service struct {
		Pool *pgxpool.Pool
		Keys *secretbox.Keyring
		Now  func() time.Time
	}
	Metadata struct {
		ID           uuid.UUID   `json:"id"`
		Label        string      `json:"label"`
		Version      int64       `json:"version"`
		Active       bool        `json:"active"`
		TeamIDs      []uuid.UUID `json:"team_ids"`
		UpdatedAt    time.Time   `json:"updated_at"`
		ResourceType string      `json:"resource_type"`
		AvailableAt  *time.Time  `json:"available_at"`
		ExpiresAt    *time.Time  `json:"expires_at"`
		Readiness    string      `json:"readiness"`
	}
)

type (
	Page struct {
		Revision  int64      `json:"revision"`
		CanManage bool       `json:"can_manage"`
		Items     []Metadata `json:"items"`
	}
	// Value exists only in set requests and explicit reveal responses. No arbitrary
	// decision text is recorded here because it could accidentally contain a value.
	Command struct {
		Action       string      `json:"action"`
		ID           uuid.UUID   `json:"id"`
		Label        string      `json:"label"`
		Value        string      `json:"value"`
		TeamIDs      []uuid.UUID `json:"team_ids"`
		ResourceType string      `json:"resource_type"`
		AvailableAt  *time.Time  `json:"available_at"`
		ExpiresAt    *time.Time  `json:"expires_at"`
	}
)

type Revealed struct {
	ID      uuid.UUID `json:"id"`
	Version int64     `json:"version"`
	Value   string    `json:"value"`
}

func binding(row dbgen.TeamCredential) []byte {
	return []byte(
		fmt.Sprintf("hackbox:team-credential:v1:%s:%s:%d", row.EventID, row.ID, row.Version),
	)
}

func manager(ctx context.Context, q *dbgen.Queries, actor, eventID uuid.UUID) bool {
	return (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		access.StaffRead,
	) == nil
}

func mayReveal(
	ctx context.Context,
	q *dbgen.Queries,
	actor uuid.UUID,
	row dbgen.TeamCredential,
	now time.Time,
) (bool, error) {
	if !row.Active || (row.AvailableAt.Valid && now.Before(row.AvailableAt.Time)) ||
		(row.ExpiresAt.Valid && !now.Before(row.ExpiresAt.Time)) {
		return false, nil
	}
	assignments, err := q.CredentialAssignments(ctx, row.EventID)
	if err != nil {
		return false, fmt.Errorf("read credential audience: %w", err)
	}
	for _, a := range assignments {
		if a.CredentialID != row.ID {
			continue
		}
		allowed, err := q.CredentialTeamAuthority(
			ctx,
			dbgen.CredentialTeamAuthorityParams{
				EventID:  row.EventID,
				PersonID: actor,
				TeamID:   a.TeamID,
			},
		)
		if err != nil {
			return false, fmt.Errorf("read credential audience: %w", err)
		}
		if allowed {
			return true, nil
		}
	}
	return false, nil
}

func metadata(
	row dbgen.TeamCredential,
	assignments []dbgen.TeamCredentialAssignment,
	now time.Time,
) Metadata {
	m := Metadata{
		ID:           row.ID,
		Label:        row.Label,
		Version:      row.Version,
		Active:       row.Active,
		UpdatedAt:    row.UpdatedAt,
		TeamIDs:      []uuid.UUID{},
		ResourceType: row.ResourceType,
		AvailableAt:  instant(row.AvailableAt), ExpiresAt: instant(row.ExpiresAt),
	}
	for _, a := range assignments {
		if a.CredentialID == row.ID {
			m.TeamIDs = append(m.TeamIDs, a.TeamID)
		}
	}
	m.Readiness = readiness(row, len(m.TeamIDs), now)
	return m
}

func load(
	ctx context.Context,
	q *dbgen.Queries,
	actor uuid.UUID,
	event dbgen.Event,
	now time.Time,
) (Page, error) {
	p := Page{
		Revision:  event.Revision,
		CanManage: manager(ctx, q, actor, event.ID),
		Items:     []Metadata{},
	}
	if err := (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		event.ID,
		event.ID,
		access.EventRead,
	); err != nil {
		return p, fmt.Errorf("read credential metadata: %w", err)
	}
	rows, err := q.TeamCredentials(ctx, event.ID)
	if err != nil {
		return p, fmt.Errorf("read credential metadata: %w", err)
	}
	assignments, err := q.CredentialAssignments(ctx, event.ID)
	if err != nil {
		return p, fmt.Errorf("read credential metadata: %w", err)
	}
	for _, row := range rows {
		if !p.CanManage {
			allowed, err := mayReveal(ctx, q, actor, row, now)
			if err != nil {
				return p, fmt.Errorf("read credential metadata: %w", err)
			}
			if !allowed {
				continue
			}
		}
		m := metadata(row, assignments, now)
		if !p.CanManage {
			m.TeamIDs = []uuid.UUID{}
		} // No other team's audience metadata.
		p.Items = append(p.Items, m)
	}
	return p, nil
}

func (s Service) List(ctx context.Context, actor, eventID uuid.UUID) (Page, error) {
	var page Page
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, eventID)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return database.ErrDatabase
		}
		page, err = load(ctx, q, actor, row, s.now())
		return err
	})
	if err != nil {
		return page, fmt.Errorf("credential operation: %w", err)
	}
	return page, nil
}

func (s Service) Change(
	ctx context.Context,
	actor, eventID uuid.UUID,
	revision int64,
	input Command,
) (Page, error) {
	var page Page
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		event, err := q.LockEvent(ctx, eventID)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return database.ErrDatabase
		}
		if err = (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.EventEdit,
		); err != nil {
			return fmt.Errorf("authorize credential management: %w", err)
		}
		if event.Revision != revision {
			return access.ErrConflict
		}
		row := dbgen.TeamCredential{ID: input.ID, EventID: eventID, Version: 1, Active: true}
		if input.Action == "create" {
			if input.ID != uuid.Nil {
				return access.ErrInvalid
			}
			row.ID = uuid.New()
		} else {
			row, err = q.TeamCredential(
				ctx,
				dbgen.TeamCredentialParams{EventID: eventID, ID: input.ID},
			)
			if errors.Is(err, pgx.ErrNoRows) {
				return access.ErrNotFound
			}
			if err != nil {
				return database.ErrDatabase
			}
		}
		row, err = s.prepare(row, input)
		if err != nil {
			return err
		}

		if err = q.SaveTeamCredential(
			ctx,
			dbgen.SaveTeamCredentialParams{
				ID:           row.ID,
				EventID:      eventID,
				Label:        row.Label,
				Version:      row.Version,
				KeyID:        row.KeyID,
				Ciphertext:   row.Ciphertext,
				Active:       row.Active,
				ResourceType: row.ResourceType,
				AvailableAt:  row.AvailableAt,
				ExpiresAt:    row.ExpiresAt,
			},
		); err != nil {
			return database.ErrDatabase
		}
		if err = assign(ctx, q, row, input); err != nil {
			return err
		}

		result := "success"
		action := map[string]audit.Action{"create": audit.CredentialCreated, "replace": audit.CredentialReplaced, "assign": audit.CredentialAssigned, "revoke": audit.CredentialRevoked, "metadata": audit.CredentialMetadataChanged, "rotate": audit.CredentialRotated}[input.Action]

		if err = audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &eventID,
				TargetID: row.ID,
				Action:   action,
				Reason:   audit.OrganiserDecision,
				Metadata: audit.Metadata{Revision: &row.Version, Result: &result},
			},
		); err != nil {
			return database.ErrDatabase
		}
		event, err = q.BumpEventRevision(ctx, eventID)
		if err != nil {
			return database.ErrDatabase
		}
		page, err = load(ctx, q, actor, event, s.now())
		return err
	})
	if err != nil {
		return page, fmt.Errorf("credential operation: %w", err)
	}
	return page, nil
}

// Reveal commits both denied and successful results. Returning a policy/key error
// inside the transaction would roll back the very audit evidence we need. Plain
// text leaves this method only after audit commit succeeds. Cancellation cannot
// erase a disclosure attempt once it has entered the service.
func (s Service) Reveal(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	revision int64,
) (Revealed, error) {
	operationCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var revealed Revealed
	var outcome error
	err := database.Within(operationCtx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		event, e := q.LockEvent(operationCtx, eventID)
		var auditEvent *uuid.UUID
		result := "denied"
		switch {
		case errors.Is(e, pgx.ErrNoRows):
			outcome = access.ErrNotFound
			result = "not_found"
		case e != nil:
			return database.ErrDatabase
		default:
			auditEvent = &eventID
			row, e := q.TeamCredential(
				operationCtx,
				dbgen.TeamCredentialParams{EventID: eventID, ID: id},
			)
			switch {
			case errors.Is(e, pgx.ErrNoRows):
				outcome = access.ErrNotFound
				result = "not_found"
			case e != nil:
				return database.ErrDatabase
			default:
				allowed, e := mayReveal(operationCtx, q, actor, row, s.now())
				if e != nil {
					return database.ErrDatabase
				}
				switch {
				case !allowed:
					outcome = access.ErrForbidden
				case event.Revision != revision:
					outcome = access.ErrConflict
					result = "conflict"
				default:
					plain, e := s.Keys.Open(row.KeyID, row.Ciphertext, binding(row))
					if e != nil {
						outcome = secretbox.ErrUnavailable
						result = "unavailable"
					} else {
						revealed = Revealed{ID: id, Version: row.Version, Value: string(plain)}
						clear(plain)
						result = "success"
					}
				}
			}
		}
		return audit.Append(
			operationCtx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  auditEvent,
				TargetID: id,
				Action:   audit.CredentialRevealed,
				Reason:   audit.UserRequest,
				Metadata: audit.Metadata{Result: &result},
			},
		)
	})
	if err != nil {
		return Revealed{}, database.ErrDatabase
	}
	if outcome != nil {
		return Revealed{}, outcome
	}
	return revealed, nil
}

func (s Service) prepare(row dbgen.TeamCredential, input Command) (dbgen.TeamCredential, error) {
	var err error
	switch input.Action {
	case "create", "replace":
		if input.Action == "create" {
			row, err = publicMetadata(row, input)
			if err != nil {
				return row, err
			}
		}
		if len(input.Value) < 1 || len(input.Value) > 16384 ||
			strings.TrimSpace(input.Label) == "" ||
			len(input.Label) > 120 {
			return row, access.ErrInvalid
		}
		if input.Action == "replace" {
			row.Version++
		}
		row.Label = strings.TrimSpace(input.Label)
		row.KeyID, row.Ciphertext, err = s.Keys.Seal([]byte(input.Value), binding(row))
		if err != nil {
			return row, fmt.Errorf("encrypt credential: %w", err)
		}
	case "metadata":
		if input.Value != "" || len(input.TeamIDs) > 0 {
			return row, access.ErrInvalid
		}
		row, err = publicMetadata(row, input)
		if err != nil {
			return row, err
		}
	case "assign":
		if input.Value != "" || input.Label != "" {
			return row, access.ErrInvalid
		}
	case "revoke":
		if input.Value != "" || input.Label != "" || len(input.TeamIDs) > 0 {
			return row, access.ErrInvalid
		}
		row.Active = false
	case "rotate":
		if input.Value != "" || input.Label != "" || len(input.TeamIDs) > 0 {
			return row, access.ErrInvalid
		}
		plain, openErr := s.Keys.Open(row.KeyID, row.Ciphertext, binding(row))
		if openErr != nil {
			return row, fmt.Errorf("decrypt credential for rotation: %w", openErr)
		}
		row.Version++
		row.KeyID, row.Ciphertext, err = s.Keys.Seal(plain, binding(row))
		clear(plain)
		if err != nil {
			return row, fmt.Errorf("encrypt credential: %w", err)
		}
	default:
		return row, access.ErrInvalid
	}
	return row, nil
}

func assign(ctx context.Context, q *dbgen.Queries, row dbgen.TeamCredential, input Command) error {
	eventID := row.EventID
	var err error
	if input.Action == "create" || input.Action == "assign" {
		if len(input.TeamIDs) > 1000 {
			return access.ErrInvalid
		}
		if err = q.ClearCredentialAssignments(
			ctx,
			dbgen.ClearCredentialAssignmentsParams{EventID: eventID, CredentialID: row.ID},
		); err != nil {
			return database.ErrDatabase
		}
		seen := map[uuid.UUID]bool{}
		for _, team := range input.TeamIDs {
			if seen[team] {
				return access.ErrInvalid
			}
			seen[team] = true
			t, e := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: eventID, ID: team})
			if e != nil || t.State == "disbanded" {
				return access.ErrInvalid
			}
			if err = q.AssignTeamCredential(
				ctx,
				dbgen.AssignTeamCredentialParams{
					EventID:      eventID,
					CredentialID: row.ID,
					TeamID:       team,
				},
			); err != nil {
				return database.ErrDatabase
			}
		}
	} else if len(input.TeamIDs) > 0 {
		return access.ErrInvalid
	}
	return nil
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func instant(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time.UTC()
	return &t
}

func nullable(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func readiness(row dbgen.TeamCredential, assigned int, now time.Time) string {
	switch {
	case !row.Active:
		return "revoked"
	case row.ExpiresAt.Valid && !now.Before(row.ExpiresAt.Time):
		return "expired"
	case assigned == 0:
		return "unassigned"
	case row.AvailableAt.Valid && now.Before(row.AvailableAt.Time):
		return "scheduled"
	default:
		return "ready"
	}
}

func publicMetadata(row dbgen.TeamCredential, input Command) (dbgen.TeamCredential, error) {
	kind := input.ResourceType
	if kind == "" && input.Action == "create" {
		kind = "credential"
	}
	if !slices.Contains(
		[]string{"credential", "account", "licence", "access_code", "other"},
		kind,
	) ||
		strings.TrimSpace(input.Label) == "" ||
		len(input.Label) > 120 {
		return row, access.ErrInvalid
	}
	if input.AvailableAt != nil && input.ExpiresAt != nil &&
		!input.ExpiresAt.After(*input.AvailableAt) {
		return row, access.ErrInvalid
	}
	row.Label = strings.TrimSpace(input.Label)
	row.ResourceType = kind
	row.AvailableAt = nullable(input.AvailableAt)
	row.ExpiresAt = nullable(input.ExpiresAt)
	return row, nil
}
