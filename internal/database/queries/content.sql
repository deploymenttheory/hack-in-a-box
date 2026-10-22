-- name: ContentItem :one
SELECT * FROM content_items WHERE event_id=$1 AND id=$2;

-- name: CreateContent :exec
INSERT INTO content_items(event_id,id,kind) VALUES($1,$2,$3);

-- name: SaveContentState :one
UPDATE content_items SET revision=$3,version=$4,state=$5,released_version=$6,scheduled_at=$7,approved_by=$8,approved_revision=$9,released_at=$10
WHERE event_id=$1 AND id=$2 RETURNING *;

-- name: InsertContentVersion :exec
INSERT INTO content_versions(event_id,content_id,version,document,actor_id,created_at) VALUES($1,$2,$3,$4,$5,$6);

-- name: ContentVersion :one
SELECT * FROM content_versions WHERE event_id=$1 AND content_id=$2 AND version=$3;

-- name: ContentItems :many
SELECT * FROM content_items WHERE event_id=$1 AND id>sqlc.arg(cursor)::uuid ORDER BY id LIMIT sqlc.arg(page_size);

-- name: ContentFileItems :many
SELECT DISTINCT c.* FROM content_items c JOIN content_files f ON f.event_id=c.event_id AND f.content_id=c.id WHERE f.event_id=$1 AND f.file_id=$2;

-- name: ContentFileReferenced :one
SELECT EXISTS(SELECT 1 FROM content_files WHERE event_id=$1 AND content_id=$2 AND version=$3 AND file_id=$4);

-- name: AddContentFile :exec
INSERT INTO content_files(event_id,content_id,version,file_id) VALUES($1,$2,$3,$4);

-- name: RecordContentChange :exec
INSERT INTO content_changes(event_id,content_id,revision,version,action,actor_id,reason,scheduled_at,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9);

-- name: ContentChanges :many
SELECT * FROM content_changes WHERE event_id=$1 AND content_id=$2 AND revision<sqlc.arg(before)::bigint ORDER BY revision DESC LIMIT sqlc.arg(page_size);

-- name: DueContent :many
SELECT * FROM content_items WHERE state='approved' AND scheduled_at<=$1 ORDER BY CASE WHEN kind='challenge' THEN 0 ELSE 1 END,scheduled_at,event_id,id LIMIT 200;

-- name: ContentNotices :many
SELECT * FROM event_notices WHERE event_id=$1 AND revision>sqlc.arg(after)::bigint ORDER BY revision LIMIT 100;

-- name: PublishChallenge :exec
UPDATE events SET published_at=$2,revision=revision+1,updated_at=$2 WHERE id=$1 AND published_at IS NULL;

-- name: ContentTeamMember :one
SELECT EXISTS(SELECT 1 FROM hack_team_members m JOIN hack_teams t ON t.event_id=m.event_id AND t.id=m.team_id WHERE m.event_id=$1 AND m.person_id=$2 AND m.snapshot='published' AND t.state!='disbanded' AND m.team_id=ANY(sqlc.arg(team_ids)::uuid[]));

-- name: InvalidateChallengeApproval :exec
WITH changed AS (
 UPDATE content_items SET state='draft',approved_by=NULL,approved_revision=NULL,revision=revision+1
 WHERE event_id=sqlc.arg(event_id)::uuid AND kind='challenge' AND state='approved'
 AND scheduled_at IS DISTINCT FROM sqlc.narg(due_at)::timestamptz
 RETURNING *
)
INSERT INTO content_changes(event_id,content_id,revision,version,action,actor_id,scheduled_at,occurred_at)
SELECT event_id,id,revision,version,'schedule_invalidated',sqlc.arg(actor_id)::uuid,scheduled_at,sqlc.arg(changed_at)::timestamptz FROM changed;

-- name: ContentAcknowledgement :one
SELECT acknowledged_at FROM content_acknowledgements WHERE event_id=$1 AND content_id=$2 AND version=$3 AND person_id=$4;

-- name: AcknowledgeContent :execrows
INSERT INTO content_acknowledgements(event_id,content_id,version,person_id,acknowledged_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING;
