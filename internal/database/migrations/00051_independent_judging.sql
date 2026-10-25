-- +goose Up
CREATE TABLE judge_rounds (
 event_id uuid NOT NULL REFERENCES events(id),
 run_id uuid NOT NULL,
 revision bigint NOT NULL DEFAULT 0,
 deliberation boolean NOT NULL DEFAULT false,
 PRIMARY KEY(event_id,run_id),
 FOREIGN KEY(event_id,run_id) REFERENCES rubric_assessment_runs(event_id,id)
);
CREATE TABLE judge_assignments (
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 person_id uuid NOT NULL REFERENCES people(id),
 active boolean NOT NULL,
 PRIMARY KEY(event_id,team_id,person_id),
 FOREIGN KEY(event_id,team_id) REFERENCES competition_entries(event_id,team_id)
);
CREATE TABLE judge_cards (
 event_id uuid NOT NULL,
 run_id uuid NOT NULL,
 team_id uuid NOT NULL,
 author_id uuid NOT NULL REFERENCES people(id),
 version bigint NOT NULL CHECK(version>0),
 document jsonb NOT NULL,
 PRIMARY KEY(event_id,run_id,team_id,author_id),
 FOREIGN KEY(event_id,run_id) REFERENCES judge_rounds(event_id,run_id),
 FOREIGN KEY(event_id,team_id) REFERENCES competition_entries(event_id,team_id)
);
CREATE TABLE judge_history (
 id bigserial PRIMARY KEY,
 event_id uuid NOT NULL REFERENCES events(id),
 run_id uuid,
 team_id uuid,
 actor_id uuid NOT NULL REFERENCES people(id),
 action text NOT NULL,
 reason text NOT NULL,
 document jsonb NOT NULL,
 occurred_at timestamptz NOT NULL
);
CREATE TABLE judge_commands (
 event_id uuid NOT NULL REFERENCES events(id),
 request_id uuid NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 fingerprint text NOT NULL,
 result jsonb NOT NULL,
 PRIMARY KEY(event_id,request_id)
);
CREATE TABLE judge_practice_cards (
 event_id uuid NOT NULL REFERENCES events(id),
 actor_id uuid NOT NULL REFERENCES people(id),
 version bigint NOT NULL CHECK(version>0),
 document jsonb NOT NULL,
 PRIMARY KEY(event_id,actor_id)
);
CREATE TABLE judge_result_approvals (
 id uuid PRIMARY KEY,
 event_id uuid NOT NULL,
 run_id uuid NOT NULL,
 binding text NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 reason text NOT NULL,
 created_at timestamptz NOT NULL,
 FOREIGN KEY(event_id,run_id) REFERENCES judge_rounds(event_id,run_id)
);
CREATE TABLE judge_result_invalidations (
 approval_id uuid PRIMARY KEY REFERENCES judge_result_approvals(id),
 actor_id uuid NOT NULL REFERENCES people(id),
 reason text NOT NULL,
 created_at timestamptz NOT NULL
);
CREATE TRIGGER judge_history_retained BEFORE UPDATE OR DELETE OR TRUNCATE ON judge_history FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER judge_commands_retained BEFORE UPDATE OR DELETE OR TRUNCATE ON judge_commands FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER judge_result_approvals_retained BEFORE UPDATE OR DELETE OR TRUNCATE ON judge_result_approvals FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER judge_result_invalidations_retained BEFORE UPDATE OR DELETE OR TRUNCATE ON judge_result_invalidations FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM judge_history) OR EXISTS(SELECT 1 FROM judge_commands) OR EXISTS(SELECT 1 FROM judge_cards) OR EXISTS(SELECT 1 FROM judge_practice_cards) OR EXISTS(SELECT 1 FROM judge_result_approvals) THEN
 RAISE EXCEPTION 'Independent judge history and approvals must be retained';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE judge_result_invalidations;
DROP TABLE judge_result_approvals;
DROP TABLE judge_practice_cards;
DROP TABLE judge_commands;
DROP TABLE judge_history;
DROP TABLE judge_cards;
DROP TABLE judge_assignments;
DROP TABLE judge_rounds;
