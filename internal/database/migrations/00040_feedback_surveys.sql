-- +goose Up
CREATE TABLE survey_heads (
 event_id uuid NOT NULL REFERENCES events(id), id uuid NOT NULL,
 current_version bigint NOT NULL CHECK(current_version>0), published_version bigint,
 cancelled boolean NOT NULL DEFAULT false, PRIMARY KEY(event_id,id)
);
CREATE TABLE survey_versions (
 event_id uuid NOT NULL, survey_id uuid NOT NULL, version bigint NOT NULL CHECK(version>0),
 document jsonb NOT NULL, actor_id uuid NOT NULL REFERENCES people(id),
 created_at timestamptz NOT NULL, PRIMARY KEY(event_id,survey_id,version),
 FOREIGN KEY(event_id,survey_id) REFERENCES survey_heads(event_id,id)
);
ALTER TABLE survey_heads ADD CONSTRAINT survey_current_version FOREIGN KEY(event_id,id,current_version) REFERENCES survey_versions(event_id,survey_id,version) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE survey_heads ADD CONSTRAINT survey_published_version FOREIGN KEY(event_id,id,published_version) REFERENCES survey_versions(event_id,survey_id,version) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE survey_approvals (
 event_id uuid NOT NULL, survey_id uuid NOT NULL, version bigint NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id), approved_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,survey_id,version), FOREIGN KEY(event_id,survey_id,version) REFERENCES survey_versions(event_id,survey_id,version)
);
CREATE TABLE survey_publications (
 event_id uuid NOT NULL, survey_id uuid NOT NULL, version bigint NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id), published_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,survey_id), FOREIGN KEY(event_id,survey_id,version) REFERENCES survey_versions(event_id,survey_id,version)
);
CREATE TABLE survey_invitations (
 event_id uuid NOT NULL, survey_id uuid NOT NULL, person_id uuid NOT NULL REFERENCES people(id),
 version bigint NOT NULL, invited_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,survey_id,person_id), FOREIGN KEY(event_id,survey_id,version) REFERENCES survey_versions(event_id,survey_id,version)
);
CREATE TABLE survey_response_heads (
 event_id uuid NOT NULL, survey_id uuid NOT NULL, person_id uuid NOT NULL REFERENCES people(id),
 revision bigint NOT NULL CHECK(revision>0), PRIMARY KEY(event_id,survey_id,person_id),
 FOREIGN KEY(event_id,survey_id,person_id) REFERENCES survey_invitations(event_id,survey_id,person_id)
);
CREATE TABLE survey_responses (
 event_id uuid NOT NULL, survey_id uuid NOT NULL, person_id uuid NOT NULL REFERENCES people(id),
 revision bigint NOT NULL CHECK(revision>0), form_version bigint NOT NULL, answers jsonb NOT NULL,
 submitted_at timestamptz NOT NULL, PRIMARY KEY(event_id,survey_id,person_id,revision),
 FOREIGN KEY(event_id,survey_id,person_id) REFERENCES survey_response_heads(event_id,survey_id,person_id),
 FOREIGN KEY(event_id,survey_id,form_version) REFERENCES survey_versions(event_id,survey_id,version)
);
CREATE TABLE survey_commands (
 event_id uuid NOT NULL, survey_id uuid NOT NULL, person_id uuid NOT NULL REFERENCES people(id),
 command_id uuid NOT NULL, payload_digest text NOT NULL, result_revision bigint NOT NULL,
 PRIMARY KEY(event_id,survey_id,person_id,command_id)
);
CREATE TABLE survey_report_snapshots (
 event_id uuid NOT NULL, survey_id uuid NOT NULL, survey_version bigint NOT NULL,
 response_digest text NOT NULL, document jsonb NOT NULL, created_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,survey_id), FOREIGN KEY(event_id,survey_id,survey_version) REFERENCES survey_versions(event_id,survey_id,version)
);
CREATE TABLE survey_report_reviews (
 event_id uuid NOT NULL, survey_id uuid NOT NULL, id uuid NOT NULL, version bigint NOT NULL CHECK(version>0),
 summary text NOT NULL, actor_id uuid NOT NULL REFERENCES people(id), reviewed_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,survey_id,id), UNIQUE(event_id,survey_id,version), FOREIGN KEY(event_id,survey_id) REFERENCES survey_report_snapshots(event_id,survey_id)
);
CREATE TABLE survey_report_approvals (
 event_id uuid NOT NULL, survey_id uuid NOT NULL, review_id uuid NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id), approved_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,survey_id,review_id), FOREIGN KEY(event_id,survey_id,review_id) REFERENCES survey_report_reviews(event_id,survey_id,id)
);
CREATE TABLE survey_report_publications (
 event_id uuid NOT NULL, survey_id uuid NOT NULL, version bigint NOT NULL CHECK(version>0), review_id uuid NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id), published_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,survey_id,version), UNIQUE(event_id,survey_id,review_id), FOREIGN KEY(event_id,survey_id,review_id) REFERENCES survey_report_reviews(event_id,survey_id,id)
);
CREATE TRIGGER survey_versions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON survey_versions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER survey_approvals_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON survey_approvals FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER survey_publications_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON survey_publications FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER survey_report_snapshots_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON survey_report_snapshots FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER survey_report_reviews_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON survey_report_reviews FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER survey_report_approvals_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON survey_report_approvals FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER survey_report_publications_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON survey_report_publications FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
-- Authored response history is immutable while retained, but explicit privacy
-- erasure must remove all identifiable revisions and commands, not just a head.
CREATE TRIGGER survey_responses_no_rewrite BEFORE UPDATE OR TRUNCATE ON survey_responses FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM survey_heads) OR EXISTS(SELECT 1 FROM survey_report_publications) THEN
  RAISE EXCEPTION 'Survey definitions, private responses or reviewed reports exist; preserve them before rollback';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE survey_report_publications,survey_report_approvals,survey_report_reviews,survey_report_snapshots,survey_commands,survey_responses,survey_response_heads,survey_invitations,survey_publications,survey_approvals;
ALTER TABLE survey_heads DROP CONSTRAINT survey_published_version, DROP CONSTRAINT survey_current_version;
DROP TABLE survey_versions,survey_heads;
