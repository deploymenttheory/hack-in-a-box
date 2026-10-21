-- +goose Up
CREATE TABLE hack_teams (
 id uuid PRIMARY KEY,
 event_id uuid NOT NULL REFERENCES events(id),
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 160),
 invitation_code uuid NOT NULL UNIQUE DEFAULT gen_random_uuid(),
 captain_id uuid REFERENCES people(id),
 published_captain_id uuid REFERENCES people(id),
 state text NOT NULL DEFAULT 'draft' CHECK(state IN ('draft','published','disbanded')),
 frozen boolean NOT NULL DEFAULT false,
 UNIQUE(event_id,id)
);
CREATE TABLE hack_team_members (
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 person_id uuid NOT NULL REFERENCES people(id),
 snapshot text NOT NULL CHECK(snapshot IN ('draft','published')),
 PRIMARY KEY(event_id,person_id,snapshot),
 FOREIGN KEY(event_id,team_id) REFERENCES hack_teams(event_id,id)
);
CREATE TABLE hack_team_requests (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 person_id uuid NOT NULL REFERENCES people(id),
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','approved','declined')),
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(event_id,team_id) REFERENCES hack_teams(event_id,id)
);
CREATE UNIQUE INDEX one_pending_team_request ON hack_team_requests(event_id,person_id) WHERE state='pending';
CREATE TABLE competition_entries (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 active boolean NOT NULL DEFAULT true,
 UNIQUE(event_id,team_id),
 FOREIGN KEY(event_id,team_id) REFERENCES hack_teams(event_id,id)
);
-- +goose Down
DROP TABLE competition_entries;
DROP TABLE hack_team_requests;
DROP TABLE hack_team_members;
DROP TABLE hack_teams;
