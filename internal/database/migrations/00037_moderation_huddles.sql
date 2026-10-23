-- +goose Up
-- Ordered upgrade from actual shared-feedback schema36.
CREATE TABLE moderation_huddle_versions (
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>0),
 run_id uuid NOT NULL,
 document jsonb NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 2000),
 changed_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,id,version),
 FOREIGN KEY(event_id,team_id) REFERENCES hack_teams(event_id,id),
 FOREIGN KEY(event_id,run_id) REFERENCES rubric_assessment_runs(event_id,id)
);
CREATE TABLE moderation_huddle_cards (
 event_id uuid NOT NULL,
 huddle_id uuid NOT NULL,
 huddle_version bigint NOT NULL,
 card_id uuid NOT NULL,
 card_version bigint NOT NULL,
 PRIMARY KEY(event_id,huddle_id,huddle_version,card_id),
 FOREIGN KEY(event_id,huddle_id,huddle_version) REFERENCES moderation_huddle_versions(event_id,id,version),
 FOREIGN KEY(event_id,card_id,card_version) REFERENCES mentor_scorecard_versions(event_id,id,version)
);
CREATE TRIGGER moderation_huddle_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON moderation_huddle_versions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER moderation_huddle_cards_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON moderation_huddle_cards FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TABLE moderation_gap_actions (
 event_id uuid NOT NULL,
 huddle_id uuid NOT NULL,
 huddle_version bigint NOT NULL,
 gap_id uuid NOT NULL,
 action_id uuid NOT NULL,
 action_version bigint NOT NULL,
 feedback_id uuid NOT NULL,
 feedback_version bigint NOT NULL,
 PRIMARY KEY(event_id,huddle_id,huddle_version,gap_id),
 FOREIGN KEY(event_id,huddle_id,huddle_version) REFERENCES moderation_huddle_versions(event_id,id,version),
 FOREIGN KEY(event_id,action_id,action_version) REFERENCES feedback_action_versions(event_id,id,version),
 FOREIGN KEY(event_id,feedback_id,feedback_version) REFERENCES feedback_versions(event_id,id,version)
);
CREATE TRIGGER moderation_gap_actions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON moderation_gap_actions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TABLE moderation_final_decisions (
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 run_id uuid NOT NULL,
 huddle_id uuid NOT NULL,
 huddle_version bigint NOT NULL,
 context_version bigint NOT NULL,
 signoff_id uuid NOT NULL,
 PRIMARY KEY(event_id,team_id,run_id),
 UNIQUE(event_id,signoff_id),
 FOREIGN KEY(event_id,team_id) REFERENCES hack_teams(event_id,id),
 FOREIGN KEY(event_id,run_id) REFERENCES rubric_assessment_runs(event_id,id),
 FOREIGN KEY(event_id,huddle_id,huddle_version) REFERENCES moderation_huddle_versions(event_id,id,version) DEFERRABLE INITIALLY DEFERRED,
 FOREIGN KEY(event_id,huddle_id,context_version) REFERENCES moderation_huddle_versions(event_id,id,version),
 FOREIGN KEY(event_id,signoff_id) REFERENCES mentor_scorecard_signoffs(event_id,id)
);
CREATE TRIGGER moderation_final_decision_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON moderation_final_decisions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE INDEX moderation_huddles_team ON moderation_huddle_versions(event_id,team_id,id,version DESC);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM moderation_huddle_versions) OR EXISTS(SELECT 1 FROM moderation_huddle_cards) OR EXISTS(SELECT 1 FROM moderation_gap_actions) OR EXISTS(SELECT 1 FROM moderation_final_decisions) THEN
  RAISE EXCEPTION 'Private moderation or canonical final history exists; rollback requires an explicit preservation plan';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE moderation_final_decisions, moderation_gap_actions, moderation_huddle_cards, moderation_huddle_versions;
