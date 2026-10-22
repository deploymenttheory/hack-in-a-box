-- Provisional rollback retained until a real ordered migration number is assigned.
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM communication_publication_approvals) OR EXISTS(SELECT 1 FROM communication_automation_versions) OR EXISTS(SELECT 1 FROM communication_occurrences) THEN
  RAISE EXCEPTION 'Automation intent/history exists; preserve it before rollback';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE inbox_notifications DROP COLUMN scope_approval;
ALTER TABLE smtp_campaign_messages DROP COLUMN scope_approval;
DROP INDEX teams_workflow_occurrence_once;
DROP INDEX teams_workflow_campaign_once;
ALTER TABLE teams_workflow_deliveries DROP COLUMN scope_approval;
ALTER TABLE teams_workflow_deliveries DROP COLUMN scope_binding;
ALTER TABLE teams_workflow_deliveries DROP COLUMN occurrence_id;
CREATE UNIQUE INDEX teams_workflow_campaign_once ON teams_workflow_deliveries(event_id,campaign_id,campaign_version) WHERE kind='broadcast';
ALTER TABLE operations DROP COLUMN retained_occurrence_id;
DROP FUNCTION retained_communication_occurrence(uuid,uuid,timestamptz);
DROP TABLE communication_automation_history;
DROP TABLE communication_occurrences;
DROP TABLE communication_publication_approvals;
DROP TABLE communication_automation_versions;
DROP TABLE communication_automation_settings;
