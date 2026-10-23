package moderation

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
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/mentors"
)

// Agenda contains coordination facts only. An organiser's metadata permission
// does not join a panel or disclose private card/proposal/evidence payloads.
type (
	Agenda struct {
		Items      []AgendaItem `json:"items"`
		NextCursor *uuid.UUID   `json:"next_cursor"`
	}
	AgendaItem struct {
		ID                uuid.UUID `json:"id"`
		Version           int64     `json:"version"`
		CheckpointID      uuid.UUID `json:"checkpoint_id"`
		CheckpointName    string    `json:"checkpoint_name"`
		Purpose           string    `json:"purpose"`
		State             string    `json:"state"`
		SourceState       string    `json:"source_state"`
		ModeratorID       uuid.UUID `json:"moderator_id"`
		PrimaryID         uuid.UUID `json:"primary_id"`
		SecondID          uuid.UUID `json:"second_id"`
		ReviewDueAt       time.Time `json:"review_due_at"`
		SecondComplete    bool      `json:"second_complete"`
		ReviewOverdue     bool      `json:"review_overdue"`
		GapCount          int       `json:"gap_count"`
		UnresolvedGaps    int       `json:"unresolved_gaps"`
		BlockingDecisions int       `json:"blocking_decisions"`
		PendingProposals  int       `json:"pending_proposals"`
		CanonicalSigned   bool      `json:"canonical_signed"`
		CanOpen           bool      `json:"can_open"`
		CanRecord         bool      `json:"can_record"`
		CanManage         bool      `json:"can_manage"`
		CanReconfigure    bool      `json:"can_reconfigure"`
	}
)

func (s Service) AgendaInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID, after uuid.UUID,
	limit int32,
) (Agenda, error) {
	out := Agenda{Items: []AgendaItem{}}
	if limit < 1 || limit > 100 {
		return out, access.ErrInvalid
	}
	if err := mentors.AuthorizeModerationPanelInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		teamID,
	); err != nil {
		return out, fmt.Errorf("authorize current agenda scope: %w", err)
	}
	q := dbgen.New(tx)
	roles, _, err := access.ContentReader(ctx, q, actor, eventID)
	if err != nil {
		return out, fmt.Errorf("authorize moderation agenda reader: %w", err)
	}
	organiser := slices.Contains(roles, "organiser")
	rows, err := tx.Query(
		ctx,
		`WITH latest AS(SELECT DISTINCT ON(id) id,document FROM moderation_huddle_versions WHERE event_id=$1 AND team_id=$2 AND id>$3 ORDER BY id,version DESC)SELECT document FROM latest WHERE $4 OR document->'panel_ids' ? $5 ORDER BY id LIMIT $6`,
		eventID,
		teamID,
		after,
		organiser,
		actor.String(),
		limit+1,
	)
	if err != nil {
		return out, fmt.Errorf("page permitted huddle agenda metadata: %w", err)
	}
	batch := []Huddle{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return out, fmt.Errorf("read huddle agenda snapshot: %w", err)
		}
		var h Huddle
		if err = json.Unmarshal(raw, &h); err != nil {
			rows.Close()
			return out, fmt.Errorf("decode huddle agenda metadata: %w", err)
		}
		batch = append(batch, h)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, fmt.Errorf("page current huddle agenda: %w", err)
	}
	if len(batch) > int(limit) {
		batch = batch[:limit]
		id := batch[len(batch)-1].Definition.ID
		out.NextCursor = &id
	}
	for _, h := range batch {
		item, readErr := s.agendaItem(ctx, tx, actor, eventID, h)
		if readErr != nil {
			return out, readErr
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

func (s Service) agendaItem(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	h Huddle,
) (AgendaItem, error) {
	out, err := s.agendaFacts(ctx, tx, actor, eventID, h)
	if err != nil {
		return out, err
	}
	run, err := dbgen.New(tx).
		RubricRun(ctx, dbgen.RubricRunParams{EventID: eventID, ID: h.Checkpoint.Definition.RunID})
	if err != nil {
		return out, fmt.Errorf("read agenda native source status: %w", err)
	}
	if run.Status != "current" {
		out.SourceState = "historical"
		return out, nil
	}
	eventRow, currentErr := s.current(ctx, tx, h.Definition.ModeratorID, eventID, h, false)
	if currentErr != nil {
		err = currentErr
		if isAgendaDenial(err) {
			return out, nil
		}
		return out, err
	}
	out.SourceState = "current"
	if out.SecondComplete {
		currentReview, reviewErr := s.Cards.SubmittedReviewInTransaction(
			ctx,
			tx,
			eventID,
			h.TeamID,
			h.Review.ReviewerID,
			h.Checkpoint,
		)
		if reviewErr != nil || h.Review.Card == nil || currentReview.Reference != *h.Review.Card {
			if reviewErr != nil && !isAgendaDenial(reviewErr) {
				return out, fmt.Errorf("validate current second review completion: %w", reviewErr)
			}
			out.SecondComplete = false
			out.ReviewOverdue = !s.now().Before(h.Review.DueAt) && h.State != "cancelled" &&
				h.State != "signed"
			out.SourceState = "review_required"
		}
	}
	if len(h.Cards) > 0 {
		if _, err = s.currentCards(ctx, tx, eventID, h); err != nil {
			if isAgendaDenial(err) {
				out.SourceState = "review_required"
				return out, nil
			}
			return out, err
		}
	}
	if count, countErr := s.unresolved(
		ctx,
		tx,
		h.Definition.ModeratorID,
		eventID,
		h,
	); countErr != nil {
		if isAgendaDenial(countErr) {
			out.SourceState = "review_required"
			return out, nil
		}
		return out, countErr
	} else {
		out.BlockingDecisions = count
	}
	if h.State == "signed" {
		decision, decisionErr := finalDecision(
			ctx,
			tx,
			eventID,
			h.TeamID,
			h.Checkpoint.Definition.RunID,
		)
		if decisionErr != nil {
			if isAgendaDenial(decisionErr) {
				out.SourceState = "review_required"
				return out, nil
			}
			return out, decisionErr
		}
		out.CanonicalSigned = decision.HuddleID == h.Definition.ID &&
			decision.HuddleVersion == h.Version &&
			h.SignoffID != nil &&
			decision.SignoffID == *h.SignoffID &&
			out.BlockingDecisions == 0
	}
	return s.agendaAccess(ctx, tx, actor, eventID, h, eventRow, out)
}

func (s Service) agendaAccess(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	h Huddle,
	eventRow dbgen.Event,
	out AgendaItem,
) (AgendaItem, error) {
	var err error
	if !slices.Contains(h.PanelIDs, actor) {
		return out, nil
	}
	if _, err = s.current(ctx, tx, actor, eventID, h, false); err != nil {
		if isAgendaDenial(err) {
			return out, nil
		}
		return out, err
	}
	if err = s.authorizeDecisionReader(ctx, tx, actor, eventID, h); err != nil {
		if isAgendaDenial(err) {
			return out, nil
		}
		return out, err
	}
	out.CanOpen = true
	out.CanRecord = eventRow.WritesAllowed &&
		slices.Contains([]string{"live", "assessment"}, eventRow.Stage) &&
		h.State != "signed" &&
		h.State != "cancelled"
	out.CanManage = out.CanRecord &&
		slices.Contains([]string{"live", "assessment"}, eventRow.Stage) &&
		actor == h.Definition.ModeratorID &&
		h.State != "signed" &&
		h.State != "cancelled"
	return out, nil
}

func isAgendaDenial(err error) bool {
	return errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) ||
		errors.Is(err, access.ErrConflict)
}

func (s Service) agendaFacts(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	h Huddle,
) (AgendaItem, error) {
	out := AgendaItem{
		ID:             h.Definition.ID,
		Version:        h.Version,
		CheckpointID:   h.Definition.CheckpointID,
		CheckpointName: h.Checkpoint.Definition.Name,
		Purpose:        h.Checkpoint.Purpose,
		State:          h.State,
		SourceState:    "review_required",
		ModeratorID:    h.Definition.ModeratorID,
		PrimaryID:      h.PrimaryID,
		SecondID:       h.Review.ReviewerID,
		ReviewDueAt:    h.Review.DueAt,
		SecondComplete: h.Review.CompletedAt != nil,
		GapCount:       len(h.Gaps),
	}
	out.ReviewOverdue = !out.SecondComplete && !s.now().Before(h.Review.DueAt) &&
		h.State != "cancelled" &&
		h.State != "signed"
	for _, gap := range h.Gaps {
		if gap.ResolvedAt == nil {
			out.UnresolvedGaps++
		}
	}
	for _, proposal := range h.Proposals {
		if proposal.State == "proposed" {
			out.PendingProposals++
		}
	}
	// A planned context may be repaired after #66 changes coverage. This
	// capability authorizes only the named current organiser's configure entry;
	// Change still validates the exact version and NEW published coverage.
	if actor == h.Definition.ModeratorID && h.State == "planned" {
		_, organiser, guardErr := s.guard(ctx, tx, actor, eventID, h.TeamID, true)
		if guardErr != nil && !isAgendaDenial(guardErr) {
			return out, fmt.Errorf("authorize planned configuration repair: %w", guardErr)
		}
		out.CanReconfigure = guardErr == nil && organiser
	}
	return out, nil
}
