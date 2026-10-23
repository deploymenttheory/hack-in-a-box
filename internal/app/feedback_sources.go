package app

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/clarification"
	"github.com/deploymenttheory/hack-in-a-box/internal/feedback"
	"github.com/deploymenttheory/hack-in-a-box/internal/help"
	"github.com/deploymenttheory/hack-in-a-box/internal/observations"
)

type observationFeedbackSource struct{ Service observations.Service }

func (o observationFeedbackSource) AuthorizeInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
	ref feedback.Source,
) error {
	if ref.Kind != "observation" {
		return access.ErrNotFound
	}
	if err := o.Service.AuthorizeEvidenceInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		teamID,
		ref.ID,
		ref.Version,
	); err != nil {
		return fmt.Errorf("native observation provenance: %w", err)
	}
	return nil
}

type questionFeedbackSource struct{ Service *clarification.Service }

func (o questionFeedbackSource) AuthorizeInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
	ref feedback.Source,
) error {
	if ref.Kind != "question" || o.Service == nil {
		return access.ErrNotFound
	}
	if err := o.Service.AuthorizeQuestionReferenceInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		teamID,
		ref.ID,
		ref.Version,
	); err != nil {
		return fmt.Errorf("native question provenance: %w", err)
	}
	return nil
}

type helpFeedbackSource struct{ Service help.Service }

func (o helpFeedbackSource) AuthorizeInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID, teamID uuid.UUID,
	ref feedback.Source,
) error {
	if ref.Kind != "help" {
		return access.ErrNotFound
	}
	if err := o.Service.AuthorizeReferenceInTransaction(
		ctx,
		tx,
		actor,
		eventID,
		teamID,
		ref.ID,
		ref.Version,
	); err != nil {
		return fmt.Errorf("native help provenance: %w", err)
	}
	return nil
}
