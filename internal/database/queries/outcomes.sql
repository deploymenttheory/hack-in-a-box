-- name: OutcomeHead :one
SELECT * FROM outcome_heads WHERE id=$1;

-- name: LockOutcomeHead :one
SELECT * FROM outcome_heads WHERE id=$1 FOR UPDATE;

-- name: CreateOutcomeHead :execrows
INSERT INTO outcome_heads(id,author_id,event_id,team_id,status) VALUES($1,$2,$3,$4,'proposed') ON CONFLICT DO NOTHING;

-- name: UpdateOutcomeHead :exec
UPDATE outcome_heads SET event_id=$2,team_id=$3,owner_id=$4,status=$5,current_version=$6 WHERE id=$1;

-- name: OutcomeVersion :one
SELECT * FROM outcome_versions WHERE outcome_id=$1 AND version=$2;

-- name: OutcomeCommand :one
SELECT * FROM outcome_versions WHERE outcome_id=$1 AND command_id=$2;

-- name: AppendOutcomeVersion :exec
INSERT INTO outcome_versions(outcome_id,version,command_id,actor_id,expected_version,command,document,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8);

-- name: OutcomeHistory :many
SELECT * FROM outcome_versions WHERE outcome_id=$1 AND (sqlc.arg(before_version)::bigint=0 OR version<sqlc.arg(before_version)) ORDER BY version DESC LIMIT sqlc.arg(page_size);

-- name: OutcomeHeadsPage :many
SELECT * FROM outcome_heads WHERE id>sqlc.arg(after_id)
AND outcome_readable(id,sqlc.arg(actor_id)::uuid,sqlc.arg(at_time)::timestamptz)
AND (sqlc.arg(mode)::text='all' OR (sqlc.arg(mode)::text='own' AND (author_id=sqlc.arg(actor_id) OR owner_id=sqlc.arg(actor_id))) OR (sqlc.arg(mode)::text='review' AND (event_id=sqlc.narg(event_id)::uuid OR event_id IS NULL)))
ORDER BY id LIMIT sqlc.arg(page_size);

-- name: OutcomeProjectHead :one
SELECT * FROM outcome_project_heads WHERE outcome_id=$1;

-- name: CreateOutcomeProjectHead :exec
INSERT INTO outcome_project_heads(outcome_id) VALUES($1) ON CONFLICT DO NOTHING;

-- name: UpdateOutcomeProjectHead :exec
UPDATE outcome_project_heads SET current_version=$2,published_version=$3 WHERE outcome_id=$1;

-- name: OutcomeProjectVersion :one
SELECT * FROM outcome_project_versions WHERE outcome_id=$1 AND version=$2;

-- name: OutcomeProjectCommand :one
SELECT * FROM outcome_project_versions WHERE outcome_id=$1 AND command_id=$2;

-- name: AppendOutcomeProjectVersion :exec
INSERT INTO outcome_project_versions(outcome_id,version,source_version,command_id,actor_id,expected_version,command,document,state,published_fields,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11);

-- name: OutcomeReviewHead :one
SELECT * FROM outcome_review_heads WHERE id=$1;

-- name: CreateOutcomeReviewHead :exec
INSERT INTO outcome_review_heads(id,event_id,outcome_id,days,state) VALUES($1,$2,$3,$4,'draft') ON CONFLICT DO NOTHING;

-- name: UpdateOutcomeReviewHead :exec
UPDATE outcome_review_heads SET current_version=$2,state=$3 WHERE id=$1;

-- name: OutcomeReviewVersion :one
SELECT * FROM outcome_review_versions WHERE review_id=$1 AND version=$2;

-- name: OutcomeReviewCommand :one
SELECT * FROM outcome_review_versions WHERE review_id=$1 AND command_id=$2;

-- name: AppendOutcomeReviewVersion :exec
INSERT INTO outcome_review_versions(review_id,version,command_id,actor_id,expected_version,command,document,proof,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9);

-- name: OutcomeReviewHeads :many
SELECT * FROM outcome_review_heads WHERE outcome_id=$1 ORDER BY days,id;

-- name: OutcomeReadable :one
SELECT outcome_readable(sqlc.arg(outcome_id)::uuid,sqlc.arg(actor_id)::uuid,sqlc.arg(at_time)::timestamptz)::boolean AS allowed;

-- name: OutcomeCurrentTeamMember :one
SELECT outcome_current_team_member(sqlc.arg(event_id)::uuid,sqlc.arg(team_id)::uuid,sqlc.arg(actor_id)::uuid)::boolean AS allowed;

-- name: OutcomeAnyOrganiser :one
SELECT EXISTS(SELECT 1 FROM event_memberships m JOIN people p ON p.id=m.person_id AND p.active WHERE m.person_id=$1 AND m.role='organiser' AND m.active)::boolean AS allowed;

-- name: OutcomeEligibleOwner :one
SELECT outcome_eligible_owner(sqlc.arg(outcome_id)::uuid,sqlc.arg(person_id)::uuid)::boolean AS allowed;

-- name: OutcomeOwnerCandidates :many
SELECT p.id,p.display_name FROM people p WHERE p.id>sqlc.arg(after_id) AND outcome_eligible_owner(sqlc.arg(outcome_id)::uuid,p.id) ORDER BY p.id LIMIT sqlc.arg(page_size);

-- name: AppendOutcomeEvidence :exec
INSERT INTO outcome_evidence(outcome_id,id,outcome_version,actor_id,document,created_at) VALUES($1,$2,$3,$4,$5,$6);

-- name: OutcomeEvidence :one
SELECT * FROM outcome_evidence WHERE outcome_id=$1 AND id=$2;

-- name: OutcomeEvidencePage :many
SELECT * FROM outcome_evidence WHERE outcome_id=$1 AND id>sqlc.arg(after_id) ORDER BY id LIMIT sqlc.arg(page_size);

-- name: OutcomePrivacyRevision :one
SELECT COALESCE(ep.revision,0)::bigint AS revision FROM events e LEFT JOIN event_privacy ep ON ep.event_id=e.id WHERE e.id=$1;

-- name: OutcomePublishedProject :one
SELECT pv.* FROM outcome_project_heads ph JOIN outcome_project_versions pv ON pv.outcome_id=ph.outcome_id AND pv.version=ph.published_version
WHERE ph.outcome_id=sqlc.arg(outcome_id) AND outcome_project_readable(ph.outcome_id,sqlc.arg(actor_id)::uuid,sqlc.arg(at_time)::timestamptz);

-- name: OutcomePublishedProjectsPage :many
SELECT pv.* FROM outcome_project_heads ph JOIN outcome_project_versions pv ON pv.outcome_id=ph.outcome_id AND pv.version=ph.published_version
JOIN outcome_heads oh ON oh.id=ph.outcome_id
WHERE ph.outcome_id>sqlc.arg(after_id)
AND outcome_project_readable(ph.outcome_id,sqlc.arg(actor_id)::uuid,sqlc.arg(at_time)::timestamptz)
AND (sqlc.arg(search)::text='' OR position(lower(sqlc.arg(search)) in lower((pv.published_fields->>'problem')||' '||(pv.published_fields->>'solution')))>0)
AND (sqlc.arg(event_id)::uuid='00000000-0000-0000-0000-000000000000'::uuid OR oh.event_id=sqlc.arg(event_id)::uuid)
AND (sqlc.arg(status)::text='' OR pv.published_fields->>'follow_up_status'=sqlc.arg(status)::text)
ORDER BY ph.outcome_id LIMIT sqlc.arg(page_size);

-- name: OutcomePublishedProjectEvents :many
SELECT event_id,event_name FROM (
SELECT DISTINCT ON (oh.event_id) oh.event_id::uuid AS event_id, COALESCE(pv.published_fields->>'event_name','')::text AS event_name
FROM outcome_project_heads ph JOIN outcome_project_versions pv ON pv.outcome_id=ph.outcome_id AND pv.version=ph.published_version
JOIN outcome_heads oh ON oh.id=ph.outcome_id
WHERE oh.event_id IS NOT NULL AND pv.published_fields->>'event_name' IS NOT NULL
AND outcome_project_readable(ph.outcome_id,sqlc.arg(actor_id)::uuid,sqlc.arg(at_time)::timestamptz)
ORDER BY oh.event_id,pv.created_at DESC,pv.outcome_id
) approved_events ORDER BY event_name,event_id;

-- name: OutcomeReviewHeadsEventPage :many
SELECT * FROM outcome_review_heads WHERE event_id=$1 AND id>sqlc.arg(after_id) ORDER BY id LIMIT sqlc.arg(page_size);

-- name: OutcomeReviewSendable :one
SELECT outcome_review_sendable(sqlc.arg(event_id)::uuid,sqlc.arg(review_id)::uuid,sqlc.arg(version)::bigint,sqlc.arg(at_time)::timestamptz)::boolean AS allowed;

-- name: OutcomeObservationsPage :many
SELECT oe.*,oh.status AS current_follow_up_status FROM outcome_evidence oe JOIN outcome_heads oh ON oh.id=oe.outcome_id
WHERE oh.event_id=sqlc.arg(event_id)::uuid
AND outcome_readable(oh.id,sqlc.arg(actor_id)::uuid,sqlc.arg(at_time)::timestamptz)
AND (oe.outcome_id,oe.id)>(sqlc.arg(after_outcome_id)::uuid,sqlc.arg(after_evidence_id)::uuid)
ORDER BY oe.outcome_id,oe.id LIMIT sqlc.arg(page_size);

-- name: OutcomePublishedEventProjectsPage :many
SELECT pv.* FROM outcome_project_heads ph JOIN outcome_project_versions pv ON pv.outcome_id=ph.outcome_id AND pv.version=ph.published_version
JOIN outcome_heads oh ON oh.id=ph.outcome_id
WHERE oh.event_id=sqlc.arg(event_id)::uuid AND ph.outcome_id>sqlc.arg(after_id)
AND outcome_project_readable(ph.outcome_id,sqlc.arg(actor_id)::uuid,sqlc.arg(at_time)::timestamptz)
ORDER BY ph.outcome_id LIMIT sqlc.arg(page_size);


-- name: OutcomeArchiveReportingFacts :one
SELECT
 (SELECT count(*) FROM outcome_heads h WHERE h.event_id=sqlc.arg(event_id)::uuid AND h.status='in_progress' AND outcome_readable(h.id,sqlc.arg(actor_id)::uuid,sqlc.arg(at_time)::timestamptz)) AS continued_projects,
 (SELECT count(*) FROM outcome_evidence e JOIN outcome_heads h ON h.id=e.outcome_id
 WHERE h.event_id=sqlc.arg(event_id)::uuid AND outcome_readable(h.id,sqlc.arg(actor_id)::uuid,sqlc.arg(at_time)::timestamptz)
 AND e.document->>'basis'='demonstrated_adoption' AND e.document->>'observed_at' IS NOT NULL
 AND length(trim(COALESCE(e.document->>'source','')))>0) AS dated_adoption_observations;
