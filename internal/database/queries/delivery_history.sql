-- name: CommunicationDeliveryHistory :many
WITH candidates AS (
 SELECT m.operation_id AS id,m.operation_id,m.event_id,m.campaign_id,m.campaign_version,m.person_id,
 'smtp'::text AS channel,'campaign'::text AS purpose,m.created_at AS selected_at,m.state AS provider_state,m.receipt_code AS reason,
 o.state AS operation_state,o.cancelled,o.revision,
 (SELECT count(*)::int FROM smtp_submission_attempts a WHERE a.operation_id=m.operation_id) AS attempt_count
 FROM smtp_campaign_messages m JOIN operations o ON o.id=m.operation_id
 WHERE m.event_id=sqlc.arg(event_id)
 UNION ALL
 SELECT m.operation_id,m.operation_id,m.event_id,t.campaign_id,t.campaign_version,m.person_id,
 'smtp','self_test',t.created_at,m.state,m.receipt_code,o.state,o.cancelled,o.revision,
 (SELECT count(*)::int FROM smtp_submission_attempts a WHERE a.operation_id=m.operation_id)
 FROM smtp_message_snapshots m JOIN operations o ON o.id=m.operation_id
 JOIN communication_test_requests t ON t.event_id=m.event_id AND t.id=m.test_request_id
 WHERE m.event_id=sqlc.arg(event_id)
 UNION ALL
 SELECT o.id,o.id,t.event_id,t.campaign_id,t.campaign_version,t.actor_id,
 'smtp','self_test',t.created_at,'queued','awaiting_submission',o.state,o.cancelled,o.revision,0
 FROM communication_test_requests t JOIN operations o ON o.event_id=t.event_id AND o.target_id=t.id AND o.kind='communication_test_smtp'
 WHERE t.event_id=sqlc.arg(event_id) AND t.channel='smtp'
 AND NOT EXISTS(SELECT 1 FROM smtp_message_snapshots m WHERE m.operation_id=o.id)
 UNION ALL
 SELECT m.intent_id,'00000000-0000-0000-0000-000000000000'::uuid,m.event_id,m.campaign_id,m.campaign_version,m.person_id,
 'smtp','omission',m.recorded_at,'unavailable',m.reason,'',false,0::bigint,0
 FROM smtp_campaign_omissions m WHERE m.event_id=sqlc.arg(event_id)
 UNION ALL
 SELECT d.id,d.operation_id,d.event_id,COALESCE(d.campaign_id,'00000000-0000-0000-0000-000000000000'::uuid),COALESCE(d.campaign_version,0::bigint),'00000000-0000-0000-0000-000000000000'::uuid,
 'teams',d.kind,d.created_at,COALESCE(r.outcome,'queued'),COALESCE(r.reason,''),o.state,o.cancelled,o.revision,
 (SELECT count(*)::int FROM teams_workflow_attempts a WHERE a.event_id=d.event_id AND a.delivery_id=d.id)
 FROM teams_workflow_deliveries d JOIN operations o ON o.id=d.operation_id
 LEFT JOIN LATERAL (
  SELECT receipt.* FROM teams_workflow_attempts a LEFT JOIN teams_workflow_receipts receipt ON receipt.event_id=a.event_id AND receipt.attempt_id=a.id
  WHERE a.event_id=d.event_id AND a.delivery_id=d.id ORDER BY a.attempt_number DESC LIMIT 1
 ) r ON true
 WHERE d.event_id=sqlc.arg(event_id)
 UNION ALL
 SELECT n.id,'00000000-0000-0000-0000-000000000000'::uuid,n.event_id,n.source_id,n.source_version,n.recipient_id,
 'in_app','campaign',n.created_at,'local_persisted','recorded_in_app','',false,0::bigint,0
 FROM inbox_notifications n WHERE n.event_id=sqlc.arg(event_id) AND n.source_kind='campaign'
)
SELECT c.*,COALESCE(p.display_name,'')::text AS recipient_name
FROM candidates c LEFT JOIN people p ON p.id=c.person_id
WHERE (sqlc.narg(campaign_filter)::uuid IS NULL OR c.campaign_id=sqlc.narg(campaign_filter)::uuid)
AND (sqlc.narg(delivery_filter)::uuid IS NULL OR c.id=sqlc.narg(delivery_filter)::uuid)
AND (sqlc.narg(channel_filter)::text IS NULL OR c.channel=sqlc.narg(channel_filter)::text)
AND (sqlc.narg(before_time)::timestamptz IS NULL OR (c.selected_at,c.id)<(sqlc.narg(before_time)::timestamptz,sqlc.narg(before_id)::uuid))
ORDER BY c.selected_at DESC,c.id DESC LIMIT sqlc.arg(page_size)::int;

-- name: SMTPDeliveryAttempts :many
SELECT a.id,a.attempt_number,a.prepared_at,r.outcome,r.receipt_code,r.recorded_at
FROM smtp_submission_attempts a LEFT JOIN smtp_submission_receipts r ON r.attempt_id=a.id
WHERE a.operation_id=$1 AND (sqlc.arg(before_number)::int=0 OR a.attempt_number<sqlc.arg(before_number)::int)
ORDER BY a.attempt_number DESC LIMIT sqlc.arg(page_size)::int;

-- name: TeamsDeliveryAttempts :many
SELECT a.id,a.attempt_number,a.started_at,r.outcome,r.reason,r.http_status,r.retry_at,r.recorded_at
FROM teams_workflow_attempts a LEFT JOIN teams_workflow_receipts r ON r.event_id=a.event_id AND r.attempt_id=a.id
WHERE a.event_id=$1 AND a.delivery_id=$2 AND (sqlc.arg(before_number)::int=0 OR a.attempt_number<sqlc.arg(before_number)::int)
ORDER BY a.attempt_number DESC LIMIT sqlc.arg(page_size)::int;
