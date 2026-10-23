package feedback

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// AuthorizeRetainedActionInTransaction checks current native reader authority
// for exact previously published feedback/action metadata. Version advancement
// does not erase legitimate history; withdrawal or current reader loss denies it.
func (s Service) AuthorizeRetainedActionInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID, feedbackID, actionID uuid.UUID,
	feedbackVersion, retainedVersion int64,
) error {
	if feedbackVersion < 1 || retainedVersion < 1 {
		return access.ErrInvalid
	}
	q := dbgen.New(tx)
	row, err := lock(ctx, q, eventID)
	if err != nil {
		return err
	}
	h, err := head(ctx, q, eventID, feedbackID)
	if err != nil {
		return err
	}
	if h.TeamID != teamID || !h.PublishedVersion.Valid {
		return access.ErrNotFound
	}
	ah, err := q.FeedbackActionHead(
		ctx,
		dbgen.FeedbackActionHeadParams{EventID: eventID, ID: actionID},
	)
	if err != nil {
		return wrap("read retained native action identity", err)
	}
	if ah.FeedbackID != feedbackID || retainedVersion > ah.CurrentVersion {
		return access.ErrConflict
	}
	published, err := document(ctx, q, eventID, feedbackID, feedbackVersion)
	if err != nil {
		return err
	}
	if published.State != "published" {
		return access.ErrForbidden
	}
	a, err := actionVersion(ctx, q, eventID, actionID, retainedVersion)
	if err != nil {
		return err
	}
	_, err = s.actionView(ctx, q, row, actor, h, a)
	return err
}
