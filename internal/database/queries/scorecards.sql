-- name: ScorecardCheckpoints :many
SELECT DISTINCT ON(id) * FROM scorecard_checkpoint_versions WHERE event_id=$1 AND id>sqlc.arg(after_id)::uuid ORDER BY id,version DESC LIMIT sqlc.arg(page_size)::integer;

-- name: ScorecardCheckpoint :one
SELECT * FROM scorecard_checkpoint_versions WHERE event_id=$1 AND id=$2 ORDER BY version DESC LIMIT 1;

-- name: InsertScorecardCheckpoint :exec
INSERT INTO scorecard_checkpoint_versions(event_id,id,version,run_id,document,actor_id,reason,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8);

-- name: OwnScorecards :many
SELECT DISTINCT ON(id) * FROM mentor_scorecard_versions WHERE event_id=$1 AND team_id=$2 AND author_id=$3 AND id>sqlc.arg(after_id)::uuid ORDER BY id,version DESC LIMIT sqlc.arg(page_size)::integer;

-- name: LatestMentorScorecard :one
SELECT * FROM mentor_scorecard_versions WHERE event_id=$1 AND id=$2 ORDER BY version DESC LIMIT 1;

-- name: MentorScorecardVersion :one
SELECT * FROM mentor_scorecard_versions WHERE event_id=$1 AND id=$2 AND version=$3;

-- name: MentorScorecardHistory :many
SELECT * FROM mentor_scorecard_versions WHERE event_id=$1 AND id=$2 AND (sqlc.narg(before_version)::bigint IS NULL OR version<sqlc.narg(before_version)::bigint) ORDER BY version DESC LIMIT sqlc.arg(page_size)::integer;

-- name: InsertMentorScorecard :exec
INSERT INTO mentor_scorecard_versions(event_id,team_id,checkpoint_id,checkpoint_version,id,version,author_id,run_id,rubric_id,rubric_version,submission_revision,document,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13);

-- name: InsertMentorScorecardSignoff :exec
INSERT INTO mentor_scorecard_signoffs(event_id,id,team_id,run_id,context_id,context_version,actor_id,proof,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9);

-- name: MentorScorecardSignoff :one
SELECT * FROM mentor_scorecard_signoffs WHERE event_id=$1 AND context_id=$2 AND context_version=$3;

-- name: InsertMentorSignoffCard :exec
INSERT INTO mentor_scorecard_signoff_cards(event_id,signoff_id,card_id,card_version) VALUES($1,$2,$3,$4);
