-- name: ArchivedEventsForReader :many
SELECT e.* FROM events e WHERE e.stage='archived' AND e.id>sqlc.arg(after_id)::uuid AND (
 EXISTS(SELECT 1 FROM people p WHERE p.id=sqlc.arg(actor_id)::uuid AND p.active AND p.platform_admin)
 OR EXISTS(SELECT 1 FROM event_memberships m WHERE m.event_id=e.id AND m.person_id=sqlc.arg(actor_id)::uuid AND m.active)
 OR EXISTS(SELECT 1 FROM competition_claims c WHERE c.event_id=e.id AND c.person_id=sqlc.arg(actor_id)::uuid AND c.state='confirmed'))
ORDER BY e.id LIMIT sqlc.arg(page_size);

-- name: ArchivedReportingEvents :many
SELECT e.* FROM events e WHERE e.stage='archived' AND e.id>sqlc.arg(after_id)::uuid
AND EXISTS(SELECT 1 FROM event_memberships m JOIN people p ON p.id=m.person_id AND p.active
 WHERE m.event_id=e.id AND m.person_id=sqlc.arg(actor_id)::uuid AND m.active AND m.role='organiser')
ORDER BY e.id LIMIT sqlc.arg(page_size);

-- name: ArchiveParticipationCounts :one
SELECT
 (SELECT count(*) FROM registrations r WHERE r.event_id=sqlc.arg(event_id)::uuid AND r.kind='attendee') AS registration_records,
 (SELECT count(DISTINCT r.person_id) FROM registrations r WHERE r.event_id=sqlc.arg(event_id)::uuid AND r.kind='attendee') AS registered_people,
 (SELECT count(*) FROM competition_claims c WHERE c.event_id=sqlc.arg(event_id)::uuid AND c.state='confirmed') AS confirmed_attendees,
 (SELECT count(DISTINCT a.person_id) FROM attendance_records a WHERE a.event_id=sqlc.arg(event_id)::uuid AND a.present) AS recorded_attendance,
 (SELECT count(*) FROM competition_entries e WHERE e.event_id=sqlc.arg(event_id)::uuid) AS entry_records,
 (SELECT count(*) FROM submission_states s JOIN competition_entries e USING(event_id,team_id) JOIN hack_teams t ON t.event_id=e.event_id AND t.id=e.team_id
  WHERE s.event_id=sqlc.arg(event_id)::uuid AND s.outcome='frozen' AND s.active_revision IS NOT NULL AND s.frozen_at IS NOT NULL AND e.active AND t.state!='disbanded') AS completed_entries,
 (SELECT count(*) FROM submission_states s WHERE s.event_id=sqlc.arg(event_id)::uuid AND s.outcome='not_submitted') AS not_submitted_entries,
 (SELECT count(*) FROM submission_states s WHERE s.event_id=sqlc.arg(event_id)::uuid AND s.outcome='withdrawn') AS withdrawn_entries,
 (SELECT count(*) FROM competition_entries e JOIN hack_teams t ON t.event_id=e.event_id AND t.id=e.team_id WHERE e.event_id=sqlc.arg(event_id)::uuid AND (NOT e.active OR t.state='disbanded')) AS unused_entries,
 (SELECT count(*) FROM competition_entries e LEFT JOIN submission_states s USING(event_id,team_id) WHERE e.event_id=sqlc.arg(event_id)::uuid AND s.team_id IS NULL) AS missing_submission_outcomes;
