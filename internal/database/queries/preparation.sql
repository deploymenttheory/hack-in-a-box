-- name: PreparationTasks :many
SELECT * FROM preparation_tasks WHERE event_id=$1 ORDER BY id;

-- name: SavePreparationTask :exec
INSERT INTO preparation_tasks(event_id,id,revision,document) VALUES($1,$2,$3,$4)
ON CONFLICT(event_id,id) DO UPDATE SET revision=EXCLUDED.revision,document=EXCLUDED.document;

-- name: RecordPreparationChange :exec
INSERT INTO preparation_task_changes(event_id,task_id,revision,actor_id,document) VALUES($1,$2,$3,$4,$5);

-- name: PreparationHistory :many
SELECT * FROM preparation_task_changes WHERE event_id=$1 AND task_id=$2 ORDER BY revision DESC LIMIT 100;
