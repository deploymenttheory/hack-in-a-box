-- name: TeamRepository :one
SELECT * FROM team_repositories WHERE event_id=$1 AND team_id=$2;

-- name: RepositoryByGitHubID :one
SELECT * FROM team_repositories WHERE github_repository_id=$1;

-- name: EventRepositories :many
SELECT r.*,t.state AS team_state FROM team_repositories r JOIN hack_teams t ON t.event_id=r.event_id AND t.id=r.team_id WHERE r.event_id=$1 ORDER BY r.team_id;

-- name: InsertTeamRepository :exec
INSERT INTO team_repositories(event_id,team_id,organization_id,organization,name,app_id,installation_id,template_repository)
VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(event_id,team_id) DO NOTHING;
