package clarification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

func loadQuestion(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, id uuid.UUID,
) (dbgen.ClarificationQuestion, error) {
	row, err := q.ClarificationQuestion(
		ctx,
		dbgen.ClarificationQuestionParams{EventID: eventID, ID: id},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, access.ErrNotFound
	}
	if err != nil {
		return row, fmt.Errorf("read private question: %w", err)
	}
	return row, nil
}

func (s Service) Triage(
	ctx context.Context,
	actor, eventID, id uuid.UUID,
	expected int64,
	action string,
) (Question, error) {
	status, auditAction := "", ""
	switch action {
	case "triage":
		status, auditAction = "triaged", "triaged"
	case "close":
		status, auditAction = "closed", "closed"
	case "reopen":
		status, auditAction = "open", "reopened"
	default:
		return Question{}, access.ErrInvalid
	}
	if expected < 1 {
		return Question{}, access.ErrInvalid
	}
	var out Question
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		now := s.now()
		if _, err := rubric.ReleasedSourceForReaderInTransaction(
			ctx,
			tx,
			actor,
			eventID,
			now,
		); err != nil {
			return fmt.Errorf("Triage: %w", err)
		}
		if err := staff(ctx, q, actor, eventID); err != nil {
			return err
		}
		e, err := q.Event(ctx, eventID)
		if err != nil {
			return fmt.Errorf("read question triage policy: %w", err)
		}
		if !e.WritesAllowed {
			return access.ErrForbidden
		}
		row, err := loadQuestion(ctx, q, eventID, id)
		if err != nil {
			return err
		}
		if row.Revision != expected {
			return access.ErrConflict
		}
		if row.Status != status {
			row, err = q.UpdateClarificationQuestion(
				ctx,
				dbgen.UpdateClarificationQuestionParams{EventID: eventID, ID: id, Status: status},
			)
			if err != nil {
				return fmt.Errorf("change question triage state: %w", err)
			}
			if err = record(
				ctx,
				q,
				eventID,
				actor,
				&id,
				nil,
				row.Revision,
				0,
				auditAction,
				now,
			); err != nil {
				return err
			}
		}
		out, err = question(row)
		return err
	})
	if err != nil {
		return Question{}, fmt.Errorf("Triage: %w", err)
	}
	return out, nil
}

// Attachment checks the native private association and current authority before
// reading bytes. Explicit staff access does not bypass the existing pre-live
// restriction on viewing a team's working files.
func (s Service) Attachment(
	ctx context.Context,
	actor, eventID, questionID, fileID uuid.UUID,
) (dbgen.File, []byte, error) {
	var metadata dbgen.File
	var data []byte
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		now := s.now()
		if _, err := rubric.ReleasedSourceForReaderInTransaction(
			ctx,
			tx,
			actor,
			eventID,
			now,
		); err != nil {
			return fmt.Errorf("Attachment: %w", err)
		}
		row, err := loadQuestion(ctx, q, eventID, questionID)
		if err != nil {
			return err
		}
		if !slices.Contains(row.Attachments, fileID) {
			return access.ErrNotFound
		}
		if err = teamAuthority(ctx, q, actor, eventID, row.TeamID, true); err != nil {
			return err
		}
		a, err := access.TeamPermissions(ctx, q, actor, eventID, row.TeamID)
		if err != nil {
			return fmt.Errorf("Attachment: %w", err)
		}
		if !a.Member {
			e, eventErr := q.Event(ctx, eventID)
			if eventErr != nil {
				return fmt.Errorf("read question attachment schedule: %w", eventErr)
			}
			var dates struct {
				LiveStart time.Time `json:"live_start"`
			}
			if eventErr = json.Unmarshal(e.Configuration, &dates); eventErr != nil {
				return fmt.Errorf("decode question attachment schedule: %w", eventErr)
			}
			if !a.Organiser || dates.LiveStart.IsZero() || now.Before(dates.LiveStart) {
				return access.ErrNotFound
			}
		}
		metadata, err = q.File(ctx, dbgen.FileParams{EventID: eventID, ID: fileID})
		if errors.Is(err, pgx.ErrNoRows) ||
			(err == nil && (metadata.TeamID == nil || *metadata.TeamID != row.TeamID)) {
			return access.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read associated private file: %w", err)
		}
		data, err = files.ReadAuthorizedStored(ctx, s.Storage, metadata)
		if err != nil {
			return fmt.Errorf("read authorised question attachment: %w", err)
		}
		return nil
	})
	if err != nil {
		return dbgen.File{}, nil, fmt.Errorf("Attachment: %w", err)
	}
	return metadata, data, nil
}
