-- +goose Up
CREATE TABLE github_link_states (
 person_id uuid PRIMARY KEY REFERENCES people(id),
 revision bigint NOT NULL DEFAULT 0
);
CREATE TABLE github_accounts (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 person_id uuid NOT NULL UNIQUE REFERENCES people(id),
 github_id bigint NOT NULL UNIQUE CHECK(github_id>0),
 login text NOT NULL,
 token_encrypted bytea NOT NULL,
 verified_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE github_link_attempts (
 state_hash bytea PRIMARY KEY,
 person_id uuid NOT NULL REFERENCES people(id),
 session_hash bytea NOT NULL,
 verifier_encrypted bytea NOT NULL,
 link_revision bigint NOT NULL,
 session_revision bigint NOT NULL,
 expires_at timestamptz NOT NULL
);
CREATE INDEX github_link_attempt_expiry ON github_link_attempts(expires_at);
-- Future repository jobs bind to the verified ID and revision, never just a login.
CREATE TABLE github_invitation_bindings (
 id uuid PRIMARY KEY,
 event_id uuid NOT NULL REFERENCES events(id),
 person_id uuid NOT NULL REFERENCES people(id),
 github_id bigint NOT NULL,
 link_revision bigint NOT NULL,
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','requires_review','delivered')),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX github_pending_invitation_person ON github_invitation_bindings(person_id) WHERE state='pending';

-- +goose Down
DROP TABLE github_invitation_bindings;
DROP TABLE github_link_attempts;
DROP TABLE github_accounts;
DROP TABLE github_link_states;
