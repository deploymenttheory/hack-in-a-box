-- +goose Up
CREATE TABLE presentation_states (
 event_id uuid PRIMARY KEY REFERENCES events(id),
 revision bigint NOT NULL DEFAULT 0 CHECK(revision>=0),
 draft jsonb NOT NULL DEFAULT '[]',
 published jsonb NOT NULL DEFAULT '[]',
 current_index integer NOT NULL DEFAULT -1 CHECK(current_index>=-1),
 mode text NOT NULL DEFAULT 'held' CHECK(mode IN ('held','running')),
 remaining_seconds integer NOT NULL DEFAULT 0 CHECK(remaining_seconds BETWEEN 0 AND 86400),
 timer_ends_at timestamptz,
 override_cue jsonb,
 override_mode text NOT NULL DEFAULT 'held' CHECK(override_mode IN ('held','running')),
 override_remaining_seconds integer NOT NULL DEFAULT 0 CHECK(override_remaining_seconds BETWEEN 0 AND 86400),
 override_timer_ends_at timestamptz
);
CREATE TABLE display_pairings (
 event_id uuid NOT NULL REFERENCES events(id),
 id uuid NOT NULL,
 label text NOT NULL CHECK(length(label) BETWEEN 1 AND 100),
 token_hash bytea NOT NULL CHECK(octet_length(token_hash)=32),
 issuer_id uuid NOT NULL REFERENCES people(id),
 created_at timestamptz NOT NULL,
 expires_at timestamptz NOT NULL,
 revoked_at timestamptz,
 last_seen_at timestamptz,
 PRIMARY KEY(event_id,id),
 UNIQUE(token_hash),
 CHECK(expires_at>created_at)
);
CREATE INDEX active_display_pairings ON display_pairings(event_id,expires_at) WHERE revoked_at IS NULL;

-- +goose Down
DROP TABLE display_pairings;
DROP TABLE presentation_states;
