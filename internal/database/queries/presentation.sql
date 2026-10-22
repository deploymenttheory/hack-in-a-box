-- name: EnsurePresentation :exec
INSERT INTO presentation_states(event_id) VALUES($1) ON CONFLICT DO NOTHING;

-- name: PresentationState :one
SELECT * FROM presentation_states WHERE event_id=$1;

-- name: SavePresentationState :exec
UPDATE presentation_states SET revision=$2,draft=$3,published=$4,current_index=$5,mode=$6,remaining_seconds=$7,timer_ends_at=$8,override_cue=$9,override_mode=$10,override_remaining_seconds=$11,override_timer_ends_at=$12 WHERE event_id=$1;

-- name: DisplayPairings :many
SELECT id,label,issuer_id,created_at,expires_at,revoked_at,last_seen_at FROM display_pairings WHERE event_id=$1 ORDER BY created_at DESC,id LIMIT 200;

-- name: CreateDisplayPairing :exec
INSERT INTO display_pairings(event_id,id,label,token_hash,issuer_id,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7);

-- name: DisplayPairingByHash :one
SELECT * FROM display_pairings WHERE event_id=$1 AND token_hash=$2;

-- name: RevokeDisplayPairing :execrows
UPDATE display_pairings SET revoked_at=$3 WHERE event_id=$1 AND id=$2 AND revoked_at IS NULL;

-- name: TouchDisplayPairing :exec
UPDATE display_pairings SET last_seen_at=sqlc.arg(seen_at)::timestamptz
WHERE event_id=sqlc.arg(event_id)::uuid AND token_hash=sqlc.arg(token_hash)::bytea
AND (last_seen_at IS NULL OR last_seen_at<sqlc.arg(throttle_before)::timestamptz);
