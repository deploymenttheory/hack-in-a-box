-- name: CommunicationCampaigns :many
SELECT DISTINCT ON (id) * FROM communication_campaign_versions WHERE event_id=$1 ORDER BY id,version DESC LIMIT 1001;

-- name: CommunicationCampaign :one
SELECT * FROM communication_campaign_versions WHERE event_id=$1 AND id=$2 ORDER BY version DESC LIMIT 1;

-- name: SaveCommunicationCampaign :exec
INSERT INTO communication_campaign_versions(event_id,id,version,document,actor_id,reason) VALUES($1,$2,$3,$4,$5,$6);

-- name: SaveCommunicationTestRequest :exec
INSERT INTO communication_test_requests(event_id,id,campaign_id,campaign_version,actor_id,channel,snapshot,fingerprint,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9);

-- name: CommunicationTestRequest :one
SELECT * FROM communication_test_requests WHERE event_id=$1 AND id=$2;

-- name: CancelCommunicationCampaignTests :exec
UPDATE operations o SET cancelled=true,approved=false,revision=revision+1,updated_at=now()
WHERE o.event_id=$1 AND o.target_id IN (SELECT id FROM communication_test_requests WHERE communication_test_requests.event_id=$1 AND communication_test_requests.campaign_id=$2)
AND o.state IN ('pending','running') AND NOT o.cancelled;

-- name: CommunicationCurrentOrganisers :many
SELECT m.person_id FROM event_memberships m JOIN people p ON p.id=m.person_id
WHERE m.event_id=$1 AND m.role='organiser' AND m.active AND p.active ORDER BY m.person_id;

-- name: CommunicationPreviewApprovers :many
SELECT a.preview_id,a.actor_id FROM communication_approvals a
JOIN (SELECT DISTINCT ON (cv.id) cv.document FROM communication_campaign_versions cv WHERE cv.event_id=$1 ORDER BY cv.id,cv.version DESC LIMIT 1001) c
ON a.preview_id=(c.document->>'preview_id')::uuid
WHERE a.event_id=$1 ORDER BY a.preview_id;
