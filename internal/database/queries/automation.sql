-- name: AutomationBindingsPage :many
SELECT DISTINCT ON(id) * FROM communication_automation_versions WHERE event_id=$1 AND id>sqlc.arg(after_id)::uuid ORDER BY id,version DESC LIMIT sqlc.arg(page_size);

-- name: AutomationBinding :one
SELECT * FROM communication_automation_versions WHERE event_id=$1 AND id=$2 ORDER BY version DESC LIMIT 1;

-- name: SaveAutomationBinding :exec
INSERT INTO communication_automation_versions(event_id,id,version,campaign_id,campaign_version,document,actor_id,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8);

-- name: AutomationPaused :one
SELECT COALESCE((SELECT paused FROM communication_automation_settings WHERE event_id=$1),false)::boolean;

-- name: SetAutomationPause :exec
INSERT INTO communication_automation_settings(event_id,paused,actor_id,reason) VALUES($1,$2,$3,$4)
ON CONFLICT(event_id) DO UPDATE SET paused=$2,actor_id=$3,reason=$4,revision=communication_automation_settings.revision+1,updated_at=now();

-- name: AutomationOccurrencesPage :many
SELECT * FROM communication_occurrences WHERE event_id=$1 AND id>sqlc.arg(after_id)::uuid ORDER BY id LIMIT sqlc.arg(page_size);

-- name: AutomationOccurrence :one
SELECT * FROM communication_occurrences WHERE event_id=$1 AND id=$2;

-- name: SaveAutomationOccurrence :one
INSERT INTO communication_occurrences(event_id,id,binding_id,binding_version,source_kind,source_id,source_revision,source_key,due_at,deadline,scope_binding,state,reason)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
ON CONFLICT(event_id,binding_id,binding_version,source_key) DO UPDATE SET source_key=EXCLUDED.source_key RETURNING *;

-- name: SetAutomationOccurrenceState :exec
UPDATE communication_occurrences SET state=$3,reason=$4,review_until=$5,updated_at=now() WHERE event_id=$1 AND id=$2;

-- name: SetAutomationOperation :exec
UPDATE communication_occurrences SET operation_id=$3 WHERE event_id=$1 AND id=$2 AND operation_id IS NULL;

-- name: CancelAutomationBindingOccurrences :exec
UPDATE communication_occurrences SET state='cancelled',reason='binding_changed',updated_at=now() WHERE event_id=$1 AND binding_id=$2 AND binding_version<>$3 AND state<>'cancelled';

-- name: AutomationEvents :many
SELECT DISTINCT event_id FROM communication_automation_versions WHERE event_id>$1 ORDER BY event_id LIMIT 50;

-- name: RecordAutomationHistory :exec
INSERT INTO communication_automation_history(event_id,occurrence_id,binding_id,actor_id,action,reason) VALUES($1,$2,$3,$4,$5,$6);

-- name: PublicationPlanApproval :one
SELECT * FROM communication_publication_approvals WHERE event_id=$1 AND id=$2;

-- name: SavePublicationPlanApproval :exec
INSERT INTO communication_publication_approvals(event_id,id,campaign_id,campaign_version,source_id,source_version,preview_id,actor_id,document,reason,approved,preview_plan_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12);

-- name: ReviewAutomationOccurrence :exec
UPDATE communication_occurrences SET state='pending',reason='explicit_review',review_until=$3,review_revision=review_revision+1,scope_binding=$4,operation_id=NULL,updated_at=now() WHERE event_id=$1 AND id=$2;

-- name: RecordAutomationChannelReports :exec
UPDATE communication_occurrences SET state=$3,reason=$4,channel_reports=$5,updated_at=now() WHERE event_id=$1 AND id=$2;

-- name: PendingAutomationOccurrences :many
SELECT * FROM communication_occurrences WHERE event_id=$1 AND id>sqlc.arg(after_id)::uuid AND state IN ('pending','paused') ORDER BY id LIMIT 200;
