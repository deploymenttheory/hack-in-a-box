package clarification

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

func (s Service) Questions(
	ctx context.Context,
	actor, eventID uuid.UUID,
	teamID *uuid.UUID,
	cursor uuid.UUID,
	limit int32,
) (QuestionList, error) {
	if limit < 1 || limit > 100 {
		return QuestionList{}, access.ErrInvalid
	}
	var out QuestionList
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := rubric.ReleasedSourceForReaderInTransaction(
			ctx,
			tx,
			actor,
			eventID,
			s.now(),
		); err != nil {
			return fmt.Errorf("authorize private question list source: %w", err)
		}
		roles, _, err := access.ContentReader(ctx, q, actor, eventID)
		if err != nil {
			return fmt.Errorf("authorize private question list reader: %w", err)
		}
		var rows []dbgen.ClarificationQuestion
		if teamID == nil && slices.Contains(roles, "organiser") {
			rows, err = q.ClarificationStaffQuestions(
				ctx,
				dbgen.ClarificationStaffQuestionsParams{
					EventID:  eventID,
					Cursor:   cursor,
					PageSize: limit + 1,
				},
			)
		} else {
			if teamID == nil {
				membership, memberErr := q.TeamAssignment(
					ctx,
					dbgen.TeamAssignmentParams{
						EventID:  eventID,
						PersonID: actor,
						Snapshot: "published",
					},
				)
				if errors.Is(memberErr, pgx.ErrNoRows) {
					return access.ErrNotFound
				}
				if memberErr != nil {
					return fmt.Errorf("read current question-list membership: %w", memberErr)
				}
				teamID = &membership.TeamID
			}
			if err = teamAuthority(ctx, q, actor, eventID, *teamID, true); err != nil {
				return err
			}
			rows, err = q.ClarificationTeamQuestions(
				ctx,
				dbgen.ClarificationTeamQuestionsParams{
					EventID:  eventID,
					TeamID:   *teamID,
					Cursor:   cursor,
					PageSize: limit + 1,
				},
			)
		}
		if err != nil {
			return fmt.Errorf("read current private questions: %w", err)
		}
		if len(rows) > int(limit) {
			next := rows[limit-1].ID
			out.NextCursor = &next
			rows = rows[:limit]
		}
		out.Items = []Question{}
		for _, row := range rows {
			item, itemErr := question(row)
			if itemErr != nil {
				return itemErr
			}
			out.Items = append(out.Items, item)
		}
		return nil
	})
	if err != nil {
		return QuestionList{}, fmt.Errorf("list private team questions: %w", err)
	}
	return out, nil
}

func (s Service) QuestionHistory(
	ctx context.Context,
	actor, eventID, questionID uuid.UUID,
	cursor uuid.UUID,
	limit int32,
) (QuestionHistory, error) {
	if limit < 1 || limit > 100 {
		return QuestionHistory{}, access.ErrInvalid
	}
	var out QuestionHistory
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := rubric.ReleasedSourceForReaderInTransaction(
			ctx,
			tx,
			actor,
			eventID,
			s.now(),
		); err != nil {
			return fmt.Errorf("authorize question history source: %w", err)
		}
		row, err := loadQuestion(ctx, q, eventID, questionID)
		if err != nil {
			return err
		}
		if err = teamAuthority(ctx, q, actor, eventID, row.TeamID, true); err != nil {
			return err
		}
		rows, err := q.ClarificationChanges(
			ctx,
			dbgen.ClarificationChangesParams{
				EventID:    eventID,
				QuestionID: &questionID,
				Cursor:     cursor,
				PageSize:   limit + 1,
			},
		)
		if err != nil {
			return fmt.Errorf("read native question transitions: %w", err)
		}
		if len(rows) > int(limit) {
			next := rows[limit-1].ID
			out.NextCursor = &next
			rows = rows[:limit]
		}
		out.Items = []Transition{}
		for _, row := range rows {
			var version *int64
			if row.Version.Valid {
				value := row.Version.Int64
				version = &value
			}
			out.Items = append(
				out.Items,
				Transition{
					ID:       row.ID,
					Revision: row.Revision,
					Version:  version,
					Action:   row.Action,
					ActorID:  row.ActorID,
					At:       row.OccurredAt,
				},
			)
		}
		return nil
	})
	if err != nil {
		return QuestionHistory{}, fmt.Errorf("read private question history: %w", err)
	}
	return out, nil
}

func (s Service) PublishedHistory(
	ctx context.Context,
	actor, eventID, answerID uuid.UUID,
	before int64,
	limit int32,
) (PublicHistory, error) {
	if before < 0 || limit < 1 || limit > 100 {
		return PublicHistory{}, access.ErrInvalid
	}
	if before == 0 {
		before = math.MaxInt64
	}
	var out PublicHistory
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		now := s.now()
		ref, err := rubric.ReleasedSourceForReaderInTransaction(ctx, tx, actor, eventID, now)
		if err != nil {
			return fmt.Errorf("authorize published correction history: %w", err)
		}
		rows, err := q.PublishedClarificationHistory(
			ctx,
			dbgen.PublishedClarificationHistoryParams{
				EventID:       eventID,
				ID:            answerID,
				BeforeVersion: before,
				PageSize:      limit + 1,
			},
		)
		if err != nil {
			return fmt.Errorf("read published correction page: %w", err)
		}
		if len(rows) > int(limit) {
			next := rows[limit-1].Version
			out.NextCursor = &next
			rows = rows[:limit]
		}
		out.Items = []PublishedAnswer{}
		for _, row := range rows {
			if now.Before(row.PublishedAt) {
				return access.ErrNotFound
			}
			item, itemErr := published(row, source(ref))
			if itemErr != nil {
				return itemErr
			}
			out.Items = append(out.Items, item)
		}
		return nil
	})
	if err != nil {
		return PublicHistory{}, fmt.Errorf("read common correction history: %w", err)
	}
	return out, nil
}

func (s Service) AnswerConfiguration(
	ctx context.Context,
	actor, eventID, questionID uuid.UUID,
) (AnswerConfiguration, error) {
	var out AnswerConfiguration
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := rubric.ReleasedSourceForReaderInTransaction(
			ctx,
			tx,
			actor,
			eventID,
			s.now(),
		); err != nil {
			return fmt.Errorf("authorize answer configuration source: %w", err)
		}
		if err := staff(ctx, q, actor, eventID); err != nil {
			return err
		}
		h, err := q.ClarificationAnswerForQuestion(
			ctx,
			dbgen.ClarificationAnswerForQuestionParams{EventID: eventID, QuestionID: questionID},
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read exact staff answer configuration: %w", err)
		}
		out, err = configuration(ctx, q, h)
		return err
	})
	if err != nil {
		return AnswerConfiguration{}, fmt.Errorf("read private answer configuration: %w", err)
	}
	return out, nil
}
