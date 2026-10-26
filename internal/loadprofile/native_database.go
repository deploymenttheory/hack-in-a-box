package loadprofile

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errNativeOracle = errors.New("invalid native load oracle")

type NativeDatabase struct {
	pool   *pgxpool.Pool
	events map[uuid.UUID]bool
}

type InstallationCensus struct {
	Events            int64 `json:"events"`
	People            int64 `json:"people"`
	ActivePeople      int64 `json:"active_people"`
	UnexpiredSessions int64 `json:"unexpired_sessions"`
}

// Installation reads global counts only in the explicitly isolated load123
// database. A scoped five-event query cannot hide an old sixth event or a
// second dormant setup-session cohort elsewhere in the same installation.
func (n NativeDatabase) Installation(ctx context.Context) (InstallationCensus, error) {
	var census InstallationCensus
	err := n.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM events),
 (SELECT count(*) FROM people), (SELECT count(*) FROM people WHERE active),
 (SELECT count(*) FROM sessions WHERE expiry>clock_timestamp())`).Scan(
		&census.Events, &census.People, &census.ActivePeople, &census.UnexpiredSessions)
	if err != nil {
		return census, fmt.Errorf("read isolated native installation census: %w", err)
	}
	return census, nil
}

func NewNativeDatabase(pool *pgxpool.Pool, eventIDs []uuid.UUID) (NativeDatabase, error) {
	if pool == nil {
		return NativeDatabase{}, fmt.Errorf(
			"%w: read-only fixture database required",
			errNativeOracle,
		)
	}
	if err := validateScope(eventIDs); err != nil {
		return NativeDatabase{}, err
	}
	events := make(map[uuid.UUID]bool, len(eventIDs))
	for _, event := range eventIDs {
		events[event] = true
	}
	return NativeDatabase{pool: pool, events: events}, nil
}

func (n NativeDatabase) scoped(eventIDs []uuid.UUID) error {
	if err := validateScope(eventIDs); err != nil {
		return err
	}
	for _, event := range eventIDs {
		if !n.events[event] {
			return fmt.Errorf("%w: event outside private fixture scope", errNativeOracle)
		}
	}
	return nil
}

type AdmissionFact struct {
	EventID                 uuid.UUID `json:"event_id"`
	ConfiguredCapacity      int       `json:"configured_capacity"`
	Applications            int       `json:"applications"`
	Confirmed               int       `json:"confirmed"`
	Waitlisted              int       `json:"waitlisted"`
	Offered                 int       `json:"offered"`
	ClaimedConfirmed        int       `json:"claimed_confirmed"`
	Staff                   int       `json:"staff"`
	People                  int       `json:"people"`
	DuplicateActive         int       `json:"duplicate_active"`
	ExcludedCompetitors     int       `json:"excluded_competitors"`
	LastConfirmedSequence   *int64    `json:"last_confirmed_sequence"`
	FirstWaitlistedSequence *int64    `json:"first_waitlisted_sequence"`
}

// Admission reads committed owner facts in a PostgreSQL read-only snapshot.
// Staff and confirmed people form a distinct union: role overlap cannot inflate
// the five-event/200-person dataset. No fixture facts are inserted by an oracle.
func (n NativeDatabase) Admission(
	ctx context.Context,
	eventIDs []uuid.UUID,
) ([]AdmissionFact, error) {
	if err := n.scoped(eventIDs); err != nil {
		return nil, err
	}
	tx, err := n.pool.BeginTx(
		ctx,
		pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly},
	)
	if err != nil {
		return nil, fmt.Errorf("begin read-only admission census: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	rows, err := tx.Query(ctx, `SELECT e.id,(e.configuration->>'capacity')::integer,
 (SELECT count(*) FROM registrations r WHERE r.event_id=e.id AND r.kind='attendee'),
 (SELECT count(*) FROM registrations r WHERE r.event_id=e.id AND r.kind='attendee' AND r.state='confirmed'),
 (SELECT count(*) FROM registrations r WHERE r.event_id=e.id AND r.kind='attendee' AND r.state='waitlisted'),
 (SELECT count(*) FROM registrations r WHERE r.event_id=e.id AND r.kind='attendee' AND r.state='offered'),
 (SELECT count(*) FROM competition_claims c WHERE c.event_id=e.id AND c.state='confirmed'),
 (SELECT count(DISTINCT m.person_id) FROM event_memberships m JOIN people p ON p.id=m.person_id AND p.active WHERE m.event_id=e.id AND m.active AND NOT EXISTS(SELECT 1 FROM competition_claims c WHERE c.event_id=e.id AND c.person_id=m.person_id AND c.state='confirmed')),
 (SELECT count(*) FROM (SELECT c.person_id FROM competition_claims c WHERE c.event_id=e.id AND c.state='confirmed' UNION SELECT m.person_id FROM event_memberships m JOIN people p ON p.id=m.person_id AND p.active WHERE m.event_id=e.id AND m.active) people),
 (SELECT count(*) FROM (SELECT r.person_id,r.kind FROM registrations r WHERE r.event_id=e.id AND r.state NOT IN ('withdrawn','declined') GROUP BY r.person_id,r.kind HAVING count(*)>1) duplicates),
 (SELECT count(*) FROM competition_claims c JOIN competition_exclusions x ON x.event_id=c.event_id AND x.person_id=c.person_id WHERE c.event_id=e.id AND c.state='confirmed'),
 (SELECT max(sequence) FROM registrations r WHERE r.event_id=e.id AND r.kind='attendee' AND r.state='confirmed'),
 (SELECT min(sequence) FROM registrations r WHERE r.event_id=e.id AND r.kind='attendee' AND r.state='waitlisted')
 FROM events e WHERE e.id=ANY($1::uuid[]) ORDER BY e.id`, eventIDs)
	if err != nil {
		return nil, fmt.Errorf("read native admission census: %w", err)
	}
	defer rows.Close()
	facts := make([]AdmissionFact, 0, len(eventIDs))
	for rows.Next() {
		var fact AdmissionFact
		if err := rows.Scan(
			&fact.EventID,
			&fact.ConfiguredCapacity,
			&fact.Applications,
			&fact.Confirmed,
			&fact.Waitlisted,
			&fact.Offered,
			&fact.ClaimedConfirmed,
			&fact.Staff,
			&fact.People,
			&fact.DuplicateActive,
			&fact.ExcludedCompetitors,
			&fact.LastConfirmedSequence,
			&fact.FirstWaitlistedSequence,
		); err != nil {
			return nil, fmt.Errorf("decode native admission census: %w", err)
		}
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("finish native admission census: %w", err)
	}
	if len(facts) != len(eventIDs) {
		return nil, fmt.Errorf("%w: not every target event exists", errNativeOracle)
	}
	return facts, nil
}

type QueueSample struct {
	PendingByKind        map[string]int64 `json:"pending_by_kind"`
	At                   time.Time        `json:"database_at"`
	Pending              int64            `json:"pending"`
	Running              int64            `json:"running"`
	Delivered            int64            `json:"native_delivered"`
	Permanent            int64            `json:"permanent"`
	Ambiguous            int64            `json:"ambiguous"`
	OverdueApproved      int64            `json:"overdue_approved"`
	MaximumDueLagSeconds float64          `json:"raw_all_operation_maximum_due_lag_seconds"`
}

func (n NativeDatabase) Queue(ctx context.Context, eventIDs []uuid.UUID) (QueueSample, error) {
	var sample QueueSample
	if err := n.scoped(eventIDs); err != nil {
		return sample, err
	}
	err := n.pool.QueryRow(ctx, `SELECT clock_timestamp(),count(*) FILTER(WHERE state='pending'),count(*) FILTER(WHERE state='running'),count(*) FILTER(WHERE state='delivered'),count(*) FILTER(WHERE state='permanent'),count(*) FILTER(WHERE state='ambiguous'),count(*) FILTER(WHERE state='pending' AND approved AND NOT cancelled AND due_at<now()),COALESCE(max(EXTRACT(epoch FROM now()-due_at)) FILTER(WHERE state='pending' AND approved AND NOT cancelled AND due_at<now()),0)::double precision,COALESCE((SELECT jsonb_object_agg(kind,total) FROM (SELECT kind,count(*) AS total FROM operations WHERE event_id=ANY($1::uuid[]) AND state='pending' GROUP BY kind) counts),'{}'::jsonb) FROM operations WHERE event_id=ANY($1::uuid[])`, eventIDs).
		Scan(&sample.At, &sample.Pending, &sample.Running, &sample.Delivered, &sample.Permanent, &sample.Ambiguous, &sample.OverdueApproved, &sample.MaximumDueLagSeconds, &sample.PendingByKind)
	if err != nil {
		return sample, fmt.Errorf("read native load queue pressure: %w", err)
	}
	return sample, nil
}

type JobStartFact struct {
	ID, EventID         uuid.UUID
	Kind, Key, State    string
	Due                 time.Time
	FirstStarted        *time.Time
	Attempts            int64
	Approved, Cancelled bool
}

// DatabaseTime supplies the native timestamp boundary for due-work accounting.
// It does not alter the clock or release any work.
func (n NativeDatabase) DatabaseTime(ctx context.Context) (time.Time, error) {
	var at time.Time
	if err := n.pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); err != nil {
		return at, fmt.Errorf("read native database measurement boundary: %w", err)
	}
	return at, nil
}

// JobStarts keeps missing/unstarted due operations in the result. The caller
// must reconcile its expected healthy native operation IDs and cannot compute
// a success-only due-start percentile by dropping pending work.
func (n NativeDatabase) JobStarts(
	ctx context.Context,
	operationIDs []uuid.UUID,
) ([]JobStartFact, error) {
	if err := validateIDs(operationIDs, 100_000); err != nil {
		return nil, err
	}
	rows, err := n.pool.Query(
		ctx,
		`SELECT o.id,o.event_id,o.kind,o.operation_key,o.state,o.due_at,min(a.started_at),count(a.id),o.approved,o.cancelled FROM operations o LEFT JOIN delivery_attempts a ON a.operation_id=o.id WHERE o.id=ANY($1::uuid[]) GROUP BY o.id ORDER BY o.id`,
		operationIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("read native guarded attempt starts: %w", err)
	}
	defer rows.Close()
	facts := make([]JobStartFact, 0, len(operationIDs))
	for rows.Next() {
		var fact JobStartFact
		if err := rows.Scan(
			&fact.ID,
			&fact.EventID,
			&fact.Kind,
			&fact.Key,
			&fact.State,
			&fact.Due,
			&fact.FirstStarted,
			&fact.Attempts,
			&fact.Approved,
			&fact.Cancelled,
		); err != nil {
			return nil, fmt.Errorf("decode native guarded attempt start: %w", err)
		}
		if !n.events[fact.EventID] {
			return nil, fmt.Errorf("%w: operation outside scoped fixture events", errNativeOracle)
		}
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("finish native guarded attempt starts: %w", err)
	}
	if len(facts) != len(operationIDs) {
		return nil, fmt.Errorf("%w: expected healthy native operation missing", errNativeOracle)
	}
	return facts, nil
}

type CueAnchor struct {
	AuditAt, DatabaseAt, DriverBefore, DriverAfter time.Time
	Phase                                          Phase
}

// CuePublication anchors the acknowledged presentation revision to its own
// immutable native audit row. Simultaneous database-clock sampling bounds
// driver/server clock offset, rather than assuming host clocks are identical.
func (n NativeDatabase) CuePublication(
	ctx context.Context,
	event, actor uuid.UUID,
	sequence int64,
	phase Phase,
) (CueAnchor, error) {
	var anchor CueAnchor
	anchor.Phase = phase
	if !phase.valid() {
		return anchor, fmt.Errorf("%w: actual cue workload phase required", errNativeOracle)
	}
	if err := n.scoped([]uuid.UUID{event}); err != nil {
		return anchor, err
	}
	if event == uuid.Nil || actor == uuid.Nil || sequence < 1 {
		return anchor, fmt.Errorf("%w: exact cue actor/event/revision required", errNativeOracle)
	}
	anchor.DriverBefore = time.Now()
	err := n.pool.QueryRow(ctx, `SELECT occurred_at,clock_timestamp() FROM audit_events WHERE event_id=$1 AND actor_id=$2 AND target_id=$1 AND action='presentation.changed' AND metadata->>'revision'=$3`, event, actor, fmt.Sprint(sequence)).
		Scan(&anchor.AuditAt, &anchor.DatabaseAt)
	anchor.DriverAfter = time.Now()
	if err != nil {
		return anchor, fmt.Errorf("read original native cue audit anchor: %w", err)
	}
	return anchor, nil
}

func validateScope(ids []uuid.UUID) error { return validateIDs(ids, 10) }
func validateIDs(ids []uuid.UUID, maximum int) error {
	if len(ids) == 0 || len(ids) > maximum {
		return fmt.Errorf("%w: bounded explicit native scope required", errNativeOracle)
	}
	seen := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		if id == uuid.Nil || seen[id] {
			return fmt.Errorf("%w: zero/duplicate native scope", errNativeOracle)
		}
		seen[id] = true
	}
	return nil
}
