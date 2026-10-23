-- name: GitHubWebhookSecret :one
SELECT * FROM github_webhook_secrets WHERE event_id=$1;

-- name: SaveGitHubWebhookSecret :exec
INSERT INTO github_webhook_secrets(event_id,app_id,version,key_id,encrypted) VALUES($1,$2,$3,$4,$5)
ON CONFLICT(event_id) DO UPDATE SET app_id=EXCLUDED.app_id,version=EXCLUDED.version,key_id=EXCLUDED.key_id,encrypted=EXCLUDED.encrypted;

-- name: DeleteGitHubWebhookSecret :exec
DELETE FROM github_webhook_secrets WHERE event_id=$1;

-- name: InsertGitHubWebhookDelivery :execrows
INSERT INTO github_webhook_deliveries(id,event_id,team_id,repository_id,app_id,installation_id,provenance,secret_version,payload_hash,kind,action,ref,after_sha,member_id,provider_at,received_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) ON CONFLICT(id) DO NOTHING;

-- name: GitHubWebhookDelivery :one
SELECT * FROM github_webhook_deliveries WHERE id=$1;

-- name: LinkGitHubWebhookOperation :exec
UPDATE github_webhook_deliveries SET operation_id=$2 WHERE id=$1 AND operation_id IS NULL;

-- name: RecordGitHubWebhookDisposition :execrows
UPDATE github_webhook_deliveries SET disposition=$2,applied_at=now() WHERE id=$1 AND disposition='pending';

-- name: AddGitHubRepositoryActivity :exec
INSERT INTO github_repository_activity(event_id,team_id,pushes,invitations,last_provider_at,last_received_at) VALUES($1,$2,$3,$4,$5,$6)
ON CONFLICT(event_id,team_id) DO UPDATE SET pushes=github_repository_activity.pushes+EXCLUDED.pushes,invitations=github_repository_activity.invitations+EXCLUDED.invitations,
last_provider_at=GREATEST(github_repository_activity.last_provider_at,EXCLUDED.last_provider_at),last_received_at=GREATEST(github_repository_activity.last_received_at,EXCLUDED.last_received_at);

-- name: ListGitHubWebhookDeliveries :many
SELECT d.*,COALESCE(o.state,'pending') AS operation_state,COALESCE(o.attempts,0)::integer AS attempts,
COALESCE(o.approved,false)::boolean AS approved,COALESCE(o.cancelled,false)::boolean AS cancelled
FROM github_webhook_deliveries d LEFT JOIN operations o ON o.id=d.operation_id WHERE d.event_id=$1 ORDER BY d.received_at DESC,d.id LIMIT 100;

-- name: GitHubRepositoryActivity :many
SELECT a.* FROM github_repository_activity a WHERE a.event_id=$1 ORDER BY a.team_id;
