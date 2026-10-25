-- +goose Up
CREATE TABLE recovery_control (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 hold_id uuid,
 held_at timestamptz,
 CHECK ((hold_id IS NULL) = (held_at IS NULL))
);
INSERT INTO recovery_control(singleton) VALUES(true);
CREATE TABLE recovery_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 hold_id uuid NOT NULL,
 action text NOT NULL CHECK(action IN ('hold','resume')),
 occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 reviewed_fingerprint text,
 CHECK ((action='resume') = (reviewed_fingerprint IS NOT NULL))
);
-- +goose StatementBegin
CREATE FUNCTION protect_recovery_history() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'recovery maintenance history is immutable';
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER recovery_history_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON recovery_history
 FOR EACH STATEMENT EXECUTE FUNCTION protect_recovery_history();

-- +goose Down
-- Refuse to remove the only startup fence while a restore is unreviewed.
-- +goose StatementBegin
DO $$
BEGIN
 IF EXISTS(SELECT 1 FROM recovery_control WHERE hold_id IS NOT NULL) THEN
  RAISE EXCEPTION 'review and resume recovery before schema rollback';
 END IF;
END;
$$;
-- +goose StatementEnd
DROP TRIGGER recovery_history_immutable ON recovery_history;
DROP FUNCTION protect_recovery_history();
DROP TABLE recovery_history;
DROP TABLE recovery_control;
