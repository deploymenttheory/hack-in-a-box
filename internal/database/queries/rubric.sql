-- name: EventRubric :one
SELECT * FROM event_rubrics WHERE event_id=$1;

-- name: CreateEventRubric :exec
INSERT INTO event_rubrics(event_id,id) VALUES($1,$2);

-- name: SaveRubricHeader :one
UPDATE event_rubrics SET revision=$2,latest_version=$3,approved_version=$4,published_version=$5 WHERE event_id=$1 RETURNING *;

-- name: RubricVersion :one
SELECT * FROM rubric_versions WHERE event_id=$1 AND rubric_id=$2 AND version=$3;

-- name: InsertRubricVersion :exec
INSERT INTO rubric_versions(event_id,rubric_id,version,document,public_document,public_digest,actor_id,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8);

-- name: RubricApproval :one
SELECT * FROM rubric_approvals WHERE event_id=$1 AND rubric_id=$2 AND version=$3;

-- name: ApproveRubricVersion :exec
INSERT INTO rubric_approvals(event_id,rubric_id,version,actor_id,reason,approved_at) VALUES($1,$2,$3,$4,$5,$6);

-- name: RubricImpactReview :one
SELECT * FROM rubric_impact_reviews WHERE event_id=$1 AND rubric_id=$2 AND version=$3;

-- name: ReviewRubricImpact :exec
INSERT INTO rubric_impact_reviews(event_id,rubric_id,version,impact_review,notice,actor_id,reviewed_at) VALUES($1,$2,$3,$4,$5,$6,$7);

-- name: RecordRubricPublication :exec
INSERT INTO rubric_publications(event_id,rubric_id,version,challenge_id,challenge_version,public_digest,published_at,reviewed_notice) VALUES($1,$2,$3,$4,$5,$6,$7,$8);

-- name: RubricPublication :one
SELECT * FROM rubric_publications WHERE event_id=$1 AND challenge_id=$2 AND challenge_version=$3;

-- name: NotifyRubricAttendees :execrows
INSERT INTO rubric_notice_recipients(event_id,challenge_id,challenge_version,person_id)
SELECT $1,$2,$3,c.person_id FROM competition_claims c JOIN people p ON p.id=c.person_id AND p.active WHERE c.event_id=$1 AND c.state='confirmed';

-- name: RubricChallenge :one
SELECT * FROM content_items WHERE event_id=$1 AND kind='challenge';

-- name: RubricRun :one
SELECT * FROM rubric_assessment_runs WHERE event_id=$1 AND id=$2;

-- name: CurrentRubricRuns :many
SELECT * FROM rubric_assessment_runs WHERE event_id=$1 AND status='current' ORDER BY purpose;

-- name: CreateRubricRun :exec
INSERT INTO rubric_assessment_runs(event_id,id,rubric_id,rubric_version,purpose,status,previous_id,reason,actor_id,created_at) VALUES($1,$2,$3,$4,$5,'current',$6,$7,$8,$9);

-- name: SupersedeRubricRun :exec
UPDATE rubric_assessment_runs SET status='superseded' WHERE event_id=$1 AND id=$2;

-- name: StartRubricRun :exec
UPDATE rubric_assessment_runs SET started_at=coalesce(started_at,$3) WHERE event_id=$1 AND id=$2;

-- name: RecordRubricChange :exec
INSERT INTO rubric_changes(event_id,rubric_id,revision,version,action,actor_id,reason,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8);

-- name: RubricChanges :many
SELECT * FROM rubric_changes WHERE event_id=$1 AND rubric_id=$2 AND revision<sqlc.arg(before)::bigint ORDER BY revision DESC LIMIT sqlc.arg(page_size);

-- name: RubricAssignedJudges :many
SELECT m.person_id FROM event_memberships m JOIN people p ON p.id=m.person_id AND p.active
JOIN resource_assignments a ON a.event_id=m.event_id AND a.person_id=m.person_id AND a.resource_id=$2
WHERE m.event_id=$1 AND m.role='judge' AND m.active AND (NOT EXISTS(SELECT 1 FROM judge_rounds j WHERE j.event_id=m.event_id) OR EXISTS(SELECT 1 FROM judge_assignments j JOIN competition_entries e ON e.event_id=j.event_id AND e.team_id=j.team_id WHERE j.event_id=m.event_id AND j.person_id=m.person_id AND j.active AND e.id=$2)) ORDER BY m.person_id;
