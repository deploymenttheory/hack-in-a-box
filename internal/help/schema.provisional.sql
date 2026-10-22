ALTER TABLE policy_resources DROP CONSTRAINT policy_resources_kind_check;
ALTER TABLE policy_resources ADD CONSTRAINT policy_resources_kind_check CHECK(kind IN ('team','assessment','credential','brief','presentation','logistics','media','help'));
-- Provisional owning-domain schema. Move into the assigned Goose migration only
-- after SMTP/Teams/inbox/automation/rubric/observation ordering is confirmed.
CREATE TABLE help_settings (
 event_id uuid PRIMARY KEY REFERENCES events(id),
 revision bigint NOT NULL CHECK(revision>0),
 setup_threshold_seconds bigint NOT NULL CHECK(setup_threshold_seconds BETWEEN 60 AND 86400),
 logistics_threshold_seconds bigint NOT NULL CHECK(logistics_threshold_seconds BETWEEN 60 AND 86400),
 output_threshold_seconds bigint NOT NULL CHECK(output_threshold_seconds BETWEEN 60 AND 86400),
 meeting_hosts text[] NOT NULL DEFAULT '{}'
);
CREATE TABLE help_requests (
 event_id uuid NOT NULL REFERENCES events(id),
 id uuid NOT NULL,
 team_id uuid NOT NULL,
 requester_id uuid NOT NULL REFERENCES people(id),
 revision bigint NOT NULL CHECK(revision>0),
 category text NOT NULL CHECK(category IN ('setup','logistics','competition_output')),
 description text NOT NULL CHECK(length(description) BETWEEN 1 AND 8000),
 location text NOT NULL CHECK(length(location)<=300),
 meeting_url text NOT NULL CHECK(length(meeting_url)<=2048),
 status text NOT NULL CHECK(status IN ('waiting','claimed','resolved','cancelled')),
 assignee_id uuid REFERENCES people(id),
 waiting_cycle bigint NOT NULL CHECK(waiting_cycle>0),
 requested_at timestamptz NOT NULL,
 waiting_since timestamptz NOT NULL,
 first_claimed_at timestamptz,
 claimed_at timestamptz,
 resolved_at timestamptz,
 updated_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,id),
 FOREIGN KEY(event_id,team_id) REFERENCES hack_teams(event_id,id),
 FOREIGN KEY(event_id,id) REFERENCES policy_resources(event_id,id),
 CHECK((status='waiting' AND assignee_id IS NULL AND claimed_at IS NULL) OR (status IN ('claimed','resolved') AND assignee_id IS NOT NULL AND claimed_at IS NOT NULL) OR (status='cancelled' AND assignee_id IS NULL AND claimed_at IS NULL)),
 CHECK((status='resolved')=(resolved_at IS NOT NULL))
);
CREATE INDEX help_requests_queue ON help_requests(event_id,status,waiting_since,id);
CREATE TABLE help_transitions (
 event_id uuid NOT NULL,
 request_id uuid NOT NULL,
 revision bigint NOT NULL,
 action text NOT NULL CHECK(action IN ('create','claim','release','transfer','resolve','reopen','cancel','assignee_unavailable','overdue')),
 status text NOT NULL CHECK(status IN ('waiting','claimed','resolved','cancelled')),
 actor_id uuid REFERENCES people(id),
 assignee_id uuid REFERENCES people(id),
 occurred_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,request_id,revision),
 FOREIGN KEY(event_id,request_id) REFERENCES help_requests(event_id,id)
);
CREATE TABLE help_commands (
 event_id uuid NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 command_id uuid NOT NULL,
 request_id uuid NOT NULL,
 payload_digest text NOT NULL,
 response_revision bigint NOT NULL,
 PRIMARY KEY(event_id,actor_id,command_id),
 FOREIGN KEY(event_id,request_id) REFERENCES help_requests(event_id,id)
);
CREATE TABLE help_followups (
 event_id uuid NOT NULL,
 request_id uuid NOT NULL,
 waiting_cycle bigint NOT NULL,
 settings_revision bigint NOT NULL,
 threshold_seconds bigint NOT NULL CHECK(threshold_seconds BETWEEN 60 AND 86400),
 created_at timestamptz NOT NULL,
 PRIMARY KEY(event_id,request_id,waiting_cycle,settings_revision),
 FOREIGN KEY(event_id,request_id) REFERENCES help_requests(event_id,id)
);
CREATE TRIGGER help_transitions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON help_transitions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER help_commands_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON help_commands FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER help_followups_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON help_followups FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
