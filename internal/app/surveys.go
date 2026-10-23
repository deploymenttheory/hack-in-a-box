package app

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/inbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/surveys"
)

type surveyInboxOwner struct{ Surveys *surveys.Service }

func (s surveyInboxOwner) AuthorizeNotification(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	source inbox.Source,
) (inbox.Destination, error) {
	if s.Surveys == nil || source.Kind != "survey" {
		return inbox.Destination{}, access.ErrNotFound
	}
	if err := s.Surveys.AuthorizeNotificationInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		source.ID,
		source.Version,
	); err != nil {
		return inbox.Destination{}, fmt.Errorf("authorize native survey reader: %w", err)
	}
	return inbox.Destination{Kind: "survey", ResourceID: source.ID}, nil
}
