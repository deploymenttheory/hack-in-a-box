-- name: MediaHead :one
SELECT * FROM event_media WHERE event_id=$1 AND id=$2;

-- name: CreateMediaHead :exec
INSERT INTO event_media(event_id,id,owner_id,version,state)VALUES($1,$2,$3,1,'draft');

-- name: SaveMediaHead :exec
UPDATE event_media SET version=$3,state=$4,released_version=$5 WHERE event_id=$1 AND id=$2;

-- name: MediaVersion :one
SELECT * FROM media_versions WHERE event_id=$1 AND id=$2 AND version=$3;

-- name: InsertMediaVersion :exec
INSERT INTO media_versions(event_id,id,version,document,actor_id,action,reason,created_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8);

-- name: InsertMediaFile :exec
INSERT INTO media_files(event_id,media_id,file_id)VALUES($1,$2,$3);

-- name: MediaFile :one
SELECT file_id FROM media_files WHERE event_id=$1 AND media_id=$2;

-- name: MediaCommand :one
SELECT * FROM media_commands WHERE event_id=$1 AND actor_id=$2 AND command_id=$3;

-- name: RecordMediaCommand :exec
INSERT INTO media_commands(event_id,id,actor_id,command_id,fingerprint,version)VALUES($1,$2,$3,$4,$5,$6);

-- name: MediaPage :many
SELECT * FROM event_media WHERE event_id=sqlc.arg(event_id)::uuid AND id>sqlc.arg(cursor)::uuid
AND (sqlc.arg(organiser)::boolean OR (sqlc.arg(photographer)::boolean AND owner_id=sqlc.arg(actor_id)::uuid) OR (state='released' AND released_version=version))
ORDER BY id LIMIT sqlc.arg(page_size);

-- name: MediaMaintenanceEvents :many
SELECT DISTINCT event_id FROM event_media WHERE event_id>sqlc.arg(cursor)::uuid AND state IN('reviewed','released') ORDER BY event_id LIMIT sqlc.arg(page_size);

-- name: MediaHistoryMetadata :many
SELECT version,action,created_at FROM media_versions
WHERE event_id=$1 AND id=$2 AND version<sqlc.arg(before_version)::bigint
ORDER BY version DESC LIMIT sqlc.arg(page_size);
