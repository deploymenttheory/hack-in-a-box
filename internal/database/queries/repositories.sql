-- name: TeamRepository :one
SELECT * FROM team_repositories WHERE event_id=$1 AND team_id=$2;

-- name: RepositoryByGitHubID :one
SELECT * FROM team_repositories WHERE github_repository_id=$1;

-- name: EventRepositories :many
SELECT r.*,t.state AS team_state FROM team_repositories r JOIN hack_teams t ON t.event_id=r.event_id AND t.id=r.team_id WHERE r.event_id=$1 ORDER BY r.team_id;

-- name: InsertTeamRepository :exec
INSERT INTO team_repositories(event_id,team_id,organization_id,organization,name,app_id,installation_id,template_repository,requested_by)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(event_id,team_id) DO UPDATE SET requested_by=EXCLUDED.requested_by;

-- name: MarkRepositoryAttempt :execrows
UPDATE team_repositories SET attempted=true,template_repository_id=$3,state='ambiguous',diagnostic='create_uncertain',updated_at=now() WHERE event_id=$1 AND team_id=$2 AND NOT attempted;

-- name: SetRepositoryResult :execrows
UPDATE team_repositories SET github_repository_id=COALESCE(github_repository_id,sqlc.narg(github_repository_id)::bigint),state=$3,diagnostic=$4,updated_at=now()
WHERE event_id=$1 AND team_id=$2;

-- name: ProvisioningEvents :many
SELECT DISTINCT e.* FROM events e JOIN hack_teams t ON t.event_id=e.id JOIN github_app_integrations a ON a.event_id=e.id
WHERE e.writes_allowed AND e.stage NOT IN ('cancelled','completed','archived') AND t.state='published' AND a.installation_id>0 ORDER BY e.id;

-- name: DBPublishedTeam :one
SELECT t.id FROM hack_teams t JOIN competition_entries e ON e.event_id=t.event_id AND e.team_id=t.id
WHERE t.event_id=$1 AND t.id=sqlc.arg(team_id)::uuid AND t.state='published' AND e.active
AND EXISTS(SELECT 1 FROM hack_team_members m WHERE m.event_id=t.event_id AND m.team_id=t.id AND m.snapshot='published');

-- name: ResolveRepositoryOperation :exec
UPDATE operations SET state='delivered',updated_at=now()
WHERE event_id=$1 AND target_id=$2 AND kind='repository.provision' AND state IN ('ambiguous','permanent') AND NOT cancelled;

-- name: AuthorizeRepositoryRecovery :exec
UPDATE team_repositories SET requested_by=$3 WHERE event_id=$1 AND team_id=$2;
