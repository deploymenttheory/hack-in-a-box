package teamsworkflow

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
)

func literalCardText(value string) string {
	return strings.NewReplacer("\\", "\\\\", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]", "`", "\\`", "#", "\\#").
		Replace(value)
}

func card(subject, body string, reference uuid.UUID) ([]byte, error) {
	payload := map[string]any{
		"type": "message",
		"attachments": []any{
			map[string]any{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"contentUrl":  nil,
				"content": map[string]any{
					"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
					"type":    "AdaptiveCard",
					"version": "1.2",
					"body": []any{
						map[string]any{
							"type":   "TextBlock",
							"text":   literalCardText(subject),
							"weight": "Bolder",
							"wrap":   true,
						},
						map[string]any{
							"type": "TextBlock",
							"text": literalCardText(body),
							"wrap": true,
						},
						map[string]any{
							"type": "TextBlock",
							"text": "Operation reference: " + reference.String(),
							"size": "Small",
							"wrap": true,
						},
					},
				},
			},
		},
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(payload); err != nil {
		return nil, fmt.Errorf("encode Teams card: %w", err)
	}
	if output.Len() > 28*1024 {
		return nil, access.ErrInvalid
	}
	return output.Bytes(), nil
}

func expectedBlock(err error) bool {
	return errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) ||
		errors.Is(err, access.ErrConflict) ||
		errors.Is(err, communications.ErrUnresolved)
}

func (s Service) validateDelivery(
	ctx context.Context,
	tx pgx.Tx,
	d jobs.Delivery,
) (dbgen.TeamsWorkflowDelivery, dbgen.TeamsWorkflowConnection, []byte, error) {
	q := dbgen.New(tx)
	intent, err := q.TeamsWorkflowDelivery(
		ctx,
		dbgen.TeamsWorkflowDeliveryParams{EventID: d.EventID, ID: d.TargetID},
	)
	if err != nil {
		return intent, dbgen.TeamsWorkflowConnection{}, nil, fmt.Errorf(
			"read Teams intent: %w",
			err,
		)
	}
	if intent.OperationID != d.ID {
		return intent, dbgen.TeamsWorkflowConnection{}, nil, access.ErrForbidden
	}
	if _, err = q.LockEvent(ctx, d.EventID); err != nil {
		return intent, dbgen.TeamsWorkflowConnection{}, nil, fmt.Errorf(
			"lock Teams execution: %w",
			err,
		)
	}
	if err = authorize(ctx, q, intent.ActorID, d.EventID, true); err != nil {
		return intent, dbgen.TeamsWorkflowConnection{}, nil, err
	}
	connection, err := q.LockTeamsWorkflowConnection(ctx, d.EventID)
	if err != nil {
		return intent, connection, nil, fmt.Errorf("read execution connection: %w", err)
	}
	if !connection.Active || connection.ID != intent.ConnectionID ||
		connection.Version != intent.ConnectionVersion ||
		revision(connection.ID, connection.Version) != intent.ChannelRevision {
		return intent, connection, nil, access.ErrConflict
	}
	subject, body := "TEST — Hack-in-a-Box Teams connection check", "This synthetic connection test is a broadcast to the configured event channel. Confirm the destination and workflow run with its owner. It is not a personal campaign self-test."
	if intent.Kind == "broadcast" {
		if intent.CampaignID == nil {
			return intent, connection, nil, access.ErrForbidden
		}
		message, err := s.Composer.ResolveApprovedBroadcastInTransaction(
			ctx,
			tx,
			d.EventID,
			*intent.CampaignID,
		)
		if err != nil {
			return intent, connection, nil, fmt.Errorf("recheck approved Teams source: %w", err)
		}
		if message.CampaignVersion != intent.CampaignVersion.Int64 ||
			message.Approval.ChannelRevisions["teams"] != intent.ChannelRevision ||
			message.Approval.PreviewID != *intent.PreviewID ||
			message.Approval.Fingerprint != intent.PreviewFingerprint.String {
			return intent, connection, nil, access.ErrConflict
		}
		subject, body = message.Subject, message.PlainText
	} else if intent.Kind != "connection_test" {
		return intent, connection, nil, access.ErrForbidden
	}
	payload, err := card(subject, body, d.ID)
	return intent, connection, payload, err
}

func (s Service) Eligible(ctx context.Context, d jobs.Delivery) (bool, error) {
	eligible := false
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		_, connection, _, err := s.validateDelivery(ctx, tx, d)
		if expectedBlock(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if connection.RetryAfter.Valid && s.now().Before(connection.RetryAfter.Time) {
			return nil
		}
		if _, openErr := s.open(connection); openErr != nil {
			return nil
		}
		last, err := q.TeamsWorkflowLatestAttempt(
			ctx,
			dbgen.TeamsWorkflowLatestAttemptParams{EventID: d.EventID, DeliveryID: d.TargetID},
		)
		if errors.Is(err, pgx.ErrNoRows) {
			eligible = true
			return nil
		}
		if err != nil {
			return fmt.Errorf("read Teams attempt eligibility: %w", err)
		}
		eligible = last.AttemptNumber < 5 && last.Outcome.Valid &&
			last.Outcome.String == "retryable" &&
			(!last.RetryAt.Valid || !s.now().Before(last.RetryAt.Time))
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("check workflow eligibility: %w", err)
	}
	return eligible, nil
}

func (s Service) Deliver(ctx context.Context, d jobs.Delivery) jobs.Outcome {
	attemptID := uuid.New()
	var fingerprint string
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		_, connection, payload, err := s.validateDelivery(ctx, tx, d)
		if err != nil {
			return err
		}
		if connection.RetryAfter.Valid && s.now().Before(connection.RetryAfter.Time) {
			return jobs.ErrRetry
		}
		last, err := q.TeamsWorkflowLatestAttempt(
			ctx,
			dbgen.TeamsWorkflowLatestAttemptParams{EventID: d.EventID, DeliveryID: d.TargetID},
		)
		number := int32(1)
		if err == nil {
			if !last.Outcome.Valid || last.Outcome.String != "retryable" ||
				last.AttemptNumber >= 5 {
				return access.ErrForbidden
			}
			if last.RetryAt.Valid && s.now().Before(last.RetryAt.Time) {
				return jobs.ErrRetry
			}
			number = last.AttemptNumber + 1
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read Teams retry: %w", err)
		}
		digest := sha256.Sum256(payload)
		fingerprint = fmt.Sprintf("%x", digest)
		if err = q.InsertTeamsWorkflowAttempt(
			ctx,
			dbgen.InsertTeamsWorkflowAttemptParams{
				EventID:            d.EventID,
				ID:                 attemptID,
				DeliveryID:         d.TargetID,
				AttemptNumber:      number,
				ConnectionVersion:  connection.Version,
				PayloadFingerprint: fingerprint,
				StartedAt:          s.now(),
			},
		); err != nil {
			return fmt.Errorf("retain Teams attempt before send: %w", err)
		}
		return q.SetTeamsWorkflowRateWindow(
			ctx,
			dbgen.SetTeamsWorkflowRateWindowParams{
				EventID:    d.EventID,
				RetryAfter: pgtype.Timestamptz{Time: s.now().Add(time.Second), Valid: true},
			},
		)
	})
	if errors.Is(err, jobs.ErrRetry) {
		return jobs.Retryable
	}
	if err != nil {
		if expectedBlock(err) {
			return jobs.Permanent
		}
		return jobs.Retryable
	}
	outcome := jobs.Ambiguous
	// The durable attempt precedes the network request. Recheck after that commit
	// and hold the event/connection locks through the bounded send so concurrent
	// revocation/source edits cannot cross the authority check and request.
	err = database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		_, connection, payload, err := s.validateDelivery(ctx, tx, d)
		r := result{Outcome: jobs.Permanent, Reason: "authority_or_source_changed"}
		if err == nil {
			digest := sha256.Sum256(payload)
			if fmt.Sprintf("%x", digest) != fingerprint {
				r.Reason = "source_changed_before_send"
			} else {
				endpoint, openErr := s.open(connection)
				if openErr != nil {
					r.Reason = "encryption_unavailable"
				} else {
					r = s.sender().send(ctx, endpoint, payload)
				}
			}
		} else if !expectedBlock(err) {
			return err
		}
		if r.Outcome == jobs.Retryable {
			last, readErr := q.TeamsWorkflowLatestAttempt(
				ctx,
				dbgen.TeamsWorkflowLatestAttemptParams{EventID: d.EventID, DeliveryID: d.TargetID},
			)
			if readErr != nil {
				return fmt.Errorf("read Teams bounded retry: %w", readErr)
			}
			if last.AttemptNumber >= 5 {
				r.Outcome = jobs.Permanent
				r.Reason = "retry_limit_reached"
			}
		}
		outcome = r.Outcome
		return s.record(ctx, q, d.EventID, attemptID, r)
	})
	if err != nil {
		return jobs.Ambiguous
	}
	return outcome
}

func (s Service) record(
	ctx context.Context,
	q *dbgen.Queries,
	eventID, attemptID uuid.UUID,
	r result,
) error {
	outcome := string(r.Outcome)
	if r.Outcome == jobs.Delivered {
		outcome = "provider_accepted"
	}
	retry := pgtype.Timestamptz{}
	if r.Outcome == jobs.Retryable {
		retry = pgtype.Timestamptz{Time: s.now().Add(r.RetryAfter), Valid: true}
		if err := q.SetTeamsWorkflowRateWindow(
			ctx,
			dbgen.SetTeamsWorkflowRateWindowParams{EventID: eventID, RetryAfter: retry},
		); err != nil {
			return fmt.Errorf("retain Teams rate window: %w", err)
		}
	}
	if r.Status < 0 || r.Status > 599 {
		return access.ErrInvalid
	}
	err := q.InsertTeamsWorkflowReceipt(
		ctx,
		dbgen.InsertTeamsWorkflowReceiptParams{
			EventID:    eventID,
			AttemptID:  attemptID,
			Outcome:    outcome,
			HttpStatus: pgtype.Int4{Int32: int32(r.Status), Valid: r.Status != 0},
			Reason:     r.Reason,
			RetryAt:    retry,
			RecordedAt: s.now(),
		},
	)
	if err != nil {
		return fmt.Errorf("record workflow receipt: %w", err)
	}
	return nil
}

func (s Service) Reconcile(ctx context.Context, d jobs.Delivery) jobs.Outcome {
	q := dbgen.New(s.Pool)
	last, err := q.TeamsWorkflowLatestAttempt(
		ctx,
		dbgen.TeamsWorkflowLatestAttemptParams{EventID: d.EventID, DeliveryID: d.TargetID},
	)
	if err != nil {
		return jobs.Ambiguous
	}
	if last.Outcome.Valid {
		switch last.Outcome.String {
		case "provider_accepted":
			return jobs.Delivered
		case "permanent":
			return jobs.Permanent
		case "retryable":
			if last.AttemptNumber < 5 {
				return jobs.Retryable
			}
		}
	}
	// No receipt proves acceptance or safe rejection after a crash. Never replay.
	_ = s.record(
		ctx,
		q,
		d.EventID,
		last.ID,
		result{Outcome: jobs.Ambiguous, Reason: "process_interrupted"},
	)
	return jobs.Ambiguous
}
