-- name: LockMailPerson :one
SELECT id,active,session_revision FROM people WHERE id=$1 FOR UPDATE;

-- name: LockPersonEmailContact :one
SELECT * FROM person_email_contacts WHERE person_id=$1 FOR UPDATE;

-- name: PersonEmailContact :one
SELECT * FROM person_email_contacts WHERE person_id=$1;

-- name: SyncVerifiedClaimContact :exec
INSERT INTO person_email_contacts(person_id,mailbox,provenance) VALUES($1,$2,'oidc_verified')
ON CONFLICT(person_id) DO UPDATE SET mailbox=EXCLUDED.mailbox,verified_at=now()
WHERE person_email_contacts.provenance='oidc_verified';

-- name: ClearUnverifiedClaimContact :exec
DELETE FROM person_email_contacts WHERE person_id=$1 AND provenance='oidc_verified';

-- name: ConfirmPersonEmail :exec
INSERT INTO person_email_contacts(person_id,mailbox,provenance) VALUES($1,$2,'code_confirmed')
ON CONFLICT(person_id) DO UPDATE SET mailbox=EXCLUDED.mailbox,provenance='code_confirmed',verified_at=now();

-- name: LatestEmailVerification :one
SELECT * FROM email_verifications WHERE person_id=$1 ORDER BY requested_at DESC,id DESC LIMIT 1;

-- name: LockEmailVerification :one
SELECT * FROM email_verifications WHERE person_id=$1 ORDER BY requested_at DESC,id DESC LIMIT 1 FOR UPDATE;

-- name: PersonEmailRequestCount :one
SELECT count(*) FROM email_verifications WHERE person_id=$1 AND requested_at >= $2;

-- name: DestinationEmailRequestCount :one
SELECT count(*) FROM email_verifications WHERE mailbox_hash=$1 AND requested_at >= $2;

-- name: InvalidatePendingEmailVerifications :exec
UPDATE email_verifications SET consumed_at=$2 WHERE person_id=$1 AND consumed_at IS NULL;

-- name: InsertEmailVerification :exec
INSERT INTO email_verifications(id,person_id,session_revision,mailbox,mailbox_hash,code_hash,requested_at,expires_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8);

-- name: RecordEmailVerificationReceipt :exec
UPDATE email_verifications SET delivery_state=$2,receipt_code=$3 WHERE id=$1;

-- name: FailEmailConfirmation :exec
UPDATE email_verifications SET failed_attempts=failed_attempts+1 WHERE id=$1 AND failed_attempts<5;

-- name: ConsumeEmailVerification :exec
UPDATE email_verifications SET consumed_at=$2 WHERE id=$1;

-- name: RetainEmailVerificationAttempts :exec
DELETE FROM email_verifications WHERE requested_at < $1;
