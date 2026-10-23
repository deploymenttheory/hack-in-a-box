package app

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/feedback"
	"github.com/deploymenttheory/hack-in-a-box/internal/inbox"
)

type feedbackInboxOwner struct {
	Service  feedback.Service
	Followup bool
}

func (o feedbackInboxOwner) AuthorizeNotification(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	source inbox.Source,
) (inbox.Destination, error) {
	if o.Followup {
		if source.Kind != "feedback_followup" || source.Version != 1 {
			return inbox.Destination{}, access.ErrNotFound
		}
		if _, err := o.Service.AuthorizeFollowupInTransaction(
			ctx,
			tx,
			actor,
			eventID,
			source.ID,
		); err != nil {
			return inbox.Destination{}, fmt.Errorf("current feedback follow-up: %w", err)
		}
	} else {
		if source.Kind != "feedback" {
			return inbox.Destination{}, access.ErrNotFound
		}
		if err := o.Service.AuthorizeNotificationInTransaction(
			ctx,
			tx,
			actor,
			eventID,
			source.ID,
			source.Version,
		); err != nil {
			return inbox.Destination{}, fmt.Errorf("current shared feedback: %w", err)
		}
	}
	return inbox.Destination{Kind: source.Kind, ResourceID: source.ID}, nil
}

type feedbackInboxSink struct{ Inbox *inbox.Service }

func (s feedbackInboxSink) RecordFeedbackInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, id uuid.UUID,
	version int64,
	recipients []uuid.UUID,
) error {
	if s.Inbox == nil {
		return access.ErrForbidden
	}
	for _, person := range recipients {
		if err := s.Inbox.RecordResourceInTransaction(
			ctx,
			tx,
			eventID,
			person,
			inbox.Source{Kind: "feedback", ID: id, Version: version},
		); err != nil {
			return fmt.Errorf("record shared feedback notice: %w", err)
		}
	}
	return nil
}

func (s feedbackInboxSink) RecordFeedbackFollowupInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, id, recipient uuid.UUID,
) error {
	if s.Inbox == nil {
		return access.ErrForbidden
	}
	if err := s.Inbox.RecordResourceInTransaction(
		ctx,
		tx,
		eventID,
		recipient,
		inbox.Source{Kind: "feedback_followup", ID: id, Version: 1},
	); err != nil {
		return fmt.Errorf("record overdue feedback follow-up: %w", err)
	}
	return nil
}
