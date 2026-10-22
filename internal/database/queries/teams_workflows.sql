-- name: TeamsWorkflowConnection :one
SELECT * FROM teams_workflow_connections WHERE event_id=$1;

-- name: LockTeamsWorkflowConnection :one
SELECT * FROM teams_workflow_connections WHERE event_id=$1 FOR UPDATE;

-- name: SaveTeamsWorkflowConnection :exec
INSERT INTO teams_workflow_connections(id,event_id,version,label,owner_label,co_owner_label,key_id,ciphertext,active,updated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT(event_id) DO UPDATE SET version=EXCLUDED.version,label=EXCLUDED.label,owner_label=EXCLUDED.owner_label,co_owner_label=EXCLUDED.co_owner_label,key_id=EXCLUDED.key_id,ciphertext=EXCLUDED.ciphertext,active=EXCLUDED.active,retry_after=NULL,updated_at=EXCLUDED.updated_at;

-- name: SetTeamsWorkflowRateWindow :exec
UPDATE teams_workflow_connections SET retry_after=$2 WHERE event_id=$1;

-- name: TeamsWorkflowDelivery :one
SELECT * FROM teams_workflow_deliveries WHERE event_id=$1 AND id=$2;

-- name: TeamsWorkflowRequest :one
SELECT * FROM teams_workflow_deliveries WHERE event_id=$1 AND request_id=$2;

-- name: TeamsWorkflowCampaignDelivery :one
SELECT * FROM teams_workflow_deliveries WHERE event_id=$1 AND campaign_id=$2 AND campaign_version=$3 AND occurrence_id IS NULL;

-- name: InsertTeamsWorkflowDelivery :exec
INSERT INTO teams_workflow_deliveries(event_id,id,operation_id,kind,connection_id,connection_version,channel_revision,actor_id,campaign_id,campaign_version,preview_id,preview_fingerprint,request_id,created_at,occurrence_id,scope_binding,scope_approval)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17);

-- name: TeamsWorkflowLatestAttempt :one
SELECT a.*,r.outcome,r.http_status,r.reason,r.retry_at,r.recorded_at FROM teams_workflow_attempts a
LEFT JOIN teams_workflow_receipts r ON r.event_id=a.event_id AND r.attempt_id=a.id
WHERE a.event_id=$1 AND a.delivery_id=$2 ORDER BY a.attempt_number DESC LIMIT 1;

-- name: InsertTeamsWorkflowAttempt :exec
INSERT INTO teams_workflow_attempts(event_id,id,delivery_id,attempt_number,connection_version,payload_fingerprint,started_at) VALUES($1,$2,$3,$4,$5,$6,$7);

-- name: InsertTeamsWorkflowReceipt :exec
INSERT INTO teams_workflow_receipts(event_id,attempt_id,outcome,http_status,reason,retry_at,recorded_at) VALUES($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT(event_id,attempt_id) DO NOTHING;

-- name: TeamsWorkflowHistory :many
SELECT d.id,d.kind,d.connection_version,d.campaign_id,d.campaign_version,d.created_at,o.state,
 r.outcome,r.http_status,r.reason,r.retry_at,r.recorded_at,
 (SELECT count(*) FROM teams_workflow_attempts a WHERE a.event_id=d.event_id AND a.delivery_id=d.id)::integer AS attempt_count
FROM teams_workflow_deliveries d JOIN operations o ON o.id=d.operation_id
LEFT JOIN LATERAL (
 SELECT receipt.* FROM teams_workflow_attempts a LEFT JOIN teams_workflow_receipts receipt ON receipt.event_id=a.event_id AND receipt.attempt_id=a.id
 WHERE a.event_id=d.event_id AND a.delivery_id=d.id ORDER BY a.attempt_number DESC LIMIT 1
) r ON true
WHERE d.event_id=$1 ORDER BY d.created_at DESC,d.id DESC LIMIT 100;

-- name: TeamsWorkflowOccurrenceDelivery :one
SELECT * FROM teams_workflow_deliveries WHERE event_id=$1 AND occurrence_id=$2;
