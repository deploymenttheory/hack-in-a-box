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

-- +goose StatementBegin
CREATE FUNCTION protect_inbox_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF ROW(NEW.id,NEW.event_id,NEW.recipient_id,NEW.source_kind,NEW.source_id,NEW.source_version,NEW.occurrence_id,NEW.scope_binding,NEW.provenance,NEW.subject,NEW.body,NEW.format,NEW.created_at)
    IS DISTINCT FROM ROW(OLD.id,OLD.event_id,OLD.recipient_id,OLD.source_kind,OLD.source_id,OLD.source_version,OLD.occurrence_id,OLD.scope_binding,OLD.provenance,OLD.subject,OLD.body,OLD.format,OLD.created_at) THEN
  RAISE EXCEPTION 'Inbox recipient, source provenance and delivered content are immutable';
 END IF;
 RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER inbox_identity BEFORE UPDATE ON inbox_notifications FOR EACH ROW EXECUTE FUNCTION protect_inbox_identity();

-- +goose Down
DROP TABLE inbox_notifications;
DROP FUNCTION protect_inbox_identity();
