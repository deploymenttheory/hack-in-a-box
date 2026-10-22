-- +goose Up
CREATE TABLE communication_versions (
 event_id uuid NOT NULL REFERENCES events(id),
 id uuid NOT NULL,
 kind text NOT NULL CHECK(kind IN ('template','audience')),
 version bigint NOT NULL CHECK(version>0),
 document jsonb NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(event_id,id,kind,version)
);
CREATE TABLE communication_previews (
 event_id uuid NOT NULL REFERENCES events(id),
 id uuid NOT NULL,
 template_id uuid NOT NULL,
 template_version bigint NOT NULL,
 audience_id uuid NOT NULL,
 audience_version bigint NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 snapshot jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(event_id,id)
);
CREATE TABLE communication_approvals (
 event_id uuid NOT NULL,
 preview_id uuid NOT NULL,
 actor_id uuid NOT NULL REFERENCES people(id),
 reason text NOT NULL CHECK(length(reason) BETWEEN 1 AND 2000),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(event_id,preview_id),
 FOREIGN KEY(event_id,preview_id) REFERENCES communication_previews(event_id,id)
);
-- Future domain owners publish only official routing facts through their own
-- transaction; private questions, assessment prose and scores have no columns.
CREATE TABLE communication_facts (
 event_id uuid NOT NULL REFERENCES events(id),
 id uuid NOT NULL,
 kind text NOT NULL CHECK(kind IN ('help','clarification','rubric','outcome_review','results')),
 revision bigint NOT NULL CHECK(revision>0),
 owner_id uuid REFERENCES people(id),
 resource_id uuid,
 status text NOT NULL CHECK(status IN ('open','resolved','published','withdrawn')),
 PRIMARY KEY(event_id,id),
 FOREIGN KEY(event_id,resource_id) REFERENCES policy_resources(event_id,id)
);
CREATE TRIGGER communication_versions_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON communication_versions FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER communication_previews_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON communication_previews FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
CREATE TRIGGER communication_approvals_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON communication_approvals FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
-- +goose Down
DROP TABLE communication_facts;
DROP TABLE communication_approvals;
DROP TABLE communication_previews;
DROP TABLE communication_versions;
