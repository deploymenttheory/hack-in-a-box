-- name: CurrentResultMentorAdvisories :many
SELECT DISTINCT ON(tie_id,team_id,author_id) * FROM result_mentor_advisory_versions
WHERE event_id=$1 AND source_binding=$2 ORDER BY tie_id,team_id,author_id,version DESC;

-- name: InsertResultMentorAdvisory :exec
INSERT INTO result_mentor_advisory_versions(event_id,tie_id,team_id,author_id,version,judge_run_id,source_binding,document) VALUES($1,$2,$3,$4,$5,$6,$7,$8);

-- name: ResultMentorAdvisoryReceipt :one
SELECT * FROM result_mentor_advisory_commands WHERE event_id=$1 AND request_id=$2;

-- name: InsertResultMentorAdvisoryReceipt :exec
INSERT INTO result_mentor_advisory_commands(event_id,request_id,tie_id,team_id,author_id,version,receipt) VALUES($1,$2,$3,$4,$5,$6,$7);
