-- +goose Up
-- Ordered upgrade from inbox schema30; existing event and private resources remain unchanged.
CREATE TABLE mentor_observation_versions (
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>0),
 author_id uuid NOT NULL REFERENCES people(id),
 observed_at timestamptz NOT NULL,
 changed_at timestamptz NOT NULL,
 category text NOT NULL CHECK(octet_length(category) BETWEEN 1 AND 128),
 evidence text NOT NULL CHECK(octet_length(evidence) BETWEEN 1 AND 30000),
 outcome text NOT NULL CHECK(octet_length(outcome)<=30000),
 feedback_reference text NOT NULL DEFAULT '' CHECK(octet_length(feedback_reference)<=256),
 PRIMARY KEY(event_id,team_id,id,version),
 FOREIGN KEY(event_id,team_id) REFERENCES hack_teams(event_id,id),
 CHECK(changed_at>=observed_at)
);
CREATE TABLE mentor_team_status_versions (
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>0),
 actor_id uuid NOT NULL REFERENCES people(id),
 state text NOT NULL CHECK(state IN ('on_track','at_risk','blocked')),
 reason text NOT NULL CHECK(octet_length(reason) BETWEEN 1 AND 2000),
 next_action text NOT NULL CHECK(octet_length(next_action)<=2000),
 changed_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,team_id,version),
 FOREIGN KEY(event_id,team_id) REFERENCES hack_teams(event_id,id)
);
-- +goose StatementBegin
CREATE FUNCTION protect_mentor_observation_history() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'Mentor observation and operational status history is append-only';
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER mentor_observation_append_only BEFORE UPDATE OR DELETE ON mentor_observation_versions FOR EACH ROW EXECUTE FUNCTION protect_mentor_observation_history();
CREATE TRIGGER mentor_status_append_only BEFORE UPDATE OR DELETE ON mentor_team_status_versions FOR EACH ROW EXECUTE FUNCTION protect_mentor_observation_history();
CREATE TRIGGER mentor_observation_no_truncate BEFORE TRUNCATE ON mentor_observation_versions FOR EACH STATEMENT EXECUTE FUNCTION protect_mentor_observation_history();
CREATE TRIGGER mentor_status_no_truncate BEFORE TRUNCATE ON mentor_team_status_versions FOR EACH STATEMENT EXECUTE FUNCTION protect_mentor_observation_history();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM mentor_observation_versions) OR EXISTS(SELECT 1 FROM mentor_team_status_versions) THEN
  RAISE EXCEPTION 'Private mentor history exists; preserve it before rollback';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE mentor_team_status_versions;
DROP TABLE mentor_observation_versions;
DROP FUNCTION protect_mentor_observation_history();
