package teamsworkflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/audit"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/secretbox"
)

const operationKind = "teams_workflow"

func (s Service) QueueTest(
	ctx context.Context,
	actor, eventID uuid.UUID,
	expected int64,
	input TestInput,
) (Page, error) {
	if s.Pool == nil || s.Jobs == nil {
		return Page{}, database.ErrDatabase
	}
	if input.RequestID == uuid.Nil {
		return Page{}, access.ErrInvalid
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		event, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock Teams test: %w", err)
		}
		if err = authorize(ctx, q, actor, eventID, true); err != nil {
			return err
		}
		existing, err := q.TeamsWorkflowRequest(
			ctx,
			dbgen.TeamsWorkflowRequestParams{EventID: eventID, RequestID: input.RequestID},
		)
		if err == nil {
			if existing.Kind != "connection_test" || existing.ActorID != actor {
				return access.ErrConflict
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read Teams test identity: %w", err)
		}
		if event.Revision != expected {
			return access.ErrConflict
		}
		connection, err := q.TeamsWorkflowConnection(ctx, eventID)
		if err != nil {
			return fmt.Errorf("read Teams test connection: %w", err)
		}
		if !connection.Active {
			return access.ErrForbidden
		}
		if _, err = s.open(connection); err != nil {
			return secretbox.ErrUnavailable
		}
		if _, err = s.enqueue(
			ctx,
			tx,
			dbgen.InsertTeamsWorkflowDeliveryParams{
				EventID:           eventID,
				ID:                uuid.New(),
				Kind:              "connection_test",
				ConnectionID:      connection.ID,
				ConnectionVersion: connection.Version,
				ChannelRevision:   revision(connection.ID, connection.Version),
				ActorID:           actor,
				RequestID:         input.RequestID,
				CreatedAt:         s.now(),
			},
		); err != nil {
			return err
		}
		event, err = q.BumpEventRevision(ctx, eventID)
		if err != nil {
			return fmt.Errorf("advance Teams test revision: %w", err)
		}
		result := "success"
		return audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &eventID,
				TargetID: connection.ID,
				Action:   audit.TeamsTestQueued,
				Reason:   audit.OrganiserDecision,
				Metadata: audit.Metadata{Result: &result, Revision: &event.Revision},
			},
		)
	})
	if err != nil {
		return Page{}, fmt.Errorf("queue Teams test: %w", err)
	}
	return s.Read(ctx, actor, eventID)
}

// QueueApprovedInTransaction is the reference-only seam for campaign execution.
// It consumes the authoritative approved broadcast, never arbitrary message text.
// The caller owns unattended due/missed-action policy; the resolver rejects early,
// changed, private or unapproved campaigns before any operation is inserted.
func (s Service) QueueApprovedInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	expectedVersion int64,
) (uuid.UUID, error) {
	if s.Jobs == nil {
		return uuid.Nil, database.ErrDatabase
	}
	message, err := s.Composer.ResolveApprovedBroadcastInTransaction(ctx, tx, eventID, campaignID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("resolve Teams broadcast: %w", err)
	}
	id, _, err := s.queueApprovedBroadcast(
		ctx,
		tx,
		eventID,
		campaignID,
		expectedVersion,
		message,
		communications.DeliveryScope{},
		communications.ScopeAudience{},
	)
	return id, err
}

func (s Service) queueApprovedBroadcast(
	ctx context.Context,
	tx pgx.Tx,
	eventID, campaignID uuid.UUID,
	expectedVersion int64,
	message communications.BroadcastMessage,
	scope communications.DeliveryScope,
	audience communications.ScopeAudience,
) (uuid.UUID, bool, error) {
	if message.CampaignVersion != expectedVersion {
		return uuid.Nil, false, access.ErrConflict
	}
	if _, err := card(message.Subject, message.PlainText, uuid.New()); err != nil {
		return uuid.Nil, false, access.ErrInvalid
	}
	q := dbgen.New(tx)
	var existing dbgen.TeamsWorkflowDelivery
	var err error
	if scope.OccurrenceID == uuid.Nil {
		existing, err = q.TeamsWorkflowCampaignDelivery(
			ctx,
			dbgen.TeamsWorkflowCampaignDeliveryParams{
				EventID:         eventID,
				CampaignID:      &campaignID,
				CampaignVersion: pgtype.Int8{Int64: message.CampaignVersion, Valid: true},
			},
		)
	} else {
		existing, err = q.TeamsWorkflowOccurrenceDelivery(
			ctx,
			dbgen.TeamsWorkflowOccurrenceDeliveryParams{
				EventID:      eventID,
				OccurrenceID: &scope.OccurrenceID,
			},
		)
	}

	if err == nil {
		if existing.CampaignID == nil || *existing.CampaignID != campaignID ||
			existing.CampaignVersion.Int64 != expectedVersion {
			return uuid.Nil, false, access.ErrConflict
		}
		if existing.ScopeBinding != audience.Binding {
			if guardErr := s.checkSettled(ctx, q, existing); guardErr != nil {
				return uuid.Nil, false, guardErr
			}
		}
		return existing.OperationID, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, fmt.Errorf("find Teams campaign operation: %w", err)
	}
	connection, err := q.TeamsWorkflowConnection(ctx, eventID)
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("read Teams broadcast connection: %w", err)
	}
	current := revision(connection.ID, connection.Version)
	if !connection.Active || message.Approval.ChannelRevisions["teams"] != current {
		return uuid.Nil, false, access.ErrConflict
	}
	if _, err = s.open(connection); err != nil {
		return uuid.Nil, false, secretbox.ErrUnavailable
	}
	previewID := message.Approval.PreviewID
	proof, err := json.Marshal(message.ScopeApproval)
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("retain Teams scoped approval: %w", err)
	}
	id, err := s.enqueue(
		ctx,
		tx,
		dbgen.InsertTeamsWorkflowDeliveryParams{
			OccurrenceID:       optionalOccurrence(scope),
			ScopeBinding:       audience.Binding,
			ScopeApproval:      proof,
			EventID:            eventID,
			ID:                 uuid.New(),
			Kind:               "broadcast",
			ConnectionID:       connection.ID,
			ConnectionVersion:  connection.Version,
			ChannelRevision:    current,
			ActorID:            message.Approval.ActorID,
			CampaignID:         &campaignID,
			CampaignVersion:    pgtype.Int8{Int64: message.CampaignVersion, Valid: true},
			PreviewID:          &previewID,
			PreviewFingerprint: pgtype.Text{String: message.Approval.Fingerprint, Valid: true},
			RequestID:          uuid.New(),
			CreatedAt:          s.now(),
		},
	)
	return id, true, err
}

func (s Service) QueueBroadcast(
	ctx context.Context,
	actor, eventID, campaignID uuid.UUID,
	expectedVersion int64,
) (Page, error) {
	if s.Pool == nil {
		return Page{}, database.ErrDatabase
	}
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if err := authorize(ctx, q, actor, eventID, true); err != nil {
			return err
		}
		id, err := s.QueueApprovedInTransaction(ctx, tx, eventID, campaignID, expectedVersion)
		if err != nil {
			return err
		}
		result := "success"
		return audit.Append(
			ctx,
			q,
			audit.Record{
				ActorID:  &actor,
				EventID:  &eventID,
				TargetID: id,
				Action:   audit.TeamsBroadcastQueued,
				Reason:   audit.OrganiserDecision,
				Metadata: audit.Metadata{Result: &result},
			},
		)
	})
	if err != nil {
		return Page{}, fmt.Errorf("queue approved Teams broadcast: %w", err)
	}
	return s.Read(ctx, actor, eventID)
}

func (s Service) enqueue(
	ctx context.Context,
	tx pgx.Tx,
	input dbgen.InsertTeamsWorkflowDeliveryParams,
) (uuid.UUID, error) {
	op, err := s.Jobs.Enqueue(
		ctx,
		tx,
		dbgen.InsertOperationParams{
			EventID:      input.EventID,
			TargetID:     input.ID,
			Kind:         operationKind,
			OperationKey: "teams:" + input.RequestID.String(),
			DueAt:        s.now(),
			Approved:     true,
		},
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("enqueue Teams operation: %w", err)
	}
	input.OperationID = op.ID
	if len(input.ScopeApproval) == 0 {
		input.ScopeApproval = []byte("null")
	}
	if err = dbgen.New(tx).InsertTeamsWorkflowDelivery(ctx, input); err != nil {
		return uuid.Nil, fmt.Errorf("retain Teams delivery intent: %w", err)
	}
	return op.ID, nil
}

// HandlerKind is fixed so API/worker wiring cannot accidentally enable a generic
// arbitrary-URL or arbitrary-payload webhook operation.
func HandlerKind() string { return operationKind }
