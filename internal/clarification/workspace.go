package clarification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

// ReviewAudience is an organiser-only aggregate of the exact common audience
// policy used at publication. It is neither a person directory nor an approval.
type ReviewAudience struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

type Workspace struct {
	Source               Source          `json:"source"`
	Archived             bool            `json:"archived"`
	TeamID               *uuid.UUID      `json:"team_id"`
	CanAsk               bool            `json:"can_ask"`
	CanReview            bool            `json:"can_review"`
	StaffAttachmentsOpen bool            `json:"staff_attachments_open"`
	ReviewAudience       *ReviewAudience `json:"review_audience"`
}

// Workspace derives capabilities and the staff preview under the owning event
// lock and actual common-source gate. Each later action rechecks its own policy.
func (s Service) Workspace(ctx context.Context, actor, eventID uuid.UUID) (Workspace, error) {
	var out Workspace
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, _ *dbgen.Queries) error {
		var err error
		out, err = s.workspaceInTransaction(ctx, tx, actor, eventID)
		return err
	})
	if err != nil {
		return Workspace{}, fmt.Errorf("clarification workspace: %w", err)
	}
	return out, nil
}

func (s Service) workspaceInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
) (Workspace, error) {
	var out Workspace
	q := dbgen.New(tx)
	now := s.now()
	ref, err := rubric.ReleasedSourceForReaderInTransaction(ctx, tx, actor, eventID, now)
	if err != nil {
		return Workspace{}, fmt.Errorf("authorize clarification workspace: %w", err)
	}
	row, err := q.Event(ctx, eventID)
	if err != nil {
		return Workspace{}, fmt.Errorf("read clarification workspace policy: %w", err)
	}
	roles, confirmed, err := access.ContentReader(ctx, q, actor, eventID)
	if err != nil {
		return Workspace{}, fmt.Errorf("read current clarification capabilities: %w", err)
	}
	out.Source, out.Archived = source(ref), !row.WritesAllowed
	out.CanReview = slices.Contains(roles, "organiser")
	if confirmed {
		membership, memberErr := q.TeamAssignment(ctx, dbgen.TeamAssignmentParams{
			EventID: eventID, PersonID: actor, Snapshot: "published",
		})
		if memberErr != nil && !errors.Is(memberErr, pgx.ErrNoRows) {
			return Workspace{}, fmt.Errorf("read current question team: %w", memberErr)
		}
		if memberErr == nil {
			if memberErr = teamAuthority(
				ctx,
				q,
				actor,
				eventID,
				membership.TeamID,
				false,
			); memberErr == nil {
				out.TeamID = &membership.TeamID
				out.CanAsk = row.WritesAllowed
			} else if !errors.Is(
				memberErr,
				access.ErrNotFound,
			) {
				return Workspace{}, memberErr
			}
		}
	}
	if out.CanReview {
		recipients, recipientErr := q.ClarificationRecipients(ctx, eventID)
		if recipientErr != nil {
			return Workspace{}, fmt.Errorf(
				"preview current common answer audience: %w",
				recipientErr,
			)
		}
		out.ReviewAudience = &ReviewAudience{
			Label: "Current confirmed attendees",
			Count: len(recipients),
		}
		var dates struct {
			LiveStart time.Time `json:"live_start"`
		}
		if err = json.Unmarshal(row.Configuration, &dates); err != nil {
			return Workspace{}, fmt.Errorf("read staff question attachment gate: %w", err)
		}
		out.StaffAttachmentsOpen = !dates.LiveStart.IsZero() && !now.Before(dates.LiveStart)
	}
	return out, nil
}
