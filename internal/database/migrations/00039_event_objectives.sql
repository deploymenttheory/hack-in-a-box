-- Normal ordered upgrade after the concrete schema38 delivery-history source.
-- +goose Up
CREATE TABLE event_objectives (
 event_id uuid NOT NULL REFERENCES events(id),
 id uuid NOT NULL,
 current_version bigint NOT NULL CHECK(current_version>0),
 PRIMARY KEY(event_id,id)
);
CREATE TABLE objective_versions (
 event_id uuid NOT NULL,
 objective_id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>0),
 definition jsonb NOT NULL,
 retired boolean NOT NULL,
 reason text NOT NULL CHECK(octet_length(reason) BETWEEN 1 AND 4000),
 actor_id uuid NOT NULL REFERENCES people(id),
 created_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,objective_id,version),
 FOREIGN KEY(event_id,objective_id) REFERENCES event_objectives(event_id,id)
);
ALTER TABLE event_objectives ADD CONSTRAINT objective_current_version
 FOREIGN KEY(event_id,id,current_version) REFERENCES objective_versions(event_id,objective_id,version)
 DEFERRABLE INITIALLY DEFERRED;
CREATE TRIGGER objective_versions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON objective_versions
 FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM event_objectives) THEN
  RAISE EXCEPTION 'Objective definition history must be preserved before rollback';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE event_objectives DROP CONSTRAINT objective_current_version;
DROP TABLE objective_versions;
DROP TABLE event_objectives;
