-- +goose Up
CREATE TABLE bookable_sessions (
 event_id uuid NOT NULL REFERENCES events(id),
 id uuid NOT NULL,
 revision bigint NOT NULL DEFAULT 1,
 calendar_sequence bigint NOT NULL DEFAULT 0,
 notification_version bigint NOT NULL DEFAULT 0,
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
 calendar_sequence bigint NOT NULL DEFAULT 0 CHECK(calendar_sequence >= 0),
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

CREATE TABLE session_notification_configurations (
 event_id uuid NOT NULL,
 session_id uuid NOT NULL,
 version bigint NOT NULL,
 id uuid NOT NULL DEFAULT gen_random_uuid() UNIQUE,
 channels jsonb NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(event_id,session_id,version),
 FOREIGN KEY(event_id,session_id) REFERENCES bookable_sessions(event_id,id)
);
CREATE TABLE session_notification_approvals (
 configuration_id uuid PRIMARY KEY REFERENCES session_notification_configurations(id),
 actor_id uuid NOT NULL REFERENCES people(id),
 provider_revisions jsonb NOT NULL,
 binding text NOT NULL,
 approved_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE session_notification_occurrences (
 sequence bigserial NOT NULL UNIQUE,
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 event_id uuid NOT NULL,
 session_id uuid NOT NULL,
 recipient_id uuid NOT NULL REFERENCES people(id),
 configuration_version bigint NOT NULL,
 session_revision bigint NOT NULL,
 booking_revision bigint NOT NULL,
 booking_state text NOT NULL CHECK(booking_state IN ('booked','waitlisted','cancelled','ineligible')),
 calendar_sequence bigint NOT NULL,
 binding text NOT NULL,
 source_state text NOT NULL CHECK(source_state IN ('approved','suppressed','unavailable')),
 reason text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(event_id,session_id,recipient_id,configuration_version,session_revision,booking_revision),
 FOREIGN KEY(event_id,session_id) REFERENCES bookable_sessions(event_id,id)
);
CREATE TABLE session_notification_reports (
 occurrence_id uuid NOT NULL REFERENCES session_notification_occurrences(id),
 channel text NOT NULL CHECK(channel IN ('in_app','smtp')),
 state text NOT NULL CHECK(state IN ('queued','unavailable')),
 queued_intents integer NOT NULL CHECK(queued_intents BETWEEN 0 AND 1),
 reason text NOT NULL,
 PRIMARY KEY(occurrence_id,channel)
);
CREATE TRIGGER session_notification_reports_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON session_notification_reports
FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER session_notification_configurations_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON session_notification_configurations
FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER session_notification_approvals_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON session_notification_approvals
FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER session_notification_occurrences_identity BEFORE UPDATE OR DELETE OR TRUNCATE ON session_notification_occurrences
FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM session_history) OR EXISTS(SELECT 1 FROM session_commands)
    OR EXISTS(SELECT 1 FROM session_bookings)
    OR EXISTS(SELECT 1 FROM session_notification_configurations)
    OR EXISTS(SELECT 1 FROM session_notification_occurrences) THEN
  RAISE EXCEPTION 'session history and command receipts must be retained';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE session_notification_reports;
DROP TABLE session_notification_occurrences;
DROP TABLE session_notification_approvals;
DROP TABLE session_notification_configurations;
DROP TABLE session_history;
DROP TABLE session_commands;
DROP TABLE session_attendance;
DROP TABLE session_bookings;
DROP TABLE bookable_sessions;
