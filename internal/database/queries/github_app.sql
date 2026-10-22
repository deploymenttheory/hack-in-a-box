-- name: GitHubAppIntegration :one
SELECT * FROM github_app_integrations WHERE event_id=$1;

-- name: SaveGitHubAppIntegration :exec
INSERT INTO github_app_integrations(event_id,app_id,app_slug,organization_id,organization,template_repository,credential_version,key_id,private_key_encrypted,installation_id)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT(event_id) DO UPDATE SET app_id=EXCLUDED.app_id,app_slug=EXCLUDED.app_slug,organization_id=EXCLUDED.organization_id,organization=EXCLUDED.organization,template_repository=EXCLUDED.template_repository,credential_version=EXCLUDED.credential_version,key_id=EXCLUDED.key_id,private_key_encrypted=EXCLUDED.private_key_encrypted,installation_id=EXCLUDED.installation_id,updated_at=now();

-- name: SetGitHubAppInstallation :exec
UPDATE github_app_integrations SET installation_id=$2,updated_at=now() WHERE event_id=$1;

-- name: DeleteGitHubAppIntegration :exec
DELETE FROM github_app_integrations WHERE event_id=$1;

-- name: ClearGitHubAppAttempts :exec
DELETE FROM github_app_install_attempts WHERE event_id=$1 OR expires_at<=now();

-- name: PutGitHubAppAttempt :exec
INSERT INTO github_app_install_attempts(state_hash,event_id,actor_id,session_hash,session_revision,event_revision,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7);

-- name: ConsumeGitHubAppAttempt :one
DELETE FROM github_app_install_attempts WHERE state_hash=$1 AND actor_id=$2 AND session_hash=$3 AND expires_at>now() RETURNING *;
