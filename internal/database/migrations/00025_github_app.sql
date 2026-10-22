-- +goose Up
CREATE TABLE github_app_integrations (
 event_id uuid PRIMARY KEY REFERENCES events(id),
 app_id bigint NOT NULL CHECK(app_id>0),
 app_slug text NOT NULL,
 organization_id bigint NOT NULL CHECK(organization_id>0),
 organization text NOT NULL,
 template_repository text NOT NULL,
 credential_version bigint NOT NULL CHECK(credential_version>0),
 key_id text NOT NULL,
 private_key_encrypted bytea NOT NULL,
 installation_id bigint NOT NULL DEFAULT 0 CHECK(installation_id>=0),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE github_app_install_attempts (
 state_hash bytea PRIMARY KEY,
 event_id uuid NOT NULL REFERENCES events(id),
 actor_id uuid NOT NULL REFERENCES people(id),
 session_hash text NOT NULL,
 session_revision bigint NOT NULL,
 event_revision bigint NOT NULL,
 expires_at timestamptz NOT NULL
);
CREATE INDEX github_app_attempt_expiry ON github_app_install_attempts(expires_at);
-- +goose Down
DROP TABLE github_app_install_attempts;
DROP TABLE github_app_integrations;
