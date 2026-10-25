package event_test

import (
	"errors"
	"testing"
	"time"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/database/dbgen"
	"github.com/deploymenttheory/hack-in-a-box/internal/event"
	"github.com/deploymenttheory/hack-in-a-box/internal/testdb"
)

// Completion starts retention; it must use the same domain clock as lifecycle
// gates and authorised archive readers, not a second database wall clock.
func TestLifecycleCompletionRetentionUsesTheNativeDomainClock(t *testing.T) {
	for _, stage := range []string{"completed", "cancelled"} {
		t.Run(stage, func(t *testing.T) {
			pool := testdb.New(t, true)
			actor := testdb.Person(t, pool)
			id := testdb.Event(t, pool, actor.ID)
			ctx := t.Context()
			_, err := pool.Exec(
				ctx,
				"INSERT INTO event_gate_facts(event_id)VALUES($1)ON CONFLICT DO NOTHING",
				id,
			)
			if err != nil {
				t.Fatal(err)
			}
			if stage == "completed" {
				if _, err = pool.Exec(
					ctx,
					"UPDATE events SET stage='assessment' WHERE id=$1",
					id,
				); err != nil {
					t.Fatal(err)
				}
			}
			now := time.Date(2030, 7, 1, 11, 0, 0, 0, time.UTC)
			owner := event.Service{Pool: pool, Clock: func() time.Time { return now }}
			q := dbgen.New(pool)
			current, err := q.Event(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = owner.Transition(
				ctx,
				actor.ID,
				id,
				current.Revision,
				stage,
				"Explicit native lifecycle decision under controlled domain clock",
				false,
			); err != nil {
				t.Fatal(err)
			}
			current, err = q.Event(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if !current.CompletedAt.Valid || !current.CompletedAt.Time.Equal(now) {
				t.Fatalf(
					"retention origin diverged from native domain clock: got %v want %v",
					current.CompletedAt,
					now,
				)
			}
			original := now
			now = now.Add(time.Hour)
			if _, retryErr := owner.Transition(
				ctx,
				actor.ID,
				id,
				current.Revision-1,
				stage,
				"Stale transition cannot restart retention",
				false,
			); !errors.Is(
				retryErr,
				access.ErrConflict,
			) {
				t.Fatal("stale transition", retryErr)
			}
			unchanged, readErr := q.Event(ctx, id)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if !unchanged.CompletedAt.Valid || !unchanged.CompletedAt.Time.Equal(original) {
				t.Fatal("stale transition changed retention origin", unchanged.CompletedAt)
			}
			if stage == "completed" {
				if _, err = owner.Transition(
					ctx,
					actor.ID,
					id,
					current.Revision,
					"assessment",
					"Explicit reviewed reopen",
					false,
				); err != nil {
					t.Fatal(err)
				}
				current, err = q.Event(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				if current.CompletedAt.Valid {
					t.Fatal("explicit reopen retained completion time")
				}
				now = now.Add(time.Hour)
				if _, err = owner.Transition(
					ctx,
					actor.ID,
					id,
					current.Revision,
					"completed",
					"Complete the reviewed reopened assessment",
					false,
				); err != nil {
					t.Fatal(err)
				}
				current, err = q.Event(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				if !current.CompletedAt.Valid || !current.CompletedAt.Time.Equal(now) {
					t.Fatal(
						"reopened completion did not use current trusted clock",
						current.CompletedAt,
					)
				}
				completed := now
				now = now.Add(time.Hour)
				if _, err = owner.Transition(
					ctx,
					actor.ID,
					id,
					current.Revision,
					"cancelled",
					"Recorded cancellation retains first completion",
					false,
				); err != nil {
					t.Fatal(err)
				}
				current, err = q.Event(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				if !current.CompletedAt.Valid || !current.CompletedAt.Time.Equal(completed) {
					t.Fatal("cancellation restarted existing retention", current.CompletedAt)
				}
				now = now.Add(24 * time.Hour)
				if _, err = owner.Transition(
					ctx,
					actor.ID,
					id,
					current.Revision,
					"archived",
					"Archive retained competition records",
					false,
				); err != nil {
					t.Fatal(err)
				}
				current, err = q.Event(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				if !current.CompletedAt.Valid || !current.CompletedAt.Time.Equal(completed) {
					t.Fatal(
						"archive changed the original completion retention clock",
						current.CompletedAt,
					)
				}
			}
		})
	}
}

func TestLifecycleDefaultCompletionUsesCurrentWallTime(t *testing.T) {
	for _, stage := range []string{"completed", "cancelled"} {
		t.Run(stage, func(t *testing.T) {
			pool := testdb.New(t, true)
			actor := testdb.Person(t, pool)
			id := testdb.Event(t, pool, actor.ID)
			ctx := t.Context()
			if _, err := pool.Exec(
				ctx,
				"INSERT INTO event_gate_facts(event_id)VALUES($1)ON CONFLICT DO NOTHING",
				id,
			); err != nil {
				t.Fatal(err)
			}
			if stage == "completed" {
				if _, err := pool.Exec(
					ctx,
					"UPDATE events SET stage='assessment' WHERE id=$1",
					id,
				); err != nil {
					t.Fatal(err)
				}
			}
			q := dbgen.New(pool)
			current, err := q.Event(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			before := time.Now().UTC().Add(-time.Second)
			if _, err = (event.Service{Pool: pool}).Transition(
				ctx,
				actor.ID,
				id,
				current.Revision,
				stage,
				"Default clock remains trusted server wall time",
				false,
			); err != nil {
				t.Fatal(err)
			}
			after := time.Now().UTC().Add(time.Second)
			current, err = q.Event(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if !current.CompletedAt.Valid || current.CompletedAt.Time.Before(before) ||
				current.CompletedAt.Time.After(after) {
				t.Fatal(
					"default clock no longer uses current server time",
					current.CompletedAt,
					before,
					after,
				)
			}
		})
	}
}
