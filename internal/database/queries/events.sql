-- name: CreateEvent :one
INSERT INTO events(id,name,slug,configuration) VALUES($1,$2,$3,$4) RETURNING *;

-- name: InitializeEventFacts :exec
INSERT INTO event_gate_facts(event_id) VALUES($1) ON CONFLICT DO NOTHING;

-- name: EventFacts :one
SELECT * FROM event_gate_facts WHERE event_id=$1;

-- name: ConfirmedAttendees :one
SELECT count(*) FROM competition_claims WHERE event_id=$1 AND state='confirmed';

-- name: ListVisibleEvents :many
SELECT e.* FROM events e WHERE e.id>sqlc.arg(cursor)::uuid AND (
 EXISTS(SELECT 1 FROM people p WHERE p.id=sqlc.arg(actor)::uuid AND p.active AND p.platform_admin)
 OR EXISTS(SELECT 1 FROM event_memberships m WHERE m.event_id=e.id AND m.person_id=sqlc.arg(actor)::uuid AND m.active)
 OR EXISTS(SELECT 1 FROM competition_claims c WHERE c.event_id=e.id AND c.person_id=sqlc.arg(actor)::uuid AND c.state='confirmed'))
ORDER BY e.id LIMIT sqlc.arg(page_size);

-- name: SaveEventConfiguration :one
UPDATE events SET name=$2,slug=$3,configuration=$4,revision=revision+1,updated_at=now()
WHERE id=$1 RETURNING *;

-- name: SetEventStage :one
UPDATE events SET stage=$2,writes_allowed=$3,completed_at=CASE WHEN $2='completed' THEN now() WHEN $2='assessment' THEN NULL ELSE completed_at END,
revision=revision+1,updated_at=now() WHERE id=$1 RETURNING *;

-- name: RecordEventDecision :one
INSERT INTO event_decisions(event_id,actor_id,kind,reason,revision) VALUES($1,$2,$3,$4,$5) RETURNING *;

-- name: ListEventDecisions :many
SELECT * FROM event_decisions WHERE event_id=$1 ORDER BY created_at DESC,id LIMIT 100;

-- name: CancelEventOperations :exec
UPDATE operations SET cancelled=true,approved=false,revision=revision+1,updated_at=now()
WHERE event_id=$1 AND state IN ('pending','running');

-- name: ScheduleItems :many
SELECT * FROM schedule_items WHERE event_id=$1 ORDER BY due_at,id;

-- name: UpsertScheduleItem :exec
INSERT INTO schedule_items(event_id,id,definition,due_at) VALUES($1,$2,$3,$4)
ON CONFLICT(event_id,id) DO UPDATE SET definition=EXCLUDED.definition,due_at=EXCLUDED.due_at,
approved=false,cancelled=false,revision=schedule_items.revision+1;

-- name: CancelScheduleOperation :exec
UPDATE operations SET cancelled=true,approved=false,revision=revision+1,updated_at=now()
WHERE id IN (SELECT s.operation_id FROM schedule_items s WHERE s.event_id=$1 AND s.id=$2)
AND state IN ('pending','running');

-- name: CancelScheduleItem :exec
UPDATE schedule_items SET cancelled=true,approved=false,revision=revision+1 WHERE event_id=$1 AND id=$2;

-- name: ApproveScheduleItem :execrows
UPDATE schedule_items SET approved=true,revision=revision+1 WHERE event_id=$1 AND id=$2 AND NOT approved AND NOT cancelled AND executed_at IS NULL AND due_at>$3;

-- name: CancelEventSchedule :exec
UPDATE schedule_items SET approved=false,cancelled=true,revision=revision+1 WHERE event_id=$1 AND executed_at IS NULL;
