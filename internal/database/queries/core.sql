-- name: Person :one
SELECT * FROM people WHERE id = $1;

-- name: UpsertIdentity :one
INSERT INTO people (issuer, subject, display_name) VALUES ($1, $2, $3)
ON CONFLICT (issuer, subject) DO UPDATE SET display_name = EXCLUDED.display_name, updated_at = now()
RETURNING *;

-- name: BootstrapAdmin :execrows
UPDATE people SET platform_admin = true, session_revision = session_revision + 1,
revision = revision + 1, updated_at = now()
WHERE id = $1 AND NOT platform_admin;

-- name: RevokeSessions :exec
UPDATE people SET session_revision = session_revision + 1, revision = revision + 1, updated_at = now() WHERE id = $1;

-- name: Event :one
SELECT * FROM events WHERE id = $1;

-- name: UpdateEventName :one
UPDATE events SET name = $2, revision = revision + 1, updated_at = now()
WHERE id = $1 AND revision = $3 RETURNING *;

-- name: PutSession :exec
INSERT INTO sessions(token, data, expiry) VALUES ($1, $2, $3)
ON CONFLICT (token) DO UPDATE SET data = EXCLUDED.data, expiry = EXCLUDED.expiry;

-- name: FindSession :one
SELECT data FROM sessions WHERE token = $1 AND expiry > now();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token = $1;

-- name: PutLoginAttempt :exec
INSERT INTO login_attempts(state_hash, browser_hash, nonce, verifier, expires_at, return_to) VALUES ($1, $2, $3, $4, $5, $6);

-- name: ConsumeLoginAttempt :one
DELETE FROM login_attempts WHERE state_hash = $1 AND browser_hash = $2 AND expires_at > now() RETURNING *;

-- name: ExpireSessions :exec
DELETE FROM sessions WHERE expiry <= now();

-- name: ExpireLoginAttempts :exec
DELETE FROM login_attempts WHERE expires_at <= now();
