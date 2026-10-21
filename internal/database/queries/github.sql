-- name: LockIdentity :one
SELECT * FROM people WHERE id=$1 FOR UPDATE;

-- name: EnsureGitHubLinkState :exec
INSERT INTO github_link_states(person_id) VALUES($1) ON CONFLICT DO NOTHING;

-- name: GitHubLinkRevision :one
SELECT COALESCE((SELECT revision FROM github_link_states WHERE person_id=$1),0)::bigint;

-- name: AdvanceGitHubLink :exec
UPDATE github_link_states SET revision=revision+1 WHERE person_id=$1;

-- name: GitHubAccount :one
SELECT * FROM github_accounts WHERE person_id=$1;

-- name: SaveGitHubAccount :one
INSERT INTO github_accounts(person_id,github_id,login,token_encrypted) VALUES($1,$2,$3,$4)
ON CONFLICT(person_id) DO UPDATE SET github_id=EXCLUDED.github_id,login=EXCLUDED.login,token_encrypted=EXCLUDED.token_encrypted,verified_at=now() RETURNING *;

-- name: RemoveGitHubAccount :exec
DELETE FROM github_accounts WHERE person_id=$1;

-- name: PutGitHubLinkAttempt :exec
INSERT INTO github_link_attempts(state_hash,person_id,session_hash,verifier_encrypted,link_revision,session_revision,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7);

-- name: ConsumeGitHubLinkAttempt :one
DELETE FROM github_link_attempts WHERE state_hash=$1 AND person_id=$2 AND session_hash=$3 AND expires_at>now() RETURNING *;

-- name: ExpireGitHubLinkAttempts :exec
DELETE FROM github_link_attempts WHERE expires_at<=now();

-- name: ClearGitHubLinkAttempts :exec
DELETE FROM github_link_attempts WHERE person_id=$1;

-- name: FlagGitHubInvitations :execrows
UPDATE github_invitation_bindings SET state='requires_review',updated_at=now() WHERE person_id=$1 AND state='pending';

-- name: GitHubInvitationsNeedingReview :one
SELECT count(*) FROM github_invitation_bindings WHERE person_id=$1 AND state='requires_review';

-- name: RefreshGitHubInvitationBindings :exec
UPDATE github_invitation_bindings SET link_revision=$2,updated_at=now() WHERE person_id=$1 AND github_id=$3 AND state='pending';
