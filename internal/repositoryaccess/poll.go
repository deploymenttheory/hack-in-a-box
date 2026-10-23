package repositoryaccess

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/jobs"
)

const PollKind = "repository.access.poll"

type PollHandler struct{ Service *Service }

func (s Service) MemberChangedInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	eventID, teamID uuid.UUID,
	repositoryID, accountID int64,
	action string,
	receivedAt time.Time,
) error {
	if repositoryID <= 0 || accountID <= 0 || receivedAt.IsZero() ||
		(action != "added" && action != "removed" && action != "edited") {
		return access.ErrInvalid
	}
	q := dbgen.New(tx)
	e, err := q.LockEvent(ctx, eventID)
	if err != nil {
		return fmt.Errorf("lock member reconciliation hint: %w", err)
	}
	r, err := repository(ctx, q, eventID, teamID)
	if err != nil {
		return err
	}
	if r.GithubRepositoryID.Int64 != repositoryID {
		return access.ErrConflict
	}
	if !e.WritesAllowed || e.OwnerID == nil {
		return nil
	}
	if err = authorize(ctx, q, *e.OwnerID, e.ID, false); err != nil {
		return nil
	}
	// receivedAt identifies a queued observation, never the order of provider
	// membership changes. Only the later current provider GET supplies state.
	_, err = q.InsertOperation(
		ctx,
		dbgen.InsertOperationParams{
			EventID:  eventID,
			TargetID: teamID,
			Kind:     PollKind,
			OperationKey: fmt.Sprintf(
				"repository-member-poll:%s:%d:%s",
				teamID,
				accountID,
				receivedAt.UTC().Format(time.RFC3339Nano),
			),
			DueAt:    receivedAt.UTC(),
			Approved: true,
		},
	)
	if err != nil {
		return fmt.Errorf("queue current repository access observation: %w", err)
	}
	return nil
}

func (h PollHandler) Eligible(ctx context.Context, d jobs.Delivery) (bool, error) {
	allowed := false
	err := database.Within(ctx, h.Service.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		e, err := q.Event(ctx, d.EventID)
		if err != nil {
			return fmt.Errorf("read current poll event: %w", err)
		}
		if !e.WritesAllowed || e.OwnerID == nil {
			return nil
		}
		if authorize(ctx, q, *e.OwnerID, e.ID, false) != nil {
			return nil
		}
		_, err = repository(ctx, q, e.ID, d.TargetID)
		allowed = err == nil
		return nil
	})
	return allowed, wrap("check repository polling eligibility", err)
}

func (h PollHandler) Deliver(ctx context.Context, d jobs.Delivery) jobs.Outcome {
	err := database.Within(ctx, h.Service.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		e, err := q.LockEvent(ctx, d.EventID)
		if err != nil {
			return fmt.Errorf("lock repository access poll: %w", err)
		}
		if !e.WritesAllowed || e.OwnerID == nil {
			return access.ErrForbidden
		}
		if err = authorize(ctx, q, *e.OwnerID, e.ID, false); err != nil {
			return err
		}
		r, err := repository(ctx, q, e.ID, d.TargetID)
		if err != nil {
			return err
		}
		return h.Service.refresh(ctx, tx, q, e, r)
	})
	if err != nil {
		return jobs.Retryable
	}
	return jobs.Delivered
}

func (h PollHandler) Reconcile(ctx context.Context, d jobs.Delivery) jobs.Outcome {
	return h.Deliver(ctx, d)
}
