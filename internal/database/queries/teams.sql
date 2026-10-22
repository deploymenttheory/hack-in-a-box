-- name: HackTeams :many
SELECT * FROM hack_teams WHERE event_id=$1 ORDER BY name,id LIMIT 10000;

-- name: HackTeam :one
SELECT * FROM hack_teams WHERE event_id=$1 AND id=$2;

-- name: TeamByInvitation :one
SELECT * FROM hack_teams WHERE event_id=$1 AND invitation_code=$2 AND state!='disbanded';

-- name: CreateHackTeam :one
INSERT INTO hack_teams(id,event_id,name,captain_id) VALUES($1,$2,$3,$4) RETURNING *;

-- name: CreateCompetitionEntry :exec
WITH entry AS (INSERT INTO competition_entries(event_id,team_id) VALUES($1,$2) ON CONFLICT(event_id,team_id) DO UPDATE SET team_id=EXCLUDED.team_id RETURNING *)
INSERT INTO policy_resources(event_id,id,kind) SELECT event_id,team_id,'team' FROM entry UNION ALL SELECT event_id,id,'assessment' FROM entry ON CONFLICT DO NOTHING;

-- name: HackTeamMembers :many
SELECT m.*,p.display_name FROM hack_team_members m JOIN people p ON p.id=m.person_id WHERE m.event_id=$1 ORDER BY m.team_id,m.person_id,m.snapshot;

-- name: TeamAssignment :one
SELECT * FROM hack_team_members WHERE event_id=$1 AND person_id=$2 AND snapshot=$3;

-- name: SetDraftTeamMember :exec
INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) VALUES($1,$2,$3,'draft')
ON CONFLICT(event_id,person_id,snapshot) DO UPDATE SET team_id=EXCLUDED.team_id;

-- name: RemoveTeamMember :exec
DELETE FROM hack_team_members WHERE event_id=$1 AND person_id=$2;

-- name: ClearDepartingCaptain :exec
UPDATE hack_teams SET captain_id=CASE WHEN captain_id=$2 THEN NULL ELSE captain_id END,published_captain_id=CASE WHEN published_captain_id=$2 THEN NULL ELSE published_captain_id END WHERE event_id=$1;

-- name: SetTeamCaptain :exec
UPDATE hack_teams SET captain_id=$3,published_captain_id=CASE WHEN frozen THEN $3 ELSE published_captain_id END WHERE event_id=$1 AND id=$2;

-- name: DisbandHackTeam :exec
UPDATE hack_teams SET state='disbanded',captain_id=NULL,published_captain_id=NULL WHERE event_id=$1 AND id=$2;

-- name: RetireCompetitionEntry :exec
UPDATE competition_entries SET active=false WHERE event_id=$1 AND team_id=$2;

-- name: DeletePublishedTeamMembers :exec
DELETE FROM hack_team_members WHERE event_id=$1 AND snapshot='published';

-- name: CopyPublishedTeamMembers :exec
INSERT INTO hack_team_members(event_id,team_id,person_id,snapshot) SELECT m.event_id,m.team_id,m.person_id,'published' FROM hack_team_members m WHERE m.event_id=$1 AND m.snapshot='draft';

-- name: PublishHackTeams :exec
UPDATE hack_teams SET state='published',published_captain_id=captain_id WHERE event_id=$1 AND state!='disbanded';

-- name: FreezeHackTeams :exec
UPDATE hack_teams SET frozen=true WHERE event_id=$1 AND state='published';

-- name: NewTeamRequest :exec
INSERT INTO hack_team_requests(event_id,team_id,person_id) VALUES($1,$2,$3);

-- name: PendingTeamRequest :one
SELECT * FROM hack_team_requests WHERE event_id=$1 AND person_id=$2 AND state='pending';

-- name: TeamRequest :one
SELECT * FROM hack_team_requests WHERE event_id=$1 AND id=$2;

-- name: ResolveTeamRequest :exec
UPDATE hack_team_requests SET state=$3 WHERE event_id=$1 AND id=$2;

-- name: PendingTeamRequests :many
SELECT r.*,p.display_name,COALESCE(c.state,'') AS registration_state FROM hack_team_requests r JOIN people p ON p.id=r.person_id
LEFT JOIN competition_claims c ON c.event_id=r.event_id AND c.person_id=r.person_id
WHERE r.event_id=$1 AND r.state='pending' ORDER BY r.created_at,r.id;

-- name: DeclinePersonTeamRequests :exec
UPDATE hack_team_requests SET state='declined' WHERE event_id=$1 AND person_id=$2 AND state='pending';

-- name: ConfirmedTeamPeople :many
SELECT p.id,p.display_name,COALESCE(r.team_preference,'solo')::text AS team_preference FROM competition_claims c JOIN people p ON p.id=c.person_id LEFT JOIN registrations r ON r.event_id=c.event_id AND r.person_id=c.person_id AND r.kind='attendee' AND r.state='confirmed'
WHERE c.event_id=$1 AND c.state='confirmed' AND p.active ORDER BY p.display_name,p.id LIMIT 10000;

-- name: CloseTeamRequests :exec
UPDATE hack_team_requests SET state='declined' WHERE event_id=$1 AND state='pending';
