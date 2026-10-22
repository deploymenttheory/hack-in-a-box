-- +goose Up
CREATE TABLE communication_campaign_versions (
 event_id uuid NOT NULL REFERENCES events(id),
 id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>0),
 document jsonb NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 2000),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(event_id,id,version)
);
CREATE TABLE communication_test_requests (
 event_id uuid NOT NULL REFERENCES events(id),
 id uuid NOT NULL,
 campaign_id uuid NOT NULL,
 campaign_version bigint NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 channel text NOT NULL CHECK(channel IN ('smtp','in_app')),
 snapshot jsonb NOT NULL,
 fingerprint text NOT NULL,
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 2000),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(event_id,id),
 FOREIGN KEY(event_id,campaign_id,campaign_version) REFERENCES communication_campaign_versions(event_id,id,version)
);
CREATE TRIGGER communication_campaign_versions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON communication_campaign_versions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER communication_test_requests_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON communication_test_requests FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
-- +goose Down
DROP TABLE communication_test_requests;
DROP TABLE communication_campaign_versions;
