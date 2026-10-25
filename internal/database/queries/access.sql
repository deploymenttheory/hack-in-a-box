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
WITH scoped_team AS (
 SELECT ht.id FROM hack_teams ht WHERE ht.event_id=$1 AND ht.id=$2
 UNION SELECT ce.team_id FROM competition_entries ce WHERE ce.event_id=$1 AND ce.id=$2
)
SELECT EXISTS(SELECT 1 WHERE (
 EXISTS(SELECT 1 FROM resource_assignments ra WHERE ra.event_id=$1 AND ra.resource_id=$2 AND ra.person_id=$3)
 OR EXISTS(
  SELECT 1 FROM mentor_assignments a
  JOIN hack_teams t ON t.event_id=a.event_id AND t.id=a.team_id AND t.state='published'
  JOIN event_memberships r ON r.event_id=a.event_id AND r.person_id=a.person_id AND r.role='mentor' AND r.active
  JOIN people p ON p.id=a.person_id AND p.active
  LEFT JOIN competition_entries e ON e.event_id=a.event_id AND e.team_id=a.team_id AND e.active
  WHERE a.event_id=$1 AND a.person_id=$3 AND a.snapshot='published'
  AND ((a.slot='primary' AND a.team_id=$2) OR (a.slot='second' AND e.id=$2))
 )
) AND NOT EXISTS(
 SELECT 1 FROM scoped_team st JOIN hack_team_members m ON m.event_id=$1 AND m.team_id=st.id
 WHERE m.person_id=$3 OR EXISTS(
  SELECT 1 FROM mentor_conflicts c WHERE c.event_id=$1 AND c.mentor_id=$3 AND c.competitor_id=m.person_id
 )
));

-- name: VisibleEvent :one
SELECT EXISTS(SELECT 1 FROM event_memberships m WHERE m.event_id=$1 AND m.person_id=$2 AND m.active)
OR EXISTS(SELECT 1 FROM competition_claims c WHERE c.event_id=$1 AND c.person_id=$2 AND c.state='confirmed');

-- name: CurrentJudgeAssignment :one
SELECT COALESCE(NOT EXISTS(SELECT 1 FROM judge_rounds r WHERE r.event_id=$1)
OR EXISTS(SELECT 1 FROM judge_assignments a WHERE a.event_id=$1 AND a.team_id=$2 AND a.person_id=$3 AND a.active),false)::boolean AS allowed;
