-- name: CommunicationVersions :many
SELECT DISTINCT ON (id,kind) * FROM communication_versions WHERE event_id=$1 ORDER BY id,kind,version DESC LIMIT 1001;

-- name: CommunicationVersion :one
SELECT * FROM communication_versions WHERE event_id=$1 AND id=$2 AND kind=$3 ORDER BY version DESC LIMIT 1;

-- name: SaveCommunicationVersion :exec
INSERT INTO communication_versions(event_id,id,kind,version,document,actor_id) VALUES($1,$2,$3,$4,$5,$6);

-- name: SaveCommunicationPreview :exec
INSERT INTO communication_previews(event_id,id,template_id,template_version,audience_id,audience_version,actor_id,snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,$8);

-- name: CommunicationPreview :one
SELECT * FROM communication_previews WHERE event_id=$1 AND id=$2;

-- name: ApproveCommunication :exec
INSERT INTO communication_approvals(event_id,preview_id,actor_id,reason) VALUES($1,$2,$3,$4);

-- name: CommunicationApproval :one
SELECT * FROM communication_approvals WHERE event_id=$1 AND preview_id=$2;

-- name: CommunicationPeople :many
SELECT p.id,p.display_name,COALESCE(c.state,'')::text AS competition_state,
 ARRAY(SELECT m.role FROM event_memberships m WHERE m.event_id=$1 AND m.person_id=p.id AND m.active ORDER BY m.role)::text[] AS roles,
 ARRAY(SELECT r.state FROM registrations r WHERE r.event_id=$1 AND r.person_id=p.id AND r.state NOT IN ('withdrawn','declined') ORDER BY r.state)::text[] AS states,
 COALESCE((SELECT t.team_id FROM hack_team_members t JOIN hack_teams h ON h.event_id=t.event_id AND h.id=t.team_id AND h.state='published' JOIN competition_entries e ON e.event_id=t.event_id AND e.team_id=t.team_id AND e.active WHERE t.event_id=$1 AND t.person_id=p.id AND t.snapshot='published'), '00000000-0000-0000-0000-000000000000'::uuid)::uuid AS team_id,
 EXISTS(SELECT 1 FROM registration_offers o JOIN registrations r ON r.event_id=o.event_id AND r.id=o.registration_id WHERE o.event_id=$1 AND r.person_id=p.id AND r.state='offered' AND o.state='pending' AND o.expires_at>sqlc.arg(at)::timestamptz) AS offer_pending
FROM people p LEFT JOIN competition_claims c ON c.event_id=$1 AND c.person_id=p.id
WHERE p.active AND (
 c.state='confirmed' OR EXISTS(SELECT 1 FROM event_memberships m WHERE m.event_id=$1 AND m.person_id=p.id AND m.active)
 OR EXISTS(SELECT 1 FROM registrations r WHERE r.event_id=$1 AND r.person_id=p.id AND r.state NOT IN ('withdrawn','declined'))
) ORDER BY p.id LIMIT 10001;

-- name: CommunicationFacts :many
SELECT * FROM communication_facts WHERE event_id=$1 ORDER BY kind,id;

-- name: SaveCommunicationFact :one
INSERT INTO communication_facts(event_id,id,kind,revision,owner_id,resource_id,status) VALUES($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT(event_id,id) DO UPDATE SET revision=EXCLUDED.revision,owner_id=EXCLUDED.owner_id,resource_id=EXCLUDED.resource_id,status=EXCLUDED.status WHERE communication_facts.kind=EXCLUDED.kind AND communication_facts.revision<EXCLUDED.revision RETURNING *;

-- name: CommunicationTeam :one
SELECT name FROM hack_teams WHERE event_id=$1 AND id=$2 AND state='published';

-- name: CommunicationOutstandingTasks :one
SELECT count(*) FROM preparation_tasks WHERE event_id=$1 AND document->'definition'->>'status' NOT IN ('complete','waived') AND (document->'definition'->>'owner_id'=$2::text OR (document->'definition'->>'owner_id' IS NULL AND document->'definition'->>'owner_role'=ANY(sqlc.arg(roles)::text[])));
