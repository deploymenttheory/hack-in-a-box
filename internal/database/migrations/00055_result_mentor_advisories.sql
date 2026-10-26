-- +goose Up
CREATE TABLE result_mentor_advisory_versions (
 event_id uuid NOT NULL,
 tie_id text NOT NULL CHECK(tie_id ~ '^[a-f0-9]{64}$'),
 team_id uuid NOT NULL,
 author_id uuid NOT NULL REFERENCES people(id),
 version bigint NOT NULL CHECK(version>0),
 judge_run_id uuid NOT NULL,
 source_binding text NOT NULL CHECK(source_binding ~ '^[a-f0-9]{64}$'),
 document jsonb NOT NULL CHECK(jsonb_typeof(document)='object'),
 PRIMARY KEY(event_id,tie_id,team_id,author_id,version),
 FOREIGN KEY(event_id,team_id) REFERENCES competition_entries(event_id,team_id),
 FOREIGN KEY(event_id,judge_run_id) REFERENCES rubric_assessment_runs(event_id,id)
);
CREATE TABLE result_mentor_advisory_commands (
 event_id uuid NOT NULL,
 request_id uuid NOT NULL,
 tie_id text NOT NULL,
 team_id uuid NOT NULL,
 author_id uuid NOT NULL,
 version bigint NOT NULL,
 receipt jsonb NOT NULL CHECK(jsonb_typeof(receipt)='object'),
 PRIMARY KEY(event_id,request_id),
 FOREIGN KEY(event_id,tie_id,team_id,author_id,version) REFERENCES result_mentor_advisory_versions(event_id,tie_id,team_id,author_id,version)
);
CREATE TRIGGER result_mentor_advisory_versions_retained BEFORE UPDATE OR DELETE OR TRUNCATE ON result_mentor_advisory_versions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER result_mentor_advisory_commands_retained BEFORE UPDATE OR DELETE OR TRUNCATE ON result_mentor_advisory_commands FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM result_mentor_advisory_versions) OR EXISTS(SELECT 1 FROM result_mentor_advisory_commands) THEN
  RAISE EXCEPTION 'mentor advisory author/correction/receipt history must be retained';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE result_mentor_advisory_commands;
DROP TABLE result_mentor_advisory_versions;
