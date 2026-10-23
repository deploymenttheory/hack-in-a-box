-- name: OperationalProfile :one
SELECT p.*,person.display_name FROM operational_profiles p JOIN people person ON person.id=p.person_id WHERE p.event_id=$1 AND p.person_id=$2;

-- name: SaveOperationalProfile :one
INSERT INTO operational_profiles(event_id,person_id,contact_email,contact_phone,dietary,accessibility,photo_consent,travel_details,social_attendance)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT(event_id,person_id) DO UPDATE SET contact_email=EXCLUDED.contact_email,contact_phone=EXCLUDED.contact_phone,
dietary=EXCLUDED.dietary,accessibility=EXCLUDED.accessibility,photo_consent_revision=CASE WHEN operational_profiles.photo_consent IS DISTINCT FROM EXCLUDED.photo_consent THEN operational_profiles.photo_consent_revision+1 ELSE operational_profiles.photo_consent_revision END,photo_consent=EXCLUDED.photo_consent,travel_details=EXCLUDED.travel_details,social_attendance=EXCLUDED.social_attendance,revision=operational_profiles.revision+1,updated_at=now()
RETURNING *;

-- name: ListOperationalProfiles :many
SELECT p.*,person.display_name FROM operational_profiles p JOIN people person ON person.id=p.person_id
WHERE p.event_id=$1 AND p.id>$2 ORDER BY p.id LIMIT $3;

-- name: DeleteOperationalProfile :execrows
DELETE FROM operational_profiles WHERE event_id=$1 AND person_id=$2;

-- name: PrivacyPolicy :one
SELECT COALESCE(p.retention_days,90)::integer AS retention_days,COALESCE(p.draft_days,7)::integer AS draft_days
FROM events e LEFT JOIN event_privacy p ON p.event_id=e.id WHERE e.id=$1;

-- name: SetPrivacyPolicy :exec
INSERT INTO event_privacy(event_id,retention_days,draft_days) VALUES($1,$2,$3)
ON CONFLICT(event_id) DO UPDATE SET retention_days=EXCLUDED.retention_days,draft_days=EXCLUDED.draft_days;

-- name: ExpiredProfiles :many
SELECT p.id,p.event_id,p.person_id FROM operational_profiles p JOIN events e ON e.id=p.event_id LEFT JOIN event_privacy policy ON policy.event_id=e.id
WHERE e.stage IN ('completed','cancelled','archived') AND e.completed_at+make_interval(days=>COALESCE(policy.retention_days,90)) <= $1
ORDER BY p.event_id,p.id LIMIT 200;

-- name: CountRemovedProfile :exec
INSERT INTO privacy_aggregates(event_id,removed_profiles) VALUES($1,1)
ON CONFLICT(event_id) DO UPDATE SET removed_profiles=privacy_aggregates.removed_profiles+1;

-- name: RemoveExpiredDrafts :execrows
DELETE FROM private_drafts WHERE expires_at<=$1;

-- name: RemovePersonDrafts :exec
DELETE FROM private_drafts WHERE event_id=$1 AND person_id=$2;

-- name: SavePrivateDraft :execrows
INSERT INTO private_drafts(id,event_id,person_id,body,expires_at) VALUES($1,$2,$3,$4,$5)
ON CONFLICT(id) DO UPDATE SET body=EXCLUDED.body,expires_at=EXCLUDED.expires_at
WHERE private_drafts.event_id=EXCLUDED.event_id AND private_drafts.person_id=EXCLUDED.person_id;

-- name: PhotoConsentSubject :one
SELECT p.id,p.display_name,p.active,profile.id AS profile_id,COALESCE(profile.photo_consent,'unknown')::text AS photo_consent,COALESCE(profile.photo_consent_revision,0)::bigint AS consent_revision,
 (EXISTS(SELECT 1 FROM competition_claims c WHERE c.event_id=sqlc.arg(event_id)::uuid AND c.person_id=p.id AND c.state='confirmed') OR EXISTS(SELECT 1 FROM event_memberships m WHERE m.event_id=sqlc.arg(event_id)::uuid AND m.person_id=p.id AND m.active AND m.role!='early_brief_access'))::boolean AS eligible
FROM people p LEFT JOIN operational_profiles profile ON profile.event_id=sqlc.arg(event_id)::uuid AND profile.person_id=p.id
WHERE p.id=sqlc.arg(person_id)::uuid;

-- name: PhotoConsentRoster :many
SELECT p.id FROM people p WHERE p.active AND p.id>sqlc.arg(cursor)::uuid AND (
 EXISTS(SELECT 1 FROM competition_claims c WHERE c.event_id=sqlc.arg(event_id)::uuid AND c.person_id=p.id AND c.state='confirmed') OR
 EXISTS(SELECT 1 FROM event_memberships m WHERE m.event_id=sqlc.arg(event_id)::uuid AND m.person_id=p.id AND m.active AND m.role!='early_brief_access'))
ORDER BY p.id LIMIT sqlc.arg(page_size);

-- name: SetOwnPhotoConsent :one
INSERT INTO operational_profiles(event_id,person_id,photo_consent) VALUES($1,$2,$3)
ON CONFLICT(event_id,person_id) DO UPDATE SET
 photo_consent_revision=CASE WHEN operational_profiles.photo_consent IS DISTINCT FROM EXCLUDED.photo_consent THEN operational_profiles.photo_consent_revision+1 ELSE operational_profiles.photo_consent_revision END,
 photo_consent=EXCLUDED.photo_consent,revision=operational_profiles.revision+1,updated_at=now()
RETURNING *;
