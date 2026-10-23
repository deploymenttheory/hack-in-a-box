package app

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/inbox"
	"github.com/deploymenttheory/hack-in-a-box/internal/outcomes"
)

type outcomeInboxOwner struct{ Service *outcomes.Service }

func (o outcomeInboxOwner) AuthorizeNotification(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	source inbox.Source,
) (inbox.Destination, error) {
	if o.Service == nil || source.Kind != "outcome_review" {
		return inbox.Destination{}, access.ErrNotFound
	}
	err := o.Service.AuthorizeReviewNotification(ctx, tx, actor, eventID, source.ID, source.Version)
	if err != nil {
		return inbox.Destination{}, fmt.Errorf("authorize current outcome-review destination: %w", err)
	}
	return inbox.Destination{Kind: "outcome_review", ResourceID: source.ID}, nil
}
