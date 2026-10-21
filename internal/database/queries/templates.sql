-- name: CanCreateEvent :one
SELECT EXISTS(SELECT 1 FROM people p WHERE p.id=$1 AND p.active AND (p.platform_admin OR EXISTS(
SELECT 1 FROM event_memberships m WHERE m.person_id=p.id AND m.role='organiser' AND m.active)));

-- name: CreateTemplate :one
INSERT INTO templates(id,owner_id,name) VALUES($1,$2,$3) RETURNING *;

-- name: Template :one
SELECT * FROM templates WHERE id=$1;

-- name: LockTemplate :one
SELECT * FROM templates WHERE id=$1 FOR UPDATE;

-- name: ListTemplates :many
SELECT * FROM templates WHERE owner_id=$1 AND id>$2 ORDER BY id LIMIT $3;

-- name: NewTemplateVersion :exec
INSERT INTO template_versions(template_id,version,configuration) VALUES($1,$2,$3);

-- name: EditTemplate :one
UPDATE templates SET name=$2,latest_version=latest_version+1,revision=revision+1,updated_at=now() WHERE id=$1 RETURNING *;

-- name: TemplateVersion :one
SELECT * FROM template_versions WHERE template_id=$1 AND version=$2;

-- name: RecordTemplatePublication :exec
INSERT INTO template_publications(template_id,version) VALUES($1,$2) ON CONFLICT DO NOTHING;

-- name: PublishTemplate :one
UPDATE templates SET published_version=$2,revision=revision+1,updated_at=now() WHERE id=$1 RETURNING *;

-- name: TemplatePublished :one
SELECT EXISTS(SELECT 1 FROM template_publications WHERE template_id=$1 AND version=$2);

-- name: EventDefinitions :one
SELECT * FROM event_definitions WHERE event_id=$1;

-- name: SaveEventDefinitions :exec
INSERT INTO event_definitions(event_id,definitions,source_template_id,source_version,clone_key) VALUES($1,$2,$3,$4,$5)
ON CONFLICT(event_id) DO UPDATE SET definitions=EXCLUDED.definitions;

-- name: LockCreationID :exec
SELECT pg_advisory_xact_lock(hashtextextended($1::text,0));
