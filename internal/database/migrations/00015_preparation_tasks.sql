-- +goose Up
CREATE TABLE preparation_tasks (
    event_id uuid NOT NULL REFERENCES events(id),
    id uuid NOT NULL,
    revision bigint NOT NULL,
    document jsonb NOT NULL,
    PRIMARY KEY(event_id,id)
);
CREATE TABLE preparation_task_changes (
    event_id uuid NOT NULL,
    task_id uuid NOT NULL,
    revision bigint NOT NULL,
    actor_id uuid NOT NULL REFERENCES people(id),
    document jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(event_id,task_id,revision),
    FOREIGN KEY(event_id,task_id) REFERENCES preparation_tasks(event_id,id)
);
CREATE TRIGGER preparation_task_changes_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON preparation_task_changes
FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();

-- +goose Down
DROP TABLE preparation_task_changes;
DROP TABLE preparation_tasks;
