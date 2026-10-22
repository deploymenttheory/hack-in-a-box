package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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

func (s Service) ensureOperation(
	ctx context.Context,
	tx pgx.Tx,
	row dbgen.CommunicationOccurrence,
) error {
	if s.Jobs == nil {
		return database.ErrDatabase
	}
	if row.OperationID != nil {
		return nil
	}
	input := dbgen.InsertOperationParams{
		EventID:      row.EventID,
		OperationKey: fmt.Sprintf("communication-dispatch:%s:%d", row.ID, row.ReviewRevision),
		Kind:         HandlerKind,
		TargetID:     row.ID,
		DueAt:        row.DueAt,
		Approved:     true,
	}
	binding, err := loadBinding(ctx, dbgen.New(tx), row.EventID, row.BindingID)
	if err != nil {
		return fmt.Errorf("dispatch communication: %w", err)
	}
	var op dbgen.Operation
	if binding.Definition.RetainAfterArchive {
		op, err = s.Jobs.EnqueueRetained(ctx, tx, input, row.ID)
	} else {
		op, err = s.Jobs.Enqueue(ctx, tx, input)
	}
	if err != nil {
		return fmt.Errorf("enqueue durable communication occurrence: %w", err)
	}
	if err = dbgen.New(tx).
		SetAutomationOperation(ctx, dbgen.SetAutomationOperationParams{EventID: row.EventID, ID: row.ID, OperationID: &op.ID}); err != nil {
		return fmt.Errorf("retain communication operation: %w", err)
	}
	return nil
}

func (s Service) discover(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
	b Binding,
) ([]Source, error) {
	if slices.Contains(
		[]string{"scheduled", "preparation.reminder", "follow_up"},
		b.Definition.Trigger,
	) {
		return []Source{
			{Kind: "schedule", ID: b.Definition.ID, Revision: b.Version, OccurredAt: b.DueAt},
		}, nil
	}
	if b.Definition.Trigger == "outcome.follow_up" && b.Definition.SourceID != nil {
		return []Source{
			{
				Kind:       "outcome.follow_up",
				ID:         *b.Definition.SourceID,
				Revision:   b.Definition.SourceVersion,
				OccurredAt: b.DueAt,
			},
		}, nil
	}
	policy := s.sourcePolicy(b.Definition.Trigger)
	if paged, ok := policy.(interface {
		DiscoverUnknownInTransaction(context.Context, pgx.Tx, uuid.UUID, Binding) ([]Source, error)
	}); ok {
		sources, err := paged.DiscoverUnknownInTransaction(ctx, tx, eventID, b)
		if err != nil {
			return nil, fmt.Errorf("discover unknown lifecycle sources: %w", err)
		}
		return sources, nil
	}
	sources, err := policy.DiscoverInTransaction(ctx, tx, eventID, b.Definition)
	if err != nil {
		return nil, fmt.Errorf("discover communication source: %w", err)
	}
	return sources, nil
}

func (s Service) reconcileEvent(ctx context.Context, eventID uuid.UUID) error {
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		event, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock communication reconciliation: %w", err)
		}
		if s.SourceMaintenance != nil {
			if err = s.SourceMaintenance(ctx, tx, eventID); err != nil {
				return fmt.Errorf("maintain native communication sources: %w", err)
			}
		}
		paused, err := q.AutomationPaused(ctx, eventID)
		if err != nil {
			return fmt.Errorf("read reconciliation pause: %w", err)
		}
		afterBinding := uuid.Nil
		for {
			rows, err := q.AutomationBindingsPage(
				ctx,
				dbgen.AutomationBindingsPageParams{
					EventID:  eventID,
					AfterID:  afterBinding,
					PageSize: 200,
				},
			)
			if err != nil {
				return fmt.Errorf("read reconciliation bindings: %w", err)
			}
			for _, raw := range rows {
				var binding Binding
				if err = json.Unmarshal(raw.Document, &binding); err != nil {
					return fmt.Errorf("decode reconciliation binding: %w", err)
				}
				if binding.Cancelled || binding.ApprovedAt == nil {
					continue
				}
				sources, err := s.discover(ctx, tx, eventID, binding)
				if errors.Is(err, communications.ErrUnresolved) {
					continue
				}
				if err != nil {
					return fmt.Errorf("dispatch communication: %w", err)
				}
				for _, source := range sources {
					if err = s.reconcileOccurrence(
						ctx,
						tx,
						q,
						event,
						binding,
						source,
						paused,
					); err != nil {
						return err
					}
				}
			}
			if len(rows) < 200 {
				break
			}
			afterBinding = rows[len(rows)-1].ID
		}
		return s.reconcilePending(ctx, tx, event, paused)
	})
	if err != nil {
		return fmt.Errorf("reconcile communication event: %w", err)
	}
	return nil
}

func (s Service) Maintain(ctx context.Context) error {
	cursor := uuid.Nil
	for {
		ids, err := dbgen.New(s.Pool).AutomationEvents(ctx, cursor)
		if err != nil {
			return fmt.Errorf("find communication automation events: %w", err)
		}
		if len(ids) == 0 {
			return nil
		}
		for _, id := range ids {
			if err = s.reconcileEvent(ctx, id); err != nil {
				return fmt.Errorf("dispatch communication: %w", err)
			}
			cursor = id
		}
	}
}

func (s Service) Eligible(ctx context.Context, d jobs.Delivery) (bool, error) {
	eligible := false
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(ctx, d.EventID); err != nil {
			return fmt.Errorf("lock communication eligibility: %w", err)
		}
		row, err := q.AutomationOccurrence(
			ctx,
			dbgen.AutomationOccurrenceParams{EventID: d.EventID, ID: d.TargetID},
		)
		if err != nil {
			return fmt.Errorf("read communication eligibility: %w", err)
		}
		if row.OperationID == nil || *row.OperationID != d.ID {
			return nil
		}
		if slices.Contains([]string{"queued", "partially_queued"}, row.State) {
			eligible = true
			return nil
		}
		binding, err := loadBinding(ctx, q, d.EventID, row.BindingID)
		if err != nil {
			return fmt.Errorf("dispatch communication: %w", err)
		}
		_, err = s.ResolveInTransaction(
			ctx,
			tx,
			d.EventID,
			binding.Definition.CampaignID,
			binding.Definition.CampaignVersion,
			communications.DeliveryScope{OccurrenceID: row.ID},
		)
		if errors.Is(err, communications.ErrUnresolved) || errors.Is(err, access.ErrForbidden) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("dispatch communication: %w", err)
		}
		eligible = true
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("check communication dispatch eligibility: %w", err)
	}
	return eligible, nil
}

func (s Service) Deliver(ctx context.Context, d jobs.Delivery) jobs.Outcome {
	var attempted *dbgen.CommunicationOccurrence
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if _, err := q.LockEvent(ctx, d.EventID); err != nil {
			return fmt.Errorf("lock communication dispatch: %w", err)
		}
		row, err := q.AutomationOccurrence(
			ctx,
			dbgen.AutomationOccurrenceParams{EventID: d.EventID, ID: d.TargetID},
		)
		if err != nil {
			return fmt.Errorf("read communication dispatch: %w", err)
		}
		if row.OperationID == nil || *row.OperationID != d.ID {
			return access.ErrConflict
		}
		if slices.Contains([]string{"queued", "partially_queued"}, row.State) {
			return nil
		}
		if row.State != "pending" {
			return access.ErrConflict
		}
		attempted = &row
		binding, err := loadBinding(ctx, q, d.EventID, row.BindingID)
		if err != nil {
			return fmt.Errorf("dispatch communication: %w", err)
		}
		scope := communications.DeliveryScope{OccurrenceID: row.ID}
		approved, err := s.Composer.ResolveApprovedScopedCampaignInTransaction(
			ctx,
			tx,
			d.EventID,
			binding.Definition.CampaignID,
			binding.Definition.CampaignVersion,
			scope,
			s,
		)
		if err != nil {
			return fmt.Errorf("dispatch communication: %w", err)
		}
		reports, state, reason, err := s.queueChannels(
			ctx,
			tx,
			d.EventID,
			binding.Definition.CampaignID,
			binding.Definition.CampaignVersion,
			scope,
			approved.Definition.Channels,
		)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(reports)
		if err != nil {
			return fmt.Errorf("encode channel reports: %w", err)
		}
		if err = q.RecordAutomationChannelReports(
			ctx,
			dbgen.RecordAutomationChannelReportsParams{
				EventID:        d.EventID,
				ID:             row.ID,
				State:          state,
				Reason:         reason,
				ChannelReports: raw,
			},
		); err != nil {
			return fmt.Errorf("record communication dispatch reports: %w", err)
		}
		return history(
			ctx,
			q,
			d.EventID,
			&row.BindingID,
			&row.ID,
			nil,
			state,
			reason,
		)
	})
	if err == nil {
		return jobs.Delivered
	}
	if errors.Is(err, communications.ErrUnresolved) || errors.Is(err, access.ErrForbidden) ||
		errors.Is(err, access.ErrConflict) {
		// No provider effects occur here. Queue writes share the rolled-back outer
		// transaction; preserve a review state instead of retrying a stale burst.
		blockedErr := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
			_, lockErr := q.LockEvent(ctx, d.EventID)
			if lockErr != nil {
				return fmt.Errorf("lock blocked communication: %w", lockErr)
			}
			current, readErr := q.AutomationOccurrence(
				ctx,
				dbgen.AutomationOccurrenceParams{EventID: d.EventID, ID: d.TargetID},
			)
			if readErr != nil {
				return fmt.Errorf("read blocked communication: %w", readErr)
			}
			// The first transaction rolled back. A review/cancellation/dispatch may
			// have won meanwhile; a retired attempt cannot mutate its successor.
			if attempted == nil || current.OperationID == nil || *current.OperationID != d.ID ||
				current.State != "pending" || current.ReviewRevision != attempted.ReviewRevision ||
				current.ScopeBinding != attempted.ScopeBinding || !current.UpdatedAt.Equal(attempted.UpdatedAt) {
				return nil
			}
			return q.SetAutomationOccurrenceState(
				ctx,
				dbgen.SetAutomationOccurrenceStateParams{
					EventID:     d.EventID,
					ID:          d.TargetID,
					State:       "blocked",
					Reason:      "current_approval_or_source_review_required",
					ReviewUntil: pgtype.Timestamptz{},
				},
			)
		})
		if blockedErr != nil {
			return jobs.Retryable
		}
		// Explicit review creates a fresh orchestration generation; the obsolete
		// attempt must not remain an automatically retrying pending operation.
		return jobs.Permanent
	}
	return jobs.Retryable
}

func (s Service) Reconcile(ctx context.Context, d jobs.Delivery) jobs.Outcome {
	row, err := dbgen.New(s.Pool).
		AutomationOccurrence(ctx, dbgen.AutomationOccurrenceParams{EventID: d.EventID, ID: d.TargetID})
	if err != nil {
		return jobs.Retryable
	}
	if row.OperationID == nil || *row.OperationID != d.ID {
		return jobs.Permanent
	}
	if slices.Contains([]string{"queued", "partially_queued"}, row.State) {
		return jobs.Delivered
	}
	if row.State == "cancelled" {
		return jobs.Permanent
	}
	return jobs.Retryable
}

func (s Service) ReadOnlyEligible(ctx context.Context, d jobs.Delivery) (bool, error) {
	eligible := false
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := q.AutomationOccurrence(
			ctx,
			dbgen.AutomationOccurrenceParams{EventID: d.EventID, ID: d.TargetID},
		)
		if err != nil {
			return fmt.Errorf("read retained dispatch proof: %w", err)
		}
		if row.OperationID == nil || *row.OperationID != d.ID {
			return nil
		}
		binding, err := loadBinding(ctx, q, d.EventID, row.BindingID)
		if err != nil {
			return fmt.Errorf("dispatch communication: %w", err)
		}
		audience, err := s.ResolveInTransaction(
			ctx,
			tx,
			d.EventID,
			binding.Definition.CampaignID,
			binding.Definition.CampaignVersion,
			communications.DeliveryScope{OccurrenceID: row.ID},
		)
		if errors.Is(err, communications.ErrUnresolved) || errors.Is(err, access.ErrForbidden) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("dispatch communication: %w", err)
		}
		eligible = audience.AllowArchived
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("verify retained dispatch: %w", err)
	}
	return eligible, nil
}

func (s Service) reconcileOccurrence(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	event dbgen.Event,
	binding Binding,
	source Source,
	paused bool,
) error {
	eventID := event.ID
	due := source.OccurredAt
	if due.Before(binding.DueAt) {
		due = binding.DueAt
	}
	deadline := due.Add(
		time.Duration(binding.Definition.MaximumLatenessSeconds) * time.Second,
	)
	state, reason := "pending", ""
	switch {
	case !event.WritesAllowed && !binding.Definition.RetainAfterArchive:
		state = "cancelled"
		reason = "event_closed"
	case source.OccurredAt.Before(*binding.ApprovedAt) && source.Kind != "schedule":
		state = "missed"
		reason = "source_precedes_approval"
	case s.now().After(deadline):
		state = "missed"
		reason = "dispatch_window_missed"
	case paused:
		state = "paused"
		reason = "event_automation_paused"
	}
	id := uuid.New()
	row, err := q.SaveAutomationOccurrence(
		ctx,
		dbgen.SaveAutomationOccurrenceParams{
			EventID:        eventID,
			ID:             id,
			BindingID:      binding.Definition.ID,
			BindingVersion: binding.Version,
			SourceKind:     source.Kind,
			SourceID:       source.ID,
			SourceRevision: source.Revision,
			SourceKey:      sourceKey(source),
			DueAt:          due,
			Deadline:       deadline,
			ScopeBinding:   scopeBinding(binding, source, 0),
			State:          state,
			Reason:         reason,
		},
	)
	if err != nil {
		return fmt.Errorf("retain communication occurrence: %w", err)
	}
	if row.ID == id {
		if err = history(
			ctx,
			q,
			eventID,
			&binding.Definition.ID,
			&id,
			nil,
			"discovered",
			reason,
		); err != nil {
			return fmt.Errorf("dispatch communication: %w", err)
		}
	} else if !slices.Contains([]string{"cancelled", "queued", "partially_queued"}, row.State) {
		until := row.Deadline
		if row.ReviewUntil.Valid {
			until = row.ReviewUntil.Time
		}
		next, nextReason := row.State, row.Reason
		switch {
		case !event.WritesAllowed && !binding.Definition.RetainAfterArchive:
			next = "cancelled"
			nextReason = "event_closed"
		case s.now().After(until):
			next = "missed"
			nextReason = "dispatch_window_missed"
		case paused:
			next = "paused"
			nextReason = "event_automation_paused"
		case row.State == "paused":
			next = "pending"
			nextReason = ""
		}
		if next != row.State || nextReason != row.Reason {
			if err = q.SetAutomationOccurrenceState(
				ctx,
				dbgen.SetAutomationOccurrenceStateParams{
					EventID:     eventID,
					ID:          row.ID,
					State:       next,
					Reason:      nextReason,
					ReviewUntil: row.ReviewUntil,
				},
			); err != nil {
				return fmt.Errorf("retain communication review state: %w", err)
			}
			if err = history(
				ctx,
				q,
				eventID,
				&binding.Definition.ID,
				&row.ID,
				nil,
				next,
				nextReason,
			); err != nil {
				return fmt.Errorf("dispatch communication: %w", err)
			}
			row.State = next
		}
	}
	if row.State == "pending" || row.State == "paused" {
		if err = s.ensureOperation(ctx, tx, row); err != nil {
			return fmt.Errorf("dispatch communication: %w", err)
		}
	}
	return nil
}
