-- name: Memberships :many
SELECT m.*, p.display_name FROM event_memberships m JOIN people p ON p.id=m.person_id
WHERE m.event_id=$1 ORDER BY p.display_name,m.person_id,m.role;

-- name: ActiveRoles :many
SELECT role FROM event_memberships WHERE event_id=$1 AND person_id=$2 AND active ORDER BY role;

-- name: SetMembership :exec
INSERT INTO event_memberships(event_id,person_id,role,active) VALUES ($1,$2,$3,$4)
ON CONFLICT(event_id,person_id,role) DO UPDATE SET active=EXCLUDED.active,changed_at=now();

-- name: MembershipState :one
SELECT active FROM event_memberships WHERE event_id=$1 AND person_id=$2 AND role=$3;

-- name: ActiveOrganisers :one
SELECT count(*) FROM event_memberships m JOIN people p ON p.id=m.person_id
WHERE m.event_id=$1 AND m.role='organiser' AND m.active AND p.active;

-- name: ExcludeCompetition :exec
INSERT INTO competition_exclusions(event_id,person_id,source) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING;

-- name: CompetitionExcluded :one
SELECT EXISTS(SELECT 1 FROM competition_exclusions WHERE event_id=$1 AND person_id=$2);

-- name: CompetitionState :one
SELECT COALESCE((SELECT state FROM competition_claims WHERE event_id=$1 AND person_id=$2),'')::text;

-- name: SetCompetitionState :exec
INSERT INTO competition_claims(event_id,person_id,state) VALUES ($1,$2,$3)
ON CONFLICT(event_id,person_id) DO UPDATE SET state=EXCLUDED.state;

-- name: SetEventOwner :exec
UPDATE events SET owner_id=$2 WHERE id=$1;

-- name: BumpEventRevision :one
UPDATE events SET revision=revision+1,updated_at=now() WHERE id=$1 RETURNING *;

-- name: PolicyResource :one
SELECT * FROM policy_resources WHERE event_id=$1 AND id=$2;

-- name: ResourceAssigned :one
SELECT EXISTS(SELECT 1 FROM resource_assignments WHERE event_id=$1 AND resource_id=$2 AND person_id=$3);

-- name: VisibleEvent :one
SELECT EXISTS(SELECT 1 FROM event_memberships m WHERE m.event_id=$1 AND m.person_id=$2 AND m.active)
OR EXISTS(SELECT 1 FROM competition_claims c WHERE c.event_id=$1 AND c.person_id=$2 AND c.state='confirmed');
