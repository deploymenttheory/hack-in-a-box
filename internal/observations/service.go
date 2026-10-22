package observations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

// guard uses the same event lock as role, conflict and assignment changes.
// An organiser may inspect history, but authorship never confers lasting access
// after the author's current assignment or mentor role is removed.
func guard(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID, teamID uuid.UUID,
) (dbgen.Event, bool, error) {
	row, err := q.LockEvent(ctx, eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, false, access.ErrNotFound
	}
	if err != nil {
		return row, false, fmt.Errorf("lock observation event: %w", err)
	}
	person, err := q.Person(ctx, actor)
	if err != nil || !person.Active {
		return row, false, access.ErrForbidden
	}
	team, err := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: eventID, ID: teamID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && team.State != "published") {
		return row, false, access.ErrNotFound
	}
	if err != nil {
		return row, false, fmt.Errorf("read observation team: %w", err)
	}
	assigned, err := q.ObservationMentorAssigned(
		ctx,
		dbgen.ObservationMentorAssignedParams{EventID: eventID, TeamID: teamID, PersonID: actor},
	)
	if err != nil {
		return row, false, fmt.Errorf("read current observation assignment: %w", err)
	}
	roles, err := q.ActiveRoles(ctx, dbgen.ActiveRolesParams{EventID: eventID, PersonID: actor})
	if err != nil {
		return row, false, fmt.Errorf("read observation roles: %w", err)
	}
	if !assigned && !slices.Contains(roles, "organiser") {
		return row, false, access.ErrForbidden
	}
	return row, assigned, nil
}

func (s Service) reviewOpen(row dbgen.Event) (bool, error) {
	var c event.Configuration
	if err := json.Unmarshal(row.Configuration, &c); err != nil {
		return false, fmt.Errorf("decode observation schedule: %w", err)
	}
	return row.PublishedAt.Valid && !s.now().Before(row.PublishedAt.Time) &&
		!c.LiveStart.IsZero() &&
		!s.now().Before(c.LiveStart), nil
}

func observation(row dbgen.MentorObservationVersion) Observation {
	return Observation{
		ID:         row.ID,
		TeamID:     row.TeamID,
		AuthorID:   row.AuthorID,
		Version:    row.Version,
		ObservedAt: row.ObservedAt,
		ChangedAt:  row.ChangedAt,
		Evidence: Evidence{
			Category: row.Category, Evidence: row.Evidence, Outcome: row.Outcome,
			FeedbackReference: row.FeedbackReference,
		},
	}
}

func status(row dbgen.MentorTeamStatusVersion) Status {
	return Status{
		Version:    row.Version,
		ActorID:    row.ActorID,
		State:      row.State,
		Reason:     row.Reason,
		NextAction: row.NextAction,
		ChangedAt:  row.ChangedAt,
	}
}

func (s Service) Read(ctx context.Context, actor, eventID, teamID uuid.UUID) (Page, error) {
	return s.ReadPage(ctx, actor, eventID, teamID, ReadOptions{})
}

func (s Service) ReadPage(
	ctx context.Context,
	actor, eventID, teamID uuid.UUID,
	options ReadOptions,
) (Page, error) {
	page := Page{Observations: []Observation{}}
	limit, err := pageLimit(options.Limit)
	if err != nil {
		return page, err
	}
	err = database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, assigned, err := guard(ctx, q, actor, eventID, teamID)
		if err != nil {
			return err
		}
		open, err := s.reviewOpen(row)
		if err != nil {
			return err
		}
		page.Revision = row.Revision
		page.CanRecord = assigned && row.WritesAllowed && open
		page.CanChangeStatus = assigned && row.WritesAllowed
		if open {
			notes, readErr := q.CurrentMentorObservations(
				ctx,
				dbgen.CurrentMentorObservationsParams{
					EventID: eventID, TeamID: teamID, AfterID: options.Cursor,
					PageSize: limit + 1,
				},
			)
			if readErr != nil {
				return fmt.Errorf("read private mentor observations: %w", readErr)
			}
			if len(notes) > int(limit) {
				notes = notes[:limit]
				cursor := notes[len(notes)-1].ID
				page.NextCursor = &cursor
			}
			for _, note := range notes {
				page.Observations = append(page.Observations, observation(note))
			}
		}
		current, err := q.CurrentMentorStatus(
			ctx,
			dbgen.CurrentMentorStatusParams{EventID: eventID, TeamID: teamID},
		)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read operational status: %w", err)
		}
		if err == nil {
			value := status(current)
			page.Status = &value
		}
		return nil
	})
	if err != nil {
		return Page{}, fmt.Errorf("read mentor evidence: %w", err)
	}
	return page, nil
}

func validate(input Command) error {
	switch input.Action {
	case "observation":
		if input.ID == uuid.Nil || input.Version < 0 ||
			strings.TrimSpace(input.Evidence.Category) == "" ||
			len(input.Evidence.Category) > 128 ||
			strings.TrimSpace(input.Evidence.Evidence) == "" ||
			len(input.Evidence.Evidence) > 30000 ||
			len(input.Evidence.Outcome) > 30000 || len(input.Evidence.FeedbackReference) > 256 {
			return access.ErrInvalid
		}
	case "status":
		if input.Version < 0 ||
			!slices.Contains([]string{"on_track", "at_risk", "blocked"}, input.State) ||
			strings.TrimSpace(input.Reason) == "" ||
			len(input.Reason) > 2000 ||
			len(input.NextAction) > 2000 {
			return access.ErrInvalid
		}
	default:
		return access.ErrInvalid
	}
	return nil
}

func (s Service) Change(
	ctx context.Context,
	actor, eventID, teamID uuid.UUID,
	revision int64,
	input Command,
) (Page, error) {
	if err := validate(input); err != nil {
		return Page{}, err
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, assigned, err := guard(ctx, q, actor, eventID, teamID)
		if err != nil {
			return err
		}
		if !assigned || !row.WritesAllowed {
			return access.ErrForbidden
		}
		if row.Revision != revision {
			return access.ErrConflict
		}
		action := audit.MentorStatusChanged
		if input.Action == "observation" {
			open, checkErr := s.reviewOpen(row)
			if checkErr != nil {
				return checkErr
			}
			if !open {
				return &event.Blocked{
					Issues: []event.Issue{
						{
							Field:   "observation",
							Code:    "live_start",
							Message: "Competition review opens at the scheduled live start after common challenge publication.",
						},
					},
				}
			}
			if err = s.writeObservation(ctx, q, actor, eventID, teamID, input); err != nil {
				return err
			}
			action = audit.MentorObservationChanged
		} else if err = s.writeStatus(ctx, q, actor, eventID, teamID, input); err != nil {
			return err
		}
		updated, err := q.BumpEventRevision(ctx, eventID)
		if err != nil {
			return fmt.Errorf("advance observation revision: %w", err)
		}
		return audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &eventID,
				TargetID: teamID,
				Action:   action,
				Reason:   audit.UserRequest,
				Metadata: audit.Metadata{PreviousRevision: &revision, Revision: &updated.Revision},
			},
		)
	})
	if err != nil {
		return Page{}, fmt.Errorf("change mentor evidence: %w", err)
	}
	return s.Read(ctx, actor, eventID, teamID)
}

func (s Service) writeObservation(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID, teamID uuid.UUID,
	input Command,
) error {
	prior, err := q.LatestMentorObservation(
		ctx,
		dbgen.LatestMentorObservationParams{EventID: eventID, TeamID: teamID, ID: input.ID},
	)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read original observation: %w", err)
	}
	observed := s.now()
	if err == nil {
		if prior.AuthorID != actor {
			return access.ErrForbidden
		}
		if prior.Version != input.Version {
			return access.ErrConflict
		}
		observed = prior.ObservedAt
	} else if input.Version != 0 {
		return access.ErrConflict
	}
	_, err = q.InsertMentorObservation(
		ctx,
		dbgen.InsertMentorObservationParams{
			EventID:           eventID,
			TeamID:            teamID,
			ID:                input.ID,
			Version:           input.Version + 1,
			AuthorID:          actor,
			ObservedAt:        observed,
			ChangedAt:         s.now(),
			Category:          input.Evidence.Category,
			Evidence:          input.Evidence.Evidence,
			Outcome:           input.Evidence.Outcome,
			FeedbackReference: input.Evidence.FeedbackReference,
		},
	)
	if err != nil {
		return fmt.Errorf("retain private observation revision: %w", err)
	}
	return nil
}

func (s Service) writeStatus(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID, teamID uuid.UUID,
	input Command,
) error {
	current, err := q.CurrentMentorStatus(
		ctx,
		dbgen.CurrentMentorStatusParams{EventID: eventID, TeamID: teamID},
	)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read current operational status: %w", err)
	}
	version := int64(0)
	if err == nil {
		version = current.Version
	}
	if version != input.Version {
		return access.ErrConflict
	}
	_, err = q.InsertMentorStatus(
		ctx,
		dbgen.InsertMentorStatusParams{
			EventID:    eventID,
			TeamID:     teamID,
			Version:    version + 1,
			ActorID:    actor,
			State:      input.State,
			Reason:     input.Reason,
			NextAction: input.NextAction,
			ChangedAt:  s.now(),
		},
	)
	if err != nil {
		return fmt.Errorf("retain operational status history: %w", err)
	}
	return nil
}
