-- +goose Up
CREATE TABLE teams_workflow_connections (
 id uuid PRIMARY KEY,
 event_id uuid NOT NULL UNIQUE REFERENCES events(id),
 version bigint NOT NULL CHECK(version>0),
 label text NOT NULL CHECK(length(label) BETWEEN 1 AND 160),
 owner_label text NOT NULL CHECK(length(owner_label) BETWEEN 1 AND 160),
 co_owner_label text NOT NULL CHECK(length(co_owner_label) BETWEEN 1 AND 160),
 key_id text NOT NULL,
 ciphertext bytea NOT NULL,
 active boolean NOT NULL,
 retry_after timestamptz,
 updated_at timestamptz NOT NULL,
 UNIQUE(event_id,id)
);
CREATE TABLE teams_workflow_deliveries (
 event_id uuid NOT NULL REFERENCES events(id),
 id uuid NOT NULL,
 operation_id uuid NOT NULL UNIQUE REFERENCES operations(id),
 kind text NOT NULL CHECK(kind IN ('connection_test','broadcast')),
 connection_id uuid NOT NULL,
 connection_version bigint NOT NULL CHECK(connection_version>0),
 channel_revision text NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 campaign_id uuid,
 campaign_version bigint,
 preview_id uuid,
 preview_fingerprint text,
 request_id uuid NOT NULL,
 created_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,id),
 UNIQUE(event_id,request_id),
 FOREIGN KEY(event_id,connection_id) REFERENCES teams_workflow_connections(event_id,id),
 FOREIGN KEY(event_id,campaign_id,campaign_version) REFERENCES communication_campaign_versions(event_id,id,version),
 FOREIGN KEY(event_id,preview_id) REFERENCES communication_previews(event_id,id),
 CHECK((kind='connection_test' AND campaign_id IS NULL AND campaign_version IS NULL AND preview_id IS NULL AND preview_fingerprint IS NULL) OR
       (kind='broadcast' AND campaign_id IS NOT NULL AND campaign_version IS NOT NULL AND preview_id IS NOT NULL AND preview_fingerprint IS NOT NULL))
);
CREATE UNIQUE INDEX teams_workflow_campaign_once ON teams_workflow_deliveries(event_id,campaign_id,campaign_version) WHERE kind='broadcast';
CREATE TABLE teams_workflow_attempts (
 event_id uuid NOT NULL,
 id uuid NOT NULL,
 delivery_id uuid NOT NULL,
 attempt_number integer NOT NULL CHECK(attempt_number BETWEEN 1 AND 5),
 connection_version bigint NOT NULL,
 payload_fingerprint text NOT NULL,
 started_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,id),
 UNIQUE(event_id,delivery_id,attempt_number),
 FOREIGN KEY(event_id,delivery_id) REFERENCES teams_workflow_deliveries(event_id,id)
);
CREATE TABLE teams_workflow_receipts (
 event_id uuid NOT NULL,
 attempt_id uuid NOT NULL,
 outcome text NOT NULL CHECK(outcome IN ('provider_accepted','retryable','permanent','ambiguous')),
 http_status integer CHECK(http_status BETWEEN 100 AND 599),
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 100),
 retry_at timestamptz,
 recorded_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,attempt_id),
 FOREIGN KEY(event_id,attempt_id) REFERENCES teams_workflow_attempts(event_id,id)
);
CREATE TRIGGER teams_workflow_deliveries_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON teams_workflow_deliveries FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER teams_workflow_attempts_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON teams_workflow_attempts FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER teams_workflow_receipts_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON teams_workflow_receipts FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM teams_workflow_connections) OR EXISTS(SELECT 1 FROM teams_workflow_deliveries) THEN
  RAISE EXCEPTION 'Teams connection or delivery history exists; rollback requires an explicit preservation plan';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE teams_workflow_receipts;
DROP TABLE teams_workflow_attempts;
DROP TABLE teams_workflow_deliveries;
DROP TABLE teams_workflow_connections;
