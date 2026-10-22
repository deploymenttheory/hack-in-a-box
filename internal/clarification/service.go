package clarification

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

// NotificationSink receives only published answer references and an authoritative
// current recipient set; private question text and attachments never cross it.
type NotificationSink interface {
	RecordClarificationInTransaction(
		context.Context,
		pgx.Tx,
		uuid.UUID,
		uuid.UUID,
		int64,
		[]uuid.UUID,
	) error
}

type Service struct {
	Pool          *pgxpool.Pool
	Clock         func() time.Time
	Storage       files.Storage
	Notifications NotificationSink
}

func (s Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC().Truncate(time.Microsecond)
	}
	return time.Now().UTC().Truncate(time.Microsecond)
}

func source(in rubric.ReleasedSource) Source {
	return Source{
		ChallengeID:      in.ChallengeID,
		ChallengeVersion: in.ChallengeVersion,
		Rubric:           in.Reference,
	}
}

func staff(ctx context.Context, q *dbgen.Queries, actor, eventID uuid.UUID) error {
	roles, _, err := access.ContentReader(ctx, q, actor, eventID)
	if err != nil {
		return fmt.Errorf("staff: %w", err)
	}
	if !slices.Contains(roles, "organiser") {
		return access.ErrNotFound
	}
	return nil
}

func teamAuthority(
	ctx context.Context,
	q *dbgen.Queries,
	actor, eventID, teamID uuid.UUID,
	allowStaff bool,
) error {
	a, err := access.TeamPermissions(ctx, q, actor, eventID, teamID)
	if err != nil {
		return fmt.Errorf("teamAuthority: %w", err)
	}
	if allowStaff && a.Organiser {
		return nil
	}
	team, err := q.HackTeam(ctx, dbgen.HackTeamParams{EventID: eventID, ID: teamID})
	if err != nil {
		return fmt.Errorf("read question team: %w", err)
	}
	if !a.Member || team.State != "published" {
		return access.ErrNotFound
	}
	return nil
}

func question(row dbgen.ClarificationQuestion) (Question, error) {
	var ref Source
	if err := json.Unmarshal(row.Source, &ref); err != nil {
		return Question{}, fmt.Errorf("decode private question source: %w", err)
	}
	return Question{
		ID:          row.ID,
		TeamID:      row.TeamID,
		AuthorID:    row.AuthorID,
		Revision:    row.Revision,
		Topic:       row.Topic,
		Body:        row.Body,
		Attachments: row.Attachments,
		Source:      ref,
		Status:      row.Status,
		CreatedAt:   row.CreatedAt,
	}, nil
}

// record retains only fixed actions, IDs, revisions and actors. Neither the native
// history nor the shared audit accepts question or answer prose.
func record(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, actor uuid.UUID,
	questionID, answerID *uuid.UUID,
	revision, version int64,
	action string,
	now time.Time,
) error {
	if err := q.RecordClarificationChange(ctx, dbgen.RecordClarificationChangeParams{
		EventID:    eventID,
		ID:         uuid.New(),
		QuestionID: questionID,
		AnswerID:   answerID,
		Revision:   revision,
		Version: pgtype.Int8{
			Int64: version,
			Valid: version > 0,
		},
		Action:     action,
		ActorID:    actor,
		OccurredAt: now,
	}); err != nil {
		return fmt.Errorf("record clarification transition: %w", err)
	}
	target := questionID
	if answerID != nil {
		target = answerID
	}
	if target == nil {
		return access.ErrInvalid
	}
	if err := audit.Append(ctx, q, audit.Record{
		ActorID:  &actor,
		EventID:  &eventID,
		TargetID: *target,
		Action:   audit.ClarificationChanged,
		Reason:   audit.UserRequest,
		Metadata: audit.Metadata{Revision: &revision},
	}); err != nil {
		return fmt.Errorf("audit clarification transition: %w", err)
	}
	return nil
}

func (s Service) Ask(
	ctx context.Context,
	actor, eventID uuid.UUID,
	input QuestionInput,
) (Question, error) {
	if err := input.validate(); err != nil {
		return Question{}, err
	}
	input.Attachments = append([]uuid.UUID{}, input.Attachments...)
	var out Question
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		now := s.now()
		ref, err := rubric.ReleasedSourceForReaderInTransaction(ctx, tx, actor, eventID, now)
		if err != nil {
			return fmt.Errorf("Ask: %w", err)
		}
		e, err := q.Event(ctx, eventID)
		if err != nil {
			return fmt.Errorf("read question write policy: %w", err)
		}
		if !e.WritesAllowed {
			return access.ErrForbidden
		}
		if err = teamAuthority(ctx, q, actor, eventID, input.TeamID, false); err != nil {
			return err
		}
		encoded, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encode question request fingerprint: %w", err)
		}
		digest := sha256.Sum256(encoded)
		fingerprint := hex.EncodeToString(digest[:])
		prior, err := q.ClarificationQuestionCommand(ctx, dbgen.ClarificationQuestionCommandParams{
			EventID: eventID, AuthorID: actor, CommandID: input.CommandID,
		})
		if err == nil {
			if prior.RequestDigest != fingerprint {
				return access.ErrConflict
			}
			out, err = question(prior)
			return err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read private question idempotency: %w", err)
		}
		for _, id := range input.Attachments {
			file, fileErr := q.File(ctx, dbgen.FileParams{EventID: eventID, ID: id})
			if errors.Is(fileErr, pgx.ErrNoRows) ||
				(fileErr == nil && (file.TeamID == nil || *file.TeamID != input.TeamID)) {
				return access.ErrNotFound
			}
			if fileErr != nil {
				return fmt.Errorf("read private question attachment: %w", fileErr)
			}
			if s.Storage == nil {
				return files.ErrMissing
			}
			if fileErr = files.VerifyStored(ctx, s.Storage, file); fileErr != nil {
				return fmt.Errorf("Ask: %w", fileErr)
			}
		}
		bound, err := json.Marshal(source(ref))
		if err != nil {
			return fmt.Errorf("encode owning question source: %w", err)
		}
		attachments := append([]uuid.UUID{}, input.Attachments...)
		row, err := q.InsertClarificationQuestion(ctx, dbgen.InsertClarificationQuestionParams{
			EventID:       eventID,
			ID:            uuid.New(),
			TeamID:        input.TeamID,
			AuthorID:      actor,
			CommandID:     input.CommandID,
			RequestDigest: fingerprint,
			Topic:         input.Topic,
			Body:          input.Body,
			Attachments:   attachments,
			Source:        bound,
			CreatedAt:     now,
		})
		if err != nil {
			return fmt.Errorf("retain private question: %w", err)
		}
		if err = record(
			ctx,
			q,
			eventID,
			actor,
			&row.ID,
			nil,
			row.Revision,
			0,
			"asked",
			now,
		); err != nil {
			return err
		}
		out, err = question(row)
		return err
	})
	if err != nil {
		return Question{}, fmt.Errorf("Ask: %w", err)
	}
	return out, nil
}

func (s Service) Question(ctx context.Context, actor, eventID, id uuid.UUID) (Question, error) {
	var out Question
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := rubric.ReleasedSourceForReaderInTransaction(
			ctx,
			tx,
			actor,
			eventID,
			s.now(),
		); err != nil {
			return fmt.Errorf("Question: %w", err)
		}
		row, err := q.ClarificationQuestion(
			ctx,
			dbgen.ClarificationQuestionParams{EventID: eventID, ID: id},
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("read private question: %w", err)
		}
		if err = teamAuthority(ctx, q, actor, eventID, row.TeamID, true); err != nil {
			return err
		}
		out, err = question(row)
		return err
	})
	if err != nil {
		return Question{}, fmt.Errorf("Question: %w", err)
	}
	return out, nil
}
