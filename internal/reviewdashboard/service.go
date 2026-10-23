package reviewdashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
	"github.com/deploymenttheory/hack-in-a-box/internal/moderation"
	"github.com/deploymenttheory/hack-in-a-box/internal/observations"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
)

type readScope struct {
	row       dbgen.Event
	dates     event.Configuration
	organiser bool
	roster    mentors.Page
}

func (s Service) guard(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
) (readScope, error) {
	var out readScope
	q := dbgen.New(tx)
	row, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return out, fmt.Errorf("lock review coverage event: %w", err)
	}
	if err = (access.EventPolicy{Queries: q}).Authorize(
		ctx,
		actor,
		eventID,
		eventID,
		access.EventRead,
	); err != nil {
		return out, fmt.Errorf("authorize review coverage event: %w", err)
	}
	roles, _, err := access.ContentReader(ctx, q, actor, eventID)
	if err != nil {
		return out, fmt.Errorf("read current review coverage role: %w", err)
	}
	out.organiser = slices.Contains(roles, "organiser")
	if !out.organiser && !slices.Contains(roles, "mentor") {
		return out, access.ErrForbidden
	}
	if err = json.Unmarshal(row.Configuration, &out.dates); err != nil {
		return out, fmt.Errorf("read review coverage dates: %w", err)
	}
	out.roster, err = (mentors.Service{Pool: s.Pool}).ReadInTransaction(ctx, tx, actor, eventID)
	if err != nil {
		return out, fmt.Errorf("read authorised review coverage scope: %w", err)
	}
	out.row = row
	return out, nil
}

func (s Service) Read(
	ctx context.Context,
	actor, eventID, after uuid.UUID,
	limit int32,
) (Page, error) {
	out := Page{Teams: []Team{}}
	if limit < 1 || limit > 50 {
		return out, access.ErrInvalid
	}
	if s.Cards == nil || s.Moderation == nil || s.Feedback == nil {
		return out, access.ErrForbidden
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		scope, err := s.guard(ctx, tx, actor, eventID)
		if err != nil {
			return err
		}
		candidates, err := teamPage(ctx, tx, eventID, after, scope, limit)
		if err != nil {
			return err
		}
		if len(candidates) > int(limit) {
			candidates = candidates[:limit]
			id := candidates[len(candidates)-1].ID
			out.NextCursor = &id
		}
		out.EventID = eventID
		out.EventName = scope.row.Name
		out.Timezone = scope.dates.Timezone
		out.Now = s.now()
		out.Organiser = scope.organiser
		helpQueue, err := s.Help.ListInTransaction(ctx, tx, actor, eventID)
		if err != nil {
			return fmt.Errorf("read authorised operational help metadata: %w", err)
		}
		source := submissions.Service{Pool: s.Pool, Clock: s.Clock}
		ids := []uuid.UUID{}
		for _, team := range candidates {
			if _, entryErr := q.CompetitionEntry(
				ctx,
				dbgen.CompetitionEntryParams{EventID: eventID, TeamID: team.ID},
			); errors.Is(
				entryErr,
				pgx.ErrNoRows,
			) {
				continue
			} else if entryErr != nil {
				return fmt.Errorf("read current team entry identity: %w", entryErr)
			}
			if scope.organiser ||
				(!scope.dates.LiveStart.IsZero() && !s.now().Before(scope.dates.LiveStart) && scope.row.PublishedAt.Valid && !s.now().Before(scope.row.PublishedAt.Time)) {
				ids = append(ids, team.ID)
			}
		}
		outcomes, err := source.SummariesInTransaction(ctx, tx, actor, eventID, ids)
		if err != nil {
			return fmt.Errorf("read canonical formal-output facts: %w", err)
		}
		for _, candidate := range candidates {
			row := Team{
				ID:             candidate.ID,
				Name:           candidate.Name,
				State:          candidate.State,
				Assignments:    []Assignment{},
				CoverageIssues: []string{},
				Agenda:         []moderation.AgendaItem{},
				Support: Support{
					Availability: "not_applicable",
					Risk:         "unknown",
					Operational:  observations.SupportStatus{State: "unknown"},
					Help:         helpMetrics(helpQueue.Items, candidate.ID, s.now()),
				},
				Assessment: Assessment{Availability: "not_open", OutputState: "not_open"},
			}
			for i := range outcomes {
				if outcomes[i].TeamID == row.ID {
					copy := outcomes[i]
					row.Output = &copy
					row.Assessment.OutputState = copy.Outcome
				}
			}
			if row.Output == nil && scope.organiser {
				row.Assessment.OutputState = "not_entered"
				row.Assessment.Availability = "not_applicable"
			}
			decorateAssignments(&row, scope.roster)
			if row.State == "published" {
				row.Support.Availability = "current"
				status, statusErr := (observations.Service{Pool: s.Pool, Now: s.Clock}).SupportInTransaction(
					ctx,
					tx,
					actor,
					eventID,
					row.ID,
				)
				if statusErr != nil {
					if expectedDenial(statusErr) {
						row.Support.Availability = "limited"
					} else {
						return fmt.Errorf("read operational status metadata: %w", statusErr)
					}
				} else {
					row.Support.Operational = status
				}
				actions, actionErr := s.Feedback.SupportInTransaction(
					ctx,
					tx,
					actor,
					eventID,
					row.ID,
				)
				if actionErr != nil {
					if expectedDenial(actionErr) {
						row.Support.Availability = "limited"
					} else {
						return fmt.Errorf("read support action metadata: %w", actionErr)
					}
				} else {
					row.Support.Actions = &actions
				}
				row.Support.Risk = supportRisk(row.Support)
				unused := row.Output != nil &&
					(row.Output.Outcome == "withdrawn" || row.Output.Outcome == "not_submitted")
				if unused {
					row.Assessment.Availability = "not_applicable"
				} else if err = s.assessment(ctx, tx, actor, eventID, &row); err != nil {
					return err
				}
			} else {
				row.Assessment.Availability = "not_applicable"
				if row.State == "disbanded" {
					row.Assessment.OutputState = "disbanded"
				}
			}
			out.Teams = append(out.Teams, row)
		}
		current, err := q.Event(ctx, eventID)
		if err != nil {
			return fmt.Errorf("read current coverage revision: %w", err)
		}
		out.Revision = current.Revision
		return nil
	})
	if err != nil {
		return Page{}, fmt.Errorf("read current support and assessment coverage: %w", err)
	}
	return out, nil
}

func teamPage(
	ctx context.Context,
	tx pgx.Tx,
	eventID, after uuid.UUID,
	scope readScope,
	limit int32,
) ([]dbgen.HackTeam, error) {
	allowed := []uuid.UUID{}
	for _, team := range scope.roster.Teams {
		allowed = append(allowed, team.ID)
	}
	rows, err := tx.Query(
		ctx,
		`SELECT id,name,state FROM hack_teams WHERE event_id=$1 AND id>$2 AND ($3 OR id=ANY($4::uuid[])) ORDER BY id LIMIT $5`,
		eventID,
		after,
		scope.organiser,
		allowed,
		limit+1,
	)
	if err != nil {
		return nil, fmt.Errorf("page current authorised review teams: %w", err)
	}
	defer rows.Close()
	out := []dbgen.HackTeam{}
	for rows.Next() {
		var row dbgen.HackTeam
		if err = rows.Scan(&row.ID, &row.Name, &row.State); err != nil {
			return nil, fmt.Errorf("read authorised review team metadata: %w", err)
		}
		out = append(out, row)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("page review team metadata: %w", err)
	}
	return out, nil
}

func decorateAssignments(row *Team, roster mentors.Page) {
	for _, assignment := range roster.Published {
		if assignment.TeamID != row.ID {
			continue
		}
		name := ""
		for _, person := range roster.Profiles {
			if person.PersonID == assignment.PersonID {
				name = person.Name
			}
		}
		row.Assignments = append(
			row.Assignments,
			Assignment{
				PersonID: assignment.PersonID,
				Name:     name,
				Slot:     assignment.Slot,
				StartsAt: assignment.Window.Start,
				EndsAt:   assignment.Window.End,
			},
		)
	}
	for _, issue := range roster.PublishedGaps {
		relevant := issue.TeamID != nil && *issue.TeamID == row.ID
		if issue.PersonID != nil {
			for _, assignment := range row.Assignments {
				if assignment.PersonID == *issue.PersonID {
					relevant = true
				}
			}
		}
		if relevant && !slices.Contains(row.CoverageIssues, issue.Code) {
			row.CoverageIssues = append(row.CoverageIssues, issue.Code)
		}
	}
}

func expectedDenial(err error) bool {
	return errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) ||
		errors.Is(err, access.ErrConflict) ||
		errors.Is(err, event.ErrBlocked)
}

func supportRisk(s Support) string {
	if s.Availability != "current" {
		return "unknown"
	}
	if s.Operational.State == "blocked" {
		return "blocked"
	}
	if s.Operational.State == "at_risk" || s.Help.OverdueWaiting > 0 ||
		(s.Actions != nil && (s.Actions.Overdue > 0 || s.Actions.UnavailableOwners > 0)) {
		return "needs_attention"
	}
	if s.Help.Unresolved > 0 || (s.Actions != nil && s.Actions.Unresolved > 0) {
		return "open_support"
	}
	if s.Operational.State == "on_track" {
		return "on_track"
	}
	return "unknown"
}
