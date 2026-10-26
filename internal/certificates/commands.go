package certificates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func loadCertificate(ctx context.Context, tx pgx.Tx, eventID, id uuid.UUID) (Certificate, error) {
	var out Certificate
	var raw []byte
	err := tx.QueryRow(ctx, "SELECT v.document FROM certificate_heads h JOIN certificate_versions v USING(event_id,id,version) WHERE h.event_id=$1 AND h.id=$2", eventID, id).
		Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, access.ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	return out, failure("commands", err)
}

func (s Service) Change(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	command Command,
) (View, error) {
	var out View
	if id == uuid.Nil || command.RequestID == uuid.Nil || command.Version < 0 ||
		!text(command.Reason, 1000) ||
		(command.Action != "issue" && command.Action != "correct" && command.Action != "revoke") {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		row, staff, err := s.guard(ctx, tx, actor, eventID, command.Action != "revoke")
		if err != nil {
			return failure("commands", err)
		}
		if !staff {
			return access.ErrForbidden
		}
		fp, err := digest(struct {
			Actor   uuid.UUID
			ID      uuid.UUID
			Command Command
		}{actor, id, command})
		if err != nil {
			return failure("commands", err)
		}
		var old string
		var raw []byte
		err = tx.QueryRow(ctx, "SELECT fingerprint,document FROM certificate_versions WHERE event_id=$1 AND request_id=$2", eventID, command.RequestID).
			Scan(&old, &raw)
		if err == nil {
			out, err = s.replay(ctx, tx, row, actor, id, command, fp, old, raw)
			return err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return failure("commands", err)
		}
		current, err := loadCertificate(ctx, tx, eventID, id)
		if command.Action == "issue" {
			if !errors.Is(err, access.ErrNotFound) {
				if err != nil {
					return failure("commands", err)
				}
				return access.ErrConflict
			}
			if command.Version != 0 {
				return access.ErrConflict
			}
		} else {
			if err != nil {
				return failure("commands", err)
			}
			if current.Version != command.Version || current.State != "issued" {
				return access.ErrConflict
			}
		}
		next := current
		next.ID = id
		next.EventID = eventID
		next.Version = command.Version + 1
		next.ChangedAt = s.now()
		key := ""
		if command.Action == "revoke" {
			if command.Target != current.Target || command.Binding != "" {
				return access.ErrInvalid
			}
			next.State = "revoked"
			err = tx.QueryRow(ctx, "SELECT recognition_key FROM certificate_heads WHERE event_id=$1 AND id=$2", eventID, id).
				Scan(&key)
			if err != nil {
				return failure("commands", err)
			}
		} else {
			next, key, err = s.recognise(ctx, tx, row, next, current, command)
			if err != nil {
				return err
			}
		}
		raw, err = json.Marshal(next)
		if err != nil {
			return failure("commands", err)
		}
		_, err = tx.Exec(
			ctx,
			"INSERT INTO certificate_versions(event_id,id,version,person_id,recognition_key,state,request_id,fingerprint,document,actor_id,reason,created_at,action) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)",
			eventID,
			id,
			next.Version,
			next.PersonID,
			key,
			next.State,
			command.RequestID,
			fp,
			raw,
			actor,
			command.Reason,
			s.now(),
			command.Action,
		)
		if err != nil {
			return failure("commands", err)
		}
		_, err = tx.Exec(
			ctx,
			"INSERT INTO certificate_heads(event_id,id,version,person_id,recognition_key) VALUES($1,$2,$3,$4,$5) ON CONFLICT(event_id,id) DO UPDATE SET version=EXCLUDED.version",
			eventID,
			id,
			next.Version,
			next.PersonID,
			key,
		)
		if err != nil {
			return failure("commands", err)
		}
		out, err = s.view(ctx, tx, row, actor, true, next)
		if err == nil {
			out.Receipt = &Receipt{
				RequestID:     command.RequestID,
				CertificateID: id,
				Version:       next.Version,
				Action:        command.Action,
				Fingerprint:   fp,
			}
		}
		return failure("commands", err)
	})
	if err != nil {
		return View{}, fmt.Errorf("certificate command: %w", err)
	}
	return out, nil
}

func (s Service) recognise(
	ctx context.Context,
	tx pgx.Tx,
	row dbgen.Event,
	next, current Certificate,
	command Command,
) (Certificate, string, error) {
	eligibility, recognition, err := s.eligibility(ctx, tx, row, command.Target)
	if err != nil {
		return next, "", failure("recognise", err)
	}
	if !eligibility.Eligible || eligibility.Binding != command.Binding {
		return next, "", access.ErrConflict
	}
	key := recognition
	if command.Action == "correct" {
		var oldKey string
		err = tx.QueryRow(ctx, "SELECT recognition_key FROM certificate_heads WHERE event_id=$1 AND id=$2", row.ID, current.ID).
			Scan(&oldKey)
		if err != nil {
			return next, "", failure("recognise", err)
		}
		if current.PersonID != command.Target.PersonID ||
			current.Kind != eligibility.Kind ||
			oldKey != key {
			return next, "", access.ErrInvalid
		}
	} else {
		var exists bool
		err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM certificate_heads WHERE event_id=$1 AND person_id=$2 AND recognition_key=$3)", row.ID, command.Target.PersonID, key).
			Scan(&exists)
		if err != nil {
			return next, "", failure("recognise", err)
		}
		if exists {
			return next, "", access.ErrConflict
		}
		next.IssuedAt = s.now()
	}
	next.PersonID = command.Target.PersonID
	next.Target = command.Target
	next.State = "issued"
	next.Kind = eligibility.Kind
	next.Fields = eligibility.Fields
	next.SettingsVersion = eligibility.SettingsVersion
	next.Binding = eligibility.Binding
	return next, key, nil
}

func (s Service) replay(
	ctx context.Context,
	tx pgx.Tx,
	row dbgen.Event,
	actor, id uuid.UUID,
	command Command,
	fingerprint, retained string,
	raw []byte,
) (View, error) {
	if retained != fingerprint {
		return View{}, access.ErrConflict
	}
	var prior Certificate
	if err := json.Unmarshal(raw, &prior); err != nil {
		return View{}, failure("decode original certificate", err)
	}
	out, err := s.view(ctx, tx, row, actor, true, prior)
	if err == nil {
		out.Receipt = &Receipt{
			RequestID:     command.RequestID,
			CertificateID: id,
			Version:       prior.Version,
			Action:        command.Action,
			Fingerprint:   fingerprint,
		}
	}
	return out, err
}
