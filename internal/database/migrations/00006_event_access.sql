-- +goose Up
ALTER TABLE events ADD COLUMN owner_id uuid REFERENCES people(id);
ALTER TABLE events ADD COLUMN writes_allowed boolean NOT NULL DEFAULT true;
ALTER TABLE login_attempts ADD COLUMN return_to text NOT NULL DEFAULT '/';

CREATE TABLE event_memberships (
    event_id uuid NOT NULL REFERENCES events(id),
    person_id uuid NOT NULL REFERENCES people(id),
    role text NOT NULL CHECK (role IN ('organiser','mentor','judge','mc','logistics','photographer','early_brief_access')),
    active boolean NOT NULL DEFAULT true,
    changed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id,person_id,role)
);
CREATE INDEX event_memberships_person ON event_memberships(person_id,event_id) WHERE active;
CREATE TABLE competition_exclusions (
    event_id uuid NOT NULL REFERENCES events(id),
    person_id uuid NOT NULL REFERENCES people(id),
    source text NOT NULL CHECK (source IN ('organiser','early_brief_access')),
    occurred_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(event_id,person_id)
);
CREATE TRIGGER competition_exclusions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON competition_exclusions
FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();

-- The registration domain writes these facts in its own transaction through
-- permissions.SetRegistrationState. It must take the same event row lock as grants.
CREATE TABLE competition_claims (
    event_id uuid NOT NULL REFERENCES events(id),
    person_id uuid NOT NULL REFERENCES people(id),
    state text NOT NULL CHECK (state IN ('received','waitlisted','confirmed','withdrawn')),
    PRIMARY KEY(event_id,person_id)
);

-- Domain owners register private resources and assign reviewers/team members.
-- Composite keys make an assignment to a resource in another event impossible.
CREATE TABLE policy_resources (
    event_id uuid NOT NULL REFERENCES events(id),
    id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('team','assessment','credential','brief','presentation','logistics','media')),
    PRIMARY KEY(event_id,id)
);
CREATE TABLE resource_assignments (
    event_id uuid NOT NULL,
    resource_id uuid NOT NULL,
    person_id uuid NOT NULL REFERENCES people(id),
    PRIMARY KEY(event_id,resource_id,person_id),
    FOREIGN KEY(event_id,resource_id) REFERENCES policy_resources(event_id,id)
);

-- +goose Down
DROP TABLE resource_assignments;
DROP TABLE policy_resources;
DROP TABLE competition_claims;
DROP TABLE competition_exclusions;
DROP TABLE event_memberships;
ALTER TABLE login_attempts DROP COLUMN return_to;
ALTER TABLE events DROP COLUMN writes_allowed;
ALTER TABLE events DROP COLUMN owner_id;
