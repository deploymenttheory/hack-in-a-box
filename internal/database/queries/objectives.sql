-- name: ObjectiveHead :one
SELECT * FROM event_objectives WHERE event_id=$1 AND id=$2;

-- name: ObjectiveVersion :one
SELECT * FROM objective_versions WHERE event_id=$1 AND objective_id=$2 AND version=$3;

-- name: ObjectiveHeads :many
SELECT * FROM event_objectives WHERE event_id=$1 AND id>sqlc.arg(cursor)::uuid ORDER BY id LIMIT sqlc.arg(page_size);

-- name: ObjectiveHistory :many
SELECT * FROM objective_versions WHERE event_id=$1 AND objective_id=$2
 AND (sqlc.arg(before_version)::bigint=0 OR version<sqlc.arg(before_version)::bigint)
 ORDER BY version DESC LIMIT sqlc.arg(page_size);

-- name: CreateObjectiveHead :exec
INSERT INTO event_objectives(event_id,id,current_version) VALUES($1,$2,1);

-- name: AppendObjectiveVersion :exec
INSERT INTO objective_versions(event_id,objective_id,version,definition,retired,reason,actor_id,created_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8);

-- name: AdvanceObjectiveHead :exec
UPDATE event_objectives SET current_version=$3 WHERE event_id=$1 AND id=$2;
