package submissions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/templates"
)

type Service struct {
	Pool  *pgxpool.Pool
	Clock func() time.Time
}

func (s Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
}

type scope struct {
	row       dbgen.Event
	config    event.Configuration
	authority access.TeamAuthority
	state     dbgen.SubmissionState
	entry     dbgen.CompetitionEntry
}

func configuration(row dbgen.Event) (event.Configuration, error) {
	c := event.DefaultConfiguration()
	if err := json.Unmarshal(row.Configuration, &c); err != nil {
		return c, fmt.Errorf("decode submission dates: %w", err)
	}
	return c, nil
}

func (s Service) scoped(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
) (scope, error) {
	v, err := s.authorizedScope(ctx, tx, actor, eventID, teamID)
	if err != nil {
		return v, err
	}
	if err = s.Settle(ctx, tx, v.row, s.now()); err != nil {
		return v, err
	}
	return loadScopeState(ctx, dbgen.New(tx), v, eventID, teamID)
}

func (s Service) authorizedScope(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
) (scope, error) {
	var v scope
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, access.ErrNotFound
	}
	if err != nil {
		return v, fmt.Errorf("lock submission event: %w", err)
	}
	v.row = row
	v.authority, err = access.TeamPermissions(ctx, q, actor, eventID, teamID)
	if err != nil {
		return v, fmt.Errorf("authorize team submission: %w", err)
	}
	a := v.authority
	if !a.Member && !a.Organiser && !a.Mentor && !a.Reviewer && !a.Judge {
		return v, access.ErrForbidden
	}
	v.config, err = configuration(row)
	if err != nil {
		return v, err
	}
	if !a.Member && !a.Organiser &&
		(!published(row, s.now()) || v.config.LiveStart.IsZero() || s.now().Before(v.config.LiveStart)) {
		return v, access.ErrForbidden
	}
	if !a.Member && !a.Organiser && !a.Mentor && !a.Reviewer &&
		(v.config.SubmissionCutoff.IsZero() || s.now().Before(v.config.SubmissionCutoff)) {
		return v, access.ErrForbidden
	}
	return v, nil
}

func loadScopeState(
	ctx context.Context,
	q *dbgen.Queries,
	v scope,
	eventID, teamID uuid.UUID,
) (scope, error) {
	var err error
	a := v.authority

	if err = q.EnsureSubmissionState(
		ctx,
		dbgen.EnsureSubmissionStateParams{EventID: eventID, TeamID: teamID},
	); err != nil {
		return v, fmt.Errorf("initialize submission state: %w", err)
	}
	v.state, err = q.SubmissionState(
		ctx,
		dbgen.SubmissionStateParams{EventID: eventID, TeamID: teamID},
	)
	if err != nil {
		return v, fmt.Errorf("read submission state: %w", err)
	}
	if !a.Member && !a.Organiser && !a.Mentor && !a.Reviewer && v.state.Outcome != "frozen" {
		return v, access.ErrForbidden
	}
	v.entry, err = q.CompetitionEntry(
		ctx,
		dbgen.CompetitionEntryParams{EventID: eventID, TeamID: teamID},
	)
	if err != nil {
		return v, fmt.Errorf("read competition entry: %w", err)
	}
	return v, nil
}

func published(row dbgen.Event, now time.Time) bool {
	return row.PublishedAt.Valid && !now.Before(row.PublishedAt.Time)
}

func live(v scope, now time.Time) bool {
	return published(v.row, now) && !v.config.LiveStart.IsZero() && !now.Before(v.config.LiveStart)
}

func open(v scope, now time.Time) bool {
	return !v.config.SubmissionCutoff.IsZero() && now.Before(v.config.SubmissionCutoff)
}

func draftAllowed(v scope, now time.Time) bool {
	return v.row.WritesAllowed && v.authority.Member && v.authority.Active &&
		published(v.row, now) &&
		open(v, now) &&
		v.state.Outcome != "withdrawn"
}

func definitions(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	c event.Configuration,
) ([]Definition, error) {
	result := []Definition{}
	raw, err := q.EventDefinitions(ctx, row.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read deliverables: %w", err)
	}
	var defs templates.Definitions
	if err = json.Unmarshal(raw.Definitions, &defs); err != nil {
		return nil, fmt.Errorf("decode deliverables: %w", err)
	}
	for _, d := range defs.Deliverables {
		due := c.SubmissionCutoff
		if d.ScheduleID != nil {
			found := false
			for _, item := range c.Schedule {
				if item.ID == *d.ScheduleID {
					found = true
					due, err = event.Plan(c, item)
					if err != nil {
						return nil, fmt.Errorf("resolve deliverable deadline: %w", err)
					}
					break
				}
			}
			if !found {
				return nil, blocked(
					"deliverables",
					"missing_schedule",
					"Choose an existing deadline for each deliverable.",
				)
			}
		}
		if due.IsZero() || c.LiveStart.IsZero() || due.Before(c.LiveStart) ||
			due.After(c.SubmissionCutoff) {
			return nil, blocked(
				"deliverables",
				"invalid_deadline",
				"Competition deliverables must be due between live start and the shared cutoff.",
			)
		}
		result = append(result, Definition{Deliverable: d, DueAt: due})
	}
	return result, nil
}

func visible(d Definition, a access.TeamAuthority) bool {
	return a.Member || a.Organiser ||
		((a.Mentor || a.Reviewer) && slices.Contains(audience(d), "mentor")) ||
		(a.Judge && slices.Contains(audience(d), "judge"))
}

func filtered(doc Document, a access.TeamAuthority) Document {
	result := Document{Definitions: []Definition{}, Evidence: []Evidence{}}
	ids := map[uuid.UUID]bool{}
	for _, d := range doc.Definitions {
		if visible(d, a) {
			result.Definitions = append(result.Definitions, d)
			ids[d.ID] = true
		}
	}
	for _, e := range doc.Evidence {
		if ids[e.DeliverableID] {
			result.Evidence = append(result.Evidence, e)
		}
	}
	return result
}

func revision(ctx context.Context, q *dbgen.Queries, v scope, n int64) (*Revision, error) {
	row, err := q.SubmissionRevision(
		ctx,
		dbgen.SubmissionRevisionParams{EventID: v.row.ID, TeamID: v.entry.TeamID, Revision: n},
	)
	if err != nil {
		return nil, fmt.Errorf("read formal revision: %w", err)
	}
	r := Revision{
		Revision:     row.Revision,
		ActorID:      row.ActorID,
		SubmittedAt:  row.SubmittedAt,
		Late:         row.Late,
		Reason:       row.Reason,
		ReviewStatus: "pending",
	}
	if err = json.Unmarshal(row.Document, &r.Document); err != nil {
		return nil, fmt.Errorf("decode formal evidence: %w", err)
	}
	r.Document = filtered(r.Document, v.authority)
	review, err := q.LatestSubmissionReview(
		ctx,
		dbgen.LatestSubmissionReviewParams{
			EventID:        v.row.ID,
			TeamID:         v.entry.TeamID,
			ActiveRevision: nullableRevision(n),
		},
	)
	if err == nil {
		r.ReviewStatus = review
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("read latest review: %w", err)
	}
	return &r, nil
}

func (s Service) page(ctx context.Context, q *dbgen.Queries, v scope) (Page, error) {
	now := s.now()
	result := Page{
		EventRevision: v.row.Revision,
		Revision:      v.state.Revision,
		EntryID:       v.entry.ID,
		TeamID:        v.entry.TeamID,
		Outcome:       v.state.Outcome,
		LiveStart:     v.config.LiveStart,
		Cutoff:        v.config.SubmissionCutoff,
		Definitions:   []Definition{},
	}
	if v.state.FrozenAt.Valid {
		result.FrozenAt = &v.state.FrozenAt.Time
	}
	result.CanReadDraft = v.authority.Member && published(v.row, now)
	result.Organisers = []Organiser{}
	contacts, err := q.SubmissionOrganisers(ctx, v.row.ID)
	if err != nil {
		return result, fmt.Errorf("read submission organisers: %w", err)
	}
	for _, p := range contacts {
		result.Organisers = append(result.Organisers, Organiser{ID: p.ID, Name: p.DisplayName})
	}

	// Competition prose may reveal the common challenge. A planned release time
	// grants no access; organisers retain their private authoring view.
	if v.authority.Organiser || published(v.row, now) {
		defs, err := definitions(ctx, q, v.row, v.config)
		if err != nil {
			return result, err
		}
		result.Definitions = filtered(Document{Definitions: defs}, v.authority).Definitions
	}
	result.CanDraft = draftAllowed(v, now)
	result.CanSubmit = v.row.WritesAllowed && v.authority.Active && live(v, now) &&
		(v.authority.Organiser || (v.authority.Member && open(v, now) && v.state.Outcome == "open" && !v.state.FrozenAt.Valid))
	if v.state.ActiveRevision.Valid && live(v, now) {
		result.Final, err = revision(ctx, q, v, v.state.ActiveRevision.Int64)
		if err != nil {
			return result, err
		}
	}
	result.CanReview = v.row.WritesAllowed && live(v, now) && v.authority.Organiser &&
		v.authority.Active &&
		result.Final != nil &&
		v.state.Outcome != "withdrawn" &&
		v.state.Outcome != "not_submitted"
	return result, nil
}

func (s Service) Read(ctx context.Context, actor, eventID, teamID uuid.UUID) (Page, error) {
	var result Page
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		v, err := s.scoped(ctx, tx, actor, eventID, teamID)
		if err != nil {
			return err
		}
		result, err = s.page(ctx, q, v)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("read team submission: %w", err)
	}
	return result, nil
}

func loadDraft(ctx context.Context, q *dbgen.Queries, eventID, teamID uuid.UUID) (Draft, error) {
	result := Draft{Evidence: []Evidence{}}
	row, err := q.SubmissionDraft(
		ctx,
		dbgen.SubmissionDraftParams{EventID: eventID, TeamID: teamID},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("read private draft: %w", err)
	}
	result.Revision = row.Revision
	result.UpdatedAt = &row.UpdatedAt
	if err = json.Unmarshal(row.Evidence, &result.Evidence); err != nil {
		return result, fmt.Errorf("decode private draft: %w", err)
	}
	return result, nil
}

func (s Service) Draft(ctx context.Context, actor, eventID, teamID uuid.UUID) (Draft, error) {
	var result Draft
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		v, err := s.scoped(ctx, tx, actor, eventID, teamID)
		if err != nil {
			return err
		}
		if !v.authority.Member || !published(v.row, s.now()) {
			return access.ErrForbidden
		}
		result, err = loadDraft(ctx, q, eventID, teamID)
		return err
	})
	if err != nil {
		return result, fmt.Errorf("read working draft: %w", err)
	}
	return result, nil
}

func (s Service) SaveDraft(
	ctx context.Context,
	actor, eventID, teamID uuid.UUID,
	expected int64,
	items []Evidence,
) (Draft, error) {
	var result Draft
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		v, err := s.scoped(ctx, tx, actor, eventID, teamID)
		if err != nil {
			return err
		}
		if !draftAllowed(v, s.now()) {
			return access.ErrForbidden
		}
		old, err := loadDraft(ctx, q, eventID, teamID)
		if err != nil {
			return err
		}
		if old.Revision != expected {
			return access.ErrConflict
		}
		defs, err := definitions(ctx, q, v.row, v.config)
		if err != nil {
			return err
		}
		if err = validateEvidence(defs, items, false); err != nil {
			return err
		}
		if err = checkFiles(ctx, q, actor, v, items, s.now()); err != nil {
			return err
		}
		data, err := json.Marshal(items)
		if err != nil {
			return fmt.Errorf("encode draft: %w", err)
		}
		now := s.now()
		result = Draft{
			Revision:  expected + 1,
			Evidence:  append([]Evidence{}, items...),
			UpdatedAt: &now,
		}
		if err = q.SaveSubmissionDraft(
			ctx,
			dbgen.SaveSubmissionDraftParams{
				EventID:   eventID,
				TeamID:    teamID,
				Revision:  result.Revision,
				ActorID:   actor,
				UpdatedAt: now,
				Evidence:  data,
			},
		); err != nil {
			return fmt.Errorf("save private draft: %w", err)
		}
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("save working draft: %w", err)
	}
	return result, nil
}
