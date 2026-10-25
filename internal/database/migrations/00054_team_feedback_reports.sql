-- +goose Up
CREATE TABLE team_feedback_versions (
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>0),
 state text NOT NULL CHECK(state IN ('draft','released')),
 author_id uuid NOT NULL REFERENCES people(id),
 actor_id uuid NOT NULL REFERENCES people(id),
 judge_run_id uuid NOT NULL,
 confirmation_id uuid NOT NULL,
 document jsonb NOT NULL CHECK(jsonb_typeof(document)='object'),
 changed_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,team_id,version),
 FOREIGN KEY(event_id,team_id) REFERENCES competition_entries(event_id,team_id),
 FOREIGN KEY(event_id,judge_run_id,confirmation_id) REFERENCES judge_result_approvals(event_id,run_id,id)
);
CREATE TABLE team_feedback_commands (
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 request_id uuid NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 version bigint NOT NULL,
 receipt jsonb NOT NULL CHECK(jsonb_typeof(receipt)='object'),
 PRIMARY KEY(event_id,request_id),
 FOREIGN KEY(event_id,team_id,version) REFERENCES team_feedback_versions(event_id,team_id,version)
);
CREATE TRIGGER team_feedback_versions_retained BEFORE UPDATE OR DELETE OR TRUNCATE ON team_feedback_versions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER team_feedback_commands_retained BEFORE UPDATE OR DELETE OR TRUNCATE ON team_feedback_commands FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM team_feedback_versions) OR EXISTS(SELECT 1 FROM team_feedback_commands) THEN
  RAISE EXCEPTION 'team feedback author/audience/publication history must be retained';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE team_feedback_commands;
DROP TABLE team_feedback_versions;
