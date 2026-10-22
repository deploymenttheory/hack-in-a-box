-- +goose Up
CREATE TABLE content_items (
 event_id uuid NOT NULL REFERENCES events(id),
 id uuid NOT NULL,
 kind text NOT NULL CHECK(kind IN ('page','brief','challenge')),
 revision bigint NOT NULL DEFAULT 0,
 version bigint NOT NULL DEFAULT 0,
 state text NOT NULL DEFAULT 'draft' CHECK(state IN ('draft','review','approved','released','cancelled','withdrawn','missed','blocked')),
 released_version bigint,
 scheduled_at timestamptz,
 approved_by uuid REFERENCES people(id),
 approved_revision bigint,
 released_at timestamptz,
 PRIMARY KEY(event_id,id)
);
CREATE UNIQUE INDEX common_challenge ON content_items(event_id) WHERE kind='challenge';
CREATE TABLE content_versions (
 event_id uuid NOT NULL,
 content_id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>0),
 document jsonb NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 created_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,content_id,version),
 FOREIGN KEY(event_id,content_id) REFERENCES content_items(event_id,id)
);
ALTER TABLE content_items ADD CONSTRAINT released_content_version FOREIGN KEY(event_id,id,released_version) REFERENCES content_versions(event_id,content_id,version);
CREATE TABLE content_files (
 event_id uuid NOT NULL,
 content_id uuid NOT NULL,
 version bigint NOT NULL,
 file_id uuid NOT NULL,
 PRIMARY KEY(event_id,content_id,version,file_id),
 FOREIGN KEY(event_id,content_id,version) REFERENCES content_versions(event_id,content_id,version),
 FOREIGN KEY(event_id,file_id) REFERENCES files(event_id,id)
);
CREATE TABLE content_changes (
 event_id uuid NOT NULL,
 content_id uuid NOT NULL,
 revision bigint NOT NULL,
 version bigint NOT NULL,
 action text NOT NULL,
 actor_id uuid REFERENCES people(id),
 reason text NOT NULL DEFAULT '',
 scheduled_at timestamptz,
 occurred_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,content_id,revision),
 FOREIGN KEY(event_id,content_id,version) REFERENCES content_versions(event_id,content_id,version)
);
-- One durable release event per immutable version, committed with its visibility.
CREATE UNIQUE INDEX content_release_once ON content_changes(event_id,content_id,version) WHERE action='released';
CREATE INDEX due_content ON content_items(scheduled_at) WHERE state='approved';
CREATE TRIGGER content_versions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON content_versions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER content_changes_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON content_changes FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER content_files_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON content_files FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();

-- +goose Down
DROP TABLE content_changes;
DROP TABLE content_files;
ALTER TABLE content_items DROP CONSTRAINT released_content_version;
DROP TABLE content_versions;
DROP TABLE content_items;
