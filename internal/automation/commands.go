package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/communications"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func validateDefinition(d Definition) error {
	if d.ID == uuid.Nil || d.CampaignID == uuid.Nil || d.CampaignVersion < 1 ||
		d.MaximumLatenessSeconds < 1 ||
		d.MaximumLatenessSeconds > 86400 ||
		!slices.Contains(
			[]string{
				"scheduled",
				"preparation.reminder",
				"follow_up",
				"facilitator.selected",
				"registration.offer",
				"registration.place_changed",
				"teams.published",
				"mentors.published",
				"challenge.released",
				"brief.released",
				"event.rescheduled",
				"help.updated",
				"clarification.published",
				"rubric.published",
				"results.published",
				"outcome.follow_up",
			},
			d.Trigger,
		) {
		return access.ErrInvalid
	}
	if d.SourceID != nil && *d.SourceID == uuid.Nil {
		return access.ErrInvalid
	}
	if d.RetainAfterArchive &&
		(d.Trigger != "outcome.follow_up" || d.RetainUntil == nil || d.SourceID == nil || d.SourceVersion < 1) {
		return access.ErrInvalid
	}
	if !d.RetainAfterArchive && d.RetainUntil != nil {
		return access.ErrInvalid
	}
	return nil
}

func history(
	ctx context.Context,
	q *dbgen.Queries,
	eventID uuid.UUID,
	bindingID, occurrenceID, actor *uuid.UUID,
	action, reason string,
) error {
	err := q.RecordAutomationHistory(
		ctx,
		dbgen.RecordAutomationHistoryParams{
			EventID:      eventID,
			BindingID:    bindingID,
			OccurrenceID: occurrenceID,
			ActorID:      actor,
			Action:       action,
			Reason:       reason,
		},
	)
	if err != nil {
		return fmt.Errorf("record automation decision: %w", err)
	}
	return nil
}

func (s Service) Change(
	ctx context.Context,
	actor, eventID uuid.UUID,
	revision int64,
	input Command,
) (Page, *communications.PublicationPlanPreview, error) {
	if s.Pool == nil || s.Composer == nil || strings.TrimSpace(input.Reason) == "" ||
		len(input.Reason) > 2000 {
		return Page{}, nil, access.ErrInvalid
	}
	var preview *communications.PublicationPlanPreview
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		row, err := q.LockEvent(ctx, eventID)
		if err != nil {
			return fmt.Errorf("lock automation command: %w", err)
		}
		if err = (access.EventPolicy{Queries: q}).Authorize(
			ctx,
			actor,
			eventID,
			eventID,
			access.StaffRead,
		); err != nil {
			return fmt.Errorf("authorize automation command: %w", err)
		}
		if row.Revision != revision {
			return access.ErrConflict
		}
		if !row.WritesAllowed &&
			!slices.Contains(
				[]string{"pause", "resume", "review_occurrence", "cancel", "cancel_occurrence"},
				input.Action,
			) {
			return access.ErrForbidden
		}
		switch input.Action {
		case "pause", "resume":
			if err = q.SetAutomationPause(
				ctx,
				dbgen.SetAutomationPauseParams{
					EventID: eventID,
					Paused:  input.Action == "pause",
					ActorID: actor,
					Reason:  input.Reason,
				},
			); err != nil {
				return fmt.Errorf("change automation pause: %w", err)
			}
			if err = history(
				ctx,
				q,
				eventID,
				nil,
				nil,
				&actor,
				input.Action,
				input.Reason,
			); err != nil {
				return fmt.Errorf("change automation binding: %w", err)
			}
		case "review_occurrence", "cancel_occurrence":
			occurrence, err := q.AutomationOccurrence(
				ctx,
				dbgen.AutomationOccurrenceParams{EventID: eventID, ID: input.OccurrenceID},
			)
			if err != nil {
				return fmt.Errorf("read automation review: %w", err)
			}
			if occurrence.State == "cancelled" {
				return access.ErrConflict
			}
			state, reason, until := "cancelled", "organiser_cancelled", pgtype.Timestamptz{}
			if input.Action == "review_occurrence" {
				binding, err := loadBinding(ctx, q, eventID, occurrence.BindingID)
				if err != nil {
					return fmt.Errorf("change automation binding: %w", err)
				}
				until = pgtype.Timestamptz{
					Time: s.now().
						Add(time.Duration(binding.Definition.MaximumLatenessSeconds) * time.Second),
					Valid: true,
				}
				state = "pending"
				reason = "explicit_late_review"
			}
			if err = q.SetAutomationOccurrenceState(
				ctx,
				dbgen.SetAutomationOccurrenceStateParams{
					EventID:     eventID,
					ID:          occurrence.ID,
					State:       state,
					Reason:      reason,
					ReviewUntil: until,
				},
			); err != nil {
				return fmt.Errorf("record occurrence review: %w", err)
			}
			if input.Action == "review_occurrence" {
				binding, err := loadBinding(ctx, q, eventID, occurrence.BindingID)
				if err != nil {
					return fmt.Errorf("change automation binding: %w", err)
				}
				scope := communications.DeliveryScope{OccurrenceID: occurrence.ID}
				if _, err = s.ResolveInTransaction(
					ctx,
					tx,
					eventID,
					binding.Definition.CampaignID,
					binding.Definition.CampaignVersion,
					scope,
				); err != nil {
					return fmt.Errorf("change automation binding: %w", err)
				}
				if _, err = s.Composer.ResolveApprovedScopedCampaignInTransaction(
					ctx,
					tx,
					eventID,
					binding.Definition.CampaignID,
					binding.Definition.CampaignVersion,
					scope,
					s,
				); err != nil {
					return fmt.Errorf("change automation binding: %w", err)
				}
				if err = s.ensureOperation(ctx, tx, occurrence); err != nil {
					return fmt.Errorf("change automation binding: %w", err)
				}
			}
			if err = history(
				ctx,
				q,
				eventID,
				&occurrence.BindingID,
				&occurrence.ID,
				&actor,
				input.Action,
				input.Reason,
			); err != nil {
				return fmt.Errorf("change automation binding: %w", err)
			}
		case "save", "approve", "cancel", "preview_publication", "approve_publication":
			if err = s.changeBinding(ctx, tx, q, actor, eventID, input, &preview); err != nil {
				return fmt.Errorf("change automation binding: %w", err)
			}
		default:
			return access.ErrInvalid
		}
		if _, err = q.BumpEventRevision(ctx, eventID); err != nil {
			return fmt.Errorf("advance automation revision: %w", err)
		}
		return nil
	})
	if err != nil {
		return Page{}, nil, fmt.Errorf("change automation: %w", err)
	}
	page, err := s.Read(ctx, actor, eventID)
	return page, preview, err
}

func (s Service) changeBinding(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	actor, eventID uuid.UUID,
	input Command,
	preview **communications.PublicationPlanPreview,
) error {
	id := input.BindingID
	if input.Action == "save" && input.Definition != nil {
		id = input.Definition.ID
	}
	binding, err := loadBinding(ctx, q, eventID, id)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("change automation binding: %w", err)
	}
	if err != nil && input.Action != "save" {
		return access.ErrNotFound
	}
	if input.Version != binding.Version {
		return access.ErrConflict
	}
	if input.Action == "save" {
		if input.Definition == nil {
			return access.ErrInvalid
		}
		if err = validateDefinition(*input.Definition); err != nil {
			return fmt.Errorf("change automation binding: %w", err)
		}
		campaign, err := currentCampaign(ctx, q, eventID, input.Definition.CampaignID)
		if err != nil {
			return fmt.Errorf("change automation binding: %w", err)
		}
		if campaign.Version != input.Definition.CampaignVersion || campaign.Cancelled {
			return access.ErrConflict
		}
		binding = Binding{
			Definition: *input.Definition,
			Version:    binding.Version + 1,
			ActorID:    actor,
		}
	} else {
		if binding.Cancelled {
			return access.ErrConflict
		}
		switch input.Action {
		case "preview_publication":
			p, err := s.previewPublicationBinding(ctx, tx, actor, eventID, binding, input)
			if err != nil {
				return fmt.Errorf("change automation binding: %w", err)
			}
			*preview = p
			return history(ctx, q, eventID, &id, nil, &actor, input.Action, input.Reason)
		case "approve_publication":
			binding, err = s.approvePublicationBinding(ctx, tx, q, actor, eventID, binding, input)
			if err != nil {
				return fmt.Errorf("change automation binding: %w", err)
			}
		case "approve":
			binding, err = s.approveGenericBinding(ctx, tx, q, eventID, binding)
			if err != nil {
				return fmt.Errorf("change automation binding: %w", err)
			}
		case "cancel":
			binding.Cancelled = true
		}
		binding.Version++
		binding.ActorID = actor
		if input.Action == "approve" || input.Action == "approve_publication" {
			now := s.now()
			binding.ApprovedAt = &now
		}
	}
	raw, err := json.Marshal(binding)
	if err != nil {
		return fmt.Errorf("encode automation binding: %w", err)
	}
	if err = q.SaveAutomationBinding(
		ctx,
		dbgen.SaveAutomationBindingParams{
			EventID:         eventID,
			ID:              binding.Definition.ID,
			Version:         binding.Version,
			CampaignID:      binding.Definition.CampaignID,
			CampaignVersion: binding.Definition.CampaignVersion,
			Document:        raw,
			ActorID:         actor,
			Reason:          input.Reason,
		},
	); err != nil {
		return fmt.Errorf("save automation binding: %w", err)
	}
	if err = q.CancelAutomationBindingOccurrences(
		ctx,
		dbgen.CancelAutomationBindingOccurrencesParams{
			EventID:        eventID,
			BindingID:      id,
			BindingVersion: binding.Version,
		},
	); err != nil {
		return fmt.Errorf("invalidate old automation intents: %w", err)
	}
	return history(ctx, q, eventID, &id, nil, &actor, input.Action, input.Reason)
}

func (s Service) previewPublicationBinding(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	binding Binding,
	input Command,
) (*communications.PublicationPlanPreview, error) {
	if binding.Definition.SourceID == nil || binding.Definition.SourceVersion < 1 ||
		!slices.Contains(
			[]string{"challenge.released", "brief.released"},
			binding.Definition.Trigger,
		) {
		return nil, access.ErrInvalid
	}
	p, err := s.Composer.PreviewExpectedPublicationInTransaction(
		ctx,
		tx,
		eventID,
		actor,
		binding.Definition.CampaignID,
		*binding.Definition.SourceID,
		binding.Definition.CampaignVersion,
		binding.Definition.SourceVersion,
		input.Reason,
	)
	if err != nil {
		return nil, fmt.Errorf("preview publication binding: %w", err)
	}
	return &p, nil
}

func (s Service) approvePublicationBinding(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	actor, eventID uuid.UUID,
	binding Binding,
	input Command,
) (Binding, error) {
	plan, err := s.Composer.ApproveExpectedPublicationInTransaction(
		ctx,
		tx,
		eventID,
		actor,
		input.PreviewPlanID,
		input.Reason,
	)
	if err != nil {
		return binding, fmt.Errorf("approve publication binding: %w", err)
	}
	if binding.Definition.SourceID == nil ||
		plan.SourceID != *binding.Definition.SourceID ||
		plan.SourceVersion != binding.Definition.SourceVersion {
		return binding, access.ErrConflict
	}
	record, err := q.PublicationPlanApproval(
		ctx,
		dbgen.PublicationPlanApprovalParams{EventID: eventID, ID: plan.ID},
	)
	if err != nil {
		return binding, fmt.Errorf("read reviewed plan binding: %w", err)
	}
	if record.CampaignID != binding.Definition.CampaignID ||
		record.CampaignVersion != binding.Definition.CampaignVersion {
		return binding, access.ErrConflict
	}
	var proof struct {
		DueAt time.Time `json:"due_at"`
	}
	if err = json.Unmarshal(record.Document, &proof); err != nil {
		return binding, fmt.Errorf("read reviewed publication due time: %w", err)
	}
	binding.PublicationApprovalID = &plan.ID
	binding.CampaignApproval = plan.Binding
	binding.DueAt = proof.DueAt
	return binding, nil
}

func (s Service) approveGenericBinding(
	ctx context.Context,
	tx pgx.Tx,
	q *dbgen.Queries,
	eventID uuid.UUID,
	binding Binding,
) (Binding, error) {
	approved, err := s.Composer.ReviewCampaignApprovalInTransaction(
		ctx,
		tx,
		eventID,
		binding.Definition.CampaignID,
	)
	if err != nil {
		return binding, fmt.Errorf("approve automation campaign: %w", err)
	}
	if approved.Version != binding.Definition.CampaignVersion {
		return binding, access.ErrConflict
	}
	if binding.Definition.Trigger == "event.rescheduled" &&
		approved.Purpose != "postponement" {
		return binding, access.ErrInvalid
	}
	if binding.Definition.RetainAfterArchive {
		if approved.Purpose != "outcome_review" {
			return binding, access.ErrInvalid
		}
		event, err := q.Event(ctx, eventID)
		if err != nil {
			return binding, fmt.Errorf("read retained approval event: %w", err)
		}
		policy, err := q.PrivacyPolicy(ctx, eventID)
		if err != nil {
			return binding, fmt.Errorf("read retained approval policy: %w", err)
		}
		if !event.CompletedAt.Valid || event.Stage != "completed" ||
			binding.Definition.RetainUntil == nil ||
			!s.now().Before(*binding.Definition.RetainUntil) ||
			binding.Definition.RetainUntil.After(
				event.CompletedAt.Time.AddDate(0, 0, int(policy.RetentionDays)),
			) {
			return binding, access.ErrInvalid
		}
	}

	if slices.Contains(approved.Definition.Channels, "teams") &&
		slices.Contains(
			[]string{
				"facilitator.selected",
				"registration.offer",
				"registration.place_changed",
				"help.updated",
				"outcome.follow_up",
			},
			binding.Definition.Trigger,
		) {
		return binding, access.ErrForbidden
	}
	binding.CampaignApproval = approved.Approval.Binding
	binding.DueAt = approved.DueAt
	binding.PublicationApprovalID = nil
	return binding, nil
}
