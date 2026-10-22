-- Unnumbered development schema. Final normal migration follows actual merged103.
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
