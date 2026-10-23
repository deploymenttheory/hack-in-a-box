-- +goose Up
ALTER TABLE inbox_notifications DROP CONSTRAINT inbox_notifications_source_kind_check;
ALTER TABLE inbox_notifications ADD CONSTRAINT inbox_notifications_source_kind_check CHECK(source_kind IN ('campaign','test_self','content','help','clarification','outcome_review','feedback','feedback_followup'));
-- Shared support is independent of private evidence, help state and assessments.
CREATE TABLE feedback_heads (
 event_id uuid NOT NULL REFERENCES events(id), id uuid NOT NULL, team_id uuid NOT NULL,
 category text NOT NULL CHECK(category IN ('setup','logistics','competition_output')),
 current_version bigint NOT NULL CHECK(current_version>0), published_version bigint,
 PRIMARY KEY(event_id,id), FOREIGN KEY(event_id,team_id) REFERENCES hack_teams(event_id,id)
);
CREATE TABLE feedback_versions (
 event_id uuid NOT NULL, id uuid NOT NULL, version bigint NOT NULL CHECK(version>0),
 document jsonb NOT NULL, PRIMARY KEY(event_id,id,version),
 FOREIGN KEY(event_id,id) REFERENCES feedback_heads(event_id,id)
);
ALTER TABLE feedback_heads ADD CONSTRAINT feedback_current_version FOREIGN KEY(event_id,id,current_version) REFERENCES feedback_versions(event_id,id,version) DEFERRABLE INITIALLY DEFERRED;
ALTER TABLE feedback_heads ADD CONSTRAINT feedback_published_version FOREIGN KEY(event_id,id,published_version) REFERENCES feedback_versions(event_id,id,version) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE feedback_action_heads (
 event_id uuid NOT NULL, feedback_id uuid NOT NULL, id uuid NOT NULL,
 current_version bigint NOT NULL CHECK(current_version>0), PRIMARY KEY(event_id,id),
 FOREIGN KEY(event_id,feedback_id) REFERENCES feedback_heads(event_id,id)
);
CREATE TABLE feedback_action_versions (
 event_id uuid NOT NULL, id uuid NOT NULL, version bigint NOT NULL CHECK(version>0),
 document jsonb NOT NULL, PRIMARY KEY(event_id,id,version),
 FOREIGN KEY(event_id,id) REFERENCES feedback_action_heads(event_id,id)
);
ALTER TABLE feedback_action_heads ADD CONSTRAINT feedback_action_current_version FOREIGN KEY(event_id,id,current_version) REFERENCES feedback_action_versions(event_id,id,version) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE feedback_acknowledgements (
 event_id uuid NOT NULL, feedback_id uuid NOT NULL, published_version bigint NOT NULL,
 person_id uuid NOT NULL REFERENCES people(id), acknowledged_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,feedback_id,published_version,person_id),
 FOREIGN KEY(event_id,feedback_id,published_version) REFERENCES feedback_versions(event_id,id,version)
);
CREATE TABLE feedback_settings_versions (
 event_id uuid NOT NULL REFERENCES events(id), version bigint NOT NULL CHECK(version>0),
 setup_seconds bigint NOT NULL CHECK(setup_seconds BETWEEN 60 AND 86400),
 logistics_seconds bigint NOT NULL CHECK(logistics_seconds BETWEEN 60 AND 86400),
 output_seconds bigint NOT NULL CHECK(output_seconds BETWEEN 60 AND 86400),
 actor_id uuid NOT NULL REFERENCES people(id), changed_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,version)
);
CREATE TABLE feedback_escalations (
 event_id uuid NOT NULL, id uuid NOT NULL, action_id uuid NOT NULL, cycle bigint NOT NULL CHECK(cycle>0),
 settings_version bigint NOT NULL CHECK(settings_version>0), recipient_id uuid NOT NULL REFERENCES people(id),
 threshold_seconds bigint NOT NULL CHECK(threshold_seconds BETWEEN 60 AND 86400),
 occurred_at timestamptz NOT NULL, PRIMARY KEY(event_id,action_id,cycle,settings_version,recipient_id), UNIQUE(event_id,id),
 FOREIGN KEY(event_id,action_id) REFERENCES feedback_action_heads(event_id,id)
);
CREATE TABLE feedback_commands (
 event_id uuid NOT NULL REFERENCES events(id), actor_id uuid NOT NULL REFERENCES people(id),
 command_id uuid NOT NULL, target_id uuid NOT NULL, kind text NOT NULL CHECK(kind IN ('feedback','action','ack')),
 payload_digest text NOT NULL, result_version bigint NOT NULL,
 PRIMARY KEY(event_id,actor_id,command_id)
);
CREATE TRIGGER feedback_versions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON feedback_versions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER feedback_action_versions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON feedback_action_versions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER feedback_acknowledgements_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON feedback_acknowledgements FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER feedback_settings_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON feedback_settings_versions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER feedback_escalations_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON feedback_escalations FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER feedback_commands_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON feedback_commands FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM feedback_heads) OR EXISTS(SELECT 1 FROM feedback_settings_versions) OR EXISTS(SELECT 1 FROM inbox_notifications WHERE source_kind IN ('feedback','feedback_followup')) THEN
  RAISE EXCEPTION 'Shared feedback or support history exists; rollback requires an explicit preservation plan';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE feedback_commands, feedback_escalations, feedback_settings_versions, feedback_acknowledgements, feedback_action_versions, feedback_action_heads, feedback_versions, feedback_heads;

ALTER TABLE inbox_notifications DROP CONSTRAINT inbox_notifications_source_kind_check;
ALTER TABLE inbox_notifications ADD CONSTRAINT inbox_notifications_source_kind_check CHECK(source_kind IN ('campaign','test_self','content','help','clarification','outcome_review'));
