package qualification

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/rubric"
)

func (s Service) signOff(
	ctx context.Context,
	tx pgx.Tx,
	actor, eventID uuid.UUID,
	v *scope,
	input Command,
) error {
	if actor != v.round.Panel.OrganiserID || v.round.Proposal == nil {
		return access.ErrForbidden
	}
	if err := validatePanel(ctx, dbgen.New(tx), eventID, v.round.Panel); err != nil {
		return failure("signoff", err)
	}
	fresh, err := s.proposal(ctx, tx, eventID, *v)
	if err != nil {
		return failure("signoff", err)
	}
	if fresh.Binding != v.round.Proposal.Binding || len(fresh.Blockers) > 0 {
		return access.ErrConflict
	}
	v.round.Proposal = &fresh
	v.round.Signoff = &Signoff{
		ActorID: actor,
		At:      s.now(),
		Binding: signedBinding(fresh, v.round),
		Reason:  input.Reason,
	}
	return nil
}

// AuthorizeFinalistInTransaction is the real native #107/#108 policy. It exports
// no mentor score, rank or prose and rechecks the exact current signed source in
// the caller's event-locked transaction. A reserve/category winner is not a finalist.
func (s Service) AuthorizeFinalistInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, teamID uuid.UUID,
) error {
	selected, err := s.SelectionInTransaction(ctx, tx, eventID)
	if err != nil {
		return err
	}
	if !slices.Contains(selected.FinalistIDs, teamID) {
		return access.ErrForbidden
	}
	return nil
}

// SelectionInTransaction is a narrow source for the separate results owner. It
// revalidates current signed native evidence and returns no mentor totals/ranks,
// private prose or publication grant. The caller must authorize its own current
// actor/recipient/command in this same transaction before exposing the metadata.
func (s Service) SelectionInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID uuid.UUID,
) (Selection, error) {
	runs, err := dbgen.New(tx).CurrentRubricRuns(ctx, eventID)
	if err != nil {
		return Selection{}, failure("signoff", err)
	}
	var runID uuid.UUID
	for _, run := range runs {
		if run.Purpose == "mentor_final" {
			runID = run.ID
		}
	}
	if runID == uuid.Nil {
		return Selection{}, access.ErrForbidden
	}
	round, err := load(ctx, tx, eventID, runID)
	if err != nil {
		return Selection{}, failure("signoff", err)
	}
	if round.Signoff == nil || round.Proposal == nil {
		return Selection{}, access.ErrForbidden
	}
	v, err := s.guard(ctx, tx, round.Signoff.ActorID, eventID, runID, currentRead)
	if err != nil {
		return Selection{}, failure("signoff", err)
	}
	fresh, err := s.proposal(ctx, tx, eventID, v)
	if err != nil {
		return Selection{}, failure("signoff", err)
	}
	if len(fresh.Blockers) > 0 || fresh.Binding != round.Proposal.Binding ||
		signedBinding(fresh, round) != round.Signoff.Binding {
		return Selection{}, access.ErrForbidden
	}
	return Selection{
		RunID:         runID,
		RoundVersion:  round.Version,
		Reference:     v.reference,
		SignedBinding: round.Signoff.Binding,
		SignerID:      round.Signoff.ActorID,
		SignedAt:      round.Signoff.At,
		FinalistIDs:   slices.Clone(fresh.FinalistIDs),
		ReserveIDs:    slices.Clone(fresh.ReserveIDs),
		Categories:    fresh.Categories,
	}, nil
}

var _ rubric.FinalistPolicy = Service{}
