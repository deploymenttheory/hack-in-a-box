package app

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/deploymenttheory/hack-in-a-box/internal/inbox"
)

type clarificationInboxOwner struct{ Answers *clarification.Service }

func (o clarificationInboxOwner) AuthorizeNotification(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	source inbox.Source,
) (inbox.Destination, error) {
	if o.Answers == nil || source.Kind != "clarification" || source.ID == uuid.Nil ||
		source.Version < 1 {
		return inbox.Destination{}, access.ErrNotFound
	}
	if err := o.Answers.AuthorizeNotificationInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		source.ID,
		source.Version,
	); err != nil {
		return inbox.Destination{}, fmt.Errorf("AuthorizeNotification: %w", err)
	}
	return inbox.Destination{Kind: "clarification", ResourceID: source.ID}, nil
}

type clarificationInboxSink struct{ Inbox *inbox.Service }

func (s clarificationInboxSink) RecordClarificationInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, answerID uuid.UUID,
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
			inbox.Source{Kind: "clarification", ID: answerID, Version: version},
		); err != nil {
			return fmt.Errorf("RecordClarificationInTransaction: %w", err)
		}
	}
	return nil
}
