-- +goose Up
-- Optional recognition records; no accreditation or public verification directory.
CREATE TABLE certificate_settings (
 event_id uuid NOT NULL REFERENCES events(id),
 version bigint NOT NULL CHECK(version>0),
 request_id uuid NOT NULL,
 fingerprint text NOT NULL CHECK(fingerprint ~ '^[a-f0-9]{64}$'),
 document jsonb NOT NULL CHECK(jsonb_typeof(document)='object'),
 actor_id uuid NOT NULL REFERENCES people(id),
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 1000),
 created_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,version), UNIQUE(event_id,request_id)
);
CREATE TABLE certificate_versions (
 event_id uuid NOT NULL REFERENCES events(id),
 id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>0),
 person_id uuid NOT NULL REFERENCES people(id),
 recognition_key text NOT NULL,
 state text NOT NULL CHECK(state IN ('issued','revoked')),
 action text NOT NULL CHECK(action IN ('issue','correct','revoke')),
 request_id uuid NOT NULL,
 fingerprint text NOT NULL CHECK(fingerprint ~ '^[a-f0-9]{64}$'),
 document jsonb NOT NULL CHECK(jsonb_typeof(document)='object'),
 actor_id uuid NOT NULL REFERENCES people(id),
 reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 1000),
 created_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,id,version), UNIQUE(event_id,request_id)
);
CREATE TABLE certificate_heads (
 event_id uuid NOT NULL,
 id uuid NOT NULL,
 version bigint NOT NULL,
 person_id uuid NOT NULL REFERENCES people(id),
 recognition_key text NOT NULL,
 PRIMARY KEY(event_id,id),
 UNIQUE(event_id,person_id,recognition_key),
 FOREIGN KEY(event_id,id,version) REFERENCES certificate_versions(event_id,id,version)
);
CREATE TRIGGER certificate_settings_retained BEFORE UPDATE OR DELETE OR TRUNCATE ON certificate_settings FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER certificate_versions_retained BEFORE UPDATE OR DELETE OR TRUNCATE ON certificate_versions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM certificate_settings) OR EXISTS(SELECT 1 FROM certificate_versions) THEN
  RAISE EXCEPTION 'certificate decisions must be retained';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE certificate_heads;
DROP TABLE certificate_versions;
DROP TABLE certificate_settings;
