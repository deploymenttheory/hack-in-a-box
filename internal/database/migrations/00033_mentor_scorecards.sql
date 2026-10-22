-- +goose Up
-- Ordered upgrade from actual merged help schema32. Existing private history remains unchanged.
CREATE TABLE scorecard_checkpoint_versions (
 event_id uuid NOT NULL REFERENCES events(id),
 id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>0),
 run_id uuid NOT NULL,
 document jsonb NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 reason text NOT NULL,
 created_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,id,version),
 FOREIGN KEY(event_id,run_id) REFERENCES rubric_assessment_runs(event_id,id)
);
CREATE TABLE mentor_scorecard_versions (
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 checkpoint_id uuid NOT NULL,
 checkpoint_version bigint NOT NULL,
 id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>0),
 author_id uuid NOT NULL REFERENCES people(id),
 run_id uuid NOT NULL,
 rubric_id uuid NOT NULL,
 rubric_version bigint NOT NULL,
 submission_revision bigint,
 document jsonb NOT NULL,
 created_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,id,version),
 UNIQUE(event_id,team_id,checkpoint_id,checkpoint_version,author_id,version),
 FOREIGN KEY(event_id,team_id) REFERENCES hack_teams(event_id,id),
 FOREIGN KEY(event_id,checkpoint_id,checkpoint_version) REFERENCES scorecard_checkpoint_versions(event_id,id,version),
 FOREIGN KEY(event_id,run_id) REFERENCES rubric_assessment_runs(event_id,id),
 FOREIGN KEY(event_id,rubric_id,rubric_version) REFERENCES rubric_approvals(event_id,rubric_id,version),
 FOREIGN KEY(event_id,team_id,submission_revision) REFERENCES submission_revisions(event_id,team_id,revision)
);
CREATE INDEX mentor_scorecards_owner ON mentor_scorecard_versions(event_id,team_id,author_id,id,version DESC);
CREATE TABLE mentor_scorecard_signoffs (
 event_id uuid NOT NULL,
 id uuid NOT NULL,
 team_id uuid NOT NULL,
 run_id uuid NOT NULL,
 context_id uuid NOT NULL,
 context_version bigint NOT NULL CHECK(context_version>0),
 actor_id uuid NOT NULL REFERENCES people(id),
 proof jsonb NOT NULL,
 created_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,id),
 UNIQUE(event_id,context_id,context_version),
 FOREIGN KEY(event_id,team_id) REFERENCES hack_teams(event_id,id),
 FOREIGN KEY(event_id,run_id) REFERENCES rubric_assessment_runs(event_id,id)
);
CREATE TABLE mentor_scorecard_signoff_cards (
 event_id uuid NOT NULL,
 signoff_id uuid NOT NULL,
 card_id uuid NOT NULL,
 card_version bigint NOT NULL,
 PRIMARY KEY(event_id,signoff_id,card_id),
 FOREIGN KEY(event_id,signoff_id) REFERENCES mentor_scorecard_signoffs(event_id,id),
 FOREIGN KEY(event_id,card_id,card_version) REFERENCES mentor_scorecard_versions(event_id,id,version)
);
CREATE TRIGGER scorecard_checkpoint_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON scorecard_checkpoint_versions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER mentor_scorecard_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON mentor_scorecard_versions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER mentor_scorecard_signoff_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON mentor_scorecard_signoffs FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER mentor_scorecard_signoff_cards_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON mentor_scorecard_signoff_cards FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM scorecard_checkpoint_versions) OR EXISTS(SELECT 1 FROM mentor_scorecard_versions) OR EXISTS(SELECT 1 FROM mentor_scorecard_signoffs) THEN
  RAISE EXCEPTION 'Scorecard configuration or immutable assessment history exists; preserve it before rollback';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE mentor_scorecard_signoff_cards;
DROP TABLE mentor_scorecard_signoffs;
DROP TABLE mentor_scorecard_versions;
DROP TABLE scorecard_checkpoint_versions;
