package app

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/deploymenttheory/hack-in-a-box/internal/inbox"
)

// Native help remains the authority for source identity and current recipients.
// These adapters carry no request prose, location, destination URL or caller
// supplied audience across the domain boundary.
type helpInboxOwner struct{ Help help.Service }

func (o helpInboxOwner) AuthorizeNotification(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	source inbox.Source,
) (inbox.Destination, error) {
	if source.Kind != "help" {
		return inbox.Destination{}, access.ErrNotFound
	}
	audience, err := o.Help.ResolveNotificationAudienceInTransaction(
		ctx,
		tx,
		eventID,
		source.ID,
		source.Version,
	)
	if err != nil {
		return inbox.Destination{}, fmt.Errorf("resolve native help notification: %w", err)
	}
	for _, recipient := range audience.RecipientIDs {
		if recipient == actor {
			return inbox.Destination{Kind: "help", ResourceID: source.ID}, nil
		}
	}
	return inbox.Destination{}, access.ErrNotFound
}

type helpInboxSink struct{ Inbox *inbox.Service }

func (s helpInboxSink) RecordHelpInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, requestID uuid.UUID,
	revision int64,
	recipients []uuid.UUID,
) error {
	for _, recipient := range recipients {
		if err := s.Inbox.RecordResourceInTransaction(
			ctx,
			tx,
			eventID,
			recipient,
			inbox.Source{Kind: "help", ID: requestID, Version: revision},
		); err != nil {
			return fmt.Errorf("record native help inbox update: %w", err)
		}
	}
	return nil
}
