-- name: ClarificationQuestion :one
SELECT * FROM clarification_questions WHERE event_id=$1 AND id=$2;

-- name: ClarificationQuestionCommand :one
SELECT * FROM clarification_questions WHERE event_id=$1 AND author_id=$2 AND command_id=$3;

-- name: InsertClarificationQuestion :one
INSERT INTO clarification_questions(event_id,id,team_id,author_id,command_id,request_digest,topic,body,attachments,source,revision,status,created_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,1,'open',$11) RETURNING *;

-- name: ClarificationTeamQuestions :many
SELECT * FROM clarification_questions WHERE event_id=$1 AND team_id=$2 AND id>sqlc.arg(cursor)::uuid ORDER BY id LIMIT sqlc.arg(page_size);

-- name: ClarificationStaffQuestions :many
SELECT * FROM clarification_questions WHERE event_id=$1 AND id>sqlc.arg(cursor)::uuid ORDER BY id LIMIT sqlc.arg(page_size);

-- name: UpdateClarificationQuestion :one
UPDATE clarification_questions SET revision=revision+1,status=$3 WHERE event_id=$1 AND id=$2 RETURNING *;

-- name: ClarificationAnswer :one
SELECT * FROM clarification_answers WHERE event_id=$1 AND id=$2;

-- name: ClarificationAnswerForQuestion :one
SELECT * FROM clarification_answers WHERE event_id=$1 AND question_id=$2;

-- name: InsertClarificationAnswer :one
INSERT INTO clarification_answers(event_id,id,question_id,revision,latest_version) VALUES($1,$2,$3,1,1) RETURNING *;

-- name: UpdateClarificationAnswer :one
UPDATE clarification_answers SET revision=$3,latest_version=$4,approved_version=$5,published_version=$6
WHERE event_id=$1 AND id=$2 RETURNING *;

-- name: ClarificationVersion :one
SELECT * FROM clarification_versions WHERE event_id=$1 AND answer_id=$2 AND version=$3;

-- name: InsertClarificationVersion :exec
INSERT INTO clarification_versions(event_id,answer_id,version,document,source,author_id,authored_at)
VALUES($1,$2,$3,$4,$5,$6,$7);

-- name: ClarificationApproval :one
SELECT * FROM clarification_approvals WHERE event_id=$1 AND answer_id=$2 AND version=$3;

-- name: InsertClarificationApproval :exec
INSERT INTO clarification_approvals(event_id,answer_id,version,actor_id,approved_at) VALUES($1,$2,$3,$4,$5);

-- name: ClarificationPublication :one
SELECT * FROM clarification_publications WHERE event_id=$1 AND answer_id=$2 AND version=$3;

-- name: InsertClarificationPublication :exec
INSERT INTO clarification_publications(event_id,answer_id,version,actor_id,published_at) VALUES($1,$2,$3,$4,$5);

-- Public queries deliberately do not select question IDs, text, attachments or team IDs.
-- name: PublishedClarifications :many
SELECT a.id,v.version,v.document,v.source,person.display_name AS author_name,v.authored_at,r.approved_at,p.published_at
FROM clarification_answers a
JOIN clarification_versions v ON (v.event_id,v.answer_id,v.version)=(a.event_id,a.id,a.published_version)
JOIN people person ON person.id=v.author_id
JOIN clarification_approvals r ON (r.event_id,r.answer_id,r.version)=(v.event_id,v.answer_id,v.version)
JOIN clarification_publications p ON (p.event_id,p.answer_id,p.version)=(v.event_id,v.answer_id,v.version)
WHERE a.event_id=$1 AND a.id>sqlc.arg(cursor)::uuid AND (sqlc.arg(search)::text='' OR
 position(lower(sqlc.arg(search)::text) in lower(v.document->>'title'))>0 OR
 position(lower(sqlc.arg(search)::text) in lower(v.document->>'body'))>0)
ORDER BY a.id LIMIT sqlc.arg(page_size);

-- name: PublishedClarificationHistory :many
SELECT a.id,v.version,v.document,v.source,person.display_name AS author_name,v.authored_at,r.approved_at,p.published_at,
 v.version=a.published_version AS current
FROM clarification_answers a
JOIN clarification_versions v ON (v.event_id,v.answer_id)=(a.event_id,a.id)
JOIN people person ON person.id=v.author_id
JOIN clarification_approvals r ON (r.event_id,r.answer_id,r.version)=(v.event_id,v.answer_id,v.version)
JOIN clarification_publications p ON (p.event_id,p.answer_id,p.version)=(v.event_id,v.answer_id,v.version)
WHERE a.event_id=$1 AND a.id=$2 AND v.version<sqlc.arg(before_version)::bigint ORDER BY v.version DESC LIMIT sqlc.arg(page_size);

-- name: RecordClarificationChange :exec
INSERT INTO clarification_changes(event_id,id,question_id,answer_id,revision,version,action,actor_id,occurred_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9);

-- name: ClarificationChanges :many
SELECT id,revision,version,action,actor_id,occurred_at FROM clarification_changes
WHERE event_id=$1 AND question_id=$2 AND id>sqlc.arg(cursor)::uuid ORDER BY id LIMIT sqlc.arg(page_size);

-- name: ClarificationRecipients :many
SELECT c.person_id FROM competition_claims c JOIN people p ON p.id=c.person_id AND p.active
WHERE c.event_id=$1 AND c.state='confirmed' ORDER BY c.person_id;

-- name: ClarificationOwningRelease :one
SELECT EXISTS(SELECT 1 FROM content_changes WHERE event_id=$1 AND content_id=$2 AND version=$3 AND action='released')::bool;
