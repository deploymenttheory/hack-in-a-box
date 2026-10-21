-- +goose Up
CREATE TABLE registrations (
    id uuid PRIMARY KEY,
    event_id uuid NOT NULL REFERENCES events(id),
    person_id uuid NOT NULL REFERENCES people(id),
    form_id uuid NOT NULL,
    form_version integer NOT NULL,
    audience text NOT NULL CHECK(audience IN ('attendee','organiser','mentor','judge','mc','logistics','photographer')),
    kind text NOT NULL CHECK(kind IN ('attendee','facilitator')),
    state text NOT NULL CHECK(state IN ('received','confirmed','waitlisted','offered','withdrawn','selected','backup','declined')),
    sequence bigserial NOT NULL UNIQUE,
    revision bigint NOT NULL DEFAULT 1,
    request_hash text NOT NULL,
    response_revision bigint NOT NULL DEFAULT 1,
    team_preference text NOT NULL CHECK(team_preference IN ('captain','join','solo')),
    invitation_reference text NOT NULL DEFAULT '' CHECK(length(invitation_reference)<=160),
    fallback_to_attendee boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(event_id,id),
    FOREIGN KEY(event_id,form_id,form_version) REFERENCES registration_form_versions(event_id,form_id,version)
);
CREATE UNIQUE INDEX registrations_one_active ON registrations(event_id,person_id,kind)
WHERE state NOT IN ('withdrawn','declined');
CREATE INDEX registrations_fifo ON registrations(event_id,sequence) WHERE state='waitlisted';
CREATE TABLE registration_history (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id uuid NOT NULL,
    registration_id uuid NOT NULL,
    actor_id uuid NOT NULL REFERENCES people(id),
    action text NOT NULL,
    from_state text NOT NULL,
    to_state text NOT NULL,
    reason text NOT NULL DEFAULT '' CHECK(length(reason)<=2000),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY(event_id,registration_id) REFERENCES registrations(event_id,id)
);
CREATE TRIGGER registration_history_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON registration_history
FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
ALTER TABLE event_gate_facts ALTER COLUMN attendance_ready SET DEFAULT true;
UPDATE event_gate_facts SET attendance_ready=true;

-- +goose Down
ALTER TABLE event_gate_facts ALTER COLUMN attendance_ready SET DEFAULT false;
UPDATE event_gate_facts SET attendance_ready=false;
DROP TABLE registration_history;
DROP TABLE registrations;
