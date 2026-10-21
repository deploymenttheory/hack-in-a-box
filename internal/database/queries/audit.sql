-- name: AppendAudit :one
INSERT INTO audit_events(actor_id, event_id, action, target_id, reason_code, metadata)
VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: ListAudit :many
SELECT * FROM audit_events WHERE event_id = $1 AND sequence > $2
ORDER BY sequence LIMIT $3;
