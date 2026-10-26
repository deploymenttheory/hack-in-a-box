package certificates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

func digest(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", failure("settings", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func text(value string, max int) bool {
	return strings.TrimSpace(value) != "" && len(value) <= max &&
		!strings.ContainsFunc(value, unicode.IsControl)
}

func (s Service) guard(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	write bool,
) (dbgen.Event, bool, error) {
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, false, access.ErrNotFound
	}
	if err != nil {
		return row, false, failure("settings", err)
	}
	if err = (access.EventPolicy{Queries: q, Clock: s.now}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		access.EventRead,
	); err != nil {
		return row, false, access.ErrNotFound
	}
	roles, err := q.ActiveRoles(ctx, dbgen.ActiveRolesParams{EventID: eventID, PersonID: actor})
	staff := slices.Contains(roles, "organiser")
	if err != nil {
		return row, false, failure("settings", err)
	}
	if write {
		if !staff {
			return row, false, access.ErrForbidden
		}
		err = (access.EventPolicy{Queries: q, Clock: s.now}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.EventEdit,
		)
	}
	return row, staff, failure("settings", err)
}

func loadSettings(ctx context.Context, tx pgx.Tx, eventID uuid.UUID) (Settings, error) {
	out := Settings{Templates: []Template{}}
	var raw []byte
	err := tx.QueryRow(ctx, "SELECT document FROM certificate_settings WHERE event_id=$1 ORDER BY version DESC LIMIT 1", eventID).
		Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, failure("settings", err)
	}
	err = json.Unmarshal(raw, &out)
	return out, failure("settings", err)
}

func (s Service) Settings(ctx context.Context, actor, eventID uuid.UUID) (Settings, error) {
	var out Settings
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		_, staff, err := s.guard(ctx, tx, actor, eventID, false)
		if err != nil {
			return failure("settings", err)
		}
		if !staff {
			return access.ErrForbidden
		}
		out, err = loadSettings(ctx, tx, eventID)
		return failure("settings", err)
	})
	return out, failure("settings", err)
}

func validateSettings(row dbgen.Event, command SettingsCommand) error {
	if command.RequestID == uuid.Nil || command.Version < 0 || !command.Reviewed ||
		!text(command.Reason, 1000) ||
		len(command.Templates) == 0 ||
		len(command.Templates) > 20 {
		return access.ErrInvalid
	}
	ids := map[uuid.UUID]bool{}
	accent := regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	for _, t := range command.Templates {
		if t.ID == uuid.Nil || ids[t.ID] || (t.Kind != "attendance" && t.Kind != "award") ||
			!text(t.Recognition, 500) ||
			!text(t.Issuer, 160) ||
			!accent.MatchString(t.Accent) {
			return access.ErrInvalid
		}
		ids[t.ID] = true
	}
	if command.Attendance != nil {
		p := command.Attendance
		if len(p.Days) == 0 || len(p.Days) > 366 || p.MinimumPresent < 1 ||
			p.MinimumPresent > len(p.Days) {
			return access.ErrInvalid
		}
		var cfg event.Configuration
		if json.Unmarshal(row.Configuration, &cfg) != nil {
			return access.ErrInvalid
		}
		zone, err := time.LoadLocation(cfg.Timezone)
		if err != nil || cfg.LiveStart.IsZero() || !cfg.LiveEnd.After(cfg.LiveStart) {
			return access.ErrInvalid
		}
		first, last := cfg.LiveStart.In(zone).
			Format(time.DateOnly),
			cfg.LiveEnd.Add(-time.Nanosecond).
				In(zone).
				Format(time.DateOnly)
		seen := map[string]bool{}
		for _, day := range p.Days {
			parsed, err := time.Parse(time.DateOnly, day)
			if err != nil || parsed.Format(time.DateOnly) != day || day < first || day > last ||
				seen[day] {
				return access.ErrInvalid
			}
			seen[day] = true
		}
	}
	return nil
}

func (s Service) Configure(
	ctx context.Context,
	actor, eventID uuid.UUID,
	command SettingsCommand,
) (Settings, error) {
	var out Settings
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		row, _, err := s.guard(ctx, tx, actor, eventID, true)
		if err != nil {
			return failure("settings", err)
		}
		if err = validateSettings(row, command); err != nil {
			return failure("settings", err)
		}
		fp, err := digest(struct {
			Actor   uuid.UUID
			Command SettingsCommand
		}{actor, command})
		if err != nil {
			return failure("settings", err)
		}
		var old string
		var raw []byte
		err = tx.QueryRow(ctx, "SELECT fingerprint,document FROM certificate_settings WHERE event_id=$1 AND request_id=$2", eventID, command.RequestID).
			Scan(&old, &raw)
		if err == nil {
			if old != fp {
				return access.ErrConflict
			}
			return failure("decode retained settings", json.Unmarshal(raw, &out))
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return failure("settings", err)
		}
		current, err := loadSettings(ctx, tx, eventID)
		if err != nil {
			return failure("settings", err)
		}
		if current.Version != command.Version {
			return access.ErrConflict
		}
		out = Settings{
			Version:    current.Version + 1,
			Attendance: command.Attendance,
			Templates:  command.Templates,
		}
		raw, err = json.Marshal(out)
		if err != nil {
			return failure("settings", err)
		}
		_, err = tx.Exec(
			ctx,
			"INSERT INTO certificate_settings(event_id,version,request_id,fingerprint,document,actor_id,reason,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)",
			eventID,
			out.Version,
			command.RequestID,
			fp,
			raw,
			actor,
			command.Reason,
			s.now(),
		)
		return failure("settings", err)
	})
	if err != nil {
		return Settings{}, fmt.Errorf("certificate settings: %w", err)
	}
	return out, nil
}

func failure(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("certificate %s: %w", operation, err)
}
