-- Development DDL. Move into the next actual ordered Goose migration only after
-- the repository barrier and webhook migrations have merged. Never apply alone.
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
 CHECK((approved_by IS NULL)=(approved_at IS NULL)),
 UNIQUE(event_id,team_id,version),
 FOREIGN KEY(event_id,team_id) REFERENCES team_repositories(event_id,team_id)
);
CREATE TABLE repository_access_effects (
 id uuid PRIMARY KEY,
 plan_id uuid NOT NULL REFERENCES repository_access_plans(id),
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
 FOREIGN KEY(event_id,team_id) REFERENCES team_repositories(event_id,team_id)
);
CREATE TABLE repository_access_grants (
 id uuid PRIMARY KEY REFERENCES repository_access_effects(id),
 event_id uuid NOT NULL,
 team_id uuid NOT NULL,
 person_id uuid NOT NULL REFERENCES people(id),
 github_id bigint NOT NULL CHECK(github_id>0),
 link_revision bigint NOT NULL CHECK(link_revision>=0),
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
