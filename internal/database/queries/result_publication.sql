-- name: CurrentResultCommunicationPublication :one
SELECT EXISTS (
 SELECT 1 FROM result_publications p
 WHERE p.event_id=sqlc.arg(event_id) AND p.id=sqlc.arg(publication_id)
 AND p.result_version=sqlc.arg(result_version) AND p.published_at<=sqlc.arg(at_time)
 AND NOT EXISTS (
  SELECT 1 FROM result_publications newer
  WHERE newer.event_id=p.event_id AND newer.kind=p.kind
  AND newer.result_version>p.result_version AND newer.published_at<=sqlc.arg(at_time)
 )
);
