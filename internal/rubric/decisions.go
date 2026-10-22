package rubric

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

func (s Service) reviewImpact(
	ctx context.Context,
	q *dbgen.Queries,
	row dbgen.Event,
	h *dbgen.EventRubric,
	actor uuid.UUID,
	input Command,
) error {
	if !row.PublishedAt.Valid || !h.ApprovedVersion.Valid ||
		input.Version != h.ApprovedVersion.Int64 ||
		(h.PublishedVersion.Valid && input.Version == h.PublishedVersion.Int64) {
		return access.ErrConflict
	}
	if strings.TrimSpace(input.ImpactReview) == "" || len(input.ImpactReview) > 10000 ||
		strings.TrimSpace(input.Notice) == "" ||
		len(input.Notice) > 4000 {
		return access.ErrInvalid
	}
	if _, err := q.RubricImpactReview(
		ctx,
		dbgen.RubricImpactReviewParams{EventID: h.EventID, RubricID: h.ID, Version: input.Version},
	); err == nil {
		return access.ErrConflict
	} else if !errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return fmt.Errorf("read rubric impact decision: %w", err)
	}
	if err := q.ReviewRubricImpact(
		ctx,
		dbgen.ReviewRubricImpactParams{
			EventID:      h.EventID,
			RubricID:     h.ID,
			Version:      input.Version,
			ImpactReview: input.ImpactReview,
			Notice:       input.Notice,
			ActorID:      actor,
			ReviewedAt:   s.now(),
		},
	); err != nil {
		return fmt.Errorf("retain rubric impact review: %w", err)
	}
	return record(ctx, q, h, &actor, input.Version, "impact_reviewed", input.Reason, s.now())
}

func (s Service) changeRun(
	ctx context.Context,
	q *dbgen.Queries,
	h *dbgen.EventRubric,
	actor uuid.UUID,
	input Command,
) error {
	if !h.ApprovedVersion.Valid || input.Version != h.ApprovedVersion.Int64 {
		return access.ErrConflict
	}
	if !slices.Contains([]string{"checkpoint", "mentor_final", "judge_final"}, input.Purpose) {
		return access.ErrInvalid
	}
	d, _, err := loadDefinition(ctx, q, *h, input.Version)
	if err != nil {
		return err
	}
	if d.Mode == AllTeams && input.Purpose == "mentor_final" {
		return access.ErrInvalid
	}
	runs, err := q.CurrentRubricRuns(ctx, h.EventID)
	if err != nil {
		return fmt.Errorf("read current assessment runs: %w", err)
	}
	var previous *uuid.UUID
	for _, run := range runs {
		if run.Purpose == input.Purpose {
			value := run.ID
			previous = &value
		}
	}
	if input.Action == "create_run" && (previous != nil || input.PreviousRunID != nil) {
		return access.ErrConflict
	}
	if input.Action == "restart_run" &&
		(previous == nil || input.PreviousRunID == nil || *previous != *input.PreviousRunID) {
		return access.ErrConflict
	}
	if previous != nil {
		if err = q.SupersedeRubricRun(
			ctx,
			dbgen.SupersedeRubricRunParams{EventID: h.EventID, ID: *previous},
		); err != nil {
			return fmt.Errorf("retain superseded rubric run: %w", err)
		}
	}
	if err = q.CreateRubricRun(
		ctx,
		dbgen.CreateRubricRunParams{
			EventID:       h.EventID,
			ID:            uuid.New(),
			RubricID:      h.ID,
			RubricVersion: input.Version,
			Purpose:       input.Purpose,
			PreviousID:    previous,
			Reason:        input.Reason,
			ActorID:       actor,
			CreatedAt:     s.now(),
		},
	); err != nil {
		return fmt.Errorf("create immutable assessment run: %w", err)
	}
	action := "run_created"
	if previous != nil {
		action = "run_restarted"
	}
	return record(ctx, q, h, &actor, input.Version, action, input.Reason, s.now())
}
