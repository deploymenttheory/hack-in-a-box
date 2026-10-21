-- name: InsertFile :one
INSERT INTO files(id, event_id, owner_id, idempotency_key, object_key, filename, media_type, checksum, size_bytes)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING *;

-- name: File :one
SELECT * FROM files WHERE event_id = $1 AND id = $2;

-- name: IdempotentFile :one
SELECT * FROM files WHERE event_id = $1 AND owner_id = $2 AND idempotency_key = $3;

-- name: LockEvent :one
SELECT * FROM events WHERE id = $1 FOR UPDATE;

-- name: EventFileUsage :one
SELECT COALESCE(sum(size_bytes), 0)::bigint FROM files WHERE event_id = $1;

-- name: ObjectExists :one
SELECT EXISTS(SELECT 1 FROM files WHERE object_key = $1);
