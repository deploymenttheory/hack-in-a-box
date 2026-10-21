-- name: Registration :one
SELECT * FROM registrations WHERE event_id=$1 AND id=$2;

-- name: OwnRegistrations :many
SELECT * FROM registrations WHERE event_id=$1 AND person_id=$2 ORDER BY sequence DESC LIMIT 100;

-- name: ActiveRegistration :one
SELECT * FROM registrations WHERE event_id=$1 AND person_id=$2 AND kind=$3 AND state NOT IN ('withdrawn','declined');

-- name: InsertRegistration :one
INSERT INTO registrations(id,event_id,person_id,form_id,form_version,audience,kind,state,request_hash,team_preference,invitation_reference,fallback_to_attendee)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING *;

-- name: RegistrationState :one
UPDATE registrations SET state=$3,revision=revision+1 WHERE event_id=$1 AND id=$2 RETURNING *;

-- name: ReservedAttendees :one
SELECT count(*) FROM registrations WHERE event_id=$1 AND kind='attendee' AND state='offered';

-- name: WaitingAttendees :one
SELECT count(*) FROM registrations WHERE event_id=$1 AND kind='attendee' AND state='waitlisted';

-- name: WaitlistPosition :one
SELECT count(*) FROM registrations WHERE event_id=$1 AND kind='attendee' AND state='waitlisted' AND sequence<=$2;

-- name: RecordRegistrationHistory :exec
INSERT INTO registration_history(event_id,registration_id,actor_id,action,from_state,to_state,reason) VALUES($1,$2,$3,$4,$5,$6,$7);

-- name: RegistrationHistory :many
SELECT * FROM registration_history WHERE event_id=$1 AND registration_id=$2 ORDER BY created_at,id LIMIT 200;

-- name: OpenRegistrationEvents :many
SELECT * FROM events WHERE stage='registration' AND writes_allowed AND id>$1 ORDER BY id LIMIT $2;

-- name: PublishedRegistrationForms :many
SELECT v.* FROM registration_forms f JOIN registration_form_versions v ON v.event_id=f.event_id AND v.form_id=f.id AND v.version=f.published_version
WHERE f.event_id=$1 ORDER BY f.id LIMIT 50;

-- name: ListRegistrations :many
SELECT r.*,p.display_name FROM registrations r JOIN people p ON p.id=r.person_id WHERE r.event_id=$1 AND r.sequence>$2 ORDER BY r.sequence LIMIT $3;

-- name: SetRegistrationResponseRevision :exec
UPDATE registrations SET response_revision=$3 WHERE event_id=$1 AND id=$2;

-- name: ForgetRegistrationInput :exec
UPDATE registrations SET request_hash='',invitation_reference='' WHERE event_id=$1;
