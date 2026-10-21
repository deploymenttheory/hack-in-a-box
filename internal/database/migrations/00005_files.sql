-- +goose Up
CREATE TABLE files (
    id uuid PRIMARY KEY,
    event_id uuid NOT NULL REFERENCES events(id),
    owner_id uuid NOT NULL REFERENCES people(id),
    idempotency_key uuid NOT NULL,
    object_key text NOT NULL UNIQUE CHECK (object_key ~ '^[a-f0-9]{64}$'),
    filename text NOT NULL CHECK (length(filename) BETWEEN 1 AND 160),
    media_type text NOT NULL CHECK (media_type IN ('application/pdf','image/png','image/jpeg','text/plain')),
    checksum text NOT NULL CHECK (checksum ~ '^[a-f0-9]{64}$'),
    size_bytes bigint NOT NULL CHECK (size_bytes > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (event_id, owner_id, idempotency_key),
    UNIQUE (event_id, id)
);
CREATE INDEX files_event ON files(event_id);

-- +goose Down
DROP TABLE files;
