package outcomes

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

// Receipts are read-only exact command outcomes. A missing receipt never
// executes a request; current authority is checked before retained data is read.
func (s Service) Receipt(ctx context.Context, actor, id, commandID uuid.UUID) (View, error) {
	var out View
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if err := active(ctx, q, actor); err != nil {
			return err
		}
		head, err := q.OutcomeHead(ctx, id)
		if err != nil {
			return wrap("read receipt resource", err)
		}
		if _, err = capabilities(ctx, q, actor, head, s.now()); err != nil {
			return err
		}
		row, err := q.OutcomeCommand(
			ctx,
			dbgen.OutcomeCommandParams{OutcomeID: id, CommandID: commandID},
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return wrap("read exact outcome receipt", err)
		}
		if row.ActorID != actor {
			return access.ErrNotFound
		}
		v, err := load(ctx, q, id, row.Version)
		if err != nil {
			return err
		}
		out, err = project(ctx, q, actor, head, v, s.now())
		return err
	})
	return out, wrap("outcome receipt", err)
}

func (s Service) PublicationReceipt(
	ctx context.Context,
	actor, id, commandID uuid.UUID,
) (PublicationView, error) {
	var out PublicationView
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		if err := active(ctx, q, actor); err != nil {
			return err
		}
		head, err := q.OutcomeHead(ctx, id)
		if err != nil {
			return wrap("read publication receipt resource", err)
		}
		if _, err = requirePublicationSource(ctx, q, actor, head, s.now()); err != nil {
			return err
		}
		ph, err := q.OutcomeProjectHead(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return wrap("read publication receipt identity", err)
		}
		row, err := q.OutcomeProjectCommand(
			ctx,
			dbgen.OutcomeProjectCommandParams{OutcomeID: id, CommandID: commandID},
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return wrap("read exact publication receipt", err)
		}
		if row.ActorID != actor {
			return access.ErrNotFound
		}
		v, err := publicationVersion(row)
		if err != nil {
			return err
		}
		out = PublicationView{PublicationVersion: v, CurrentVersion: ph.CurrentVersion}
		if ph.PublishedVersion.Valid {
			out.PublishedVersion = &ph.PublishedVersion.Int64
		}
		return nil
	})
	return out, wrap("publication receipt", err)
}

func (s Service) ReviewReceipt(
	ctx context.Context,
	actor, id, commandID uuid.UUID,
) (PlanView, error) {
	var out PlanView
	err := database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		if err := active(ctx, q, actor); err != nil {
			return err
		}
		rh, err := q.OutcomeReviewHead(ctx, id)
		if err != nil {
			return wrap("read review receipt identity", err)
		}
		head, err := q.OutcomeHead(ctx, rh.OutcomeID)
		if err != nil {
			return wrap("read review receipt resource", err)
		}
		cap, err := capabilities(ctx, q, actor, head, s.now())
		if err != nil {
			return err
		}
		if !cap.CanModerate && (head.OwnerID == nil || *head.OwnerID != actor) {
			return access.ErrNotFound
		}
		row, err := q.OutcomeReviewCommand(
			ctx,
			dbgen.OutcomeReviewCommandParams{ReviewID: id, CommandID: commandID},
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return access.ErrNotFound
		}
		if err != nil {
			return wrap("read exact review receipt", err)
		}
		if row.ActorID != actor {
			return access.ErrNotFound
		}
		v, err := reviewVersion(row)
		if err != nil {
			return err
		}
		event, err := q.Event(ctx, rh.EventID)
		if err != nil {
			return wrap("read review receipt event", err)
		}
		out, err = s.planView(ctx, tx, q, event, rh, v)
		return err
	})
	return out, wrap("review receipt", err)
}
