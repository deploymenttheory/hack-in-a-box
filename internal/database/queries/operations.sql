-- name: InsertOperation :one
INSERT INTO operations(event_id, operation_key, kind, target_id, due_at, approved)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (event_id, operation_key) DO UPDATE SET operation_key = EXCLUDED.operation_key
RETURNING *;

-- name: Operation :one
SELECT * FROM operations WHERE id = $1;

-- name: LockOperation :one
SELECT * FROM operations WHERE id = $1 FOR UPDATE;

-- name: SetApproval :one
UPDATE operations SET approved = $2, revision = revision + 1, updated_at = now()
WHERE id = $1 AND revision = $3 AND state = 'pending' AND NOT cancelled RETURNING *;

-- name: CancelOperation :execrows
UPDATE operations SET cancelled = true, state = 'cancelled', revision = revision + 1, updated_at = now()
WHERE id = $1 AND state = 'pending';

-- name: ClaimOperation :one
UPDATE operations SET state = 'running', lease_token = $2, lease_until = $3,
attempts = attempts + 1, updated_at = now()
WHERE id = $1 AND state = 'pending' AND approved AND NOT cancelled AND due_at <= now() RETURNING *;

-- name: BeginAttempt :exec
INSERT INTO delivery_attempts(operation_id, lease_token, outcome) VALUES ($1,$2,'running');

-- name: FinishAttempt :exec
UPDATE delivery_attempts SET outcome = $2, finished_at = now() WHERE lease_token = $1 AND outcome = 'running';

-- name: FinishOperation :execrows
UPDATE operations SET state = $3, lease_until = NULL, lease_token = NULL, updated_at = now()
WHERE id = $1 AND lease_token = $2 AND state = 'running';

-- name: PendingOperations :many
SELECT * FROM operations WHERE state = 'pending' AND approved AND NOT cancelled ORDER BY due_at;

-- name: ExpiredOperations :many
SELECT * FROM operations WHERE state = 'running' AND lease_until < now() FOR UPDATE SKIP LOCKED;

-- name: RecordReconciliation :exec
INSERT INTO reconciliation_runs(recovered_leases,enqueued) VALUES ($1,$2);

-- name: JobMetrics :one
SELECT
 (SELECT count(*) FROM operations WHERE state IN ('permanent','ambiguous'))::bigint AS failed,
 (SELECT count(*) FROM operations WHERE state = 'ambiguous')::bigint AS ambiguous,
 COALESCE((SELECT max(EXTRACT(epoch FROM now() - due_at)) FROM operations WHERE state = 'pending' AND approved AND NOT cancelled AND due_at < now()),0)::double precision AS due_lag_seconds,
 COALESCE((SELECT sum(recovered_leases) FROM reconciliation_runs),0)::bigint AS recovered_leases;

-- name: RetainedCommunicationOccurrence :one
SELECT retained_communication_occurrence(sqlc.arg(event_id)::uuid,sqlc.arg(occurrence_id)::uuid,sqlc.arg(at_time)::timestamptz)::boolean AS allowed;

-- name: SetRetainedOccurrence :execrows
UPDATE operations SET retained_occurrence_id=$2 WHERE id=$1 AND (retained_occurrence_id IS NULL OR retained_occurrence_id=$2);
