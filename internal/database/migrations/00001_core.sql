-- +goose Up
CREATE TABLE people (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    issuer text NOT NULL,
    subject text NOT NULL,
    display_name text NOT NULL DEFAULT '',
    active boolean NOT NULL DEFAULT true,
    platform_admin boolean NOT NULL DEFAULT false,
    session_revision bigint NOT NULL DEFAULT 1 CHECK (session_revision > 0),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (issuer, subject)
);

-- Event lifecycle and registration policies are added by their domain tickets.
CREATE TABLE events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 160),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE events;
DROP TABLE people;
