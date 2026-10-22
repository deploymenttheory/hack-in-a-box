-- +goose Up
CREATE TABLE team_credentials (
 id uuid PRIMARY KEY,
 event_id uuid NOT NULL REFERENCES events(id),
 label text NOT NULL CHECK (length(label) BETWEEN 1 AND 120),
 version bigint NOT NULL DEFAULT 1 CHECK (version>0),
 key_id text NOT NULL,
 ciphertext bytea NOT NULL,
 active boolean NOT NULL DEFAULT true,
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(event_id,id)
);
CREATE TABLE team_credential_assignments (
 event_id uuid NOT NULL,
 credential_id uuid NOT NULL,
 team_id uuid NOT NULL,
 PRIMARY KEY(event_id,credential_id,team_id),
 FOREIGN KEY(event_id,credential_id) REFERENCES team_credentials(event_id,id),
 FOREIGN KEY(event_id,team_id) REFERENCES hack_teams(event_id,id)
);
-- +goose Down
DROP TABLE team_credential_assignments;
DROP TABLE team_credentials;
