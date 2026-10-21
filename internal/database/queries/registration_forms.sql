-- name: RegistrationForm :one
SELECT * FROM registration_forms WHERE event_id=$1 AND id=$2;

-- name: RegistrationForms :many
SELECT * FROM registration_forms WHERE event_id=$1 ORDER BY id LIMIT 50;

-- name: NewRegistrationForm :one
INSERT INTO registration_forms(event_id,id) VALUES($1,$2) RETURNING *;

-- name: AdvanceRegistrationForm :one
UPDATE registration_forms SET latest_version=latest_version+1,revision=revision+1 WHERE event_id=$1 AND id=$2 RETURNING *;

-- name: NewRegistrationFormVersion :exec
INSERT INTO registration_form_versions(event_id,form_id,version,definition) VALUES($1,$2,$3,$4);

-- name: RegistrationFormVersion :one
SELECT * FROM registration_form_versions WHERE event_id=$1 AND form_id=$2 AND version=$3;

-- name: PublishRegistrationForm :one
UPDATE registration_forms SET published_version=$3,revision=revision+1 WHERE event_id=$1 AND id=$2 RETURNING *;

-- name: RecordFormPublication :exec
INSERT INTO registration_form_publications(event_id,form_id,version) VALUES($1,$2,$3) ON CONFLICT DO NOTHING;

-- name: WasFormPublished :one
SELECT EXISTS(SELECT 1 FROM registration_form_publications WHERE event_id=$1 AND form_id=$2 AND version=$3);

-- name: FormResponse :one
SELECT * FROM form_responses WHERE event_id=$1 AND person_id=$2 AND form_id=$3;

-- name: AdvanceFormResponse :one
INSERT INTO form_responses(event_id,person_id,form_id) VALUES($1,$2,$3)
ON CONFLICT(event_id,person_id,form_id) DO UPDATE SET revision=form_responses.revision+1 RETURNING *;

-- name: NewFormResponseVersion :exec
INSERT INTO form_response_versions(event_id,person_id,form_id,revision,form_version,answers) VALUES($1,$2,$3,$4,$5,$6);

-- name: CurrentFormResponse :one
SELECT v.* FROM form_responses r JOIN form_response_versions v USING(event_id,person_id,form_id,revision)
WHERE r.event_id=$1 AND r.person_id=$2 AND r.form_id=$3;

-- name: ExpiredFormResponseEvents :many
SELECT DISTINCT r.event_id FROM form_responses r JOIN events e ON e.id=r.event_id LEFT JOIN event_privacy p ON p.event_id=e.id
WHERE e.stage IN ('completed','cancelled','archived') AND e.completed_at+make_interval(days=>COALESCE(p.retention_days,90)) <= $1
ORDER BY r.event_id LIMIT 50;

-- name: DeleteFormResponses :execrows
DELETE FROM form_responses WHERE event_id=$1;
