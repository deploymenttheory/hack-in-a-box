-- +goose Up
-- Provisional #118 number while real #115/#117 migrations are developed.
-- Final predecessor/ordered-upgrade proof is required before readiness.
ALTER TABLE operational_profiles ADD COLUMN photo_consent_revision bigint NOT NULL DEFAULT 1 CHECK(photo_consent_revision>0);
CREATE TABLE event_media (
 event_id uuid NOT NULL REFERENCES events(id),
 id uuid NOT NULL,
 owner_id uuid NOT NULL REFERENCES people(id),
 version bigint NOT NULL CHECK(version>0),
 state text NOT NULL CHECK(state IN('draft','reviewed','released','withdrawn')),
 released_version bigint,
 PRIMARY KEY(event_id,id)
);
CREATE TABLE media_versions (
 event_id uuid NOT NULL,
 id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>0),
 document jsonb NOT NULL,
 actor_id uuid REFERENCES people(id),
 action text NOT NULL,
 CHECK(actor_id IS NOT NULL OR action='consent_revoked'),
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 2000),
 created_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,id,version),
 FOREIGN KEY(event_id,id) REFERENCES event_media(event_id,id)
);
ALTER TABLE event_media ADD CONSTRAINT event_media_version FOREIGN KEY(event_id,id,version) REFERENCES media_versions(event_id,id,version) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE event_media ADD CONSTRAINT event_media_released FOREIGN KEY(event_id,id,released_version) REFERENCES media_versions(event_id,id,version) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE media_files (
 event_id uuid NOT NULL,
 media_id uuid NOT NULL,
 file_id uuid NOT NULL,
 PRIMARY KEY(event_id,file_id),
 UNIQUE(event_id,media_id),
 FOREIGN KEY(event_id,media_id) REFERENCES event_media(event_id,id),
 FOREIGN KEY(event_id,file_id) REFERENCES files(event_id,id)
);
CREATE TABLE media_commands (
 event_id uuid NOT NULL,
 id uuid NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 command_id uuid NOT NULL,
 fingerprint text NOT NULL,
 version bigint NOT NULL,
 PRIMARY KEY(event_id,actor_id,command_id),
 FOREIGN KEY(event_id,id,version) REFERENCES media_versions(event_id,id,version)
);
CREATE TRIGGER media_versions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON media_versions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER media_files_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON media_files FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER media_commands_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON media_commands FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE INDEX media_owner_page ON event_media(event_id,owner_id,id);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM event_media) OR EXISTS(SELECT 1 FROM media_versions) OR EXISTS(SELECT 1 FROM media_files) OR EXISTS(SELECT 1 FROM media_commands) THEN
  RAISE EXCEPTION 'Reviewed media/file/consent history exists; rollback requires a preservation plan';
 END IF;
 IF EXISTS(SELECT 1 FROM operational_profiles WHERE photo_consent_revision<>1) OR EXISTS(SELECT 1 FROM audit_events WHERE action IN ('photo_consent.granted','photo_consent.revoked','photo_consent.unknown')) THEN
  RAISE EXCEPTION 'Photo-consent revision history exists; rollback requires a preservation plan';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE media_commands,media_files;
ALTER TABLE event_media DROP CONSTRAINT event_media_version,DROP CONSTRAINT event_media_released;
DROP TABLE media_versions,event_media;
ALTER TABLE operational_profiles DROP COLUMN photo_consent_revision;
