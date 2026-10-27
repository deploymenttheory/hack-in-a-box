package submissions

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/hack-in-a-box/internal/database"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
)

// Settle runs in the caller's event-locked transaction, including immediately
// before assessment transitions. The logical freeze instant is the cutoff even
// when maintenance observes it later. No external repository content is inferred.
func (s Service) Settle(ctx context.Context, tx pgx.Tx, row dbgen.Event, now time.Time) error {
	locked, err := dbgen.New(tx).LockEvent(ctx, row.ID)
	if err != nil {
		return fmt.Errorf("lock final outcomes: %w", err)
	}
	return s.settleLocked(ctx, tx, locked, now)
}

// Only callers that already hold this exact event row lock may use this seam.
func (s Service) settleLocked(
	ctx context.Context,
	tx pgx.Tx,
	locked dbgen.Event,
	now time.Time,
) error {
	q := dbgen.New(tx)
	// Archived competition facts are immutable, including absent outcomes for
	// never-held events. A late maintenance/read must not invent a result.
	if locked.Stage == "archived" {
		return nil
	}
	c, err := configuration(locked)
	if err != nil {
		return err
	}
	// Source owners can revalidate many cards in one event transaction. Read the
	// current entry/state census together rather than reinitialising and loading
	// every team's unchanged state on each nested validation. The event lock still
	// serialises withdrawals, new states, cutoff settlement and gate facts.
	entries, err := q.SubmissionSettlementCensus(ctx, locked.ID)
	if err != nil {
		return fmt.Errorf("read final outcome teams: %w", err)
	}
	due := !c.SubmissionCutoff.IsZero() && !now.Before(c.SubmissionCutoff)
	var settled int32
	for _, e := range entries {
		state := dbgen.SubmissionState{
			EventID:        locked.ID,
			TeamID:         e.TeamID,
			Revision:       e.StateRevision,
			ActiveRevision: e.ActiveRevision,
			Outcome:        e.Outcome,
			FrozenAt:       e.FrozenAt,
		}
		if e.StateMissing {
			if err = q.EnsureSubmissionState(
				ctx,
				dbgen.EnsureSubmissionStateParams{EventID: locked.ID, TeamID: e.TeamID},
			); err != nil {
				return fmt.Errorf("initialize final outcome: %w", err)
			}
			state, err = q.SubmissionState(
				ctx,
				dbgen.SubmissionStateParams{EventID: locked.ID, TeamID: e.TeamID},
			)
			if err != nil {
				return fmt.Errorf("read final outcome: %w", err)
			}
		}
		active := e.Active && e.TeamState != "disbanded"
		outcome := state.Outcome
		if !active {
			outcome = "withdrawn"
		} else if due && outcome == "open" {
			outcome = "not_submitted"
			if state.ActiveRevision.Valid {
				outcome = "frozen"
			}
		}
		freeze := due && !state.FrozenAt.Valid
		if outcome != state.Outcome || freeze {
			state.Revision++
			state.Outcome = outcome
			if due {
				state.FrozenAt = pgtype.Timestamptz{Time: c.SubmissionCutoff, Valid: true}
			}
			if err = saveState(ctx, q, state); err != nil {
				return err
			}
			if err = record(ctx, q, state, nil, outcome, "", "", now); err != nil {
				return err
			}
		}
		if active && due && state.Outcome != "open" {
			settled++
		}
	}
	if due &&
		(len(entries) == 0 || !entries[0].SubmissionsReady || entries[0].SettledSubmissions != settled) {
		if err = event.RecordSubmissionFacts(ctx, tx, locked.ID, settled); err != nil {
			return fmt.Errorf("supply assessment outcomes: %w", err)
		}
	}
	return nil
}

func (s Service) Maintain(ctx context.Context) error {
	rows, err := dbgen.New(s.Pool).SubmissionMaintenanceEvents(ctx, s.now())
	if err != nil {
		return fmt.Errorf("find due submission cutoffs: %w", err)
	}
	for _, id := range rows {
		err = database.Within(ctx, s.Pool, func(tx pgx.Tx, q *dbgen.Queries) error {
			row, err := q.LockEvent(ctx, id)
			if err != nil {
				return fmt.Errorf("read due submission event: %w", err)
			}
			return s.Settle(ctx, tx, row, s.now())
		})
		if err != nil {
			return fmt.Errorf("settle due submissions: %w", err)
		}
	}
	return nil
}
