-- +goose Up
CREATE TABLE communication_automation_settings (
 event_id uuid PRIMARY KEY REFERENCES events(id),
 paused boolean NOT NULL DEFAULT false,
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 actor_id uuid NOT NULL REFERENCES people(id),
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 2000),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE communication_automation_versions (
 event_id uuid NOT NULL REFERENCES events(id),
 id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>0),
 campaign_id uuid NOT NULL,
 campaign_version bigint NOT NULL,
 document jsonb NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 2000),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(event_id,id,version),
 FOREIGN KEY(event_id,campaign_id,campaign_version) REFERENCES communication_campaign_versions(event_id,id,version)
);
CREATE TABLE communication_publication_approvals (
 approved boolean NOT NULL,
 preview_plan_id uuid,
 event_id uuid NOT NULL REFERENCES events(id),
 id uuid NOT NULL,
 campaign_id uuid NOT NULL,
 campaign_version bigint NOT NULL,
 source_id uuid NOT NULL,
 source_version bigint NOT NULL CHECK(source_version>0),
 preview_id uuid NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 document jsonb NOT NULL,
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 2000),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(event_id,id),
 FOREIGN KEY(event_id,campaign_id,campaign_version) REFERENCES communication_campaign_versions(event_id,id,version),
 FOREIGN KEY(event_id,preview_id) REFERENCES communication_previews(event_id,id),
 FOREIGN KEY(event_id,source_id,source_version) REFERENCES content_versions(event_id,content_id,version)
);
CREATE TRIGGER communication_publication_approvals_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON communication_publication_approvals FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TABLE communication_occurrences (
 review_revision bigint NOT NULL DEFAULT 0 CHECK(review_revision>=0),
 channel_reports jsonb NOT NULL DEFAULT '{}'::jsonb,
 event_id uuid NOT NULL REFERENCES events(id),
 id uuid NOT NULL,
 binding_id uuid NOT NULL,
 binding_version bigint NOT NULL,
 source_kind text NOT NULL,
 source_id uuid NOT NULL,
 source_revision bigint NOT NULL CHECK(source_revision>0),
 source_key text NOT NULL,
 due_at timestamptz NOT NULL,
 deadline timestamptz NOT NULL,
 scope_binding text NOT NULL,
 state text NOT NULL CHECK(state IN ('pending','paused','missed','blocked','queued','partially_queued','cancelled')),
 reason text NOT NULL DEFAULT '',
 review_until timestamptz,
 operation_id uuid UNIQUE REFERENCES operations(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(event_id,id),
 FOREIGN KEY(event_id,binding_id,binding_version) REFERENCES communication_automation_versions(event_id,id,version),
 UNIQUE(event_id,binding_id,binding_version,source_key)
);
CREATE INDEX communication_occurrences_source ON communication_occurrences(event_id,binding_id,binding_version,source_kind,source_id,source_revision);
CREATE TABLE communication_automation_history (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 event_id uuid NOT NULL REFERENCES events(id),
 occurrence_id uuid,
 binding_id uuid,
 actor_id uuid REFERENCES people(id),
 action text NOT NULL,
 reason text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(event_id,occurrence_id) REFERENCES communication_occurrences(event_id,id)
);
CREATE TRIGGER communication_automation_versions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON communication_automation_versions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER communication_automation_history_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON communication_automation_history FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
ALTER TABLE operations ADD COLUMN retained_occurrence_id uuid;
ALTER TABLE operations ADD CONSTRAINT operation_retained_occurrence FOREIGN KEY(event_id,retained_occurrence_id) REFERENCES communication_occurrences(event_id,id);
-- +goose StatementBegin
CREATE FUNCTION retained_communication_occurrence(e uuid,o uuid,at_time timestamptz) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(
 SELECT 1 FROM communication_occurrences co
 JOIN communication_automation_versions av ON av.event_id=co.event_id AND av.id=co.binding_id AND av.version=co.binding_version
 JOIN events ev ON ev.id=co.event_id
 LEFT JOIN event_privacy ep ON ep.event_id=ev.id
 JOIN communication_facts f ON f.event_id=co.event_id AND f.id=co.source_id AND f.revision=co.source_revision
 JOIN people recipient ON recipient.id=f.owner_id AND recipient.active
 JOIN event_memberships authority ON authority.event_id=av.event_id AND authority.person_id=av.actor_id AND authority.role='organiser' AND authority.active
 JOIN people approver ON approver.id=av.actor_id AND approver.active
 WHERE co.event_id=e AND co.id=o AND co.source_kind='outcome.follow_up'
 AND co.state IN ('pending','queued','partially_queued') AND COALESCE(co.review_until,co.deadline)>=at_time
 AND av.version=(SELECT max(v.version) FROM communication_automation_versions v WHERE v.event_id=e AND v.id=av.id)
 AND COALESCE((av.document->>'cancelled')::boolean,false)=false AND av.document->>'approved_at' IS NOT NULL
 AND av.document->'definition'->>'trigger'='outcome.follow_up'
 AND (av.document->'definition'->>'retain_after_archive')::boolean=true
 AND (av.document->'definition'->>'retain_until')::timestamptz>at_time
 AND ev.stage IN ('completed','archived') AND ev.completed_at IS NOT NULL
 AND (av.document->'definition'->>'retain_until')::timestamptz<=ev.completed_at+make_interval(days=>COALESCE(ep.retention_days,90))
 AND at_time<ev.completed_at+make_interval(days=>COALESCE(ep.retention_days,90))
 AND f.kind='outcome_review' AND f.status='open' AND f.owner_id IS NOT NULL
 );
$$;
-- +goose StatementEnd
-- Typed approval provenance stays attached to immutable message/recipient data.
ALTER TABLE smtp_campaign_messages ADD COLUMN scope_approval jsonb GENERATED ALWAYS AS(snapshot->'scope_approval') STORED;
ALTER TABLE inbox_notifications ADD COLUMN scope_approval jsonb GENERATED ALWAYS AS(provenance->'scope_approval') STORED;
-- Scoped Teams intents coexist with the original one-shot campaign route.
ALTER TABLE teams_workflow_deliveries ADD COLUMN occurrence_id uuid;
ALTER TABLE teams_workflow_deliveries ADD COLUMN scope_binding text NOT NULL DEFAULT '';
ALTER TABLE teams_workflow_deliveries ADD COLUMN scope_approval jsonb NOT NULL DEFAULT 'null'::jsonb;
DROP INDEX teams_workflow_campaign_once;
CREATE UNIQUE INDEX teams_workflow_campaign_once ON teams_workflow_deliveries(event_id,campaign_id,campaign_version) WHERE kind='broadcast' AND occurrence_id IS NULL;
CREATE UNIQUE INDEX teams_workflow_occurrence_once ON teams_workflow_deliveries(event_id,occurrence_id) WHERE occurrence_id IS NOT NULL;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM communication_automation_settings) OR EXISTS(SELECT 1 FROM communication_automation_history) OR EXISTS(SELECT 1 FROM communication_publication_approvals) OR EXISTS(SELECT 1 FROM communication_automation_versions) OR EXISTS(SELECT 1 FROM communication_occurrences) THEN
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
