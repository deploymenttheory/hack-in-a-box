-- name: SurveyHead :one
SELECT * FROM survey_heads WHERE event_id=$1 AND id=$2;
-- name: SurveyHeadsPage :many
SELECT * FROM survey_heads WHERE event_id=$1 AND id>sqlc.arg(after_id) ORDER BY id LIMIT sqlc.arg(page_size);
-- name: InsertSurveyHead :exec
INSERT INTO survey_heads(event_id,id,current_version) VALUES($1,$2,1);
-- name: UpdateSurveyHead :exec
UPDATE survey_heads SET current_version=$3,published_version=$4,cancelled=$5 WHERE event_id=$1 AND id=$2;
-- name: SurveyVersion :one
SELECT * FROM survey_versions WHERE event_id=$1 AND survey_id=$2 AND version=$3;
-- name: InsertSurveyVersion :exec
INSERT INTO survey_versions(event_id,survey_id,version,document,actor_id,created_at) VALUES($1,$2,$3,$4,$5,$6);
-- name: SurveyApproval :one
SELECT * FROM survey_approvals WHERE event_id=$1 AND survey_id=$2 AND version=$3;
-- name: ApproveSurvey :exec
INSERT INTO survey_approvals(event_id,survey_id,version,actor_id,approved_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING;
-- name: PublishSurvey :exec
INSERT INTO survey_publications(event_id,survey_id,version,actor_id,published_at) VALUES($1,$2,$3,$4,$5);
-- name: SurveyPublication :one
SELECT * FROM survey_publications WHERE event_id=$1 AND survey_id=$2;
-- name: InviteSurveyPerson :exec
INSERT INTO survey_invitations(event_id,survey_id,person_id,version,invited_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING;
-- name: SurveyInvitation :one
SELECT * FROM survey_invitations WHERE event_id=$1 AND survey_id=$2 AND person_id=$3;
-- name: SurveyInvitations :many
SELECT * FROM survey_invitations WHERE event_id=$1 AND survey_id=$2 ORDER BY person_id LIMIT 10001;
-- name: OwnSurveyInvitationsPage :many
SELECT * FROM survey_invitations WHERE event_id=$1 AND person_id=$2 AND survey_id>sqlc.arg(after_id) ORDER BY survey_id LIMIT sqlc.arg(page_size);
-- name: SurveyResponse :one
SELECT r.* FROM survey_response_heads h JOIN survey_responses r ON r.event_id=h.event_id AND r.survey_id=h.survey_id AND r.person_id=h.person_id AND r.revision=h.revision WHERE h.event_id=$1 AND h.survey_id=$2 AND h.person_id=$3;
-- name: SurveyResponseVersion :one
SELECT * FROM survey_responses WHERE event_id=$1 AND survey_id=$2 AND person_id=$3 AND revision=$4;
-- name: SurveyResponseHeadsPage :many
SELECT * FROM survey_response_heads WHERE event_id=$1 AND survey_id=$2 AND person_id>sqlc.arg(after_person) ORDER BY person_id LIMIT sqlc.arg(page_size);
-- name: SurveyResponseHistoryPage :many
SELECT * FROM survey_responses WHERE event_id=$1 AND survey_id=$2 AND person_id=$3 AND revision<sqlc.arg(before_revision) ORDER BY revision DESC LIMIT sqlc.arg(page_size);
-- name: CurrentSurveyResponses :many
SELECT r.* FROM survey_response_heads h JOIN survey_responses r ON r.event_id=h.event_id AND r.survey_id=h.survey_id AND r.person_id=h.person_id AND r.revision=h.revision WHERE h.event_id=$1 AND h.survey_id=$2 ORDER BY h.person_id LIMIT 10001;
-- name: SetSurveyResponseHead :exec
INSERT INTO survey_response_heads(event_id,survey_id,person_id,revision) VALUES($1,$2,$3,$4) ON CONFLICT(event_id,survey_id,person_id) DO UPDATE SET revision=EXCLUDED.revision;
-- name: InsertSurveyResponse :one
INSERT INTO survey_responses(event_id,survey_id,person_id,revision,form_version,answers,submitted_at) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING *;
-- name: SurveyCommand :one
SELECT * FROM survey_commands WHERE event_id=$1 AND survey_id=$2 AND person_id=$3 AND command_id=$4;
-- name: InsertSurveyCommand :exec
INSERT INTO survey_commands(event_id,survey_id,person_id,command_id,payload_digest,result_revision) VALUES($1,$2,$3,$4,$5,$6);
-- name: SurveyReportSnapshot :one
SELECT * FROM survey_report_snapshots WHERE event_id=$1 AND survey_id=$2;
-- name: InsertSurveyReportSnapshot :exec
INSERT INTO survey_report_snapshots(event_id,survey_id,survey_version,aggregate_digest,document,created_at) VALUES($1,$2,$3,$4,$5,$6);
-- name: SurveyReportReview :one
SELECT * FROM survey_report_reviews WHERE event_id=$1 AND survey_id=$2 AND id=$3;
-- name: LatestSurveyReportReview :one
SELECT * FROM survey_report_reviews WHERE event_id=$1 AND survey_id=$2 ORDER BY version DESC LIMIT 1;
-- name: InsertSurveyReportReview :exec
INSERT INTO survey_report_reviews(event_id,survey_id,id,version,summary,actor_id,reviewed_at) VALUES($1,$2,$3,$4,$5,$6,$7);
-- name: SurveyReportApproval :one
SELECT * FROM survey_report_approvals WHERE event_id=$1 AND survey_id=$2 AND review_id=$3;
-- name: ApproveSurveyReport :exec
INSERT INTO survey_report_approvals(event_id,survey_id,review_id,actor_id,approved_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING;
-- name: SurveyReportPublication :one
SELECT * FROM survey_report_publications WHERE event_id=$1 AND survey_id=$2 ORDER BY version DESC LIMIT 1;
-- name: PublishSurveyReport :exec
INSERT INTO survey_report_publications(event_id,survey_id,version,review_id,actor_id,published_at) VALUES($1,$2,$3,$4,$5,$6);
-- name: DeleteSurveyPersonCommands :execrows
DELETE FROM survey_commands WHERE event_id=$1 AND person_id=$2;
-- name: DeleteSurveyPersonResponses :execrows
DELETE FROM survey_responses WHERE event_id=$1 AND person_id=$2;
-- name: DeleteSurveyPersonHeads :execrows
DELETE FROM survey_response_heads WHERE event_id=$1 AND person_id=$2;
-- name: DeleteSurveyPersonInvitations :execrows
DELETE FROM survey_invitations WHERE event_id=$1 AND person_id=$2;
-- name: SurveyPrivacyEvents :many
SELECT DISTINCT e.id FROM events e JOIN survey_invitations i ON i.event_id=e.id WHERE e.stage IN ('completed','cancelled','archived') AND e.completed_at IS NOT NULL AND e.id>sqlc.arg(after_id) ORDER BY e.id LIMIT 100;
-- name: SurveyPrivacyPeoplePage :many
SELECT DISTINCT person_id FROM survey_invitations WHERE event_id=$1 AND person_id>sqlc.arg(after_person) ORDER BY person_id LIMIT 100;
-- name: SurveyOpenSourcesPage :many
SELECT p.survey_id,p.version,v.document FROM survey_publications p JOIN survey_heads h ON h.event_id=p.event_id AND h.id=p.survey_id AND h.published_version=p.version JOIN survey_versions v ON v.event_id=p.event_id AND v.survey_id=p.survey_id AND v.version=p.version WHERE p.event_id=$1 AND NOT h.cancelled AND (sqlc.narg(source_id)::uuid IS NULL OR p.survey_id=sqlc.narg(source_id)) AND p.survey_id>sqlc.arg(after_id) AND (v.document->>'opens_at')::timestamptz<=sqlc.arg(at_time)::timestamptz AND (v.document->>'closes_at')::timestamptz>sqlc.arg(at_time) ORDER BY p.survey_id LIMIT sqlc.arg(page_size);

-- name: SurveyResponseAuthorNames :many
SELECT p.id,p.display_name FROM survey_response_heads h JOIN people p ON p.id=h.person_id
WHERE h.event_id=$1 AND h.survey_id=$2 ORDER BY p.id LIMIT 10001;
