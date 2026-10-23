-- +goose Up
-- Webhook secrets have a separate cryptographic purpose from App signing keys.
CREATE TABLE github_webhook_secrets (
 event_id uuid PRIMARY KEY REFERENCES github_app_integrations(event_id) ON DELETE CASCADE,
 app_id bigint NOT NULL CHECK(app_id>0),
 version bigint NOT NULL CHECK(version>0),
 key_id text NOT NULL,
 encrypted bytea NOT NULL
);
CREATE TABLE github_webhook_deliveries (
 id uuid PRIMARY KEY,
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 repository_id bigint NOT NULL CHECK(repository_id>0),
 app_id bigint NOT NULL,
 installation_id bigint NOT NULL,
 provenance uuid NOT NULL,
 secret_version bigint NOT NULL,
 payload_hash bytea NOT NULL CHECK(octet_length(payload_hash)=32),
 kind text NOT NULL CHECK(kind IN ('push','member')),
 action text NOT NULL DEFAULT '' CHECK(action IN ('','added','removed','edited')),
 ref text NOT NULL DEFAULT '' CHECK(length(ref)<=1024),
 after_sha text NOT NULL DEFAULT '' CHECK(length(after_sha)<=64),
 member_id bigint NOT NULL DEFAULT 0 CHECK(member_id>=0),
 provider_at timestamptz,
 received_at timestamptz NOT NULL DEFAULT now(),
 applied_at timestamptz,
 disposition text NOT NULL DEFAULT 'pending' CHECK(disposition IN ('pending','recorded','discarded')),
 operation_id uuid UNIQUE REFERENCES operations(id),
 FOREIGN KEY(event_id,team_id) REFERENCES team_repositories(event_id,team_id)
);
CREATE INDEX github_webhook_event_received ON github_webhook_deliveries(event_id,received_at DESC);
-- Only additive informational aggregates: no branch head, entry status or score.
CREATE TABLE github_repository_activity (
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 pushes bigint NOT NULL DEFAULT 0,
 invitations bigint NOT NULL DEFAULT 0,
 last_provider_at timestamptz,
 last_received_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,team_id),
 FOREIGN KEY(event_id,team_id) REFERENCES team_repositories(event_id,team_id)
);
-- +goose Down
DROP TABLE github_repository_activity;
DROP TABLE github_webhook_deliveries;
DROP TABLE github_webhook_secrets;
