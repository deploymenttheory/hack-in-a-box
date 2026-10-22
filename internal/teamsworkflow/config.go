package teamsworkflow

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
)

func revision(id uuid.UUID, version int64) string {
	return id.String() + ":" + strconv.FormatInt(version, 10)
}

func binding(eventID, id uuid.UUID, version int64) []byte {
	return []byte("hackbox:teams-workflow:" + eventID.String() + ":" + revision(id, version))
}

func authorize(ctx context.Context, q *dbgen.Queries, actor, eventID uuid.UUID, write bool) error {
	action := access.StaffRead
	if write {
		action = access.EventEdit
	}
	if err := (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		action,
	); err != nil {
		return fmt.Errorf("authorize workflow: %w", err)
	}
	return nil
}

func (s Service) open(c dbgen.TeamsWorkflowConnection) (string, error) {
	raw, err := s.Keys.Open(c.KeyID, c.Ciphertext, binding(c.EventID, c.ID, c.Version))
	if err != nil {
		return "", secretbox.ErrUnavailable
	}
	defer clear(raw)
	value := string(raw)
	if _, ok := validateEndpoint(value); !ok {
		return "", access.ErrInvalid
	}
	return value, nil
}

func (s Service) keyReady() bool {
	_, _, err := s.Keys.Seal(nil, []byte("hackbox:teams-readiness"))
	return err == nil
}

func (s Service) Read(ctx context.Context, actor, eventID uuid.UUID) (Page, error) {
	p := Page{History: []Receipt{}}
	if s.Pool == nil {
		return p, database.ErrDatabase
	}
	q := dbgen.New(s.Pool)
	if err := authorize(ctx, q, actor, eventID, false); err != nil {
		return p, fmt.Errorf("authorize Teams settings: %w", err)
	}
	event, err := q.Event(ctx, eventID)
	if err != nil {
		return p, fmt.Errorf("read Teams revision: %w", err)
	}
	p.Revision = event.Revision
	p.EncryptionReady = s.keyReady()
	c, err := q.TeamsWorkflowConnection(ctx, eventID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return p, fmt.Errorf("read Teams settings: %w", err)
	}
	if err == nil {
		_, openErr := s.open(c)
		p.Connection = &Connection{
			ID:        c.ID,
			Version:   c.Version,
			Label:     c.Label,
			Owner:     c.OwnerLabel,
			CoOwner:   c.CoOwnerLabel,
			Active:    c.Active,
			Available: openErr == nil,
			UpdatedAt: c.UpdatedAt,
		}
	}
	history, err := q.TeamsWorkflowHistory(ctx, eventID)
	if err != nil {
		return p, fmt.Errorf("read Teams receipts: %w", err)
	}
	for _, h := range history {
		r := Receipt{
			ID:                h.ID,
			Kind:              h.Kind,
			ConnectionVersion: h.ConnectionVersion,
			CampaignID:        h.CampaignID,
			CreatedAt:         h.CreatedAt,
			Status:            h.State,
			Reason:            h.Reason.String,
			Attempts:          h.AttemptCount,
		}
		switch {
		case h.Outcome.Valid:
			r.Status = h.Outcome.String
		case h.State == "running":
			r.Status = "sending"
			r.Reason = "request_in_progress"
		case h.AttemptCount > 0:
			r.Status = "ambiguous"
			r.Reason = "receipt_not_recorded"
		}
		if p.Connection != nil && h.ConnectionVersion != p.Connection.Version &&
			h.State == "pending" {
			r.Status = "requires_reapproval"
			r.Reason = "connection_changed"
		}
		if h.HttpStatus.Valid {
			v := h.HttpStatus.Int32
			r.HTTPStatus = &v
		}
		if h.RetryAt.Valid {
			v := h.RetryAt.Time
			r.RetryAt = &v
		}
		if h.RecordedAt.Valid {
			v := h.RecordedAt.Time
			r.RecordedAt = &v
		}
		p.History = append(p.History, r)
	}
	return p, nil
}

func (s Service) Change(
	ctx context.Context,
	actor, eventID uuid.UUID,
	expected int64,
	input Command,
) (Page, error) {
	if s.Pool == nil {
		return Page{}, database.ErrDatabase
	}
	if input.Action != "save" && input.Action != "revoke" && input.Action != "rotate" {
		return Page{}, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		event, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock Teams event: %w", err)
		}
		if err = authorize(ctx, q, actor, eventID, true); err != nil {
			return err
		}
		if event.Revision != expected {
			return access.ErrConflict
		}
		current, err := q.TeamsWorkflowConnection(ctx, eventID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read Teams connection: %w", err)
		}
		id, version := current.ID, current.Version+1
		if id == uuid.Nil {
			id = uuid.New()
		}
		label, owner, coOwner := current.Label, current.OwnerLabel, current.CoOwnerLabel
		endpoint := ""
		active := input.Action != "revoke"
		switch input.Action {
		case "save":
			label, owner, coOwner = input.Label, input.Owner, input.CoOwner
			if !validLabels(label, owner, coOwner) {
				return access.ErrInvalid
			}
			endpoint = input.Endpoint
			u, ok := validateEndpoint(endpoint)
			if !ok {
				return access.ErrInvalid
			}
			dnsCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			addresses, err := s.sender().lookup.LookupNetIP(dnsCtx, "ip", u.Hostname())
			if err != nil || len(addresses) == 0 {
				return access.ErrInvalid
			}
			for _, ip := range addresses {
				if !publicAddress(ip) {
					return access.ErrInvalid
				}
			}
		case "rotate":
			if current.ID == uuid.Nil || !current.Active {
				return access.ErrInvalid
			}
			endpoint, err = s.open(current)
			if err != nil {
				return err
			}
		case "revoke":
			if current.ID == uuid.Nil {
				return access.ErrNotFound
			}
		}
		keyID, ciphertext := "", []byte{}
		if active {
			keyID, ciphertext, err = s.Keys.Seal([]byte(endpoint), binding(eventID, id, version))
			if err != nil {
				return secretbox.ErrUnavailable
			}
		}
		err = q.SaveTeamsWorkflowConnection(
			ctx,
			dbgen.SaveTeamsWorkflowConnectionParams{
				ID:           id,
				EventID:      eventID,
				Version:      version,
				Label:        label,
				OwnerLabel:   owner,
				CoOwnerLabel: coOwner,
				KeyID:        keyID,
				Ciphertext:   ciphertext,
				Active:       active,
				UpdatedAt:    s.now(),
			},
		)
		if err != nil {
			return fmt.Errorf("save Teams settings: %w", err)
		}
		event, err = q.BumpEventRevision(ctx, eventID)
		if err != nil {
			return fmt.Errorf("advance Teams revision: %w", err)
		}
		result := "success"
		action := map[string]audit.Action{"save": audit.TeamsConnectionSaved, "rotate": audit.TeamsConnectionRotated, "revoke": audit.TeamsConnectionRevoked}[input.Action]
		return audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &eventID,
				TargetID: id,
				Action:   action,
				Reason:   audit.OrganiserDecision,
				Metadata: audit.Metadata{Result: &result, Revision: &event.Revision},
			},
		)
	})
	if err != nil {
		return Page{}, fmt.Errorf("change Teams settings: %w", err)
	}
	return s.Read(ctx, actor, eventID)
}

func validLabels(values ...string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" || len(value) > 160 ||
			strings.ContainsAny(value, "\r\n\x00") ||
			strings.Contains(value, "https://") {
			return false
		}
	}
	return true
}
