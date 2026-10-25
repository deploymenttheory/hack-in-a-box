-- +goose Up
CREATE TABLE submission_branch_configurations (
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 revision bigint NOT NULL CHECK(revision>0),
 repository_id bigint NOT NULL CHECK(repository_id>0),
 provenance uuid NOT NULL,
 branch text NOT NULL CHECK(length(branch) BETWEEN 1 AND 1000),
 actor_id uuid NOT NULL REFERENCES people(id),
 PRIMARY KEY(event_id,team_id),
 FOREIGN KEY(event_id,team_id) REFERENCES competition_entries(event_id,team_id)
);
CREATE TABLE submission_freeze_heads (
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 revision bigint NOT NULL DEFAULT 0 CHECK(revision>=0),
 PRIMARY KEY(event_id,team_id),
 FOREIGN KEY(event_id,team_id) REFERENCES competition_entries(event_id,team_id)
);
CREATE TABLE submission_freeze_sources (
 id uuid PRIMARY KEY,
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 cutoff timestamptz NOT NULL,
 binding jsonb NOT NULL,
 definition jsonb NOT NULL,
 UNIQUE(event_id,team_id),
 FOREIGN KEY(event_id,team_id) REFERENCES submission_freeze_heads(event_id,team_id)
);
CREATE TABLE submission_freeze_observations (
 id uuid PRIMARY KEY,
 source_id uuid NOT NULL REFERENCES submission_freeze_sources(id),
 observed_at timestamptz NOT NULL,
 definition jsonb NOT NULL
);
CREATE TABLE submission_freeze_decisions (
 id uuid PRIMARY KEY,
 source_id uuid NOT NULL REFERENCES submission_freeze_sources(id),
 revision bigint NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 decided_at timestamptz NOT NULL,
 definition jsonb NOT NULL,
 UNIQUE(source_id,revision)
);
CREATE TABLE submission_freeze_commands (
 event_id uuid NOT NULL,
 request_id uuid NOT NULL,
 team_id uuid NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 fingerprint text NOT NULL,
 result jsonb NOT NULL,
 PRIMARY KEY(event_id,request_id),
 FOREIGN KEY(event_id,team_id) REFERENCES submission_freeze_heads(event_id,team_id)
);
CREATE TRIGGER submission_freeze_sources_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON submission_freeze_sources FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER submission_freeze_observations_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON submission_freeze_observations FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER submission_freeze_decisions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON submission_freeze_decisions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER submission_freeze_commands_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON submission_freeze_commands FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM submission_branch_configurations)
 OR EXISTS(SELECT 1 FROM submission_freeze_sources)
 OR EXISTS(SELECT 1 FROM submission_freeze_commands)
 OR EXISTS(SELECT 1 FROM operations WHERE kind='submission.capture') THEN
  RAISE EXCEPTION 'submission branch bindings, freeze history and commands must be retained';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE submission_freeze_commands;
DROP TABLE submission_freeze_decisions;
DROP TABLE submission_freeze_observations;
DROP TABLE submission_freeze_sources;
DROP TABLE submission_freeze_heads;
DROP TABLE submission_branch_configurations;
