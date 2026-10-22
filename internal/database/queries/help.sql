-- name: HelpSettings :one
SELECT COALESCE(h.revision,1)::bigint AS revision, COALESCE(h.setup_threshold_seconds,900)::bigint AS setup_threshold_seconds,
COALESCE(h.logistics_threshold_seconds,900)::bigint AS logistics_threshold_seconds,COALESCE(h.output_threshold_seconds,1200)::bigint AS output_threshold_seconds,
COALESCE(h.meeting_hosts,'{}'::text[])::text[] AS meeting_hosts FROM events e LEFT JOIN help_settings h ON h.event_id=e.id WHERE e.id=$1;

-- name: SetHelpSettings :exec
INSERT INTO help_settings(event_id,revision,setup_threshold_seconds,logistics_threshold_seconds,output_threshold_seconds,meeting_hosts)
VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(event_id) DO UPDATE SET revision=EXCLUDED.revision,setup_threshold_seconds=EXCLUDED.setup_threshold_seconds,logistics_threshold_seconds=EXCLUDED.logistics_threshold_seconds,output_threshold_seconds=EXCLUDED.output_threshold_seconds,meeting_hosts=EXCLUDED.meeting_hosts;

-- name: HelpRequest :one
SELECT * FROM help_requests WHERE event_id=$1 AND id=$2;

-- name: HelpRequests :many
SELECT h.* FROM help_requests h WHERE h.event_id=$1 AND EXISTS(SELECT 1 FROM policy_resources r WHERE r.event_id=h.event_id AND r.id=h.id AND r.kind='help') ORDER BY h.requested_at,h.id LIMIT 1001;

-- name: InsertHelpRequest :exec
INSERT INTO help_requests(event_id,id,team_id,requester_id,revision,category,description,location,meeting_url,status,waiting_cycle,requested_at,waiting_since,updated_at)
VALUES($1,$2,$3,$4,1,$5,$6,$7,$8,'waiting',1,$9,$9,$9);

-- name: UpdateHelpRequest :exec
UPDATE help_requests SET revision=$3,status=$4,assignee_id=$5,waiting_cycle=$6,waiting_since=$7,first_claimed_at=$8,claimed_at=$9,resolved_at=$10,updated_at=$11 WHERE event_id=$1 AND id=$2;

-- name: HelpTransitions :many
SELECT * FROM help_transitions WHERE event_id=$1 AND request_id=$2 ORDER BY revision;

-- name: InsertHelpTransition :exec
INSERT INTO help_transitions(event_id,request_id,revision,action,status,actor_id,assignee_id,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8);

-- name: HelpCommand :one
SELECT * FROM help_commands WHERE event_id=$1 AND actor_id=$2 AND command_id=$3;

-- name: InsertHelpCommand :exec
INSERT INTO help_commands(event_id,actor_id,command_id,request_id,payload_digest,response_revision) VALUES($1,$2,$3,$4,$5,$6);

-- name: InsertHelpFollowup :execrows
INSERT INTO help_followups(event_id,request_id,waiting_cycle,settings_revision,created_at,threshold_seconds) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING;

-- name: HelpAuthority :one
SELECT p.active,
EXISTS(SELECT 1 FROM event_memberships m WHERE m.event_id=sqlc.arg(event_id) AND m.person_id=p.id AND m.role='organiser' AND m.active)::boolean AS organiser,
EXISTS(SELECT 1 FROM hack_team_members tm JOIN hack_teams t ON t.event_id=tm.event_id AND t.id=tm.team_id JOIN competition_claims c ON c.event_id=tm.event_id AND c.person_id=tm.person_id WHERE tm.event_id=sqlc.arg(event_id) AND tm.team_id=sqlc.arg(team_id) AND tm.person_id=p.id AND tm.snapshot='published' AND t.state='published' AND c.state='confirmed')::boolean AS team_member,
(EXISTS(SELECT 1 FROM event_memberships m WHERE m.event_id=sqlc.arg(event_id) AND m.person_id=p.id AND m.active AND (m.role='organiser' OR (m.role='logistics' AND sqlc.arg(category)::text IN ('setup','logistics')) OR (m.role='mentor' AND EXISTS(SELECT 1 FROM mentor_assignments a WHERE a.event_id=m.event_id AND a.person_id=m.person_id AND a.team_id=sqlc.arg(team_id) AND a.snapshot='published' AND a.starts_at<=sqlc.arg(now_at)::timestamptz AND a.ends_at>sqlc.arg(now_at)::timestamptz) AND NOT EXISTS(SELECT 1 FROM mentor_profiles mp WHERE mp.event_id=m.event_id AND mp.person_id=m.person_id AND jsonb_array_length(mp.availability)>0 AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(mp.availability) w WHERE (w->>'start')::timestamptz<=sqlc.arg(now_at)::timestamptz AND (w->>'end')::timestamptz>sqlc.arg(now_at)::timestamptz))))) AND EXISTS(SELECT 1 FROM hack_teams ht WHERE ht.event_id=sqlc.arg(event_id) AND ht.id=sqlc.arg(team_id) AND ht.state='published') AND NOT EXISTS(SELECT 1 FROM mentor_conflicts cf JOIN hack_team_members tm ON tm.event_id=cf.event_id AND tm.person_id=cf.competitor_id AND tm.snapshot='published' JOIN competition_claims cc ON cc.event_id=tm.event_id AND cc.person_id=tm.person_id AND cc.state='confirmed' WHERE cf.event_id=sqlc.arg(event_id) AND cf.mentor_id=p.id AND tm.team_id=sqlc.arg(team_id)))::boolean AS helper
FROM people p WHERE p.id=sqlc.arg(person_id);

-- name: HelpTeamActive :one
SELECT EXISTS(SELECT 1 FROM hack_teams WHERE event_id=$1 AND id=$2 AND state='published')::boolean;

-- name: HelpRecipientCandidates :many
SELECT DISTINCT p.id FROM people p WHERE p.active AND (EXISTS(SELECT 1 FROM event_memberships m WHERE m.event_id=$1 AND m.person_id=p.id AND m.active AND m.role IN ('organiser','mentor','logistics')) OR EXISTS(SELECT 1 FROM hack_team_members tm WHERE tm.event_id=$1 AND tm.person_id=p.id AND tm.team_id=$2 AND tm.snapshot='published')) ORDER BY p.id;

-- name: RegisterHelpResource :execrows
INSERT INTO policy_resources(event_id,id,kind) VALUES($1,$2,'help') ON CONFLICT DO NOTHING;

-- name: HelpNotificationSourcesPage :many
SELECT r.id AS request_id,r.revision,h.occurred_at,h.action FROM help_requests r JOIN help_transitions h ON h.event_id=r.event_id AND h.request_id=r.id AND h.revision=r.revision
WHERE r.event_id=sqlc.arg(event_id) AND EXISTS(SELECT 1 FROM policy_resources p WHERE p.event_id=r.event_id AND p.id=r.id AND p.kind='help') AND r.id>sqlc.arg(after_id) AND (sqlc.narg(requested_id)::uuid IS NULL OR r.id=sqlc.narg(requested_id)::uuid)
ORDER BY r.id LIMIT sqlc.arg(page_size);
