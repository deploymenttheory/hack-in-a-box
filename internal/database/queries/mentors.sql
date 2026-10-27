-- name: MentorDefaultCapacity :one
SELECT COALESCE((SELECT default_capacity FROM mentor_settings WHERE event_id=$1),2)::integer;

-- name: SetMentorDefaultCapacity :exec
INSERT INTO mentor_settings(event_id,default_capacity) VALUES($1,$2) ON CONFLICT(event_id) DO UPDATE SET default_capacity=EXCLUDED.default_capacity;

-- name: MentorProfiles :many
SELECT p.id AS person_id,p.display_name,m.active AND p.active AS active,
COALESCE(v.skills,'{}'::text[])::text[] AS skills,COALESCE(v.availability,'[]'::jsonb)::jsonb AS availability,v.maximum_teams
FROM event_memberships m JOIN people p ON p.id=m.person_id LEFT JOIN mentor_profiles v ON v.event_id=m.event_id AND v.person_id=m.person_id
WHERE m.event_id=$1 AND m.role='mentor' ORDER BY p.id;

-- name: SetMentorProfile :exec
INSERT INTO mentor_profiles(event_id,person_id,skills,availability,maximum_teams) VALUES($1,$2,$3,$4,$5)
ON CONFLICT(event_id,person_id) DO UPDATE SET skills=EXCLUDED.skills,availability=EXCLUDED.availability,maximum_teams=EXCLUDED.maximum_teams;

-- name: MentorConflicts :many
SELECT * FROM mentor_conflicts WHERE event_id=$1 ORDER BY mentor_id,competitor_id,kind;

-- name: MentorPanelTeamAuthority :one
SELECT EXISTS (
 SELECT 1 FROM hack_teams t
 WHERE t.event_id=sqlc.arg(event_id) AND t.id=sqlc.arg(team_id) AND t.state='published'
 AND NOT EXISTS (
  SELECT 1 FROM hack_team_members m
  WHERE m.event_id=t.event_id AND m.team_id=t.id
  AND (m.person_id=sqlc.arg(person_id) OR EXISTS (
   SELECT 1 FROM mentor_conflicts c
   WHERE c.event_id=m.event_id AND c.mentor_id=sqlc.arg(person_id) AND c.competitor_id=m.person_id
  ))
 )
) AS eligible,
EXISTS (
 SELECT 1 FROM mentor_assignments a
 WHERE a.event_id=sqlc.arg(event_id) AND a.team_id=sqlc.arg(team_id)
 AND a.person_id=sqlc.arg(person_id) AND a.snapshot='published'
) AS assigned;

-- name: SetMentorConflict :exec
INSERT INTO mentor_conflicts(event_id,mentor_id,competitor_id,kind) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING;

-- name: DeleteMentorConflict :exec
DELETE FROM mentor_conflicts WHERE event_id=$1 AND mentor_id=$2 AND competitor_id=$3 AND kind=$4;

-- name: MentorTeamSkills :many
SELECT * FROM mentor_team_skills WHERE event_id=$1 ORDER BY team_id;

-- name: SetMentorTeamSkills :exec
INSERT INTO mentor_team_skills(event_id,team_id,skills) VALUES($1,$2,$3) ON CONFLICT(event_id,team_id) DO UPDATE SET skills=EXCLUDED.skills;

-- name: MentorAssignments :many
SELECT * FROM mentor_assignments WHERE event_id=$1 ORDER BY snapshot,team_id,slot;

-- name: SetMentorAssignment :exec
INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at) VALUES($1,$2,$3,$4,'draft',$5,$6)
ON CONFLICT(event_id,team_id,slot,snapshot) DO UPDATE SET person_id=EXCLUDED.person_id,starts_at=EXCLUDED.starts_at,ends_at=EXCLUDED.ends_at;

-- name: DeleteMentorAssignment :exec
DELETE FROM mentor_assignments WHERE event_id=$1 AND team_id=$2 AND slot=$3 AND snapshot='draft';

-- name: ClearMentorDraft :exec
DELETE FROM mentor_assignments WHERE event_id=$1 AND snapshot='draft';

-- name: DeletePublishedMentors :exec
DELETE FROM mentor_assignments WHERE event_id=$1 AND snapshot='published';

-- name: PublishMentors :exec
INSERT INTO mentor_assignments(event_id,team_id,person_id,slot,snapshot,starts_at,ends_at)
SELECT a.event_id,a.team_id,a.person_id,a.slot,'published',a.starts_at,a.ends_at FROM mentor_assignments a JOIN hack_teams t ON t.event_id=a.event_id AND t.id=a.team_id
WHERE a.event_id=$1 AND a.snapshot='draft' AND t.state='published';
