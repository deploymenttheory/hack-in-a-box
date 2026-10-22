-- name: AttendanceRoster :many
SELECT p.id,p.display_name,p.active,
 COALESCE(a.present,false)::boolean AS present,COALESCE(a.revision,0)::bigint AS revision,a.updated_at
FROM competition_claims c JOIN people p ON p.id=c.person_id
LEFT JOIN attendance_records a ON a.event_id=c.event_id AND a.person_id=c.person_id AND a.attendance_day=sqlc.arg(attendance_day)::date
WHERE c.event_id=sqlc.arg(event_id)::uuid AND c.state='confirmed'
ORDER BY p.display_name,p.id;

-- name: AttendanceRecord :one
SELECT * FROM attendance_records WHERE event_id=$1 AND person_id=$2 AND attendance_day=$3;

-- name: SaveAttendance :one
INSERT INTO attendance_records(event_id,person_id,attendance_day,present) VALUES($1,$2,$3,$4)
ON CONFLICT(event_id,person_id,attendance_day) DO UPDATE SET present=EXCLUDED.present,revision=attendance_records.revision+1,updated_at=now()
RETURNING *;

-- name: AttendanceRequest :one
SELECT * FROM attendance_changes WHERE event_id=$1 AND request_id=$2;

-- name: RecordAttendanceChange :exec
INSERT INTO attendance_changes(event_id,request_id,person_id,attendance_day,present,expected_revision,revision,actor_id,reason,changed)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10);

-- name: AttendanceHistory :many
SELECT * FROM attendance_changes WHERE event_id=$1 AND person_id=$2 AND attendance_day=$3 AND changed
ORDER BY created_at DESC,request_id LIMIT 200;

-- name: CateringBuffer :one
SELECT COALESCE(c.buffer,0)::integer AS buffer FROM events e LEFT JOIN catering_settings c ON c.event_id=e.id WHERE e.id=$1;

-- name: SaveCateringBuffer :exec
INSERT INTO catering_settings(event_id,buffer) VALUES($1,$2) ON CONFLICT(event_id) DO UPDATE SET buffer=EXCLUDED.buffer;

-- name: AttendanceStaff :many
SELECT DISTINCT p.id,p.display_name,EXISTS(SELECT 1 FROM competition_claims c WHERE c.event_id=m.event_id AND c.person_id=p.id AND c.state='confirmed') AS competing
FROM event_memberships m JOIN people p ON p.id=m.person_id
WHERE m.event_id=$1 AND m.active AND p.active AND m.role!='early_brief_access'
ORDER BY p.display_name,p.id;

-- name: OperationalRoster :many
SELECT p.* FROM operational_profiles p WHERE p.event_id=$1 AND p.person_id>sqlc.arg(cursor)::uuid AND (
 EXISTS(SELECT 1 FROM competition_claims c WHERE c.event_id=p.event_id AND c.person_id=p.person_id AND c.state='confirmed') OR
 EXISTS(SELECT 1 FROM event_memberships m JOIN people person ON person.id=m.person_id WHERE m.event_id=p.event_id AND m.person_id=p.person_id AND m.active AND person.active AND m.role!='early_brief_access'))
ORDER BY p.person_id LIMIT sqlc.arg(page_size);
