-- name: ObservationMentorAssigned :one
SELECT EXISTS(
 SELECT 1 FROM mentor_assignments a
 JOIN hack_teams t ON t.event_id=a.event_id AND t.id=a.team_id AND t.state='published'
 JOIN event_memberships r ON r.event_id=a.event_id AND r.person_id=a.person_id AND r.role='mentor' AND r.active
 JOIN people p ON p.id=a.person_id AND p.active
 WHERE a.event_id=$1 AND a.team_id=$2 AND a.person_id=$3 AND a.snapshot='published'
 AND NOT EXISTS(
  SELECT 1 FROM hack_team_members m WHERE m.event_id=a.event_id AND m.team_id=a.team_id AND m.snapshot='published'
  AND (m.person_id=a.person_id OR EXISTS(SELECT 1 FROM mentor_conflicts c WHERE c.event_id=m.event_id AND c.mentor_id=a.person_id AND c.competitor_id=m.person_id))
 )
);

-- name: CurrentMentorObservations :many
SELECT DISTINCT ON(id) * FROM mentor_observation_versions WHERE event_id=$1 AND team_id=$2
AND (sqlc.narg(after_id)::uuid IS NULL OR id>sqlc.narg(after_id)::uuid)
ORDER BY id,version DESC LIMIT sqlc.arg(page_size)::integer;

-- name: LatestMentorObservation :one
SELECT * FROM mentor_observation_versions WHERE event_id=$1 AND team_id=$2 AND id=$3 ORDER BY version DESC LIMIT 1;

-- name: MentorObservationHistory :many
SELECT * FROM mentor_observation_versions WHERE event_id=$1 AND team_id=$2 AND id=$3
AND (sqlc.narg(before_version)::bigint IS NULL OR version<sqlc.narg(before_version)::bigint)
ORDER BY version DESC LIMIT sqlc.arg(page_size)::integer;

-- name: InsertMentorObservation :one
INSERT INTO mentor_observation_versions(event_id,team_id,id,version,author_id,observed_at,changed_at,category,evidence,outcome,feedback_reference)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING *;

-- name: MentorStatusHistory :many
SELECT * FROM mentor_team_status_versions WHERE event_id=$1 AND team_id=$2
AND (sqlc.narg(before_version)::bigint IS NULL OR version<sqlc.narg(before_version)::bigint)
ORDER BY version DESC LIMIT sqlc.arg(page_size)::integer;

-- name: CurrentMentorStatus :one
SELECT * FROM mentor_team_status_versions WHERE event_id=$1 AND team_id=$2 ORDER BY version DESC LIMIT 1;

-- name: InsertMentorStatus :one
INSERT INTO mentor_team_status_versions(event_id,team_id,version,actor_id,state,reason,next_action,changed_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING *;
