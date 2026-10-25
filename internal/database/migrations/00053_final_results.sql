-- +goose Up
ALTER TABLE judge_result_approvals ADD CONSTRAINT result_exact_judge_approval UNIQUE(event_id,run_id,id);
CREATE TABLE result_versions (
 event_id uuid NOT NULL REFERENCES events(id),
 version bigint NOT NULL CHECK(version>0),
 judge_run_id uuid,
 judge_approval_id uuid,
 qualification_run_id uuid,
 qualification_version bigint,
 actor_id uuid NOT NULL REFERENCES people(id),
 action text NOT NULL,
 reason text NOT NULL CHECK(length(btrim(reason))>0),
 document jsonb NOT NULL CHECK(jsonb_typeof(document)='object'),
 changed_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,version),
 CHECK(judge_approval_id IS NULL OR judge_run_id IS NOT NULL),
 CHECK((qualification_run_id IS NULL AND qualification_version IS NULL) OR (qualification_run_id IS NOT NULL AND qualification_version IS NOT NULL AND qualification_version>0)),
 FOREIGN KEY(event_id,judge_run_id) REFERENCES rubric_assessment_runs(event_id,id),
 FOREIGN KEY(event_id,judge_run_id,judge_approval_id) REFERENCES judge_result_approvals(event_id,run_id,id),
 FOREIGN KEY(event_id,qualification_run_id,qualification_version) REFERENCES mentor_qualification_rounds(event_id,run_id,version)
);
CREATE TABLE result_commands (
 event_id uuid NOT NULL,
 request_id uuid NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 result_version bigint NOT NULL,
 receipt jsonb NOT NULL CHECK(jsonb_typeof(receipt)='object'),
 PRIMARY KEY(event_id,request_id),
 FOREIGN KEY(event_id,result_version) REFERENCES result_versions(event_id,version)
);
-- A publication row is the single authoritative communication/display event.
-- Its document contains only deliberately released award/finalist projection.
CREATE TABLE result_publications (
 event_id uuid NOT NULL,
 id uuid NOT NULL,
 kind text NOT NULL CHECK(kind IN ('finalists','awards')),
 result_version bigint NOT NULL,
 source_binding text NOT NULL CHECK(source_binding ~ '^[a-f0-9]{64}$'),
 document jsonb NOT NULL CHECK(jsonb_typeof(document)='object'),
 actor_id uuid NOT NULL REFERENCES people(id),
 published_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,id),
 UNIQUE(event_id,kind,source_binding),
 FOREIGN KEY(event_id,result_version) REFERENCES result_versions(event_id,version)
);
CREATE TRIGGER result_versions_retained BEFORE UPDATE OR DELETE OR TRUNCATE ON result_versions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER result_commands_retained BEFORE UPDATE OR DELETE OR TRUNCATE ON result_commands FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER result_publications_retained BEFORE UPDATE OR DELETE OR TRUNCATE ON result_publications FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM result_versions) OR EXISTS(SELECT 1 FROM result_commands) OR EXISTS(SELECT 1 FROM result_publications) THEN
  RAISE EXCEPTION 'result decisions and publications must be retained';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE result_publications;
DROP TABLE result_commands;
DROP TABLE result_versions;
ALTER TABLE judge_result_approvals DROP CONSTRAINT result_exact_judge_approval;
