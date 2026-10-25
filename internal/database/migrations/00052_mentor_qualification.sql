-- +goose Up
ALTER TABLE moderation_final_decisions ADD CONSTRAINT qualification_exact_final_source UNIQUE(event_id,team_id,run_id,signoff_id);
CREATE TABLE mentor_qualification_rounds (
 event_id uuid NOT NULL REFERENCES events(id),
 run_id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>0),
 actor_id uuid NOT NULL REFERENCES people(id),
 action text NOT NULL,
 reason text NOT NULL,
 document jsonb NOT NULL,
 changed_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,run_id,version),
 FOREIGN KEY(event_id,run_id) REFERENCES rubric_assessment_runs(event_id,id)
);
CREATE TABLE mentor_qualification_sources (
 event_id uuid NOT NULL,
 run_id uuid NOT NULL,
 round_version bigint NOT NULL,
 team_id uuid NOT NULL,
 signoff_id uuid NOT NULL,
 PRIMARY KEY(event_id,run_id,round_version,team_id),
 FOREIGN KEY(event_id,run_id,round_version) REFERENCES mentor_qualification_rounds(event_id,run_id,version),
 FOREIGN KEY(event_id,team_id,run_id,signoff_id) REFERENCES moderation_final_decisions(event_id,team_id,run_id,signoff_id),
 FOREIGN KEY(event_id,signoff_id) REFERENCES mentor_scorecard_signoffs(event_id,id)
);
-- +goose StatementBegin
CREATE FUNCTION reject_qualification_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'mentor qualification history is immutable';
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER qualification_rounds_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON mentor_qualification_rounds FOR EACH STATEMENT EXECUTE FUNCTION reject_qualification_mutation();
CREATE TRIGGER qualification_sources_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON mentor_qualification_sources FOR EACH STATEMENT EXECUTE FUNCTION reject_qualification_mutation();
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM mentor_qualification_rounds) THEN
  RAISE EXCEPTION 'cannot remove retained mentor qualification decisions';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE mentor_qualification_sources;
DROP TABLE mentor_qualification_rounds;
ALTER TABLE moderation_final_decisions DROP CONSTRAINT qualification_exact_final_source;
DROP FUNCTION reject_qualification_mutation();
