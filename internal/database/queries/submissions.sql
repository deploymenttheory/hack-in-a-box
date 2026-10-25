-- name: CompetitionEntry :one
SELECT * FROM competition_entries WHERE event_id=$1 AND team_id=$2;

-- name: SubmissionEntries :many
SELECT e.*,t.state AS team_state,t.name AS team_name FROM competition_entries e JOIN hack_teams t ON t.event_id=e.event_id AND t.id=e.team_id WHERE e.event_id=$1 ORDER BY e.team_id;

-- name: EnsureSubmissionState :exec
INSERT INTO submission_states(event_id,team_id) VALUES($1,$2) ON CONFLICT DO NOTHING;

-- name: SubmissionState :one
SELECT * FROM submission_states WHERE event_id=$1 AND team_id=$2;

-- name: SaveSubmissionState :exec
UPDATE submission_states SET revision=$3,active_revision=$4,outcome=$5,frozen_at=$6 WHERE event_id=$1 AND team_id=$2;

-- name: InsertSubmissionRevision :exec
INSERT INTO submission_revisions(event_id,team_id,revision,actor_id,submitted_at,late,reason,document) VALUES($1,$2,$3,$4,$5,$6,$7,$8);

-- name: SubmissionRevision :one
SELECT * FROM submission_revisions WHERE event_id=$1 AND team_id=$2 AND revision=$3;

-- name: SubmissionRevisions :many
SELECT * FROM submission_revisions WHERE event_id=$1 AND team_id=$2 AND revision < $3 ORDER BY revision DESC LIMIT $4;

-- name: InsertSubmissionChange :exec
INSERT INTO submission_changes(event_id,team_id,revision,actor_id,action,active_revision,review_status,reason,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9);

-- name: SubmissionChanges :many
SELECT * FROM submission_changes WHERE event_id=$1 AND team_id=$2 AND revision < $3 ORDER BY revision DESC LIMIT $4;

-- name: LatestSubmissionReview :one
SELECT review_status FROM submission_changes WHERE event_id=$1 AND team_id=$2 AND active_revision=$3 AND action='reviewed' ORDER BY revision DESC LIMIT 1;

-- name: SubmissionDraft :one
SELECT * FROM submission_drafts WHERE event_id=$1 AND team_id=$2;

-- name: SaveSubmissionDraft :exec
INSERT INTO submission_drafts(event_id,team_id,revision,actor_id,updated_at,evidence) VALUES($1,$2,$3,$4,$5,$6)
ON CONFLICT(event_id,team_id) DO UPDATE SET revision=EXCLUDED.revision,actor_id=EXCLUDED.actor_id,updated_at=EXCLUDED.updated_at,evidence=EXCLUDED.evidence;

-- name: InsertSubmissionFileReference :exec
INSERT INTO submission_file_references(event_id,team_id,submission_revision,file_id,audience) VALUES($1,$2,$3,$4,$5);

-- name: SubmissionFileAudiences :many
SELECT audience,submission_revision FROM submission_file_references WHERE event_id=$1 AND team_id=$2 AND file_id=$3;

-- name: SubmissionMaintenanceEvents :many
SELECT id FROM events WHERE configuration->>'submission_cutoff' IS NOT NULL AND configuration->>'submission_cutoff' != '0001-01-01T00:00:00Z' AND (configuration->>'submission_cutoff')::timestamptz <= $1::timestamptz ORDER BY id;

-- name: SubmissionOrganisers :many
SELECT p.id,p.display_name FROM event_memberships m JOIN people p ON p.id=m.person_id
WHERE m.event_id=$1 AND m.role='organiser' AND m.active AND p.active ORDER BY p.display_name,p.id;

-- name: SubmissionCutoffOutcome :one
SELECT * FROM submission_changes WHERE event_id=$1 AND team_id=$2
AND actor_id IS NULL AND action IN ('frozen','not_submitted','withdrawn') AND occurred_at >= $3
ORDER BY revision LIMIT 1;
