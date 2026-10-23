package outcomes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func publicationVersion(row dbgen.OutcomeProjectVersion) (PublicationVersion, error) {
	var v PublicationVersion
	if err := json.Unmarshal(row.Document, &v); err != nil {
		return v, wrap("decode publication review", err)
	}
	if v.OutcomeID != row.OutcomeID || v.Version != row.Version || v.CommandID != row.CommandID ||
		v.ActorID != row.ActorID {
		return v, access.ErrConflict
	}
	return v, nil
}

func publicationReplay(raw []byte, c PublicationCommand) bool {
	var stored PublicationCommand
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

func requirePublicationSource(
	ctx context.Context,
	q *dbgen.Queries,
	actor uuid.UUID,
	head dbgen.OutcomeHead,
	now time.Time,
) (dbgen.Event, error) {
	if head.EventID == nil {
		return dbgen.Event{}, access.ErrInvalid
	}
	cap, err := capabilities(ctx, q, actor, head, now)
	if err != nil {
		return dbgen.Event{}, err
	}
	if !cap.CanModerate {
		return dbgen.Event{}, access.ErrForbidden
	}
	row, err := q.Event(ctx, *head.EventID)
	return row, wrap("read publication source event", err)
}

func (s Service) Publication(ctx context.Context, actor, id uuid.UUID) (PublicationView, error) {
	var out PublicationView
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if err := active(ctx, q, actor); err != nil {
			return err
		}
		head, err := q.OutcomeHead(ctx, id)
		if err != nil {
			return wrap("read publication identity", err)
		}
		if _, err = requirePublicationSource(ctx, q, actor, head, s.now()); err != nil {
			return err
		}
		ph, err := q.OutcomeProjectHead(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return wrap("read current publication", err)
		}
		row, err := q.OutcomeProjectVersion(
			ctx,
			dbgen.OutcomeProjectVersionParams{OutcomeID: id, Version: ph.CurrentVersion},
		)
		if err != nil {
			return wrap("read current publication version", err)
		}
		v, err := publicationVersion(row)
		if err != nil {
			return err
		}
		out = PublicationView{PublicationVersion: v, CurrentVersion: ph.CurrentVersion}
		if ph.PublishedVersion.Valid {
			out.PublishedVersion = &ph.PublishedVersion.Int64
		}
		return nil
	})
	return out, wrap("publication review", err)
}

func (s Service) ChangePublication(
	ctx context.Context,
	actor, id uuid.UUID,
	expected int64,
	c PublicationCommand,
) (PublicationView, error) {
	var out PublicationView
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
		head, err := lockWrite(ctx, q, id, nil)
		if err != nil {
			return err
		}
		event, err := requirePublicationSource(ctx, q, actor, head, s.now())
		if err != nil {
			return err
		}
		if err = q.CreateOutcomeProjectHead(ctx, id); err != nil {
			return wrap("create publication identity", err)
		}
		ph, err := q.OutcomeProjectHead(ctx, id)
		if err != nil {
			return wrap("read publication base", err)
		}
		replay, replayErr := q.OutcomeProjectCommand(
			ctx,
			dbgen.OutcomeProjectCommandParams{OutcomeID: id, CommandID: c.ID},
		)
		if replayErr == nil {
			out, err = publicationReceipt(replay, ph, actor, expected, c)
			return err
		}
		if !errors.Is(replayErr, pgx.ErrNoRows) {
			return wrap("read publication command", replayErr)
		}
		if ph.CurrentVersion != expected || c.SourceVersion != head.CurrentVersion {
			return access.ErrConflict
		}
		source, err := load(ctx, q, id, head.CurrentVersion)
		if err != nil {
			return err
		}
		v := PublicationVersion{OutcomeID: id, SourceVersion: c.SourceVersion}
		if expected > 0 {
			prior, readErr := q.OutcomeProjectVersion(
				ctx,
				dbgen.OutcomeProjectVersionParams{OutcomeID: id, Version: expected},
			)
			if readErr != nil {
				return wrap("read publication review base", readErr)
			}
			v, err = publicationVersion(prior)
			if err != nil {
				return err
			}
		}
		public, err := s.applyPublication(
			ctx,
			tx,
			q,
			actor,
			event,
			head,
			source,
			ph,
			expected,
			c,
			&v,
		)
		if err != nil {
			return err
		}
		v.Version = expected + 1
		v.CommandID = c.ID
		v.ActorID = actor
		v.Reason = c.Reason
		v.At = s.now()
		public.Version = v.Version
		if err = appendPublication(ctx, q, v, c, expected, public); err != nil {
			return err
		}
		switch c.Action {
		case "publish":
			ph.PublishedVersion = pgtype.Int8{Int64: v.Version, Valid: true}
		case "withdraw":
			ph.PublishedVersion = pgtype.Int8{}
		}
		if err = q.UpdateOutcomeProjectHead(
			ctx,
			dbgen.UpdateOutcomeProjectHeadParams{
				OutcomeID:        id,
				CurrentVersion:   v.Version,
				PublishedVersion: ph.PublishedVersion,
			},
		); err != nil {
			return wrap("advance publication identity", err)
		}
		if err = audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  head.EventID,
				TargetID: id,
				Action:   audit.OutcomeProjectChanged,
				Reason:   audit.OrganiserDecision,
				Metadata: audit.Metadata{Revision: &v.Version, PreviousRevision: &expected},
			},
		); err != nil {
			return wrap("audit publication decision", err)
		}
		out = PublicationView{PublicationVersion: v, CurrentVersion: v.Version}
		if ph.PublishedVersion.Valid {
			out.PublishedVersion = &ph.PublishedVersion.Int64
		}
		return nil
	})
	return out, wrap("change publication", err)
}

func (s Service) publicProjection(
	ctx context.Context,
	tx pgx.Tx,
	actor uuid.UUID,
	row dbgen.OutcomeProjectVersion,
) (Project, error) {
	var out Project
	if err := json.Unmarshal(row.PublishedFields, &out); err != nil {
		return out, wrap("decode positive project projection", err)
	}
	if out.ID != row.OutcomeID || out.Version != row.Version || row.State != "published" {
		return Project{}, access.ErrNotFound
	}
	review, err := publicationVersion(row)
	if err != nil {
		return out, err
	}
	for _, ref := range review.Fields.Media {
		if s.Media == nil {
			continue
		}
		link, readErr := s.Media.ResolveReferenceInTransaction(ctx, tx, actor, ref)
		if errors.Is(readErr, access.ErrNotFound) || errors.Is(readErr, access.ErrForbidden) ||
			errors.Is(readErr, access.ErrConflict) {
			continue
		}
		if readErr != nil {
			return Project{}, wrap("recheck current media reader", readErr)
		}
		link.Media = true
		out.Links = append(out.Links, link)
	}
	return out, nil
}

func (s Service) Public(ctx context.Context, actor, id uuid.UUID) (Project, error) {
	var out Project
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if err := active(ctx, q, actor); err != nil {
			return err
		}
		row, err := q.OutcomePublishedProject(
			ctx,
			dbgen.OutcomePublishedProjectParams{OutcomeID: id, ActorID: actor, AtTime: s.now()},
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return wrap("read currently permitted published project", err)
		}
		out, err = s.publicProjection(ctx, tx, actor, row)
		return err
	})
	return out, wrap("public project", err)
}

func (s Service) Projects(
	ctx context.Context,
	actor, cursor uuid.UUID,
	search string,
	limit int32,
) (ProjectPage, error) {
	out := ProjectPage{Items: []Project{}}
	if limit < 1 || limit > 100 || !text(search, 200) {
		return out, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if err := active(ctx, q, actor); err != nil {
			return err
		}
		rows, err := q.OutcomePublishedProjectsPage(
			ctx,
			dbgen.OutcomePublishedProjectsPageParams{
				AfterID:  cursor,
				ActorID:  actor,
				AtTime:   s.now(),
				Search:   search,
				PageSize: limit + 1,
			},
		)
		if err != nil {
			return wrap("search permitted published projects", err)
		}
		more := len(rows) > int(limit)
		if more {
			rows = rows[:limit]
		}
		for _, row := range rows {
			item, projectErr := s.publicProjection(ctx, tx, actor, row)
			if projectErr != nil {
				return projectErr
			}
			out.Items = append(out.Items, item)
		}
		if more {
			next := rows[len(rows)-1].OutcomeID
			out.NextCursor = &next
		}
		return nil
	})
	return out, wrap("projects", err)
}

func (s Service) publishProjection(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	actor uuid.UUID,
	event dbgen.Event,
	head dbgen.OutcomeHead,
	source Version,
	v *PublicationVersion,
) (Project, error) {
	public := Project{ID: head.ID, Links: []PublicLink{}, Source: source.Provenance}
	if source.Definition.ThemeSensitive &&
		(!event.PublishedAt.Valid || s.now().Before(event.PublishedAt.Time)) {
		return Project{}, access.ErrForbidden
	}
	for _, ref := range v.Fields.Media {
		if s.Media == nil {
			return Project{}, access.ErrConflict
		}
		if _, err := s.Media.ResolveReferenceInTransaction(ctx, tx, actor, ref); err != nil {
			return Project{}, wrap("review current media reference", err)
		}
	}
	v.State = "published"
	public.Problem = v.Fields.Problem
	public.Solution = v.Fields.Solution
	public.FollowUpStatus = source.Status
	public.PublishedAt = s.now()
	if v.Fields.ShareEventName {
		public.EventName = &event.Name
	}
	if v.Fields.ShareTeamName && head.TeamID != nil {
		team, readErr := q.HackTeam(
			ctx,
			dbgen.HackTeamParams{EventID: *head.EventID, ID: *head.TeamID},
		)
		if readErr != nil {
			return Project{}, wrap("resolve intentionally shared source team", readErr)
		}
		public.TeamName = &team.Name
	}
	for _, link := range v.Fields.Links {
		public.Links = append(public.Links, PublicLink{Label: link.Label, URL: link.URL})
	}

	return public, nil
}

func appendPublication(
	ctx context.Context,
	q *dbgen.Queries,
	v PublicationVersion,
	c PublicationCommand,
	expected int64,
	public Project,
) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return wrap("encode publication version", err)
	}
	cmd, err := json.Marshal(c)
	if err != nil {
		return wrap("encode publication command", err)
	}
	projected, err := json.Marshal(public)
	if err != nil {
		return wrap("encode positive publication projection", err)
	}
	if err = q.AppendOutcomeProjectVersion(
		ctx,
		dbgen.AppendOutcomeProjectVersionParams{
			OutcomeID:       v.OutcomeID,
			Version:         v.Version,
			SourceVersion:   v.SourceVersion,
			CommandID:       c.ID,
			ActorID:         v.ActorID,
			ExpectedVersion: expected,
			Command:         cmd,
			Document:        raw,
			State:           v.State,
			PublishedFields: projected,
			CreatedAt:       v.At,
		},
	); err != nil {
		return wrap("append publication version", err)
	}
	return nil
}

func publicationReceipt(
	replay dbgen.OutcomeProjectVersion,
	ph dbgen.OutcomeProjectHead,
	actor uuid.UUID,
	expected int64,
	c PublicationCommand,
) (PublicationView, error) {
	if replay.ActorID != actor || replay.ExpectedVersion != expected ||
		!publicationReplay(replay.Command, c) {
		return PublicationView{}, access.ErrConflict
	}
	v, readErr := publicationVersion(replay)
	if readErr != nil {
		return PublicationView{}, readErr
	}
	out := PublicationView{PublicationVersion: v, CurrentVersion: ph.CurrentVersion}
	if ph.PublishedVersion.Valid {
		out.PublishedVersion = &ph.PublishedVersion.Int64
	}
	return out, nil
}

func (s Service) applyPublication(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	actor uuid.UUID,
	event dbgen.Event,
	head dbgen.OutcomeHead,
	source Version,
	ph dbgen.OutcomeProjectHead,
	expected int64,
	c PublicationCommand,
	v *PublicationVersion,
) (Project, error) {
	var err error
	public := Project{ID: head.ID, Links: []PublicLink{}, Source: source.Provenance}
	switch c.Action {
	case "save":
		if c.Fields.Audience == "team" && head.TeamID == nil ||
			c.Fields.ShareTeamName && head.TeamID == nil {
			return Project{}, access.ErrInvalid
		}
		v.Fields = *c.Fields
		if v.Fields.Links == nil {
			v.Fields.Links = []ProjectLink{}
		}
		if v.Fields.Media == nil {
			v.Fields.Media = []MediaReference{}
		}
		v.State = "draft"
		v.SourceVersion = c.SourceVersion
	case "publish":
		if expected == 0 || v.State != "draft" || v.SourceVersion != head.CurrentVersion {
			return Project{}, access.ErrConflict
		}
		public, err = s.publishProjection(ctx, tx, q, actor, event, head, source, v)
		if err != nil {
			return Project{}, err
		}
	case "withdraw":
		if !ph.PublishedVersion.Valid {
			return Project{}, access.ErrConflict
		}
		v.State = "withdrawn"
	default:
		return Project{}, access.ErrInvalid
	}

	return public, nil
}
