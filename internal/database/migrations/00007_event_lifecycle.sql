-- +goose Up
ALTER TABLE events ADD COLUMN slug text;
UPDATE events SET slug='event-' || id::text;
ALTER TABLE events ALTER COLUMN slug SET NOT NULL;
ALTER TABLE events ALTER COLUMN slug SET DEFAULT gen_random_uuid()::text;
ALTER TABLE events ADD CONSTRAINT events_slug_unique UNIQUE(slug);
ALTER TABLE events ADD COLUMN configuration jsonb NOT NULL DEFAULT '{}';
ALTER TABLE events ADD COLUMN stage text NOT NULL DEFAULT 'draft' CHECK(stage IN ('draft','registration','preparation','live','assessment','completed','cancelled','archived'));
ALTER TABLE events ADD COLUMN published_at timestamptz;
ALTER TABLE events ADD COLUMN team_frozen_at timestamptz;
ALTER TABLE events ADD COLUMN completed_at timestamptz;
CREATE TABLE event_gate_facts (
    event_id uuid PRIMARY KEY REFERENCES events(id),
    attendance_ready boolean NOT NULL DEFAULT false,
    teams_ready boolean NOT NULL DEFAULT false,
    assigned_attendees integer NOT NULL DEFAULT 0 CHECK(assigned_attendees>=0),
    total_teams integer NOT NULL DEFAULT 0 CHECK(total_teams>=0),
    frozen_teams integer NOT NULL DEFAULT 0 CHECK(frozen_teams>=0),
    submissions_ready boolean NOT NULL DEFAULT false,
    settled_submissions integer NOT NULL DEFAULT 0 CHECK(settled_submissions>=0)
);
INSERT INTO event_gate_facts(event_id) SELECT id FROM events;
CREATE TABLE event_decisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id uuid NOT NULL REFERENCES events(id),
    actor_id uuid NOT NULL REFERENCES people(id),
    kind text NOT NULL,
    reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 2000),
    revision bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER event_decisions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON event_decisions
FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TABLE schedule_items (
    id uuid NOT NULL,
    event_id uuid NOT NULL REFERENCES events(id),
    definition jsonb NOT NULL,
    due_at timestamptz NOT NULL,
    approved boolean NOT NULL DEFAULT false,
    cancelled boolean NOT NULL DEFAULT false,
    executed_at timestamptz,
    operation_id uuid REFERENCES operations(id),
    revision bigint NOT NULL DEFAULT 1,
    PRIMARY KEY(event_id,id)
);
CREATE TABLE event_notices (
    id uuid PRIMARY KEY,
    event_id uuid NOT NULL REFERENCES events(id),
    revision bigint NOT NULL,
    live_start timestamptz NOT NULL,
    live_end timestamptz NOT NULL,
    submission_cutoff timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(event_id,revision)
);

-- +goose Down
DROP TABLE event_notices;
DROP TABLE schedule_items;
DROP TABLE event_decisions;
DROP TABLE event_gate_facts;
ALTER TABLE events DROP COLUMN completed_at;
ALTER TABLE events DROP COLUMN team_frozen_at;
ALTER TABLE events DROP COLUMN published_at;
ALTER TABLE events DROP COLUMN stage;
ALTER TABLE events DROP COLUMN configuration;
ALTER TABLE events DROP COLUMN slug;
