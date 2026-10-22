package observations

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func pageLimit(value int) (int32, error) {
	if value == 0 {
		return 50, nil
	}
	if value < 1 || value > 100 {
		return 0, access.ErrInvalid
	}
	return int32(value), nil
}

func versionCursor(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func (s Service) History(
	ctx context.Context,
	actor, eventID, teamID, id uuid.UUID,
) (History, error) {
	return s.HistoryPage(ctx, actor, eventID, teamID, HistoryOptions{ObservationID: id})
}

func (s Service) HistoryPage(
	ctx context.Context,
	actor, eventID, teamID uuid.UUID,
	options HistoryOptions,
) (History, error) {
	result := History{Observations: []Observation{}, Statuses: []Status{}}
	limit, err := pageLimit(options.Limit)
	if err != nil {
		return result, err
	}
	if options.ObservationCursor != nil &&
		(*options.ObservationCursor < 1 || options.ObservationID == uuid.Nil) {
		return result, access.ErrInvalid
	}
	if options.StatusCursor != nil && *options.StatusCursor < 1 {
		return result, access.ErrInvalid
	}
	err = database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		row, _, err := guard(ctx, q, actor, eventID, teamID)
		if err != nil {
			return err
		}
		if options.ObservationID != uuid.Nil {
			open, err := s.reviewOpen(row)
			if err != nil {
				return err
			}
			if !open {
				return access.ErrForbidden
			}
			// Establish existence independently of the cursor, so the end of a valid
			// history is an empty page rather than a false not-found error.
			_, err = q.LatestMentorObservation(ctx, dbgen.LatestMentorObservationParams{
				EventID: eventID, TeamID: teamID, ID: options.ObservationID,
			})
			if errors.Is(err, pgx.ErrNoRows) {
				return access.ErrNotFound
			} else if err != nil {
				return fmt.Errorf("read observation identity: %w", err)
			}
			rows, err := q.MentorObservationHistory(ctx, dbgen.MentorObservationHistoryParams{
				EventID: eventID, TeamID: teamID, ID: options.ObservationID,
				BeforeVersion: versionCursor(options.ObservationCursor), PageSize: limit + 1,
			})
			if err != nil {
				return fmt.Errorf("read private observation history: %w", err)
			}
			if len(rows) > int(limit) {
				rows = rows[:limit]
				cursor := rows[len(rows)-1].Version
				result.NextObservationCursor = &cursor
			}
			for _, item := range rows {
				result.Observations = append(result.Observations, observation(item))
			}
		}
		statuses, err := q.MentorStatusHistory(ctx, dbgen.MentorStatusHistoryParams{
			EventID: eventID, TeamID: teamID,
			BeforeVersion: versionCursor(options.StatusCursor), PageSize: limit + 1,
		})
		if err != nil {
			return fmt.Errorf("read operational status history: %w", err)
		}
		if len(statuses) > int(limit) {
			statuses = statuses[:limit]
			cursor := statuses[len(statuses)-1].Version
			result.NextStatusCursor = &cursor
		}
		for _, item := range statuses {
			result.Statuses = append(result.Statuses, status(item))
		}
		return nil
	})
	if err != nil {
		return History{}, fmt.Errorf("read mentor history: %w", err)
	}
	return result, nil
}
