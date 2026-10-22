-- +goose Up
CREATE TABLE content_acknowledgements (
 event_id uuid NOT NULL,
 content_id uuid NOT NULL,
 version bigint NOT NULL,
 person_id uuid NOT NULL REFERENCES people(id),
 acknowledged_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,content_id,version,person_id),
 FOREIGN KEY(event_id,content_id,version) REFERENCES content_versions(event_id,content_id,version)
);
CREATE TRIGGER content_acknowledgements_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON content_acknowledgements FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();

-- +goose Down
DROP TABLE content_acknowledgements;
