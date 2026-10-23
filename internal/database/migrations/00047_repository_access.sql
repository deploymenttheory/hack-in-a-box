-- +goose Up
CREATE TABLE repository_access_plans (
 id uuid PRIMARY KEY,
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 version bigint NOT NULL CHECK(version>0),
 fingerprint text NOT NULL,
 definition jsonb NOT NULL CHECK(jsonb_typeof(definition)='object'),
 created_by uuid NOT NULL REFERENCES people(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 approved_by uuid REFERENCES people(id),
 approved_at timestamptz,
 approval_reason text NOT NULL DEFAULT '',
 CHECK((approved_at IS NULL AND approval_reason='') OR (approved_at IS NOT NULL AND length(btrim(approval_reason))>0)),
 CHECK((approved_by IS NULL)=(approved_at IS NULL)),
 UNIQUE(event_id,team_id,version),
 UNIQUE(id,event_id,team_id),
 FOREIGN KEY(event_id,team_id) REFERENCES team_repositories(event_id,team_id)
);
CREATE TABLE repository_access_effects (
 id uuid PRIMARY KEY,
 plan_id uuid NOT NULL,
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 definition jsonb NOT NULL CHECK(jsonb_typeof(definition)='object'),
 attempted boolean NOT NULL DEFAULT false,
 dispatch_id uuid,
 state text NOT NULL DEFAULT 'planned' CHECK(state IN('planned','ambiguous','delivered','failed','cancelled')),
 diagnostic text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(attempted=(dispatch_id IS NOT NULL)),
 FOREIGN KEY(plan_id,event_id,team_id) REFERENCES repository_access_plans(id,event_id,team_id),
 UNIQUE(id,event_id,team_id),
 FOREIGN KEY(event_id,team_id) REFERENCES team_repositories(event_id,team_id)
);
CREATE TABLE repository_access_grants (
 id uuid PRIMARY KEY,
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 FOREIGN KEY(id,event_id,team_id) REFERENCES repository_access_effects(id,event_id,team_id),
 person_id uuid NOT NULL REFERENCES people(id),
 github_id bigint NOT NULL CHECK(github_id>0),
 link_revision bigint NOT NULL CHECK(link_revision>=0),
 coverage_start timestamptz,
 coverage_end timestamptz,
 CHECK((coverage_start IS NULL)=(coverage_end IS NULL)),
 CHECK(coverage_end IS NULL OR coverage_end>coverage_start),
 role text NOT NULL CHECK(role IN('competitor','mentor')),
 permission text NOT NULL CHECK(permission IN('push','pull')),
 app_managed boolean NOT NULL DEFAULT false,
 state text NOT NULL CHECK(state IN('ambiguous','pending','accepted','expired','failed','revoked')),
 invitation_id bigint CHECK(invitation_id>0),
 diagnostic text NOT NULL DEFAULT '',
 observed_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(event_id,team_id) REFERENCES team_repositories(event_id,team_id)
);
CREATE UNIQUE INDEX repository_access_current_account ON repository_access_grants(event_id,team_id,github_id) WHERE state IN('ambiguous','pending','accepted');
CREATE TABLE repository_access_observations (
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 snapshot jsonb NOT NULL CHECK(jsonb_typeof(snapshot)='object'),
 observed_at timestamptz,
 diagnostic text NOT NULL DEFAULT '',
 PRIMARY KEY(event_id,team_id),
 FOREIGN KEY(event_id,team_id) REFERENCES team_repositories(event_id,team_id)
);
-- +goose StatementBegin
CREATE FUNCTION protect_repository_access_history() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='repository_access_plans' THEN
  IF (NEW.id,NEW.event_id,NEW.team_id,NEW.version,NEW.fingerprint,NEW.definition,NEW.created_by,NEW.created_at) IS DISTINCT FROM
     (OLD.id,OLD.event_id,OLD.team_id,OLD.version,OLD.fingerprint,OLD.definition,OLD.created_by,OLD.created_at)
   OR (OLD.approved_at IS NOT NULL AND (NEW.approved_by,NEW.approved_at,NEW.approval_reason) IS DISTINCT FROM (OLD.approved_by,OLD.approved_at,OLD.approval_reason))
  THEN RAISE EXCEPTION 'repository access review history is immutable'; END IF;
 ELSIF TG_TABLE_NAME='repository_access_effects' THEN
  IF (NEW.id,NEW.plan_id,NEW.event_id,NEW.team_id,NEW.definition,NEW.created_at) IS DISTINCT FROM
     (OLD.id,OLD.plan_id,OLD.event_id,OLD.team_id,OLD.definition,OLD.created_at)
   OR (OLD.attempted AND (NOT NEW.attempted OR NEW.dispatch_id IS DISTINCT FROM OLD.dispatch_id OR NEW.state='planned'))
  THEN RAISE EXCEPTION 'repository access dispatch history is immutable'; END IF;
 ELSIF TG_TABLE_NAME='repository_access_grants' THEN
  IF (NEW.id,NEW.event_id,NEW.team_id,NEW.person_id,NEW.github_id,NEW.link_revision,NEW.coverage_start,NEW.coverage_end,NEW.role,NEW.permission,NEW.created_at) IS DISTINCT FROM
     (OLD.id,OLD.event_id,OLD.team_id,OLD.person_id,OLD.github_id,OLD.link_revision,OLD.coverage_start,OLD.coverage_end,OLD.role,OLD.permission,OLD.created_at)
   OR (OLD.app_managed AND NOT NEW.app_managed)
   OR (OLD.invitation_id IS NOT NULL AND NEW.invitation_id IS DISTINCT FROM OLD.invitation_id)
  THEN RAISE EXCEPTION 'repository access grant provenance is immutable'; END IF;
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER repository_access_plans_identity BEFORE UPDATE ON repository_access_plans FOR EACH ROW EXECUTE FUNCTION protect_repository_access_history();
CREATE TRIGGER repository_access_effects_identity BEFORE UPDATE ON repository_access_effects FOR EACH ROW EXECUTE FUNCTION protect_repository_access_history();
CREATE TRIGGER repository_access_grants_identity BEFORE UPDATE ON repository_access_grants FOR EACH ROW EXECUTE FUNCTION protect_repository_access_history();
CREATE TRIGGER repository_access_plans_retained BEFORE DELETE OR TRUNCATE ON repository_access_plans FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER repository_access_effects_retained BEFORE DELETE OR TRUNCATE ON repository_access_effects FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER repository_access_grants_retained BEFORE DELETE OR TRUNCATE ON repository_access_grants FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM repository_access_plans)
 OR EXISTS(SELECT 1 FROM repository_access_effects)
 OR EXISTS(SELECT 1 FROM repository_access_grants)
 OR EXISTS(SELECT 1 FROM repository_access_observations)
 OR EXISTS(SELECT 1 FROM operations WHERE kind IN ('repository.access','repository.access.poll')) THEN
  RAISE EXCEPTION 'Repository access history exists; preserve it before rollback';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE repository_access_observations;
DROP TABLE repository_access_grants;
DROP TABLE repository_access_effects;
DROP TABLE repository_access_plans;
DROP FUNCTION protect_repository_access_history();
