package clarification

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

type Review struct {
	Draft     *AnswerConfiguration `json:"draft"`
	Workspace Workspace            `json:"workspace"`
}

// Review distinguishes an authorised empty draft from denied access. The exact
// draft and the current common source/audience preview share one event lock.
func (s Service) Review(ctx context.Context, actor, eventID, questionID uuid.UUID) (Review, error) {
	var out Review
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		workspace, err := s.workspaceInTransaction(ctx, tx, actor, eventID)
		if err != nil {
			return err
		}
		if !workspace.CanReview {
			return access.ErrNotFound
		}
		if _, err = loadQuestion(ctx, q, eventID, questionID); err != nil {
			return err
		}
		out.Workspace = workspace
		header, err := q.ClarificationAnswerForQuestion(
			ctx,
			dbgen.ClarificationAnswerForQuestionParams{
				EventID: eventID, QuestionID: questionID,
			},
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read current official answer draft: %w", err)
		}
		draft, err := configuration(ctx, q, header)
		if err != nil {
			return err
		}
		out.Draft = &draft
		return nil
	})
	if err != nil {
		return Review{}, fmt.Errorf("current official answer review: %w", err)
	}
	return out, nil
}
