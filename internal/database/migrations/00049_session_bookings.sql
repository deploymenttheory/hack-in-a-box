-- +goose Up
CREATE TABLE bookable_sessions (
 event_id uuid NOT NULL REFERENCES events(id),
 id uuid NOT NULL,
 revision bigint NOT NULL DEFAULT 1,
 calendar_sequence bigint NOT NULL DEFAULT 0,
 definition jsonb NOT NULL,
 source_snapshot jsonb NOT NULL,
 needs_review boolean NOT NULL DEFAULT false,
 cancelled boolean NOT NULL DEFAULT false,
 PRIMARY KEY(event_id,id),
 CHECK(revision > 0 AND calendar_sequence >= 0)
);
CREATE TABLE session_bookings (
 event_id uuid NOT NULL,
 session_id uuid NOT NULL,
 person_id uuid NOT NULL REFERENCES people(id),
 state text NOT NULL CHECK(state IN ('booked','waitlisted','cancelled','ineligible')),
 queue_sequence bigserial NOT NULL UNIQUE,
 revision bigint NOT NULL DEFAULT 1,
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(event_id,session_id,person_id),
 FOREIGN KEY(event_id,session_id) REFERENCES bookable_sessions(event_id,id)
);
CREATE INDEX session_bookings_fifo ON session_bookings(event_id,session_id,queue_sequence) WHERE state='waitlisted';
CREATE TABLE session_attendance (
 event_id uuid NOT NULL,
 session_id uuid NOT NULL,
 person_id uuid NOT NULL,
 present boolean NOT NULL,
 revision bigint NOT NULL DEFAULT 1,
 PRIMARY KEY(event_id,session_id,person_id),
 FOREIGN KEY(event_id,session_id,person_id) REFERENCES session_bookings(event_id,session_id,person_id)
);
CREATE TABLE session_commands (
 event_id uuid NOT NULL,
 session_id uuid NOT NULL,
 request_id uuid NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 fingerprint text NOT NULL,
 result jsonb NOT NULL,
 PRIMARY KEY(event_id,request_id),
 FOREIGN KEY(event_id,session_id) REFERENCES bookable_sessions(event_id,id)
);
CREATE TABLE session_history (
 id bigserial PRIMARY KEY,
 event_id uuid NOT NULL,
 session_id uuid NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 person_id uuid REFERENCES people(id),
 action text NOT NULL,
 session_revision bigint NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(event_id,session_id) REFERENCES bookable_sessions(event_id,id)
);
CREATE TRIGGER session_history_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON session_history
FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER session_commands_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON session_commands
FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM session_history) OR EXISTS(SELECT 1 FROM session_commands)
    OR EXISTS(SELECT 1 FROM session_bookings) THEN
  RAISE EXCEPTION 'session history and command receipts must be retained';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE session_history;
DROP TABLE session_commands;
DROP TABLE session_attendance;
DROP TABLE session_bookings;
DROP TABLE bookable_sessions;
