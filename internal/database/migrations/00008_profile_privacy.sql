-- +goose Up
CREATE TABLE event_privacy (
    event_id uuid PRIMARY KEY REFERENCES events(id),
    retention_days integer NOT NULL DEFAULT 90 CHECK(retention_days BETWEEN 1 AND 3650),
    draft_days integer NOT NULL DEFAULT 7 CHECK(draft_days BETWEEN 1 AND 90)
);
CREATE TABLE operational_profiles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id uuid NOT NULL REFERENCES events(id),
    person_id uuid NOT NULL REFERENCES people(id),
    contact_email text NOT NULL DEFAULT '',
    contact_phone text NOT NULL DEFAULT '',
    dietary text NOT NULL DEFAULT '',
    accessibility text NOT NULL DEFAULT '',
    photo_consent text NOT NULL DEFAULT 'unknown' CHECK(photo_consent IN ('unknown','yes','no')),
    revision bigint NOT NULL DEFAULT 1,
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(event_id,person_id)
);
CREATE TABLE private_drafts (
    id uuid PRIMARY KEY,
    event_id uuid NOT NULL REFERENCES events(id),
    person_id uuid NOT NULL REFERENCES people(id),
    body bytea NOT NULL,
    expires_at timestamptz NOT NULL,
    CHECK(octet_length(body)<=65536)
);
CREATE INDEX private_drafts_expiry ON private_drafts(expires_at);
CREATE TABLE privacy_aggregates (
    event_id uuid PRIMARY KEY REFERENCES events(id),
    removed_profiles bigint NOT NULL DEFAULT 0
);
UPDATE events SET completed_at=updated_at WHERE stage='cancelled' AND completed_at IS NULL;

-- +goose Down
DROP TABLE privacy_aggregates;
DROP TABLE private_drafts;
DROP TABLE operational_profiles;
DROP TABLE event_privacy;
