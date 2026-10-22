-- +goose Up
CREATE TABLE inbox_notifications (
 id uuid PRIMARY KEY,
 event_id uuid NOT NULL REFERENCES events(id),
 recipient_id uuid NOT NULL REFERENCES people(id),
 source_kind text NOT NULL CHECK(source_kind IN ('campaign','test_self','content','help','clarification','outcome_review')),
 source_id uuid NOT NULL,
 occurrence_id uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
 scope_binding text NOT NULL DEFAULT '',
 source_version bigint NOT NULL CHECK(source_version > 0),
 provenance jsonb NOT NULL DEFAULT '{}'::jsonb,
 subject text NOT NULL CHECK(octet_length(subject) <= 4000),
 body text NOT NULL CHECK(octet_length(body) <= 200000),
 format text NOT NULL CHECK(format IN ('text','markdown','html')),
 created_at timestamptz NOT NULL DEFAULT now(),
 read_at timestamptz,
 UNIQUE(event_id,recipient_id,source_kind,source_id,source_version,occurrence_id)
);
CREATE INDEX inbox_recipient ON inbox_notifications(recipient_id,id DESC);

-- +goose Down
DROP TABLE inbox_notifications;
