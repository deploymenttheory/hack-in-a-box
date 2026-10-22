-- name: TeamCredentials :many
SELECT * FROM team_credentials WHERE event_id=$1 ORDER BY id LIMIT 10000;

-- name: TeamCredential :one
SELECT * FROM team_credentials WHERE event_id=$1 AND id=$2;

-- name: SaveTeamCredential :exec
INSERT INTO team_credentials(id,event_id,label,version,key_id,ciphertext,active) VALUES($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT(id) DO UPDATE SET label=EXCLUDED.label,version=EXCLUDED.version,key_id=EXCLUDED.key_id,ciphertext=EXCLUDED.ciphertext,active=EXCLUDED.active,updated_at=now();

-- name: CredentialAssignments :many
SELECT * FROM team_credential_assignments WHERE event_id=$1 ORDER BY credential_id,team_id;

-- name: ClearCredentialAssignments :exec
DELETE FROM team_credential_assignments WHERE event_id=$1 AND credential_id=$2;

-- name: AssignTeamCredential :exec
INSERT INTO team_credential_assignments(event_id,credential_id,team_id) VALUES($1,$2,$3);

-- name: CredentialTeamAuthority :one
SELECT EXISTS(SELECT 1 FROM hack_team_members m
 JOIN hack_teams t ON t.event_id=m.event_id AND t.id=m.team_id AND t.state='published'
 JOIN competition_entries e ON e.event_id=m.event_id AND e.team_id=m.team_id AND e.active
 JOIN competition_claims c ON c.event_id=m.event_id AND c.person_id=m.person_id AND c.state='confirmed'
 JOIN people p ON p.id=m.person_id AND p.active
 WHERE m.event_id=$1 AND m.person_id=$2 AND m.snapshot='published' AND m.team_id=$3)::boolean AS allowed;
