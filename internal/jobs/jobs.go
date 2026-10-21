// Package jobs combines River execution with application-owned delivery history.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
)

type Outcome string

const (
	Delivered Outcome = "delivered"
	Retryable Outcome = "retryable"
	Permanent Outcome = "permanent"
	Ambiguous Outcome = "ambiguous"
)

var (
	ErrRetry       = errors.New("delivery can be retried")
	ErrIdentity    = errors.New("operation key already identifies a different operation")
	ErrEligibility = errors.New("current delivery eligibility could not be checked")
)

// Delivery contains references only. Integrations resolve credentials privately.
// The durable operation ID is the provider idempotency key across all attempts.
type Delivery struct {
	ID, EventID, TargetID uuid.UUID
	Key                   string
}
type Handler interface {
	Eligible(context.Context, Delivery) (bool, error)
	Deliver(context.Context, Delivery) Outcome
	// Reconcile must prove that retry is safe before returning Retryable. Otherwise
	// return Ambiguous: a process can crash after a successful external send.
	Reconcile(context.Context, Delivery) Outcome
}
type Args struct {
	OperationID uuid.UUID `json:"operation_id"`
}

func (Args) Kind() string { return "deliver_operation" }
func (Args) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 10, UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

type Service struct {
	river.WorkerDefaults[Args]
	Pool   *pgxpool.Pool
	Client *river.Client[pgx.Tx]
	// Maintenance performs local cleanup alongside durable schedule reconciliation.
	Maintenance func(context.Context) error
	handlers    map[string]Handler
}

func New(pool *pgxpool.Pool, logger *slog.Logger, handlers map[string]Handler) (*Service, error) {
	service := &Service{Pool: pool, handlers: handlers}
	workers := river.NewWorkers()
	river.AddWorker(workers, service)
	client, err := river.NewClient(
		riverpgxv5.New(pool),
		&river.Config{
			Logger:               logger,
			Workers:              workers,
			Queues:               map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 4}},
			JobTimeout:           45 * time.Second,
			RescueStuckJobsAfter: 2 * time.Minute,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create job client: %w", err)
	}
	service.Client = client
	return service, nil
}

// Enqueue must be called in the domain transaction, after its own policy checks.
func (s *Service) Enqueue(
	ctx context.Context,
	tx pgx.Tx,
	input dbgen.InsertOperationParams,
) (dbgen.Operation, error) {
	if input.EventID == uuid.Nil || input.TargetID == uuid.Nil || input.Kind == "" ||
		input.OperationKey == "" ||
		input.DueAt.IsZero() {
		return dbgen.Operation{}, access.ErrInvalid
	}
	input.DueAt = input.DueAt.UTC().Truncate(time.Microsecond)
	event, err := dbgen.New(tx).LockEvent(ctx, input.EventID)
	if err != nil {
		return dbgen.Operation{}, fmt.Errorf("check operation event: %w", err)
	}
	if !event.WritesAllowed {
		return dbgen.Operation{}, access.ErrForbidden
	}
	op, err := dbgen.New(tx).InsertOperation(ctx, input)
	if err != nil {
		return dbgen.Operation{}, fmt.Errorf("persist operation: %w", err)
	}
	if op.Kind != input.Kind || op.TargetID != input.TargetID || !op.DueAt.Equal(input.DueAt) {
		return dbgen.Operation{}, ErrIdentity
	}
	if op.State == "pending" && !op.Cancelled {
		if _, err := s.Client.InsertTx(
			ctx,
			tx,
			Args{op.ID},
			&river.InsertOpts{ScheduledAt: op.DueAt},
		); err != nil {
			return dbgen.Operation{}, fmt.Errorf("enqueue operation: %w", err)
		}
	}
	return op, nil
}

func (s *Service) Work(ctx context.Context, job *river.Job[Args]) error {
	return s.Execute(ctx, job.Args.OperationID)
}

func (s *Service) Execute(ctx context.Context, id uuid.UUID) error {
	// A connection-scoped guard prevents a second live process from recovering an
	// operation while its original handler is still running, even after lease expiry.
	conn, err := s.Pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire delivery connection: %w", err)
	}
	defer conn.Release()
	var locked bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,0))", id.String()).
		Scan(&locked); err != nil {
		return fmt.Errorf("lock delivery: %w", err)
	}
	if !locked {
		return fmt.Errorf("delivery already owned: %w", river.JobSnooze(5*time.Second))
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(
			cleanup,
			"SELECT pg_advisory_unlock(hashtextextended($1,0))",
			id.String(),
		); err != nil {
			_ = conn.Conn().Close(cleanup)
		}
	}()
	return s.executeLocked(ctx, id)
}

func (s *Service) executeLocked(ctx context.Context, id uuid.UUID) error {
	var op dbgen.Operation
	token := uuid.New()
	claimed := false
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		var err error
		op, err = q.LockOperation(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("lock operation: %w", err)
		}
		if op.State != "pending" || !op.Approved || op.Cancelled || op.DueAt.After(time.Now()) {
			return nil
		}
		event, err := q.Event(ctx, op.EventID)
		if err != nil {
			return fmt.Errorf("recheck event operation eligibility: %w", err)
		}
		if !event.WritesAllowed {
			return nil
		}
		handler := s.handlers[op.Kind]
		if handler == nil {
			return nil
		} // New integration kinds remain pending until their handler is deployed.
		eligible, err := handler.Eligible(ctx, delivery(op))
		if err != nil {
			return ErrEligibility
		}
		if !eligible {
			return nil
		}
		op, err = q.ClaimOperation(
			ctx,
			dbgen.ClaimOperationParams{
				ID:         id,
				LeaseToken: &token,
				LeaseUntil: pgtype.Timestamptz{Time: time.Now().Add(2 * time.Minute), Valid: true},
			},
		)
		if err != nil {
			return fmt.Errorf("claim operation: %w", err)
		}
		claimed = true
		return q.BeginAttempt(ctx, dbgen.BeginAttemptParams{OperationID: id, LeaseToken: token})
	})
	if err != nil {
		return fmt.Errorf("claim delivery: %w", err)
	}
	if !claimed {
		return nil
	}
	effectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	outcome := s.handlers[op.Kind].Deliver(effectCtx, delivery(op))
	cancel()
	// Unknown provider outcomes are always ambiguous, never implicitly retryable.
	outcome = normalize(outcome)
	cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer stop()
	if err := s.finish(cleanup, op.ID, token, outcome); err != nil {
		return err
	}
	if outcome == Retryable {
		return ErrRetry
	}
	return nil
}

func delivery(op dbgen.Operation) Delivery {
	return Delivery{ID: op.ID, EventID: op.EventID, TargetID: op.TargetID, Key: op.ID.String()}
}

func normalize(outcome Outcome) Outcome {
	switch outcome {
	case Delivered, Retryable, Permanent, Ambiguous:
		return outcome
	default:
		return Ambiguous
	}
}

func (s *Service) finish(ctx context.Context, id, token uuid.UUID, outcome Outcome) error {
	state := string(outcome)
	if outcome == Retryable {
		state = "pending"
	}
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		changed, err := q.FinishOperation(
			ctx,
			dbgen.FinishOperationParams{ID: id, LeaseToken: &token, State: state},
		)
		if err != nil {
			return fmt.Errorf("finish operation: %w", err)
		}
		if changed == 0 {
			return nil
		}
		return q.FinishAttempt(
			ctx,
			dbgen.FinishAttemptParams{LeaseToken: token, Outcome: string(outcome)},
		)
	})
	if err != nil {
		return fmt.Errorf("finish delivery: %w", err)
	}
	return nil
}

// Reconcile reconstructs execution from durable business records, independent of
// River retention. Ambiguous operations require explicit provider reconciliation.
func (s *Service) Reconcile(ctx context.Context) error {
	var expired []dbgen.Operation
	err := database.Within(ctx, s.Pool, func(_ pgx.Tx, q *dbgen.Queries) error {
		var err error
		expired, err = q.ExpiredOperations(ctx)
		if err != nil {
			return fmt.Errorf("find expired operations: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("find expired leases: %w", err)
	}
	for _, op := range expired {
		if err := s.recover(ctx, op); err != nil {
			return err
		}
	}
	err = database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
		pending, err := q.PendingOperations(ctx)
		if err != nil {
			return fmt.Errorf("pending operations: %w", err)
		}
		enqueued := int32(0)
		for _, op := range pending {
			if s.handlers[op.Kind] == nil {
				continue
			}
			if _, err := s.Client.InsertTx(
				ctx,
				tx,
				Args{op.ID},
				&river.InsertOpts{ScheduledAt: op.DueAt},
			); err != nil {
				return fmt.Errorf("insert tx: %w", err)
			}
			enqueued++
		}
		if err := q.ExpireSessions(ctx); err != nil {
			return fmt.Errorf("expire sessions: %w", err)
		}
		if err := q.ExpireLoginAttempts(ctx); err != nil {
			return fmt.Errorf("expire login attempts: %w", err)
		}
		return q.RecordReconciliation(
			ctx,
			dbgen.RecordReconciliationParams{Enqueued: enqueued},
		)
	})
	if err != nil {
		return fmt.Errorf("reconcile operations: %w", err)
	}
	return nil
}

func (s *Service) recover(ctx context.Context, op dbgen.Operation) error {
	conn, err := s.Pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire recovery connection: %w", err)
	}
	defer conn.Release()
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin recovery guard: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	var locked bool
	if err := tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))", op.ID.String()).
		Scan(&locked); err != nil {
		return fmt.Errorf("lock recovery: %w", err)
	}
	if !locked {
		return nil
	}
	current, err := dbgen.New(tx).LockOperation(ctx, op.ID)
	if err != nil {
		return fmt.Errorf("reload expired operation: %w", err)
	}
	if current.State != "running" || current.LeaseToken == nil ||
		current.LeaseUntil.Time.After(time.Now()) {
		return nil
	}
	result := Ambiguous
	if handler := s.handlers[current.Kind]; handler != nil {
		checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		result = normalize(handler.Reconcile(checkCtx, delivery(current)))
		cancel()
	}
	state := string(result)
	if result == Retryable {
		state = "pending"
	}
	q := dbgen.New(tx)
	if _, err := q.FinishOperation(
		ctx,
		dbgen.FinishOperationParams{ID: current.ID, LeaseToken: current.LeaseToken, State: state},
	); err != nil {
		return fmt.Errorf("recover operation: %w", err)
	}
	if err := q.FinishAttempt(
		ctx,
		dbgen.FinishAttemptParams{LeaseToken: *current.LeaseToken, Outcome: string(result)},
	); err != nil {
		return fmt.Errorf("recover attempt: %w", err)
	}
	if err := q.RecordReconciliation(
		ctx,
		dbgen.RecordReconciliationParams{RecoveredLeases: 1},
	); err != nil {
		return fmt.Errorf("record lease recovery: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit recovery: %w", err)
	}
	return nil
}

func (s *Service) Run(ctx context.Context, shutdown time.Duration) error {
	if err := s.Reconcile(ctx); err != nil {
		return err
	}
	if err := s.Client.Start(ctx); err != nil {
		return fmt.Errorf("start workers: %w", err)
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			stop, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdown)
			defer cancel()
			if err := s.Client.Stop(stop); err != nil {
				force, cancelForce := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				defer cancelForce()
				_ = s.Client.StopAndCancel(force)
				return fmt.Errorf("stop workers: %w", err)
			}
			return nil
		case <-ticker.C:
			if err := s.Reconcile(ctx); err != nil {
				stop, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdown)
				defer cancel()
				_ = s.Client.StopAndCancel(stop)
				return err
			}
			if s.Maintenance != nil {
				if err := s.Maintenance(ctx); err != nil {
					stop, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdown)
					defer cancel()
					_ = s.Client.StopAndCancel(stop)
					return err
				}
			}
		}
	}
}
