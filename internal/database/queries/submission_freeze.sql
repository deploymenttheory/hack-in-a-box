-- name: SubmissionFreezeHead :one
SELECT revision FROM submission_freeze_heads WHERE event_id=$1 AND team_id=$2;

-- name: EnsureSubmissionFreezeHead :exec
INSERT INTO submission_freeze_heads(event_id,team_id) VALUES($1,$2) ON CONFLICT DO NOTHING;

-- name: AdvanceSubmissionFreezeHead :one
UPDATE submission_freeze_heads SET revision=revision+1 WHERE event_id=$1 AND team_id=$2 RETURNING revision;

-- name: SubmissionBranchConfiguration :one
SELECT * FROM submission_branch_configurations WHERE event_id=$1 AND team_id=$2;

-- name: SaveSubmissionBranchConfiguration :exec
INSERT INTO submission_branch_configurations(event_id,team_id,revision,repository_id,provenance,branch,actor_id) VALUES($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT(event_id,team_id) DO UPDATE SET revision=EXCLUDED.revision,repository_id=EXCLUDED.repository_id,provenance=EXCLUDED.provenance,branch=EXCLUDED.branch,actor_id=EXCLUDED.actor_id;

-- name: SubmissionFreezeEvents :many
SELECT id FROM events WHERE writes_allowed AND stage NOT IN ('cancelled','completed','archived') AND configuration->>'submission_cutoff' IS NOT NULL ORDER BY id;

-- name: SubmissionFreezeSource :one
SELECT * FROM submission_freeze_sources WHERE event_id=$1 AND team_id=$2;

-- name: InsertSubmissionFreezeSource :exec
INSERT INTO submission_freeze_sources(id,event_id,team_id,cutoff,binding,definition) VALUES($1,$2,$3,$4,$5,$6);

-- name: InsertSubmissionFreezeObservation :exec
INSERT INTO submission_freeze_observations(id,source_id,observed_at,definition) VALUES($1,$2,$3,$4);

-- name: SubmissionFreezeObservations :many
SELECT * FROM submission_freeze_observations WHERE source_id=$1 ORDER BY observed_at,id;

-- name: InsertSubmissionFreezeDecision :exec
INSERT INTO submission_freeze_decisions(id,source_id,revision,actor_id,decided_at,definition) VALUES($1,$2,$3,$4,$5,$6);

-- name: SubmissionFreezeDecisions :many
SELECT * FROM submission_freeze_decisions WHERE source_id=$1 ORDER BY revision;

-- name: SubmissionFreezeCommand :one
SELECT * FROM submission_freeze_commands WHERE event_id=$1 AND request_id=$2;

-- name: InsertSubmissionFreezeCommand :exec
INSERT INTO submission_freeze_commands(event_id,request_id,team_id,actor_id,fingerprint,result) VALUES($1,$2,$3,$4,$5,$6);

-- name: SubmissionFreezePushEvidence :many
SELECT id,repository_id,app_id,installation_id,provenance,ref,after_sha,provider_at,received_at,payload_hash FROM github_webhook_deliveries
WHERE event_id=$1 AND team_id=$2 AND repository_id=$3 AND provenance=$4 AND kind='push' AND ref=$5
AND id > $6 ORDER BY id LIMIT $7;

-- name: SubmissionFreezeExactPush :one
SELECT id,repository_id,app_id,installation_id,provenance,ref,after_sha,provider_at,received_at,payload_hash FROM github_webhook_deliveries WHERE event_id=$1 AND team_id=$2 AND id=$3 AND kind='push';
