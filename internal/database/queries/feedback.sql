-- name: FeedbackHead :one
SELECT * FROM feedback_heads WHERE event_id=$1 AND id=$2;
-- name: FeedbackHeadsPage :many
SELECT * FROM feedback_heads WHERE event_id=$1 AND id>sqlc.arg(after_id) AND (sqlc.narg(team_id)::uuid IS NULL OR team_id=sqlc.narg(team_id)) ORDER BY id LIMIT sqlc.arg(page_size);
-- name: InsertFeedbackHead :exec
INSERT INTO feedback_heads(event_id,id,team_id,category,current_version) VALUES($1,$2,$3,$4,1);
-- name: UpdateFeedbackHead :exec
UPDATE feedback_heads SET current_version=$3,published_version=$4 WHERE event_id=$1 AND id=$2;
-- name: FeedbackVersion :one
SELECT document FROM feedback_versions WHERE event_id=$1 AND id=$2 AND version=$3;
-- name: InsertFeedbackVersion :exec
INSERT INTO feedback_versions(event_id,id,version,document) VALUES($1,$2,$3,$4);
-- name: FeedbackHistoryPage :many
SELECT document FROM feedback_versions WHERE event_id=$1 AND id=$2 AND version<sqlc.arg(before_version) ORDER BY version DESC LIMIT sqlc.arg(page_size);
-- name: FeedbackActionHead :one
SELECT * FROM feedback_action_heads WHERE event_id=$1 AND id=$2;
-- name: FeedbackActionHeadsPage :many
SELECT * FROM feedback_action_heads WHERE event_id=$1 AND feedback_id=$2 AND id>sqlc.arg(after_id) ORDER BY id LIMIT sqlc.arg(page_size);
-- name: InsertFeedbackActionHead :exec
INSERT INTO feedback_action_heads(event_id,feedback_id,id,current_version) VALUES($1,$2,$3,1);
-- name: UpdateFeedbackActionHead :exec
UPDATE feedback_action_heads SET current_version=$3 WHERE event_id=$1 AND id=$2;
-- name: FeedbackActionVersion :one
SELECT document FROM feedback_action_versions WHERE event_id=$1 AND id=$2 AND version=$3;
-- name: InsertFeedbackActionVersion :exec
INSERT INTO feedback_action_versions(event_id,id,version,document) VALUES($1,$2,$3,$4);
-- name: FeedbackActionHistoryPage :many
SELECT document FROM feedback_action_versions WHERE event_id=$1 AND id=$2 AND version<sqlc.arg(before_version) ORDER BY version DESC LIMIT sqlc.arg(page_size);
-- name: FeedbackAcknowledgements :many
SELECT * FROM feedback_acknowledgements WHERE event_id=$1 AND feedback_id=$2 AND published_version=$3 ORDER BY person_id;
-- name: AcknowledgeFeedback :exec
INSERT INTO feedback_acknowledgements(event_id,feedback_id,published_version,person_id,acknowledged_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING;
-- name: FeedbackSettings :one
SELECT * FROM feedback_settings_versions WHERE event_id=$1 ORDER BY version DESC LIMIT 1;
-- name: InsertFeedbackSettings :exec
INSERT INTO feedback_settings_versions(event_id,version,setup_seconds,logistics_seconds,output_seconds,actor_id,changed_at) VALUES($1,$2,$3,$4,$5,$6,$7);
-- name: InsertFeedbackEscalation :execrows
INSERT INTO feedback_escalations(event_id,id,action_id,cycle,settings_version,recipient_id,threshold_seconds,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING;
-- name: FeedbackEscalations :many
SELECT e.* FROM feedback_escalations e JOIN feedback_action_heads h ON h.event_id=e.event_id AND h.id=e.action_id WHERE e.event_id=$1 AND h.feedback_id=$2 ORDER BY e.occurred_at,e.action_id,e.recipient_id;
-- name: FeedbackCommand :one
SELECT * FROM feedback_commands WHERE event_id=$1 AND actor_id=$2 AND command_id=$3;
-- name: InsertFeedbackCommand :exec
INSERT INTO feedback_commands(event_id,actor_id,command_id,target_id,kind,payload_digest,result_version) VALUES($1,$2,$3,$4,$5,$6,$7);
-- name: FeedbackMaintenanceEvents :many
SELECT DISTINCT e.id FROM events e JOIN feedback_heads f ON f.event_id=e.id WHERE e.writes_allowed AND e.id>sqlc.arg(after_id) ORDER BY e.id LIMIT 100;

-- name: FeedbackEscalation :one
SELECT * FROM feedback_escalations WHERE event_id=$1 AND id=$2;
