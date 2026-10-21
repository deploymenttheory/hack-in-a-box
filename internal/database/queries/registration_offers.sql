-- name: RegistrationSettings :one
SELECT * FROM registration_settings WHERE event_id=$1;

-- name: SaveRegistrationSettings :one
INSERT INTO registration_settings(event_id,automated,working_days,approved_by) VALUES($1,$2,$3,$4)
ON CONFLICT(event_id) DO UPDATE SET automated=$2,working_days=$3,approved_by=$4,revision=registration_settings.revision+1 RETURNING *;

-- name: LatestRegistrationOffer :one
SELECT * FROM registration_offers WHERE event_id=$1 AND registration_id=$2 ORDER BY created_at DESC,id DESC LIMIT 1;

-- name: PendingRegistrationOffers :many
SELECT * FROM registration_offers WHERE event_id=$1 AND state='pending' ORDER BY expires_at,id;

-- name: CreateRegistrationOffer :one
INSERT INTO registration_offers(event_id,registration_id,expires_at) VALUES($1,$2,$3) RETURNING *;

-- name: ResolveRegistrationOffer :exec
UPDATE registration_offers SET state=$3 WHERE event_id=$1 AND id=$2 AND state='pending';

-- name: NextWaitlistedRegistration :one
SELECT * FROM registrations WHERE event_id=$1 AND kind='attendee' AND state='waitlisted' ORDER BY sequence LIMIT 1;

-- name: OfferMaintenanceEvents :many
SELECT DISTINCT event_id FROM registrations WHERE event_id>$1 AND state IN ('waitlisted','offered') ORDER BY event_id LIMIT 50;

-- name: RegistrationResponseVersion :one
SELECT * FROM form_response_versions WHERE event_id=$1 AND person_id=$2 AND form_id=$3 AND revision=$4;

-- name: CancelOfferNotification :exec
UPDATE operations SET state='cancelled',cancelled=true,revision=revision+1,updated_at=now()
WHERE event_id=$1 AND operation_key=$2 AND state='pending';
