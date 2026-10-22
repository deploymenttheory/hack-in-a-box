-- +goose Up
CREATE TABLE mentor_settings (
 event_id uuid PRIMARY KEY REFERENCES events(id),
 default_capacity integer NOT NULL DEFAULT 2 CHECK (default_capacity BETWEEN 1 AND 10000)
);
CREATE TABLE mentor_profiles (
 event_id uuid NOT NULL REFERENCES events(id),
 person_id uuid NOT NULL REFERENCES people(id),
 skills text[] NOT NULL DEFAULT '{}',
 availability jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(availability)='array'),
 maximum_teams integer CHECK (maximum_teams BETWEEN 1 AND 10000),
 PRIMARY KEY(event_id,person_id)
);
CREATE TABLE mentor_conflicts (
 event_id uuid NOT NULL REFERENCES events(id),
 mentor_id uuid NOT NULL REFERENCES people(id),
 competitor_id uuid NOT NULL REFERENCES people(id),
 kind text NOT NULL CHECK (kind IN ('reporting','own_team')),
 PRIMARY KEY(event_id,mentor_id,competitor_id,kind)
);
CREATE TABLE mentor_team_skills (
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 skills text[] NOT NULL DEFAULT '{}',
 PRIMARY KEY(event_id,team_id),
 FOREIGN KEY(event_id,team_id) REFERENCES hack_teams(event_id,id)
);
CREATE TABLE mentor_assignments (
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 person_id uuid NOT NULL REFERENCES people(id),
 slot text NOT NULL CHECK (slot IN ('primary','second')),
 snapshot text NOT NULL CHECK (snapshot IN ('draft','published')),
 starts_at timestamptz NOT NULL,
 ends_at timestamptz NOT NULL CHECK (ends_at>starts_at),
 PRIMARY KEY(event_id,team_id,slot,snapshot),
 UNIQUE(event_id,team_id,person_id,snapshot),
 FOREIGN KEY(event_id,team_id) REFERENCES hack_teams(event_id,id)
);
CREATE INDEX mentor_assignments_person ON mentor_assignments(event_id,person_id,snapshot);

-- +goose Down
DROP TABLE mentor_assignments;
DROP TABLE mentor_team_skills;
DROP TABLE mentor_conflicts;
DROP TABLE mentor_profiles;
DROP TABLE mentor_settings;
