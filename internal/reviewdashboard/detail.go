package reviewdashboard

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
	"github.com/deploymenttheory/hack-in-a-box/internal/moderation"
	"github.com/deploymenttheory/hack-in-a-box/internal/submissions"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func scopedTeam(ctx context.Context, q *dbgen.Queries, scope readScope, eventID, teamID uuid.UUID) (dbgen.HackTeam, error) {
	if !scope.organiser && !slices.ContainsFunc(scope.roster.Teams, func(team mentors.Team) bool { return team.ID == teamID }) {
		return dbgen.HackTeam{}, access.ErrNotFound
	}
	row, err := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: eventID, ID: teamID})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, access.ErrNotFound
	}
	if err != nil {
		return row, fmt.Errorf("read current dashboard team scope: %w", err)
	}
	return row, nil
}

func (s Service) Preview(ctx context.Context, actor, eventID, teamID uuid.UUID) (FactualPreview, error) {
	var out FactualPreview
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		scope, err := s.guard(ctx, tx, actor, eventID)
		if err != nil {
			return err
		}
		// This explicit organiser preview does not create future assigned-judge access.
		if !scope.organiser {
			return access.ErrForbidden
		}
		team, err := scopedTeam(ctx, q, scope, eventID, teamID)
		if err != nil {
			return err
		}
		if _, entryErr := q.CompetitionEntry(ctx, dbgen.CompetitionEntryParams{EventID: eventID, TeamID: teamID}); errors.Is(entryErr, pgx.ErrNoRows) {
			out = FactualPreview{EventID: eventID, TeamID: teamID, TeamName: team.Name, TeamState: team.State, GeneratedAt: s.now(), Output: submissions.OutputSummary{TeamID: teamID, Outcome: "not_entered", Artifacts: []submissions.ArtifactCount{}}}
			return nil
		} else if entryErr != nil {
			return fmt.Errorf("read factual output entry: %w", entryErr)
		}
		values, err := (submissions.Service{Pool: s.Pool, Clock: s.Clock}).SummariesInTransaction(ctx, tx, actor, eventID, []uuid.UUID{teamID})
		if err != nil {
			return fmt.Errorf("read factual frozen-output preview: %w", err)
		}
		if len(values) != 1 {
			return access.ErrNotFound
		}
		out = FactualPreview{EventID: eventID, TeamID: teamID, TeamName: team.Name, TeamState: team.State, GeneratedAt: s.now(), Output: values[0]}
		return nil
	})
	if err != nil {
		return out, fmt.Errorf("preview factual judge handoff without assessment: %w", err)
	}
	return out, nil
}

func (s Service) Checkpoints(ctx context.Context, actor, eventID, teamID, after uuid.UUID, limit int) (CheckpointPage, error) {
	var out CheckpointPage
	if s.Cards == nil {
		return out, access.ErrForbidden
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		scope, err := s.guard(ctx, tx, actor, eventID)
		if err != nil {
			return err
		}
		if _, err = scopedTeam(ctx, q, scope, eventID, teamID); err != nil {
			return err
		}
		if scope.organiser || (!scope.dates.LiveStart.IsZero() && !s.now().Before(scope.dates.LiveStart)) {
			if _, entryErr := q.CompetitionEntry(ctx, dbgen.CompetitionEntryParams{EventID: eventID, TeamID: teamID}); entryErr == nil {
				if _, err = (submissions.Service{Pool: s.Pool, Clock: s.Clock}).SummariesInTransaction(ctx, tx, actor, eventID, []uuid.UUID{teamID}); err != nil {
					return err
				}
			} else if !errors.Is(entryErr, pgx.ErrNoRows) {
				return fmt.Errorf("read checkpoint entry scope: %w", entryErr)
			}
		}
		out.Coverage, err = s.Cards.CoverageInTransaction(ctx, tx, actor, eventID, teamID, after, limit)
		if err != nil {
			return fmt.Errorf("read scoped completion metadata: %w", err)
		}
		current, err := q.Event(ctx, eventID)
		if err != nil {
			return fmt.Errorf("read checkpoint coverage revision: %w", err)
		}
		out.Revision = current.Revision
		out.Timezone = scope.dates.Timezone
		out.Now = s.now()
		return nil
	})
	if err != nil {
		return out, fmt.Errorf("read checkpoint coordination page: %w", err)
	}
	return out, nil
}

func (s Service) Huddles(ctx context.Context, actor, eventID, teamID, after uuid.UUID, limit int32) (AgendaPage, error) {
	out := AgendaPage{Availability: "not_applicable", Agenda: moderation.Agenda{Items: []moderation.AgendaItem{}}}
	if s.Moderation == nil {
		return out, access.ErrForbidden
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		scope, err := s.guard(ctx, tx, actor, eventID)
		if err != nil {
			return err
		}
		team, err := scopedTeam(ctx, q, scope, eventID, teamID)
		if err != nil {
			return err
		}
		out.Timezone = scope.dates.Timezone
		out.Now = s.now()
		applicable := team.State == "published"
		if applicable && (scope.organiser || (!scope.dates.LiveStart.IsZero() && !s.now().Before(scope.dates.LiveStart))) {
			if _, entryErr := q.CompetitionEntry(ctx, dbgen.CompetitionEntryParams{EventID: eventID, TeamID: teamID}); errors.Is(entryErr, pgx.ErrNoRows) {
				applicable = false
			} else if entryErr != nil {
				return fmt.Errorf("read huddle entry scope: %w", entryErr)
			} else {
				outputs, outputErr := (submissions.Service{Pool: s.Pool, Clock: s.Clock}).SummariesInTransaction(ctx, tx, actor, eventID, []uuid.UUID{teamID})
				if outputErr != nil {
					return outputErr
				}
				applicable = len(outputs) == 1 && outputs[0].Outcome != "withdrawn" && outputs[0].Outcome != "not_submitted"
			}
		}
		if applicable {
			out.Agenda, err = s.Moderation.AgendaInTransaction(ctx, tx, actor, eventID, teamID, after, limit)
			if err != nil {
				if !expectedDenial(err) {
					return err
				}
			} else {
				out.Availability = "current"
			}
		}
		current, err := q.Event(ctx, eventID)
		if err != nil {
			return fmt.Errorf("read huddle coordination revision: %w", err)
		}
		out.Revision = current.Revision
		return nil
	})
	if err != nil {
		return out, fmt.Errorf("read scoped huddle agenda: %w", err)
	}
	return out, nil
}
