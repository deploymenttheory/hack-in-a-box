-- +goose Up
CREATE TABLE audit_events (
    sequence bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    id uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    actor_id uuid REFERENCES people(id),
    event_id uuid REFERENCES events(id),
    action text NOT NULL,
    target_id uuid NOT NULL,
    reason_code text NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}',
    occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_event_sequence ON audit_events(event_id, sequence);
-- +goose StatementBegin
CREATE FUNCTION reject_audit_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit records are append-only';
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER audit_append_only BEFORE UPDATE OR DELETE OR TRUNCATE ON audit_events
FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();

-- +goose Down
DROP TABLE audit_events;
DROP FUNCTION reject_audit_mutation();
