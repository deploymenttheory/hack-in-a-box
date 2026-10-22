-- +goose Up
ALTER TABLE files ADD COLUMN team_id uuid;
ALTER TABLE files ADD CONSTRAINT files_team_scope FOREIGN KEY(event_id,team_id) REFERENCES hack_teams(event_id,id);
CREATE TABLE submission_states (
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 revision bigint NOT NULL DEFAULT 0 CHECK(revision >= 0),
 active_revision bigint,
 outcome text NOT NULL DEFAULT 'open' CHECK(outcome IN ('open','frozen','not_submitted','withdrawn')),
 frozen_at timestamptz,
 PRIMARY KEY(event_id,team_id),
 FOREIGN KEY(event_id,team_id) REFERENCES competition_entries(event_id,team_id),
 CHECK(outcome != 'frozen' OR (active_revision IS NOT NULL AND frozen_at IS NOT NULL))
);
CREATE TABLE submission_revisions (
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 revision bigint NOT NULL CHECK(revision > 0),
 actor_id uuid NOT NULL REFERENCES people(id),
 submitted_at timestamptz NOT NULL,
 late boolean NOT NULL,
 reason text NOT NULL DEFAULT '',
 document jsonb NOT NULL,
 PRIMARY KEY(event_id,team_id,revision),
 FOREIGN KEY(event_id,team_id) REFERENCES submission_states(event_id,team_id)
);
ALTER TABLE submission_states ADD CONSTRAINT active_submission_revision
 FOREIGN KEY(event_id,team_id,active_revision) REFERENCES submission_revisions(event_id,team_id,revision);
CREATE TABLE submission_changes (
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 revision bigint NOT NULL,
 actor_id uuid REFERENCES people(id),
 action text NOT NULL CHECK(action IN ('submitted','frozen','not_submitted','withdrawn','reviewed')),
 active_revision bigint,
 review_status text NOT NULL DEFAULT '',
 reason text NOT NULL DEFAULT '',
 occurred_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,team_id,revision),
 FOREIGN KEY(event_id,team_id) REFERENCES submission_states(event_id,team_id),
 FOREIGN KEY(event_id,team_id,active_revision) REFERENCES submission_revisions(event_id,team_id,revision)
);
CREATE TABLE submission_drafts (
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 revision bigint NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 updated_at timestamptz NOT NULL,
 evidence jsonb NOT NULL,
 PRIMARY KEY(event_id,team_id),
 FOREIGN KEY(event_id,team_id) REFERENCES competition_entries(event_id,team_id)
);
CREATE TABLE submission_file_references (
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 submission_revision bigint NOT NULL,
 file_id uuid NOT NULL,
 audience text[] NOT NULL,
 PRIMARY KEY(event_id,team_id,submission_revision,file_id),
 FOREIGN KEY(event_id,team_id,submission_revision) REFERENCES submission_revisions(event_id,team_id,revision),
 FOREIGN KEY(event_id,file_id) REFERENCES files(event_id,id)
);
CREATE TRIGGER submission_revisions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON submission_revisions
 FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER submission_changes_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON submission_changes
 FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER submission_files_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON submission_file_references
 FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
INSERT INTO policy_resources(event_id,id,kind) SELECT event_id,team_id,'team' FROM competition_entries ON CONFLICT DO NOTHING;
INSERT INTO policy_resources(event_id,id,kind) SELECT event_id,id,'assessment' FROM competition_entries ON CONFLICT DO NOTHING;

-- +goose Down
DROP TABLE submission_file_references;
DROP TABLE submission_changes;
DROP TABLE submission_drafts;
ALTER TABLE submission_states DROP CONSTRAINT active_submission_revision;
DROP TABLE submission_revisions;
DROP TABLE submission_states;
ALTER TABLE files DROP CONSTRAINT files_team_scope;
ALTER TABLE files DROP COLUMN team_id;
