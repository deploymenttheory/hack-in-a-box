-- +goose Up
CREATE TABLE attendance_records (
    event_id uuid NOT NULL REFERENCES events(id),
    person_id uuid NOT NULL REFERENCES people(id),
    attendance_day date NOT NULL,
    present boolean NOT NULL,
    revision bigint NOT NULL DEFAULT 1,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(event_id,person_id,attendance_day)
);
CREATE TABLE attendance_changes (
    event_id uuid NOT NULL REFERENCES events(id),
    request_id uuid NOT NULL,
    person_id uuid NOT NULL REFERENCES people(id),
    attendance_day date NOT NULL,
    present boolean NOT NULL,
    expected_revision bigint NOT NULL,
    revision bigint NOT NULL,
    actor_id uuid NOT NULL REFERENCES people(id),
    reason text NOT NULL DEFAULT '' CHECK(length(reason)<=500),
    changed boolean NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(event_id,request_id)
);
CREATE INDEX attendance_changes_person ON attendance_changes(event_id,person_id,attendance_day,created_at);
CREATE TRIGGER attendance_changes_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON attendance_changes
FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TABLE catering_settings (
    event_id uuid PRIMARY KEY REFERENCES events(id),
    buffer integer NOT NULL DEFAULT 0 CHECK(buffer BETWEEN 0 AND 10000)
);
ALTER TABLE operational_profiles ADD COLUMN travel_details text NOT NULL DEFAULT '' CHECK(length(travel_details)<=2000);
ALTER TABLE operational_profiles ADD COLUMN social_attendance text NOT NULL DEFAULT 'unknown' CHECK(social_attendance IN ('unknown','yes','no'));

-- +goose Down
ALTER TABLE operational_profiles DROP COLUMN social_attendance;
ALTER TABLE operational_profiles DROP COLUMN travel_details;
DROP TABLE catering_settings;
DROP TABLE attendance_changes;
DROP TABLE attendance_records;
