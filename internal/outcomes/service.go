package outcomes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/objectives"
)

func wrap(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("outcomes %s: %w", operation, err)
}

func active(ctx context.Context, q *dbgen.Queries, actor uuid.UUID) error {
	person, err := q.Person(ctx, actor)
	if err != nil || !person.Active {
		return access.ErrForbidden
	}
	return nil
}

func load(ctx context.Context, q *dbgen.Queries, id uuid.UUID, version int64) (Version, error) {
	row, err := q.OutcomeVersion(ctx, dbgen.OutcomeVersionParams{OutcomeID: id, Version: version})
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, access.ErrNotFound
	}
	if err != nil {
		return Version{}, wrap("load retained version", err)
	}
	var value Version
	if err = json.Unmarshal(row.Document, &value); err != nil {
		return value, wrap("decode retained version", err)
	}
	if value.ID != id || value.Version != version || value.CommandID != row.CommandID ||
		value.ActorID != row.ActorID {
		return Version{}, access.ErrConflict
	}
	return value, nil
}

func currentOrganiser(
	ctx context.Context,
	q *dbgen.Queries,
	actor uuid.UUID,
	eventID *uuid.UUID,
) (bool, error) {
	if eventID == nil {
		allowed, err := q.OutcomeAnyOrganiser(ctx, actor)
		return allowed, wrap("check organisation reviewer", err)
	}
	roles, err := q.ActiveRoles(ctx, dbgen.ActiveRolesParams{EventID: *eventID, PersonID: actor})
	return slices.Contains(roles, "organiser"), wrap("check current organiser", err)
}

func outputLive(row dbgen.Event, now time.Time) error {
	var config event.Configuration
	if err := json.Unmarshal(row.Configuration, &config); err != nil {
		return wrap("read live output boundary", err)
	}
	if !row.PublishedAt.Valid || now.Before(row.PublishedAt.Time) || now.Before(config.LiveStart) ||
		row.Stage == "cancelled" {
		return access.ErrForbidden
	}
	return nil
}

func capabilities(
	ctx context.Context,
	q *dbgen.Queries,
	actor uuid.UUID,
	head dbgen.OutcomeHead,
	now time.Time,
) (Capability, error) {
	var out Capability
	allowed, err := q.OutcomeReadable(
		ctx,
		dbgen.OutcomeReadableParams{OutcomeID: head.ID, ActorID: actor, AtTime: now},
	)
	if err != nil {
		return out, wrap("check current resource reader", err)
	}
	if !allowed {
		return out, access.ErrNotFound
	}
	organiser, err := currentOrganiser(ctx, q, actor, head.EventID)
	if err != nil {
		return out, err
	}
	team := false
	if head.EventID != nil && head.TeamID != nil {
		team, err = q.OutcomeCurrentTeamMember(
			ctx,
			dbgen.OutcomeCurrentTeamMemberParams{
				EventID: *head.EventID,
				TeamID:  *head.TeamID,
				ActorID: actor,
			},
		)
		if err != nil {
			return out, wrap("check current source team", err)
		}
	}
	owner := head.OwnerID != nil && *head.OwnerID == actor
	out.CanRevise = organiser ||
		head.Status == "proposed" && (head.AuthorID == actor && head.TeamID == nil || team)
	out.CanModerate = organiser
	out.CanFollowUp = owner || organiser
	out.CanAdopt = organiser && head.EventID == nil
	if head.EventID != nil {
		row, readErr := q.Event(ctx, *head.EventID)
		if readErr != nil {
			return out, wrap("read outcome event boundary", readErr)
		}
		out.Archived = row.Stage == "archived"
		if row.Stage == "cancelled" {
			out.CanRevise = false
			out.CanModerate = false
			out.CanFollowUp = false
			out.CanAdopt = false
		}
	}
	return out, nil
}

func project(
	ctx context.Context,
	q *dbgen.Queries,
	actor uuid.UUID,
	head dbgen.OutcomeHead,
	v Version,
	now time.Time,
) (View, error) {
	cap, err := capabilities(ctx, q, actor, head, now)
	if err != nil {
		return View{}, err
	}
	return View{Version: v, Capability: cap, CurrentVersion: head.CurrentVersion}, nil
}

func (s Service) Read(ctx context.Context, actor, id uuid.UUID, version int64) (View, error) {
	var out View
	if id == uuid.Nil || version < 0 {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if err := active(ctx, q, actor); err != nil {
			return err
		}
		head, err := q.OutcomeHead(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return wrap("read identity", err)
		}
		if _, err = capabilities(ctx, q, actor, head, s.now()); err != nil {
			return err
		}
		if version == 0 {
			version = head.CurrentVersion
		}
		v, err := load(ctx, q, id, version)
		if err != nil {
			return err
		}
		out, err = project(ctx, q, actor, head, v, s.now())
		return err
	})
	return out, wrap("read", err)
}

// lockWrite preserves event→resource lock order. A concurrent first adoption
// changes the discovered scope and requires an explicit fresh version instead
// of acquiring an event lock after an already-held resource lock.
func lockWrite(
	ctx context.Context,
	q *dbgen.Queries,
	id uuid.UUID,
	target *uuid.UUID,
) (dbgen.OutcomeHead, error) {
	seen, err := q.OutcomeHead(ctx, id)
	if err != nil {
		return seen, wrap("find write scope", err)
	}
	scope := seen.EventID
	if scope == nil {
		scope = target
	}
	if scope != nil {
		if _, err = q.LockEvent(ctx, *scope); err != nil {
			return seen, wrap("lock associated event", err)
		}
	}
	head, err := q.LockOutcomeHead(ctx, id)
	if err != nil {
		return head, wrap("lock current outcome", err)
	}
	if !sameID(head.EventID, seen.EventID) {
		return head, access.ErrConflict
	}
	return head, nil
}

func sameID(
	a, b *uuid.UUID,
) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func sameCommand(raw []byte, c Command) bool {
	var stored Command
	if json.Unmarshal(raw, &stored) != nil {
		return false
	}
	a, err := json.Marshal(stored)
	if err != nil {
		return false
	}
	b, err := json.Marshal(c)
	return err == nil && bytes.Equal(a, b)
}

func validateSource(
	ctx context.Context,
	q *dbgen.Queries,
	actor uuid.UUID,
	d Definition,
	p Provenance,
	now time.Time,
) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if d.Kind == "team_outcome" && (p.EventID == nil || p.TeamID == nil) {
		return access.ErrInvalid
	}
	if p.EventID == nil {
		return nil
	}
	if err := (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		*p.EventID,
		*p.EventID,
		access.EventRead,
	); err != nil {
		return wrap("authorize submitted event context", err)
	}
	row, err := q.Event(ctx, *p.EventID)
	if err != nil {
		return wrap("read source event", err)
	}
	if row.Stage == "cancelled" {
		return access.ErrForbidden
	}
	if p.TeamID != nil {
		member, memberErr := q.OutcomeCurrentTeamMember(
			ctx,
			dbgen.OutcomeCurrentTeamMemberParams{
				EventID: *p.EventID,
				TeamID:  *p.TeamID,
				ActorID: actor,
			},
		)
		if memberErr != nil {
			return wrap("check submitted own team", memberErr)
		}
		organiser, orgErr := currentOrganiser(ctx, q, actor, p.EventID)
		if orgErr != nil {
			return orgErr
		}
		if !member && !organiser {
			return access.ErrForbidden
		}
	}
	if d.Kind == "team_outcome" {
		return outputLive(row, now)
	}
	return nil
}

func (s Service) createProposal(
	ctx context.Context,
	q *dbgen.Queries,
	actor, id uuid.UUID,
	c Command,
) error {
	provenance := Provenance{}
	if c.Provenance != nil {
		provenance = *c.Provenance
	}
	if provenance.EventID != nil {
		if _, err := q.LockEvent(ctx, *provenance.EventID); err != nil {
			return wrap("lock proposed source", err)
		}
	}
	if err := validateSource(
		ctx,
		q,
		actor,
		*c.Definition,
		provenance,
		s.now(),
	); err != nil {
		return err
	}
	if _, err := q.CreateOutcomeHead(
		ctx,
		dbgen.CreateOutcomeHeadParams{
			ID:       id,
			AuthorID: actor,
			EventID:  provenance.EventID,
			TeamID:   provenance.TeamID,
		},
	); err != nil {
		return wrap("create identity", err)
	}
	return nil
}

func (s Service) Change(
	ctx context.Context,
	actor, id uuid.UUID,
	expected int64,
	c Command,
) (View, error) {
	var out View
	if id == uuid.Nil || expected < 0 {
		return out, access.ErrInvalid
	}
	if err := c.Validate(); err != nil {
		return out, err
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if err := active(ctx, q, actor); err != nil {
			return err
		}
		var target *uuid.UUID
		if c.Provenance != nil {
			target = c.Provenance.EventID
		}
		if c.Action == "propose" && expected == 0 {
			if err := s.createProposal(ctx, q, actor, id, c); err != nil {
				return err
			}
		}
		head, err := lockWrite(ctx, q, id, target)
		if err != nil {
			return err
		}
		cap, err := capabilities(ctx, q, actor, head, s.now())
		if err != nil {
			return err
		}
		replay, replayErr := q.OutcomeCommand(
			ctx,
			dbgen.OutcomeCommandParams{OutcomeID: id, CommandID: c.ID},
		)
		if replayErr == nil {
			if replay.ActorID != actor || replay.ExpectedVersion != expected ||
				!sameCommand(replay.Command, c) {
				return access.ErrConflict
			}
			v, readErr := load(ctx, q, id, replay.Version)
			if readErr != nil {
				return readErr
			}
			out, readErr = project(ctx, q, actor, head, v, s.now())
			return readErr
		}
		if !errors.Is(replayErr, pgx.ErrNoRows) {
			return wrap("read exact command", replayErr)
		}
		if head.CurrentVersion != expected {
			return access.ErrConflict
		}
		v := Version{
			ID:         id,
			AuthorID:   head.AuthorID,
			Provenance: Provenance{EventID: head.EventID, TeamID: head.TeamID},
			Status:     "proposed",
			Measures:   []objectives.MeasureReference{},
		}
		if expected > 0 {
			v, err = load(ctx, q, id, expected)
			if err != nil {
				return err
			}
		}
		v.EvidenceAddedID = nil
		if err = s.apply(ctx, tx, q, actor, head, cap, &v, c); err != nil {
			return err
		}
		v.Version = expected + 1
		v.CommandID = c.ID
		v.ActorID = actor
		v.At = s.now()
		v.Reason = c.Reason
		document, err := json.Marshal(v)
		if err != nil {
			return wrap("encode version", err)
		}
		command, err := json.Marshal(c)
		if err != nil {
			return wrap("encode command", err)
		}
		if err = q.AppendOutcomeVersion(
			ctx,
			dbgen.AppendOutcomeVersionParams{
				OutcomeID:       id,
				Version:         v.Version,
				CommandID:       c.ID,
				ActorID:         actor,
				ExpectedVersion: expected,
				Command:         command,
				Document:        document,
				CreatedAt:       v.At,
			},
		); err != nil {
			return wrap("append immutable version", err)
		}
		if err = appendEvidence(ctx, q, v, c); err != nil {
			return err
		}
		if err = q.UpdateOutcomeHead(
			ctx,
			dbgen.UpdateOutcomeHeadParams{
				ID:             id,
				EventID:        v.Provenance.EventID,
				TeamID:         v.Provenance.TeamID,
				OwnerID:        v.OwnerID,
				Status:         v.Status,
				CurrentVersion: v.Version,
			},
		); err != nil {
			return wrap("advance current identity", err)
		}
		action, err := outcomeAuditAction(ctx, q, v)
		if err != nil {
			return err
		}
		reason := audit.UserRequest
		if cap.CanModerate {
			reason = audit.OrganiserDecision
		}
		if err = audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  v.Provenance.EventID,
				TargetID: id,
				Action:   action,
				Reason:   reason,
				Metadata: audit.Metadata{Revision: &v.Version, PreviousRevision: &expected},
			},
		); err != nil {
			return wrap("audit outcome update", err)
		}
		head.EventID = v.Provenance.EventID
		head.TeamID = v.Provenance.TeamID
		head.OwnerID = v.OwnerID
		head.Status = v.Status
		head.CurrentVersion = v.Version
		out, err = project(ctx, q, actor, head, v, s.now())
		return err
	})
	return out, wrap("change", err)
}

func (s Service) apply(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	actor uuid.UUID,
	head dbgen.OutcomeHead,
	cap Capability,
	v *Version,
	c Command,
) error {
	switch c.Action {
	case "propose":
		if head.CurrentVersion != 0 || head.AuthorID != actor {
			return access.ErrConflict
		}
		v.Definition = *c.Definition
	case "revise":
		if !cap.CanRevise {
			return access.ErrForbidden
		}
		if c.Definition.Kind != v.Definition.Kind {
			return access.ErrInvalid
		}
		v.Definition = *c.Definition
	case "adopt":
		if !cap.CanAdopt || head.EventID != nil {
			return access.ErrForbidden
		}
		organiser, err := currentOrganiser(ctx, q, actor, c.Provenance.EventID)
		if err != nil {
			return err
		}
		if !organiser {
			return access.ErrForbidden
		}
		if err = validateSource(ctx, q, actor, v.Definition, *c.Provenance, s.now()); err != nil {
			return err
		}
		v.Provenance = *c.Provenance
	case "progress":
		if !cap.CanFollowUp || head.EventID == nil ||
			!slices.Contains([]string{"accepted", "in_progress"}, v.Status) {
			return access.ErrForbidden
		}
		v.Status = c.Status
	case "moderate":
		return applyModeration(ctx, tx, actor, cap, v, c)
	case "assign":
		if !cap.CanModerate || head.EventID == nil {
			return access.ErrForbidden
		}
		if c.OwnerID != nil {
			if err := active(ctx, q, *c.OwnerID); err != nil {
				return err
			}
			eligible, err := q.OutcomeEligibleOwner(
				ctx,
				dbgen.OutcomeEligibleOwnerParams{OutcomeID: head.ID, PersonID: *c.OwnerID},
			)
			if err != nil {
				return wrap("check current owner candidate", err)
			}
			if !eligible {
				return access.ErrInvalid
			}
		}
		v.OwnerID = c.OwnerID
		v.ReviewAt = c.ReviewAt
	case "evidence":
		return s.applyEvidence(ctx, q, head, cap, v, c)
	default:
		return access.ErrInvalid
	}
	return nil
}

func (s Service) applyEvidence(
	ctx context.Context,
	q *dbgen.Queries,
	head dbgen.OutcomeHead,
	cap Capability,
	v *Version,
	c Command,
) error {
	if !cap.CanFollowUp || v.Provenance.EventID == nil {
		return access.ErrForbidden
	}
	if c.Evidence.ObservedAt != nil && c.Evidence.ObservedAt.After(s.now()) {
		return access.ErrInvalid
	}
	if v.Definition.Kind == "team_outcome" {
		row, err := q.Event(ctx, *v.Provenance.EventID)
		if err != nil {
			return wrap("read follow-up live boundary", err)
		}
		if err = outputLive(row, s.now()); err != nil {
			return err
		}
	}
	for _, ref := range c.Evidence.Measures {
		if !slices.Contains(v.Measures, ref) {
			return access.ErrInvalid
		}
	}
	_, err := q.OutcomeEvidence(
		ctx,
		dbgen.OutcomeEvidenceParams{OutcomeID: head.ID, ID: c.Evidence.ID},
	)
	if err == nil {
		return access.ErrConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return wrap("read evidence identity", err)
	}
	v.EvidenceAddedID = &c.Evidence.ID
	return nil
}

func appendEvidence(ctx context.Context, q *dbgen.Queries, v Version, c Command) error {
	if c.Action == "evidence" {
		raw, e := json.Marshal(c.Evidence)
		if e != nil {
			return wrap("encode restricted evidence", e)
		}
		if e = q.AppendOutcomeEvidence(
			ctx,
			dbgen.AppendOutcomeEvidenceParams{
				OutcomeID:      v.ID,
				ID:             c.Evidence.ID,
				OutcomeVersion: v.Version,
				ActorID:        v.ActorID,
				Document:       raw,
				CreatedAt:      v.At,
			},
		); e != nil {
			return wrap("append restricted evidence", e)
		}
	}
	return nil
}

func applyModeration(
	ctx context.Context,
	tx pgx.Tx,
	actor uuid.UUID,
	cap Capability,
	v *Version,
	c Command,
) error {
	if !cap.CanModerate {
		return access.ErrForbidden
	}
	if len(c.Measures) > 0 && v.Provenance.EventID == nil {
		return access.ErrInvalid
	}
	for _, ref := range c.Measures {
		if _, err := objectives.ResolveMeasureInTransaction(
			ctx,
			tx,
			actor,
			*v.Provenance.EventID,
			ref,
		); err != nil {
			return wrap("attach exact measure", err)
		}
	}
	v.Status = c.Status
	if c.Measures != nil {
		v.Measures = c.Measures
	}
	return nil
}

func outcomeAuditAction(ctx context.Context, q *dbgen.Queries, v Version) (audit.Action, error) {
	action := audit.OutcomeChanged
	if v.Provenance.EventID != nil {
		row, readErr := q.Event(ctx, *v.Provenance.EventID)
		if readErr != nil {
			return "", wrap("read audited update boundary", readErr)
		}
		if row.Stage == "archived" {
			action = audit.OutcomeArchivedUpdated
		}
	}

	return action, nil
}
