package feedback

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func (s Service) notify(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	row dbgen.Event,
	h dbgen.FeedbackHead,
	version int64,
) error {
	if s.Notifications == nil {
		return nil
	}
	people, err := q.HelpRecipientCandidates(
		ctx,
		dbgen.HelpRecipientCandidatesParams{EventID: row.ID, TeamID: h.TeamID},
	)
	if err != nil {
		return wrap("current shared-feedback audience", err)
	}
	recipients := []uuid.UUID{}
	for _, id := range people {
		a, err := s.authority(ctx, q, id, row.ID, h.TeamID, h.Category)
		if errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if a.TeamMember {
			recipients = append(recipients, id)
		}
	}
	return wrap(
		"record shared-feedback notice",
		s.Notifications.RecordFeedbackInTransaction(ctx, tx, row.ID, h.ID, version, recipients),
	)
}

// AuthorizeNotificationInTransaction checks historical released content against
// current publication and current team membership; no private source read grant
// is conferred by a notification or its retained immutable version.
func (s Service) AuthorizeNotificationInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, id uuid.UUID,
	version int64,
) error {
	q := dbgen.New(tx)
	row, err := lock(ctx, q, eventID)
	if err != nil {
		return err
	}
	h, err := head(ctx, q, eventID, id)
	if err != nil {
		return err
	}
	a, err := s.authority(ctx, q, actor, eventID, h.TeamID, h.Category)
	if err != nil {
		return err
	}
	if !a.TeamMember || !h.PublishedVersion.Valid {
		return access.ErrNotFound
	}
	if err = s.gate(row, h.Category); err != nil {
		return err
	}
	d, err := document(ctx, q, eventID, id, version)
	if err != nil {
		return err
	}
	if d.State != "published" {
		return access.ErrNotFound
	}
	return nil
}

// Followup checks the fixed recipient's current category role and current native
// feedback resource; an old escalation is history, not a claim it is still due.
func (s Service) Followup(ctx context.Context, actor, eventID, id uuid.UUID) (Page, error) {
	var out Page
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		feedbackID, err := s.AuthorizeFollowupInTransaction(ctx, tx, actor, eventID, id)
		if err != nil {
			return err
		}
		row, err := lock(ctx, q, eventID)
		if err != nil {
			return err
		}
		h, err := head(ctx, q, eventID, feedbackID)
		if err != nil {
			return err
		}
		out, err = s.project(ctx, tx, q, row, actor, h)
		return err
	})
	return out, wrap("read current feedback follow-up", err)
}

func (s Service) AuthorizeFollowupInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, id uuid.UUID,
) (uuid.UUID, error) {
	q := dbgen.New(tx)
	row, err := lock(ctx, q, eventID)
	if err != nil {
		return uuid.Nil, err
	}
	notice, err := q.FeedbackEscalation(
		ctx,
		dbgen.FeedbackEscalationParams{EventID: eventID, ID: id},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, access.ErrNotFound
	}
	if err != nil {
		return uuid.Nil, wrap("read fixed follow-up target", err)
	}
	if notice.RecipientID != actor {
		return uuid.Nil, access.ErrNotFound
	}
	ah, err := q.FeedbackActionHead(
		ctx,
		dbgen.FeedbackActionHeadParams{EventID: eventID, ID: notice.ActionID},
	)
	if err != nil {
		return uuid.Nil, wrap("read follow-up action", err)
	}
	h, err := head(ctx, q, eventID, ah.FeedbackID)
	if err != nil {
		return uuid.Nil, err
	}
	if !h.PublishedVersion.Valid {
		return uuid.Nil, access.ErrNotFound
	}
	if err = s.escalationAuthority(ctx, q, actor, eventID, h); err != nil {
		return uuid.Nil, err
	}
	if err = s.gate(row, h.Category); err != nil {
		return uuid.Nil, err
	}
	return h.ID, nil
}
