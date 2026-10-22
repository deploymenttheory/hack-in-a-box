-- name: InboxCandidates :many
SELECT n.*,e.name AS event_name FROM inbox_notifications n JOIN events e ON e.id=n.event_id
WHERE n.recipient_id=$1 AND (sqlc.narg(event_filter)::uuid IS NULL OR n.event_id=sqlc.narg(event_filter)::uuid)
ORDER BY n.id DESC LIMIT 10001;

-- name: InboxNotification :one
SELECT n.*,e.name AS event_name FROM inbox_notifications n JOIN events e ON e.id=n.event_id
WHERE n.id=$1 AND n.recipient_id=$2;

-- name: InsertInboxNotification :one
INSERT INTO inbox_notifications(id,event_id,recipient_id,source_kind,source_id,source_version,provenance,subject,body,format,occurrence_id,scope_binding)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT(event_id,recipient_id,source_kind,source_id,source_version,occurrence_id) DO UPDATE SET source_id=EXCLUDED.source_id
RETURNING *;

-- name: MarkInboxRead :one
UPDATE inbox_notifications SET read_at=CASE WHEN sqlc.arg(is_read)::bool THEN COALESCE(read_at,now()) ELSE NULL END
WHERE id=$1 AND recipient_id=$2 RETURNING read_at;

-- name: InboxCommunicationVersion :one
SELECT * FROM communication_versions WHERE event_id=$1 AND id=$2 AND kind=$3 AND version=$4;
