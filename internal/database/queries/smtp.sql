-- name: SMTPMessageSnapshot :one
SELECT * FROM smtp_message_snapshots WHERE operation_id=$1;

-- name: SaveSMTPMessageSnapshot :exec
INSERT INTO smtp_message_snapshots(operation_id,event_id,person_id,test_request_id,mailbox,message_id,snapshot)
VALUES($1,$2,$3,$4,$5,$6,$7);

-- name: RecordSMTPReceipt :exec
UPDATE smtp_message_snapshots SET state=$2,receipt_code=$3,updated_at=now() WHERE operation_id=$1;

-- name: SMTPTestOperation :one
SELECT * FROM operations WHERE event_id=$1 AND target_id=$2 AND kind='communication_test_smtp';

-- name: NextEmailCalendarSequence :one
INSERT INTO email_calendar_sequences(event_id,test,fingerprint) VALUES($1,$2,$3)
ON CONFLICT(event_id,test) DO UPDATE SET
sequence=CASE WHEN email_calendar_sequences.fingerprint<>EXCLUDED.fingerprint THEN email_calendar_sequences.sequence+1 ELSE email_calendar_sequences.sequence END,
fingerprint=EXCLUDED.fingerprint
RETURNING sequence;

-- name: CancelExpiredSMTPTestOperations :execrows
UPDATE operations SET cancelled=true,approved=false,state='cancelled',revision=revision+1,updated_at=now()
WHERE kind='communication_test_smtp' AND state='pending' AND NOT cancelled AND due_at <= $1;

-- name: SMTPCampaignMessage :one
SELECT * FROM smtp_campaign_messages WHERE operation_id=$1;

-- name: SaveSMTPCampaignMessage :exec
INSERT INTO smtp_campaign_messages(operation_id,event_id,intent_id,campaign_id,campaign_version,approval_binding,occurrence_id,occurrence_binding,provider_revision,person_id,mailbox,snapshot)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT(operation_id) DO NOTHING;

-- name: RecordSMTPCampaignReceipt :exec
UPDATE smtp_campaign_messages SET state=$2,receipt_code=$3,updated_at=now() WHERE operation_id=$1;

-- name: PrepareSMTPSubmissionAttempt :one
INSERT INTO smtp_submission_attempts(operation_id,source_kind,attempt_number)
SELECT sqlc.arg(operation_id)::uuid,sqlc.arg(source_kind)::text,coalesce(max(attempt_number),0)+1
FROM smtp_submission_attempts WHERE operation_id=sqlc.arg(operation_id)::uuid
RETURNING *;

-- name: LatestSMTPSubmissionAttempt :one
SELECT * FROM smtp_submission_attempts WHERE operation_id=$1 ORDER BY attempt_number DESC LIMIT 1;

-- name: RecordSMTPSubmissionReceipt :exec
INSERT INTO smtp_submission_receipts(attempt_id,outcome,receipt_code) VALUES($1,$2,$3);

-- name: SMTPSubmissionHistory :many
SELECT a.id,a.operation_id,a.source_kind,a.attempt_number,a.prepared_at,r.outcome,r.receipt_code,r.recorded_at
FROM smtp_submission_attempts a LEFT JOIN smtp_submission_receipts r ON r.attempt_id=a.id
WHERE a.operation_id=$1 ORDER BY a.attempt_number;
